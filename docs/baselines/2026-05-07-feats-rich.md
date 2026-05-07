# 2026-05-07 — FEATS-rich gold + dict + adapters end-to-end

**Parser version stamp**: `2026.05.07k` (`parsecore.ParserVersion` to be bumped on land)

**Detail JSONs**:
- FI: [`2026-05-07-feats-rich-fi-core.json.gz`](2026-05-07-feats-rich-fi-core.json.gz), [`2026-05-07-feats-rich-fi-grammar.json.gz`](2026-05-07-feats-rich-fi-grammar.json.gz), [`2026-05-07-feats-rich-fi-manual-v1.json.gz`](2026-05-07-feats-rich-fi-manual-v1.json.gz), [`2026-05-07-feats-rich-fi-manual-v2.json.gz`](2026-05-07-feats-rich-fi-manual-v2.json.gz)
- ET: [`2026-05-07-feats-rich-et-grammar.json.gz`](2026-05-07-feats-rich-et-grammar.json.gz), [`2026-05-07-feats-rich-et-manual.json.gz`](2026-05-07-feats-rich-et-manual.json.gz)

**Reproduce**:
```bash
make parser
export DYLD_LIBRARY_PATH="$(pwd)/parser/target/release:$DYLD_LIBRARY_PATH"
export FINNESTDB_OMORFI_CMD="$(pwd)/.venv-omorfi/bin/python $(pwd)/scripts/omorfi_adapter_example.py"
bash scripts/parser-comparison.sh -o /tmp/feats-rich-out/fi-summary.md \
    testdata/parser-eval/fi/gold/fi-core-v1.json \
    testdata/parser-eval/fi/gold/fi-grammar-v1.json \
    testdata/parser-eval/fi/gold/fi-manual-v1.json \
    testdata/parser-eval/fi/gold/fi-manual-v2.json
go run ./cmd/parsertest -dataset testdata/parser-eval/fi/gold/fi-manual-v1.json \
    -parsers basic,custom,omorfi -warmup 2 -repeat 5 \
    -out reports/parser-eval/<ts>-fi-manual-v1.json   # workaround for the v1/v2 slug collision
bash scripts/parser-comparison-et.sh -o /tmp/feats-rich-out/et-summary.md
```

## What changed

The 6 manual gold files now carry full UD FEATS (~370 tokens), the `cmd/importdict` kaikki importer translates `Tags []string` to `feats` on every form row, the case-suffix-strip path projects `Case=` from its emitted `grammar_label`, the FST tables persist a `Feats` field via the new `pkg/lemmatizer-fi-et/udfeats` composer, and the EstNLTK adapter emits the same UD FEATS shape omorfi already did. Result: every layer of the pipeline that produces or consumes morphological information now speaks the same UD FEATS vocabulary.

The eval framework's per-FEATS-attribute table at [parser-compare/main.go:270](../../cmd/parser-compare/main.go:270) was complete since [#130](https://github.com/sagarinbabel/finnestdb/pull/130) but had been silent on every committed baseline because no gold set carried FEATS to score against. This is the first baseline where it fires across every dataset.

## Headline FI numbers (custom parser, lemma/POS unchanged from `2026-05-07j`)

| Dataset | Cases | Lemma | POS | Grammar | Full | Coverage |
|---|---:|---:|---:|---:|---:|---:|
| fi-core | 6 | 85.0 | 90.0 | 30.0 | 0.0 | 95.7 |
| fi-grammar | 80 | 96.8 | 98.1 | 59.5 | 0.0 | 99.7 |
| fi-manual-v1 | 22 | 81.4 | 85.7 | 6.7 | 0.0 | 91.2 |
| fi-manual-v2 | 4 | 88.9 | 100.0 | 33.3 | 11.1 | 100.0 |

The `Full` column is now correctly gated on FEATS-equality too, hence the drop from `2026-05-07j` (where `Full` ignored FEATS). The lemma/POS/grammar columns are unchanged from the prior baseline's custom-parser numbers — the parser's resolution behavior didn't change in this entry; the eval just has more to compare against.

## Headline ET numbers (custom parser)

| Dataset | Cases | Lemma | POS | Grammar | Full | Coverage |
|---|---:|---:|---:|---:|---:|---:|
| et-grammar | 50 | 88.6 | 96.2 | 19.6 | 0.0 | 98.9 |
| et-manual | 4 | 77.8 | 77.8 | 0.0 | 0.0 | 91.7 |

## Per-FEATS-attribute accuracy — the new metric

Every committed FI/ET baseline now reports per-attribute accuracy across the 13 FI / 7 ET FEATS attributes the gold sets carry. Headline view:

### FI per-attribute (omorfi vs custom)

| Attribute | fi-core eligible | fi-core omorfi | fi-grammar eligible | fi-grammar omorfi | fi-manual-v1 eligible | fi-manual-v1 omorfi |
|---|---:|---:|---:|---:|---:|---:|
| Case | 17 | 100.0% | 130 | 99.2% | (24 fi-manual combined) | 100.0% |
| Number | 19 | 100.0% | 154 | 100.0% | 31 | 100.0% |
| Mood | 2 | 100.0% | 25 | 100.0% | 1 | 100.0% |
| Tense | 2 | 100.0% | 21 | 100.0% | 1 | 100.0% |
| Person | 3 | 100.0% | 24 | 100.0% | 1 | 100.0% |
| VerbForm | 3 | 100.0% | 28 | 100.0% | 1 | 100.0% |
| Voice | 3 | 100.0% | 28 | 100.0% | 1 | 100.0% |
| Degree | 3 | 100.0% | 13 | 100.0% | — | — |
| Number[psor] | 1 | 100.0% | 8 | 100.0% | 3 | 100.0% |
| Person[psor] | 1 | 100.0% | 14 | 100.0% | 3 | 100.0% |
| PronType / PartForm / InfForm / Style | 1–2 each | 100.0% | 1–4 each | 100.0% | — | — |

omorfi scores ≥99% on every FEATS attribute on every dataset — expected, since the gold was seeded from omorfi's own analysis with the gold's existing `grammar_label` as the Case anchor (deterministic overwrite).

`basic` and `custom` score 0% on every FEATS attribute because the live SQLite DB this measurement runs against doesn't yet carry FEATS — it was imported before Stage 1 of this PR's kaikki tag mapper landed, so `forms.feats IS NULL` for all 27.2M rows. To populate, re-run import:
```bash
go run ./cmd/importdict -lang fi -reimport -db finnestdb.db -file localdata/kaikki.org-fi.jsonl.gz
# Or, with no DB downtime:
go run ./cmd/importdict -lang fi -backfill-feats -db finnestdb.db -file localdata/kaikki.org-fi.jsonl.gz
```

After re-import, `custom` will pick up FEATS via the dict layer and `featsFromFSTAnalysis` will compose runtime FEATS for FST-resolved forms — both paths exist and are unit-tested ([`internal/store/dict_test.go::TestCaseSuffixStrip_Inessive`](../../internal/store/dict_test.go), [`cmd/importdict/feats_test.go::TestImportJSONL_WritesFeats`](../../cmd/importdict/feats_test.go)). The next baseline (`2026-05-07l` or later) will measure the post-import state.

### ET per-attribute (estnltk vs custom)

| Attribute | et-grammar eligible | et-grammar estnltk | et-manual eligible | et-manual estnltk |
|---|---:|---:|---:|---:|
| Case | 51 | 94.1% | 6 | 100.0% |
| Number | 102 | 100.0% | 9 | 100.0% |
| Mood | 24 | 100.0% | 1 | 100.0% |
| Tense | 24 | 100.0% | 1 | 100.0% |
| Person | 23 | 100.0% | 1 | 100.0% |
| VerbForm | 25 | 100.0% | 1 | 100.0% |
| Voice | 24 | 100.0% | 1 | 100.0% |

estnltk Case at 94.1% on et-grammar reflects 3 case-disagreement edge cases the seeding pass surfaced in the `.diff.md` files — Estonian "õuna" / "kirja" can analyse as Gen or Par; gold's grammar_label said partitive, estnltk's per-token reading said genitive without sentence context. Gold won, surfaced in the report, and the user-visible diff documents the disagreement.

## Pipeline diagram (new state)

```
[kaikki JSONL] --tags-> [kaikkiTagsToFeats] -+
                                              \
[Ekilex morph_code] --> [ekilexMorphToFeats] -+--> forms.feats (SQLite)
                                                              |
[Voikko VFST] --> [voikkomap.Parse + udfeats.Compose] ---> Analysis.Feats
[Giellalt HFST] --> [giellaltmap.Parse + udfeats.Compose] -> Analysis.Feats
                                                              |
                                                              v
[case-suffix strip] --grammar_label--> [featsFromCaseLabel] --+--> custom parser
                                                              |    output Token.Feats
[omorfi adapter Python] --ufeats dict-->                      |
[estnltk adapter Python] --vabamorf_form_to_feats-->          |
                                                              v
                                                    [eval per-FEATS table]
```

## Open issues this surfaced

- **Live DB lacks FEATS**: the production DB at `finnestdb.db` was last imported on 2026-05-05, before any of the FEATS mappers. Until re-import, the `custom` parser can't show FEATS in its output. The kaikki source JSONLs aren't on this machine — `make import-dict-fi` would need to re-fetch from kaikki.org first. Captured as runbook above.
- **The fi-manual-v1/v2 collision is unfixed**: per the [reference_eval_setup memory](../../.claude/projects/-Users-sagar-Downloads-projects-finnestdb/memory/reference_eval_setup.md), `parser-comparison.sh` slugifies `dataset.name` and overwrites v1's report with v2's. This baseline worked around it by re-running v1 explicitly with `-out`. A fix would land alongside this PR or as a separate cleanup.
- **OOV compound nouns in fi-manual-v1** got Case= via the suffix-strip fallback in `cmd/enrichgoldfeats`, but Number= is not inferred — surface alone can't disambiguate Sg vs Pl for unanalysed compounds. Marked in [`fi-manual-v1.diff.md`](../../testdata/parser-eval/fi/gold/fi-manual-v1.diff.md) (16 tokens). A maintainer with FI fluency can manually disambiguate; for now the FEATS column is honestly empty for those rare tokens.
