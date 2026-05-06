# Finnish Parser Baseline — 2026-05-06

Refresh of [`2026-04-28-fi-summary.md`](2026-04-28-fi-summary.md) on
current `main` (commit `46d8b77`). Anchors PR 0 of the Finnish lexical
plan — the reference point Phases 2–5 measure regressions/improvements
against. See [`docs/LEXICAL_PLAN.md`](../LEXICAL_PLAN.md).

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

## Measured throughput

The eval timer was rewritten in PR #103 to record nanoseconds end-to-end
(was: integer milliseconds, which truncated nearly every per-case sample
to 0). Re-running the same `cmd/parsertest -warmup 2 -repeat 5` sweep
on `main` after the fix gives us per-case latency and throughput we can
actually quote, on `claude/sweet-swanson-183437` against the same
dictionary state described above (Apple Silicon, single-threaded, warm
cache):

| Dataset (cases) | Parser | avg / p50 / p95 (ms) | words/s | chars/s |
|---|---|---|---:|---:|
| fi-core-v1 (6)    | basic   | 0.097 / 0.094 / 0.126 | 39.3k | 311k |
| fi-core-v1        | custom  | 0.095 / 0.093 / 0.118 | 40.4k | 319k |
| fi-manual-v1 (22) | basic   | 0.137 / 0.120 / 0.202 | 64.3k | 650k |
| fi-manual-v1      | custom  | 0.217 / 0.205 / 0.381 | 40.7k | 411k |
| fi-manual-v2 (4)  | basic   | 0.072 / 0.064 / 0.107 | 41.9k | 433k |
| fi-manual-v2      | custom  | 0.070 / 0.065 / 0.086 | 43.1k | 445k |
| fi-grammar-v1 (80)| basic   | 0.075 / 0.074 / 0.100 | 47.9k | 361k |
| fi-grammar-v1     | custom  | 0.073 / 0.072 / 0.098 | 49.2k | 371k |

Reading this:

- **Steady-state Finnish throughput is ~40–50k words/s on `custom`**,
  the parser we ship to users. Per-case latency clusters around
  70–220 µs, so a typical web-form paragraph parses in well under a
  millisecond.
- `fi-manual-v1` is the only dataset where `custom` is slower than
  `basic` (0.217 ms vs 0.137 ms): the gap is the compound and
  possessive fallback work doing more `BatchLookupForms` calls
  (lookup time is 0.127 ms vs 0.051 ms). On the other datasets dict
  coverage is already high enough that fallback rules don't fire,
  and the two parsers run essentially the same code path.
- The throughput claim in [`docs/DECISIONS.md`](../DECISIONS.md)
  ("fast — Rust + dictionary lookup") now has a measured floor we
  can defend regressions against. It is not a hand-wave anymore.

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
