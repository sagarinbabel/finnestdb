# FinEstDB

A language learning application for Finnish and Estonian using spaced repetition (FSRS).

## Table of Contents

- [How to Run & Test](#how-to-run--test)
  - [Prerequisites](#prerequisites)
  - [Build & Start](#build--start)
  - [Testing the Parse Feature](#testing-the-parse-feature)
  - [Language Validation](#language-validation)
  - [Known Limitations](#known-limitations)
- [Project Structure](#project-structure)
- [Documentation](#documentation)
- [License](#license)

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

### Testing the Parse Feature

1. Open **http://localhost:8080**
2. Select a language: **Finnish (FI)** or **Estonian (ET)**
3. Paste text into the textarea (up to 10,000 characters — roughly 2 pages)
4. Click **Parse Text**
5. You'll see a word list table:

| Column | What it shows |
|--------|--------------|
| Lemma | Base/dictionary form of the word |
| Part of Speech | NOUN, VERB, ADJ, etc. |
| Forms in Text | Inflected forms found in the text |
| Count | How many times the word appears |

Results are sorted from most frequent to least frequent.

**Good test texts to try:**
- Any Finnish Wikipedia article (copy-paste from the browser)
- Any Estonian news article from err.ee or postimees.ee
- The sample sentences in `docs/GETTING_STARTED.md`

### Language Validation

The app checks whether your pasted text matches the selected language:

- **Estonian detection:** the character `õ` is unique to Estonian — its presence is a strong signal
- **Finnish detection:** `ä` and `ö` appear in >1.5% of letters in typical Finnish text
- If neither signal is found, you'll see a warning (English text, for example, will trigger this)

The warning is advisory only — you can still parse the text.

### Known Limitations (stub parser)

The current Rust parser is a **basic heuristic stub**. It:
- Tokenises on whitespace (periods may attach to words as `kauppaan.`)
- Guesses POS from word endings (rough approximation)
- Does **not** do real morphological analysis (Omorfi/Vabamorf not yet integrated)

This means lemmas and POS tags will often be incorrect. The infrastructure
(HTTP endpoint, word list table, language validation) is the thing being tested.
The parser accuracy will improve once Omorfi/Vabamorf are wired in.

## Project Structure

```
/cmd/server          Go HTTP server entry point
/internal/api        API handlers (POST /api/parse, etc.)
/internal/parserffi  CGO bindings to the Rust parser library
/internal/store      SQLite database layer
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

- [Getting Started Guide](docs/GETTING_STARTED.md)
- [Implementation Analysis](IMPLEMENTATION_ANALYSIS.md)
- [PRD (Alpha)](finnestdb-prd-alpha.md)
- [TODO / Findings](TODO.md)

## License

MIT
