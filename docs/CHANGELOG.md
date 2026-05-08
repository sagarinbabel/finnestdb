# Documentation Changelog

This file tracks notable changes to FinEstDB planning, architecture, and
product documentation. Code changes belong in git history, not here.

Entries are reverse-chronological. Each entry links to the docs it
introduced or modified so the docs index stays navigable.

**Cross-reference convention:** CHANGELOG records what changed; [`DECISIONS.md`](DECISIONS.md)
records why we chose to change it that way. Where the same event appears
in both files, both entries cross-link.

## 2026-05-09 — Architecture and corpus documentation audit

Refreshes the living docs after the corpus pipeline and baseline-compression
PRs landed, while leaving historical reports and frozen baselines
append-only.

- Modified: [`ARCHITECTURE.md`](../ARCHITECTURE.md) — updated the current
  product surface, Mermaid diagram, layer responsibilities, data flows,
  localdata/FST boundaries, and near-term direction to include the corpus
  pipeline.
- Modified: [`corpus_pipeline/docs/CORPUS_PIPELINE.md`](../corpus_pipeline/docs/CORPUS_PIPELINE.md) —
  replaced stale future-work wording with built-vs-deferred status, refreshed
  profile guidance, and corrected FST-table troubleshooting.
- Modified: [`corpus_pipeline/docs/PR_ROADMAP.md`](../corpus_pipeline/docs/PR_ROADMAP.md),
  [`corpus_pipeline/v2plan.md`](../corpus_pipeline/v2plan.md), and
  [`corpus_pipeline/Makefile`](../corpus_pipeline/Makefile) — aligned roadmap
  statuses and profile help text with the landed corpus PRs.
- Modified: [`docs/FST_LEMMATIZER.md`](FST_LEMMATIZER.md) — documented the
  ET generated-table command and remaining production-promotion conditions.
- Modified: [`docs/INDEX.md`](INDEX.md), [`README.md`](../README.md),
  [`TODO.md`](../TODO.md), and [`finnestdb-prd-alpha.md`](../finnestdb-prd-alpha.md) —
  refreshed canonical navigation, PR state, and the historical PRD's
  implementation snapshot.
- Added: [`docs/qa-reports/2026-05-08T2229Z-doc-architecture-corpus-audit.md`](qa-reports/2026-05-08T2229Z-doc-architecture-corpus-audit.md) —
  timestamped full-doc audit and Git/GitHub branch-state report.

## 2026-05-09 — Typographic quote tokenization docs (PR #171)

Documents the current Rust parser contract after PR
[#171](https://github.com/sagarinbabel/finnestdb/pull/171): leading/trailing
punctuation cleanup includes common typographic quote marks, and opening
punctuation labels are part of sentence-text spacing reconstruction.

- Modified: [`README.md`](../README.md) — clarified the known-limitations
  summary for tokenizer punctuation and opening-quote spacing behavior.

## 2026-05-07 — Voikko `[P4]` Voice + participle field cleanup (PR #158)

Closes the Voice accuracy gap flagged in the parser audit (FI custom
5.3% vs omorfi 89.7% on fi-ftb) by fixing two specific bugs in
`voikkomap` on top of the rich Voice/VerbForm/PartForm extraction that
already landed in PRs [#154](https://github.com/sagarinbabel/finnestdb/pull/154)
and [#155](https://github.com/sagarinbabel/finnestdb/pull/155):

1. **`[P4]` no longer leaks as `Person=4`.** Finnish passive is
   grammatically the "4th person" in Voikko's tag set, but UD `Person`
   is 1/2/3. `[P4]` now sets `Voice=Pass` and leaves `Person` empty;
   `[P1-P3]` set `Voice=Act` alongside the UD `Person` value, so
   active finite verbs no longer compose FEATS without Voice.
2. **`applyParticiple` clears finite-only fields.** Defense-in-depth:
   when `[R*]` wins, Mood/Tense/Person are reset so a participle
   never composes contradictory FEATS like `Tense=Past|VerbForm=Part`
   — UD encodes the past/present distinction in `PartForm=`, not
   `Tense=`.

The shared Voice/VerbForm plumbing (Analysis fields, `applyParticiple`
per-tag mapping, `[Tn1-n5]` → `VerbForm=Inf`, Giellalt Act/Pass/Inf
extraction) all landed in PRs #154 and #155; PR #158 fills in the two
Voikko-specific gaps those PRs left open.

The `[E*]` tags were investigated as a possible voice signal and found
to encode connegative status (Ef=false, Et=true, Eb=both) — confirmed
from libvoikko's `FinnishVfstAnalyzer.cpp::parseBasicAttributes`.
Documented in the voikkomap header. Not projected to UD because the
runtime already gets `Connegative=Yes` from the orthogonal `[Cn]` tag.

- Modified: [`docs/FST_LEMMATIZER.md`](FST_LEMMATIZER.md) — new
  "Voikko Voice extraction" subsection covering the `[P*]` Voice
  derivation and participle field cleanup; updated stale 5-param
  `Compose` reference.
- Modified: [`docs/PARSER_EVOLUTION.md`](PARSER_EVOLUTION.md) — new
  entry for PR #158.

## 2026-05-07 — Baseline filename convention + freeze-baseline script

Standardizes the baseline filename convention so every baseline has a
**date AND time** stamp and the `docs/baselines/` directory stays
**append-only** in practice, not just in policy.

Canonical form going forward:

```
docs/baselines/YYYY-MM-DD<rev>-T<HHMM>Z-<dataset>.<ext>
```

— matching the `2026-05-07k-T1118Z-fi-core.json` logical baseline name
introduced with the [#140](https://github.com/sagarinbabel/finnestdb/pull/140)
baseline. Raw JSON files are now stored with a `.gz` suffix to keep repository
line counts manageable. Older tagged-style files
(`2026-05-06-final-*`, `2026-05-07-feats-rich-*`, etc.) are left as-is —
renaming them would break PR/commit cross-references the
[#141](https://github.com/sagarinbabel/finnestdb/pull/141) append-only history section was meant to
preserve.

- New [`scripts/freeze-baseline.sh`](../scripts/freeze-baseline.sh)
  takes the comparison-script `RUN_TS`, reads the parser-version letter
  from `parsecore.ParserVersion`, derives the date + UTC HHMM, and
  compresses per-dataset JSONs and copies cross-language summaries from
  `reports/parser-eval/` into `docs/baselines/` under the canonical
  name. **Refuses to overwrite** an existing target file — append-only
  is enforced mechanically, not just by convention. Override the
  parser-version letter with `-rev <letter>` for the rare case of
  freezing a measurement made before a same-day version bump.
- [`docs/baselines/README.md`](baselines/README.md) gains a
  "Filename convention" section spelling out the spec, with examples,
  the rationale for `T<HHMM>Z` (multiple same-day same-`<rev>`
  re-measures), and a pointer to the script.
- [`docs/PARSER_EVAL_METHODOLOGY.md §5 Freeze a baseline`](PARSER_EVAL_METHODOLOGY.md)
  replaces the manual `cp` recipe with a one-line `scripts/freeze-baseline.sh "$RUN_TS"`
  invocation.

## 2026-05-07 — Newcomer experience: `make doctor` + setup symmetry

Closes the gap between "the docs say setup is one command" and "the
parser silently runs in degraded mode because something didn't fetch".

- New [`make doctor`](../cmd/doctor/main.go) reports DB presence + per-
  source row counts, FST table presence, analyzer venv presence
  (`.venv`), Ekilex shard presence, UD cache,
  frequency baselines, and the Rust parser shared library. Each missing
  piece carries a one-line hint and a "go to" target. Returns 0 unless
  the DB or the FI/ET dictionary is missing entirely; everything else is
  informational so the user understands the *degraded modes* their setup
  implies. Added to [`docs/INDEX.md`](INDEX.md) and the
  [`README.md`](../README.md) Quickstart.
- [`scripts/parser-comparison.sh`](../scripts/parser-comparison.sh) and
  [`scripts/parser-comparison-et.sh`](../scripts/parser-comparison-et.sh)
  now slug from the dataset *file basename* rather than the JSON `name`
  field. Pre-fix, `fi-manual-v1.json` and `fi-manual-v2.json` both
  declared `name="fi-manual"` and silently overwrote each other in
  `reports/parser-eval/`. Fix called out in
  [`docs/PARSER_EVAL_METHODOLOGY.md`](PARSER_EVAL_METHODOLOGY.md) §pitfalls.
- `make setup-nlp` creates a unified `.venv/` containing both omorfi and
  estnltk instead of pip-installing into the active interpreter.
  `parser-comparison.sh` auto-constructs `FINNESTDB_OMORFI_CMD` from
  the venv when present, matching the EstNLTK auto-detection. Closes the
  open issue noted in
  [`docs/PARSER_EVAL_METHODOLOGY.md`](PARSER_EVAL_METHODOLOGY.md) §pitfalls
  about asymmetric venv discovery.
- New `BackfillLegacyKaikkiProvenance` migration in
  [`internal/store/db.go`](../internal/store/db.go) labels FI/ET rows
  that legacy importers left with empty `source` / `source_priority=0`
  as `(source='kaikki', source_priority=10)`. Idempotent — runs every
  startup but matches no rows once applied. Surfaced as a WARN in
  `make doctor` until a server start has run the migration.

## 2026-05-07 PM — Park autoresearch as post-live idea

Clarifies that autoresearch is an idea parking lot for after the app is shipped
and live, not active roadmap work.

- Added root [`AGENTS.md`](../AGENTS.md) with LLM-facing instructions:
  ignore autoresearch unless the user explicitly asks for it, and do not block
  unrelated parser/product work on `cmd/autoresearch` behavior.
- Added the same guardrail to [`docs/INDEX.md`](INDEX.md) and
  [`docs/AUTORESEARCH.md`](AUTORESEARCH.md).
- Relabeled top-level references in [`README.md`](../README.md),
  [`ARCHITECTURE.md`](../ARCHITECTURE.md), [`TODO.md`](../TODO.md),
  [`docs/FEATURES.md`](FEATURES.md), and [`docs/DECISIONS.md`](DECISIONS.md)
  so autoresearch reads as deferred post-live exploration.

## 2026-05-07 PM — Docs restructure + LLM-friendly navigation

Restructures the spine docs so a reader (human or LLM) can answer
"what's shipped, what's next, what's open, why" without cross-doc
detective work.

- [`TODO.md`](../TODO.md) restructured 765 → 646 lines: replaced "Roadmap
  Status" Phase 1–5 with explicit "What's in main" / "What's not in
  main yet" / "Open PRs" / "Research Goals" / "Notes & historical"
  sections. Implementation Backlog 1–19 distributed by area (Parser
  quality, Learner experience, Self-improving feedback loop, etc.).
  Critical Findings reframed as historical traceability.
- [`docs/DECISIONS.md`](DECISIONS.md) reordered latest-first (995
  lines): 4 new 2026-05-07 decisions added (Single-folder bootstrap
  rule; FST as parallel scorer; ESTONIAN_LEXICAL_PLAN consolidation;
  IMPLEMENTATION.md split). Absorbed 8 "Locked Decisions" from
  LEXICAL_PLAN.md as Decisions 7–14 (2026-05-06). Header renamed to
  "Decisions Log" (roadmap moved to TODO.md). Project Roadmap section
  preserved as historical with status updated.
- [`docs/LEXICAL_PLAN.md`](LEXICAL_PLAN.md) trimmed: "Locked Decisions"
  section moved to DECISIONS.md, "Phasing" section moved to
  PARSER_EVOLUTION.md as historical entries, "Migration Framework
  Plan" moved to TODO.md "What's not in main yet". Doc now focuses on
  current architecture (schema, resolver, importer pattern,
  per-language source choices) rather than mixing architecture + plan
  + decisions.
- [`README.md`](../README.md) Project Structure expanded with 5
  previously-missing cmd binaries (`importkotus`, `importud`,
  `scrapegutenberg`, `fetchfrequency`, `genlemmatizertables`) and the
  `pkg/lemmatizer-fi-et` package. Custom parser description now
  mentions FST candidate scoring (post-#127). `localdata/` bullet
  expanded to enumerate every gitignored runtime artifact.

**See also:** DECISIONS.md Decisions 17 (ESTONIAN_LEXICAL_PLAN consolidation)
and 18 (IMPLEMENTATION.md split) — both also record the 2026-05-07 AM PR #135
that did the first round of doc consolidation.

## 2026-05-07 AM — Doc parity sweep + 07k baseline freeze (PR #135)

Doc-parity sweep driven by an audit of all spine docs against the day's
PRs (#127–#134). Plus the `2026-05-07k-T0944Z` baseline freeze
(companion to the PARSER_EVOLUTION.md `2026-05-07k` row).

- Date headers refreshed in [`ARCHITECTURE.md`](../ARCHITECTURE.md),
  [`docs/FEATURES.md`](FEATURES.md),
  [`docs/CROSS_LANGUAGE_STRATEGY.md`](CROSS_LANGUAGE_STRATEGY.md).
- "FST step 5 fallback" description corrected (post-#127/#129) in
  [`docs/PARSER_EVAL_METHODOLOGY.md`](PARSER_EVAL_METHODOLOGY.md),
  [`docs/SYSTEM_VERSIONING.md`](SYSTEM_VERSIONING.md), and
  [`ARCHITECTURE.md`](../ARCHITECTURE.md) Custom parser bullet.
- [`ARCHITECTURE.md`](../ARCHITECTURE.md): removed obsolete
  `data/voikko/...` Voikko-seed paragraph; updated FI lexical pipeline
  status (Phases 1–3 shipped, Phase 4 superseded, Phase 5 partial);
  removed stale "in flight as #78"; added `cmd/importkotus`,
  `cmd/genlemmatizertables`, `cmd/fetchfrequency` to the cmd list.
- [`docs/PARSER_EVOLUTION.md`](PARSER_EVOLUTION.md): broken
  `FINNISH_LEXICAL_PLAN.md` link fixed to `LEXICAL_PLAN.md` (renamed
  in #112); `data/kotus/` path fixed to `localdata/kotus/`.
- [`docs/ESTONIAN_LEXICAL_PLAN.md`](LEXICAL_PLAN.md) merged into
  [`docs/LEXICAL_PLAN.md`](LEXICAL_PLAN.md) as the "Estonian-specific
  source choices and adapter contract" section. Original file deleted.
- [`docs/IMPLEMENTATION.md`](IMPLEMENTATION.md) split: "Suggest fix"
  UX → new [`docs/PARSER_FEEDBACK_LOOP.md`](PARSER_FEEDBACK_LOOP.md);
  IMPLEMENTATION.md replaced with a redirect stub.
- [`IMPLEMENTATION_ANALYSIS.md`](../IMPLEMENTATION_ANALYSIS.md) gained
  a historical banner pointing readers to ARCHITECTURE.md and
  DECISIONS.md Decision 1.
- 2026-05-07k baseline freeze: 8 files under
  `docs/baselines/2026-05-07k-T0944Z-*` (FI + ET, JSON reports +
  summary markdown). `parsecore.ParserVersion` bumped to
  `2026.05.07k`.

**See also:** DECISIONS.md Decisions 16, 17, 18 (the three doc/code
decisions this PR enforced).

## 2026-05-07 — Single-folder data root + ET UD gold materialized

Consolidates every gitignored runtime data artifact under
[`localdata/`](../localdata/) so a single tarball captures the entire
bootstrap state. Materializes the ET UD parser-eval gold and the
FI/ET UD train splits that PR #113 (Plan C / PR 1) had documented but
not yet generated on the user's machine.

- New [`docs/data_enhancement.md`](data_enhancement.md): single-source-of-truth
  ledger of every gold/silver/dictionary corpus the project pulls in.
  Each row tracks source URL, license, size, path, added date, and
  last-refreshed date. Update on every import.
- Path consolidation:
  - `data/ud-cache/` → `localdata/ud-cache/`
  - `testdata/parser-eval/fi/gold-train/` → `localdata/parser-eval/fi/gold-train/`
  - `testdata/parser-eval/et/gold/ud-et-*.json` → `localdata/parser-eval/et/gold/`
  - `testdata/parser-eval/et/gold-train/` → `localdata/parser-eval/et/gold-train/`
  Committed FI dev/test gold under `testdata/parser-eval/fi/gold/` is
  unchanged (still byte-identical after re-import).
- [`scripts/fetch-and-import-ud.sh`](../scripts/fetch-and-import-ud.sh)
  writes to the new locations.
- [`scripts/parser-comparison.sh`](../scripts/parser-comparison.sh) and
  [`scripts/parser-comparison-et.sh`](../scripts/parser-comparison-et.sh)
  auto-discover from both `testdata/parser-eval/<lang>/gold/` and
  `localdata/parser-eval/<lang>/gold/`. Held-out discipline preserved
  (still excludes `*-dev-v*` and `gold-train/`).
- [`scripts/setup-local.sh`](../scripts/setup-local.sh) summary lists
  every `localdata/` subtree on completion and emits the bootstrap
  tar instruction (`tar czf finnestdb-bootstrap.tgz localdata/ finnestdb.db`).
- [`.gitignore`](../.gitignore) collapsed: the `localdata/` blanket rule
  covers everything; legacy `data/` paths kept as a belt-and-braces guard.
- [`docs/ARTIFACT_POLICY.md`](ARTIFACT_POLICY.md) gains a "Single-folder
  bootstrap rule" section documenting the invariant.
- [`ARCHITECTURE.md`](../ARCHITECTURE.md) §Evaluation Stack treebank
  table updated with the new path strings; Estonian-EWT train count
  corrected (5,375 actual vs. 5,380 documented).
- Local gold available after this PR + a `make import-ud-gold` run:
  ~37k FI cases / 339k FI tokens, ~37.9k ET cases / 437k ET tokens —
  ~7.5× the cases and ~8.5× the tokens previously visible in `git`.

**Why now:** the previous layout had three gitignored data roots
(`localdata/`, `data/ud-cache/`, two carve-outs under
`testdata/parser-eval/`). Handing a teammate a "fast bootstrap" zip
required either three separate archives or a custom recipe that knew
the carve-outs. Consolidating to one root removes the foot-gun.

## 2026-05-07 — Runtime docs parity pass

Aligns user-facing docs with the E2E behavior report in
[`docs/qa-reports/2026-05-06-e2e-doc-behavior-report.md`](qa-reports/2026-05-06-e2e-doc-behavior-report.md).

- [`README.md`](../README.md) now distinguishes unknown-language advisory
  warnings from blocking Finnish/Estonian mismatch warnings in the language
  detection overview.
- [`docs/FEATURES.md`](FEATURES.md) now describes the signed-in browser Parse
  flow, clarifies that direct unauthenticated API parses are ephemeral
  development behavior, and frames multi-candidate deck cards as
  dictionary-coverage dependent rather than guaranteed for the `joon` example.

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

**Generated-table migration link:** future production FI/ET morphology
tables will reuse the same `-baseline-dir` machinery. Gold case files
already carry a `feats` field after PR #113, so the per-attribute
extension is purely on the report side.

## 2026-05-07 — Gutenberg-FI silver corpus scraper (Plan C / PR 3)

First silver-tier corpus source. Scrapes public-domain Finnish books from
Project Gutenberg (https://www.gutenberg.org/ebooks/search/?query=l.fi),
strips PG boilerplate, saves cleaned text + a JSONL manifest under
`data/silver-fi/`.

- Added [`cmd/scrapegutenberg`](../cmd/scrapegutenberg/main.go): polite
  HTTP scraper (1.5s between requests, transparent User-Agent, single
  connection). Tries cache-epub UTF-8 → files-0 UTF-8 → files-8
  ISO-8859-1 in order; decodes ISO-8859-1 via golang.org/x/text/encoding;
  strips Project Gutenberg "*** START OF" / "*** END OF" boilerplate;
  rejects non-Finnish leaks (English-authored books with l.fi metadata)
  via an ä/ö frequency + common-particle heuristic.
- Added [`data/silver-fi/`](../data/silver-fi/) with 14 books (~511k
  tokens) on first run: Kalevala, Aleksis Kivi (Seitsemän veljestä),
  Minna Canth, Aleksis Kivi-era prose, Finnish translations of Jack
  London / Molière / Drachmann, plus modern works (Pekkarinen,
  Haanpää, Järnefelt). Manifest at
  [`data/silver-fi/manifest.jsonl`](../data/silver-fi/manifest.jsonl)
  records id, title, author, source URL, encoding, fetched_at, token
  count per book.
- Added Makefile target `make scrape-gutenberg-fi` (overridable
  `TARGET_TOKENS=N`).
- Idempotent — already-fetched books are skipped on re-run.

**Why now:** with ~900k UD gold tokens and ~500k Gutenberg silver
tokens, we're at the corpus scale where bootstrap CIs from Plan C / PR 2
are tight (±0.4–0.6pp) and "did our parser regress" is answerable
with confidence. Next silver sources (runosto.net for poetry, ET
Wikisource for Estonian, Wikipedia FI/ET for breadth) follow the same
pattern; this PR establishes the scaffolding.

**Silver tagging deferred:** the actual morphological annotation
(Voikko + Omorfi agreement filter for FI; Vabamorf + Ekilex for ET)
ships in Plan C / PR 4. This PR delivers the raw corpus only.

**Generated-table migration link:** the silver tagger can use future
production generated morphology tables as one half of the agreement
filter. Omorfi via the Python adapter remains the other FI comparison
path.

## 2026-05-06 — Numeric-hyphen tokenization (FI + ET)

Surfaced by manual testing on Estonian text containing `65-aastane`. The
shared Rust tokenizer at [`parser/src/lib.rs`](../parser/src/lib.rs) was
keeping `65-aastane`, `1990-luvulla`, `250 000`, etc. as opaque single tokens
or pairs of NOUN stubs, with no `NUM` POS for digit forms. Confirmed Finnish
had the identical bug (the tokenizer ignores its `_lang` parameter).

Following [`docs/CROSS_LANGUAGE_STRATEGY.md`](CROSS_LANGUAGE_STRATEGY.md) on
shared error categories and shared-pipeline investments, the fix is four
tokenizer-only rules (R1–R4) — no per-language rule tables.

- Added [`docs/qa-reports/2026-05-06-numeric-hyphen-tokenization.md`](qa-reports/2026-05-06-numeric-hyphen-tokenization.md):
  bug repro, root cause, R1–R4 with worked examples in both languages,
  measured impact (zero regression on all 6 existing gold datasets), and
  follow-ups.
- Added Decision 6 to [`docs/DECISIONS.md`](DECISIONS.md) recording that
  numeric-hyphen handling lives in the shared tokenizer rather than in
  language-specific rule tables.

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
  uses `BatchLookupAllForms` so, when the dictionary has multiple
  candidates for a surface form such as ET `joon`, the saved deck gets
  one occurrence row (and one card) per dict candidate; the parser's
  single pick is only used when the dict is silent. Migration handled by
  `EnsureMultiLemmaSchema` / `rebuildIfLegacyKey` in `internal/store/db.go`.

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
  concept, mobile direction, and the technology differentiators as
  described at the time. The autoresearch idea mentioned in that round
  was later parked as post-live exploration; see the 2026-05-07 PM
  guardrail entry above.
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
