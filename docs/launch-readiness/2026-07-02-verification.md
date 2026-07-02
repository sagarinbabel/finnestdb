# Release Verification — 2026-07-02

Scope: full release-verification suite from `docs/GO_LIVE_CHECKLIST.md`, run at
the tip of the launch PR stack (#240–#247, branch `ops/security-review-fin27`)
against the production-size local DB (26.8M FI / 6.3M ET forms).

Companion: [`2026-07-02-security-review.md`](2026-07-02-security-review.md)
(FIN-27). Previous run: [`2026-06-03-public-launch-verification.md`](2026-06-03-public-launch-verification.md).

## Results

| Check | Result | Notes |
|---|---:|---|
| `go vet ./...` | PASS | Clean |
| `go test ./...` | PASS | 28 packages ok, including all new suites (comprehension, review log, cold-start seeder, correction-loop Phases 2–4, health probe, reset CLI store methods) |
| `go test -race ./internal/api ./internal/auth ./internal/store` | PASS | Clean (macOS `ld` LC_DYSYMTAB warning is linker noise) |
| `npm test` (Playwright) | PASS | **113 passed** (108 previous + 5 new: deck comprehension ×3, dashboard progress ×2) |
| `make live-api-smoke` | PASS | 15/15 against a live stack-tip server on `:8097` |
| `make db-invariants` | PASS | `integrity_check ok`; zero orphans/overlaps; source breakdown FI kaikki 26.8M, ET ekilex 6.03M + kaikki 222k |
| `make compare-parsers` (FI) | PASS with one justified delta | See regression analysis below; reports `20260702T175817Z` |
| `make compare-parsers-et` (ET) | PASS | All committed-baseline datasets identical to `2026-05-12b` |
| forms.feats coverage | PASS | FI 99.3%, ET 96.2% of rows carry FEATS; live custom-mode parses emit full UD FEATS (FI + ET verified) |
| Full-book latency | PASS | `POST /api/decks` 1.6–2.0 s for real novels (70k tokens); 30 s WriteTimeout has ≥7× headroom (details in PR #243) |

## Parser regression analysis vs frozen baseline `2026-05-12b-T1606Z`

Headline `custom` lemma/POS/joint accuracy compared per dataset:

- **Identical** on all 7 committed gold sets (fi-core, fi-grammar, fi-manual
  v1/v2, fi-analyzer-traps, et-grammar, et-manual, et-analyzer-traps).
- **Slightly up** on ud-fi-ftb (+0.05pp POS), ud-fi-tdt (+0.04pp lemma);
  ud-fi-pud unchanged.
- **One dip**: ud-fi-ood lemma −0.22pp (0.6343 → 0.6321), joint −0.22pp.

Token-level diff of the ood run: 44 of 16,151 analyses changed. The dominant
pattern is deverbal action nouns in clinical text resolving to their source
verb as lemma (*hengitys*→*hengittää* ×10, *hapetus*→*hapettaa* ×9,
*eritys*→*erittää* ×3) plus scattered ranking flips.

**Root cause: dictionary data drift, not launch-stack code.** The eval path
(`BatchLookupForms` → picker) was not touched by any PR in the stack (the
#240 dict change only affects rows with `source='custom_overrides'`, of which
the live DB has zero). What did change since the baseline froze is the
intentional `forms.feats` backfill re-import — the candidate picker is
FEATS-aware (PR #139), so newly FEATS-bearing candidates shift tie-breaks on
ambiguous inflected forms. The effect is a wash across FI UD sets (+/−0.05pp)
except the clinical OOD set, whose vocabulary is dense in exactly the
deverbal-noun ambiguity class.

**Disposition:** justified, not blocking. Two follow-ups recorded:

1. Re-freeze a post-reimport baseline (next letter per the baseline naming
   convention) so future runs diff against the current intended data state —
   maintainer call, since FINAL baselines require maintainer-local FST tables.
2. The deverbal-noun ranking flips are the exact error class the now-live
   correction loop (#247) handles: one accepted correction per surface fixes
   it authoritatively, and 3 independent reports auto-queue a gold candidate.

## Verdict

All gates pass. With the stack (#240–#247) merged, the deployment runbook
executed on a production host (TLS/proxy, backups, purge cron, uptime
alerting, admin pre-registration, gold-surfaces import, starter-deck seeding),
and `make live-api-smoke` re-run against that host, the app is ready for
public alpha.
