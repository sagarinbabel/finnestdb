# System Versioning

_Current as of 2026-05-01 - see [CHANGELOG.md](CHANGELOG.md) for revisions._

FinEstDB should version the behavior of major subsystems separately. The parser,
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
| Parser behavior | `parser-v1` (= dated tag `2026.05.07k`, see [`PARSER_EVOLUTION.md`](PARSER_EVOLUTION.md)) | `basic`, `custom`, evaluation-only `omorfi`/`estnltk` wired through `parsecore`. The constant `parsecore.ParserVersion` carries the dated tag and is stamped into every eval JSON report's `parser_version` field. |
| Parser evaluation baseline | `parser-baseline-2026-05-07-k-T1118Z` | The current k freeze under `docs/baselines/2026-05-07k-T1118Z-*`: 8 FI datasets / 61,927 tokens + 2 ET datasets / 190 tokens; first measurement with FI + ET FEATS loaded into DB end-to-end (FI 99.3% FEATS coverage via `cmd/importdict -backfill-feats`; ET 96.0% via Ekilex bulk drop `morph_code → FEATS`). See [`PARSER_EVOLUTION.md`](PARSER_EVOLUTION.md) §2026-05-07k-T1118Z. Earlier same-day runs (`-T0944Z`, `-feats-rich`) are retained for trend continuity. The j freeze (2026-05-07j; pre-FEATS-eval) is at `docs/baselines/2026-05-07-post-fst-*`. The wall-clock `THHMMZ` suffix is part of the baseline ID — multiple baselines on the same parser-behavior version are distinguished by run-start UTC time. |
| Deck review system | `review-v0` | Backend and UI scaffolding exist, but review scheduling is not yet a locked production contract. |
| API contract | `api-v0` | Alpha API surface; parse is the most mature contract. |
| Data schema | implicit | Schema exists in code today; explicit migrations should become the source of truth before production data matters. |

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
