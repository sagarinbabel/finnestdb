# FinEstDB TODO — Status & Action Items

_Current as of 2026-05-15 — see [docs/CHANGELOG.md](docs/CHANGELOG.md) for revisions._

## Purpose

This is the single repo-level task list. It answers two questions for any reader:

1. **What's in main today?** — what works, what was shipped.
2. **What's next?** — open work, by area.

Other status lives elsewhere:
- **Why** decisions were made → [`docs/DECISIONS.md`](docs/DECISIONS.md)
- **What changed when** → [`docs/CHANGELOG.md`](docs/CHANGELOG.md)
- **Measured parser quality over time** → [`docs/PARSER_EVOLUTION.md`](docs/PARSER_EVOLUTION.md)
- **System architecture** → [`ARCHITECTURE.md`](ARCHITECTURE.md), [`docs/LEXICAL_PLAN.md`](docs/LEXICAL_PLAN.md)
- **Product framing** → [`docs/FEATURES.md`](docs/FEATURES.md)

## Table of Contents

- [What's in main](#whats-in-main)
- [What's not in main yet](#whats-not-in-main-yet)
- [Open PRs](#open-prs)
- [Research Goals](#research-goals)
- [Notes & historical](#notes--historical)
  - [Critical Findings (PRD review, 2026-04-29)](#critical-findings-prd-review-2026-04-29)
  - [Consumer alpha execution plan (2026-04-29)](#consumer-alpha-execution-plan-2026-04-29)
  - [Consumer flow review (2026-05-07)](#consumer-flow-review-2026-05-07)

## What's in main

Snapshot of capabilities currently shipped on main, organized by area.

### Parser core

- Rust tokenizer (NFC, sentence splitting, punctuation, numeric-hyphen rules R1–R4 per [Decision 6](docs/DECISIONS.md))
- `basic` and `custom` parser modes
- FST as parallel scorer in dict step 1 ([PR #127](https://github.com/sagarinbabel/finnestdb/pull/127))
- FST candidate-merge FEATS enrichment ([PR #129](https://github.com/sagarinbabel/finnestdb/pull/129))
- Per-attribute FEATS eval ([PR #130](https://github.com/sagarinbabel/finnestdb/pull/130))
- Parser behavior stamp `parser-v2` / `2026.05.15a`
- Lexical overlays, bad-lemma blocklists, MA/A-infinitive biasing, and low-value dict-alternative suppression from the 2026-05-12 parser-quality run
- Multi-source dictionary with row-level provenance and source-priority-first ranker ([Decision 21](docs/DECISIONS.md))
- Multi-lemma surface forms (`forms` PK = `(form, lang, lemma, pos)`)
- Possessive suffix stripping (FI), compound splitting (FI/ET), case-suffix matcher (FI/ET)
- Case-suffix grammar-label stopgap (`attachCaseLabelIfStemMatches`) — transitional

### Lexical layer

- Schema: `lemmas`, `forms`, `translations`, `definitions`, `dict_metadata` with source priorities
- FI: kaikki.org Finnish + Kotus sanalista (populates `paradigm_class`)
- ET: kaikki.org Estonian + Ekilex bulk pipeline (~178k lemmas + ~6.2M form rows)
- ET FEATS via Ekilex morph_code → UD FEATS

### Evaluation

- Gold sets: ~9.8k FI committed, ~37.9k ET local-only (CC BY-NC-SA), ~37k FI train (local)
- 2026-05-07k baseline frozen ([PR #135](https://github.com/sagarinbabel/finnestdb/pull/135)) — FI 8 datasets / 61,927 tokens, ET 2 datasets / 190 tokens
- Per-attribute FEATS eval (Case, Number, Tense, Mood, Voice, Person)
- Bootstrap CIs in comparison reports ([PR #114](https://github.com/sagarinbabel/finnestdb/pull/114))
- Held-out discipline: dev splits gitignored under `localdata/`

### App surface

- Real password auth (Argon2id, sliding sessions)
- Role-aware: anonymous / user / admin
- Routes: landing, sign-in, Inspect, Decks, Review, admin workbench, admin parse-feedback queue
- Inspect/workbench `.txt`, `.md`, and `.epub` upload extraction via `POST /api/import/extract`
- Deck CRUD, sentence/occurrence persistence, multi-lemma deck cards
- Known-word import + delete + list (`POST /api/known-words`)
- Parse feedback submission + admin triage (status only — no lexical writeback yet)
- Hand-rolled step scheduler (NOT FSRS — see "What's not in main yet")
- Hybrid language detection (auto-switch on high confidence; block on conflict; advisory on unknown)

### Data and infrastructure

- Single-folder bootstrap: gitignored runtime data under `localdata/` ([PR #131](https://github.com/sagarinbabel/finnestdb/pull/131))
- `docs/data_enhancement.md` ledger of every external corpus
- ARTIFACT_POLICY: no transducer blobs in git, generated factual tables only via local generators
- Public frequency baselines via `cmd/fetchfrequency` ([PR #134](https://github.com/sagarinbabel/finnestdb/pull/134))
- Setup automation: `scripts/setup-local.sh` (10 best-effort steps)
- Release verification targets: `make live-api-smoke` for live API/security
  probes and `make db-invariants` for production-candidate SQLite integrity,
  orphan, overlap, and source-breakdown checks

## What's not in main yet

Open work, organized by area. Each entry is brief; follow cross-links for detail.

### Parser quality

- [x] **ET lemmatizer table generator** — shipped in `cmd/genlemmatizertables -lang et -hfstol ...` plus `make gen-lemmatizer-tables-et`. Remaining production work is a real ET wordlist, provenance notes, row counts, and a fresh eval gate before relying on a full ET table in deployment.
- [x] **Re-freeze baselines once gold sets get a `feats` field** — done 2026-05-07k via PR [#139](https://github.com/sagarinbabel/finnestdb/pull/139). All 6 manual gold sets now carry FEATS (`cmd/enrichgoldfeats`); new baselines committed at `docs/baselines/2026-05-07-feats-rich-*`. The `feats_attributes` table is non-empty for omorfi (FI) and estnltk (ET); for `custom` it stays at 0% until the live SQLite DB is re-imported with the new FEATS-aware `cmd/importdict` (runbook in the methodology doc).
- [ ] **Re-import the live DB to populate `forms.feats`** — ship-after-#139. `go run ./cmd/importdict -lang fi -reimport` (or `-backfill-feats` for no downtime) populates ~26.8M FI rows from kaikki Wiktionary tags; `make import-ekilex-details-et` populates the ET rows from morph_codes. Until this runs, `custom` parser FEATS output stays empty even though all the producer code is in place.
- [ ] **Remove the `attachCaseLabelIfStemMatches` stopgap** in `internal/store/dict.go` once the FST runtime emits FEATS for direct dict hits. PR [#139](https://github.com/sagarinbabel/finnestdb/pull/139) added `featsFromCaseLabel` so the stopgap's output is at least UD-shaped (`Case=Xxx`); the remove condition still requires production FST tables.
- [ ] **Re-run FI/ET gold baselines** after each fix and keep only justified gains. Use the new eval regressions to prioritize parser fixes. Recursive compounds and consonant gradation are *not* candidates here — they're gated behind the FST migration. See [`docs/DECISIONS.md`](docs/DECISIONS.md) Decision 5.
- [ ] **Disambiguation model**: select UD treebanks (FI, ET); train initial POS tagging model; establish evaluation metrics and baseline; version model artifacts.
- [ ] **Custom dictionary knowledge graph spike**: separate custom lexicon for FI/ET that accumulates data from multiple upstream dictionaries plus manual edits; provenance tables; compiled read model for hot-path lookups; live-merge admin view; manual injection flows for curated edits, CSV/JSONL imports, precedence rules. Michael owns the full Ekilex `word/details` enrichment scrape (~87+ GB raw JSON, 174k headwords) — resumable batch job with checkpointing by `word_id`, conservative rate limiting, retry/backoff, raw responses in ignored `localdata/`, compact reduced JSONL artifact for review.

### Learner experience

- [ ] **Migrate alpha scheduler to real FSRS**. `internal/store/db.go::nextAlphaStepScheduleForRating` is a hand-rolled step scheduler with hardcoded day arrays — **not** FSRS. The launch contract now documents this honestly; [`docs/srs-deck-spec.md`](docs/srs-deck-spec.md) keeps [`go-fsrs`](https://github.com/open-spaced-repetition/go-fsrs) as the post-launch target.
  - [ ] Add `go-fsrs` dependency. Plan schema delta on `card_state` (FSRS needs stability, difficulty, last review, last rating, retrievability).
  - [ ] Implement `FSRSScheduleForRating(card, rating, now) (next time.Time, newState CardState)` behind a feature flag. Keep `nextAlphaStepScheduleForRating` as fallback while migration is in flight.
  - [ ] Migration plan for existing `card_state` rows: derive starter FSRS state from `Step`/`Streak` heuristically; document in `docs/srs-deck-spec.md`.
  - [ ] Cutover: flip flag on staging DB, validate against small cohort, then production.
  - [x] Fallback if we *don't* go to FSRS for alpha: rename the runtime scheduler honestly and update the spec to say "alpha intentionally ships a step scheduler; FSRS migration is post-alpha."

- [x] **Comprehension prediction per deck** — shipped 2026-07-02. `store.DeckComprehension` computes token-position coverage in SQL (multi-lemma positions covered when ANY candidate is known; ignored lemmas count as covered — decisions recorded in [`docs/srs-deck-spec.md` §Coverage metrics](docs/srs-deck-spec.md)). `GET /api/decks/:id/comprehension` returns coverage + top-10 unlocks with marginal gain; `comprehension_pct` rides the deck list and `/api/me` dashboard summaries. Frontend: deck-list headline, deck-detail projection panel with before→after expansion. Covered by store, handler, and Playwright tests. Cross-deck study ordering remains open below.

- [ ] **Highest-leverage study ordering across decks**. Extend new-card ranking to consider comprehension gain across all study-list decks, not just `token_count` within a single source; user weighting (high/medium/low) for deck priority. Cross-deck variant of marginal gain.

- [ ] **Source-agnostic learning-target correction overlays**. Implement the
  DB-backed model described in [`docs/CORRECTION_TAXONOMY.md`](docs/CORRECTION_TAXONOMY.md):
  learning targets can be lemma, surface, phrase, or proper-name entries, and
  accepted feedback writes to parser-identity, meaning-cue, contextual-sense,
  phrase-boundary, example-quality, or card-presentation overlay rows. This
  must work for pasted text, EPUBs, articles, subtitles, Anki imports, and
  future catalog decks, with Finnish and Estonian correction content kept
  separate.

- [x] **EPUB and file upload support**. Server-side extraction lives in `internal/epub` (zip walk + XHTML strip, ported from `corpus_pipeline/cmd/extractcorpus/extract_epub.go`). The inspect and workbench forms now accept `.txt`, `.md`, and `.epub`; `.epub` uploads are POSTed to `POST /api/import/extract` which returns plain text that lands in the textarea so the existing parse → save-deck flow handles books. Plain text continues to be read client-side. Auth-gated, 16 MiB upload cap, 1.5M-char return cap matching the textarea limit. The TODO originally named `POST /api/import/decks`; an extract-only primitive was chosen because the user flow goes through the existing `/api/decks` save path — a one-shot deck-from-file endpoint can be layered on later if needed.

- [ ] **External vocabulary import (Anki, CSV)**. Design `POST /api/import/known-words` accepting list of known lemma+POS pairs; support Anki deck export (`.apkg` or exported `.txt`); support plain CSV/TSV. Map imported surface forms to known lemmas via existing dictionary lookup + fallback chain.

- [ ] **Progress dashboard**. Fill the existing dashboard tab with: total known lemmas, cards in review, comprehension trend per deck, daily review count, cumulative comprehension chart over time.

- [ ] **Native iOS app for FinnEstDB (post-go-live)**. After the responsive
  web alpha is shipped and stable, create a native iOS app for FinnEstDB.
  Treat draft PR [#212](https://github.com/sagarinbabel/finnestdb/pull/212)
  as parked planning input; do not pull this into current go-live scope unless
  explicitly reprioritized.

- [x] **Parse history / deletion UI** so logged-in users can review and delete source context retained by saved decks and parser feedback.

- [x] **Ephemeral Inspect parse behavior** on `/api/parse` so logged-in users get non-persisted parses by default.

### Self-improving feedback loop

Accepted lemma/POS parse-feedback corrections now write `custom_overrides`
lexical rows after admin approval. Grammar/FEATS corrections, gold promotion,
and eval-gated safety checks still need follow-up. See
[`docs/FEATURES.md` "User correction loop"](docs/FEATURES.md).

- [x] **Phase 1 — apply accepted lemma/POS corrections** as a `custom_overrides` lexical row. On admin acceptance, write `forms`/`lemmas` rows with `source='custom_overrides'`, `source_priority=1000`, proposed `lemma`/`pos`, and a back-pointer to `parse_feedback.id`.
- [ ] **Phase 2 — apply accepted grammar-label corrections** to `forms.feats` for the specific surface form. Smaller blast radius. Few-day task.
- [ ] **Phase 3 — auto-promote a corrected `(surface, lemma, pos)` tuple to a gold-eval case** when N independent users submit the same correction. Threshold and review workflow TBD.
- [ ] **Phase 4 — eval-backed safety check before applying.** Run candidate `custom_overrides` row against frozen gold sets; reject on regression of ≥N cases. Reuse the existing parser-eval/baseline discipline, but do not revive or expand `cmd/autoresearch` for alpha. If it adds >100ms to admin-accept latency, push to background job.
- [ ] **Phase 5 (research, not engineering)** — automatic re-ranking of source priorities when a single source consistently produces accepted corrections in one direction. Out of scope for alpha.

Phase 1 is gated on FEATS threading (already shipped via [PR #130](https://github.com/sagarinbabel/finnestdb/pull/130)) so corrections can update FEATS, not just GrammarLabel.

### Observability

- [ ] Add analyzer cache-hit and unknown-lemma counters to complement existing stage-timing stats.
- [ ] Track analyzer cache hit rates in production.
- [ ] Monitor unknown lemma frequency.
- [ ] Create dashboards/alerts for parser health.

### Backend hardening (mostly @chickendude / go-live)

- [x] **Legacy mock-auth/raw-cookie replacement** — current auth uses Argon2id password hashes and DB-backed `session_token` sessions. Remaining go-live auth work is bootstrap retirement, CSRF/Origin posture, and operational controls in [`docs/GO_LIVE_CHECKLIST.md`](docs/GO_LIVE_CHECKLIST.md).
- [x] **@chickendude go-live app-level controls**: add rate limiting and CSRF/strict-Origin posture to `POST /api/parse`, `POST /api/parse/feedback`, login, register, and cookie-authenticated state-changing routes before broad public rollout. Deployment-level WAF/monitoring remains in [`docs/GO_LIVE_CHECKLIST.md`](docs/GO_LIVE_CHECKLIST.md).
- [x] Define and implement a retention policy for `parse_sessions.source_text`; current alpha behavior is ephemeral parse by default, with raw source text retained only for saved decks and parser feedback, then purged after 30 days by `make purge-parse-context`.
- [ ] Preserve existing `card_state` scheduling data when rebuilding `cards` during schema migrations instead of dropping and recreating.
- [ ] Batch known/ignored checks during deck creation so card seeding does not do one lookup per unique `(lang, lemma, pos)` pair.
- [ ] Replace `COUNT(*)` existence checks in known-word and parse-feedback paths with `EXISTS`/short-circuit queries.
- [ ] Document parse-session storage behavior directly in the parse UI, not only in docs.
- [x] Add a production startup guard that refuses to serve if `finnestdb.db` is missing, empty, or lacks expected FI/ET dictionary rows unless an explicit dev-only degraded mode is set. Enabled by `APP_ENV=production`; `FINNESTDB_ALLOW_DEGRADED_DB=1` is an explicit development/emergency override.
- [ ] **Security review and hardening pass**: session bootstrap retirement, role enforcement, CSRF/Origin checks, XSS, request-size caps before JSON decode, HTTP timeouts, rate limiting, data isolation, admin-route leakage, and correction-submission abuse. The 2026-06-03 deterministic tests and smoke probes cover the implemented go-live controls; full Codex Security repository-wide scanning still needs explicit subagent authorization.
- [ ] Either implement real auth/deck/review behavior or narrow the exposed stub endpoints to match current product focus. Remove or isolate non-parser scaffolding that no longer reflects the active roadmap.
- [x] **Operational constraints for parsing — measured 2026-07-02, background queue deferred.** Live measurement against the production-size DB (26.8M FI forms) with real full-novel EPUB texts: `POST /api/decks` with a 550k-char book (70,234 tokens, 10,360 unique words) completed in 1.6s; the largest local book (809k chars) in 2.0s; `POST /api/parse` on the same inputs 1.3–4.0s (cold cache worst case). That is ~0.2–0.6s per 10k tokens — the 30s `WriteTimeout` has ≥7x headroom even cold at the shipped input caps (4 MiB JSON body, 1.5M-char textarea). **Decision: deck creation stays synchronous.** A job queue + "processing" state + polling would add restart-recovery and UX complexity to shave nothing a user can feel. Revisit only if input caps rise materially or production p95 approaches ~10s (watch `parse_duration_ms` in parse stats). Latency expectations are documented in [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md).

### Sentence-level features

- [ ] **Sentence generation**: design sentence-level synthesis API; implement agreement rules; validate via re-parsing; test with various feature changes. Either expand the FFI to whole-sentence synthesis (lemma + desired feature change → grammatical sentences) or move generation to Go and call Rust only for token-level inflections. Document agreement, pronoun insertion, enclitic handling.
- [ ] **MWE lexicon schema**: draft schema for `pattern_json`, acceptance thresholds for PMI/LLR, review loop for user-submitted candidates. Consider "seed only" for alpha to bound risk. Draft schema so frontend (highlighting, counts) can be exercised with dummy data.
- [ ] **PR 8 — Track B live quality metrics** (from consumer alpha plan): production metrics from parse usage + accepted corrections. Capture: parse id, user id, language, parser mode, token count, unique lemma count, correction submissions, accepted corrections. Derived: accepted correction rate per 1,000 tokens / per 1,000 unique lemmas, by language and parser mode. Deliver as weekly admin report first; document in `docs/EVAL_AND_CI.md` and `docs/PARSER_FEEDBACK_LOOP.md`.

### Documentation

- [ ] Document the expected browser-QA setup more clearly in the repo so Playwright use is obvious on a fresh checkout.
- [ ] Review additional Finnish and Estonian draft cases for promotion after more corpus mining.

### Performance

- [ ] **Bloom filter for compound pre-filtering**. Profile compound splitting on large texts (10k+ tokens) before implementing. Each unresolved form currently triggers up to N×2 SQLite queries for split-point attempts. A Bloom filter over `forms` could eliminate most impossible splits without DB queries. Only implement if profiling shows compound splitting >10% of parse time.

### Suspended / superseded

- ~~**Three-part / recursive compound splitting via `tryCompoundSplit()`**~~ — SUPERSEDED by FST migration. libvoikko VFST handles recursive compounds natively via concatenated `[Xp]...[X]` segments. Do NOT extend `tryCompoundSplit()`. See [`docs/DECISIONS.md`](docs/DECISIONS.md) Decision 5.
- ~~**Finnish consonant gradation rules in `internal/parserules/` or `tryCaseSuffixStrip`**~~ — REJECTED. Gradation belongs in the FST's lexicon-aware paradigm tables (`pkg/lemmatizer-fi-et/`), not in string-rewrite rules over the surface. See [`docs/DECISIONS.md`](docs/DECISIONS.md) Decision 5.

## Open PRs

Snapshot. Refresh by running:

```sh
gh pr list --state open --json number,title,headRefName --jq '.[] | "- #\(.number) `\(.headRefName)` — \(.title)"'
```

Currently open as of 2026-06-01:

- [#212](https://github.com/sagarinbabel/finnestdb/pull/212) `codex/swift-offline-migration-plan` — draft/parked Swift offline migration plan.
- [#211](https://github.com/sagarinbabel/finnestdb/pull/211) `codex/correction-overlay-schema` — correction overlay schema; uniqueness-index feedback has been addressed on the PR branch and is awaiting review/CI.

## Research Goals

_Added 2026-05-07._

Larger investigations recorded here so they don't get lost between PRs.
These are not "must-do this quarter"; they are explicit research bets we
want to pursue alongside execution work.

### Discover the most-frequent inflected forms in user-pasted text

**What.** As users paste text, aggregate per-form counts into a running
tally per language. Periodically recompute and publish a ranked top-N
list of **inflected surface forms** (not lemmas) for Finnish and Estonian.

**Why this is novel (working hypothesis).** Public Finnish/Estonian
frequency lists either rank lemmas (wrong unit for a learner reading
running text) or rank forms on a fixed corpus (subtitle, news, Wikipedia)
that may not reflect what real learners want to read. The aggregated
user-pasted corpus reflects real reader interest. The "novel" claim is a
hypothesis until checked against public baselines under
[`docs/FREQUENCY_BASELINES.md`](docs/FREQUENCY_BASELINES.md).

**Tasks.**

- [ ] Schema for per-language inflected-form counts; document
      retention/anonymization policy
- [ ] Aggregation job — online (per-parse increment) or offline batch
- [ ] Ranked top-N publication: UI surface + downloadable artifact
- [ ] Comparison against the public baselines in [`docs/FREQUENCY_BASELINES.md`](docs/FREQUENCY_BASELINES.md)
      to validate the "differs from existing lists" claim
- [ ] Surface comprehension-coverage curves to learners
      ("learn N forms → understand X% of running text"), linking to
      `Comprehension prediction per deck` below

**Cross-references.**

- Rationale, ML angle, and constraints: [`docs/ML_IDEAS.md` §2b](docs/ML_IDEAS.md)
- Public baseline lists used for comparison: [`docs/FREQUENCY_BASELINES.md`](docs/FREQUENCY_BASELINES.md)
- Comprehension prediction (already tracked as §13 below) is the
  natural consumer of this ranking; both should ship together.

### Re-test FI vs ET top-N coverage with corpus-size-comparable data

**Added 2026-05-07.**

**What.** Confirm or refute the ~3–7pp Estonian-vs-Finnish coverage
advantage observed in our 2026-05-07 baseline measurements (see
[`docs/CROSS_LANGUAGE_STRATEGY.md` "Measurable Divergences"](docs/CROSS_LANGUAGE_STRATEGY.md))
using corpora that are size-matched and register-matched between the two
languages.

**Why.** The current measurement is mixed evidence:

- In OpenSubtitles 2018, the ET corpus is roughly half the size of the
  FI corpus, so smaller-corpus effect (shorter long tail) could
  partially inflate ET's top-N coverage.
- In UD treebanks, the ET treebank is roughly 2× the FI treebank — the
  opposite of the inflation direction — and ET still has a slight
  coverage advantage. So the gap is at least partly real.

A size-matched, register-matched re-measurement would let us state with
high confidence whether the gap reflects a genuine morphological
property (ET being marginally less inflectionally rich than FI) or
sampling artifacts.

**Tasks.**

- [ ] Identify or sub-sample one comparable corpus per language at the
      same register (e.g. matched-size Yle FI news + ERR ET news,
      matched-size Project Gutenberg FI + DEA ET, etc.)
- [ ] Compute and compare top-N coverage curves with bootstrap CIs
- [ ] Report the result in `docs/CROSS_LANGUAGE_STRATEGY.md` and
      `docs/FREQUENCY_BASELINES.md`

**Cross-references.**

- Initial measurement: `docs/FREQUENCY_BASELINES.md`
- Theoretical context (register variation in Zipfian distributions —
  well-studied; this work is empirical confirmation on FI/ET): see
  citations in CROSS_LANGUAGE_STRATEGY.md once added.

### Close the self-improving feedback loop (accepted corrections → lexical updates)

**Added 2026-05-07.**

**What.** A logged-in user can submit a parse correction
(`POST /api/parse/feedback`, [internal/api/handlers.go](internal/api/handlers.go))
and an admin can change its status to `accepted`
([internal/store/db.go::ReviewParseFeedback](internal/store/db.go)).
Accepted lemma/POS corrections now write `custom_overrides` lexical rows
that can change future parser output. FEATS corrections, gold-case
promotion, and eval-gated safety checks remain open.

**Why.** The correction-feedback moat is one of the project's core
differentiators (see
[`docs/FEATURES.md` "User correction loop"](docs/FEATURES.md)). Every
accepted correction that doesn't update the lexicon is a learner who fixed
something for the next learner — and didn't.

**Tasks (sequenced).**

- [x] **Phase 1 — apply accepted lemma/POS corrections as a `custom_overrides`
      lexical row.** Schema: a new source `custom_overrides` with the
      highest priority (1000), as described in
      [`docs/LEXICAL_PLAN.md`](docs/LEXICAL_PLAN.md) "Resolution Layer".
      On admin acceptance, write rows to `forms` and `lemmas` with
      `source='custom_overrides'`, `source_priority=1000`, the proposed
      `lemma`/`pos`, and a back-pointer to `parse_feedback.id`.
- [ ] **Phase 2 — apply accepted grammar-label corrections to `forms.feats`**
      for the specific surface form. Smaller blast radius than full lemma
      rewrites; useful for the 0%-grammar-on-some-datasets gap. As of
      `2026.05.07k` the `forms.feats` column is populated by the import
      pipelines themselves (`cmd/importdict/feats.go::kaikkiTagsToFeats`,
      `cmd/importekilexdetails/feats.go::ekilexMorphToFeats`), so a
      correction PR can update an existing row's FEATS instead of
      writing a parallel `custom_overrides` row in many cases.
- [ ] **Phase 3 — auto-promote a corrected `(surface, lemma, pos)` tuple
      to a gold-eval case** when N independent users submit the same
      correction. Avoids one user's typo becoming a permanent override.
      Threshold and review workflow TBD.
- [ ] **Phase 4 — eval-backed safety check before applying.** Run the
      candidate `custom_overrides` row against the frozen gold sets; reject
      if it causes a regression on N or more cases. Use the parser-eval
      baseline discipline directly; `cmd/autoresearch` remains parked.
- [ ] **Phase 5 (long-tail) — automatic re-ranking of source priorities**
      when a single source consistently produces accepted corrections in
      one direction. Out of scope for alpha; revisit after Phase 4 is
      stable.

**Cross-references.**

- Schema groundwork: source priority columns + `dict_metadata` already
  exist; the `custom_overrides` source is named in
  [`docs/LEXICAL_PLAN.md` "Resolution Layer"](docs/LEXICAL_PLAN.md).
- `cmd/autoresearch` is parked as a post-live idea. It may be useful later as
  inspiration, but it is not part of the alpha implementation scope.
- This work is gated on the FEATS threading PR (already shipped via
  [PR #130](https://github.com/sagarinbabel/finnestdb/pull/130)) so
  corrections can update FEATS, not just GrammarLabel.

**Confidence.** Phase 1 is a 1-week task with high confidence. Phase 2
is a few-day task. Phase 3 needs design before scoping. Phase 4 needs
a measured eval-time budget; if it adds >100ms to admin-accept latency,
push it to a background job. Phase 5 is research, not engineering.

## Notes & historical

### Critical Findings (PRD review, 2026-04-29)

These four findings came out of the 2026-04-29 PRD review. Most are
addressed or actively tracked in "What's not in main yet" above; this
section is preserved for traceability.

#### 1. Synchronous Deck Creation Blocking Issue

**Problem.** Synchronous deck creation assumed the entire 2 MB upload
is parsed in-request (§3.2) while running the full Rust pipeline
(steps 1–7) and even MWE discovery (§5.1). No latency/error budget,
timeout story, or fallbacks if Omorfi/Vabamorf hiccups.

**References.** `finnestdb-prd-alpha.md:73-137`.

**Status.** Tracked under "Backend hardening" → operational constraints
for parsing + background job system. Parser observability shipped
(stage timings); analyzer cache hits / unknown-lemma counters tracked
under "Observability".

#### 2. Disambiguation Model Specification Missing

**Problem.** Spec named disambiguation techniques (Viterbi over UD
tags, lemma frequency priors) but never stated where the training
data/model lives, how it's versioned, or how to evaluate "good enough."

**References.** `finnestdb-prd-alpha.md:125-137,295-303`.

**Status.** Tracked under "Parser quality" → disambiguation model.

#### 3. MWE Handling Underspecified

**Problem.** MWE described as "seed lexicon + PMI/LLR + DP segmentation"
but lexicon format, scoring thresholds, governance not defined.

**References.** `finnestdb-prd-alpha.md:133-166,300-314`.

**Status.** Tracked under "Sentence-level features" → MWE lexicon schema.

#### 4. Example Generation FFI Contract Incomplete

**Problem.** Example generation relies on "FST synthesizer + reparse
to validate features" (§4.3) yet the FFI only exposes `inflect` per
token. No sentence-level agreement (subject pronouns, enclitic
placement) or grammatical filler word assembly.

**References.** `finnestdb-prd-alpha.md:114-159`.

**Status.** Tracked under "Sentence-level features" → sentence generation.

### Consumer alpha execution plan (2026-04-29)

**This was the locked execution plan when the alpha was scoped. Most of
PR 1–6 has shipped (auth roles, frontend surface split, known
words/global cards, parse feedback subsystem, deck CRUD + review). PR 7
(ET evaluation parity) is partially shipped via Plan C / PRs
[#113](https://github.com/sagarinbabel/finnestdb/pull/113)/[#114](https://github.com/sagarinbabel/finnestdb/pull/114)/[#115](https://github.com/sagarinbabel/finnestdb/pull/115)
+ FST migration. PR 8 (Track B live quality metrics) and PR 9 (security
review) remain open. The full plan is preserved here for traceability
and is not actively re-litigated. See "What's not in main yet" above
for up-to-date open work.**

5. **Migrate alpha scheduler to real FSRS** _(added 2026-05-07)_

   `internal/store/db.go::nextAlphaStepScheduleForRating` is a hand-rolled step scheduler with hardcoded day arrays `{1,3,7,14,30,60}` (good) / `{3,7,14,30,60,90}` (easy). `again` is 10 minutes; `hard` is 8 hours. This is **not** FSRS; `docs/srs-deck-spec.md §13–24` already recommends [`go-fsrs`](https://github.com/open-spaced-repetition/go-fsrs) as the post-launch target.

   - [ ] Add the `go-fsrs` dependency. Plan the schema delta on `card_state` (FSRS needs stability, difficulty, last review, last rating, retrievability — multiple fields the current schema doesn't carry).
   - [ ] Implement `FSRSScheduleForRating(card, rating, now) (next time.Time, newState CardState)` behind a feature flag. Keep `nextAlphaStepScheduleForRating` as fallback while the migration is in flight.
   - [ ] Migration plan for existing `card_state` rows: derive starter FSRS state from `Step`/`Streak` heuristically; document it in `docs/srs-deck-spec.md`.
   - [ ] Cutover: flip the feature flag on a staging DB, validate against a small user cohort, then cutover production.
   - [x] If we *don't* go to FSRS for alpha, **rename the runtime scheduler honestly** and update the spec to say "alpha intentionally ships a step scheduler; FSRS migration is post-alpha." Either ship FSRS or stop calling the alpha scheduler "FSRS-shaped."

6. **Disambiguation model**
   - [ ] Select UD treebanks (Finnish, Estonian)
   - [ ] Train initial POS tagging model
   - [ ] Establish evaluation metrics and baseline
   - [ ] Version model artifacts

7. **Server surface cleanup**
   - [ ] Either implement real auth/deck/review behavior or narrow the exposed stub endpoints so the server surface matches the parser-workbench product focus
   - [ ] Remove or isolate non-parser product scaffolding that no longer reflects the active roadmap

8. **Custom dictionary knowledge graph spike**
   - [ ] Spike a separate custom lexicon for Finnish and Estonian that can accumulate data from multiple upstream dictionaries plus manual edits
   - [ ] Michael: run the full Ekilex `word/details` enrichment scrape when you have time and enough disk space. Sagar does not currently have space for this locally. The lightweight Ekilex import in `localdata/ekilex/eki-public-words-2026-et.jsonl` only uses `/api/public_word/eki`; the richer endpoint has POS, definitions, usage examples, and paradigms, but sample payloads were about 492 KB for `koer` and 770 KB for `maja`. Fetching details for all 174,229 public Estonian headwords would be roughly 87+ GB of raw JSON and 174k HTTP requests, likely many hours before retries or rate limits. Please run it as a resumable batch job with checkpointing by `word_id`, conservative rate limiting, retry/backoff, raw responses in ignored `localdata/`, and a compact reduced JSONL artifact for review/commit after validation.
   - [ ] Design provenance tables so accepted fields (definition, examples, morphology, register) retain source attribution and fetch/import metadata
   - [ ] Design a compiled read model so hot-path lookups remain indexed and near direct-lookup cost, with no live provenance merge in request handling
   - [ ] Define a slower live-merge/admin view for curation, debugging, and experimenting with merge rules outside the request path
   - [ ] Define how fallback lookups append new source facts and trigger per-entry recompilation rather than full-database rebuilds
   - [ ] Define manual injection flows for curated edits, CSV/JSONL imports, and precedence rules between manual facts and auto-imported facts

9. **Background job system**
   - [ ] Design async processing architecture for deck creation
   - [ ] Implement job queue (in-memory or external)
   - [ ] Add "processing" state to deck model
   - [ ] Create webhook/polling mechanism for status updates

10. **Sentence generation**
   - [ ] Design sentence-level synthesis API
   - [ ] Implement agreement rules
   - [ ] Add validation via re-parsing
   - [ ] Test with various feature changes

11. **EPUB and file upload support**
   - [x] Add EPUB text extraction to the import pipeline (parse XHTML content documents, strip markup, concatenate chapter text) — `internal/epub/extract.go`
   - [x] Accept file upload alongside raw text — `POST /api/import/extract` returns extracted text; inspect/workbench forms route `.epub` through it
   - [x] Support plain-text (.txt) and EPUB (.epub) as initial formats — `.txt`/`.md` read client-side, `.epub` via server endpoint
   - Surasura already does EPUB extraction for Japanese/Chinese; same approach applies to Finnish/Estonian content
   - Lowers friction for book-based learners who currently have to paste text manually

12. **External vocabulary import (Anki, CSV)**
   - [ ] Design an import endpoint (`POST /api/import/known-words`) that accepts a list of known lemmas+POS pairs
   - [ ] Support Anki deck export (.apkg or exported .txt) as an import source for bootstrapping `user_known_lemmas`
   - [ ] Support plain CSV/TSV import for users with custom vocabulary lists
   - [ ] Map imported surface forms to known lemmas using the existing dictionary lookup + fallback chain
   - Surasura imports known vocabulary from Anki, Migaku, and Jiten.moe to bootstrap the user's known-word state; same idea applies here so coverage metrics and new-card selection are useful from day one

13. **Comprehension prediction per deck**
   - [ ] Add a "predicted comprehension %" display to deck detail views using token-weighted coverage
   - [ ] Show before/after projection: "if you learn the top N words from this deck, your comprehension goes from X% to Y%"
   - [ ] Compute marginal comprehension gain per word to drive study ordering
   - Token-weighted coverage (`srs-deck-spec.md §Coverage metrics`) already defines the formula; this item is about surfacing it as a prominent UI feature
   - Surasura's core UX centers on showing comprehension percentages before and after consuming media

   **Sequencing and parallelization _(added 2026-05-07)_.**

   This work is **parallel-safe with the FEATS-threading PR and the
   CRF-disambiguator track** (see `docs/ML_IDEAS.md §1a`). It touches
   `web/app.ts`, a small handful of new API endpoints, and a couple of
   read-side store helpers. None of those overlap with the parser hot
   path. The math is small; the open questions are product design.

   **Formulas (verified against [`docs/srs-deck-spec.md` §Coverage
   metrics](docs/srs-deck-spec.md)).**

   ```
   personal_coverage(text, user_known_set) =
       Σ token_count[t] for t in tokens(text) where t.lemma ∈ user_known_set
       ÷ Σ token_count[t] for t in tokens(text)

   marginal_gain(text, user_known_set, candidate_lemma) =
       Σ token_count[t] for t in tokens(text) where t.lemma == candidate_lemma
       ÷ Σ token_count[t] for t in tokens(text)
   ```

   Both are O(N) over the deck's `occurrence` rows once a known-lemma
   set lookup is indexed. No new tables required. The deck's
   per-`(lemma, pos)` token counts are already materialized at deck
   creation time (`internal/store/db.go::CreateDeck` expands tokens
   into `occurrence` rows).

   **Backend tasks.**

   - [ ] Read-side helper: `store.DeckLemmaStats(deckID) []LemmaCount`
         returning `(lemma, pos, token_count)` rows from `occurrence`,
         sorted by `token_count` desc. Cache invalidation: invalidate
         on deck content change (rare).
   - [ ] Read-side helper: `store.UserKnownLemmaSet(userID, lang) map[LemmaKey]bool`
         (this likely already exists in some form for known-words
         filtering during deck creation; verify and reuse if so).
   - [ ] New endpoint: `GET /api/decks/:id/comprehension`
         - returns `{ coverage_pct: float, total_tokens: int, known_tokens: int, top_unlocks: [{lemma, pos, gain_pct, token_count}, ...] }`
         - `top_unlocks` is the top N (default 10) candidate lemmas
           ranked by `marginal_gain` for the current user.
   - [ ] Extend `GET /api/decks/:id` response to include the headline
         `comprehension_pct` so the deck list / dashboard can render
         it without an extra round trip per deck.

   **Frontend tasks.**

   - [ ] Deck-detail page: add a "Predicted comprehension" badge near
         the deck title showing `X% / 100%`. Click → expand to the
         marginal-gain projection table.
   - [ ] Deck list / dashboard: add the comprehension column.
   - [ ] Marginal-gain projection: "Learn these N words to reach Y%
         comprehension." Show the next 10 unlocks; let the user "mark
         as known" inline.

   **Open product-design questions (decide before frontend work).**

   - [ ] Where does this surface live: deck detail page, parse results
         page, or both? Recommendation: deck detail first (highest
         signal-to-noise), then optionally on parse results once the
         shape is settled.
   - [ ] How does coverage interact with the user's *ignored* lemma
         set? Treat ignored as known (so "I don't care about proper
         names" raises my coverage), or as separate? `srs-deck-spec.md`
         doesn't take a position; needs a call.
   - [ ] Form-level vs lemma-level coverage display. Most users will
         want lemma-level ("I know the word"); advanced learners may
         want form-level ("I know this exact inflected form"). Pick
         lemma-level for v1; expose form-level as a toggle later.

   **Suggested form to start in parallel.** Sketch the UI layout
   (Figma or a hand drawing is fine), write the backend endpoint
   stubs against an in-memory test deck, ship the deck-detail badge
   first as the smallest meaningful slice. The marginal-gain
   projection can be a separate PR. Confidence: high that this
   shipping order minimizes blast radius.

   **Eval / sanity check.** Before shipping, sanity-check the
   coverage numbers against the public baselines under
   [`docs/FREQUENCY_BASELINES.md`](docs/FREQUENCY_BASELINES.md) — if a user with the top-1000
   FI inflected forms as their known set sees `personal_coverage`
   far from 65% on subtitle-style decks or far from 40% on written
   decks, something is wrong. The expected band is the calibration
   data we already collected.

   **Cross-references.**

   - Formulas: [`docs/srs-deck-spec.md` §Coverage metrics](docs/srs-deck-spec.md)
   - Calibration baselines:
     [`docs/FREQUENCY_BASELINES.md`](docs/FREQUENCY_BASELINES.md) and
     [`docs/CROSS_LANGUAGE_STRATEGY.md` "Measurable Divergences"](docs/CROSS_LANGUAGE_STRATEGY.md)
   - User-text-aggregated frequency feeds the same machinery —
     see Research Goals above.
   - Cross-deck variant of marginal gain — see §6 below.

14. **Highest-leverage study ordering across decks**
    - [ ] Extend new-card ranking to consider comprehension gain across all study-list decks, not just token_count within a single source
    - [ ] Rank candidate words by: "how many tokens across all active decks does learning this lemma unlock?"
    - [ ] Allow the user to weight decks by priority (high/medium/low) so words in high-priority content are preferred
    - Current ranking (`srs-deck-spec.md §New card selection`) sorts by token_count within the selected source; cross-deck optimization would be a meaningful upgrade
    - Surasura generates "highest-leverage order" study sequences by analyzing frequency across a user's entire content library

15. **Progress dashboard**
    - [ ] Implement the dashboard tab with learning progress visualization over time
    - [ ] Show: total known lemmas, cards in review, comprehension trend per deck, daily review count
    - [ ] Add a cumulative comprehension chart: how does total coverage change as the user learns more words?
    - The frontend already has a dashboard tab placeholder; this is about filling it with meaningful data
    - Surasura has an interactive HTML dashboard with progress tracking that users find motivating

16. **Observability**
    - [x] Add timing instrumentation to parser steps
    - [ ] Track analyzer cache hit rates
    - [ ] Monitor unknown lemma frequency
    - [ ] Create dashboards/alerts for parser health

17. **Three-part compound splitting** — ~~SUPERSEDED by FST migration~~
    - Recursive compounds are handled natively by libvoikko VFST via concatenated `[Xp]...[X]` segments; see [PR #107](https://github.com/sagarinbabel/finnestdb/pull/107) (FI) and the planned ET equivalent.
    - Do NOT extend `tryCompoundSplit()` — see `docs/DECISIONS.md` Decision 5.

18. **Consonant gradation rules** — ~~REJECTED~~
    - Gradation does not belong in `internal/parserules/` or
      `internal/store/dict.go::tryCaseSuffixStrip`. It belongs in the FST's
      lexicon-aware paradigm tables (`pkg/lemmatizer-fi-et/`).
    - Adding strong↔weak grade pairs to a string-rewrite path produces
      false positives at lemma boundaries and double-counts cases the FST
      already handles. See `docs/DECISIONS.md` Decision 5.

19. **Bloom filter for compound pre-filtering**
    - [ ] Profile compound splitting performance on large texts (10k+ tokens) before implementing
    - Currently each unresolved form triggers up to N×2 SQLite queries for split-point attempts
    - A Bloom filter over the forms table could eliminate most impossible splits without DB queries
    - Only implement if profiling shows compound splitting as a bottleneck (>10% of parse time)

### Consumer flow review (2026-05-07)

Companion docs:

- [`docs/USER_FLOWS.md`](docs/USER_FLOWS.md) — screen-by-screen consumer alpha spec with wireframes and the recommended correction-flow design
- [`docs/DESIGN_AI_PROMPTS.md`](docs/DESIGN_AI_PROMPTS.md) — prompt templates for v0 / Lovable / Bolt / Cursor that respect the existing token system
- [`experiments/2026-05-07-top-1000-inflected-forms.md`](experiments/2026-05-07-top-1000-inflected-forms.md) — research plan for the cold-start seed deck

New work surfaced by the review (not yet broken into sequenced PRs):

- [ ] **Anonymous browser parse surface**. The `/api/parse` endpoint supports
  ephemeral rate-limited unauthenticated calls; decide whether to expose that
  path in the public browser UI. See `docs/USER_FLOWS.md` §1.
- [ ] **Live stats strip under the textarea**. Detected language, char count, token count, unique-form count, number count — debounced. Drives the language-mismatch banner. See `docs/USER_FLOWS.md` §1.
- [ ] **Anki .apkg upload**. Front-field extraction client-side, dropped into the textarea. New file-upload type alongside `.txt` / `.md` / `.epub`.
- [ ] **Carry-forward of anonymous parses on sign-up**. Last-N parses held in **`sessionStorage`** (tab-scoped — `localStorage` would survive browser restarts and break the anonymous-is-ephemeral promise), POSTed and persisted after account creation so the user doesn't lose what they just did. Cross-restart survival, if we ever ship it, must be an explicit opt-in checkbox.
- [ ] **Google OAuth**. Adds `auth_provider`, `auth_provider_uid` columns; `password_hash` becomes nullable for OAuth accounts. Verify the Google ID token and copy/require its `email_verified` claim rather than assuming every returned email is verified. Email+password path stays the default. See `docs/USER_FLOWS.md` §3.
- [ ] **`first_name` on the user profile**. Required at signup; used for greeting copy on the dashboard.
- [ ] **"Add to existing deck" save path**. Results-page save panel gains a radio for new-deck vs. add-to-existing; merge by `(lemma, pos)` with `deck_lemma_stats` accumulation. New verb on the deck-import API. See `docs/USER_FLOWS.md` §6.
- [x] **Ephemeral Inspect parses by default**. `/api/parse` does not write `parse_sessions`; source context is retained only when the user saves a deck or submits parser feedback.
- [x] **Parse-history UI**. Logged-in users can list retained parse sessions and delete one or all retained sessions server-side.
- [ ] **Correction flow lighter entry point**. Replace the per-row correction button with a hover/focus-revealed `✎ Wrong?` link. Add a "flag-only" radio path so users who notice a wrong parse but don't know the right answer can still submit signal. Backend: `parse_feedback.proposed_lemma`/`proposed_pos` become nullable; add `flag_only` boolean. See `docs/USER_FLOWS.md` §10.
- [ ] **Sentence translation endpoint**. `POST /api/translate-sentence` backed by Sonnet 4.6 with prompt caching. Persist results in a new `sentence_translations` table only for retained parse/deck content, keyed on source/target language + prompt version + `hash(text)`; ephemeral Inspect parses use no shared persistent cache. Wires into the review-card back and the deck-detail rows. Companion to `docs/ideas.md` "Making it AI native" Phase 1.
- [ ] **Cold-start "Top 1000" CTA**. Dashboard empty state and a `/decks/top-1000-{lang}` route that creates a private deck seeded from the baseline TSV. Gated on the research project shipping.
- [ ] **First-run register picker**. Once on first sign-in, ask "What kinds of texts do you want to read most? Conversation / News & books / Mixed." Persists to `user_language_settings`. Drives which top-1000 register the cold-start uses, and may later weight new-card ranking.
- [ ] **Account deletion**. Cascade through parses, decks, known-word lists, sessions. Profile page is otherwise out of scope for the first version, but deletion is privacy-table-stakes.
- [x] **Privacy chip on the parse form**. Persistent visible signifier under the signed-in parse textarea replaces the doc-only privacy commitment in `FEATURES.md`.

Already on this list and just confirmed by the review:

- Anonymous parse should reuse the existing `.txt` / `.md` / `.epub` extraction path; signed-in Inspect/workbench upload support is already in main via `POST /api/import/extract`
- FSRS migration — public alpha ships the documented fixed-step scheduler; do not market it as FSRS until `go-fsrs` lands
- Comprehension prediction per deck — wireframe is in `docs/USER_FLOWS.md` §8
- Rate limiting on `/api/parse` — gated on the anonymous-parse path
- Highest-leverage study ordering across decks — recommended UX gate is "user has 2+ decks", not always-on

## Notes

### Post-Alpha Follow-Ups from Alpha PR Review

- [x] Replace the legacy mock-auth/raw-cookie path; current auth uses Argon2id password hashes and DB-backed `session_token` sessions
- [x] @chickendude go-live: add rate limiting, abuse controls, and CSRF/strict-Origin posture to parse, feedback, login, register, and other cookie-authenticated state-changing routes; see `docs/GO_LIVE_CHECKLIST.md`
- [ ] Define and implement a retention policy for `parse_sessions.source_text`; current alpha behavior is ephemeral parse by default, with source context retained only for saved decks and parser feedback
- [ ] Preserve existing `card_state` scheduling data when rebuilding `cards` during schema migrations instead of dropping and recreating the table
- [ ] Batch known/ignored checks during deck creation so card seeding does not do one lookup per unique `(lang, lemma, pos)` pair
- [ ] Replace `COUNT(*)` existence checks in known-word and parse-feedback paths with `EXISTS`/short-circuit queries once alpha correctness work is merged
- [x] Parse history / deletion UI so logged-in users can review and delete source context retained by saved decks and parser feedback
- [x] Make signed-in `/api/parse` ephemeral by default; no per-parse opt-out flag needed
- [x] Document parse-session storage behavior directly in the parse UI, not only in docs

- These findings were identified during PRD review and stub implementation
- Items are organized by severity and implementation priority
- Check off items as they are completed
- Update this document as new findings emerge or priorities change

---

## 2026-04-29 — Consumer alpha execution plan

**This was the locked execution plan when the alpha was scoped. Most of
PR 1–6 has shipped (auth roles, frontend surface split, known
words/global cards, parse feedback subsystem, deck CRUD + review). PR 7
(ET evaluation parity) is partially shipped via Plan C / PRs
[#113](https://github.com/sagarinbabel/finnestdb/pull/113)/[#114](https://github.com/sagarinbabel/finnestdb/pull/114)/[#115](https://github.com/sagarinbabel/finnestdb/pull/115)
+ FST migration. PR 8 (Track B live quality metrics) and PR 9 (security
review) remain open. The full plan is preserved here for traceability
and is not actively re-litigated. See "What's not in main yet" above
for up-to-date open work.**

This is the locked execution plan for the FinEstDB consumer alpha. Where
this plan disagrees with older sections of `TODO.md`,
`finnestdb-prd-alpha.md`, `ARCHITECTURE.md`, or `docs/IMPLEMENTATION.md`,
this plan wins. Older sections remain for historical context but are not
re-litigated here.

Companion docs introduced alongside this plan:

- [docs/FEATURES.md](docs/FEATURES.md)
- [docs/CROSS_LANGUAGE_STRATEGY.md](docs/CROSS_LANGUAGE_STRATEGY.md)
- [docs/CHANGELOG.md](docs/CHANGELOG.md)

#### Summary

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

#### Product model

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

#### Implementation sequence

##### PR 1 — Planning and product docs

- Append this plan to `TODO.md` under a dated execution-plan section.
- Create `docs/FEATURES.md`, `docs/CHANGELOG.md`, and
  `docs/CROSS_LANGUAGE_STRATEGY.md`.
- Update live planning/architecture docs with dated headers and changelog
  references.
- `docs/FEATURES.md` is user-perspective and decision-complete about: what
  the product is, how users learn before reading, leverage and comprehension
  concepts, progress tracking concept, mobile web direction, and technology
  differentiators (fast parser, benchmarked quality, user-correction loop,
  inflected-form-aware frequency). Autoresearch belongs in the post-live idea
  parking lot, not the active alpha plan.

##### PR 2 — Auth roles and surface separation

- Extend the current mock-cookie auth into a role-aware alpha auth model.
- Add an admin flag to the user model and role-aware response behavior in
  `GET /api/me`.
- Split surfaces into anonymous, authenticated user, and admin-only.
- Restrict the current workbench in `web/index.html` and `web/app.ts` to
  admin access.
- Add a lightweight parse-inspection surface for logged-in users.
- Correction submission requires login. No anonymous full correction flow.

##### PR 3 — Frontend surface split

- Separate anonymous, user, and admin surfaces in the UI.
- Keep the existing frontend architecture in `web/app.ts`.
- Add: landing/product explanation, sign-in, deck list, rename/delete,
  review, known-word import/manage, lightweight parse inspection,
  admin-only workbench gating, admin feedback queue surface.
- Preserve one responsive app and existing breakpoints in `web/styles.css`.
- Validate mobile usability at 375 px.

##### PR 4 — Known words and global cards

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

##### PR 5 — Parse feedback subsystem

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

##### PR 6 — Deck CRUD and review flow

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

##### PR 7 — Track A evaluation parity (Estonian)

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

##### PR 8 — Track B live quality metrics

- Define production metrics sourced from parse usage plus accepted
  corrections.
- Minimum capture: parse id, user id, language, parser mode, token count,
  unique lemma count, correction submissions, accepted corrections.
- Minimum derived metrics: accepted correction rate per 1,000 tokens and
  per 1,000 unique lemmas, by language and by parser mode.
- Deliver first as a weekly admin report, not a polished analytics dashboard.
- Document Track B in `docs/EVAL_AND_CI.md` and
  `docs/PARSER_FEEDBACK_LOOP.md`.

##### PR 9 — Security review and hardening pass

- Scope: auth/session behavior, role enforcement on admin-only routes, CSRF
  posture for cookie-based auth, XSS exposure in feedback and parse views,
  rate limiting on login and feedback endpoints, data isolation between
  users, admin-route leakage to non-admins, correction submission abuse
  surface.
- Record findings and dispositions in `docs/SECURITY_REVIEW_ALPHA.md`.
- Fix any high-severity issues before stopping for merge review.

#### Parallel ownership split

- **Main backend owner**: PR-2 (Auth/Roles), PR-4 (Known Words + Global
  Cards), PR-5 (Parse Feedback), PR-6 (Deck CRUD + Review), PR-8 (Track B
  Reporting), PR-9 (Security Review).
- **Second model (parallel safe)**: PR-1 (Planning + Product Docs), PR-3
  (Frontend Surface Split, after PR-2 contract is fixed), PR-7 (ET
  Evaluation + Benchmark + Baselines).

High-conflict files where parallel edits must be avoided:

- `internal/api/handlers.go`
- `internal/store/db.go`

#### Public APIs

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

#### Documentation deliverables

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

#### Acceptance criteria

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

#### Assumptions

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
