# Gloss Coverage Audit — 2026-05-09

This report measures lexical-source gloss coverage for the corpus wordlists
under `localdata/{fi,et}-corpus/_derived/`. The before/after split brackets
the meaning-sources changes that landed with PR 1 of the corpus-pipeline
roadmap (`corpus_pipeline/docs/PR_ROADMAP.md`).

## Methodology

`cmd/glosscoverage` reads `localdata/{lang}-corpus/_derived/wordlist.tsv` and
left-joins the parser-choice rows against the dictionary DB's `lemmas` table.
Two metrics:

- **Pair coverage** — distinct `(lemma, pos)` tuples in the wordlist whose
  `lemmas.gloss` is non-empty.
- **Token coverage** — same join, weighted by each row's
  `surface_count_total`. The user-experience number, since a wordlist row's
  frequency in the corpus is what determines how often a learner actually
  reads the gloss.

Three buckets per metric:

- *with gloss* — corpus pair joined to a dict row whose gloss is non-empty.
- *in dict, no gloss* — corpus pair has a matching dict row but its gloss is
  empty. The fillable bucket — meaning data exists somewhere; the import path
  is the bottleneck.
- *not in dict* — no matching dict row. Compounds, proper nouns, neologisms,
  and FST-generated analyses for surfaces no dictionary lists.

Reproduce:

```sh
cd corpus_pipeline
go run ./cmd/glosscoverage -lang fi -out reports/2026-05-09-coverage-fi.json
go run ./cmd/glosscoverage -lang et -out reports/2026-05-09-coverage-et.json
```

## FI Coverage

| Metric                       | Pairs                | Tokens (occurrences)        |
|------------------------------|----------------------|-----------------------------|
| Total                        | 5,057,506            | 267,130,363                 |
| With gloss                   | 154,574 (3.06%)      | 210,085,306 (78.65%)        |
| In dict, no gloss (fillable) | 0 (0.00%)            | 0 (0.00%)                   |
| Not in dict                  | 4,902,932 (96.94%)   | 57,045,057 (21.35%)         |

`kaikki` source: 259,145 lemmas / 100.00% coverage.

FI is gloss-saturated for the entries the dictionary already lists. The 21.35%
token gap is entirely "lemma not in dict" — compounds, proper nouns, and
spurious FST analyses. No existing source fixes that bucket; deferred to a
future PR (see `corpus_pipeline/docs/MEANING_SOURCES.md`).

The audit produced no changes for FI between the before and after run.

## ET Coverage — Before

| Metric                       | Pairs                | Tokens (occurrences)        |
|------------------------------|----------------------|-----------------------------|
| Total                        | 2,328,080            | 96,090,526                  |
| With gloss                   | 51,186 (2.20%)       | 71,054,502 (73.95%)         |
| In dict, no gloss (fillable) | 62,362 (2.68%)       | 4,450,819 (4.63%)           |
| Not in dict                  | 2,214,532 (95.12%)   | 20,585,205 (21.42%)         |

By source:

| Source | Lemma rows | With gloss | Coverage |
|--------|-----------:|-----------:|---------:|
| ekilex | 178,032    | 67,367     | 37.84%   |
| kaikki | 176,199    |  6,207     |  3.52%   |

The ekilex bulk import was joining only English translations into
`lemmas.gloss`. Where Ekilex carried Estonian-language definitions but no EN
translations, the import dropped them. That is the 4.63% token bucket.

## ET Coverage — After

After re-running `cmd/importekilexdetails` against the same Ekilex source data
with the new ET-definitions fallback path:

| Metric                       | Pairs                | Tokens (occurrences)        |
|------------------------------|----------------------|-----------------------------|
| Total                        | 2,328,080            | 96,090,526                  |
| With gloss                   | 105,507 (4.53%)      | 75,292,988 (78.36%)         |
| In dict, no gloss (fillable) | 8,041 (0.35%)        | 212,333 (0.22%)             |
| Not in dict                  | 2,214,532 (95.12%)   | 20,585,205 (21.42%)         |

By source:

| Source | Lemma rows | With gloss | Coverage |
|--------|-----------:|-----------:|---------:|
| ekilex | 178,032    | 158,683    | 89.13%   |
| kaikki | 176,199    |   6,207    |  3.52%   |

## What Changed

- Ekilex source coverage rose from 37.84% to 89.13% — 91,316 lemma rows that
  previously had empty `gloss` got filled with `[ET] `-prefixed Estonian
  definitions.
- ET token coverage rose from 73.95% to 78.36% (+4.41 percentage points).
- The fillable in-dict-no-gloss bucket collapsed from 4.63% to 0.22% — only
  ~212k tokens remain in the bucket, all of them entries where Ekilex carries
  neither EN translations nor ET definitions (mostly POS=X invariables and
  abbreviations).
- The `definitions` table, previously empty, now holds 326,349 sense-level
  Estonian-language definition rows. Available for downstream consumers
  (the user-friendly wordlist export, future glossary tools).

The "not in dict" bucket (21.42% of ET tokens, 95.12% of pairs) is unchanged —
that gap requires new sources or compound decomposition, both out of scope
for this PR.

## Reproducing the After Run

```sh
go run ./cmd/importekilexdetails \
    -db finnestdb.db \
    -data localdata/ekilex
```

Idempotent: running the same command again produces byte-identical output. The
Ekilex source files under `localdata/ekilex/definitions/` are the input.

## Source Audit

| Source              | License                              | Status   | Importer                       |
|---------------------|--------------------------------------|----------|--------------------------------|
| kaikki.org FI       | CC BY-SA 4.0 (Wiktionary)            | Imported | `cmd/importdict -lang fi`      |
| kaikki.org ET       | CC BY-SA 4.0 (Wiktionary)            | Imported | `cmd/importdict -lang et`      |
| Ekilex public_word  | CC BY-SA 4.0 (EKI public)            | Imported | `cmd/importekilex`             |
| Ekilex bulk EKI     | CC BY-SA 4.0 (EKI bulk; verify per dataset) | Imported | `cmd/importekilexdetails`      |
| Kotus paradigms     | CC BY 4.0                            | Imported | `cmd/importkotus`              |

See `corpus_pipeline/docs/MEANING_SOURCES.md` for the full source-by-source
strategy, sources that were considered and rejected (Linguee, Glosbe,
sonaveeb scrape), and the deferred work (compound decomposition, named-entity
filtering, LLM-pass on `definitions_et` for English glossing).
