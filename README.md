# FinEstDB

A parser workbench for Finnish and Estonian text, focused on dictionary-backed
lemmatization and parser evaluation.

## Table of Contents

- [What makes this parser special](#what-makes-this-parser-special)
- [How to Run & Test](#how-to-run--test)
  - [Prerequisites](#prerequisites)
  - [Build & Start](#build--start)
  - [Frontend Build](#frontend-build)
  - [Dictionary import](#dictionary-import)
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

### Two parser modes today

The current app exposes two parser modes on the parse page:

- **Basic parser**: Rust tokenization + stub lemma/POS guesses, then direct dictionary lookup only.
- **Custom parser**: the same Rust parser output, plus enrichment fallbacks in Go:
  - Finnish possessive suffix stripping
  - Finnish/Estonian compound splitting
  - Finnish/Estonian case suffix stripping

So today these are best understood as **two parser modes**, not two completely
separate morphology engines. The next planned parser milestone is an
**Omorfi-backed Finnish baseline** that we can compare against the custom mode
for quality and speed.

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

No login required — the app opens directly to the parse page.

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
make import-dict-fi    # Finnish  (~12-20M forms, downloads ~200MB)
make import-dict-et    # Estonian (optional)
```

`make run` does **not** auto-import — import once manually, then `make run`
every subsequent time. The data persists in `finnestdb.db`.

To force a full refresh (e.g. after a new kaikki.org release):

```bash
make reimport-dict-fi   # drops existing FI entries, re-imports
```

To add custom gloss overrides (e.g. domain-specific vocabulary):

```bash
go run ./cmd/importdict -lang fi -db finnestdb.db -custom-glosses ./my-overrides.csv
# CSV format: word,pos,lang,gloss
```

### Testing the Parse Feature

1. Open **http://localhost:8080**
2. Select a language: **Finnish (FI)** or **Estonian (ET)**
3. Paste text into the textarea (up to 300,000 Unicode characters)
4. Click **Basic Parser** or **Custom Parser**
5. You'll see a word list table:

| Column | What it shows |
|--------|--------------|
| # | Row number in the current sort order |
| Lemma | Base/dictionary form of the word (click "▸ example" to see context sentence) |
| Part of Speech | NOUN, VERB, ADJ, etc. |
| Forms in Text | Inflected forms found in the text |
| Definition | English gloss from kaikki.org (Wiktionary); `Missing` if not in dictionary |
| Grammar | Case or grammar label inferred by enrichment when available |
| Tokens | How many times the lemma appears in the parsed text |

The results header also shows:
- which parser mode was used
- a coverage proxy score
- parse duration

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

Run the sample dataset:

```bash
go run ./cmd/parsertest -dataset ./testdata/parser-eval/fi-gold-small.json
```

Run the expanded Finnish comparison set:

```bash
go run ./cmd/parsertest -dataset ./testdata/parser-eval/fi/gold/fi-core-v1.json -parsers basic,custom,omorfi
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
- an external Omorfi adapter slot for Finnish baseline comparisons

So the custom mode is stronger than the basic mode for many dictionary-backed
cases, but it is still not a full morphology parser.

## Project Structure

```
/cmd/importdict      One-time dictionary import CLI (kaikki.org JSONL → SQLite)
/cmd/parsertest      Parser evaluation CLI
/cmd/server          Go HTTP server entry point
/internal/api        API handlers (POST /api/parse, etc.)
  handlers.go        Route handlers; parse requests delegate into parsecore
/internal/eval       Dataset-based parser evaluation
/internal/parsecore  Shared parser orchestration and parser registry
/internal/parserffi  CGO bindings to the Rust parser library
/internal/store      SQLite database layer
  db.go              Schema + CRUD (users, decks, sentences, occurrences)
  dict.go            BatchLookupForms (+ Finnish possessive strip), BatchLookupGlosses
/parser              Rust parser library (stub tokenisation)
/web                 Frontend (HTML, CSS, JavaScript)
  index.html         Single-page app shell
  app.js             Application logic (compiled from app.ts)
  app.ts             TypeScript source
  styles.css         Styling (light + dark theme)
docs/                Additional documentation
finnestdb-prd-alpha.md  Full product requirements document
```

## Documentation

- [Architecture](ARCHITECTURE.md)
- [Getting Started Guide](docs/GETTING_STARTED.md)
- [Omorfi Adapter Notes](docs/OMORFI_ADAPTER.md)
- [Parser Eval Datasets](docs/PARSER_EVAL_DATASETS.md)
- [Implementation Analysis](IMPLEMENTATION_ANALYSIS.md)
- [PRD (Alpha)](finnestdb-prd-alpha.md)
- [TODO / Findings](TODO.md)

## License

MIT
