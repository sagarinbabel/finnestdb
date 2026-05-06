# Documentation Changelog

This file tracks notable changes to FinEstDB planning, architecture, and
product documentation. Code changes belong in git history, not here.

Entries are reverse-chronological. Each entry links to the docs it
introduced or modified so the docs index stays navigable.

## 2026-05-07 — 3-column comparison reports + bootstrap CIs (Plan C / PR 2)

Restructures `cmd/parser-compare` so committed comparison reports answer the
right question by default: "did *our* parser regress against the analyzer
upper bound?" Three-column headline (custom-prev / custom-now / analyzer)
replaces the legacy "every parser side-by-side" framing, with case-level
bootstrap CIs so 22-case noise can no longer be misread as signal.

- Added `-baseline-dir` flag to [`cmd/parser-compare`](../cmd/parser-compare/main.go).
  When set, each "now" report is paired by dataset name with a prior report
  in that directory; the headline becomes `(custom-prev, custom-now, Δ,
  analyzer)`. Without `-baseline-dir` the legacy table is the only output
  (back-compat).
- Added `-bootstrap N` flag (default 1000). Each accuracy cell shows
  `82.3% ±0.4` — half the 95% case-level bootstrap CI width. Set
  `-bootstrap 0` to disable. Deterministic seed by default so committed
  reports diff cleanly.
- Added `-main-parser` flag (default `custom`) to control which parser is
  treated as "now" in the headline.
- Legacy "all parsers" table moves to an appendix when `-baseline-dir` is
  set; remains the default output otherwise.
- 4 unit tests covering the per-case stats extractor, bootstrap
  half-width on uniform vs heterogeneous accuracy, and analyzer-parser
  detection.

**Why now:** the eval harness changes in
[#109](https://github.com/sagarinbabel/finnestdb/pull/109) and
[#113](https://github.com/sagarinbabel/finnestdb/pull/113) gave us reliable
gold + always-present analyzer columns. The remaining gap was the report
structure itself: today's reports compare basic-vs-custom head-to-head, but
the meaningful comparison is custom-prev vs custom-now (did we improve)
against the analyzer (how far is the upper bound). Bootstrap CIs make it
honest — a 2.2pp gain on 22 cases stops being headline-worthy.

**FST migration link:** the per-attribute eval planned alongside the FST
runtime ([PRs #106](https://github.com/sagarinbabel/finnestdb/pull/106) /
[#107](https://github.com/sagarinbabel/finnestdb/pull/107)) will reuse the
same `-baseline-dir` machinery — gold case files already carry a `feats`
field after PR #113, so the per-attribute extension is purely on the
report side.

## 2026-05-06c — UD treebank gold expansion (Plan C / PR 1)

Lifts the parser-eval gold set from ~166 cases to ~14k cases (committed
FI) / ~22k cases (FI committed + ET local) by ingesting the published
Universal Dependencies treebanks for Finnish and Estonian.

- Added [`cmd/importud`](../cmd/importud/main.go): pure-Go CoNLL-U →
  parser-eval gold JSON converter. Skips MWT range rows and elliptical
  nodes; preserves full UD FEATS string in a new `feats` field on each
  token (forward-compat for the planned per-attribute eval); projects
  `Case=Xxx` into the legacy `grammar_label` field for back-compat with
  the existing case-only metric.
- Added [`scripts/fetch-and-import-ud.sh`](../scripts/fetch-and-import-ud.sh):
  clones each UD treebank under `data/ud-cache/` (gitignored) and runs
  the importer over each train/dev/test split.
- Added Makefile targets `make import-ud-gold-fi`, `make
  import-ud-gold-et`, `make import-ud-gold` (both).
- New committed FI gold (CC BY / CC BY-SA): ~9.8k cases / ~86k tokens
  across UD-Finnish-TDT/FTB/PUD/OOD test+dev splits.
- New local-only ET gold (CC BY-NC-SA — gitignored under
  `testdata/parser-eval/et/gold/ud-et-*.json`): ~8k cases / ~115k
  tokens across UD-Estonian-EDT/EWT test+dev.
- Train splits go under `testdata/parser-eval/{fi,et}/gold-train/`
  (gitignored) so headline `make compare-parsers` runs don't get
  bloated by 30k-sentence files. Used for OOV/coverage analysis with
  explicit `-dataset` flags.
- Held-out discipline: [`scripts/parser-comparison.sh`](../scripts/parser-comparison.sh)
  and [`scripts/parser-comparison-et.sh`](../scripts/parser-comparison-et.sh)
  default discovery now excludes `*-dev-v*` files. Test sets are the
  held-out anchor; dev is for per-commit watching (run explicitly with
  `-dataset`).
- [`ARCHITECTURE.md`](../ARCHITECTURE.md) §Evaluation Stack updated with
  per-treebank table, license info, and held-out workflow.

**Why now:** the old gold was 22 cases on fi-manual-v1, 4 on
et-manual-v1. Any number computed on a 22-case set is one bad sentence
away from a 4.5pp swing. UD gives us train/dev/test splits with
human-checked morphology; we pay nothing to use them.

**FST migration link:** still on the roadmap — see PRs
[#106](https://github.com/sagarinbabel/finnestdb/pull/106) /
[#107](https://github.com/sagarinbabel/finnestdb/pull/107). The expanded
gold makes that migration's regression checks meaningful (a 3pp lemma
gain on 22 cases is noise; on 86k tokens it's signal).

## 2026-05-06b — Eval harness parity + grammar-label stopgap

Two changes to the parser-evaluation pipeline, plus a recorded decision on
how *not* to fix grammar accuracy.

- **Always benchmark against the analyzer baseline.**
  [`scripts/parser-comparison.sh`](../scripts/parser-comparison.sh) now
  *requires* omorfi for Finnish; [`scripts/parser-comparison-et.sh`](../scripts/parser-comparison-et.sh)
  requires estnltk/Vabamorf for Estonian. Both fail with `exit 2` and a
  setup hint when the analyzer is missing. A `--allow-missing-baseline`
  flag remains for ad-hoc local experiments — committed reports must
  include the analyzer column.
  - Why: dict-only basic/custom numbers were being read in isolation,
    masking that grammar accuracy was 0% across all FI and ET datasets in
    [`docs/baselines/2026-05-06b-summary.md`](baselines/2026-05-06b-summary.md).
    The analyzer column is the upper bound; without it there is no way to
    tell whether 88% lemma is "good enough" or a regression. Locking it in
    by default closes the eval-harness gap.
- **Stopgap grammar-label attachment on dict hits.**
  [`internal/store/dict.go`](../internal/store/dict.go) `BatchLookupForms`
  now runs the case-suffix matcher additively when a direct dict hit
  succeeds (custom mode only), attaching a case label when the
  suffix-strip lemma matches the dict lemma exactly. Previously
  `grammar_label` was empty on every direct hit, which is why grammar
  accuracy was structurally 0%. Stopgap; will be removed once the FST
  runtime in [`pkg/lemmatizer-fi-et/`](../pkg/lemmatizer-fi-et/) emits
  FEATS for direct hits — see PRs
  [#106](https://github.com/sagarinbabel/finnestdb/pull/106) /
  [#107](https://github.com/sagarinbabel/finnestdb/pull/107).
- **Recorded the decision not to extend the suffix table.**
  [`docs/DECISIONS.md`](DECISIONS.md) Decision 5 explains why suffix-table
  extension is the wrong investment direction (stem alternation,
  suffix-shaped lemmas, ambiguity, compound interaction) and why the FST
  runtime is. TODO items #15 (ternary compounds) and #16 (consonant
  gradation) are gated behind the FST migration as a result.

## 2026-05-06 — Lexical pipelines: ET ships, FI plan locks

Locks the dictionary layer as multi-source with row-level provenance and
priority, ships the Estonian source-data pipeline end-to-end, and stages
the Finnish equivalent at the schema layer with a fully scoped plan.

- Added [`docs/ESTONIAN_LEXICAL_PLAN.md`](ESTONIAN_LEXICAL_PLAN.md):
  EstNLTK/Vabamorf as the analyzer baseline, EKI/Ekilex as the
  sanctioned lexical-data source, attribution requirements per import,
  parity correction flow shared with Finnish.
- Added [`docs/LEXICAL_PLAN.md`](LEXICAL_PLAN.md): Kotus
  sanalista + Voikko (offline paradigm computation) + kaikki.org
  Wiktionary as the three open-source pillars; Kielitoimiston
  deliberately excluded; five-phase rollout (Phase 1 schema delta
  shipped, Phases 2–5 staged).
- Added "Making it AI native" section in
  [`docs/ideas.md`](ideas.md): five-phase roadmap for layering Claude
  features (grounded `/api/explain`, agentic tutor, LLM morphology
  fallback, embeddings, optional speech) onto the rule-based pipeline.
- Updated [`docs/CROSS_LANGUAGE_STRATEGY.md`](CROSS_LANGUAGE_STRATEGY.md)
  with EstNLTK adapter wiring and the dictionary-attribution metadata
  contract.

Locked decisions captured in this round of docs:

- The dictionary tables carry row-level `source` and `source_priority`,
  not a single dominant source. Per-language priority order is
  `custom_overrides` (1000) > rich generators/curated (20–30) > kaikki
  (10), with ties broken deterministically.
- Finnish paradigm coverage is *computed* from Kotus class + Voikko
  rather than scraped, and ships as a static JSONL artifact under
  `data/voikko/` rather than via runtime libvoikko.
- Translations and definitions live in dedicated tables (not
  `lemmas.gloss`); schema groundwork (`paradigm_class`, `feats`,
  `translations`, `definitions`) ships before the FI adapters that
  populate them.
- Schema migrations stay on the established idempotent
  `ALTER TABLE`/`CREATE TABLE IF NOT EXISTS` pattern with grouped
  `EnsureXxx` helpers in `internal/store/db.go`. A real migration
  framework is deferred until non-additive migrations or merge-conflict
  pressure force the move.
- Wikisanakirja (via kaikki.org's Finnish edition) covers monolingual
  FI definitions for alpha; Kielitoimiston is not bulk-imported.
- The Ekilex pipeline is four binaries with distinct roles:
  `cmd/fetchekilex` (resumable scrape against `/api/word/details`),
  `cmd/reduceekilex` (golden-tested reduction into sharded JSONL/TSV),
  `cmd/importekilexdetails` (bulk-load the reduced data drop into
  `lemmas`/`forms`), and `cmd/importekilex` (the lighter public-headword
  snapshot loader). `cmd/importdict -source-key ekilex` remains for
  on-demand API queries.
- Ambiguous surface forms get one row per `(lemma, pos)` candidate.
  `forms` PK is `(form, lang, lemma, pos)` and the deck-ingest path
  uses `BatchLookupAllForms` so a token like ET `joon` produces one
  occurrence row (and one card) per dict candidate; the parser's
  single pick is only used when the dict is silent. Migration handled
  by `EnsureMultiLemmaSchema` / `rebuildIfLegacyKey` in
  `internal/store/db.go`.

## 2026-05-01 — Architecture diagram and subsystem versioning

Separates architecture visibility from subsystem behavior tracking.

- Added a Mermaid architecture diagram to [`ARCHITECTURE.md`](../ARCHITECTURE.md)
  with explicit parser and deck/review system boundaries.
- Added [`docs/SYSTEM_VERSIONING.md`](SYSTEM_VERSIONING.md) to track parser
  behavior, parser baselines, deck review behavior, API contracts, and data
  schema versions independently.
- Updated [`docs/architecture.md`](architecture.md) to point to the canonical
  architecture and subsystem-versioning docs.

## 2026-04-29 — Consumer alpha execution plan

Locks the alpha as a consumer language-learning product with
Finnish/Estonian parity, an admin-only parser workbench, a logged-in
correction loop, and dual evaluation tracks.

- Appended the execution plan to [`TODO.md`](../TODO.md) under the
  "2026-04-29 — Consumer alpha execution plan" section.
- Added [`docs/FEATURES.md`](FEATURES.md): user-perspective product
  description, learn-before-reading framing, leverage/comprehension
  concept, mobile direction, and the four technology differentiators
  (fast parser, benchmarked quality, user correction loop, future
  autoresearch).
- Added [`docs/CROSS_LANGUAGE_STRATEGY.md`](CROSS_LANGUAGE_STRATEGY.md):
  how Finnish and Estonian improve together via shared infrastructure,
  shared evaluation, and a shared error taxonomy without copying
  morphology blindly between languages.
- Added this changelog ([`docs/CHANGELOG.md`](CHANGELOG.md)).
- Added "Current as of 2026-04-29" headers and changelog cross-links
  to the active authoritative docs:
  [`ARCHITECTURE.md`](../ARCHITECTURE.md),
  [`TODO.md`](../TODO.md),
  [`docs/IMPLEMENTATION.md`](IMPLEMENTATION.md),
  [`docs/DECISIONS.md`](DECISIONS.md),
  [`docs/srs-deck-spec.md`](srs-deck-spec.md).

Locked decisions captured in this round of docs:

- The product is a consumer language-learning app, not a parser
  workbench. The workbench remains, but admin-only.
- Logged-in users get a lightweight parse-inspection view and may
  submit parser corrections. Anonymous correction submission is out of
  scope for alpha.
- Cards are global. Deck deletion does not erase learning state.
- Two evaluation tracks: Track A (offline gold + external benchmark)
  and Track B (live accepted-correction metrics).
- Finnish external benchmark: Omorfi.
- Estonian external benchmark: EstNLTK / Vabamorf.
- [`docs/baselines/`](baselines/) is the single canonical frozen
  baseline store.
- Cross-language improvement is shared at the
  infrastructure/evaluation/error-taxonomy layer, not by copying
  morphology rules between languages.

Companion docs that will be added in later PRs and are referenced from
the execution plan but not yet present:

- `docs/SYSTEM_OVERVIEW.md`
- `docs/PARSER.md`
- `docs/PARSER_FEEDBACK_LOOP.md`
- `docs/EVAL_AND_CI.md`
- `docs/KNOWN_WORDS.md`
- `docs/MICHAEL_TODO.md`
- `docs/SECURITY_REVIEW_ALPHA.md`
