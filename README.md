# FinEstDB

A role-aware Finnish and Estonian reading app backed by dictionary-based
lemmatization, parser evaluation, spaced repetition, and an admin-only parser
workbench.

## Table of Contents

- [What makes this parser special](#what-makes-this-parser-special)
- [How to Run & Test](#how-to-run--test)
  - [Prerequisites](#prerequisites)
  - [Build & Start](#build--start)
  - [Frontend Build](#frontend-build)
  - [Dictionary import](#dictionary-import)
    - [Refreshing the Ekilex enrichment data](#refreshing-the-ekilex-enrichment-data)
  - [Testing the Parse Feature](#testing-the-parse-feature)
  - [Language Validation](#language-validation)
  - [Known Limitations](#known-limitations)
- [Project Structure](#project-structure)
- [Documentation](#documentation)
- [License](#license)

## What makes this parser special

### Dictionary-backed lemmatization

Most tokenizers produce a "stemmed" guess at the base form. This parser is
different: **every word is looked up in a real dictionary** derived from
Wiktionary (via [kaikki.org](https://kaikki.org)) and resolved to its actual
canonical lemma. "pankkiin" → "pankki" because the dictionary says so, not
because of a suffix rule.

```
Input:  "menin pankkiin"
Step 1: Rust tokenizer → tokens: {form:"menin"}, {form:"pankkiin"}
Step 2: BatchLookupForms → pankkiin → {lemma:"pankki", pos:"NOUN"}
                         → menin    → {lemma:"mennä",  pos:"VERB"}
Step 3: BatchLookupGlosses → pankki/NOUN → "bank (financial institution)"
                           → mennä/VERB  → "to go"
Output: word list with real base forms + English definitions
```

This approach requires a one-time dictionary import (~5 min, ~500MB) but
produces dramatically better results than any rule-based stemmer for Finnish
and Estonian — languages with highly agglutinative morphology.

### Finnish possessive suffix stripping

Finnish words can carry possessive suffixes fused with case endings.
For example: "kirjassani" = "kirjassa" (inessive of *kirja*) + "ni" (1st person singular possessive).

Rather than importing every possible possessive form into the dictionary
(which would roughly **triple the DB size** to ~50M rows), possessive suffixes
are stripped at enrichment time:

```
"kirjassani" → not in forms table
  → try strip "-ni"  → "kirjassa" → found → lemma: kirja ✓

"talo" → not in forms table
  → try strip "-si"  → "tal"      → not found → reject ✗ (stub fallback)
```

Suffixes are tried longest-first (`-nsa/-nsä/-mme/-nne` before `-ni/-si`)
to avoid partial matches. The stripped result is **always validated against
the dictionary** — preventing false positives where a suffix strip produces a
non-word. Estonian does not use this rule (different possessive marking system).

### Sentence context tracking

Every word in the parse results is linked to the **first sentence it appeared
in** from the source text. Click "▸ example" on any word to expand an inline
example sentence. This is the foundation of JPDB-style sentence mining: learn
words in the order and context they appear in text you actually care about.

When a text is saved as a deck, every token is also recorded in an `occurrence`
table (`deck_id, sentence_id, token_index, lemma, pos`), enabling per-word
sentence counts and corpus analytics in the review phase.

### Parser modes today

The current app exposes two parser modes on the parse page:

- **Basic parser**: Rust tokenization + stub lemma/POS guesses, then direct dictionary lookup only.
- **Custom parser**: the same Rust parser output, plus enrichment fallbacks in Go:
  - Finnish possessive suffix stripping
  - Finnish/Estonian compound splitting
  - Finnish/Estonian case suffix stripping

The parser core also has evaluation-only external adapter modes:

- **Omorfi** for Finnish (`omorfi`)
- **EstNLTK/Vabamorf** for Estonian (`estnltk`)

These are baseline/comparison adapters, not browser-facing parser buttons.

### Language detection

The app detects Finnish vs Estonian by character frequency analysis before
you submit:

- `õ` present → Estonian (this character never appears in Finnish)
- `ä`/`ö` > 1.5% of letters → Finnish
- Neither → warning shown (advisory only; you can still parse)

---

## How to Run & Test

### Prerequisites

| Tool | Version | Check |
|------|---------|-------|
| Go   | 1.21+   | `go version` |
| Rust | stable  | `cargo --version` |

SQLite is bundled via `go-sqlite3` — no separate install needed.

> **Note:** You cannot open `web/index.html` directly in the browser.
> The app makes API calls to the Go server, so the server must be running first.

### Build & Start

```bash
git clone https://github.com/sagarinbabel/finnestdb.git
cd finnestdb
make run
```

Then open **http://localhost:8080** in your browser.

The app opens to the public landing page. Sign in with an email address to use
Inspect, Decks, and Review. The current auth flow is an alpha stub; see
[`docs/GO_LIVE_CHECKLIST.md`](docs/GO_LIVE_CHECKLIST.md) before exposing the app
to real users.

### Frontend Build

The browser loads [`web/app.js`](web/app.js), which is compiled from [`web/app.ts`](web/app.ts).

If you change the TypeScript, rebuild it with:

```bash
cd web
npm install
npm run build
```

`npm install` is only needed the first time or after dependency changes.

### Dictionary import

The first time you run the app, you need to import the kaikki.org dictionary.
This is a one-time operation (~5 minutes per language):

```bash
make import-dict-fi             # Finnish  (~12-20M forms, downloads ~200MB)
make import-dict-et             # Estonian (optional)
make import-ekilex-et           # Estonian EKI 2026 public headwords (tracked snapshot)
make import-ekilex-details-et   # Estonian Ekilex full reduced drop (~178k lemmas, ~6.2M forms)
make import-dict-et-ekilex      # Estonian Ekilex API (on-demand, requires EKILEX_API_KEY)
```

`make run` does **not** auto-import — import once manually, then `make run`
every subsequent time. The data persists in `finnestdb.db`.

Imported rows carry a `source` and `source_priority`, so multiple sources can
coexist deterministically (kaikki=10, ekilex=20, custom=100). Higher-priority
rows replace lower-priority rows on conflict; lower-priority rows are kept
when no higher-priority entry exists.

The Ekilex snapshot is a compact export from EKI ühendsõnastik 2026 public
headwords. It adds missing Estonian direct headword lookups without overwriting
richer Kaikki-derived lemma/POS/gloss rows. The API key used to refresh that
snapshot must stay local and is not needed to import the tracked snapshot.
For end-to-end Ekilex enrichment (definitions, paradigms, ~6M form rows), the
pipeline is `cmd/fetchekilex` (resumable scrape) → `cmd/reduceekilex`
(golden-tested reduce) → tracked sharded data under
[`data/ekilex/`](data/ekilex/) → `cmd/importekilexdetails` (bulk-load into
the dictionary tables, multi-lemma aware). The first three stages run
offline; only the loader step is required at deploy time.

#### Refreshing the Ekilex enrichment data

Most contributors will not need to run this — the reduced output is already
committed under [`data/ekilex/`](data/ekilex/). Run it only when refreshing
the upstream snapshot. The full pipeline is four ordered steps:

1. **Fetch the list of words** — `make fetch-ekilex-refresh` re-fetches
   `/api/public_word/eki` and overwrites the tracked headword queue
   (`data/ekilex/eki-public-words-2026-et.jsonl`) *only if* the headword
   set has changed.
2. **Scrape per-word details** — `make fetch-ekilex` (see below) walks the
   queue and pulls `/api/word/details` for every `word_id`, writing gzipped
   raw payloads under `localdata/ekilex/details/raw/` (gitignored).
3. **Extract / reduce** — `make reduce-ekilex` reduces the raw payloads
   into sharded committable artifacts under [`data/ekilex/`](data/ekilex/):
   `definitions/<letter>.jsonl` (lemma + morphology + meanings) and
   `forms/<letter>.tsv` (one row per inflected form). Golden-tested; see
   the `reduce-ekilex` notes in [Makefile](Makefile).
4. **Load into the dictionary** — `make import-ekilex-details-et` bulk-loads
   the reduced data into the lemma/form/translation tables in
   `finnestdb.db`. This is the only step required at deploy time.

`make fetch-ekilex-sample` is an optional aid alongside step 2: it fetches a
small spread of headwords with both the `eki`-filtered and unfiltered dataset
variants so you can compare payload size/content before committing to a full
run.

##### `make fetch-ekilex` notes

Prerequisites:

- Export `EKILEX_API_KEY` (create one in your Ekilex user profile; sent as
  the `ekilex-api-key` header). Without it the command exits immediately.
- Disk: gzipped raw payloads land under `localdata/ekilex/details/raw/`
  (gitignored) — budget ~1–2 GB for the full Estonian set.

Behavior:

- **Resumable.** Per-`word_id` progress is tracked in a local SQLite
  checkpoint, so interrupting and re-running picks up where it left off.
  Already-fetched words are skipped, not re-downloaded.
- **Circuit breaker.** After 10 consecutive failures all workers pause and
  the runner probes a known-good word_id (`183007` / *koer*) every 5 minutes
  until the API recovers, then resumes automatically. Override with
  `-circuit-failures` / `-circuit-poll` if invoking the binary directly.
- **Rate limiting.** `-rps` is a *global* request-rate cap shared across
  workers, not per-worker. `-workers` should be ~2× rps so request latency
  doesn't become the bottleneck.

Tune throughput via the make variables (defaults: 16 workers, 16 rps):

```bash
make fetch-ekilex EKILEX_RPS=16 EKILEX_WORKERS=16
```

The defaults (16/16) have been run end-to-end against the full Estonian
headword set without issues. Pushing above ~20 rps / 20 workers tends to
upset the upstream API — the circuit breaker starts tripping repeatedly and
overall throughput drops. Treat 20/20 as a soft ceiling unless you've
verified the API can sustain more.

To force a full refresh (e.g. after a new kaikki.org release):

```bash
make reimport-dict-fi   # drops existing FI entries, re-imports
```

To add custom gloss overrides (e.g. domain-specific vocabulary):

```bash
go run ./cmd/importdict -lang fi -db finnestdb.db -custom-glosses ./my-overrides.csv
# CSV format: word,pos,lang,gloss
```

### Testing the Inspect Feature

1. Open **http://localhost:8080**
2. Sign in with an email address.
3. Open **Inspect**.
4. Select a language: **Finnish (FI)** or **Estonian (ET)**.
5. Paste text into the textarea (up to 300,000 Unicode characters).
6. Click **Inspect text**.
7. You'll see a word list table:

| Column | What it shows |
|--------|--------------|
| # | Row number in the current sort order |
| Lemma | Base/dictionary form of the word (click "▸ example" to see context sentence) |
| Part of Speech | NOUN, VERB, ADJ, etc. |
| Forms in Text | Inflected forms found in the text |
| Definition | English gloss from kaikki.org (Wiktionary); `Missing` if not in dictionary |
| Grammar | Case or grammar label inferred by enrichment when available |
| Tokens | How many times the lemma appears in the parsed text |

The user-facing Inspect result shows dictionary coverage, unique lemmas, token
count, definitions, grammar labels when available, and correction actions.
Internal parser-mode and parse-duration details remain visible in the admin
workbench.

`Coverage score` means: **how much of this text produced usable
dictionary-backed output**.

By default the table is sorted in parser output order, and each column can be
sorted from the UI.

**Good test texts to try:**
- Any Finnish Wikipedia article (copy-paste from the browser)
- Any Estonian news article from err.ee or postimees.ee
- The sample sentences in `docs/GETTING_STARTED.md`

### Parser Evaluation CLI

The parser evaluation MVP is currently a terminal tool, not a browser UI.

Standard make targets:

```bash
make eval
make compare-parsers
make compare-parsers-et
make eval-check
```

Run the sample dataset:

```bash
go run ./cmd/parsertest -dataset ./testdata/parser-eval/fi-gold-small.json
```

Run the expanded Finnish comparison set:

```bash
go run ./cmd/parsertest -dataset ./testdata/parser-eval/fi/gold/fi-core-v1.json -parsers basic,custom,omorfi
```

Run the expanded Estonian comparison set:

```bash
go run ./cmd/parsertest -dataset ./testdata/parser-eval/et/gold/et-manual-v1.json -parsers basic,custom,estnltk
```

This will:
- run the selected dataset against `basic` and `custom`
- print a short metrics summary to the terminal
- write a structured JSON report under `reports/parser-eval/`

Useful flags:

```bash
go run ./cmd/parsertest \
  -dataset ./testdata/parser-eval/fi-gold-small.json \
  -db ./finnestdb.db \
  -parsers basic,custom \
  -warmup 1 \
  -repeat 5 \
  -out ./reports/parser-eval/my-run.json
```

Available parser names:
- `basic`
- `custom`
- `omorfi`
- `estnltk`

`omorfi` is an external-adapter slot for a third Finnish baseline. To enable it,
set `FINNESTDB_OMORFI_CMD` to a command that reads source text from stdin and
returns JSON in the same token/sentence shape as the Rust FFI parser.

Example:

```bash
export FINNESTDB_OMORFI_CMD="/path/to/omorfi-adapter"
go run ./cmd/parsertest \
  -dataset ./testdata/parser-eval/fi-gold-small.json \
  -parsers basic,custom,omorfi
```

`estnltk` is the equivalent external-adapter slot for Estonian. To enable it:

```bash
make setup-estnltk
go run ./cmd/parsertest \
  -dataset ./testdata/parser-eval/et/gold/et-manual-v1.json \
  -parsers basic,custom,estnltk
```

See `docs/ESTONIAN_LEXICAL_PLAN.md` for the EKI/Ekilex lexical-data import plan.

Dataset format:
- `name`, `version`, `language`
- `cases[]` with `id`, `text`, and expected `tokens[]`
- each expected token includes `surface`, `lemma`, `pos`, and optional `grammar_label` / `occurrence`

The JSON report is designed to be reusable by a future eval UI.

Golden dataset guidance:
- see [docs/PARSER_EVAL_DATASETS.md](docs/PARSER_EVAL_DATASETS.md)

### Language Validation

The app checks whether your pasted text matches the selected language:

- **Estonian detection:** the character `õ` is unique to Estonian — its presence is a strong signal
- **Finnish detection:** `ä` and `ö` appear in >1.5% of letters in typical Finnish text
- If neither signal is found, you'll see a warning (English text, for example, will trigger this)

The warning is advisory only — you can still parse the text.

### Known Limitations

The current Rust parser is still a **heuristic stub**. It does:
- NFC normalization
- sentence splitting with simple punctuation heuristics
- tokenization that separates leading/trailing punctuation
- rough POS guessing from word endings

The two parser modes differ only in how much enrichment happens after that:
- **Basic parser** stops after direct dictionary lookup
- **Custom parser** adds possessive, compound, and case-suffix fallback rules

What still does **not** exist yet in the browser-facing parser flow:
- bundled full morphological analysis from Omorfi/Vabamorf
- statistical disambiguation
- MWE detection

What **does** exist now for parser research:
- gold-set evaluation datasets
- a parser evaluation CLI
- external adapter slots for the Omorfi (FI) and EstNLTK (ET) baselines
- an Ekilex (ET) extraction pipeline (`fetchekilex` → `reduceekilex`)
  with golden-tested reductions tracked under `data/ekilex/`

So the custom mode is stronger than the basic mode for many dictionary-backed
cases, but it is still not a full morphology parser.

## Project Structure

```
/cmd/server               Go HTTP server entry point
/cmd/parsertest           Parser evaluation CLI (basic, custom, omorfi, estnltk)
/cmd/parser-compare       Render parser-eval reports as a markdown comparison table
/cmd/corpusmine           Mine corpus text for disagreement-heavy gold candidates
/cmd/autoresearch         Automated rule-ablation loop driven by parser-eval
/cmd/importdict           Dictionary import: kaikki.org JSONL or Ekilex API → SQLite
/cmd/importekilex         Compact Ekilex public_word snapshot importer (ET headwords)
/cmd/importekilexdetails  Bulk-load reduced Ekilex data drop into dict tables (ET)
/cmd/fetchekilex          Resumable Ekilex /api/word/details scraper (multi-worker)
/cmd/reduceekilex         Reduce raw Ekilex payloads to sharded JSONL/TSV artifacts
/internal/api             API handlers (POST /api/parse, auth, decks, feedback)
/internal/auth            Argon2id passwords + DB-backed sliding sessions
/internal/eval            Dataset-based parser evaluation engine
/internal/parsecore       Shared parser orchestration and parser registry
/internal/parserffi       CGO bindings to the Rust parser library
/internal/parserules      Per-language enrichment rules (finnish.go, estonian.go)
/internal/store           SQLite layer
  db.go                   Schema + CRUD (users, decks, sentences, occurrences,
                          lemmas/forms with source priority, translations,
                          definitions, paradigm_class, feats)
  dict.go                 BatchLookupForms / BatchLookupGlosses
/parser                   Rust tokenizer / sentence splitter (stub heuristics)
/web                      Frontend (HTML, CSS, TypeScript)
/data/ekilex              Tracked Ekilex CC BY 4.0 snapshots: public-word JSONL,
                          sharded definitions/forms reductions, NOTICE.md
/testdata/parser-eval     Frozen gold datasets per language
/docs/baselines           Frozen parser-eval baseline reports
finnestdb-prd-alpha.md    Full product requirements document
```

## Documentation

Architecture and ops:
- [Architecture](ARCHITECTURE.md) and [docs/SYSTEM_VERSIONING.md](docs/SYSTEM_VERSIONING.md)
- [Implementation Analysis](IMPLEMENTATION_ANALYSIS.md) · [docs/IMPLEMENTATION.md](docs/IMPLEMENTATION.md)
- [Documentation Changelog](docs/CHANGELOG.md) · [Decisions Log](docs/DECISIONS.md)
- [Go-Live Checklist](docs/GO_LIVE_CHECKLIST.md)

Product and strategy:
- [PRD (Alpha)](finnestdb-prd-alpha.md) · [docs/FEATURES.md](docs/FEATURES.md)
- [TODO / Findings](TODO.md)
- [docs/CROSS_LANGUAGE_STRATEGY.md](docs/CROSS_LANGUAGE_STRATEGY.md) — what is shared vs. language-specific
- [docs/ideas.md](docs/ideas.md) — exploratory roadmap, includes AI-native phasing

Lexical pipelines:
- [docs/FINNISH_LEXICAL_PLAN.md](docs/FINNISH_LEXICAL_PLAN.md) — Kotus + Voikko + kaikki.org
- [docs/ESTONIAN_LEXICAL_PLAN.md](docs/ESTONIAN_LEXICAL_PLAN.md) — EstNLTK + EKI/Ekilex

Parser tooling:
- [docs/PARSER_EVOLUTION.md](docs/PARSER_EVOLUTION.md) — chronological log of parser-quality measurements and what moved them
- [docs/OMORFI_ADAPTER.md](docs/OMORFI_ADAPTER.md) · [docs/OMORFI_COMPARISON.md](docs/OMORFI_COMPARISON.md)
- [docs/PARSER_EVAL_DATASETS.md](docs/PARSER_EVAL_DATASETS.md)
- [docs/AUTORESEARCH.md](docs/AUTORESEARCH.md)

Onboarding:
- [docs/GETTING_STARTED.md](docs/GETTING_STARTED.md)

## License

MIT
