# Ekilex Source Attribution

This directory contains lexical data derived from
[Ekilex](https://ekilex.ee/), operated by the Institute of the Estonian
Language (Eesti Keele Instituut). The primary corpus is **EKI ühendsõnastik
2026** (every word in `definitions/` carries an `eki` lexeme), supplemented
by content from ~150 additional Sõnaveeb datasets — see the contributor
table below.

## License

**CC BY 4.0** — https://creativecommons.org/licenses/by/4.0/

Per [Sõnaveeb's published guidance](https://sonaveeb.ee/about), recommended
citation form for content derived from this directory:

> Word or phrase. EKI ühendsõnastik 2026. Eesti Keele Instituut, Sõnaveeb
> 2026. https://sonaveeb.ee/&lt;word&gt; (DD.MM.YYYY)

For content sourced from a terminology database (e.g. `esterm`, `mil`,
`med`), substitute the database name for "EKI ühendsõnastik 2026".

## Per-dataset copyright

The Institute of the Estonian Language holds copyright on EKI
ühendsõnastik and Esterm. Specialized databases may have individual
contributors/authors as copyright holders. Consult
[Sõnaveeb's about page](https://sonaveeb.ee/about) for the authoritative
per-database listing and any terms beyond CC BY 4.0.

## Files

- `definitions/<letter>.jsonl` — one JSON object per word_id with lemma,
  morphology metadata (`homonym_nr`, `morphophono_form`, `inflection_type`,
  `word_class`, `morph_comment`), source `datasets`, and per-meaning
  content (`definitions_et`, `definitions_en`, `usages_et`, `usages_en`,
  `translations_en`).
- `forms/<letter>.tsv` — one row per inflected form: `lemma`, `form`,
  `morph_code`. Header row included. Suitable for direct ingest into the
  parser's forms table.
- `eki-public-words-2026-et.jsonl` — the public headword index used by
  the scraper as its work queue.

Files are sharded by the first lowercase letter of the lemma. Estonian
letters (ä, ö, ü, õ, š, ž) each get their own shard; everything else
(digits, symbols, foreign scripts) lands in `_other`.

## Modifications applied

The data here is a transformation of the raw Ekilex `/api/word/details`
payloads, not a copy. Specifically the reduction:

- filters to Estonian-language headwords only (`lang=est`);
- collapses each lexeme into a meaning record carrying only definitions,
  usages, and English translations (audio, image refs, internal IDs other
  than `meaning_id`, and HTML markup are dropped);
- sorts and deduplicates string fields within each meaning;
- deduplicates inflected forms within a word and sorts them by form;
- emits two stable, line-oriented formats (JSONL and TSV) sorted by
  `word_id`.

See [`cmd/fetchekilex`](../../cmd/fetchekilex/main.go) for the scraping
step and [`cmd/reduceekilex`](../../cmd/reduceekilex/main.go) for the
exact reduction logic. Re-running them with a fresh API key reproduces
this directory.

## Snapshot

Snapshot taken: **2026-05-05**. Source headword count: **176,993**.
Re-fetching from `/api/public_word/eki` will surface any additions made
to EKI ühendsõnastik since this date.
