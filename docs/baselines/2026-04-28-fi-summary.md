# Finnish Parser Baseline — 2026-04-28

First frozen baseline after **B1** (gold-data expansion to 108 cases).
Run on `claude/finalize-jpdb-clone-bdGL0` with the kaikki.org Finnish
dictionary imported (12.2M forms, 145K lemmas).

## Numbers

| Dataset (cases) | Parser | Lemma | POS | Grammar | Full | Coverage |
|---|---|---|---|---|---|---|
| **fi-core-v1** (6) | basic  | 85.0% | 90.0% | 0.0% | 35.0% | 91.3% |
| fi-core-v1         | custom | 85.0% | 90.0% | 0.0% | 35.0% | 95.7% |
| **fi-manual-v1** (22) | basic  | 44.3% | 75.7% | 0.0% | 32.9% | 69.6% |
| fi-manual-v1          | custom | **72.9%** | **85.7%** | 0.0% | **57.1%** | **93.8%** |
| **fi-grammar-v1** (80) | basic  | 96.2% | 98.1% | 0.0% | 51.3% | 99.0% |
| fi-grammar-v1          | custom | **96.8%** | 98.1% | 0.0% | 51.3% | 99.7% |

## Observations

1. **Custom beats basic by a wide margin on real-world text**
   (`fi-manual-v1`): +28.6 pts lemma, +24.2 pts coverage. This is the
   compound + possessive enrichment paying off on news/article text
   that contains derived/compound nouns.

2. **Grammar accuracy is 0% across the board.** The case-suffix
   enrichment rule does fire for novel inflected forms, but at 99%
   dict coverage the dict already returns the correct lemma and POS
   without going through suffix-stripping — so no `grammar_label` is
   emitted. The eval only credits grammar when the parser produces a
   label, so this rounds to zero. Two ways to fix this in a later
   pass:
   - Tag dict-derived hits with a grammar label inferred from the
     suffix (cheap heuristic, lossy).
   - Run the suffix-strip path **alongside** the dict path and prefer
     it when both agree.

3. **Per-case timing rounds to 0ms.** The current report uses integer
   milliseconds and most cases parse in <1ms, so we have no useful
   speed signal. To get timing, switch to nanoseconds or batch many
   cases per run. This is a tooling fix, not a parser fix.

4. **fi-grammar-v1 has surprisingly high lemma accuracy (96.8%)**
   even on basic. This says the dict already covers most of the
   grammatical phenomena directly — this dataset is a good ceiling
   indicator, not a stress test for enrichment.

## Files

- `2026-04-28-fi-core-v1.json` — raw report
- `2026-04-28-fi-manual-v1.json` — raw report
- `2026-04-28-fi-grammar-v1.json` — raw report

## Reproduction

```bash
make parser
export LD_LIBRARY_PATH="$(pwd)/parser/target/release:$LD_LIBRARY_PATH"
for ds in fi-core-v1 fi-manual-v1 fi-grammar-v1; do
  go run ./cmd/parsertest \
      -dataset "testdata/parser-eval/fi/gold/$ds.json" \
      -parsers basic,custom -warmup 2 -repeat 5
done
```
