# FinEstDB TODO - Findings & Action Items

This document tracks findings from the PRD review and action items for future implementation.

## Table of Contents

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
   - [ ] Prototype `analyze_text` + Omorfi wiring with a small corpus to validate throughput
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

7. **Observability**
   - [ ] Add timing instrumentation to parser steps
   - [ ] Track analyzer cache hit rates
   - [ ] Monitor unknown lemma frequency
   - [ ] Create dashboards/alerts for parser health

8. **Three-part compound splitting**
7. **Three-part compound splitting**
   - [ ] Extend `tryCompoundSplit()` to handle recursive/ternary compounds (e.g. "lentokenttäbussi" = lento+kenttä+bussi)
   - Currently only binary splits are supported, which covers ~90% of Finnish/Estonian compounds
   - Profile real-world miss rates before implementing — may not be worth the false-positive risk

9. **Consonant gradation rules**
8. **Consonant gradation rules**
   - [ ] Add Finnish consonant gradation tables (kk→k, pp→p, tt→t, etc.) to case suffix stripping
   - Case suffix stripping currently requires an exact stem match in the lemmas table; gradation would allow "kaupassa" → "kauppa" (pp→p at morpheme boundary)
   - Requires a rule table mapping strong↔weak grade pairs; start with the 15 most common patterns

10. **Bloom filter for compound pre-filtering**
9. **Bloom filter for compound pre-filtering**
   - [ ] Profile compound splitting performance on large texts (10k+ tokens) before implementing
   - Currently each unresolved form triggers up to N×2 SQLite queries for split-point attempts
   - A Bloom filter over the forms table could eliminate most impossible splits without DB queries
   - Only implement if profiling shows compound splitting as a bottleneck (>10% of parse time)

## Notes

- These findings were identified during PRD review and stub implementation
- Items are organized by severity and implementation priority
- Check off items as they are completed
- Update this document as new findings emerge or priorities change
