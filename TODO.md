# FinEstDB TODO - Findings & Action Items

This is the single repo-level task list. It tracks current audit work, active
engineering backlog, and longer-term findings from the PRD review.

## Table of Contents

- [Current Audit Status](#current-audit-status)
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

## Active Engineering Backlog

Near-term items that remain open after the audit:

- [ ] Add backend/API tests for `POST /api/parse` and the partial deck/review/auth handlers in `internal/api`
- [ ] Expand browser coverage beyond one parse/results smoke flow: nav shell behavior, POS filter chips, language-switch warning, and file-upload flow
- [ ] Decide whether Dashboard/Catalog/Review stay as disabled placeholders in the nav shell or get hidden until routes exist
- [ ] Either implement real auth/deck/review behavior or narrow the exposed stub endpoints so the server surface matches the current product focus
- [ ] Review the new Finnish draft/gold parser-eval cases for additional promotion or correction after more corpus mining
- [ ] Document the expected browser-QA setup more clearly in the repo so Playwright use is obvious on a fresh checkout

## Critical Findings

### 1. Synchronous Deck Creation Blocking Issue

**Problem:**
Synchronous deck creation currently assumes the entire 2 MB upload is parsed in-request (§3.2) while running the full Rust pipeline (steps 1-7) and even MWE discovery (§5.1). There's no latency/error budget, timeout story, or fallbacks if Omorfi/Vabamorf hiccup, so the dashboard may block on a 10-20 s call or fail outright.

**References:** `finnestdb-prd-alpha.md:73-137`

**Action Items:**
- [ ] Define operational constraints for parsing: expected latency per 10k tokens, max retries, and when to push work to a background job/queue so `/api/decks` can return quickly with a "processing" state rather than blocking
- [ ] Plan parser observability: per-step timings, analyzer cache hits, and counters for "unknown lemma / guesser used" so you can see when the pipeline drifts or when corpora/lexicons need updates

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
   - [ ] Measure baseline performance (tokens/second, memory usage)
   - [ ] Establish timeout and retry policies

2. **MWE lexicon schema**
   - [ ] Draft the MWE lexicon schema
   - [ ] Create seed lexicon with example entries
   - [ ] Define pattern matching rules

3. **Background job system**
   - [ ] Design async processing architecture for deck creation
   - [ ] Implement job queue (in-memory or external)
   - [ ] Add "processing" state to deck model
   - [ ] Create webhook/polling mechanism for status updates

### Medium Priority

4. **Disambiguation model**
   - [ ] Select UD treebanks (Finnish, Estonian)
   - [ ] Train initial POS tagging model
   - [ ] Establish evaluation metrics and baseline
   - [ ] Version model artifacts

5. **Sentence generation**
   - [ ] Design sentence-level synthesis API
   - [ ] Implement agreement rules
   - [ ] Add validation via re-parsing
   - [ ] Test with various feature changes

6. **Custom dictionary knowledge graph spike**
   - [ ] Spike a separate custom lexicon for Finnish and Estonian that can accumulate data from multiple upstream dictionaries plus manual edits
   - [ ] Design provenance tables so accepted fields (definition, examples, morphology, register) retain source attribution and fetch/import metadata
   - [ ] Design a compiled read model so hot-path lookups remain indexed and near direct-lookup cost, with no live provenance merge in request handling
   - [ ] Define a slower live-merge/admin view for curation, debugging, and experimenting with merge rules outside the request path
   - [ ] Define how fallback lookups append new source facts and trigger per-entry recompilation rather than full-database rebuilds
   - [ ] Define manual injection flows for curated edits, CSV/JSONL imports, and precedence rules between manual facts and auto-imported facts

### Low Priority

7. **EPUB and file upload support**
   - [ ] Add EPUB text extraction to the import pipeline (parse XHTML content documents, strip markup, concatenate chapter text)
   - [ ] Accept file upload in `/api/import/decks` alongside raw text paste
   - [ ] Support plain-text (.txt) and EPUB (.epub) as initial formats
   - Surasura already does EPUB extraction for Japanese/Chinese; same approach applies to Finnish/Estonian content
   - Lowers friction for book-based learners who currently have to paste text manually

8. **External vocabulary import (Anki, CSV)**
   - [ ] Design an import endpoint (`POST /api/import/known-words`) that accepts a list of known lemmas+POS pairs
   - [ ] Support Anki deck export (.apkg or exported .txt) as an import source for bootstrapping `user_known_lemmas`
   - [ ] Support plain CSV/TSV import for users with custom vocabulary lists
   - [ ] Map imported surface forms to known lemmas using the existing dictionary lookup + fallback chain
   - Surasura imports known vocabulary from Anki, Migaku, and Jiten.moe to bootstrap the user's known-word state; same idea applies here so coverage metrics and new-card selection are useful from day one

9. **Comprehension prediction per deck**
   - [ ] Add a "predicted comprehension %" display to deck detail views using token-weighted coverage
   - [ ] Show before/after projection: "if you learn the top N words from this deck, your comprehension goes from X% to Y%"
   - [ ] Compute marginal comprehension gain per word to drive study ordering
   - Token-weighted coverage (`srs-deck-spec.md §Coverage metrics`) already defines the formula; this item is about surfacing it as a prominent UI feature
   - Surasura's core UX centers on showing comprehension percentages before and after consuming media

10. **Highest-leverage study ordering across decks**
    - [ ] Extend new-card ranking to consider comprehension gain across all study-list decks, not just token_count within a single source
    - [ ] Rank candidate words by: "how many tokens across all active decks does learning this lemma unlock?"
    - [ ] Allow the user to weight decks by priority (high/medium/low) so words in high-priority content are preferred
    - Current ranking (`srs-deck-spec.md §New card selection`) sorts by token_count within the selected source; cross-deck optimization would be a meaningful upgrade
    - Surasura generates "highest-leverage order" study sequences by analyzing frequency across a user's entire content library

11. **Progress dashboard**
    - [ ] Implement the dashboard tab with learning progress visualization over time
    - [ ] Show: total known lemmas, cards in review, comprehension trend per deck, daily review count
    - [ ] Add a cumulative comprehension chart: how does total coverage change as the user learns more words?
    - The frontend already has a dashboard tab placeholder; this is about filling it with meaningful data
    - Surasura has an interactive HTML dashboard with progress tracking that users find motivating

12. **Observability**
    - [ ] Add timing instrumentation to parser steps
    - [ ] Track analyzer cache hit rates
    - [ ] Monitor unknown lemma frequency
    - [ ] Create dashboards/alerts for parser health

13. **Three-part compound splitting**
    - [ ] Extend `tryCompoundSplit()` to handle recursive/ternary compounds (e.g. "lentokenttäbussi" = lento+kenttä+bussi)
    - Currently only binary splits are supported, which covers ~90% of Finnish/Estonian compounds
    - Profile real-world miss rates before implementing — may not be worth the false-positive risk

14. **Consonant gradation rules**
    - [ ] Add Finnish consonant gradation tables (kk→k, pp→p, tt→t, etc.) to case suffix stripping
    - Case suffix stripping currently requires an exact stem match in the lemmas table; gradation would allow "kaupassa" → "kauppa" (pp→p at morpheme boundary)
    - Requires a rule table mapping strong↔weak grade pairs; start with the 15 most common patterns

15. **Bloom filter for compound pre-filtering**
    - [ ] Profile compound splitting performance on large texts (10k+ tokens) before implementing
    - Currently each unresolved form triggers up to N×2 SQLite queries for split-point attempts
    - A Bloom filter over the forms table could eliminate most impossible splits without DB queries
    - Only implement if profiling shows compound splitting as a bottleneck (>10% of parse time)

## Notes

- These findings were identified during PRD review and stub implementation
- Items are organized by severity and implementation priority
- Check off items as they are completed
- Update this document as new findings emerge or priorities change
