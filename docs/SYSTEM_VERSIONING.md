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
| Parser behavior | `parser-v1` | `basic`, `custom`, and evaluation-only `omorfi` are wired through `parsecore`. |
| Parser evaluation baseline | `parser-baseline-2026-04-28-1` | Existing frozen reports and gold data live under `docs/baselines/` and `testdata/parser-eval/`. |
| Deck review system | `review-v0` | Backend and UI scaffolding exist, but review scheduling is not yet a locked production contract. |
| API contract | `api-v0` | Alpha API surface; parse is the most mature contract. |
| Data schema | implicit | Schema exists in code today; explicit migrations should become the source of truth before production data matters. |

## Versioning Rules

Use separate versions when a change alters behavior inside one subsystem without
changing the others. For example, a Finnish possessive suffix improvement should
bump `parser-vN`, while a new review interval algorithm should bump `review-vN`.

Use the evaluation baseline version to freeze evidence. Parser code can move from
`parser-v1` to `parser-v2`, and the comparison should state which baseline proved
the change, such as `parser-baseline-2026-05-01-1`.

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
