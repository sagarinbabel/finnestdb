# Documentation Changelog

This file tracks notable changes to FinEstDB planning, architecture, and
product documentation. Code changes belong in git history, not here.

Entries are reverse-chronological. Each entry links to the docs it
introduced or modified so the docs index stays navigable.

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

**FST migration link:** the silver tagger in PR 4 will use the FST
runtime from [PRs #106](https://github.com/sagarinbabel/finnestdb/pull/106)–[#112](https://github.com/sagarinbabel/finnestdb/pull/112)
as one half of the agreement filter (the Voikko side); Omorfi via the
Python adapter is the other half. So PR 4 stacks meaningfully on the
FST series.

## 2026-05-06 — Lexical pipelines: ET ships, FI plan locks

Locks the dictionary layer as multi-source with row-level provenance and
priority, ships the Estonian source-data pipeline end-to-end, and stages
the Finnish equivalent at the schema layer with a fully scoped plan.

- Added [`docs/ESTONIAN_LEXICAL_PLAN.md`](ESTONIAN_LEXICAL_PLAN.md):
  EstNLTK/Vabamorf as the analyzer baseline, EKI/Ekilex as the
  sanctioned lexical-data source, attribution requirements per import,
  parity correction flow shared with Finnish.
- Added [`docs/FINNISH_LEXICAL_PLAN.md`](FINNISH_LEXICAL_PLAN.md): Kotus
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
