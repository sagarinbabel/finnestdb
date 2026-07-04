# System Versioning

_Current as of 2026-05-15 - see [CHANGELOG.md](CHANGELOG.md) for revisions._

FinnEst should version the behavior of major subsystems separately. The parser,
deck review loop, and evaluation baselines change for different reasons and
should not be forced into one product-wide version number.

## Versioned Systems

| System | Version ID | Owns | Changes when |
|--------|------------|------|--------------|
| Parser behavior | `parser-vN` | tokenization, parser modes, dictionary resolution, morphology enrichment, parse output contract | parser output, coverage, grammar labels, or parser-mode semantics change |
| Parser evaluation baseline | `parser-baseline-YYYY-MM-DD-N` | gold datasets, frozen reports, comparison thresholds | datasets, expected outputs, benchmark parser set, or regression thresholds change |
| Deck review system | `review-vN` | deck creation semantics, card state, review queue, scheduling policy | card lifecycle, scheduling math, queue selection, or review outcome semantics change |
| API contract | `api-vN` | request/response shapes used by browser and tools | externally consumed JSON fields are added, removed, renamed, or reinterpreted |
| Data schema | migration IDs | SQLite tables, indexes, constraints, migrations | persistent storage shape changes |

## Current Starting Versions

These are documentation starting points, not claims that the systems are stable.

| System | Current version | Notes |
|--------|-----------------|-------|
| Parser behavior | `parser-v2` (= dated tag `2026.05.15a`, see [`PARSER_EVOLUTION.md`](PARSER_EVOLUTION.md)) | Production `basic`/`custom` parser modes live in `parsecore`; evaluation-only `omorfi`/`estnltk` live in `internal/evalparsers`. The constant `parsecore.ParserVersion` carries the dated tag and is stamped into every eval JSON report's `parser_version` field. |
| Parser evaluation baseline | `parser-baseline-2026-05-12-b-T1606Z` (latest; see "Parser evaluation baseline history" below) | Re-freeze on `parser-v2` / `2026.05.12b`, closes the `peatus`/`joon`/`naeris` regressions from `2026-05-12-a-T1526Z` via a targeted ET verb-inflection bias and a post-#189 Ekilex reimport. Current parser behavior has moved to `parser-v2` / `2026.05.15a` after the §2026-05-12c source-backed ET learner cleanup, the §2026-05-12d verb-inflection bias merge, the §2026-05-12e review follow-ups (PR #205), and the §2026-05-15a FI manual-card trap promotions (`sanoin`, `Maria`, `Norjan`). Run a new freeze before headline baseline numbers reflect the combined state. Full history is preserved in the dedicated section below this table and as a row-per-event trend in [`PARSER_EVOLUTION.md`](PARSER_EVOLUTION.md). The wall-clock `THHMMZ` suffix is part of the baseline ID — multiple baselines on the same parser-behavior version are distinguished by run-start UTC time. |
| Deck review system | `review-v0` | Backend and UI scaffolding exist, but review scheduling is not yet a locked production contract. |
| API contract | `api-v0` | Alpha API surface; parse is the most mature contract. |
| Data schema | implicit | Schema exists in code today; explicit migrations should become the source of truth before production data matters. |

## Parser evaluation baseline history

**Append-only list — when a new baseline freezes, add a row at the top, never edit or remove prior rows.** Cross-references in PR descriptions, commit messages, and external citations rely on these IDs staying valid. The trend table in [`PARSER_EVOLUTION.md`](PARSER_EVOLUTION.md) is the headline-numbers companion; this list is the ID-stability ledger.

| Baseline ID | Date / commit | Notes |
|---|---|---|
| `parser-baseline-2026-05-12-b-T1606Z` (**latest**) | 2026-05-12T16:06Z, [`c13d74e`](https://github.com/sagarinbabel/finnestdb/commit/c13d74e) | Re-freeze after the `peatus`/`joon`/`naeris` regression fix on §2026-05-12a-T1526Z. New `etVerbInflectionBias` in `pickBestResolutionCandidate` picks the inflected ET verb reading over a citation-form noun homograph when both candidates compete at equal-or-higher source priority. Plus the post-#189 Ekilex reimport that lands the importer-side `Inf`/`Sup`/`Ger` corrections (e.g. `õppida` FEATS `Sup` → `Inf`). FI numbers byte-stable vs §2026-05-12a-T1526Z (ET-only gate). et-grammar lemma 91.4 → 92.4 / full 80.0 → 82.9; et-manual lemma 88.9 → 100. See [`PARSER_EVOLUTION.md`](PARSER_EVOLUTION.md) §2026-05-12b-T1606Z and [`baselines/2026-05-12b-T1606Z-{fi,et}.md`](baselines/). |
| `parser-baseline-2026-05-12-a-T1526Z` | 2026-05-12T15:26Z, [`a5a4808`](https://github.com/sagarinbabel/finnestdb/commit/a5a4808) | First measurement after the 2026-05-12 PR cascade (#183, #185, #187, #188, #189, #191, #193, #195). FI FST table regenerated with PR #188's non-finite paradigm coverage (482,835 surfaces; fi_min.json 79 MB → 255 MB). ET FST table regenerated with PR #189/#191's giellaltmap fixes (Voice on +Pers/+Imprs, Sup/Ger/Inf mappings). DB not reimported, so PR #189's dict-side Ekilex importer fix is not yet reflected. 9 FI datasets / 61,825 scored tokens + 3 ET datasets / 125 scored tokens (now incl. analyzer-traps). See [`PARSER_EVOLUTION.md`](PARSER_EVOLUTION.md) §2026-05-12a-T1526Z and [`baselines/2026-05-12a-T1526Z-{fi,et}.md`](baselines/). |
| `parser-baseline-2026-05-07-k-T1118Z` | 2026-05-07T11:18Z, [`ffd7584`](https://github.com/sagarinbabel/finnestdb/commit/ffd7584) | First measurement with FI + ET FEATS loaded into DB end-to-end (FI 99.3% FEATS coverage via `cmd/importdict -backfill-feats`; ET 96.0% via Ekilex bulk drop `morph_code → FEATS`). 8 FI datasets / 61,927 tokens + 2 ET datasets / 190 tokens. See [`PARSER_EVOLUTION.md`](PARSER_EVOLUTION.md) §2026-05-07k-T1118Z and [`baselines/2026-05-07k-T1118Z-{fi,et}.md`](baselines/). |
| `parser-baseline-2026-05-07-k-feats-rich` | 2026-05-07, PR [#135](https://github.com/sagarinbabel/finnestdb/pull/135)→[#139](https://github.com/sagarinbabel/finnestdb/pull/139) | First baseline with non-empty per-FEATS-attribute eval table; ships the data-thread wiring (kaikki tags → FEATS, case-suffix → FEATS, Vabamorf form codes → FEATS, gold FEATS seeding via `cmd/enrichgoldfeats`). See [`PARSER_EVOLUTION.md`](PARSER_EVOLUTION.md) §2026-05-07k-feats-rich and [`baselines/2026-05-07-feats-rich-{fi,et}.md`](baselines/). |
| `parser-baseline-2026-05-07-k-T0944Z` | 2026-05-07T09:44Z, [`317ab1b`](https://github.com/sagarinbabel/finnestdb/commit/317ab1b) | Earlier same-day k re-measure; FST disabled (no production tables); same DB as j. Superseded by `-T1118Z` for headline numbers; retained for trend continuity. See [`PARSER_EVOLUTION.md`](PARSER_EVOLUTION.md) §2026-05-07k-T0944Z. |
| `parser-baseline-2026-05-07-j` | 2026-05-07, PR [#133](https://github.com/sagarinbabel/finnestdb/pull/133) | Pre-FEATS-eval baseline; case-suffix grammar-label stopgap with smoke FST tables; first UD-at-scale baseline (8 FI datasets, 61,927 tokens). Files at [`docs/baselines/2026-05-07-post-fst-*`](baselines/). |
| `parser-baseline-2026-05-06-final` | 2026-05-06, PR [#112](https://github.com/sagarinbabel/finnestdb/pull/112) | FST stack PR4 final freeze. Files at [`docs/baselines/2026-05-06-final-*`](baselines/). See `PARSER_EVOLUTION.md` §2026-05-06i. |
| `parser-baseline-2026-04-28-1` | 2026-04-28, PR [#37](https://github.com/sagarinbabel/finnestdb/pull/37) (FI) / [#42](https://github.com/sagarinbabel/finnestdb/pull/42) (ET) | First frozen baseline after the eval harness landed in PR [#29](https://github.com/sagarinbabel/finnestdb/pull/29). Reference floor; pre-FST stack. Files at [`docs/baselines/2026-04-28-*`](baselines/). |

For the full per-stage narrative (every PR that affected parser behavior, with headline-number deltas), see [`PARSER_EVOLUTION.md`](PARSER_EVOLUTION.md).

## Versioning Rules

Use separate versions when a change alters behavior inside one subsystem without
changing the others. For example, a Finnish possessive suffix improvement should
bump `parser-vN`, while a new review interval algorithm should bump `review-vN`.

Use the evaluation baseline version to freeze evidence. Parser code can move from
`parser-v1` to `parser-v2`, and the comparison should state which baseline proved
the change, such as `parser-baseline-2026-05-07-j`.

### Parser-version naming: SemVer-N vs. dated tag

The parser uses two parallel naming schemes, kept consistent with each other:

- **`parser-vN`** (this doc) is the SemVer-style scope marker. Bump on
  parser-affecting PRs that change observable behavior. Coarse — one per
  major shift.
- **`YYYY.MM.DDx`** (in [`PARSER_EVOLUTION.md`](PARSER_EVOLUTION.md), e.g.
  `2026.05.07j`) is the per-baseline dated iteration tag. Fine-grained — one
  per measurement event. The constant `parsecore.ParserVersion` carries this
  tag and is stamped into every eval JSON report.

The dated tag is what lives in code and in saved reports; the SemVer-N is what
goes in human conversation and release notes. Map between them in this table
when each `parser-vN` ships:

| `parser-vN` | Latest dated iteration when bumped | Notable behaviors introduced |
|---|---|---|
| `parser-v2` | `2026.05.15b` | Learner-quality parser fixes from PR #183: lexical-adverb overlays, MA-infinitive FEATS normalization/ranking, bad-lemma blocklists, structural-gloss ingest filtering, and `BatchLookupSenses`; deck/parse low-value alternative suppression from PR #185; source-priority-first dict/FST candidate ranking from PR #187; source-backed ET learner cleanup for special-capitalized dictionary entries, invariant closed-class FEATS, ET verb dictionary forms, and high-frequency ET gloss overrides; ET verb-inflection bias closing the `peatus`/`joon`/`naeris` regression class from §2026-05-12a-T1526Z; PR #205 review follow-ups (basic-mode special-cap FEATS sanitization parity, attribute-based ET verb dictionary-form FEATS check, explicit `TA` lex-overlay bypass test); FI manual-card trap promotions for `sanoin`, `Maria`, and `Norjan` (`2026.05.15a`); FI candidate-inclusion gap closed by merging FST-known homograph readings into the ambiguity candidate set (`BatchLookupAllFormsWithOptions`/`MergeFSTReadings`), gated off the deck-expansion path — FI ambiguity inclusion 72.9% → 95.8%, selection unchanged, headline baselines byte-stable (`2026.05.15b`). |
| `parser-v1` | `2026.05.07k` | Baseline scope: dict (basic/custom), case-suffix grammar-label stopgap, FST as parallel scorer in dict step 1 (post-#127) against smoke fixtures, FST candidate-merge FEATS enrichment (post-#129), per-attribute FEATS eval (post-#130), omorfi/estnltk eval columns. Iterations within v1: `j` (2026-05-07, pre-FEATS-eval), `k` (2026-05-07, post-FEATS-eval). |

Do not bump a subsystem version for implementation-only refactors that preserve
observable behavior. Record those in git history instead.

When a change crosses boundaries, bump every affected system. For example, adding
parse confidence to the browser JSON response may require both `parser-vN` and
`api-vN`.

## Recommended Change Record

Every subsystem behavior change should record:

- old version
- new version
- changed behavior
- datasets or tests used as evidence
- migration or rollback notes, if persistent data is affected

Suggested format:

```markdown
## parser-v2 - 2026-05-01

- Changed: Finnish compound resolution now prefers dictionary-backed splits.
- Evidence: `make eval`, `make compare-parsers`.
- Baseline: `parser-baseline-2026-05-01-1`.
- Compatibility: parse response shape unchanged; no API bump.
```
