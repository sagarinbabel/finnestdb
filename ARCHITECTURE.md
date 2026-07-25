# FinnEst Architecture

_Current as of 2026-05-09 - see [docs/CHANGELOG.md](docs/CHANGELOG.md) for revisions._

Role-aware Finnish and Estonian reading app focused on dictionary-backed
lemmatization, deck creation, review, parser feedback, and parser evaluation.

Subsystem behavior versions are tracked separately in
[`docs/SYSTEM_VERSIONING.md`](docs/SYSTEM_VERSIONING.md). Parser behavior,
parser baselines, and deck review scheduling should not share a single version
number because they change independently.

## What Exists Today

Current product surface on `main`:

- public landing/about/sign-in routes
- real password-based auth: Argon2id hashing + DB-backed sliding sessions in
  `internal/auth`, plus `/admin/users` administration page
- authenticated dashboard, Inspect, Decks, Review, and Results routes
- admin-only parser workbench and feedback routes
- Inspect page for Finnish or Estonian text
- parser selection in the admin workbench: `basic` and `custom`
- file load support for `.txt` / `.md` / `.epub`
- hybrid language policy: high-confidence paste/file ingest warns and blocks
  parsing until the learner switches the selector; unknown-language warnings
  remain advisory
- results page with sortable output, POS filter chips, coverage gauge, and parse duration
- structured parse-stage stats returned from `parsecore` and `/api/parse`
- deck, known-word, review, and parse-feedback APIs
- parser evaluation CLI for `basic`, `custom`, `omorfi`, and `estnltk`
- dataset-driven evaluation workflow in `internal/eval`
- expanded Finnish and Estonian gold datasets under `testdata/parser-eval/*/gold`
- external-adapter slots in `internal/parsecore` for the Omorfi (FI) and
  EstNLTK/Vabamorf (ET) baselines, with per-parser configurable subprocess
  timeouts
- multi-source dictionary tables: row-level `source` + `source_priority` on
  `lemmas` and `forms`; conflict resolution favors higher priority
- lexical-enrichment schema groundwork on `lemmas`/`forms`: `paradigm_class`
  (FI Kotus class join key), `feats` (UD-style morph features as JSON),
  plus dedicated `translations` and `definitions` tables
- Estonian source-data pipeline end-to-end: `cmd/fetchekilex` resumable
  scraper → `cmd/reduceekilex` golden-tested reducer → gitignored CC BY 4.0
  artifacts under `localdata/ekilex/` (~177k headwords) → loaded into the
  dictionary tables by `cmd/importekilexdetails` (~178k lemmas, ~6.2M form
  rows, ~15s wall time)
- multi-lemma `forms` PK `(form, lang, lemma, pos)`: a single ambiguous
  surface form (e.g. ET `joon` = noun "line" + 1Sg of `jooma`) maps to
  multiple `(lemma, pos)` candidates; deck ingest emits one card per
  candidate
- tracked corpus pipeline under `corpus_pipeline/` for fetching,
  extracting, aggregating, verifying, promoting, enriching, and per-book
  EPUB deck generation; runtime corpus data remains gitignored under
  `localdata/{fi,et}-corpus/`
- learner-facing corpus exports:
  `wordlist_user_friendly.tsv`, `sentences_user_friendly.tsv`,
  `sentence_occurrences.tsv`, parser-improvement mining files, and
  gloss-coverage reports
- Playwright coverage for parse/results, POS filtering, language switching, file upload, and mobile nav

Important distinction:

- `basic` and `custom` are admin workbench parser modes in the browser UI
- `omorfi` and `estnltk` exist today as evaluation/parser-core integration
  points, but not as browser buttons
- The Finnish lexical pipeline (Kotus + generated morphology tables +
  kaikki.org) is documented in [`docs/LEXICAL_PLAN.md`](docs/LEXICAL_PLAN.md).
  `cmd/importkotus` is present; FI/ET generated morphology table
  generators exist, but production tables are local runtime artifacts
  under `localdata/lemmatizer-fi-et/tables/`, not committed files.
  Smoke fixtures under `testdata/lemmatizer/` are only unit-test ground
  truth.

## High-Level Architecture

```mermaid
flowchart TB
  subgraph Client["Browser UI"]
    Inspect["Inspect / parser workbench"]
    Decks["Decks"]
    Review["Review session"]
    Results["Results and parser feedback"]
  end

  subgraph API["Go HTTP API"]
    Handlers["internal/api handlers"]
    Auth["password auth + sliding sessions (internal/auth)"]
  end

  subgraph ParserSystem["Parser system versioned independently"]
    ParseCore["parsecore analyzer registry"]
    ParserModes["parser modes: basic, custom, omorfi, estnltk"]
    RustParser["Rust tokenizer / sentence splitter"]
    Enrichment["dictionary and morphology enrichment"]
    ParserEval["parser evaluation CLI"]
    GoldData["gold datasets and frozen baselines"]
  end

  subgraph DeckReviewSystem["Deck and review system versioned independently"]
    DeckService["deck APIs"]
    Occurrences["sentence and token occurrences"]
    Cards["cards and card state"]
    Scheduler["review queue / scheduling policy"]
  end

  subgraph Data["Persistence and source data"]
    Store["SQLite store"]
    Dict["multi-source dictionary import (kaikki, Ekilex, custom)"]
    Feedback["parse feedback"]
    LocalData["localdata: DB inputs, FST tables, corpus artifacts"]
  end

  subgraph CorpusPipeline["Corpus pipeline (tracked code, localdata data)"]
    Fetch["fetchcorpus / scrapers"]
    Extract["extractcorpus"]
    Aggregate["aggregatecorpus"]
    Verify["corpusverify / corpuspromote"]
    EnrichCorpus["enrichcorpus"]
    EpubDeck["epubdeck"]
    CorpusExports["wordlists, sentence banks, mining TSVs, reports"]
  end

  Inspect -->|"POST /api/parse"| Handlers
  Results -->|"corrections"| Handlers
  Decks -->|"deck CRUD"| Handlers
  Review -->|"review answers"| Handlers
  Auth --> Handlers

  Handlers --> ParseCore
  ParseCore --> ParserModes
  ParserModes --> RustParser
  ParserModes --> Enrichment
  Enrichment --> Store
  LocalData --> Enrichment
  LocalData --> Dict
  Dict --> Store
  ParserEval --> ParseCore
  ParserEval --> GoldData

  Handlers --> DeckService
  DeckService --> Occurrences
  DeckService --> Cards
  Cards --> Scheduler
  DeckService --> Store
  Scheduler --> Store
  Handlers --> Feedback
  Feedback --> Store

  Fetch --> LocalData
  LocalData --> Extract
  Extract --> Aggregate
  Aggregate --> ParseCore
  Aggregate --> Store
  Aggregate --> CorpusExports
  EnrichCorpus --> ParserModes
  EnrichCorpus --> CorpusExports
  EpubDeck --> Aggregate
  Verify --> CorpusExports
  CorpusExports --> LocalData

  classDef versioned fill:#eef6ff,stroke:#2b6cb0,color:#102a43
  classDef data fill:#f7f7f7,stroke:#666,color:#222
  class ParserSystem,DeckReviewSystem,CorpusPipeline versioned
  class Data data
```

## Responsibilities By Layer

### 1. Browser UI

Files:

- `web/index.html`
- `web/app.ts`
- `web/app.js`
- `web/styles.css`

Responsibilities:

- render the role-aware app shell and mobile menu
- collect text and selected language
- expose learner flows for Inspect, Decks, Review, Results, and known words
- expose admin workbench controls for `basic` and `custom`
- submit parse requests to `/api/parse`
- render parser metadata, coverage gauge, POS filters, and sortable results
- explain coverage score and timing

### 2. API Layer

Files:

- `cmd/server/main.go`
- `internal/api/handlers.go`
- `internal/auth/` - password hashing, session creation/validation,
  registration, admin user management

Responsibilities:

- start the HTTP server with `Cache-Control: no-store` on static assets
- initialize SQLite-backed store
- expose parse, deck, review, known-word, import, and parse-feedback endpoints
- expose real password-based auth (`/api/auth/register`, login, logout)
  with Argon2id hashing and DB-backed sliding sessions (7-day expiry)
- pass parse work into `internal/parsecore`

### 3. Parse Core

Files:

- `internal/parsecore/parsecore.go`

Responsibilities:

- define parser result types
- maintain parser registry
- unify parser behavior behind one API
- enrich parser output with dictionary/gloss data
- support both browser parsing and CLI evaluation

### 4. Rust Parser

Files:

- `parser/src/lib.rs`
- `internal/parserffi/bindings.go`

Responsibilities:

- normalization
- sentence splitting
- tokenization
- heuristic POS guessing
- JSON/FFI bridge into Go

### 5. Dictionary / Persistence

Files:

- `internal/store/db.go` - schema + `EnsureXxx` migration helpers
- `internal/store/dict.go` - `BatchLookupForms`, `BatchLookupAllForms`
  (multi-lemma), `BatchLookupGlosses`, Finnish possessive stripping
- `cmd/importdict/` - kaikki.org JSONL or Ekilex API → SQLite (chooses
  shape via `-source-key`)
- `cmd/importekilex/` - compact Ekilex public-headword snapshot loader
- `cmd/importekilexdetails/` - loads the reduced Ekilex data drop
  (`localdata/ekilex/{definitions,forms}/`) into the dictionary tables;
  ~178k lemmas + ~6.2M form rows
- `cmd/importkotus/` - Kotus sanalista TSV → SQLite, populates `paradigm_class`
- `cmd/fetchekilex/` - resumable Ekilex `/api/word/details` scraper
- `cmd/reduceekilex/` - reduces raw payloads into sharded JSONL + TSV
  artifacts, with golden tests covering all 41 Estonian inflection classes
- `cmd/genlemmatizertables/` - generates FI/ET lemmatizer JSON tables
  under `localdata/lemmatizer-fi-et/tables/` from local analyser files
  (`mor.vfst` for FI, `.hfstol` for ET; no analyser blob committed)
- `cmd/fetchfrequency/` - downloads public FI/ET frequency baselines
  (OpenSubtitles + UD treebanks) into `localdata/frequency/` for
  comparison against user-aggregated frequency

Responsibilities:

- import dictionary data from multiple sources (kaikki.org, Ekilex,
  custom CSV overrides) and store it with row-level `source` and
  `source_priority` for deterministic conflict resolution
- preserve per-source attribution metadata (`source_name`, `source_url`,
  `source_version`, `license`, `attribution`, `imported_at`,
  `changes_note`) in `dict_metadata`
- resolve forms to lemma/POS (multi-lemma aware via the
  `(form, lang, lemma, pos)` PK), applying Finnish possessive stripping
  and language-specific case-suffix fallbacks
- supply glosses from `lemmas.gloss`, `translations`, and `definitions`
- maintain language-specific lexical-enrichment columns
  (`lemmas.paradigm_class`, `forms.feats`) and the new `translations` /
  `definitions` tables
- store deck/sentence/occurrence/review data; deck ingest expands a
  single ambiguous surface form into one occurrence/card per dict
  `(lemma, pos)` candidate
- own user authentication state via `users` and `user_sessions`

#### Multi-lemma forms

The `forms` table uses `PRIMARY KEY (form, lang, lemma, pos)` so a single
surface form can map to multiple `(lemma, pos)` candidates. This models
homonyms - e.g. ET `joon` is both the noun "line" (`SgN` of `joon`) and
the 1st-person-singular form of the verb `jooma` ("to drink"). At deck
ingest time, every dict candidate becomes its own `occurrence` row and
its own card; the parser's single pick is only used when the dict has
no entries for the form.

The corresponding `occurrence` UNIQUE is
`(deck_id, sentence_id, token_ix, lemma, pos)`. Migration from the legacy
single-lemma schema is handled by `EnsureMultiLemmaSchema` in
`internal/store/db.go`.

#### POS mapping (Ekilex → UPOS)

`cmd/importekilexdetails` translates Ekilex meaning-level POS codes (and
falls back to entry-level `word_class` when the meaning has no POS) into
Universal POS codes the parser pipeline emits. The mapping:

| Ekilex `meaning.pos` | UPOS | Notes |
|---|---|---|
| `s` | `NOUN` | substantive |
| `v`, `vrm` | `VERB` | `vrm` is rare |
| `adj`, `adjg`, `adjid` | `ADJ` | |
| `adv` | `ADV` | |
| `prop`, `propgen` | `PROPN` | proper noun |
| `pron` | `PRON` | |
| `num` | `NUM` | |
| `postp`, `prep` | `ADP` | |
| `konj` | `CCONJ` | no C/S split visible in data |
| `interj` | `INTJ` | |

Fallback when `meaning.pos` is empty (uses entry-level `word_class`):

| `word_class` | UPOS |
|---|---|
| `noomen` | `NOUN` |
| `verb` | `VERB` |
| `muutumatu` | `X` |

Forms in `forms/<letter>.tsv` carry only `(lemma, form, morph_code)` -
they don't say which homonym a form belongs to. When a lemma has multiple
homonyms with different POS (e.g. `jooma` is both VERB and NOUN), the
importer disambiguates by classifying the morph code: codes prefixed
`Ind/Imp/Knd/Kvt/Sup/Pts/Inf/Ger/Neg` are verbal and only attribute to
VERB; codes prefixed `Sg/Pl` (and the invariant marker `ID`) are nominal
and only attribute to non-VERB POSes.

### 6. Corpus Pipeline

Files:

- `corpus_pipeline/cmd/fetchcorpus/` - source registry downloads into
  `localdata/{fi,et}-corpus/<source>/raw/`
- `corpus_pipeline/cmd/extractcorpus/` - format-specific text extraction
  for EPUB, VRT, Leipzig, CSV, HTML, Hugging Face, Markdown, SKVR, gzip,
  fixtures, and miscellaneous text sources
- `corpus_pipeline/cmd/aggregatecorpus/` - deterministic aggregation into
  canonical and learner-facing TSV exports
- `corpus_pipeline/cmd/corpusverify/` and
  `corpus_pipeline/cmd/corpuspromote/` - smoke/pilot/full QA gates and
  promotion state
- `corpus_pipeline/cmd/enrichcorpus/` - analyzer-agreement enrichment for
  silver parser-improvement candidates
- `corpus_pipeline/cmd/epubdeck/` - per-book wordlist/deck export
- `corpus_pipeline/cmd/glosscoverage/` - dictionary gloss-coverage audit
- `corpus_pipeline/docs/CORPUS_PIPELINE.md`

Responsibilities:

- keep corpus implementation tracked while keeping bulk corpus data out of git
- fetch and extract source material into `localdata/{fi,et}-corpus/`
- aggregate wordlists, sentence banks, sentence occurrences, poems,
  document metadata, QA metadata, and mining TSVs
- produce learner-facing exports with meanings, parsed morphology, source
  counts, analysis provenance, parser/FST fingerprints, and example refs
- verify hard/soft QA gates before promotion between smoke, pilot, and full
- generate silver candidates only from `enrichcorpus` analyzer agreement
- preserve append-only repair/error history under `_derived/errors/`

The corpus pipeline is not a runtime dependency of the browser app today.
It is the offline path for better learner material, better frequency lists,
and parser-improvement candidates. The app can consume promoted artifacts
later without changing the parser/eval contracts.

### 7. Evaluation Stack

Files:

- `cmd/parsertest/main.go` - runs gold datasets across selected parsers
- `cmd/parser-compare/main.go` - assembles markdown comparison tables
  from one or more `cmd/parsertest` reports
- `cmd/importud/main.go` - converts Universal Dependencies CoNLL-U files
  into our parser-eval gold JSON; drives Plan C / PR 1 corpus expansion
- `cmd/corpusmine/main.go` - mines cleaned corpus text for
  disagreement-heavy gold candidates
- `cmd/autoresearch/main.go` - parked post-live idea for automated
  rule-ablation loops; not active alpha scope
- `scripts/fetch-and-import-ud.sh` - clone UD treebanks and run importud
- `scripts/parser-comparison.sh` · `scripts/parser-comparison-et.sh` -
  always include the analyzer baseline (omorfi/estnltk); fail fast when
  missing
- `cmd/scrapegutenberg/main.go` - Plan C / PR 3 silver-corpus scraper
  for Project Gutenberg Finnish books
- `internal/eval/eval.go`
- `testdata/parser-eval/{fi,et}/gold/` - committed gold (FI UD CC BY,
  manual sets, fi-grammar-v1)
- `localdata/parser-eval/{fi,et}/{gold,gold-train}/` - gitignored
  parser-eval gold for sources we can't redistribute (NC-licensed ET UD
  dev/test) and for splits we just don't want auto-discovered (FI/ET
  UD train splits, used for OOV/coverage only). All under localdata/
  per the single-folder bootstrap rule.
- `localdata/silver-fi/` - legacy Plan C silver-tier corpus
  (Gutenberg-FI raw text + JSONL manifest)
- `localdata/{fi,et}-corpus/_derived/mining/` - corpus-pipeline mining outputs such
  as unresolved, ambiguous, parser-disagreement, internal-consensus, and
  silver-candidate TSVs
- `docs/baselines/` - frozen baseline reports per parser/language
- `docs/PARSER_EVAL_DATASETS.md`
- `docs/OMORFI_ADAPTER.md` · `docs/OMORFI_COMPARISON.md`
- `docs/AUTORESEARCH.md`

Responsibilities:

- compare parser outputs on labeled datasets
- mine cleaned corpus text for disagreement-heavy candidate sentences
- record quality, observability, and performance metrics
- detect regressions and priority failures
- provide the workflow for judging parser improvements

Held-out discipline (introduced 2026-05-06c, Plan C / PR 1):

- Every committed comparison report must include the analyzer baseline
  column (omorfi for FI, estnltk for ET); see "Eval harness parity"
  in `docs/CHANGELOG.md`
- Default discovery in the comparison scripts excludes `*-dev-*` files
  (used for per-commit watching, not headline eval) and `gold-train/`
  files (used for OOV / coverage analysis only)
- UD test sets are the held-out anchor; manual / fi-grammar-v1 / etc.
  are curated adversarial sets retained alongside

UD-derived gold (Plan C / PR 1, paths consolidated 2026-05-07):

Committed FI test/dev gold lives under `testdata/parser-eval/fi/gold/`.
Everything else (FI/ET train splits, NC-licensed ET UD dev/test) lives
under `localdata/parser-eval/{fi,et}/{gold,gold-train}/` so the entire
gitignored bootstrap state fits in a single zip of `localdata/`.

| Treebank      | License        | Test cases | Dev cases | Train cases | Gold JSON                                                                       |
|---------------|----------------|-----------:|----------:|------------:|---------------------------------------------------------------------------------|
| Finnish-TDT   | CC BY-SA 4.0   |     1,554  |    1,358  |     12,204  | committed: `testdata/parser-eval/fi/gold/ud-fi-tdt-{test,dev}-v1.json.gz`; train at `localdata/parser-eval/fi/gold-train/ud-fi-tdt-train-v1.json` |
| Finnish-FTB   | CC BY 4.0      |     1,867  |    1,875  |     14,972  | committed: `testdata/parser-eval/fi/gold/ud-fi-ftb-{test,dev}-v1.json.gz`; train at `localdata/parser-eval/fi/gold-train/ud-fi-ftb-train-v1.json` |
| Finnish-PUD   | CC BY-SA 3.0   |     1,000  |        - |          - | committed: `testdata/parser-eval/fi/gold/ud-fi-pud-test-v1.json.gz`               |
| Finnish-OOD   | CC BY-SA 4.0   |     2,106  |        - |          - | committed: `testdata/parser-eval/fi/gold/ud-fi-ood-test-v1.json.gz`               |
| Estonian-EDT  | CC BY-NC-SA    |     3,190  |    3,110  |     24,419  | local-only: `localdata/parser-eval/et/gold/ud-et-edt-{test,dev}-v1.json` + `localdata/parser-eval/et/gold-train/ud-et-edt-train-v1.json` |
| Estonian-EWT  | CC BY-NC-SA    |       910  |      823  |      5,375  | local-only: `localdata/parser-eval/et/gold/ud-et-ewt-{test,dev}-v1.json` + `localdata/parser-eval/et/gold-train/ud-et-ewt-train-v1.json` |

About 776k gold tokens locally (FI committed test/dev + everything
else under localdata/). Run `make import-ud-gold` after a fresh
checkout to materialize the local files. See
[`docs/data_enhancement.md`](docs/data_enhancement.md) for the
single-source-of-truth ledger of every gold/silver corpus.

## Lexical Pipelines

The two languages stitch together different source mixes behind the same
multi-source schema. See [`docs/LEXICAL_PLAN.md`](docs/LEXICAL_PLAN.md)
for the combined FI + ET lexical-layer architecture (Estonian-specific
content lives there now; the legacy `docs/ESTONIAN_LEXICAL_PLAN.md` was
deleted in PR #135).

Estonian (live):

- analyzer baseline: EstNLTK / Vabamorf via the `estnltk` adapter
- lexical sources: kaikki.org (priority 10) + EKI/Ekilex (priority 20)
- Ekilex bulk path: `cmd/fetchekilex` → `cmd/reduceekilex` → gitignored
  sharded artifacts in `localdata/ekilex/` → loaded by
  `cmd/importekilexdetails` (multi-lemma aware, with morph-class
  homonym disambiguation and `(lemma, pos)` gloss merging)
- Ekilex API path (smaller queries, on-demand): `cmd/importdict
  -source-key ekilex` against the `/api/word/details` endpoint
- generated morphology tables: `make gen-lemmatizer-tables-et
  HFSTOL_PATH=/path/to/analyser-gt-desc.hfstol` writes
  `localdata/lemmatizer-fi-et/tables/et_min.json`; production promotion
  still needs a production wordlist, provenance notes, row counts, and
  eval gate

Finnish (live):

- analyzer baseline: Omorfi adapter slot
- lexical sources: kaikki.org (priority 10) + Kotus sanalista (priority 10,
  fills `paradigm_class`)
- generated morphology tables: `pkg/lemmatizer-fi-et/` reads JSON tables
  from `localdata/lemmatizer-fi-et/tables/` (gitignored per
  `docs/ARTIFACT_POLICY.md`); production tables generated locally by
  `make gen-lemmatizer-tables-fi VFST_PATH=/path/to/mor.vfst` from a
  user-installed libvoikko. Smoke-fixture tables ship under
  `testdata/lemmatizer/` for tests; runtime falls back to dict-only when
  production tables are absent.
- post-#127/#129/#130 the FST is a parallel scorer in dict step 1 with
  candidate-merge FEATS enrichment, no longer a step-5 fallback.

## Parser Modes and Baselines

### Browser-exposed modes

- `basic`
  - Rust parse
  - direct dictionary lookup only
  - unresolved tokens fall back to stub lemma/POS

- `custom`
  - Rust parse
  - dictionary lookup with parallel-FST candidate scoring (post-#127)
    and FST candidate-merge FEATS enrichment (post-#129)
  - Finnish possessive stripping
  - Finnish/Estonian compound splitting
  - Finnish/Estonian case-suffix fallback rules
  - case-suffix grammar-label stopgap on dict hits (`attachCaseLabelIfStemMatches`,
    transitional until production FST tables emit FEATS for direct hits)

### Evaluation-only baseline path

- `omorfi`
  - Finnish only
  - configured through `FINNESTDB_OMORFI_CMD`
  - external adapter slot, not bundled runtime
  - useful for comparison against `basic` and `custom`

- `estnltk`
  - Estonian only
  - configured through `FINNESTDB_ESTNLTK_CMD` (or autodiscovered from
    `scripts/estnltk_adapter_example.py`)
  - subprocess timeout overridable via `FINNESTDB_ESTNLTK_TIMEOUT`
  - same JSON shape as the Rust FFI parser; comparison-only, not a
    browser button

## Data Flow

### User-facing parse flow

1. User submits Finnish or Estonian text in the browser.
2. Frontend calls `POST /api/parse`.
3. API handler calls `parsecore.Analyze(...)`.
4. `parsecore` picks the requested parser.
5. Rust or external analyzer returns token-level analysis.
6. `parsecore` resolves forms via dictionary/enrichment logic.
7. `parsecore` aggregates words, glosses, grammar labels, and examples.
8. API returns parse JSON and parse-stage stats to the browser.
9. Frontend renders summary stats, coverage gauge, POS-filtered rows, and sortable output.

### Deck and review flow

1. A learner saves inspected text into a deck or imports deck material.
2. API handlers persist decks, sentences, token occurrences, cards, and
   card state in SQLite.
3. Multi-lemma dictionary candidates produce distinct occurrence/card rows
   keyed by `(deck_id, sentence_id, token_ix, lemma, pos)`.
4. Review endpoints select due cards, accept learner answers, and update
   review state separately from parser behavior.
5. Known-word endpoints track learner vocabulary state without changing
   parser baselines.

### Evaluation flow

1. Developer runs `go run ./cmd/parsertest ...`.
2. Dataset JSON is loaded and validated.
3. Each parser is run across each case with warmup and timed repeats.
4. Output is compared against expected token annotations.
5. Accuracy, coverage, and parse-stage observability metrics are written to a report JSON file.

### Corpus pipeline flow

1. Operators run `cd corpus_pipeline && make fetch-corpus[-fi|-et]` or place
   folder-driven material under `localdata/{fi,et}-corpus/<source>/raw/`.
2. `extractcorpus` normalizes formats into `text.txt`, `poems.jsonl`, and
   `documents.jsonl`.
3. `aggregatecorpus` builds canonical exports and learner-facing exports,
   using parser/FST/dictionary evidence for analysis and provenance.
4. `corpusverify` checks hard and soft gates; `corpuspromote` walks
   smoke -> pilot -> full and records promotion state.
5. `enrichcorpus`, `epubdeck`, and `glosscoverage` add silver candidates,
   per-book learner wordlists, and meaning-source coverage reports.

## Current Boundaries and Caveats

- The browser UI is still centered on `basic` and `custom`.
- `omorfi` and `estnltk` are not browser choices; they are eval and corpus
  enrichment baselines.
- Auth is real (Argon2id + DB-backed sessions). Public exposure still needs
  the remaining go-live hardening called out in
  [`docs/GO_LIVE_CHECKLIST.md`](docs/GO_LIVE_CHECKLIST.md).
- Omorfi and EstNLTK are not bundled into normal app startup; they are
  external adapter paths used by evaluation and offline enrichment.
- The evaluation pipeline and corpus pipeline are first-class
  architectural components, but corpus bulk data and generated analyzer
  tables remain local artifacts under `localdata/`.
- The Finnish lexical pipeline columns are populated:
  `paradigm_class` is set on FI lemmas by `cmd/importkotus` (Phase 3),
  `forms.feats` is populated by `cmd/importdict` (FI/ET kaikki, via
  `kaikkiTagsToFeats` since 2026-05-07k / PR #139),
  by `cmd/importekilexdetails` (ET, via Ekilex morph_code), and by FST
  candidate merge in `BatchLookupForms` (post-#129).
  `translations` and `definitions` are populated by `cmd/importdict`
  (kaikki) and `cmd/importekilexdetails` (Ekilex) per Phase 2.
  The case-suffix-strip fallback path projects `Case=` into `feats`
  via `featsFromCaseLabel` so even that resolution path emits FEATS.
  The shared composer `pkg/lemmatizer-fi-et/udfeats::Compose` is the
  canonical FEATS-string assembler; FST analyses persist `Analysis.Feats`
  at parse time so generated tables are self-describing on disk.
  Production FI and ET lemmatizer tables (the local
  `localdata/lemmatizer-fi-et/` artifact) are generated locally by users;
  the runtime falls back to dict-only for a language when that language's
  table is absent.

## Near-Term Direction

The intended sequence from the current codebase is:

1. harden auth/session/abuse controls and retention before public go-live
2. keep parser improvements tied to frozen baselines, stratified reports,
   and analyzer columns (`omorfi` for FI, `estnltk` for ET)
3. improve dictionary-entry attachment, especially bulk-scale homonym
   ranking surfaced by the Ekilex import
4. keep production FI/ET FST table generation reproducible with row counts,
   provenance notes, and eval gates
5. use the corpus pipeline to feed learner-facing frequency artifacts,
   sentence banks, and parser-improvement candidates
6. polish learner flows around Inspect -> Decks -> Review -> known words
   once parser output and corpus artifacts are trustworthy
