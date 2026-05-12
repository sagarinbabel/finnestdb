# Documentation Index

_Created 2026-05-07 PM; refreshed 2026-05-12. Single map of every doc in
this repo, organized by purpose. Designed to be cold-readable: any reader
(human or LLM) should be able to find the right doc for their question in
under a minute._

If you're new to the repo, read in this order: [`../README.md`](../README.md) →
[`../TODO.md`](../TODO.md) → [`../ARCHITECTURE.md`](../ARCHITECTURE.md) →
this index for everything else.

## LLM guardrail

`cmd/autoresearch` and [`docs/AUTORESEARCH.md`](AUTORESEARCH.md) are parked
post-live ideas. Future agents should ignore autoresearch as current work unless
the user explicitly asks for it in the current turn. Do not block parser,
evaluation, or product changes on autoresearch behavior.

## Quick reference

Compact one-liner per doc, by purpose. Use this when you need to pick a
doc in 30 seconds; use ["By purpose"](#by-purpose) below for the longer
descriptions when two docs sound similar.

- **Entry point:** [`../README.md`](../README.md) (what the project is, how to run, project structure, doc index)
- **Setup verifier:** [`make doctor`](../cmd/doctor/main.go) plus [`docs/LOCAL_TOOLING.md`](LOCAL_TOOLING.md) — local tool/artifact inventory, canonical paths, and fallback lookup rules
- **System architecture:** [`../ARCHITECTURE.md`](../ARCHITECTURE.md) (current), [`docs/LEXICAL_PLAN.md`](LEXICAL_PLAN.md) (lexical-layer detail), [`corpus_pipeline/docs/CORPUS_PIPELINE.md`](../corpus_pipeline/docs/CORPUS_PIPELINE.md) (offline corpus pipeline)
- **Product framing:** [`docs/FEATURES.md`](FEATURES.md) (user-facing), [`docs/DESIGN_REVIEW.md`](DESIGN_REVIEW.md) (design folder audit + TODO)
- **What's done / what's next:** [`../TODO.md`](../TODO.md) (the only doc you need for status)
- **Why we made choices:** [`docs/DECISIONS.md`](DECISIONS.md) (latest-first, 21 decisions)
- **What changed when:** [`docs/CHANGELOG.md`](CHANGELOG.md) (latest-first, cross-linked to DECISIONS)
- **Measured parser quality:** [`docs/PARSER_EVOLUTION.md`](PARSER_EVOLUTION.md) (chronological log)
- **ML roadmap:** [`docs/ML_IDEAS.md`](ML_IDEAS.md)
- **Calibration data:** [`docs/FREQUENCY_BASELINES.md`](FREQUENCY_BASELINES.md), [`docs/data_enhancement.md`](data_enhancement.md)
- **Specialized:** [`docs/PARSER_FEEDBACK_LOOP.md`](PARSER_FEEDBACK_LOOP.md), [`docs/ARTIFACT_POLICY.md`](ARTIFACT_POLICY.md), [`docs/PARSER_EVAL_METHODOLOGY.md`](PARSER_EVAL_METHODOLOGY.md), [`docs/SYSTEM_VERSIONING.md`](SYSTEM_VERSIONING.md), [`docs/srs-deck-spec.md`](srs-deck-spec.md), [`docs/CROSS_LANGUAGE_STRATEGY.md`](CROSS_LANGUAGE_STRATEGY.md)

## At a glance

| If you want to know... | Go to |
|---|---|
| What this project is and how to run it | [`README.md`](../README.md) |
| What's shipped and what's open | [`TODO.md`](../TODO.md) |
| Why we made the choices we did | [`docs/DECISIONS.md`](DECISIONS.md) |
| What changed when (per-PR doc impact) | [`docs/CHANGELOG.md`](CHANGELOG.md) |
| How the system is wired together | [`ARCHITECTURE.md`](../ARCHITECTURE.md) |
| Whether local analyzers, models, DBs, or FST tables are installed | [`make doctor`](../cmd/doctor/main.go) and [`docs/LOCAL_TOOLING.md`](LOCAL_TOOLING.md) |
| How the lexical layer works | [`docs/LEXICAL_PLAN.md`](LEXICAL_PLAN.md) |
| How corpora become wordlists, sentence banks, and mining files | [`corpus_pipeline/docs/CORPUS_PIPELINE.md`](../corpus_pipeline/docs/CORPUS_PIPELINE.md) |
| What the product is from a learner's view | [`docs/FEATURES.md`](FEATURES.md) |
| Design audit + open TODO | [`docs/DESIGN_REVIEW.md`](DESIGN_REVIEW.md) |
| How parser quality has moved over time | [`docs/PARSER_EVOLUTION.md`](PARSER_EVOLUTION.md) |
| Why FI and ET diverge measurably | [`docs/CROSS_LANGUAGE_STRATEGY.md`](CROSS_LANGUAGE_STRATEGY.md) |
| What ML directions fit the project | [`docs/ML_IDEAS.md`](ML_IDEAS.md) |

## By purpose

### Entry point

- [`../README.md`](../README.md) — what FinEstDB is, how to run it, project
  structure, build instructions, browser regression tests, dictionary import,
  parser evaluation CLI, known limitations, documentation index pointer.

### System architecture (current state)

- [`../ARCHITECTURE.md`](../ARCHITECTURE.md) — system architecture, layer
  responsibilities, data flow, parser modes (browser vs eval-only),
  baselines summary. Updated with each parser-affecting PR.
- [`docs/LOCAL_TOOLING.md`](LOCAL_TOOLING.md) — local-only tool and artifact
  inventory: `make doctor`, `.venv/`, omorfi, estnltk, Voikko, Giellalt
  HFSTOL, FST tables, `finnestdb.db`, `localdata/`, and legacy fallback
  paths agents must check before claiming something is missing.
- [`docs/LEXICAL_PLAN.md`](LEXICAL_PLAN.md) — lexical layer architecture:
  schema, source priority semantics, multi-lemma surface forms,
  FST↔dict boundary, importer pattern, FI source choices, ET source
  choices (consolidated from the deleted `ESTONIAN_LEXICAL_PLAN.md`).
- [`corpus_pipeline/docs/CORPUS_PIPELINE.md`](../corpus_pipeline/docs/CORPUS_PIPELINE.md) —
  operator manual for the tracked offline corpus pipeline: fetch,
  extract, aggregate, verify, promote, enrich, EPUB deck export, and
  gloss-coverage audit. Bulk corpus data stays under `localdata/`.
- [`architecture.md`](architecture.md) — redirect stub pointing at
  `ARCHITECTURE.md` (kept so old links resolve).

### Product framing (user-facing)

- [`docs/FEATURES.md`](FEATURES.md) — what the product is from a
  learner's perspective. Inspect → correct → deck → review loop.
  Technology differentiators (fast parser, benchmarked quality, user
  correction loop, inflected-form-aware frequency). Notes about
  autoresearch are post-live ideas, not current roadmap.

- [`docs/USER_FLOWS.md`](USER_FLOWS.md) — screen-by-screen consumer
  alpha spec with ASCII wireframes. Anonymous landing → sign-up hook
  → dashboard → parse → results → save/add-to-existing deck → review.
  Includes the recommended correction-flow design (flag-only path,
  ✎ entry point) and the open questions on translation, register
  picker, and cold-start.

- [`docs/DESIGN_REVIEW.md`](DESIGN_REVIEW.md) — audit of the `design/`
  folder: branding, Aalto app system, wireframe clickthrough, view
  components, mobile prototype, and flow diagram. Includes a prioritized
  TODO list (P0–P3) covering the leverage → word-list → study-deck flow,
  correction round-trip, and design-system convergence.

- [`docs/DESIGN_AI_PROMPTS.md`](DESIGN_AI_PROMPTS.md) — system prompt
  + per-screen prompt templates for handing FinEstDB screens to v0,
  Lovable, Bolt, Cursor, or Figma Make without losing the existing
  type/colour system.

### Status and planning

- [`../TODO.md`](../TODO.md) — single repo-level task list. **What's in
  main / What's not in main yet / Open PRs / Research Goals / Notes
  & historical**. The first thing to read if you want to do work.
- [`docs/DECISIONS.md`](DECISIONS.md) — decisions log, latest-first.
  21 entries with date, context, decision, reasoning, trade-off,
  how-to-revisit. Cross-linked to CHANGELOG where the same event
  appears in both.
- [`docs/CHANGELOG.md`](CHANGELOG.md) — documentation changelog,
  reverse-chronological. Tracks doc-level changes per PR, cross-linked
  to DECISIONS.

### Parser quality and measurement

- [`docs/PARSER_EVOLUTION.md`](PARSER_EVOLUTION.md) — chronological log
  of measured parser-quality changes. Trend table at top, dated entries
  below (one per parser-affecting PR). The "did we improve?" record.
- [`docs/PARSER_EVAL_METHODOLOGY.md`](PARSER_EVAL_METHODOLOGY.md) —
  how `make compare-parsers` works: what's measured, parsers under
  comparison, schema details, baseline-freezing convention.
- [`docs/PARSER_EVAL_DATASETS.md`](PARSER_EVAL_DATASETS.md) — gold
  dataset structure, how new cases are annotated, how draft sets get
  promoted.
- [`docs/baselines/`](baselines/) — frozen per-PR eval reports (FI +
  ET, compressed JSON + summary markdown). Latest logical reference set
  is `2026-05-07k-T1118Z`.
  See [`docs/baselines/README.md`](baselines/README.md) for the
  baseline schema.
- [`docs/SYSTEM_VERSIONING.md`](SYSTEM_VERSIONING.md) — how parser
  behavior, eval baselines, deck/review scheduler, API contract, and
  data schema are versioned independently.
- [`docs/OMORFI_ADAPTER.md`](OMORFI_ADAPTER.md) — Finnish external
  baseline integration (Helsinki HFST → Python adapter → FFI).
- [`docs/OMORFI_COMPARISON.md`](OMORFI_COMPARISON.md) — methodology
  for comparing custom parser vs Omorfi.
- [`docs/srs-deck-spec.md`](srs-deck-spec.md) — spaced-repetition deck
  spec: card lifecycle, scheduling math, coverage metrics,
  comprehension prediction formulas.

### Cross-language strategy

- [`docs/CROSS_LANGUAGE_STRATEGY.md`](CROSS_LANGUAGE_STRATEGY.md) —
  what's shared between FI and ET (pipeline shape, eval harness, error
  taxonomy) vs language-specific (morphology rules, lexicon,
  disambiguation). Includes "Measurable Divergences" with
  register-vs-language coverage findings.

### Roadmap and exploration

- [`docs/ML_IDEAS.md`](ML_IDEAS.md) — ML roadmap: word-level
  disambiguator (CRF), neural lemmatizer for unknown words,
  knowledge-distillation, fastText embeddings, user-text-aggregated
  frequency, comprehension prediction, sentence-level cached
  translations. Confidence levels per item.
- [`docs/ideas.md`](ideas.md) — exploratory roadmap including
  AI-native phasing.
- [`docs/AUTORESEARCH.md`](AUTORESEARCH.md) — parked post-live idea for
  automated rule-ablation loops. Ignore for current work unless the user
  explicitly asks about autoresearch.

### Specialized infrastructure

- [`docs/ARTIFACT_POLICY.md`](ARTIFACT_POLICY.md) — what is and isn't
  allowed in git. Single-folder bootstrap rule (everything gitignored
  goes under `localdata/`). No transducer blobs. No bulk corpora.
  Generator/fetcher code only.
- [`docs/data_enhancement.md`](data_enhancement.md) — single-source-of-
  truth ledger of every external corpus the project pulls in (gold,
  silver, dictionary, treebank cache, frequency baselines). Each row:
  source URL, license, size, path, added date, last-refreshed date.
- [`docs/FREQUENCY_BASELINES.md`](FREQUENCY_BASELINES.md) —
  methodology and license attribution for the public Finnish/Estonian
  frequency baselines (OpenSubtitles 2018 + UD treebanks). Used as
  comparison anchors for the user-text-aggregated frequency work.
- [`docs/FST_LEMMATIZER.md`](FST_LEMMATIZER.md) — change document for
  the FST lemmatizer migration (PRs #106–#112) that produced the
  `pkg/lemmatizer-fi-et/` runtime.
- [`docs/FST_LEMMATIZER_ROADMAP.md`](FST_LEMMATIZER_ROADMAP.md) —
  per-PR sequencing of the FST migration work.
- [`docs/PARSER_FEEDBACK_LOOP.md`](PARSER_FEEDBACK_LOOP.md) — UX and
  schema for "Suggest fix" parse-feedback flow. Admin queue triage,
  always-attach-parse-session rule, planned writeback to `custom_overrides`
  lexical rows.
- [`../corpus_pipeline/docs/CORPUS_PIPELINE.md`](../corpus_pipeline/docs/CORPUS_PIPELINE.md) —
  corpus operations manual and architecture map for public-source corpus
  artifacts under `localdata/{fi,et}-corpus/`.

### Process and operations

- [`docs/GETTING_STARTED.md`](GETTING_STARTED.md) — first-run guide
  for trying the parser interactively.
- [`docs/GO_LIVE_CHECKLIST.md`](GO_LIVE_CHECKLIST.md) — security and
  hardening posture required before exposing the alpha publicly.
- [`docs/LEARNINGS.md`](LEARNINGS.md) — parser-eval learnings as
  patterns we want to repeat (or avoid).

### Historical / superseded

- [`docs/IMPLEMENTATION.md`](IMPLEMENTATION.md) — redirect stub.
  Original content split: "Suggest fix" UX → `PARSER_FEEDBACK_LOOP.md`,
  build instructions → README.md, limitations → README.md.
- [`../IMPLEMENTATION_ANALYSIS.md`](../IMPLEMENTATION_ANALYSIS.md) —
  March 2026 pre-implementation analysis. Recommends Postgres + Python,
  which the project did not adopt. Banner directs readers to
  ARCHITECTURE.md instead.
- [`../finnestdb-prd-alpha.md`](../finnestdb-prd-alpha.md) — original
  product requirements document. Kept as historical reference for the
  initial product vision; the consumer-alpha plan in
  [`../TODO.md`](../TODO.md) supersedes it where they conflict.
- [`../ALTERNATIVE_NAMES.md`](../ALTERNATIVE_NAMES.md) — exploratory
  naming brainstorm from March 2026. Not authoritative.

### Reports and one-off artifacts (not living docs)

- [`docs/qa-reports/`](qa-reports/) — dated QA reports (E2E doc
  behavior, numeric-hyphen tokenization regression check). Each is a
  one-off; safe to read for history but not maintained. Latest doc-state
  audit: [`2026-05-08T2229Z-doc-architecture-corpus-audit.md`](qa-reports/2026-05-08T2229Z-doc-architecture-corpus-audit.md).
- [`docs/baselines/`](baselines/) — frozen eval reports per PR (see
  Parser quality section above).
- [`../experiments/`](../experiments/) — dated spike reports and
  research plans. Current entries:
  - [`2026-05-06-phase3.5-voikko-generator-spike.md`](../experiments/2026-05-06-phase3.5-voikko-generator-spike.md)
    — phase 3.5 Voikko generator spike.
  - [`2026-05-07-top-1000-inflected-forms.md`](../experiments/2026-05-07-top-1000-inflected-forms.md)
    — research plan for a static, register-aware top-1000
    inflected-form table per language and the cold-start "Start with
    the top 1000" seed deck.
  - Plus two `2026-04-28-autoresearch-fi-manual-*.jsonl` runs from
    `cmd/autoresearch`.
- [`testdata/parser-eval/`](../testdata/parser-eval/) — gold dataset
  files plus per-language `notes/annotation-notes.md`.
- [`pkg/lemmatizer-fi-et/data/{fi,et}/README.md`](../pkg/lemmatizer-fi-et/data/)
  — license stubs and provenance for upstream Voikko / Giellalt /
  HFST sources used by the local table generator.

## Doc cross-reference graph (reading paths)

For an LLM building a project model from cold:

1. **Project model**: README → ARCHITECTURE → LEXICAL_PLAN → CROSS_LANGUAGE_STRATEGY
2. **Status / what to do**: README → TODO (and that's enough; TODO links out everywhere it needs to)
3. **History / "what happened?"**: CHANGELOG → DECISIONS (latest-first, cross-linked) → PARSER_EVOLUTION
4. **Parser quality**: PARSER_EVAL_METHODOLOGY → PARSER_EVOLUTION → docs/baselines/
5. **Architecture deep-dive**: ARCHITECTURE → LEXICAL_PLAN → ARTIFACT_POLICY → data_enhancement
6. **Roadmap / future**: TODO Research Goals → ML_IDEAS → ideas
7. **Product / UX**: FEATURES → PARSER_FEEDBACK_LOOP → srs-deck-spec → DESIGN_REVIEW (open design TODO)

Autoresearch exception: even though `docs/AUTORESEARCH.md` exists, it is not
part of the active roadmap. Treat it as an idea parking lot until the app is
shipped and live.

For a human starting from the URL: README is the entry point, this index
is the second-tier map, every doc has a one-line purpose statement
above so you can skim before opening it.

## Conventions

- **Dates** at top of doc (`_Current as of YYYY-MM-DD_`) tell you when
  the doc was last verified against code state.
- **CHANGELOG vs DECISIONS overlap**: CHANGELOG records what changed;
  DECISIONS records why. Where the same event appears in both, both
  cross-link.
- **Status markers**: items in TODO marked `[x]` are shipped, `[ ]` are
  open. Items marked `~~strikethrough~~` are superseded — see
  [`docs/DECISIONS.md`](DECISIONS.md) for the reasoning.
- **Local-only paths** (under `localdata/`) are gitignored; treat any
  doc reference to `localdata/foo` as "you must populate this locally
  via `scripts/setup-local.sh`."
- **PR references** (`#NNN`) link to GitHub. Closed-without-merging
  PRs are noted explicitly when relevant; see DECISIONS.md.
