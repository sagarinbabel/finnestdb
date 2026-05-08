# Meaning Sources

This document catalogues the lexical sources finnestdb consults for lemma
meanings, the license under each, and the strategy for combining them so the
user-friendly wordlist export ships with the highest possible gloss coverage.

It satisfies item 1 of `corpus_pipeline/docs/PR_ROADMAP.md` — meaning-sources
research, the prerequisite for the user-friendly wordlist work.

## Coverage Baseline (before this PR)

Measured against the full FI corpus (15M sentences, 7.5M unique surfaces) and
ET corpus (4M sentences, 3.4M unique surfaces) on 2026-05-09. See
`corpus_pipeline/reports/2026-05-09-gloss-coverage.md` for the run script and
raw numbers.

| Lang | Tokens (occurrences) | Tokens with gloss | Tokens in dict, no gloss | Tokens not in dict |
|------|---------------------:|------------------:|--------------------------:|-------------------:|
| FI   | 267,130,363          | 210,085,306 (78.65%) | 0 (0.00%)            | 57,045,057 (21.35%) |
| ET   | 96,090,526           |  71,054,502 (73.95%) | 4,450,819 (4.63%)    | 20,585,205 (21.42%) |

Lemma-table view (independent of corpus weighting):

| Lang | Source       | Lemma rows | With gloss | Coverage |
|------|--------------|-----------:|-----------:|---------:|
| FI   | kaikki       | 259,145    | 259,137    | 100.00%  |
| ET   | ekilex       | 178,032    |  67,367    |  37.84%  |
| ET   | kaikki       | 176,199    |   6,207    |   3.52%  |

The FI gap is "lemma not in any dictionary" — 21% of tokens come from compounds,
proper nouns, neologisms, and FST-generated analyses for surfaces the
dictionary does not list. No existing source fixes that bucket.

The ET gap has two parts: 4.6% of tokens are in the dictionary but the gloss
column is empty (the ekilex public-words importer adds the lemma without
populating gloss), and 21.4% of tokens are not in the dictionary at all. The
4.6% bucket is the immediate target for this PR.

## Source Catalogue

### kaikki.org (Wiktionary derivatives)

- **URL**: <https://kaikki.org/dictionary/Finnish/> and
  <https://kaikki.org/dictionary/Estonian/>.
- **License**: Wiktionary content is dual-licensed under CC BY-SA 4.0 and GFDL.
  kaikki.org redistributes Wiktionary's English-language extractions as JSONL.
  Attribution required; redistributable.
- **Format**: Per-headword JSONL with `senses[].glosses[]` (English),
  `forms[].form/tags`, `pos`, `word`, `lang_code`. ~250 MB compressed for FI;
  ~5 MB for ET (English Wiktionary has limited Estonian coverage).
- **Status**: Imported. FI gives 100% gloss coverage on 259k lemmas. ET gives
  ~13k entries — mostly characters, inflected-form pages, and a thin headword
  list. Drives `lemmas.gloss` and `translations` (target_lang=EN).
- **Importer**: `cmd/importdict` (`make import-kaikki-fi` / `import-kaikki-et`).
- **Source priority**: 10. Lower than ekilex (20) for ET so EKI wins when both
  cover the same headword.

### Ekilex public_word (full headword list)

- **URL**: <https://ekilex.ee/api/public_word/eki>.
- **License**: Estonian Language Institute (EKI) public Ekilex export.
  Permissive — redistribution allowed with attribution. The JSON public_word
  endpoint is a thin headword list without senses; the underlying EKI sources
  carry their own licenses (EKSS = CC BY-SA 4.0, ÕS = CC BY-SA 4.0, IATE =
  EUPL).
- **Format**: JSON per headword, plain `word` + `homonym_nr` only.
- **Status**: Imported once (174k rows). Adds POS=X stub rows for headwords
  missing from the reduced bulk drop. Mostly redundant once
  `cmd/importekilexdetails` runs against the bulk reduction.
- **Importer**: `cmd/importekilex` (`make import-ekilex-public-words`).

### Ekilex bulk reduction (`reduceekilex` → `definitions/`, `forms/`)

- **URL**: <https://ekilex.ee/dataset/eki> (bulk dump).
- **License**: Same as Ekilex public_word — CC BY-SA derivatives apply per
  underlying dataset. Stays local-only by default; redistributable with
  attribution per EKI terms.
- **Format**: After running `cmd/reduceekilex`, the data lands as
  `definitions/<letter>.jsonl` (per-headword: lemma, homonym_nr, word_class,
  meanings → `pos`, `definitions_et`, `translations_en`, `usages_et`) and
  `forms/<letter>.tsv` (lemma, form, morph_code).
- **Status**: Imported. ET ekilex gives 178k lemmas, of which 67k have an EN
  translation (current source for `lemmas.gloss`). The remaining 110k have
  Estonian-language definitions (`definitions_et`) that the importer **drops
  on the floor today**. This is the bucket this PR rescues.
- **Importer**: `cmd/importekilexdetails` (`make import-ekilex-details-et`).
- **Source priority**: 20.

### Kotus Nykysuomen sanalista

- **URL**: <https://kaino.kotus.fi/sanat/nykysuomi/>.
- **License**: CC BY 4.0.
- **Format**: XML headword list with Kotus paradigm classes.
- **Status**: Imported. Fills `lemmas.paradigm_class` for FI. Carries no gloss
  data — paradigm-only.
- **Importer**: `cmd/importkotus` (`make import-kotus-paradigms`).

### Considered but not adopted

- **EKI Sõnaveeb scrape** (<https://sonaveeb.ee>): the user-facing aggregator
  surfaces additional EKI dictionaries (ÕS 2018, EKSS, IATE, etc.). The
  underlying datasets are accessible through Ekilex bulk; scraping the website
  duplicates work and may violate EKI's terms. Skip — get the same data
  through Ekilex.
- **fi.wiktionary.org dump** (Finnish-language definitions for Finnish
  headwords): would give Finnish-language glosses, not English. Useful for
  native-speaker lookups but not for the learner-facing wordlist whose audience
  is English readers. Track as a future addition, not in scope for the
  user-friendly wordlist.
- **Linguee / Glosbe / Reverso**: aggregators with strict terms forbidding
  redistribution. Skip.
- **OPUS parallel corpora**: provide translation pairs at sentence level, not
  headword glosses. Already used in the corpus pipeline for example sentences
  and frequency, not for meanings.

## Strategy: Combine sources, don't pick one

The wordlist exporter joins on `(lemma, pos, lang)` against `lemmas.gloss`. The
gloss is denormalized — one string per `(lemma, pos)` — so the importer is
where source-combination happens. The chain runs in source-priority order, so
each later import refines earlier ones without erasing them:

1. `cmd/importdict -lang fi -source-key kaikki` — FI Wiktionary glosses
   (priority 10). Saturates FI.
2. `cmd/importdict -lang et -source-key kaikki` — ET Wiktionary glosses
   (priority 10). Thin coverage, but fills the few entries that are in EN
   Wiktionary but not Ekilex.
3. `cmd/importekilex` — ET headword stubs (priority 20). Adds POS=X rows for
   headwords the bulk drop misses.
4. `cmd/importekilexdetails` — ET Ekilex bulk (priority 20). The current path
   joins EN translations into `lemmas.gloss`. **This PR extends it** to also
   write Estonian-language definitions into the `definitions` table (which is
   currently empty in production), and to fall back to the first
   `definitions_et` entry as a `[ET]`-prefixed gloss when no EN translation
   exists for a `(lemma, pos)`.
5. `cmd/importkotus` — FI paradigm classes (independent of gloss).

After this PR, the `(lemma, pos)` rows that previously had no gloss because
ekilex provided only `definitions_et` will gain a fallback gloss prefixed with
`[ET]`. The prefix marks the gloss as Estonian-language so the wordlist export
(item 2) can render it differently — a cue to the learner that they're reading
a definition in the source language, not an English translation.

## Out of scope (track for future)

- **English-language gloss generation for ET headwords missing EN
  translations**: an LLM pass over `definitions_et` could yield English glosses
  for the 4.6% bucket and improve over the `[ET]`-prefixed fallback. Defer
  until the user-friendly wordlist ships and we have telemetry on which
  headwords learners actually hit.
- **Filling the 21% "not in dict" bucket**: those headwords are absent because
  they are compounds, proper nouns, neologisms, OOV, or spurious FST
  analyses. A productive split would be (a) compound decomposition for FI
  using existing Voikko / Omorfi compound boundaries, (b) named-entity
  detection to filter PROPN before gloss-lookup, (c) accepting that some FST
  analyses will never lemmatize to a real headword. None of this is
  blocked by the wordlist export and can ship later.
- **Examples / usage data**: `usages_et` is already captured per-meaning in the
  Ekilex JSONL but not yet imported. The user-friendly export currently joins
  to `sentences.tsv` for examples; pulling Ekilex's curated example
  sentences would improve quality for headwords with few corpus hits. Track
  as a follow-up to item 2.
