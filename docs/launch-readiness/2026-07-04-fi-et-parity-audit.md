# FI/ET Equal-Status Parity Audit — 2026-07-04

Scope: the "Equal-Status Alpha Gate" journey-first audit from
[`docs/CROSS_LANGUAGE_STRATEGY.md`](../CROSS_LANGUAGE_STRATEGY.md#equal-status-alpha-gate),
required by the [`TODO.md` "FI/ET equal-status parity audit"](../../TODO.md#public-alpha-gates)
gate and the [`docs/GO_LIVE_CHECKLIST.md`](../GO_LIVE_CHECKLIST.md#runtime-reproducibility-and-data-readiness)
equal-status bullet.

Run in a fresh worktree at `main` (branch `docs/fi-et-parity-audit`), against
the symlinked production-size local DB (`finnestdb.db`: 26,826,071 FI forms /
6,256,916 ET forms) with FST tables, Ekilex shards, UD cache, and frequency
baselines all present (`make doctor` all-green after symlinking `localdata/`
in addition to the documented `.venv`/`finnestdb.db`/`parser/target`). Server
built via `make server` and run on `127.0.0.1:8083` against the same DB.

Reviewer: Claude (Fable 5), single working session.

## Journey-by-journey results

| Step | FI evidence | ET evidence | Verdict | Classification | Notes |
|---|---|---|---|---|---|
| 1. Anonymous parse (`POST /api/parse`, `parser=custom`, unauthenticated) | 7-word sentence, `total_tokens=7`, `resolved_tokens=7`, `unresolved_tokens=0`, all 6 non-punct words carry `gloss` + `feats` | Same sentence structure, `total_tokens=7`, `resolved_tokens=7`, `unresolved_tokens=0`, all words carry `gloss` + `feats` | Parity | — | 100% resolution, 100% gloss attachment both languages. `source_counts` differ (FI used `dict+fst_feats` for 2 tokens, ET used `dict` only) — expected, ET FST/HFSTOL wiring is dict+Ekilex+FST per `make doctor`, FI is dict+FST; not user-visible. |
| 2. Signed-in Inspect parse (same texts, authenticated scratch user) | `total_tokens=7`, `resolved=7`, `unresolved=0` | `total_tokens=7`, `resolved=7`, `unresolved=0` | Parity | — | `learning_state` is `null` for all words in both languages (scratch user has no known/ignored state yet) — expected, symmetric. |
| 3. Deck save → detail → review-next | `POST /api/decks` → `deck_id=9`; detail: `total_tokens=11`, 11 words returned; `GET /api/review/next?deck_id=9` → HTTP 200, card with front/back populated | `POST /api/decks` → `deck_id=10`; detail: `total_tokens=13`, 13 words returned; `GET /api/review/next?deck_id=10` → HTTP 200, card with front/back populated | Parity | — | Both decks fully round-trip: create → detail → first review card, same response shape. |
| 4. Known-word import | `POST /api/known-words` with 3 lemmas (`kissa`, `koira`, `talo`) → 3 imported, 0 unresolved; `GET` confirms 3 rows | Same with 3 ET lemmas (`kass`, `koer`, `maja`) → 3 imported, 0 unresolved; `GET` confirms 3 rows | Parity | — | `koer` resolved to the ADJ ("naughty") sense rather than the more common NOUN ("dog") sense — a within-language ambiguity-ranking quirk, not an FI/ET asymmetry; noted for a possible separate ranking ticket, out of scope here. |
| 5. Parser feedback submission | `POST /api/parse/feedback` → `feedback_id=10`, `status="submitted"` | `POST /api/parse/feedback` → `feedback_id=11`, `status="submitted"` | Parity | — | `status` is DB-default `'submitted'` and not client-settable at creation for either language — verified in `internal/store/db.go` schema and confirmed via both live responses. Left in `submitted` status per audit instructions. |
| 6. Admin feedback queue | `GET /api/admin/parse-feedback?status=submitted` (as a resetpassword-minted admin) returns the FI item (`id=10`, `lang=FI`) | Same response includes the ET item (`id=11`, `lang=ET`) | Parity (with a doc-drift caveat) | Post-alpha (doc fix) | Both languages' items appear in the same unfiltered queue. `docs/PARSER_FEEDBACK_LOOP.md` line 251-253 claims the endpoint "filters by status and language," but `handleAdminParseFeedback` / `ListParseFeedback(status string)` only accept a `status` param — there is no server-side `lang` filter. Not a learner-facing or admin-blocking gap (both languages are visible; an admin can filter client-side), but the doc should be corrected to avoid an admin assuming a `?lang=` param works. |
| 7. Data readiness (read-only SQL) | forms: 26,826,071 rows (source=`kaikki`, priority=10). lemmas: 259,145 rows. `lemmas.gloss` coverage: 259,137/259,145 = 99.997%. `translations` table: **0 rows**. `definitions` table: **0 rows**. `forms.feats` coverage: 26,631,634/26,826,071 = **99.28%**. `gold_surfaces`: 255 rows. | forms: 6,256,916 rows (`ekilex` priority=20: 6,034,479; `kaikki` priority=10: 222,437). lemmas: 356,393 rows. `lemmas.gloss` coverage: 167,457/356,393 = 47.0%. `translations` table: 186,494 rows (`ekilex`, target_lang=EN). `definitions` table: 319,609 rows (`ekilex`). `forms.feats` coverage: 6,021,050/6,256,916 = **96.23%**. `gold_surfaces`: 112 rows. | Asymmetry (data substrate), Parity (learner-facing) | Language-specific | The raw `translations`/`definitions` table asymmetry looks alarming in isolation, but the live API's `BatchLookupGlosses` (the only gloss path wired into `/api/parse` and deck responses) explicitly falls back to `lemmas.gloss` when no `translations` row exists — confirmed in code (`internal/store/dict.go:1500-1518`) and empirically in journeys 1-3 above (both languages returned a gloss for every resolved word). FI's single-sense-only gloss cache vs. ET's richer multi-sense `translations`/`definitions` substrate reflects the documented source difference (Decision 12: FI glosses come from kaikki.org/Wikisanakirja only; ET additionally has the licensed Ekilex bulk import) — not a bug. The `definitions` table and `BatchLookupSenses` (`internal/store/dict.go:1598`) are **not called from any handler today** for either language, so this is dormant substrate, not a live learner-facing gap. FEATS coverage (99.28% FI / 96.23% ET) matches the 2026-07-02 verification report almost exactly, confirming no regression since that run. |
| 8. Eval/observability: baselines per language | Newest FI baseline: `2026-05-12b-T1606Z` (7 files: fi-core, fi-grammar, fi-manual-v1/v2, fi-analyzer-traps, + 4 UD sets: ftb-dev/test, tdt-dev/test, pud-test, ood-test) | Newest ET baseline: `2026-05-12b-T1606Z` (same date/letter; 3 files: et-grammar, et-manual, et-analyzer-traps) | Parity (freeze cadence) / Asymmetry (UD gold breadth) | Language-specific | Both languages froze their **most recent baseline on the same date and letter** (`2026-05-12b`) — no staleness gap. FI gold cases: fi-core 6, fi-grammar 80, fi-manual-v1 22, fi-manual-v2 4, fi-analyzer-traps 23 = 135 curated cases, plus committed UD sets (ud-fi-ftb-test 1,867 + ud-fi-ood-test 2,106 + ud-fi-pud-test 1,000 + ud-fi-tdt-test 1,554 = 6,527 tokens; TODO.md's "~9.8k FI committed" figure includes dev splits too). ET gold cases: et-grammar 50, et-manual 4, et-analyzer-traps 11 = 65 curated cases, **zero UD-Estonian test/dev cases committed to git**. This gap is explicitly documented and licensed, not an oversight: UD-Estonian-EDT/EWT ship CC BY-NC-SA (non-commercial), so they're gitignored under `localdata/parser-eval/et/gold/` (per `docs/data_enhancement.md` and `docs/PARSER_EVAL_METHODOLOGY.md` line 305) — confirmed present locally (`localdata/parser-eval/et/gold/ud-et-{edt,ewt}-{dev,test}-v1.json`, matching TODO.md's "~37.9k ET local-only" figure) and auto-discovered by `make compare-parsers-et` without extra flags. |
| 9. Tests: Playwright + Go | Playwright: `parse-results.spec.ts` (39 test/describe blocks, 60 FI string refs / 36 ET), `vocab-anki.spec.ts` (66 blocks, 3 FI / 59 ET — ET-dominant), `deck-comprehension.spec.ts` (3 blocks, FI-only fixture), `official-decks.spec.ts` (7 blocks, FI-only fixture). Go: 15 `_test.go` files reference `"FI"`. | Playwright: same files; `vocab-anki.spec.ts` is actually ET-heavy. Go: 12 `_test.go` files reference `"ET"`. | Mostly parity, one asymmetry | Non-blocking (test-coverage note) | Aggregate Playwright coverage is close (FI and ET both exercised heavily in the two largest spec files, each skewed toward a different language but both present). Two small specs — `deck-comprehension.spec.ts` (95 lines) and `official-decks.spec.ts` (218 lines) — mock only FI-language decks; the underlying UI logic under test (comprehension %, official-deck subscribe flow) is language-agnostic in the mock, so this is a coverage gap rather than a functional gap. Flagged below as a non-blocking rough edge, not alpha-blocking, since the same code path is proven to work for ET via the live API journeys above and via `parse-results.spec.ts`/`vocab-anki.spec.ts`. |
| 10. Embedded catalog | Absent (`TODO.md` line 960, unchecked `[ ]`) | Absent (same TODO item, both languages) | Parity (symmetric gap) | Post-alpha (in progress separately per task brief) | Confirmed via `grep` — no embedded-catalog code path exists yet for either language; TODO.md tracks it as one unchecked item covering both, not two language-specific items. Consistent with the task brief noting it's "being built tonight separately." |
| 11. Frequency/starter (official) decks | Zero `decks` rows with `is_public=1` for FI in the live DB | Zero `decks` rows with `is_public=1` for ET in the live DB (all 2 existing decks are private, `user_id=4`, both language ET) | Symmetric gap — not yet run | Alpha-blocking (as a launch-readiness verification gap) | `cmd/seedcolddeck` (the "Top 1000 official deck" seeder) is a **documented one-time-per-deployment operator step** (`docs/DEPLOYMENT.md` line 217-218), not something expected to already exist in an arbitrary local dev DB — so its absence here is not itself a code defect. However, `TODO.md` line 990 marks "Cold-start Top 1000 CTA" **`[x]` shipped**, stating it was "verified end-to-end against the full local DB" — and the actual DB backing that claim today has **zero** official decks for either language. The frequency source files it depends on (`localdata/frequency/{fi,et}/opensubtitles-2018-*-50k.txt`) are present and symmetric for both languages, and the seeder code takes `-lang FI`/`-lang ET` as parallel paths with no asymmetric logic found in `cmd/seedcolddeck/main.go`. I did not execute the seeder against the shared production-like DB (it would create durable, hard-to-undo official-deck rows visible to all users of that DB, which exceeds the "keep audit writes minimal" instruction). **Action needed before public alpha**: run `cmd/seedcolddeck` for both FI and ET against the real launch DB as part of the `docs/DEPLOYMENT.md` runbook, and correct or re-verify the TODO.md "verified end-to-end" claim — the verification described there does not describe the current DB state. |

## Cross-cutting evidence not tied to a single journey

- `make doctor`: all rows green for both languages once `localdata/` was
  symlinked in addition to the documented `.venv`/`finnestdb.db`/`parser/target`
  (worth folding into the AGENTS.md worktree setup instructions as a follow-up,
  since the task brief's setup list omitted it and `make doctor` initially
  reported FST tables and analyzers as absent/degraded for both languages
  symmetrically until that symlink was added).
- Server startup: `FI dictionary loaded: 26826071 forms` then
  `ET dictionary loaded: 6256916 forms` — both languages load successfully
  with no errors or warnings in server logs.
- Dashboard payload (`GET /api/me` for the scratch admin after both decks +
  known-words were created) shows `languages.stats` with parallel structure
  for FI and ET (`{"decks":1,"known_words":3}` each) and both decks appear in
  `dashboard.decks[]` with per-language `comprehension_pct` computed
  identically (18.2% both, expected since both decks have the same
  known/unique word ratio by construction).

## Untested

- **Load/throughput parity**: not part of this journey-first pass; the
  existing `docs/GO_LIVE_CHECKLIST.md` load-test gate is separate and not
  re-run here.
- **Admin quarantine workflow** (accept/reject a correction end-to-end,
  including gold-guard conflict handling): only the queue-listing half of
  admin triage (journey 6) was exercised; accept/reject was not exercised for
  either language to avoid mutating `forms`/`lemmas` rows with
  `source='custom_overrides'` in the shared DB, which would be a durable,
  higher-blast-radius write than the audit's "keep writes minimal" instruction
  allows. Both languages share the same `PATCH /api/admin/parse-feedback`
  handler with no language branching in the code path (spot-checked), so this
  is judged low-risk to leave untested, but it is not empirically verified
  end-to-end for either language in this pass.
- **`cmd/seedcolddeck` actual execution**: see journey 11 — verified by code
  reading only, not run against the shared DB.

## Summary

### Alpha-blocking

1. **Official "Top 1000" starter decks do not exist in the DB for either
   language**, despite `TODO.md` marking the feature `[x]` shipped and
   "verified end-to-end against the full local DB." This is a symmetric gap
   (affects FI and ET equally) but it fails the "equal-status alpha journey"
   requirement in `docs/GO_LIVE_CHECKLIST.md` (cold-start decks are part of
   the signed-in learner loop) for **both** languages simultaneously, and the
   TODO.md claim needs correcting either way. See ledger row below.

### Language-specific (not blocking)

1. `translations`/`definitions` table population differs sharply (FI: 0 rows
   in both tables, relies on `lemmas.gloss` cache; ET: 186,494 translation
   rows + 319,609 definition rows from Ekilex) — but the live gloss-serving
   path (`BatchLookupGlosses`) falls back correctly for FI, so every learner
   sees a gloss in both languages. Documented and intentional per Decision 12
   (Wikisanakirja/kaikki.org is FI's only licensed definitions source today).
2. UD treebank gold-set breadth: FI has ~6.5k committed UD test-token cases in
   git; ET has zero committed (CC BY-NC-SA license prevents public
   redistribution) but ~37.9k tokens available local-only per maintainer,
   auto-discovered by `make compare-parsers-et`. Documented in
   `docs/data_enhancement.md` and `docs/PARSER_EVAL_METHODOLOGY.md`.
3. `lemmas.gloss` cache coverage: FI 99.997% vs. ET 47.0% — again explained by
   FI relying solely on the cache (so it must be fully populated) while ET's
   primary gloss source is the richer `translations` table, with the cache
   only partially backfilled. Not learner-visible because `BatchLookupGlosses`
   prefers `translations` when present.

### Post-alpha

1. `docs/PARSER_FEEDBACK_LOOP.md` documents a `lang` filter on
   `GET /api/admin/parse-feedback` that does not exist in code
   (`ListParseFeedback` only accepts `status`). Both languages are visible in
   the unfiltered response, so this is a documentation fix, not a product gap.
2. Two Playwright specs (`deck-comprehension.spec.ts`,
   `official-decks.spec.ts`) mock FI-only fixtures for otherwise
   language-agnostic UI logic. Low risk given the same logic is covered for
   ET via `vocab-anki.spec.ts` and `parse-results.spec.ts`, and via the live
   API journeys in this audit, but adding an ET fixture variant would close
   the gap cheaply.
3. `definitions` table and `BatchLookupSenses` are populated for ET (via
   Ekilex) but wired into no handler for either language — dormant capacity,
   not a current asymmetry, worth remembering when multi-sense display ships.

### Overall equal-status verdict

**Conditional pass.** Every learner-facing journey actually exercised
end-to-end (anonymous parse, signed-in parse, deck save/detail/review,
known-word import, parser-feedback submission, admin feedback visibility)
showed full FI/ET parity with concrete evidence — resolution rates, gloss
attachment, FEATS coverage, and response shapes were symmetric or the
asymmetry was traced to a documented, licensed, non-learner-visible cause.
The one alpha-blocking finding (official starter decks absent for both
languages, contradicting a TODO.md "shipped and verified" claim) is a
same-day fixable runbook-execution gap, not a design or code asymmetry
between FI and ET — but it must be closed (by running `cmd/seedcolddeck` for
both languages against the launch DB, or by correcting/re-scoping the TODO.md
claim) before this gate can be marked passed.

## Cleanup appendix — durable rows this audit created

All rows below live in the real shared `finnestdb.db` (symlinked from the
main repo root), created via the live API against `127.0.0.1:8083`. None were
deleted or updated by this audit; only new rows were added.

| Table | Row(s) | Notes |
|---|---|---|
| `users` | id 28, `parity-audit-2026-07-04@example.test` | Created via `POST /api/auth/register`; promoted to `is_admin=1` via `go run ./cmd/resetpassword -email parity-audit-2026-07-04@example.test -admin -keep-sessions` (password rotated to a generated value, printed once to this session's terminal and not otherwise recorded). Recommend deleting this account (or at least revoking admin) after this report is reviewed. |
| `decks` | id 9, `parity-audit-fi-deck` (FI, private, owner 28) | Created via `POST /api/decks`, 11 tokens. |
| `decks` | id 10, `parity-audit-et-deck` (ET, private, owner 28) | Created via `POST /api/decks`, 13 tokens. |
| `parse_sessions` | ids 17, 18 | Created implicitly by the two deck saves above. |
| `user_known_lemmas` | 3 FI rows (`kissa`/NOUN, `koira`/NOUN, `talo`/NOUN), 3 ET rows (`kass`/NOUN, `koer`/ADJ, `maja`/NOUN), all `user_id=28` | Via `POST /api/known-words`. |
| `parse_feedback` | id 10 (FI, surface `haukkuu`, status `submitted`, note `"parity-audit test feedback FI"`) | Left in `submitted` per audit instructions; not accepted/rejected. |
| `parse_feedback` | id 11 (ET, surface `jookseb`, status `submitted`, note `"parity-audit test feedback ET"`) | Same. |
| `cards` / `card_state` / `review_log` | Cards seeded for decks 9 and 10 via the review-next call | One review card was fetched (not answered) per deck; no `review_log` rows were written since `POST /api/review/answer` was not called. |

Recommended cleanup: delete user id 28 (cascades decks/known-words/feedback
per the existing account-deletion path), or leave in place if the team wants
a standing QA account — flagging for a maintainer decision rather than
deleting unilaterally, since deletion is a destructive operation outside this
audit's read-mostly mandate.
