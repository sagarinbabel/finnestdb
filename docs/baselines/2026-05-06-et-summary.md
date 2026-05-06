# Estonian Parser Baseline — 2026-05-06

Refresh of [`2026-04-28-et-summary.md`](2026-04-28-et-summary.md) on
current `main` (commit `46d8b77`). Captures the post-Ekilex state and
surfaces a real lemma/POS regression that needs fixing before more
form-source additions land.

## Dictionary state

| Language | Forms | Lemmas | Sources |
|---|---:|---:|---|
| ET | 6,178,514 | 354,231 | kaikki.org Estonian + Ekilex bulk drop ([#78](https://github.com/sagarinbabel/finnestdb/pull/78)) |
| FI | 26,826,071 | 259,145 | kaikki.org Finnish |

ET form count grew **27×** from April (228,428 → 6,178,514) thanks
to the Ekilex import (`cmd/importekilexdetails`). Multi-lemma schema
([#78](https://github.com/sagarinbabel/finnestdb/pull/78))
preserves multiple `(lemma, pos)` candidates per surface form.

## Numbers

Run: `cmd/parsertest -warmup 1 -repeat 3 -parsers basic,custom`.

| Dataset (cases) | Parser | Lemma | POS | Grammar | Full | Coverage |
|---|---|---:|---:|---:|---:|---:|
| **et-grammar-v1** (50) | basic | 80.0 | 83.8 | 0.0 | 42.9 | **100.0** |
| et-grammar-v1 | custom | 80.0 | 83.8 | 0.0 | 42.9 | **100.0** |
| **et-manual-v1** (4) | basic | 88.9 | 88.9 | 0.0 | 22.2 | 91.7 |
| et-manual-v1 | custom | 88.9 | 88.9 | 0.0 | 22.2 | **100.0** |

## Deltas vs. 2026-04-28 (et-grammar-v1, the only directly comparable dataset)

| Parser | Δ Lemma | Δ POS | Δ Grammar | Δ Full | Δ Coverage |
|---|---:|---:|---:|---:|---:|
| basic | **-8.6** | **-13.3** | 0.0 | -0.9 | **+7.1** |
| custom | **-7.6** | **-13.3** | -2.0 | 0.0 | **+5.4** |

**Coverage jumped to 100%** (Ekilex covers everything in the gold set)
**but lemma and POS accuracy dropped 8–13 percentage points.** This is
a real regression: 20 of 178 tokens went from correct → wrong. 8 went
the other way. Net: 12 tokens worse on basic.

## Root cause: arbitrary multi-lemma resolution

`internal/store/dict.go BatchLookupForms` uses a single-row `QueryRow`
against `forms`. With the multi-lemma PK
`(form, lang, lemma, pos)` and Ekilex contributing many homonym
candidates, the lookup now returns "an arbitrary candidate" — and the
arbitrary one is often wrong. Pattern from the regressed cases:

| Surface | Wanted | Got | Sense |
|---|---|---|---|
| `linnas` | `linn`/NOUN ("in the city") | `Linna`/PROPN | Place name homonym wins |
| `raamatu(t)` | `raamat`/NOUN ("book") | `Raamatu`/PROPN | Place name homonym wins |
| `õuna` | `õun`/NOUN ("apple") | `Õuna`/PROPN | Place name homonym wins |
| `Koer(ad)` | `koer`/NOUN ("dog") | `koer`/ADJ | Adjective homonym wins |
| `naeris` | `naerma`/VERB ("laughed") | `naeris`/NOUN | Noun homonym wins |
| `keelt` | `keel`/NOUN ("language") | `kee`/NOUN | Different lemma form wins |

Most of these are PROPN winning over NOUN — Estonian place names share
inflected forms with common nouns, and the Ekilex bulk drop adds the
proper-noun candidates.

## Implications for the Finnish lexical plan

This is the regression Phase 4 (Voikko) would trigger on FI without a
fix: every form-source we add multiplies homonym candidates, and
`BatchLookupForms` arbitrarily picks one. **The source-priority +
POS-aware resolution layer described in
[LEXICAL_PLAN.md "Resolution Layer"](../LEXICAL_PLAN.md)
needs to land before more form-sources do**, or each new source
trades coverage gains for accuracy losses.

Two compounding issues:

1. **`cmd/importekilexdetails` does not set the row-level `source` /
   `source_priority` columns** on the lemmas/forms it inserts —
   only `dict_metadata`. All ET rows currently have `source=''`,
   `source_priority=0`. Even if the read path ranked by priority,
   it would have nothing to rank by.
2. **`BatchLookupForms` does not order by source priority or use any
   POS preference.** It's a single `QueryRow`.

Both need fixing before Phase 4. Suggested follow-up PR (call it
"PR 0.5"):

- `cmd/importekilexdetails`: tag inserted rows with
  `source='ekilex'`, `source_priority=20`. Mirror the
  `cmd/importdict -source-key ekilex` convention (#67/#68).
- `internal/store/dict.go`: query all matching rows for a form,
  prefer (a) higher `source_priority`, then (b) NOUN/VERB over PROPN
  when the surface form's case is lowercase, then (c) deterministic
  tiebreak by source name. Re-run this baseline; expect ET regression
  to close.

## Standing observations

- **Grammar accuracy is 0%.** Same as Finnish — no source populates
  `feats` yet. Ekilex's morph codes are present in `data/ekilex/forms/`
  but the importer drops them rather than mapping into UD features.
  Worth picking up either alongside PR 0.5 or in parallel with the FI
  Phase 4 (Voikko) work.
- **Coverage is ~100%** post-Ekilex. The bulk drop covers more than
  the gold set has cases for. Coverage is no longer the bottleneck for
  Estonian — accuracy is.

## Files

- `2026-05-06-et-grammar-v1.json`
- `2026-05-06-et-manual-v1.json`

## Reproduction

```bash
make parser
export DYLD_LIBRARY_PATH="$(pwd)/parser/target/release:$DYLD_LIBRARY_PATH"

# Dictionary state must match the Estonian row in the table above.
# A fresh build:
make import-dict-fi
make import-dict-et
make import-ekilex-et
make import-ekilex-details-et   # the post-#78 piece — without this, ET coverage stays ~93%

for ds in et-grammar-v1 et-manual-v1; do
  go run ./cmd/parsertest \
      -dataset "testdata/parser-eval/et/gold/$ds.json" \
      -parsers basic,custom -warmup 1 -repeat 3 \
      -out "docs/baselines/2026-05-06-$ds.json"
done
```
