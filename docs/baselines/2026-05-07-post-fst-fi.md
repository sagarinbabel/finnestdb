# Finnish parser comparison — 2026-05-07j

**Run date:** 2026-05-07 · **Branch:** `main` · **Commit:** [`42e95d9`](https://github.com/sagarinbabel/finnestdb/commit/42e95d9) · **Parser code last touched in:** PR [#109](https://github.com/sagarinbabel/finnestdb/pull/109) ([`da37ae9`](https://github.com/sagarinbabel/finnestdb/commit/da37ae9), case-suffix grammar-label stopgap) · **Methodology:** [`../PARSER_EVAL_METHODOLOGY.md`](../PARSER_EVAL_METHODOLOGY.md)

> **Provenance.** Re-measured on `main` after PR [#109](https://github.com/sagarinbabel/finnestdb/pull/109)
> ([`da37ae9`](https://github.com/sagarinbabel/finnestdb/commit/da37ae9)) added a
> case-suffix grammar-label stopgap and the artifact-policy migration
> ([`8d75dbf`](https://github.com/sagarinbabel/finnestdb/commit/8d75dbf), shipped
> via PR [#125](https://github.com/sagarinbabel/finnestdb/pull/125)) reduced the
> embedded FST tables (`pkg/lemmatizer-fi-et/tables/{fi,et}_min.json`) to smoke
> fixtures.
>
> The 2026-05-06i FINAL snapshots ([`2026-05-06-final-fi*`](2026-05-06-final-fi.md))
> were taken on the maintainer's PR4 branch where `fi_min.json` had been locally
> regenerated against a full Voikko `mor.vfst`. Those numbers are reproducible
> only with that local table generation; on a fresh clone of `main` today the FST
> runtime has 12-form coverage by design (artifact policy: no upstream
> transducer blobs, derived tables stay locally generated). This baseline
> reflects two simultaneous effects:
>
> 1. **+** PR #109's case-suffix grammar-label stopgap attaches `GrammarLabel`
>    to dict-resolved tokens whenever the suffix-strip independently agrees on
>    the lemma. Reproducible from the public code on every clone.
> 2. **−** The public-facing FST tables (smoke fixtures) cover only 12 forms,
>    so the FST analyzer's coverage contribution to lemma/coverage scores is
>    effectively zero on real-world tokens. The FINAL baseline measured those
>    contributions; this baseline does not.
>
> **Dictionary state at measurement** (no Ekilex bulk drop active):
>
> | Lang | Forms | Lemmas | Sources |
> |---|---:|---:|---|
> | FI | 26,826,071 | 259,145 | kaikki.org Finnish |

## Headline numbers — custom vs. omorfi (Helsinki HFST)

All figures are percentages. **Bold = winner of that cell.** Curated sets are
hand-built to exercise specific parser features; UD test sets are real-world
treebanks (proper nouns, foreign words, hyphenated compounds, numerals,
informal punctuation) — the "in the wild" reading.

| Dataset | Cases | Tokens | Lemma custom | Lemma omorfi | POS custom | POS omorfi | Gram custom | Gram omorfi | Full custom | Full omorfi | Cov custom | Cov omorfi |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| fi-core (curated) | 6 | 23 | 85.0 | 85.0 | **90.0** | 85.0 | 30.0 | **100.0** | 50.0 | **80.0** | 95.7 | 95.7 |
| fi-grammar (curated) | 80 | 156 | **96.8** | 95.5 | **98.1** | 96.2 | 59.5 | **100.0** | 51.9 | **94.2** | 99.7 | **100.0** |
| fi-manual-v1 (curated) | 22 | 187 | **81.4** | 74.3 | **85.7** | 77.1 | 6.7 | **60.0** | 64.3 | **71.4** | **91.2** | 87.6 |
| fi-manual-v2 (curated) | 4 | 12 | 88.9 | **100.0** | 100.0 | 100.0 | 33.3 | **100.0** | 66.7 | **100.0** | 100.0 | 100.0 |
| ud-fi-ftb-test (UD) | 1867 | 13,973 | 71.4 | **83.3** | 66.4 | **75.0** | 22.4 | **83.0** | 41.1 | **69.9** | 92.5 | **94.4** |
| ud-fi-ood-test (UD) | 2106 | 16,151 | 62.5 | **68.5** | 65.6 | **70.7** | 23.0 | **77.5** | 39.3 | **61.6** | 85.0 | **85.6** |
| ud-fi-pud-test (UD) | 1000 | 13,474 | 60.0 | **68.1** | 66.0 | **72.0** | 19.7 | **77.0** | 34.7 | **62.0** | 85.5 | **85.6** |
| ud-fi-tdt-test (UD) | 1554 | 17,951 | 60.2 | **69.2** | 67.8 | **75.6** | 22.2 | **80.9** | 36.3 | **63.3** | 89.6 | **89.9** |

**Total evaluated tokens:** 378 (curated) + 61,549 (UD) = **61,927 FI tokens** across 8 datasets.

## Headline gaps (omorfi − custom, percentage points)

Positive Δ = omorfi is ahead; negative = custom is ahead.

| Dataset | Δ Lemma | Δ POS | Δ Grammar | Δ Full | Δ Coverage |
|---|---:|---:|---:|---:|---:|
| fi-core | +0.0 | −5.0 | **+70.0** | +30.0 | +0.0 |
| fi-grammar | −1.3 | −1.9 | **+40.5** | +42.3 | +0.3 |
| fi-manual-v1 | −7.1 | −8.6 | **+53.3** | +7.1 | −3.6 |
| fi-manual-v2 | +11.1 | +0.0 | **+66.7** | +33.3 | +0.0 |
| ud-fi-ftb-test | **+11.9** | **+8.6** | **+60.6** | **+28.8** | +1.9 |
| ud-fi-ood-test | **+6.0** | **+5.1** | **+54.5** | **+22.3** | +0.6 |
| ud-fi-pud-test | **+8.1** | **+6.0** | **+57.3** | **+27.3** | +0.1 |
| ud-fi-tdt-test | **+9.0** | **+7.8** | **+58.7** | **+27.0** | +0.3 |

**Reading**

- **The curated-set picture (lemma/POS)** is misleadingly favourable. On `fi-grammar` and `fi-manual-v1` custom *beats* omorfi by 1–9pp lemma — but those sets are hand-built to exercise dictionary-friendly cases.
- **The UD picture is the realistic one.** Across **6,527 cases / 61,549 evaluated tokens** custom runs **9–12pp behind omorfi on lemma**, **5–9pp behind on POS**, and **55–60pp behind on grammar**. UD treebanks cover proper nouns, foreign words, hyphenated compounds, numerals, informal punctuation — the long tail the dictionary doesn't have a clean answer for. The headline LEARNINGS finding ([§2026-05-07](../LEARNINGS.md)) "real-world lemma is 53–60%, not 97%" is reproduced and now under continuous measurement.
- **Grammar gap is the dominant gap.** Across every dataset (curated and UD), the omorfi grammar lead is 40–73pp. PR #109's case-suffix stopgap moved 0→20–60% but omorfi sits at 77–100%. The remaining gap is what the FEATS migration (in flight) + the FST runtime (once tables aren't smoke-only) need to close.
- **Coverage is close.** Custom is within 1–2pp of omorfi on coverage on every UD set — the dictionary catches *some* lemma for ~90% of UD tokens; the question is whether it's the *right* lemma. That's the lemma-disambiguation gap, not a coverage gap.

## Net effect vs `2026-05-06-final-fi*` (custom parser)

| Dataset | Lemma | POS | Grammar | Full | Coverage |
|---|---:|---:|---:|---:|---:|
| fi-core | +0.0 | +0.0 | **+30.0** | **+15.0** | −4.3 |
| fi-grammar | −0.6 | −0.6 | **+58.1** | **+27.6** | −0.3 |
| fi-manual-v1 | −1.5 | −1.4 | −6.6 | +1.4 | −5.7 |
| fi-manual-v2 | +0.0 | +0.0 | **+33.3** | +11.1 | +0.0 |

The grammar/full lifts come from PR #109's case-suffix `GrammarLabel` stopgap
attached additively after dict Step 1 — reproducible from the public repo. The
lemma/coverage drops on `fi-core` and `fi-manual-v1` track the FST-table
coverage gap (smoke fixture vs. maintainer-local full table) — they are not
regressions in the parser code.

## Reproduction

See [`../PARSER_EVAL_METHODOLOGY.md`](../PARSER_EVAL_METHODOLOGY.md) for the full
methodology. Quick path:

```
make parser
export DYLD_LIBRARY_PATH="$(pwd)/parser/target/release:$DYLD_LIBRARY_PATH"
export FINNESTDB_OMORFI_CMD="$(pwd)/.venv-omorfi/bin/python $(pwd)/scripts/omorfi_adapter_example.py"
export FINNESTDB_OMORFI_TIMEOUT=60s   # UD treebanks have occasional long sentences
make setup-omorfi   # populates ~/.cache/omorfi/omorfi.analyse.hfst (25 MB tarball; pyhfst-backed in 0.9.12)
bash scripts/parser-comparison.sh
```

The bundled `parser-comparison.sh` discovers every `testdata/parser-eval/fi/gold/*.json`
not matching `-dev-v` (held-out test discipline). With omorfi at ~400 ms/case
the four UD test sets together take ~50 min for a single timed pass. The
committed UD JSON reports (`docs/baselines/2026-05-07-post-fst-ud-fi-*-test.json.gz`)
are summary-stripped — `cases[]` per-case detail is omitted to keep file sizes
proportional to the existing curated baselines (~250 KB instead of ~35 MB).
Re-running locally produces the full `cases[]` array; `dataset.case_count` and
`summary.<parser>.expected_tokens` retain the case/token counts.
