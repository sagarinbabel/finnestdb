# Finnish Parser Baseline — 2026-05-06

Refresh of [`2026-04-28-fi-summary.md`](2026-04-28-fi-summary.md) on
current `main` (commit `46d8b77`). Anchors PR 0 of the Finnish lexical
plan — the reference point Phases 2–5 measure regressions/improvements
against. See [`docs/FINNISH_LEXICAL_PLAN.md`](../FINNISH_LEXICAL_PLAN.md).

## Dictionary state

| Language | Forms | Lemmas | Sources |
|---|---:|---:|---|
| FI | 26,826,071 | 259,145 | kaikki.org Finnish |
| ET | 6,178,514 | 354,231 | kaikki.org Estonian + Ekilex bulk drop (`cmd/importekilexdetails`) |

The Finnish numbers are essentially unchanged from April since no new
FI sources have landed yet — Phases 3 (Kotus) and 4 (Voikko) are the
ones that grow this. The form count looks high because the multi-lemma
schema ([#78](https://github.com/sagarinbabel/finnestdb/pull/78))
preserves multiple `(lemma, pos)` candidates per surface form.

## Numbers

Run: `cmd/parsertest -warmup 1 -repeat 3 -parsers basic,custom`.
Each row is `lemma / POS / grammar / full / coverage` percentages.

| Dataset (cases) | Parser | Lemma | POS | Grammar | Full | Coverage |
|---|---|---:|---:|---:|---:|---:|
| **fi-core-v1** (6) | basic | 85.0 | 90.0 | 0.0 | 35.0 | 91.3 |
| fi-core-v1 | custom | 85.0 | 90.0 | 0.0 | 35.0 | 95.7 |
| **fi-manual-v1** (22) | basic | 55.7 | 78.6 | 0.0 | 41.4 | 75.3 |
| fi-manual-v1 | custom | **81.4** | **85.7** | 0.0 | **62.9** | **91.2** |
| **fi-manual-v2** (4) | basic | 88.9 | 100.0 | 0.0 | 55.6 | 91.7 |
| fi-manual-v2 | custom | 88.9 | 100.0 | 0.0 | 55.6 | **100.0** |
| **fi-grammar-v1** (80) | basic | 96.8 | 98.1 | 0.0 | 51.3 | 99.7 |
| fi-grammar-v1 | custom | 96.8 | 98.1 | 0.0 | 51.3 | 99.7 |

## Deltas vs. 2026-04-28

| Dataset | Parser | Δ Lemma | Δ POS | Δ Grammar | Δ Full | Δ Coverage |
|---|---|---:|---:|---:|---:|---:|
| fi-core-v1 | basic | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 |
| fi-core-v1 | custom | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 |
| fi-manual-v1 | basic | +11.4 | +2.9 | 0.0 | +8.5 | +5.7 |
| fi-manual-v1 | custom | +8.5 | 0.0 | 0.0 | +5.8 | -2.6 |
| fi-grammar-v1 | basic | +0.6 | 0.0 | 0.0 | 0.0 | +0.7 |
| fi-grammar-v1 | custom | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 |

Mostly flat, with `fi-manual-v1` showing meaningful gains on `basic`
(+11.4 lemma) and `custom` (+8.5 lemma). The most likely cause is
[PR #65](https://github.com/sagarinbabel/finnestdb/pull/65) — Estonian
case-suffix improvements were broad enough that some Finnish
fallback rules picked up similar gains, plus tokenizer changes from
[PR #65](https://github.com/sagarinbabel/finnestdb/pull/65) treating
trailing `*` as punctuation. `custom` coverage on `fi-manual-v1` ticked
down 2.6pp — worth investigating during Phase 2 work.

## Standing observations (unchanged from April)

- **Grammar accuracy is 0% across the board.** No Finnish parser path
  emits a `grammar_label` because no source populates `feats` yet.
  Phase 4 (Voikko) is the lever that fixes this — Voikko-generated
  rows ship UD-style features per inflected form. Until then, the
  custom path's case-suffix fallback rules don't typically fire
  because dict coverage already returns the lemma without going
  through suffix-stripping.
- **`custom` continues to beat `basic` on real-world text** by a wide
  margin (+25.7pp lemma on `fi-manual-v1`). Compound + possessive
  enrichment is paying off.
- **Per-case timing rounds to 0ms.** Same tooling limitation as April
  — switch eval reporter to nanoseconds when timing data is needed.

## Files

- `2026-05-06-fi-core-v1.json`
- `2026-05-06-fi-manual-v1.json`
- `2026-05-06-fi-manual-v2.json`
- `2026-05-06-fi-grammar-v1.json`

## Reproduction

```bash
make parser
export DYLD_LIBRARY_PATH="$(pwd)/parser/target/release:$DYLD_LIBRARY_PATH"

# Dictionary state must match the Finnish row in the table above.
# A fresh build:
make import-dict-fi
make import-dict-et
make import-ekilex-et
make import-ekilex-details-et   # adds Ekilex bulk drop into ET (does not change FI)

for ds in fi-core-v1 fi-manual-v1 fi-manual-v2 fi-grammar-v1; do
  go run ./cmd/parsertest \
      -dataset "testdata/parser-eval/fi/gold/$ds.json" \
      -parsers basic,custom -warmup 1 -repeat 3 \
      -out "docs/baselines/2026-05-06-$ds.json"
done
```
