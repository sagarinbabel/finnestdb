# FinEstDB TODO - Findings & Action Items

_Current as of 2026-04-29 — see [docs/CHANGELOG.md](docs/CHANGELOG.md) for revisions._

This is the single repo-level task list. It tracks current audit work, active
engineering backlog, and longer-term findings from the PRD review.

The dated execution plan for the consumer alpha lives at the bottom of this
file under [2026-04-29 — Consumer alpha execution plan](#2026-04-29--consumer-alpha-execution-plan).
That plan supersedes the older PRD/parser-workbench framing where they
conflict.

## Table of Contents

- [Current Audit Status](#current-audit-status)
- [Roadmap Status](#roadmap-status)
- [Active Engineering Backlog](#active-engineering-backlog)
- [Critical Findings](#critical-findings)
  - [1. Synchronous Deck Creation Blocking Issue](#1-synchronous-deck-creation-blocking-issue)
  - [2. Disambiguation Model Specification Missing](#2-disambiguation-model-specification-missing)
  - [3. MWE Handling Underspecified](#3-mwe-handling-underspecified)
  - [4. Example Generation FFI Contract Incomplete](#4-example-generation-ffi-contract-incomplete)
- [Implementation Priorities](#implementation-priorities)
  - [High Priority](#high-priority)
  - [Medium Priority](#medium-priority)
  - [Low Priority](#low-priority)
- [Notes](#notes)

## Current Audit Status

Checklist from the 2026-04-29 repo-wide audit:

- [x] Run Go test suite (`go test ./...`)
- [x] Run Rust parser tests (`cargo test`)
- [x] Build frontend TypeScript (`npm run build`)
- [x] Run browser smoke test (`npx playwright test`)
- [x] Fix results-page regression from the nav/results redesign
- [x] Compare implementation against `ARCHITECTURE.md` and `docs/IMPLEMENTATION.md`
- [x] Consolidate current work into this single repo-level TODO file

## Roadmap Status

Current roadmap state:

- [x] Phase 1: workbench hardening and test coverage
- [x] Phase 2: evaluation data expansion and parser observability
- [ ] Phase 3: targeted parser improvements driven by eval regressions
- [ ] Phase 4: design the richer lexical knowledge layer
- [ ] Phase 5: reintroduce study-product features only after parser quality is strong enough

## Active Engineering Backlog

Near-term items that remain open after Phase 1 and Phase 2:

- [x] Add backend/API tests for `POST /api/parse` and the partial deck/review/auth handlers in `internal/api`
- [x] Expand browser coverage beyond one parse/results smoke flow: nav shell behavior, POS filter chips, language-switch warning, and file-upload flow
- [x] Add structured parse observability to parser output and evaluation reporting
- [x] Promote additional Finnish and Estonian gold datasets for parser evaluation
- [ ] Either implement real auth/deck/review behavior or narrow the exposed stub endpoints so the server surface matches the current product focus
- [ ] Freeze refreshed baseline reports that include the new observability fields and latest Finnish/Estonian gold sets
- [ ] Review additional Finnish and Estonian draft cases for promotion after more corpus mining
- [ ] Use the new eval regressions to prioritize parser fixes, starting with recursive compounds and consonant gradation
- [ ] Add analyzer cache-hit and unknown-lemma counters to complement the existing stage-timing stats
- [ ] Document the expected browser-QA setup more clearly in the repo so Playwright use is obvious on a fresh checkout

## Critical Findings

### 1. Synchronous Deck Creation Blocking Issue

**Problem:**
Synchronous deck creation currently assumes the entire 2 MB upload is parsed in-request (§3.2) while running the full Rust pipeline (steps 1-7) and even MWE discovery (§5.1). There's no latency/error budget, timeout story, or fallbacks if Omorfi/Vabamorf hiccup, so the dashboard may block on a 10-20 s call or fail outright.

**References:** `finnestdb-prd-alpha.md:73-137`

**Action Items:**
- [ ] Define operational constraints for parsing: expected latency per 10k tokens, max retries, and when to push work to a background job/queue so `/api/decks` can return quickly with a "processing" state rather than blocking
- [ ] Extend parser observability beyond current stage timings with analyzer cache hits and counters for "unknown lemma / guesser used" so you can see when the pipeline drifts or when corpora/lexicons need updates

### 2. Disambiguation Model Specification Missing

**Problem:**
The parser spec names disambiguation techniques (Viterbi over UD tags, lemma frequency priors) but never states where the training data/model lives, how it's versioned, or how you'll evaluate "good enough," so you can't tell whether you're shipping raw analyzer output or a tuned tagger.

**References:** `finnestdb-prd-alpha.md:125-137,295-303`

**Action Items:**
- [ ] Nail down the disambiguation assets: choose specific UD treebanks + license, spell out model training (features, smoothing), publish evaluation metrics (UAS/LAS or lemma accuracy), and version the model alongside the Rust crate so regressions are testable

### 3. MWE Handling Underspecified

**Problem:**
MWE handling is described as "seed lexicon + PMI/LLR + DP segmentation" but the lexicon format, scoring thresholds, or governance aren't defined; without that, ingestion may spam users with false positives or miss idioms entirely, and nothing guarantees phrases line up with the deck sentences you plan to highlight.

**References:** `finnestdb-prd-alpha.md:133-166,300-314`

**Action Items:**
- [ ] Formalize the MWE subsystem before coding: schema for `pattern_json`, acceptance thresholds for PMI/LLR, and a review loop for user-submitted candidates; consider starting with "seed only" for alpha to keep risk bounded
- [ ] Draft the MWE lexicon schema so front-end requirements (highlighting, counts) can be exercised even with dummy data

### 4. Example Generation FFI Contract Incomplete

**Problem:**
Example generation relies on "FST synthesizer + reparse to validate features" (§4.3) yet the FFI only exposes `inflect` per token. There's no mention of sentence-level agreement (e.g., subject pronouns, enclitic placement) or how you assemble grammatical filler words, so generated sentences risk being ungrammatical even if the target word changes case correctly.

**References:** `finnestdb-prd-alpha.md:114-159`

**Action Items:**
- [ ] Expand the FFI contract to cover whole-sentence synthesis: expose a helper that given a lemma + desired feature change returns one or more grammatically sound sentences (maybe templated), or alternatively move generation to Go and only call Rust for token-level inflections
- [ ] Document how agreement, pronoun insertion, and enclitic handling work

## Implementation Priorities

### High Priority

1. **Prototype parser with Omorfi integration**
   - [x] Prototype `analyze_text` + Omorfi wiring with a small corpus to validate throughput
   - [x] Measure baseline performance via parser-eval timing output and parse-stage observability
   - [ ] Establish timeout and retry policies

2. **MWE lexicon schema**
   - [ ] Draft the MWE lexicon schema
   - [ ] Create seed lexicon with example entries
   - [ ] Define pattern matching rules

3. **Parser quality fixes from eval regressions**
   - [ ] Extend `tryCompoundSplit()` to handle recursive/ternary compounds (e.g. "lentokenttäbussi" = lento+kenttä+bussi)
   - [ ] Add Finnish consonant gradation tables for common stem alternations
   - [ ] Re-run Finnish and Estonian gold baselines after each fix and keep only justified gains

### Medium Priority

4. **Disambiguation model**
   - [ ] Select UD treebanks (Finnish, Estonian)
   - [ ] Train initial POS tagging model
   - [ ] Establish evaluation metrics and baseline
   - [ ] Version model artifacts

5. **Server surface cleanup**
   - [ ] Either implement real auth/deck/review behavior or narrow the exposed stub endpoints so the server surface matches the parser-workbench product focus
   - [ ] Remove or isolate non-parser product scaffolding that no longer reflects the active roadmap

6. **Custom dictionary knowledge graph spike**
   - [ ] Spike a separate custom lexicon for Finnish and Estonian that can accumulate data from multiple upstream dictionaries plus manual edits
   - [ ] Design provenance tables so accepted fields (definition, examples, morphology, register) retain source attribution and fetch/import metadata
   - [ ] Design a compiled read model so hot-path lookups remain indexed and near direct-lookup cost, with no live provenance merge in request handling
   - [ ] Define a slower live-merge/admin view for curation, debugging, and experimenting with merge rules outside the request path
   - [ ] Define how fallback lookups append new source facts and trigger per-entry recompilation rather than full-database rebuilds
   - [ ] Define manual injection flows for curated edits, CSV/JSONL imports, and precedence rules between manual facts and auto-imported facts

### Low Priority

7. **Background job system**
   - [ ] Design async processing architecture for deck creation
   - [ ] Implement job queue (in-memory or external)
   - [ ] Add "processing" state to deck model
   - [ ] Create webhook/polling mechanism for status updates

8. **Sentence generation**
   - [ ] Design sentence-level synthesis API
   - [ ] Implement agreement rules
   - [ ] Add validation via re-parsing
   - [ ] Test with various feature changes

9. **EPUB and file upload support**
   - [ ] Add EPUB text extraction to the import pipeline (parse XHTML content documents, strip markup, concatenate chapter text)
   - [ ] Accept file upload in `/api/import/decks` alongside raw text paste
   - [ ] Support plain-text (.txt) and EPUB (.epub) as initial formats
   - Surasura already does EPUB extraction for Japanese/Chinese; same approach applies to Finnish/Estonian content
   - Lowers friction for book-based learners who currently have to paste text manually

10. **External vocabulary import (Anki, CSV)**
   - [ ] Design an import endpoint (`POST /api/import/known-words`) that accepts a list of known lemmas+POS pairs
   - [ ] Support Anki deck export (.apkg or exported .txt) as an import source for bootstrapping `user_known_lemmas`
   - [ ] Support plain CSV/TSV import for users with custom vocabulary lists
   - [ ] Map imported surface forms to known lemmas using the existing dictionary lookup + fallback chain
   - Surasura imports known vocabulary from Anki, Migaku, and Jiten.moe to bootstrap the user's known-word state; same idea applies here so coverage metrics and new-card selection are useful from day one

11. **Comprehension prediction per deck**
   - [ ] Add a "predicted comprehension %" display to deck detail views using token-weighted coverage
   - [ ] Show before/after projection: "if you learn the top N words from this deck, your comprehension goes from X% to Y%"
   - [ ] Compute marginal comprehension gain per word to drive study ordering
   - Token-weighted coverage (`srs-deck-spec.md §Coverage metrics`) already defines the formula; this item is about surfacing it as a prominent UI feature
   - Surasura's core UX centers on showing comprehension percentages before and after consuming media

12. **Highest-leverage study ordering across decks**
    - [ ] Extend new-card ranking to consider comprehension gain across all study-list decks, not just token_count within a single source
    - [ ] Rank candidate words by: "how many tokens across all active decks does learning this lemma unlock?"
    - [ ] Allow the user to weight decks by priority (high/medium/low) so words in high-priority content are preferred
    - Current ranking (`srs-deck-spec.md §New card selection`) sorts by token_count within the selected source; cross-deck optimization would be a meaningful upgrade
    - Surasura generates "highest-leverage order" study sequences by analyzing frequency across a user's entire content library

13. **Progress dashboard**
    - [ ] Implement the dashboard tab with learning progress visualization over time
    - [ ] Show: total known lemmas, cards in review, comprehension trend per deck, daily review count
    - [ ] Add a cumulative comprehension chart: how does total coverage change as the user learns more words?
    - The frontend already has a dashboard tab placeholder; this is about filling it with meaningful data
    - Surasura has an interactive HTML dashboard with progress tracking that users find motivating

14. **Observability**
    - [x] Add timing instrumentation to parser steps
    - [ ] Track analyzer cache hit rates
    - [ ] Monitor unknown lemma frequency
    - [ ] Create dashboards/alerts for parser health

15. **Three-part compound splitting**
    - [ ] Extend `tryCompoundSplit()` to handle recursive/ternary compounds (e.g. "lentokenttäbussi" = lento+kenttä+bussi)
    - Currently only binary splits are supported, which covers ~90% of Finnish/Estonian compounds
    - Profile real-world miss rates before implementing — may not be worth the false-positive risk

16. **Consonant gradation rules**
    - [ ] Add Finnish consonant gradation tables (kk→k, pp→p, tt→t, etc.) to case suffix stripping
    - Case suffix stripping currently requires an exact stem match in the lemmas table; gradation would allow "kaupassa" → "kauppa" (pp→p at morpheme boundary)
    - Requires a rule table mapping strong↔weak grade pairs; start with the 15 most common patterns

17. **Bloom filter for compound pre-filtering**
    - [ ] Profile compound splitting performance on large texts (10k+ tokens) before implementing
    - Currently each unresolved form triggers up to N×2 SQLite queries for split-point attempts
    - A Bloom filter over the forms table could eliminate most impossible splits without DB queries
    - Only implement if profiling shows compound splitting as a bottleneck (>10% of parse time)

## Notes

### Post-Alpha Follow-Ups from Alpha PR Review

- [ ] Define and implement a retention policy for `parse_sessions.source_text`, including whether full source text should be stored eagerly on every parse or only once a user submits feedback against that session
- [ ] Add rate limiting and abuse controls to `POST /api/parse`, `POST /api/parse/feedback`, and login routes before broad public rollout
- [ ] Preserve existing `card_state` scheduling data when rebuilding `cards` during schema migrations instead of dropping and recreating the table
- [ ] Batch known/ignored checks during deck creation so card seeding does not do one lookup per unique `(lang, lemma, pos)` pair
- [ ] Replace `COUNT(*)` existence checks in known-word and parse-feedback paths with `EXISTS`/short-circuit queries once alpha correctness work is merged
- [ ] Parse history UI so logged-in users can review and delete their stored parse sessions
- [ ] Add an opt-in ephemeral parse flag on `/api/parse` so logged-in users can request a non-persisted parse
- [ ] Document parse-session storage behavior directly in the parse UI, not only in docs

- These findings were identified during PRD review and stub implementation
- Items are organized by severity and implementation priority
- Check off items as they are completed
- Update this document as new findings emerge or priorities change

---

## 2026-04-29 — Consumer alpha execution plan

This is the locked execution plan for the FinEstDB consumer alpha. Where
this plan disagrees with older sections of `TODO.md`,
`finnestdb-prd-alpha.md`, `ARCHITECTURE.md`, or `docs/IMPLEMENTATION.md`,
this plan wins. Older sections remain for historical context but are not
re-litigated here.

Companion docs introduced alongside this plan:

- [docs/FEATURES.md](docs/FEATURES.md)
- [docs/CROSS_LANGUAGE_STRATEGY.md](docs/CROSS_LANGUAGE_STRATEGY.md)
- [docs/CHANGELOG.md](docs/CHANGELOG.md)

### Summary

- Build the alpha as a consumer language-learning product with a clear split
  between:
  - public/anonymous product surfaces
  - authenticated user study surfaces
  - admin-only parser and feedback operations
- Ship only when Finnish and Estonian are both first-class across the same
  core user flow: `paste -> inspect -> correct -> deck -> review`.
- Use global cards so vocabulary knowledge belongs to the user, not to decks.
- Keep the full parser workbench admin-only.
- Allow logged-in users to access a lightweight parse-inspection view and
  submit parser corrections.
- Use two evaluation tracks:
  - Track A: offline gold + external benchmark
  - Track B: live accepted-correction metrics from real usage
- External benchmark parity:
  - Finnish: Omorfi
  - Estonian: EstNLTK / Vabamorf
- Reuse [`docs/baselines/`](docs/baselines/) as the canonical baseline store.
- Implement in reviewable slices and open PRs per slice; resolve conflicts
  as they arise, then stop for review before merge.

### Product model

- **Anonymous visitor**
  - can see landing and product explanation surfaces
  - can sign in
  - cannot create decks
  - cannot review
  - cannot submit full parser corrections in alpha
- **User**
  - can sign in
  - can paste/import text
  - can view lightweight parse inspection
  - can import known words
  - can create decks
  - can review cards
  - can submit parser corrections
  - cannot access workbench internals, admin queue, or benchmark/eval tools
- **Admin**
  - everything a user can do
  - full parser workbench access
  - feedback triage queue
  - parser comparison/eval surfaces if exposed
  - weekly parser quality reporting
  - annotation/testing surfaces
- **Michael's scope**
  - production auth and session model
  - user/admin role separation
  - payment/paywall/free-tier limits
  - deployment and live hardening
  - testing and ET annotation support
  - not parser-quality strategy owner

### Implementation sequence

#### PR 1 — Planning and product docs

- Append this plan to `TODO.md` under a dated execution-plan section.
- Create `docs/FEATURES.md`, `docs/CHANGELOG.md`, and
  `docs/CROSS_LANGUAGE_STRATEGY.md`.
- Update live planning/architecture docs with dated headers and changelog
  references.
- `docs/FEATURES.md` is user-perspective and decision-complete about: what
  the product is, how users learn before reading, leverage and comprehension
  concepts, progress tracking concept, mobile web direction, and technology
  differentiators (fast parser, benchmarked quality, user-correction loop,
  future autoresearch).

#### PR 2 — Auth roles and surface separation

- Extend the current mock-cookie auth into a role-aware alpha auth model.
- Add an admin flag to the user model and role-aware response behavior in
  `GET /api/me`.
- Split surfaces into anonymous, authenticated user, and admin-only.
- Restrict the current workbench in `web/index.html` and `web/app.ts` to
  admin access.
- Add a lightweight parse-inspection surface for logged-in users.
- Correction submission requires login. No anonymous full correction flow.

#### PR 3 — Frontend surface split

- Separate anonymous, user, and admin surfaces in the UI.
- Keep the existing frontend architecture in `web/app.ts`.
- Add: landing/product explanation, sign-in, deck list, rename/delete,
  review, known-word import/manage, lightweight parse inspection,
  admin-only workbench gating, admin feedback queue surface.
- Preserve one responsive app and existing breakpoints in `web/styles.css`.
- Validate mobile usability at 375 px.

#### PR 4 — Known words and global cards

- Keep global cards as the alpha model.
- Implement known-word import with canonical resolution at import time using
  the existing resolver chain in `internal/store/dict.go`.
- `POST /api/known-words` returns resolved imports and unresolved inputs.
- `GET /api/known-words?lang=`
- delete-one support for known words
- Deck creation:
  - persists `sentences` and `occurrence`
  - derives unique `(lemma, pos)` pairs
  - skips `user_known_lemmas` and `user_ignored_lemmas`
  - ensures one global `cards` row and one `card_state` row per remaining pair

#### PR 5 — Parse feedback subsystem

- Add or verify `parse_feedback` schema in `internal/store/db.go`.
- Add parse/session identifiers to parse results so feedback ties to a
  specific run.
- Implement `POST /api/parse/feedback`.
- Logged-in users can submit corrections from the lightweight
  parse-inspection view.
- Add admin queue surface (`/admin/feedback.html` or equivalent) with
  accept / reject / needs follow-up actions.
- Accepted corrections become the official signal for live error metrics.
- Document the full flow in `docs/PARSER_FEEDBACK_LOOP.md`.

#### PR 6 — Deck CRUD and review flow

- Implement: `GET /api/decks`, `PATCH /api/decks/{id}`, `DELETE /api/decks/{id}`.
- Replace mocked dashboard counts in `GET /api/me`.
- `DELETE /api/decks/{id}` deletes only the deck content graph
  (`occurrence`, `sentences`, `decks`). Do not delete `cards` or
  `card_state`.
- Add alpha scheduler module and document the deviation from full FSRS in
  `docs/srs-deck-spec.md`.
- `GET /api/review/next?deck_id=` means due global cards, optionally
  filtered to cards appearing in the selected deck's occurrences.
- `POST /api/review/answer`, `POST /api/card/known`, `POST /api/card/ignore`.
- Alpha backside content is intentionally thin: lemma, gloss, one example
  sentence, optional grammar label.

#### PR 7 — Track A evaluation parity (Estonian)

- Use two offline subtracks for each language: gold dataset evaluation and
  external benchmark comparison.
- Finnish: compare `basic`, `custom`, `omorfi`.
- Estonian: compare `basic`, `custom`, external EstNLTK/Vabamorf adapter mode.
- Expand ET manual gold to at least Finnish manual scale and comparable
  annotation density.
- Audit `internal/parserules/estonian.go` against
  `internal/parserules/finnish.go`: implement ET equivalents where
  appropriate; document N/A where not applicable; add ET-specific handling
  for already-identified morphology categories.
- Add `make eval`, `make compare-parsers`, `make eval-check`.
- Freeze FI and ET reports under `docs/baselines/`.
- `docs/EVAL_AND_CI.md` describes gold evaluation, external benchmark
  evaluation, and baseline regression policy.

#### PR 8 — Track B live quality metrics

- Define production metrics sourced from parse usage plus accepted
  corrections.
- Minimum capture: parse id, user id, language, parser mode, token count,
  unique lemma count, correction submissions, accepted corrections.
- Minimum derived metrics: accepted correction rate per 1,000 tokens and
  per 1,000 unique lemmas, by language and by parser mode.
- Deliver first as a weekly admin report, not a polished analytics dashboard.
- Document Track B in `docs/EVAL_AND_CI.md` and
  `docs/PARSER_FEEDBACK_LOOP.md`.

#### PR 9 — Security review and hardening pass

- Scope: auth/session behavior, role enforcement on admin-only routes, CSRF
  posture for cookie-based auth, XSS exposure in feedback and parse views,
  rate limiting on login and feedback endpoints, data isolation between
  users, admin-route leakage to non-admins, correction submission abuse
  surface.
- Record findings and dispositions in `docs/SECURITY_REVIEW_ALPHA.md`.
- Fix any high-severity issues before stopping for merge review.

### Parallel ownership split

- **Main backend owner**: PR-2 (Auth/Roles), PR-4 (Known Words + Global
  Cards), PR-5 (Parse Feedback), PR-6 (Deck CRUD + Review), PR-8 (Track B
  Reporting), PR-9 (Security Review).
- **Second model (parallel safe)**: PR-1 (Planning + Product Docs), PR-3
  (Frontend Surface Split, after PR-2 contract is fixed), PR-7 (ET
  Evaluation + Benchmark + Baselines).

High-conflict files where parallel edits must be avoided:

- `internal/api/handlers.go`
- `internal/store/db.go`

### Public APIs

- `GET /api/me` — role-aware, real dashboard counts
- `POST /api/auth/login` — role-aware session behavior (alpha entry)
- `GET /api/decks`
- `PATCH /api/decks/{id}`
- `DELETE /api/decks/{id}`
- `POST /api/known-words`
- `GET /api/known-words?lang=`
- `GET /api/review/next?deck_id=`
- `POST /api/review/answer`
- `POST /api/card/known`
- `POST /api/card/ignore`
- `POST /api/parse/feedback`
- Admin-only feedback review interface

### Documentation deliverables

- `docs/FEATURES.md` (this PR)
- `docs/CHANGELOG.md` (this PR)
- `docs/CROSS_LANGUAGE_STRATEGY.md` (this PR)
- `docs/SYSTEM_OVERVIEW.md` (later PR)
- `docs/PARSER.md` (later PR)
- `docs/PARSER_FEEDBACK_LOOP.md` (PR 5)
- `docs/EVAL_AND_CI.md` (PR 7 / PR 8)
- `docs/KNOWN_WORDS.md` (PR 4)
- `docs/MICHAEL_TODO.md` (later PR)
- `docs/SECURITY_REVIEW_ALPHA.md` (PR 9)

### Acceptance criteria

1. This plan is appended to the repo's live planning doc and linked from
   `docs/CHANGELOG.md`.
2. Anonymous users can access only marketing/product explanation and sign-in.
3. Logged-in users can complete `paste -> inspect -> correct -> deck -> review`
   in both FI and ET.
4. Admins can access workbench and feedback queue; normal users cannot.
5. Full correction submission requires login.
6. Known-word import resolves to canonical `(lemma, pos)` and reports
   unresolved inputs.
7. Deck deletion removes deck content but not global learning state.
8. Finnish has gold evaluation plus Omorfi comparison.
9. Estonian has gold evaluation plus EstNLTK/Vabamorf comparison.
10. ET manual gold reaches at least FI manual scale and comparable density.
11. `make eval`, `make compare-parsers`, and `make eval-check` cover both FI
    and ET.
12. Frozen FI and ET baseline reports live under `docs/baselines/`.
13. Weekly admin reporting shows accepted correction rates by language and
    parser mode.
14. `docs/CROSS_LANGUAGE_STRATEGY.md` explicitly captures how the parsers
    improve together at the strategy level.
15. A lightweight security review is completed and documented before signoff.
16. PRs are opened in reviewable slices and work stops for review before
    merge.

### Assumptions

- Global cards are the alpha learning model.
- Parser workbench is admin-only.
- Lightweight parse inspection is user-visible only after login.
- Anonymous full correction submission is out of scope for alpha.
- Omorfi remains the Finnish external benchmark.
- EstNLTK / Vabamorf is the Estonian external benchmark.
- `docs/baselines/` is the single canonical baseline store.
- Cross-language improvement is shared at the
  infrastructure/evaluation/error-taxonomy layer, not by copying morphology
  blindly between languages.
