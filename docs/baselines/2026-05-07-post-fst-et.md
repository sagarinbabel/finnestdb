# Estonian parser comparison — 2026-05-07j

**Run date:** 2026-05-07 · **Branch:** `main` · **Commit:** [`42e95d9`](https://github.com/sagarinbabel/finnestdb/commit/42e95d9) · **Parser code last touched in:** PR [#109](https://github.com/sagarinbabel/finnestdb/pull/109) ([`da37ae9`](https://github.com/sagarinbabel/finnestdb/commit/da37ae9), case-suffix grammar-label stopgap, bilingual) · **Methodology:** [`../PARSER_EVAL_METHODOLOGY.md`](../PARSER_EVAL_METHODOLOGY.md)

> **Provenance.** Re-measured on `main` after PR [#109](https://github.com/sagarinbabel/finnestdb/pull/109)
> ([`da37ae9`](https://github.com/sagarinbabel/finnestdb/commit/da37ae9), bilingual:
> applies for FI + ET) added a case-suffix grammar-label stopgap, and the
> artifact-policy migration ([`8d75dbf`](https://github.com/sagarinbabel/finnestdb/commit/8d75dbf),
> shipped via PR [#125](https://github.com/sagarinbabel/finnestdb/pull/125)) reduced
> the embedded ET FST table (`pkg/lemmatizer-fi-et/tables/et_min.json`) to a
> smoke fixture (~12 forms).
>
> The 2026-05-06i FINAL snapshots ([`2026-05-06-final-et*`](2026-05-06-final-et.md))
> were taken on the maintainer's PR4 branch where `et_min.json` had been locally
> regenerated against a full Giellalt `lang-est` HFST analyser. Those numbers are
> reproducible only with that local table generation. Two simultaneous effects
> show up vs FINAL:
>
> 1. **+** Case-suffix grammar-label stopgap (PR #109) attaches `GrammarLabel`
>    additively to dict-resolved tokens whenever the suffix-strip independently
>    agrees on the lemma. Reproducible from the public code on every clone.
> 2. **−** The public smoke `et_min.json` covers ~12 forms, so the analyser's
>    coverage contribution to lemma/coverage on real-world tokens is essentially
>    zero. The FINAL baseline measured those contributions; this baseline does
>    not.
>
> **Dictionary state at measurement** (no Ekilex bulk drop active in this DB):
>
> | Lang | Forms | Lemmas | Sources |
> |---|---:|---:|---|
> | ET | 392,863 | 186,798 | kaikki.org Estonian + Ekilex public-headwords (2026-05-05 snapshot) |
>
> The bulk Ekilex import ([#78](https://github.com/sagarinbabel/finnestdb/pull/78))
> is *not* loaded in this DB. The latent ET FEATS lift via Ekilex `morph_code`
> ([`2febc31`](https://github.com/sagarinbabel/finnestdb/commit/2febc31)) is therefore
> muted — see [`docs/LEARNINGS.md`](../LEARNINGS.md) for the projected ~95% ET
> grammar accuracy with the bulk drop active.

## Headline numbers — custom vs. estnltk (Vabamorf)

All figures are percentages. **Bold = winner of that cell.**

| Dataset | Cases | Lemma custom | Lemma estnltk | POS custom | POS estnltk | Gram custom | Gram estnltk | Full custom | Full estnltk | Cov custom | Cov estnltk |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| et-grammar (curated) | 50 | 88.6 | **98.1** | 96.2 | **97.1** | 19.6 | **92.2** | 51.4 | **92.4** | 98.9 | **100.0** |
| et-manual (curated) | 4 | 77.8 | **100.0** | 77.8 | **100.0** | 0.0 | **100.0** | 11.1 | **100.0** | 91.7 | **100.0** |

## Headline gaps (estnltk − custom, percentage points)

Positive Δ = estnltk is ahead; negative = custom is ahead.

| Dataset | Δ Lemma | Δ POS | Δ Grammar | Δ Full | Δ Coverage |
|---|---:|---:|---:|---:|---:|
| et-grammar | +9.5 | +1.0 | **+72.6** | +41.0 | +1.1 |
| et-manual | +22.2 | +22.2 | **+100.0** | +88.9 | +8.3 |

**Reading**

- **Estonian is one-sided.** estnltk dominates on every metric. The 2026-05-05
  estnltk ceiling baseline saw the same gap; PR #109's stopgap closed 0–2% to
  19.6%, which is the largest piece of the lift, but ~73pp grammar gap remains.
- **Lemma gap (88.6 → 98.1) is real-world disambiguation.** Multi-lemma cases like
  `naeris` NOUN-vs-VERB or `keelt` different-lemma-form need contextual
  disambiguation that estnltk's Vabamorf has and custom's dict ranker doesn't.
- **The headroom path forward** is the Ekilex `morph_code → FEATS` migration
  (in flight). Per [`../LEARNINGS.md`](../LEARNINGS.md), this is projected to
  lift ET grammar from 19.6% → ~95% in one PR — closes the gap to estnltk
  almost entirely without requiring an FST step.

## Net effect vs `2026-05-06-final-et*` (custom parser)

| Dataset | Lemma | POS | Grammar | Full | Coverage |
|---|---:|---:|---:|---:|---:|
| et-grammar | +0.0 | +0.0 | **+17.6** | **+8.5** | −1.1 |
| et-manual | −11.1 | +0.0 | −16.7 | −11.1 | −8.3 |

`et-grammar` grammar accuracy reflects PR #109's case-suffix `GrammarLabel`
stopgap (reproducible from public code). `et-manual` regressions track the
FST-table coverage gap (smoke `et_min.json` vs. maintainer-local full
Giellalt-derived table) — they are not regressions in the parser code. The
`et-manual` grammar drop (16.7 → 0.0) is the same effect: the FINAL baseline
saw a label from the local-full FST table; the smoke table doesn't have those
forms, and the case-suffix stopgap doesn't fire on the v1 manual cases either
(the suffix-strip lemma doesn't agree with the dict's resolution).

## Note on case-count expansion

This baseline currently measures only **54 ET cases** across two curated sets.
ET UD treebanks (`UD_Estonian-EDT`, `UD_Estonian-EWT`) are licensed CC BY-NC-SA
and so the derived gold JSON is gitignored under `localdata/parser-eval/et/gold/`
(after the PR [#131](https://github.com/sagarinbabel/finnestdb/pull/131)
consolidation). Anyone with local UD-ET clones can reproduce ET UD numbers by
running `make import-ud-gold-et` to populate that directory, then re-running
`make compare-parsers-et` — the comparison script auto-discovers gold sets
from both `testdata/` and `localdata/`, so no extra flags needed. See
[`../PARSER_EVAL_METHODOLOGY.md`](../PARSER_EVAL_METHODOLOGY.md) for the full plan.

## Reproduction

See [`../PARSER_EVAL_METHODOLOGY.md`](../PARSER_EVAL_METHODOLOGY.md) for the full
methodology. Quick path:

```
make parser
export DYLD_LIBRARY_PATH="$(pwd)/parser/target/release:$DYLD_LIBRARY_PATH"
make setup-estnltk   # populates .venv-estnltk + nltk_data
bash scripts/parser-comparison-et.sh
```
