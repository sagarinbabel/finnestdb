# Candidate-merging architecture (proper full fix)

The "proper full fix" for the cascade-vs-merge architectural bug
documented in [docs/LEARNINGS.md](LEARNINGS.md). Replaces the existing
single-resolution-per-form `BatchLookupForms` with a multi-candidate
collection + unified ranker, so every analysis path (dict, possessive,
compound, case-suffix, FST) contributes candidates and the ranker picks
the winner with FEATS-aware scoring.

This document is the durable spec; the implementation lands across four
PRs (Phase 1 below, then 2-4 separately).

## Why we're doing this

The current scheduling has three problems:

1. **Cascade > merge.** `BatchLookupForms` returns the first hit. A dict
   PROPN-homonym misrank can't be corrected by an FST analysis that
   would have ranked the inflected-form lemma higher, because the FST
   never runs on dict hits.
2. **Two FST calls per form.** With PR #117's step promotion, the FST
   runs once during step 1 (to attach FEATS to dict hits) and again as
   step 5 (when dict misses). Same form, same analysis, twice.
3. **Source-priority is one-dimensional.** The ranker scores
   case-match → POS-sanity → source_priority → tiebreaks. It can't
   express "dict and FST agree on lemma, prefer this over a singleton
   dict candidate," which is the natural way to use cross-source
   agreement.

## Target architecture

```
                        ┌─────────────────────────┐
                        │ BatchLookupAllCandidates│
                        └──────────┬──────────────┘
                                   │
        ┌──────────┬───────────────┼──────────────┬────────────┐
        ▼          ▼               ▼              ▼            ▼
   ┌────────┐ ┌──────────┐   ┌──────────┐   ┌────────────┐ ┌────────┐
   │ dict   │ │possessive│   │ compound │   │case-suffix │ │  FST   │
   │ rows   │ │ strip    │   │ split    │   │ strip      │ │analyze │
   └───┬────┘ └────┬─────┘   └────┬─────┘   └─────┬──────┘ └────┬───┘
       │          │              │               │             │
       └──────────┴───── candidates ─────────────┴─────────────┘
                                   │
                                   ▼
                        ┌─────────────────────────┐
                        │      pickBest…          │  ← FEATS-aware ranker
                        └──────────┬──────────────┘
                                   │
                                   ▼
                            FormResolution
```

Every form goes through every path concurrently. Each path contributes
zero or more `FormCandidate` records. The ranker scores the union and
picks one for the legacy `FormResolution` API, while preserving the
candidate list for callers that want it (n-best eval, contextual
disambiguator, debug UI).

## The new core type

```go
// FormCandidate is one analysis of a surface form. Multi-candidate
// callers see the full list; legacy callers get the ranker's pick via
// FormResolution.
type FormCandidate struct {
    Lemma          string
    POS            string
    Feats          string  // UD FEATS, e.g. "Case=Ine|Number=Sing"
    GrammarLabel   string  // Case-only projection for back-compat
    Source         string  // "dict", "possessive", "compound", "case_suffix", "fst:vfst", "fst:hfstol"
    SourcePriority int     // dict source-priority + path-specific bonus
    Confidence     float64 // 0-1; reserved for future probabilistic sources
}
```

Differences from today's `formCandidate` (lowercase, internal):

- Adds `Feats`, `GrammarLabel`, `Confidence` (see below)
- Source string identifies the path *and* the underlying source, not just
  the dict source-priority

## The four phases

### Phase 1 — Foundational types and dual API (this PR)

Introduces the new `FormCandidate` type as a first-class API. Adds
`BatchLookupAllCandidates(forms, lang, mode) map[string][]FormCandidate`
implemented as a wrapper that calls today's flow and converts the
single-result `FormResolution` to a one-element candidate slice.

**No behavior change.** The legacy `BatchLookupForms` keeps its current
shape — Phase 2 swaps its internals.

Why a separate phase: it's pure types. Reviewable in isolation.
Subsequent phases can incrementally migrate callers.

### Phase 2 — Parallel candidate collection

Rewrites `BatchLookupForms` internals to:

1. Always run dict step 1 (with multi-row results from forms.feats).
2. Always run FST when available (FI: VFST + HFSTOL via lemmatizer pkg;
   ET: HFSTOL only).
3. In `custom` mode, also run possessive (FI), compound, case-suffix
   strip — all in parallel, each contributing zero-or-more candidates.
4. Pass the union to `pickBestFormCandidate` (renamed/extended; see
   Phase 2.5 below).

The cascading "if hit return" pattern is gone.

**Performance**: today's PR #117 promotion already runs FST on every
custom-mode token. Phase 2 doesn't change FST call count; it
*eliminates* the duplicate step-1.5 vs step-5 calls.

### Phase 2.5 — FEATS-aware ranker

Today's `pickBestFormCandidate` scores: case-match → POS-sanity →
source_priority → tiebreaks. Extended ranker:

| Tier  | Signal                                       | Notes |
|-------|----------------------------------------------|-------|
| 1     | Case-match (uppercase surface ↔ uppercase lemma) | Existing |
| 2     | POS-sanity (lowercase surface ↛ PROPN)       | Existing |
| 3     | **Cross-source agreement on (lemma, POS)**   | NEW: lemma-POS pair appearing in 2+ sources outranks singletons |
| 4     | **Cross-source agreement on FEATS**          | NEW: same FEATS string from 2+ sources |
| 5     | FEATS-richness (more attributes wins)        | NEW: dict candidate with `Case=Ine` loses to FST candidate with `Case=Ine\|Number=Sing\|Person=3` |
| 6     | Source priority                              | Existing |
| 7     | Deterministic tiebreaks                      | Existing |

Tier 3 + 4 are the cross-validation the user's "no compromises" memory
calls for: combining sources to break ties.

### Phase 3 — Sentence-level disambiguation

Today the ranker decides one token at a time. UD train sets give us
~290k FI / ~407k ET tokens to learn from. Build a Go-native bigram POS
tagger:

- Train on UD-Finnish-TDT-train + UD-FTB-train (FI) and UD-Estonian-EDT-train + UD-EWT-train (ET)
- Features: trigram-of-POS, bigram-of-(POS, FEATS-Case)
- Smoothing: simple Witten-Bell or Kneser-Ney
- Storage: gob-serialized model file embedded via `go:embed`, ~5-10 MB
- Implementation: ~500 LOC in `pkg/disambiguator/`

`BatchLookupForms` returns one candidate per form today. Phase 3 lets
the disambiguator override the ranker's per-token pick when a different
candidate has higher sentence-level joint probability.

### Phase 4 — N-best eval

Eval ParserSummary gains `NBestAccuracy` — for each metric, what
fraction of tokens have the correct answer in the top-N candidates.
Distinguishes "the parser had it right but ranker picked wrong"
(addressable in Phase 2.5/3) from "the parser never had it" (needs
new analyzer source).

`cmd/parser-compare` adds n-best columns to the headline.

## Measurement targets

Numbers are estimates against ud-fi-tdt-test (1,554 cases / 21k tokens),
on the post-FEATS-migration eval. Set against today's headline:

| Metric          | Today (post-#117) | Target after Phase 2 | Target after Phase 3 |
|-----------------|------------------:|---------------------:|---------------------:|
| Lemma           |             53.4% |              ~60–65% |              ~75–80% |
| POS             |             59.9% |              ~70–75% |              ~85–90% |
| Case            |             49.6% |              ~70–75% |              ~85–90% |
| FeatsRecall     |             33.9% |              ~50–60% |              ~70–80% |

Phase 2 gains come from cross-source agreement breaking PROPN homonym
misranks; Phase 3 from contextual disambiguation removing per-token
ambiguity. These are estimates; actual numbers come from running the
phases.

## Open design questions

1. **How aggressive on cross-source agreement?** Tier 3 says "lemma-POS
   pair in 2+ sources beats singletons." But the FST may have many
   spurious analyses. Threshold: 2 sources is fine; 3 is suspicious of
   noise. We'll pick the threshold empirically per phase 2 results.
2. **Confidence calibration.** Phase 1 reserves a `Confidence` field;
   we don't fill it yet. Phase 2 sets it from source-priority + agreement
   count; Phase 3 replaces it with the disambiguator's posterior.
3. **API exposure.** Should the public `/api/parse` return n-best? Adds
   payload size. Defer to product decision.

## Out of scope

- Speed-of-light optimizations (mmap'd FSTs, query batching). Today's
  ~12k words/s on custom is acceptable; if Phase 2's parallel
  collection costs another 2× we'll address.
- Multi-language disambiguator transfer. Phase 3's tagger is per-language.
