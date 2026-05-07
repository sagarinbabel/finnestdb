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
- file load support for `.txt` / `.md`
- hybrid language handling: paste/file auto-switch on high-confidence detection,
  blocking mismatch warning when selected and detected language conflict, and
  advisory unknown-language warning
- coverage gauge with token/row-weighted proxy score
- POS filter chips above the results table
- results table with row numbering and sorting
- user-facing dictionary coverage copy
- admin-only parser mode and parse duration details
- inline example sentence expansion
- correction submission modal
- deck save, deck list, and review flow
- theme toggle

Note: Deck-detail “Suggest fix” feedback is currently wired as a **stub/skeleton**
path that attributes suggestions to the **parser run used during deck creation**
(a stored `parse_session`). It does not yet imply any automated parser training
or immediate dictionary mutation. Deck-detail rows use stored occurrence surface
forms, so submitted feedback reports a real source form rather than falling back
to the aggregate lemma.

### “Suggest fix” (parser feedback) flow — recommended UX + semantics

**Intent:** “Suggest fix” is **parser feedback**, not an immediate “fix my deck”
action. Submissions are meant to be reviewed (admin queue) and inform future
dictionary/enrichment/parser improvements.

Recommended improvements:

- **Always attach a parse session**
  - Inspect results already carry `parse_id` from `POST /api/parse`.
  - Reopened deck detail should also carry a real `parse_id` (either the deck’s
    originating `parse_session`, or a new “replay” session created on open).
  - Avoid any UX that exposes the button but cannot submit.

- **Make copy explicit**
  - Consider renaming the CTA to “Report parser issue” / “Suggest correction”.
  - In the modal, state clearly: feedback is queued for review and **does not**
    immediately change deck contents.

- **Include enough context for triage**
  - Show the source sentence (with the surface form highlighted).
  - Include parser mode and whatever stable token reference is available
    (occurrence, token index, etc.) so reviewers can reproduce.

- **Close the loop**
  - On success: confirm submission and that it’s queued for review.
  - On failure: show a specific, actionable reason (missing parse session,
    auth required, etc.), not a generic error.

- **Guardrails**
  - Hide/disable for rows that are not meaningful to correct (e.g. PUNCT).
  - Add basic spam-prevention later (rate limiting / dedupe) as needed.

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
- paste/file language auto-switching
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
