# FinEstDB

A role-aware Finnish and Estonian reading app backed by dictionary-based
lemmatization, parser evaluation, spaced repetition, and an admin-only parser
workbench.

## Table of Contents

- [What makes this parser special](#what-makes-this-parser-special)
- [How to Run & Test](#how-to-run--test)
  - [Prerequisites](#prerequisites)
  - [Quick start](#quick-start)
    - [A — You got a `finnestdb-bootstrap.tgz`](#a--you-got-a-finnestdb-bootstraptgz-from-a-teammate)
    - [B — Setting up from scratch](#b--setting-up-from-scratch)
    - [C — Compile only, no data](#c--compile-only-no-data-import)
  - [Frontend Build](#frontend-build)
  - [Refreshing dictionary data](#refreshing-dictionary-data)
    - [Refreshing the Ekilex data](#refreshing-ekilex-data)
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
  - **FST candidate scoring in parallel with dict step 1** (post-PR #127), with
    candidate-merge FEATS enrichment (post-PR #129). When local lemmatizer
    tables are present in `localdata/lemmatizer-fi-et/tables/`, the FST
    contributes morphological analyses alongside the dict lookup; otherwise
    the FST step is silently disabled and the dict-only path runs.

The parser core also has evaluation-only external adapter modes:

- **Omorfi** for Finnish (`omorfi`)
- **EstNLTK/Vabamorf** for Estonian (`estnltk`)

These are baseline/comparison adapters, not browser-facing parser buttons.

### Language detection

The app detects Finnish vs Estonian by character frequency analysis before
you submit:

- `õ` present → Estonian (this character never appears in Finnish)
- `ä`/`ö` > 1.5% of letters → Finnish
- Neither → advisory unknown-language warning; you can still parse

---

## How to Run & Test

### Prerequisites

| Tool      | Version | Check                | Why |
|-----------|---------|----------------------|-----|
| Go        | 1.21+   | `go version`         | Server + import tooling |
| Rust      | stable  | `cargo --version`    | Tokenizer (built once via `make parser`) |
| `curl`    | any     | `curl --version`     | Fetches Kotus + Ekilex during setup |
| `sqlite3` | any     | `sqlite3 --version`  | Used by import targets to check DB state |

The Go server uses `go-sqlite3` (bundled), so the `sqlite3` CLI is only needed
during dev/setup, not at runtime.

> **Note:** You cannot open `web/index.html` directly in the browser. The app
> makes API calls to the Go server, so the server must be running first.

### Quick start

Pick whichever path matches your situation. Each leaves you with a server on
**http://localhost:8080**.

#### A — You got a `finnestdb-bootstrap.tgz` from a teammate

The tarball contains the populated `finnestdb.db` and the fetched/reduced data
under `localdata/`. Untar next to the repo and you skip every fetch and import:

```bash
git clone https://github.com/sagarinbabel/finnestdb.git
cd finnestdb
tar xzf path/to/finnestdb-bootstrap.tgz   # extracts localdata/ + finnestdb.db
make run
```

#### B — Setting up from scratch

```bash
git clone https://github.com/sagarinbabel/finnestdb.git
cd finnestdb
bash scripts/setup-local.sh   # builds parser, fetches data, populates finnestdb.db
make run
```

`scripts/setup-local.sh` is the single bootstrap entry point — see
[`docs/ARTIFACT_POLICY.md`](docs/ARTIFACT_POLICY.md) for what data lives where.
It is idempotent: re-running skips already-fetched content. Knobs:

- **No `EKILEX_API_KEY`**: the script auto-skips the multi-hour Ekilex
  `/api/word/details` scrape. The parser still works; Estonian coverage is
  reduced. Get a free key at <https://ekilex.ee/> and re-run when you want
  the full enrichment.
- **`SKIP_UD=1`**: skip the ~50 MB × 6 UD treebank clones (parser-eval only).
- **`SKIP_SILVER=1`**: skip the Gutenberg-FI silver scrape (parser-eval only).
- **`SKIP_EKILEX_DETAILS=1`**: force-skip Ekilex details even with API key.

If you only need the dictionary lookup path running, the fast variant is:

```bash
SKIP_UD=1 SKIP_SILVER=1 bash scripts/setup-local.sh
```

#### C — Compile only, no data import

```bash
make build       # builds Rust parser + Go server binary
go test ./...    # runs the test suite
```

Use this when you're hacking on parser code and don't need a populated DB.
`./finnestdb` will start but the parse page returns zero matches until you
populate the DB via path A or B.

#### Verifying your setup: `make doctor`

Once any of paths A / B / C completes, run `make doctor` to see what
quality mode the parser will run in:

```bash
make doctor
```

Reports DB presence + per-source row counts, FST table presence, analyzer
venv presence (`.venv-omorfi`, `.venv-estnltk`), Ekilex shard presence,
UD cache, frequency baselines, and the Rust parser shared library — each
with a one-line hint for the missing pieces. Returns `0` unless the DB
or the FI/ET dictionary is missing entirely; everything else is
informational so you understand the *degraded modes* your setup implies
rather than discovering them from surprise eval numbers.

---

The app opens to the public landing page. Sign in with an email address and
password (8+ chars) to use Parse, Decks, and Review. Auth is password-based
with Argon2id hashes and DB-backed `session_token` sessions; see
[`docs/GO_LIVE_CHECKLIST.md`](docs/GO_LIVE_CHECKLIST.md) for the remaining
go-live controls before exposing the app to real users.

### Frontend Build

The browser loads [`web/app.js`](web/app.js), which is compiled from [`web/app.ts`](web/app.ts).

If you change the TypeScript, rebuild it with:

```bash
cd web
npm install
npm run build
```

`npm install` is only needed the first time or after dependency changes.

### Browser regression tests (Playwright)

There is a Playwright browser suite for the role-aware app:

```bash
cd web
npx playwright test
```

The test boots the Go server on `:8081` via [`web/playwright.config.ts`](web/playwright.config.ts) and checks:

- anonymous / user / admin route guards
- parse / results rendering
- deck creation and review flow
- parser-feedback (correction) submission
- POS filter behavior
- hybrid language detection (auto-switch on high-confidence paste, blocking mismatch warning)
- file upload flow
- mobile nav behavior at 375 px

Run from a fresh checkout: `make parser && cd web && npm install && npx playwright test`.

### Refreshing dictionary data

> **Most contributors don't need this section.** `scripts/setup-local.sh`
> (path B above) chains every relevant import target end-to-end. Read on
> only if you're refreshing one language without re-running the full
> bootstrap, or debugging a specific import step.

Per-language import targets (one-time, ~5 min per language):

```bash
make import-dict-fi             # Finnish  (~12-20M forms, downloads ~200MB)
make import-dict-et             # Estonian (kaikki.org base)
make import-ekilex-et           # Estonian EKI 2026 public headwords (from localdata/)
make import-ekilex-details-et   # Estonian Ekilex full reduced drop (~178k lemmas, ~6.2M forms)
make import-dict-et-ekilex      # (optional) Estonian Ekilex API smoke import (requires EKILEX_API_KEY; not recommended for full runs)
```

Recommended Estonian import (reliable, fast at import time, assumes
`localdata/ekilex/` is populated):

```bash
make import-dict-et-recommended
```

`make run` does **not** auto-import — import once via path A, B, or one of
the targets above, then `make run` every subsequent time. The data persists
in `finnestdb.db`.

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
`localdata/ekilex/` → `cmd/importekilexdetails` (bulk-load into
the dictionary tables, multi-lemma aware). The first three stages run
offline; only the loader step is required at deploy time.

#### Refreshing Ekilex Data

**Most contributors will not need to run this — the reduced output is already
committed under `localdata/ekilex/`.** You only need to run it when
refreshing the latest Ekilex data. The full pipeline is four ordered steps;
most users will only need step 4:

1. **Fetch the list of words** — `make fetch-ekilex-refresh`
   - re-fetches `/api/public_word/eki` and overwrites the local headword list
     (`localdata/ekilex/eki-public-words-2026-et.jsonl`, gitignored)
   - only updates if the headword set has changed
2. **Scrape per-word details** — `make fetch-ekilex`
   - see setup instructions below
   - goes through the wordlist from step 1 and pulls `/api/word/details` for every `word_id`
   - writes gzipped raw payloads under `localdata/ekilex/details/raw/` (gitignored)
3. **Extract / reduce** — `make reduce-ekilex`
   - reduces the raw payloads into sharded local artifacts under `localdata/ekilex/`:
     - `definitions/<letter>.jsonl`: extracts lemma + morphology + meanings
     - `forms/<letter>.tsv`: a list of inflected forms, one row per inflected form with the corresponding lemma
   - golden-tested; see the `reduce-ekilex` notes in [Makefile](Makefile).
4. **Load into the dictionary** — `make import-ekilex-details-et`
   - bulk-loads the reduced data into the lemma/form/translation tables in
     `finnestdb.db`
   - this is the only step required at deploy time

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

**Important:** `make import-dict-et-ekilex` uses the live Ekilex API and does
per-word network calls. It is intentionally configured as a **small smoke
import** (see `EKILEX_LIMIT` in the Makefile) and may time out on slow or
rate-limited connections. For a reliable local setup, prefer:

```bash
make import-dict-et
make import-ekilex-et
make import-ekilex-details-et
```
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
2. Sign in with an email address and password (8+ chars).
3. Open **Parse**.
4. Select a language: **Finnish (FI)** or **Estonian (ET)**.
5. Paste text into the textarea (up to 300,000 Unicode characters).
6. Click **Parse text**.
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

The user-facing parse result shows dictionary coverage, unique lemmas, token
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

`omorfi` is the external-analyzer slot for a third Finnish baseline. The
recommended path is:

```bash
make setup-omorfi
go run ./cmd/parsertest \
  -dataset ./testdata/parser-eval/fi-gold-small.json \
  -parsers basic,custom,omorfi
```

That target creates a repo-local `.venv-omorfi/` (matching `.venv-estnltk/`),
installs `omorfi==0.9.12`, and downloads the HFST models into
`~/.cache/omorfi/`. The runtime auto-discovers the venv next to the bundled
adapter at `scripts/omorfi_adapter_example.py`, so no environment variables
need to be exported. To override (different python, alternative adapter), set
`FINNESTDB_OMORFI_CMD` to a command that reads source text from stdin and
returns JSON in the same token/sentence shape as the Rust FFI parser:

```bash
export FINNESTDB_OMORFI_CMD="/path/to/omorfi-adapter"
```

`estnltk` is the equivalent external-adapter slot for Estonian. To enable it:

```bash
make setup-estnltk
go run ./cmd/parsertest \
  -dataset ./testdata/parser-eval/et/gold/et-manual-v1.json \
  -parsers basic,custom,estnltk
```

See `docs/LEXICAL_PLAN.md` "Estonian-specific source choices and adapter contract" for the EKI/Ekilex lexical-data import plan.

Dataset format:
- `name`, `version`, `language`
- `cases[]` with `id`, `text`, and expected `tokens[]`
- each expected token includes `surface`, `lemma`, `pos`, and optional `grammar_label` / `feats` / `occurrence`. `feats` is a UD FEATS string like `Case=Ine|Number=Sing|Person=1`; the eval scores it per-attribute when present and is a no-op when absent. All six manual gold sets (`fi-core-v1`, `fi-grammar-v1`, `fi-manual-v1/v2`, `et-grammar-v1`, `et-manual-v1`) carry FEATS as of `2026.05.07k`

The JSON report is designed to be reusable by a future eval UI.

Golden dataset guidance:
- see [docs/PARSER_EVAL_DATASETS.md](docs/PARSER_EVAL_DATASETS.md)

### Language Validation

The app checks whether pasted or file-loaded text matches the selected language:

- **Estonian detection:** the character `õ` is unique to Estonian — its presence is a strong signal
- **Finnish detection:** `ä` and `ö` appear in >1.5% of letters in typical Finnish text
- **Fast path:** high-confidence pasted or file-loaded text auto-switches the selected language
- **Guardrail:** if the selected language still conflicts with detected Finnish or Estonian, parse is blocked until you switch languages
- If neither signal is found, you'll see an advisory warning (English text, for example, will trigger this)

Unknown-language warnings are advisory only, so you can still parse. Detected Finnish/Estonian mismatch warnings are blocking because parsing under the wrong language produces lower-quality results.

### Known Limitations

The current Rust parser is still a **heuristic stub**. It does:
- NFC normalization
- sentence splitting with simple punctuation heuristics
- tokenization that separates leading/trailing punctuation, including common
  typographic quote marks, and labels opening punctuation for sentence-text
  spacing reconstruction
- rough POS guessing from word endings

The two parser modes differ only in how much enrichment happens after that:
- **Basic parser** stops after direct dictionary lookup
- **Custom parser** adds possessive, compound, and case-suffix fallback rules. Every resolution path attaches UD FEATS where the source has it: kaikki tags via `cmd/importdict/feats.go::kaikkiTagsToFeats`, Ekilex morph_codes via `cmd/importekilexdetails/feats.go::ekilexMorphToFeats`, FST analyses via `pkg/lemmatizer-fi-et/udfeats::Compose` (called from `voikkomap.Parse` / `giellaltmap.Parse`), and the case-suffix fallback projects `Case=` via `internal/store/dict.go::featsFromCaseLabel`

What still does **not** exist yet in the browser-facing parser flow:
- bundled full morphological analysis from Omorfi/Vabamorf (they're external
  evaluation adapters, not in-process parsers)
- statistical disambiguation (CRF tagger planned in
  [`docs/ML_IDEAS.md` §1a](docs/ML_IDEAS.md))
- MWE detection (schema not yet defined; see [`TODO.md`](TODO.md)
  "Sentence-level features")
- production FI/ET lemmatizer tables (current `pkg/lemmatizer-fi-et/`
  ships smoke fixtures only — production tables are generated locally with
  `make gen-lemmatizer-tables-fi VFST_PATH=/path/to/mor.vfst` and
  `make gen-lemmatizer-tables-et HFSTOL_PATH=/path/to/analyser-gt-desc.hfstol`;
  see [`docs/ARTIFACT_POLICY.md`](docs/ARTIFACT_POLICY.md))

What **does** exist now for parser research:
- FST candidate scoring in parallel with dict step 1 (post-PR #127)
  with candidate-merge FEATS enrichment (post-PR #129)
- per-attribute FEATS eval (Case, Number, Tense, Mood, Voice, Person —
  post-PR #130)
- ~9.8k FI committed gold cases + ~37.9k ET local-only (CC BY-NC-SA);
  ~37k FI train (local). See [`docs/data_enhancement.md`](docs/data_enhancement.md)
- a parser evaluation CLI with bootstrap CIs (post-PR #114)
- external adapter slots for the Omorfi (FI) and EstNLTK (ET) baselines
- an Ekilex (ET) extraction pipeline (`fetchekilex` → `reduceekilex`)
  with golden-tested reductions written to `localdata/ekilex/` (gitignored)

Product-surface limitations (alpha):

- alpha auth is real (Argon2id + DB-backed sessions) but the wider
  go-live posture (rate limiting, CSRF, audit logging) needs the
  hardening pass tracked in
  [`docs/GO_LIVE_CHECKLIST.md`](docs/GO_LIVE_CHECKLIST.md). Don't expose
  the alpha to the public internet without it.
- no signed-in parse-history / delete-my-parse-history UI yet
- known-word import / manage UI is still maturing
- admin parse-feedback triage UI is functional but minimal
- review scheduling is a hand-rolled step scheduler, **not FSRS** —
  see [`docs/srs-deck-spec.md`](docs/srs-deck-spec.md) and
  [`TODO.md`](TODO.md) "Migrate alpha scheduler to real FSRS"
- accepted parse corrections are recorded but do not yet update lexical
  rows — see [`TODO.md`](TODO.md) "Self-improving feedback loop"

So the custom mode is stronger than the basic mode for many dictionary-backed
cases, but it is still not a full morphology parser. Production FST tables
will close most of the morphology gap; the disambiguator and feedback loop
will close most of the long tail.

## Project Structure

```
/cmd/server               Go HTTP server entry point
/cmd/parsertest           Parser evaluation CLI (basic, custom, omorfi, estnltk)
/cmd/parser-compare       Render parser-eval reports as a markdown comparison table
/cmd/corpusmine           Mine corpus text for disagreement-heavy gold candidates
/cmd/autoresearch         Parked post-live idea: parser-eval rule-ablation loop
/cmd/importdict           Dictionary import: kaikki.org JSONL or Ekilex API → SQLite
/cmd/importkotus          Kotus sanalista TSV → SQLite (populates paradigm_class)
/cmd/importekilex         Compact Ekilex public_word snapshot importer (ET headwords)
/cmd/importekilexdetails  Bulk-load reduced Ekilex data drop into dict tables (ET)
/cmd/importud             Convert Universal Dependencies CoNLL-U → parser-eval gold JSON
/cmd/fetchekilex          Resumable Ekilex /api/word/details scraper (multi-worker)
/cmd/reduceekilex         Reduce raw Ekilex payloads to sharded JSONL/TSV artifacts
/cmd/scrapegutenberg      Public-domain FI book scraper for silver-tier corpus
/cmd/fetchfrequency       Public FI/ET frequency baselines (OpenSubtitles + UD)
/cmd/genlemmatizertables  Generate FI/ET lemmatizer JSON tables from local analysers
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
/parser                   Rust tokenizer / sentence splitter (heuristic, with R1–R4
                          numeric-hyphen rules — see DECISIONS.md Decision 6)
/pkg/lemmatizer-fi-et     Generated-table FST runtime (loads from localdata/)
/web                      Frontend (HTML, CSS, TypeScript)
/localdata                Single-folder bootstrap root (gitignored). Populated by
                          `scripts/setup-local.sh`: Ekilex CC BY 4.0 shards,
                          Kotus sanalista, Gutenberg-FI silver corpus, UD treebank
                          cache, FI/ET parser-eval gold (NC-licensed ET stays here),
                          FI/ET train splits, generated lemmatizer tables, public
                          frequency baselines. `tar czf finnestdb-bootstrap.tgz
                          localdata/ finnestdb.db` captures the entire bootstrap
                          state — see docs/ARTIFACT_POLICY.md for the policy.
/testdata/parser-eval     Frozen gold datasets per language (CC BY/BY-SA only;
                          NC-licensed gold lives under localdata/parser-eval/)
/testdata/lemmatizer      Hand-authored unit-test fixtures for pkg/lemmatizer-fi-et
/docs/baselines           Frozen parser-eval baseline reports
finnestdb-prd-alpha.md    Full product requirements document (historical)
```

## Documentation

**Doc index:** [`docs/INDEX.md`](docs/INDEX.md) — single map of every doc
in this repo, organized by purpose. Read this first if you're not sure
where to look.

Architecture and ops:
- [Architecture](ARCHITECTURE.md) and [docs/SYSTEM_VERSIONING.md](docs/SYSTEM_VERSIONING.md)
- [docs/ARTIFACT_POLICY.md](docs/ARTIFACT_POLICY.md) — what's allowed in git, what lives under `localdata/`
- [docs/data_enhancement.md](docs/data_enhancement.md) — ledger of every external corpus pulled in
- [Documentation Changelog](docs/CHANGELOG.md) · [Decisions Log](docs/DECISIONS.md)
- [Go-Live Checklist](docs/GO_LIVE_CHECKLIST.md)
- [docs/IMPLEMENTATION.md](docs/IMPLEMENTATION.md) — redirect stub (split across README, PARSER_FEEDBACK_LOOP, ARCHITECTURE)
- [Implementation Analysis](IMPLEMENTATION_ANALYSIS.md) — historical pre-implementation notes (banner)

Product and strategy:
- [PRD (Alpha)](finnestdb-prd-alpha.md) · [docs/FEATURES.md](docs/FEATURES.md)
- [TODO / Findings](TODO.md)
- [docs/CROSS_LANGUAGE_STRATEGY.md](docs/CROSS_LANGUAGE_STRATEGY.md) — what is shared vs. language-specific
- [docs/ideas.md](docs/ideas.md) — exploratory roadmap, includes AI-native phasing

Lexical pipelines:
- [docs/LEXICAL_PLAN.md](docs/LEXICAL_PLAN.md) — combined FI + ET lexical layer architecture (Kotus + kaikki.org for FI; EstNLTK + EKI/Ekilex for ET; shared schema and source-priority resolver)

Parser tooling:
- [docs/PARSER_EVOLUTION.md](docs/PARSER_EVOLUTION.md) — chronological log of parser-quality measurements and what moved them
- [docs/OMORFI_ADAPTER.md](docs/OMORFI_ADAPTER.md) · [docs/OMORFI_COMPARISON.md](docs/OMORFI_COMPARISON.md)
- [docs/PARSER_EVAL_DATASETS.md](docs/PARSER_EVAL_DATASETS.md)
- [docs/AUTORESEARCH.md](docs/AUTORESEARCH.md)

Onboarding:
- [docs/GETTING_STARTED.md](docs/GETTING_STARTED.md)

## License

MIT
