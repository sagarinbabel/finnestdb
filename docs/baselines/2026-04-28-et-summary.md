# Estonian Parser Baseline + Dictionary Verification - 2026-04-28

First Estonian baseline measurement after **C1** (gold dataset of
50 cases) and **C2** (kaikki.org Estonian dictionary import).

## Dictionary state after `make import-dict-et`

| Language | Forms | Lemmas | Source |
|---|---:|---:|---|
| ET | 228,428 | 12,606 | kaikki.org Estonian dump |
| FI | 12,262,117 | 145,672 | kaikki.org Finnish dump |

Estonian has **~50× fewer forms** than Finnish in the kaikki.org
data. This is a real coverage limitation, not a bug - kaikki.org's
Wiktionary-derived pipeline produces less material for Estonian
than Finnish. Future Estonian work may need a richer source
(e.g. EstNLTK lexicons, Sõnaveeb, etc.).

## Eval numbers (`et-grammar-v1.json`, 50 cases)

| Parser | Lemma | POS | Grammar | Full | Coverage |
|---|---:|---:|---:|---:|---:|
| basic  | **88.6%** | 97.1% | 0.0% | 43.8% | 92.9% |
| custom | 87.6% | 97.1% | 2.0% | 42.9% | 94.6% |

## Findings

1. **Coverage went from 0% to 92.9%**. The dictionary import is
   working as expected. C1's prior measurement (0%) confirmed the
   baseline floor; this measurement confirms the import is the lever
   that unlocks ET parsing.

2. **`custom` is slightly *worse* than `basic` on Estonian** (-1 pt
   lemma). Drilling into the case reports shows exactly **one token**
   regressed:

   ```
   et-0032  surface="Rongisõit"  gold=rongisõit  custom=rongõis
            source=compound
   ```

   The compound splitter is producing a lemma that isn't a substring
   of the input, suggesting it's gluing two found stems whose
   combined characters don't reconstruct the surface form correctly.
   The same compound rule is fine on Finnish, so this is an
   ET-specific edge case. Worth fixing in **C3** (Estonian rule
   refinement).

3. **Grammar accuracy is 2% (one match)**. As on Finnish, the
   case-suffix enrichment path almost never fires because the
   dictionary already returns the correct (lemma, pos) for inflected
   forms. The 2% comes from one case where stem alternation made the
   inflected form absent from the dict, and the suffix-strip path
   produced a valid stem-only match in the lemmas table.

4. **POS accuracy is 97.1%**, very high. The Rust tokenizer's
   stub-POS heuristic plus dictionary lookup gets POS right for the
   vast majority of tokens.

## Reproducing

```bash
make parser
make import-dict-et
export LD_LIBRARY_PATH="$(pwd)/parser/target/release:$LD_LIBRARY_PATH"
go run ./cmd/parsertest \
    -dataset testdata/parser-eval/et/gold/et-grammar-v1.json \
    -parsers basic,custom -warmup 2 -repeat 5
```

## Companion files

- `2026-04-28-et-grammar-v1.json.gz` - raw eval report
