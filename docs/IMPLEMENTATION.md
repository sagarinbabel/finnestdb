# FinEstDB Implementation Notes

This document describes the current implementation on `main`.

## Current Product Surface

The shipped frontend is a parser workbench:

- workbench nav shell with mobile menu
- parse page for Finnish or Estonian text
- two parser modes: `basic` and `custom`
- file load support for `.txt` and `.md`
- results page with parser metadata, coverage gauge, parse duration, POS filter chips, and sortable output
- browser-tested parse flows for POS filtering, language switching, file upload, and mobile nav

The dashboard, review, and account systems described in older planning docs are
not part of the current user-facing flow on `main`.

## Current Architecture

### Rust parser (`/parser`)

The Rust library is still a heuristic parser, not a full morphology engine.

Current responsibilities:

- NFC normalization
- sentence splitting with punctuation heuristics
- tokenization with punctuation separation
- simple POS guessing from endings
- JSON output over FFI

### Go API (`/internal/api`, `/internal/store`)

The main active endpoint is:

- `POST /api/parse`

Current parse flow:

1. validate language and text length (`300,000` Unicode characters)
2. call the Rust parser
3. collect unique surface forms
4. resolve forms against the dictionary
5. enrich output with glosses, example sentence, and grammar labels
6. return parser results plus parse duration and parse-stage stats

Current parse stats returned by `parsecore` and `/api/parse` include:

- unique-form and sentence counts
- resolved vs unresolved token counts
- per-source token counts (`dict`, `stub`, fallback sources, `punct`)
- timing breakdowns for analyzer, form lookup, gloss lookup, sentence resolution, and word enrichment

There is also partial backend support for deck creation and review-related
tables/endpoints, but those are not the current focus and are not represented
in the frontend flow on `main`.

### Parser modes

- `basic`
  - direct dictionary lookup only
  - unresolved forms fall back to stub lemma/POS

- `custom`
  - direct dictionary lookup
  - Finnish possessive suffix stripping
  - Finnish/Estonian compound splitting
  - Finnish/Estonian case suffix stripping

These are two parser modes over the same Rust parser output, not two fully
independent morphology engines.

### Frontend (`/web`)

The frontend is a small vanilla TypeScript app compiled to `app.js`.

Current UI features:

- desktop/mobile navigation shell focused on the parser workbench
- parse form with language selector
- language mismatch warning
- file load support for `.txt` / `.md`
- coverage gauge with token/row-weighted proxy score
- POS filter chips above the results table
- results table with row numbering and sorting
- parser mode badge
- parse duration badge
- inline example sentence expansion
- theme toggle

## Build and Tooling

### Core build

- `make parser` builds the Rust parser library
- `make server` builds the Go server
- `make run` builds and runs the app

### Frontend build

The frontend does have a build step now:

```bash
cd web
npm install
npm run build
```

This compiles `app.ts` to `app.js` using the local TypeScript dependency.

### Browser regression coverage

There is a Playwright browser suite for the parser workbench:

```bash
cd web
npx playwright test
```

The test boots the Go server on `:8081` via `web/playwright.config.ts` and
checks:

- parse/results rendering
- POS filter behavior
- language mismatch blocking and switching
- file upload flow
- mobile nav behavior

## Current Limitations

- no bundled Omorfi/Vabamorf runtime in the browser-facing parse flow
- no statistical disambiguation yet
- no current user-facing dashboard/review/account flow
- deck/review backend work remains partial and mostly stubbed

What does exist now:

- dataset-based parser evaluation
- parser comparison CLI
- parse-stage observability metrics for analyzer, lookups, resolution, and enrichment
- expanded Finnish and Estonian gold datasets for manual regression checks
- external Omorfi adapter slot for Finnish baseline testing

## Near-Term Direction

The immediate focus is parser quality, not user-account features.

Near-term work:

1. narrow or remove auth/deck/review stubs that do not match the parser-workbench product surface
2. continue reviewing and promoting Finnish/Estonian gold cases
3. freeze refreshed baselines with the new observability fields
4. improve the custom parser against the new eval regressions
5. keep Omorfi as a comparison baseline for Finnish
6. design a richer lexical knowledge layer after parser quality improves

Only after that parser layer is strong enough should user accounts, known-word
tracking, and review features return as active product work.
