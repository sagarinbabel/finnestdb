# FSRS scheduler rollout - staging-style validation (2026-07-04)

Executes the gate in [`../DEPLOYMENT.md`](../DEPLOYMENT.md) "FSRS scheduler
rollout" against the narrow FSRS scope in
[`../srs-deck-spec.md`](../srs-deck-spec.md) "Alpha FSRS Scope" / "Implemented
FSRS state model". **Verdict: all drills GREEN → FSRS enabled by default** (flag
becomes opt-OUT; see the CHANGELOG entry and DEPLOYMENT rollout section).

Harness: `internal/store/fsrs_validation_test.go` (stays in the suite; runs on
in-memory / temp DBs; never writes the shared `finnestdb.db`). It drives the
scheduler through `recordReviewAnswerAt(..., fsrsEnabled, now)` - the same seam
the runtime uses via `RecordReviewAnswer` → `FSRSEnabled()` - with a
deterministic injected clock.

Reproduce:

```
go test ./internal/store/ -run TestFSRSValidation -count=1 -v
```

## Verdict summary

| Drill | Test | Result |
|-------|------|--------|
| 1. Seeded-history validation | `TestFSRSValidationSeededHistory` | PASS |
| 1b. Due-queue + daily-new gate | `TestFSRSValidationDueQueueAndNewLimit` | PASS |
| 2. Migration correctness at scale | `TestFSRSValidationMigrationAtScale` | PASS |
| 3. Rollback drill | `TestFSRSValidationRollbackDrill` | PASS |
| 4. Real-DB smoke (read-mostly) | `TestFSRSValidationRealDBSmoke` | PASS |

## Drill 1 - seeded-history validation

Four realistic prior-state shapes plus a NULL-state card, flag ON, rated at a
fixed `now = 2026-07-04 09:00Z`. Scheduled interval (days from `now`) per
button; the FSRS invariant **Again < Hard < Good < Easy** must hold on every
shape.

| Seed shape | Again | Hard | Good | Easy | Ordering |
|------------|------:|-----:|-----:|-----:|:--------:|
| new (NULL state) | 0.0007 | 0.0035 | 0.0069 | 16 | ✅ |
| learning mid-step (`step 1/streak 1`, ~7h interval) | 0.0035 | 1 | 2 | 3 | ✅ |
| mature long (`step 5/streak 12`, ~59d interval) | 0.0035 | 75 | 130 | 270 | ✅ |
| legacy short (`step 3/streak 4`, ~6d interval) | 0.0035 | 8 | 17 | 38 | ✅ |

Notes:
- New-card Again/Hard/Good stay on the FSRS **learning ladder** (~1m / ~5m /
  ~10m); Easy graduates straight to Review (~16d). This is expected FSRS
  behavior, not a bug.
- Mature/legacy seeds derive a Review-state card from the observed interval, so
  Good/Easy produce interval growth proportional to the seeded stability
  (130d / 270d for the ~59d mature seed) - sane, not explosive.

**Monotonic stability growth on a Good streak** (NULL card, each Good applied on
its due date):

| Rep | State | Stability | Next interval (days) |
|----:|:-----:|----------:|---------------------:|
| 1 | Learning | 3.17 | 0.007 |
| 2 | Review | 4.47 | 4 |
| 3 | Review | 14.22 | 14 |
| 4 | Review | 43.73 | 44 |
| 5 | Review | 124.80 | 125 |
| 6 | Review | 328.47 | 328 |

Stability is strictly increasing across the streak. ✅

**State-integrity assertions (all shapes, all ratings):** after every rating the
persisted `fsrs_json` is a valid versioned FSRS payload (never NULL/corrupt) and
`next_due` / `last_answer_at` / `introduced_at` are all populated. ✅

### Drill 1b - due-queue + daily-new-card gate (scheduler-agnostic)

| Property | Assertion | Result |
|----------|-----------|:------:|
| Daily new-card gate | `new_per_day=2`; introducing 2 fresh cards drives `remainingNewCardsToday`→0; a 3rd un-introduced new card is withheld by `GetNextReviewCard` | PASS |
| Due-queue ordering | An FSRS-scheduled card forced overdue (`next_due` < now) is surfaced; a future-due one is withheld - queue keys off `next_due` regardless of which scheduler wrote it | PASS |

## Drill 2 - migration correctness at scale

1000 cards seeded across three state shapes (≈⅓ NULL, ⅓ legacy short-interval,
⅓ mature long-interval). Flag ON. A deterministic sample (every 7th card, 142
cards) is rated once with Good.

| Check | Result |
|-------|:------:|
| Lazy migration touches **only rated** cards - every un-rated card's `fsrs_json` is byte-identical to its seed (no bulk rewrite) | PASS (0 un-rated cards mutated) |
| Rated NULL card → fresh FSRS card, exactly 1 rep | PASS |
| Rated legacy/mature card → Review-state seed, reps ≥ 2 (legacy streak carried + this rating) | PASS |
| Derived seed is conservative: `deriveFSRSCard` sets `Stability` = observed interval (`next_due − last_answer_at`, days), state = Review | PASS (legacy ~6d and mature ~59d seeds re-derived and matched to ±0.001d) |

## Drill 3 - rollback drill (round trip)

| Phase | Action | Assertion | Result |
|-------|--------|-----------|:------:|
| A | Flag ON, two on-time Good reviews | FSRS payload persisted; multi-day interval builds (4d after rep 2) | PASS |
| B | Flag OFF, rate Good (rollback) | Writes a **legacy step payload** (not FSRS); step derived from the FSRS interval (`step 1`, not 0 - progress kept); `next_due` in the future (7d out) | PASS |
| C | Flag ON again | FSRS resumes cleanly; new versioned payload + populated `next_due` | PASS |

The flag-off byte-identical regression pin
(`TestRecordReviewAnswerFlagOffByteIdenticalToStepScheduler`) still exists and
passes - it guarantees the rollback path produces exactly what the step
scheduler would, so the env override remains a real, safe rollback lever.

## Drill 4 - real-DB smoke (read-mostly)

The shared `finnestdb.db` (5.2 GB) is **ATTACHed read-only** (`mode=ro`); a
bounded sample of real `cards`/`card_state` rows is copied into a temp DB and
re-driven. The shared file is never written.

Sampled `card_state` shape distribution (first 5000 non-MWE cards; 4031
returned):

| Shape | Count |
|-------|------:|
| NULL state | 4028 |
| Legacy `{step,streak}` | 3 |
| Existing FSRS payload | 0 |
| Rows with `next_due` | 3 |
| Rows with `last_answer_at` | 3 |

Drill: 4031 real cards recreated with their exact state; 500 rated through
FSRS then rollback.

| Check | Result |
|-------|:------:|
| Every FSRS rating persisted a valid payload (0 corrupt / NULL) | PASS (0/500) |
| `next_due` populated after every FSRS rating | PASS |
| Rollback rating wrote a valid step payload (no corruption) on real shapes | PASS |

This confirms the real DB is overwhelmingly fresh (NULL) state pre-launch - the
common path - with a few legacy rows also exercised. No shape drift vs. the
synthetic seeds surfaced.

> The smoke drill **skips** (does not fail) when the shared DB is absent, so CI
> without the 5 GB artifact stays green; on a host with the DB it runs and must
> pass.

## Conclusion

All five drills pass. Interval progression is sane for all four buttons across
new / learning / mature / legacy / NULL states, stability grows monotonically on
Good streaks, the due queue and daily-new-card gate stay scheduler-agnostic,
lazy migration only touches rated cards with conservative derived seeds, and the
rollback round trip preserves progress without corruption on both synthetic and
real row shapes. **FSRS is enabled by default in this PR**; the
`FINNESTDB_FSRS_ENABLED=0` env override is retained as the rollback lever.
