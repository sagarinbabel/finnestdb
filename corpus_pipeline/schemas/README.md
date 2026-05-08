# Corpus Pipeline Schemas

The TSVs are written with Go `encoding/csv` using tab as the delimiter.

## `wordlist.tsv`

One row per distinct `(surface, lemma, pos, feats)` analysis. A surface can have
multiple rows when it has multiple analyses.

| Column | Meaning |
|---|---|
| `surface` | Exact token form seen in corpus text. |
| `surface_count_prose` | Occurrences of this surface across all prose sources in the current aggregate. |
| `surface_count_poetry` | Occurrences across poetry sources. |
| `surface_count_total` | Prose + poetry occurrences. |
| `doc_count_prose` | Count of prose documents containing the surface. |
| `doc_count_poetry` | Count of poetry documents containing the surface. |
| `source_counts_json` | Per-source occurrence counts as JSON. |
| `lang` | `fi` or `et`; redundant while files remain separate, useful when combined. |
| `lemma` | Dictionary/base form for this analysis. |
| `pos` | Part of speech. |
| `feats` | UD-style morphology such as `Case=Ine|Number=Sing`. |
| `analysis_sources` | Semicolon-separated sources for the analysis, e.g. `parser_choice;fst`. |
| `analysis_rank` | Rank among analyses for the same surface. |
| `is_parser_choice` | `1` when this is the parser-selected analysis. |
| `parser_version` | Parser version/fingerprint used during aggregate. |
| `fst_tables_sha` | FST table fingerprint used during aggregate. |
| `dict_fingerprint` | Dictionary DB fingerprint used during aggregate. |
| `example_ref_type` | `sentence` or `poem`, when an example was resolved. |
| `example_ref_id` | ID in `sentences.tsv` or `poems.tsv`. |
| `example_text` | Denormalized, truncated example text; removable because it can be rejoined by ID. |

## `wordlist-enriched.tsv`

One row per unique surface after external analyzer enrichment.

| Column | Meaning |
|---|---|
| `surface` | Exact token form. |
| `lang` | `fi` or `et`. |
| `external_lemma` | Lemma chosen by omorfi/vabamorf/estnltk adapter. |
| `external_pos` | External analyzer POS. |
| `external_feats` | External analyzer morphology. |
| `external_analysis_count` | Number of analyses returned by the external analyzer. |
| `external_source` | Analyzer name. |

## `sentences.tsv`

One row per unique reconstructed sentence text.

| Column | Meaning |
|---|---|
| `id` | Deterministic sentence ID within this aggregate. |
| `lang` | `fi` or `et`; redundant while files remain separate. |
| `text` | Deduplicated sentence text. |

## `sentence_occurrences.tsv`

One row per observed sentence occurrence.

| Column | Meaning |
|---|---|
| `sentence_id` | Foreign key to `sentences.tsv.id`. |
| `source` | Source slug. |
| `document_id` | Source-specific document ID. |
| `sentence_ix` | Sentence index within the document. |
| `quality_flags` | Semicolon-separated flags such as `very_short` or `has_url`. |
