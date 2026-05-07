# Project Decisions and Roadmap

_Current as of 2026-04-29 — see [CHANGELOG.md](CHANGELOG.md) for revisions._

> **Note (2026-04-29):** The consumer-alpha execution plan in
> [`../TODO.md`](../TODO.md) and the product framing in
> [`FEATURES.md`](FEATURES.md) take precedence over older decisions
> recorded here where they conflict. Older entries remain for historical
> context.

This document tracks key architectural decisions, the reasoning behind them, and the
project roadmap. It serves as a journal of how the project evolves and why we made
the choices we did.

---

## Product Vision

FinEstDB is a **JPDB clone for Finnish and Estonian**. The core user flow:

1. **Paste text** — User submits Finnish (or Estonian) text
2. **Parse perfectly** — System extracts lemmas, POS, definitions
3. **Review word list** — User sees all unique words with meanings
4. **Mark known/unknown** — User indicates which words they already know
5. **Create deck** — User saves unknown words as a study deck
6. **SRS study** — Spaced repetition moves words from "learning" to "known"
7. **Loop** — Next time user pastes text, known words are dimmed, focus is on new vocabulary

The value proposition: **pre-mine vocabulary before reading** so the reading experience
is more enjoyable and comprehension is higher.

---

## Decision 1: Build a Custom Parser (Not Use Omorfi Directly)

**Date:** 2026-04-28

### Context

Omorfi (Open Morphology of Finnish) is the gold standard for Finnish morphological
analysis. It uses finite-state transducers (FST) built over 15+ years of linguistic
work. The question: should we use Omorfi directly, or build our own parser?

### Decision

**Build our own custom parser**, using Omorfi as a quality benchmark.

### Reasoning

| Factor | Our Parser | Omorfi |
|--------|-----------|--------|
| **Speed** | ~40–50k words/s on FI on `custom`, single core ([baseline](baselines/2026-05-06-fi-summary.md#measured-throughput)) | FST traversal + Python subprocess startup; not yet benchmarked under our harness |
| **Deployment** | Self-contained Go binary | Requires Python + HFST + .hfst files |
| **Customization** | Add a rule in Go, redeploy | Fork FST project, recompile transducers |
| **Licensing** | Permissive (we control it) | GPL-3.0 (copyleft implications) |
| **Estonian support** | Same architecture, different rules | Would need separate tool (EstNLTK) |

### Trade-off Accepted

We accept that our parser may not match Omorfi's accuracy on edge
cases. The goal is **comparable lemma/POS accuracy with deployment
and licensing properties Omorfi can't give us** — speed is a
property we own, not the headline argument.

### How We Measure Success

- **Accuracy:** Compare lemma/POS output against gold-annotated test cases.
- **Speed:** `cmd/parsertest` reports per-case latency (avg / p50 / p95
  in ms, ns-precision under the hood) and aggregate `words/s` and
  `chars/s` per parser per dataset. Current floor on Finnish is
  ~40–50k words/s on `custom` against the 2026-05-06 dictionary state;
  treat anything below that on the same datasets as a regression to
  triage. Speed claims must always cite a measurement — comparing
  finnestdb against external baselines requires running both under the
  same harness, not eyeballing numbers from external papers.
- **Coverage:** Percentage of tokens resolved to dictionary entries.

> **Speed claims policy:** never quote a "we're faster than X" number
> in this repo without a `cmd/parsertest` run on a comparable dataset
> and a link to the JSON report. The 2026-05-06 timer fix (PR #103)
> exists because we previously couldn't.

### Omorfi's Role

Omorfi is used as:
- A **benchmark** to measure our parser's accuracy against
- A **tool for generating gold annotations** when building test cases
- Not a production runtime dependency

---

## Decision 2: Parser Architecture

**Date:** 2026-04-28

### Current Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│  Rust Parser (parser/src/lib.rs)                                │
│  - NFC normalization                                            │
│  - Sentence splitting                                           │
│  - Tokenization                                                 │
│  - Heuristic POS guessing                                       │
└─────────────────────────────────────────────────────────────────┘
                              │
                              │ FFI (JSON)
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  Go Parse Core (internal/parsecore/parsecore.go)                │
│  - Parser registry (basic, custom, omorfi)                      │
│  - Dictionary resolution                                        │
│  - Enrichment rules (possessive, compound, case suffix)         │
│  - Gloss lookup                                                 │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  SQLite Dictionary (internal/store/dict.go)                     │
│  - forms table: surface form → lemma + POS                      │
│  - lemmas table: lemma + POS → gloss                            │
│  - Source: kaikki.org (Wiktionary-derived)                      │
└─────────────────────────────────────────────────────────────────┘
```

### Parser Modes

| Mode | Description | Use Case |
|------|-------------|----------|
| `basic` | Dictionary lookup only, no enrichment | Speed baseline |
| `custom` | Dictionary + possessive/compound/case rules | Production parser |
| `omorfi` | External adapter for comparison | Evaluation only |

### Enrichment Rules (in `custom` mode)

1. **Possessive suffix stripping** (Finnish)
   - `kirjassani` → strip `-ni` → `kirjassa` → lookup → `kirja`

2. **Compound word splitting** (Finnish + Estonian)
   - `pankkiautomaatti` → `pankki` + `automaatti` → lookup both

3. **Case suffix stripping** (Finnish + Estonian)
   - `kirjassa` → strip `-ssa` → `kirja` + grammar label "inessive"

### Future Consideration: FST

We may eventually need FST-like tooling for **novel/unseen words** that aren't in
the dictionary. For now, the dictionary + heuristic rules cover most cases.

---

## Decision 3: Evaluation-Driven Development

**Date:** 2026-04-28

### Approach

Parser improvements are driven by **measured evaluation** against gold test cases,
not intuition.

### Infrastructure Built

- **Gold datasets:** `testdata/parser-eval/fi/gold/fi-manual-v1.json` (22 cases)
- **Eval CLI:** `cmd/parsertest` — runs parsers against datasets, outputs accuracy metrics
- **Metrics:** Lemma accuracy, POS accuracy, grammar label accuracy, coverage, timing

### Evaluation Workflow

```
1. Run eval:  go run ./cmd/parsertest -dataset fi-gold.json -parsers custom
2. Review failures: Which tokens did we get wrong?
3. Add rule: Fix the failure pattern
4. Re-run eval: Confirm improvement, no regressions
```

### Future: Automatic Improvement

Inspired by [karpathy/autoresearch](https://github.com/karpathy/autoresearch), we plan
to build an automated improvement loop:

```
1. Agent modifies parser rules
2. Run eval automatically
3. If accuracy improves: commit and continue
4. If accuracy drops: revert and try different approach
5. Repeat overnight → 100 experiments vs 5 manual attempts
```

This requires:
- Larger gold dataset (100+ sentences, currently 22)
- Consolidated rules in one file (clear edit scope for agent)
- Automated git workflow

---

## Project Roadmap

### Track A: Core Product (User-Facing)

| Phase | Work | Status |
|-------|------|--------|
| A1 | Parse Experience — results table polish, coverage gauge | Planned |
| A2 | Deck Creation — "Save as Deck" CTA from results | Planned |
| A3 | Known Words — mark known/ignored in results table | Planned |
| A4 | Navigation Shell — nav bar, dark theme alignment | Planned |
| A5 | SRS Core — review queue, card scheduling, session UI | Planned |
| A6 | Known Words Loop — SRS → known list → dims in future parses | Planned |
| A7 | Import Known Words — upload CSV of already-known vocabulary | Planned |

### Track B: Parser Quality (Development Infrastructure)

| Phase | Work | Status |
|-------|------|--------|
| B1 | Gold Data Expansion — 100+ annotated Finnish sentences | Planned |
| B2 | Baseline Benchmark — record current accuracy/speed | Planned |
| B3 | Rule Consolidation — all rules in one file | Planned |
| B4 | Omorfi Comparison — side-by-side accuracy measurement | Planned |
| B5 | Auto-Improvement Loop — autoresearch-style experiments | Planned |

### Track C: Estonian (Parallel Path)

| Phase | Work | Status |
|-------|------|--------|
| C1 | Estonian Gold Data — expand from 1 to 50+ cases | Planned |
| C2 | Estonian Dictionary — verify kaikki.org coverage | Planned |
| C3 | Estonian Rules — case suffixes, compounds | Planned |

### Execution Order

```
Priority 1: A1, A4      (UI foundation)
Priority 2: B1-B5       (Parser quality infrastructure)
Priority 3: C1, C2, C3  (Estonian support)
Priority 4: A2, A3      (Deck + known words)
Priority 5: A5-A7       (Full SRS loop)
```

---

## Decision 4: Parse Feedback Requires Login (v1)

**Date:** 2026-04-29

### Context

PR #53 introduces a parse-feedback flow: a user reviewing a parse result can flag
that the parser tagged a token incorrectly (wrong lemma/POS/grammar label) and
propose a correction. The endpoint that accepts this feedback (`POST
/api/parse/feedback`) needed an auth model.

### Decision

**Require login to submit parse feedback in v1.** No anonymous feedback path.

### Reasoning

- **Spam control.** Without an authenticated identity, the feedback queue is open
  to drive-by submissions with no rate-limit anchor and no way to hold a reporter
  accountable.
- **Admin signal-to-noise.** Admins reviewing the queue need to be able to
  weight reporters (returning user vs. one-time submitter). Anonymous breaks that.
- **Follow-up.** If a correction is ambiguous, admins need a way to ask the
  reporter for context. Anonymous feedback can't be followed up on.
- **Scope.** A "light anonymous feedback" path requires its own design (one-shot
  feedback tokens scoped to a parse session, separate rate limiting, separate
  admin UI). Out of scope for alpha.

### Trade-off Accepted

Some users will hit a parser bug, want to flag it, and won't bother creating an
account to do so — that signal is lost. We accept this for v1 in exchange for a
clean, low-noise feedback queue.

### Related Source-Text Retention Decision

The original product call here was **Option B**:

- do not persist parse sessions during `/api/parse`
- persist parse-session context only when a logged-in user explicitly submits
  feedback
- treat feedback submission as the consent boundary for storing source text

Rationale from a user perspective:

- The parse UI feels ephemeral; users will paste personal/sensitive content
  (private messages, work documents, copyrighted material) without expecting it
  to be stored.
- Storing only on feedback-submit aligns persistence with consent: the user has
  actively asked us to look at this parse, so retaining the context is
  justified.
- Eliminates unbounded growth from anonymous parse traffic.

### Amendment (2026-04-30): alpha shipped as Option A

Alpha shipped as **Option A** instead:

- authenticated `/api/parse` calls create `parse_sessions` rows
- anonymous `/api/parse` calls do **not** create `parse_sessions` rows
- anonymous parses do **not** persist source text
- parse feedback still requires login and still references a server-issued
  `parse_id`

We accepted the deviation from the original Option B decision for alpha because
it:

- solves the immediate unbounded-growth concern from anonymous parse traffic
- keeps the frontend and backend contract simpler
- preserves a clean path to a future parse-history UI for logged-in users

### Trade-off Accepted

This leaves a real privacy gap for alpha:

> Logged-in users have their pasted text stored automatically during Inspect,
> without a separate per-paste consent moment.

That gap is accepted for alpha only. It will be closed with:

- a parse-history UI
- per-user delete controls for stored parse sessions
- an opt-in ephemeral parse mode for logged-in users

### Post-v1 Reconsideration

If parser-quality work outgrows the volunteer feedback signal, revisit anonymous
"light feedback" as a separate, rate-limited path with its own queue.

---

## Decision 5: Don't Extend the Case-Suffix Table; Generated Morphology Tables Are the Real Answer

**Date:** 2026-05-06

### Context

`internal/parserules/{finnish,estonian}.go` defines suffix→case-label tables
(15 Finnish entries, 17 Estonian entries) used by
`internal/store/dict.go::tryCaseSuffixStrip`. The matcher strips a suffix off
the surface form, looks up the residual stem in the `lemmas` table, and on
hit returns `(lemma, pos, grammar_label)`.

The natural-feeling reaction to the 0% grammar-accuracy result on
`grammar_label` (see `docs/baselines/2026-05-06b-summary.md`) is to grow this
table — add more suffix entries, encode consonant gradation, handle ternary
compounds, etc. Existing TODO items #15 (three-part compound splitting) and
#16 (consonant gradation rules) point in that direction. We are choosing
**not** to.

### Decision

**Freeze the case-suffix table at its current size.** Further morphology
investment goes into generated factual morphology tables under
[`pkg/lemmatizer-fi-et/tables/`](../pkg/lemmatizer-fi-et/tables/) and the
offline generator/reader code that can reproduce them from local upstream
analysers. Per [docs/ARTIFACT_POLICY.md](ARTIFACT_POLICY.md), upstream
transducer blobs are local-only and are not committed.

Two near-term exceptions are in scope:

1. **The stopgap label-attach pass on dict hits**
   (`attachCaseLabelIfStemMatches` in `internal/store/dict.go`). Lifts grammar
   accuracy off zero on tokens whose stem doesn't change under inflection.
   Explicitly stopgap; removed once production generated tables emit FEATS
   for direct hits.
2. **Bug fixes** to existing entries if a wrong label is being attached.

### Reasoning

Suffix-stripping is the wrong *shape* of operation for Finnish/Estonian
morphology. Five reasons, each grounded in real tokens from our gold sets:

1. **Stem alternation can't be expressed by end-of-string rules.**
   `toas → tuba` (et-grammar-v1, inessive of "room"): suffix-strip removes
   `s`, leaves `toa`, but the lemma is `tuba` — `o ↔ u` flips inside the
   stem with consonant alternation. A suffix table operates only on the
   suffix; it has no way to encode "after stripping `s`, also rewrite
   `oa → uba` for stems in grade-alternation class III." Encoding that is
   reimplementing an FST with worse abstractions. Same with `Naabri →
   naaber` (epenthetic vowel), `linnas → linn` (final-`a` deletion after
   `s`-stripping), `majja → maja` (gemination + `a`-insertion produced
   the illative; reverse isn't a suffix strip at all).

2. **Suffix-shaped lemmas trigger false positives.** `aas` (meadow), `mees`
   (man), `loss` (castle) all end in `s`. Stripping `s` gives `aa` / `mee` /
   `los` — none of which are lemmas. The table has no way to know which
   `-s` is paradigmatic and which is part of the lemma. An FST knows
   because it has the lexicon and the inflectional paradigm together.

3. **Genuine ambiguity needs a candidate set, not a single answer.**
   `linnas` is `Case=Ine|Number=Sing` of `linn` ("in the city") AND, in
   some readings, `Case=Gen|Number=Sing` of a personal name `linnas`. A
   suffix table emits one tuple `(lemma, label)`; the alternative is
   discarded. Real morphology produces a candidate set and lets the
   disambiguator pick. `pickBestFormCandidate` already exists for direct
   dict — case-suffix-strip output should be folded into the same ranker
   in the FST world, not the suffix world.

4. **Compound interaction.** Estonian compounds extensively. Suffix-strip
   over-fires on compounds: `linnasüda` ("city heart") ending in
   suffix-shaped `a` parses as essive of a fake lemma. Compounds need to
   be split *before* suffix logic, and the split needs paradigm-class
   awareness — FST territory.

5. **We are already building a table-backed morphology path.** PRs
   [#107](https://github.com/sagarinbabel/finnestdb/pull/107),
   [#108](https://github.com/sagarinbabel/finnestdb/pull/108), and
   [#110](https://github.com/sagarinbabel/finnestdb/pull/110) add the
   generated-table runtime and offline analyser readers. The suffix table
   should remain fallback code while production tables are generated.

### Trade-off Accepted

The stopgap will not produce grammar labels for stem-alternating forms
(`toas`, `Naabri`, `linnas`-as-inessive). That's acceptable because:

- Stem-alternating forms are exactly what generated analyser-derived
  tables are meant to cover;
- The existing 15+17 entries are sufficient to lift grammar accuracy off
  zero on the easy majority case (Finnish has cleaner suffixation than
  Estonian; Estonian's harder cases were always going to need the FST).

### What This Closes

- **TODO #15** (three-part compound splitting) — DEFER to FST migration.
  The VFST handles compounds natively via concatenated `[Xp]...[X]`
  segments; see `pkg/lemmatizer-fi-et/voikkomap/` parser in PR #107.
- **TODO #16** (consonant gradation rules in suffix-strip) — REJECT.
  Gradation lives in the FST's lexicon-aware paradigm tables, not in
  string-rewrite rules over the surface.

Both items are restated in `TODO.md` under the
"FST migration supersedes" section.

### How to Revisit

If the FST migration stalls or is reversed, this decision should be
re-litigated. Until then, PRs that add suffix-table entries or implement
gradation/ternary-compound logic in `internal/parserules/` or
`internal/store/dict.go` should be redirected to
`pkg/lemmatizer-fi-et/` instead.

---

## Decision 6: Numeric-Hyphen Tokenization Lives in the Shared Tokenizer

**Date:** 2026-05-06

### Context

A user pasted Estonian text containing `65-aastane` ("65-year-old") into the
parser during manual testing and noticed neither `65` nor `aastane` showed up
as separate words. Pure numbers like `65` weren't tagged `NUM` either. The
same construction is just as productive in Finnish (`65-vuotias`,
`1990-luvulla`, etc.), and the tokenizer at
[`parser/src/lib.rs:308`](../parser/src/lib.rs:308) takes an unused `_lang`
parameter — Finnish was guaranteed to have the identical bug.

### Decision

Fix this in the shared Rust tokenizer with four pure-tokenizer rules. Do **not**
add per-language entries to
[`internal/parserules/finnish.go`](../internal/parserules/finnish.go) or
[`internal/parserules/estonian.go`](../internal/parserules/estonian.go).

- **R1** — split a chunk at the first hyphen where one side is pure digits and
  the other starts with a letter (`65-aastane`, `65-vuotias`, `1990-luvulla`).
  Skip mixed-prefix abbreviations (`B1-tase`, `well-known`).
- **R2** — `guess_pos` returns `NUM` for `^\d+$`, `^\d+\.\d+$`,
  `^\d+,\d+$`, with internal whitespace stripped.
- **R3** — post-pass that merges `\d{1,3}( \d{3})+` runs into one NUM token.
  Form keeps spaces (`"250 000"`); lemma drops them (`"250000"`) so SI-spaced
  and unspaced numbers group as one entry in the words list.
- **R4** — split a chunk at the only hyphen if both sides are pure digits
  (`1990-2020`). Multi-hyphen forms (ISO dates `2026-05-06`) stay whole because
  R4 requires exactly one hyphen.

### Reasoning

[`docs/CROSS_LANGUAGE_STRATEGY.md`](CROSS_LANGUAGE_STRATEGY.md) lists
*tokenization or sentence-split error* and *compound segmentation error* as
shared error categories, and prescribes investing in shared infrastructure
when one language surfaces a problem the other has too. This is exactly that
case: the four rules are language-agnostic (digits, letters, hyphens, spaces
are universals across FI/ET) and any per-language inflection of the freed
stems (`aastane`, `vuotias`, `luku`) is handled by the existing language-
specific lookup chain. Splitting the tokenizer fix into two per-language
implementations would have been duplicate work with a high risk of drift.

### Trade-off Accepted

- Conservative R4 (one-hyphen-only) leaves ISO dates whole as a single
  unresolved NOUN literal, which is the same as the pre-fix state for those
  forms (no regression). A dedicated date detector can layer on later.
- ET `65-aastast` (partitive of `65-aastane`) splits cleanly via R1 but
  `aastast` doesn't lemmatize back to `aastane` without a `-ne` ADJ
  inflection table — separate piece of work.
- Negative numbers like `-5` stay as one token. Acceptable for alpha;
  negation is usually written `miinus 5` in both languages.

### How We Measure Success

- 13 new Rust unit tests added; full suite: 41 passed, 0 failed.
- Zero regression on all 6 existing gold datasets (et-grammar-v1,
  et-manual-v1, fi-core-v1, fi-manual-v1, fi-manual-v2, fi-grammar-v1) at the
  Phase-2 baseline DB. Numbers identical to
  [2026-05-06b](baselines/2026-05-06b-summary.md).
- A direct probe of 13 sentences across both languages (5 FI + 5 ET fix
  cases + 3 regression cases) confirms R1–R4 produce the intended tokens
and freed stems (`aastane`, `luku`) hit the dictionary cleanly. Full
trace in
[`docs/qa-reports/2026-05-06-numeric-hyphen-tokenization.md`](qa-reports/2026-05-06-numeric-hyphen-tokenization.md).

---

## Open Questions

1. **FST for novel words:** At what accuracy level do we need FST-like morphological
   analysis for unseen words? Current heuristics may plateau at ~95%.

2. **Gold data source:** Should we use Omorfi to generate candidate annotations, then
   human-verify? Or fully manual annotation?

3. **Auto-improvement scope:** Which files should the agent be allowed to modify?
   Just rules? Or also the Rust tokenizer?

## Changelog

| Date | Change |
|------|--------|
| 2026-04-28 | Initial decisions documented: custom parser rationale, architecture, evaluation approach, roadmap |
| 2026-04-29 | Decision 4 added: parse feedback requires login in v1; source_text persisted only on feedback submit |
| 2026-04-30 | Recorded parse-feedback persistence amendment: alpha ships authenticated parse-session storage as Option A |
| 2026-05-06 | Decision 5 added: freeze the case-suffix table; further morphology work goes into generated morphology tables under `pkg/lemmatizer-fi-et/tables/` |
| 2026-05-06 | Decision 6 added: numeric-hyphen tokenization (R1–R4) lives in the shared Rust tokenizer, no per-language rule tables |
