# Corpus Pipeline Schemas

The TSVs are written with Go `encoding/csv` using tab as the delimiter.

## `wordlist.tsv`

Canonical parser-evidence export. One row per distinct
`(surface, lemma, pos, feats)` analysis; a surface can have multiple rows
when it has multiple analyses.

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

`example_text` was previously the 20th column but was removed on
2026-05-09 - at full FI scale it accounted for the majority of
`wordlist.tsv` size for information already available elsewhere.
Reconstruct example bodies by joining `example_ref_id` against
`sentences.tsv.id` when `example_ref_type=sentence`, or against
`poems.tsv.id` when `example_ref_type=poem`. The user-friendly export
documented below carries the same `example_ref` pair.

## `wordlist_user_friendly.tsv`

Learner-facing export. One row per analysis (same row count as
`wordlist.tsv`). Adds the dictionary gloss and splits the UD FEATS string
into named morphology columns so a UI doesn't need to parse the
pipe-delimited string itself.

The `meaning` column mirrors the precedence used by the runtime read path
(`store.BatchLookupGlosses`):

1. The translation row in `translations` whose `source` matches the
   `lemmas` row's `source` for the same `(lemma, pos, lang)`, picked by
   `lemmas.source_priority DESC, translations.sense_idx ASC`.
2. `lemmas.gloss`, when no matching-source translation exists for the
   `(lemma, pos, lang)`.
3. Empty, when neither is populated.

Source coupling matters: `cmd/importekilexdetails` can upgrade an
existing kaikki lemma to `source='ekilex'` while preserving the older
kaikki gloss (the empty-gloss guard). The user-friendly export then
correctly returns the matching ekilex translation, not the preserved
kaikki gloss - same answer the server's read path returns.

| Column | Meaning |
|---|---|
| `surface` | Exact token form. |
| `meaning` | Dictionary gloss for `(lemma, pos, lang)`. Resolved by the precedence above. Empty when the dictionary doesn't list the headword and no translation exists. |
| `lang` | `fi` or `et`. |
| `lemma` | Dictionary/base form for this analysis. |
| `pos` | Part of speech. |
| `case` | UD `Case` value, or empty. |
| `number` | UD `Number` value, or empty. |
| `mood` | UD `Mood` value, or empty. |
| `tense` | UD `Tense` value, or empty. |
| `person` | UD `Person` value, or empty. |
| `voice` | UD `Voice` value, or empty. |
| `verbform` | UD `VerbForm` value, or empty. |
| `feats` | Full UD FEATS string preserved for completeness. |
| `surface_count_prose` | Same as canonical wordlist. |
| `surface_count_poetry` | Same as canonical wordlist. |
| `surface_count_total` | Same as canonical wordlist. |
| `doc_count_prose` | Same as canonical wordlist. |
| `doc_count_poetry` | Same as canonical wordlist. |
| `source_counts_json` | Per-source occurrence counts as JSON. |
| `analysis_sources` | Same as canonical wordlist. |
| `analysis_rank` | Rank among analyses for the same surface. |
| `is_parser_choice` | `1` when this is the parser-selected analysis. |
| `parser_version` | Same as canonical wordlist. |
| `fst_tables_sha` | Same as canonical wordlist. |
| `dict_fingerprint` | Same as canonical wordlist. |
| `example_ref_type` | `sentence` or `poem`, when an example was resolved. |
| `example_ref_id` | ID in `sentences.tsv` or `poems.tsv`. |

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

## `sentences_user_friendly.tsv`

A filtered, learner-facing subset of `sentences.tsv`. The canonical
`sentences.tsv` remains auditable and keeps every deduped sentence-like unit,
while this export omits title-only, name-only, front-matter, URL/ISBN, and other
obvious extraction-residue rows.

| Column | Meaning |
|---|---|
| `id` | The matching deterministic `sentences.tsv.id`. |
| `lang` | `fi` or `et`; redundant while files remain separate. |
| `text` | Sentence text suitable for learner-facing examples or manual review. |

## `sentence_occurrences.tsv`

One row per observed sentence occurrence.

| Column | Meaning |
|---|---|
| `sentence_id` | Foreign key to `sentences.tsv.id`. |
| `source` | Source slug. |
| `document_id` | Source-specific document ID. |
| `sentence_ix` | Sentence index within the document. |
| `quality_flags` | Semicolon-separated flags such as `very_short` or `has_url`. |
