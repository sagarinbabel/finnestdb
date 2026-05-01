# FinEstDB Implementation Notes

_Current as of 2026-05-01 — see [CHANGELOG.md](CHANGELOG.md) for revisions._

This document describes the current implementation on `main`.

## Current Product Surface

The shipped frontend is a role-aware single-page app:

- public landing/about/sign-in routes
- authenticated dashboard, Inspect, Decks, Review, and shared results routes
- admin-only parser workbench and parser-feedback routes
- Inspect page for Finnish or Estonian text
- file load support for `.txt` and `.md`
- results page with dictionary coverage, POS filter chips, sortable output,
  correction actions, and deck save flow
- browser-tested auth routing, Inspect/results, deck/review, correction, and
  admin workbench flows

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

There is also backend support for deck creation, known words, review cards, and
parser feedback. Some product surfaces are still intentionally lightweight and
are tracked in `TODO.md`.

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

- desktop/mobile navigation with public, user, and admin route groups
- Inspect form with language selector
- language mismatch warning
- file load support for `.txt` / `.md`
- coverage gauge with token/row-weighted proxy score
- POS filter chips above the results table
- results table with row numbering and sorting
- user-facing dictionary coverage copy
- admin-only parser mode and parse duration details
- inline example sentence expansion
- correction submission modal
- deck save, deck list, and review flow
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

There is a Playwright browser suite for the role-aware app:

```bash
cd web
npx playwright test
```

The test boots the Go server on `:8081` via `web/playwright.config.ts` and
checks:

- anonymous/user/admin route guards
- parse/results rendering
- deck creation and review flow
- correction submission
- POS filter behavior
- language mismatch blocking and switching
- file upload flow
- mobile nav behavior

## Current Limitations

- no bundled Omorfi/Vabamorf runtime in the browser-facing parse flow
- no statistical disambiguation yet
- alpha auth is a stub and not safe for go-live
- no signed-in parse history/delete UI yet
- known-word import/manage UI is still maturing
- admin feedback triage UI is still maturing

What does exist now:

- dataset-based parser evaluation
- parser comparison CLI
- parse-stage observability metrics for analyzer, lookups, resolution, and enrichment
- expanded Finnish and Estonian gold datasets for manual regression checks
- external Omorfi adapter slot for Finnish baseline testing

## Near-Term Direction

Near-term work:

1. complete go-live auth/session hardening before any public deployment
2. finish known-word import/manage UX and admin feedback triage UX
3. continue reviewing and promoting Finnish/Estonian gold cases
4. freeze refreshed baselines with the new observability fields
5. improve the custom parser against the eval regressions
6. keep Omorfi as a comparison baseline for Finnish
