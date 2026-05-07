# Lexical Plan

This is the working plan for making Finnish and Estonian lexical coverage
first-class. Schema-level groundwork (multi-source rows, source priority,
attribution metadata, translations, definitions, and parser eval discipline)
is shared across languages. Source choices and importer details remain
language-specific.

[#66]: https://github.com/sagarinbabel/finnestdb/pull/66
[#67]: https://github.com/sagarinbabel/finnestdb/pull/67
[#68]: https://github.com/sagarinbabel/finnestdb/pull/68

## Language-agnostic Lexical Architecture

The parser and deck flow query one multi-source dictionary API where:

- **Provenance is row-level and deterministic.** Lemma, form,
  translation, definition, and generated-table rows identify their
  source. What the resolver returns is reproducible from committed data.
- **New sources fill gaps or intentionally override.** Higher-priority
  sources can win over lower-priority sources; lower-priority sources
  still contribute candidates where higher-priority sources are silent.
- **FI and ET share the same shape.** The source mix differs by
  language, but importer structure, attribution, eval, and resolver
  semantics should stay shared.

### Schema overview

| Table/artifact | Role | Provenance |
|---|---|---|
| `lemmas` | one row per `(lemma, pos, lang, source)` | `source`, `source_priority` |
| `forms` | one or more `(form, lang, lemma, pos)` candidates | `source`, `source_priority` |
| `translations` | per-meaning translations into `target_lang` | `source` |
| `definitions` | monolingual definitions in `lang` | `source` |
| `dict_metadata` | per-source attribution, version, license, import notes | one row per source |
| `localdata/lemmatizer-fi-et/tables/` (gitignored) | generated factual morphology analyses | table provenance, generator command, upstream source |

The `forms` primary key allows one surface form to map to multiple
lemma/POS candidates. That matters for homonyms such as ET `joon` and FI
`osat`: deck ingest can preserve multiple dictionary candidates while
the parser still picks one best analysis for display.

### Source priority semantics _(added 2026-05-07)_

Every row carries `source_priority` (integer, higher wins). The resolver
ranks candidate rows by:

1. **Source priority** (higher first).
2. **Source name** (deterministic tiebreak, ascending).
3. **Surface plausibility** (case-match against the input form;
   demote `PROPN` candidates when the input is lowercase).

The third rule was added in
[PR #84](https://github.com/sagarinbabel/finnestdb/pull/84) ("source-aware
lookup ranking") after the Ekilex bulk import surfaced PROPN homonyms
beating common-noun lemmas — see
[`docs/baselines/2026-05-06b-summary.md`](baselines/2026-05-06b-summary.md)
for the regression-and-recovery story. **Whenever a new source lands,
re-run the gold eval and freeze a new baseline before promoting it to
production priority.** Adding a source without re-evaluation is the
known way to silently regress accuracy.

### Multi-lemma surface forms (homonyms) _(added 2026-05-07)_

The `forms` PK is `(form, lang, lemma, pos)` so one surface form can
map to multiple `(lemma, pos)` candidates. Examples:

- ET `joon` = NOUN "line" + 1Sg present of VERB `jooma` ("to drink")
- FI `osat` = NOUN nominative plural of `osa` ("part") + 2Sg present
  of VERB `osata` ("to know how to")

At deck-ingest time, every dict candidate becomes its own `occurrence`
row and its own card. The parser's single pick is only used when the
dict has zero entries for the form. This means a learner studying
`joon` sees both senses; correctness is preserved, and disambiguation
is deferred to the user-correction loop and (eventually) a contextual
disambiguator (see [`docs/ML_IDEAS.md` §1a](ML_IDEAS.md)).

### Generated tables and dictionary boundary

The lemmatizer package now follows
[docs/ARTIFACT_POLICY.md](ARTIFACT_POLICY.md): runtime code loads generated
factual tables from local disk, and neither those tables nor upstream
transducer blobs are tracked in git.

Resolution order is:

1. **Generated table lookup** (`pkg/lemmatizer-fi-et/`): exact
   surface-form analyses from local JSON tables. Post PRs
   [#127](https://github.com/sagarinbabel/finnestdb/pull/127) and
   [#129](https://github.com/sagarinbabel/finnestdb/pull/129), the
   FST contributes candidates in parallel with dict step 1, not as a
   step-5 fallback.
2. **Multi-source dictionary lookup** (`internal/store/dict.go`):
   SQLite rows ranked by source priority, source name, and surface
   plausibility (see "Source priority semantics" above).
3. **Rule fallback** (`internal/parserules/`): suffix stripping,
   compound splitting, and language-specific fallback behavior.
4. **Stub fallback**: preserve the input surface when nothing resolves.

Post-FST goals:

- FST handles the vast majority of FI tokens (target: >95% of words).
- Dict catches the long tail (proper nouns, English loans, brand
  names) and supplies translations/definitions.
- Suffix/compound rules near-zero firing rate on FI; if higher than
  noise, that signals an FST gap worth investigating.

The current FI/ET generated tables are smoke fixtures (9 FI keys, 7 ET
keys; see [`testdata/lemmatizer/`](../testdata/lemmatizer/)). They
prove the integration and policy, not production morphology coverage.
Production coverage requires running `make gen-lemmatizer-tables-fi`
and/or `make gen-lemmatizer-tables-et` locally to write full tables to
`localdata/lemmatizer-fi-et/tables/`.

### Importer pattern

Each rich source should have a dedicated `cmd/import<source>/` or
`cmd/gen<artifact>/` entry point. Importers and generators should:

- read the source's native format or local analyser output;
- write deterministic rows or generated tables;
- record source, priority, attribution, license, version, and command;
- be idempotent or support explicit source replacement;
- keep focused tests and fixtures for reducer/generator behavior.

### Cross-references _(added 2026-05-07)_

- **Per-language source choices**: this doc covers both the Finnish
  sections below ("Current Decision", "Adapters") and the consolidated
  Estonian section ("Estonian-specific source choices and adapter
  contract"). The legacy `docs/ESTONIAN_LEXICAL_PLAN.md` was deleted
  on 2026-05-07; its unique content lives here now.
- **System-level architecture**: [`ARCHITECTURE.md` §5 "Dictionary /
  Persistence"](../ARCHITECTURE.md) for how the lexical layer plugs
  into parsecore, the API, and the deck flow.
- **Cross-language strategy**: [`docs/CROSS_LANGUAGE_STRATEGY.md`](CROSS_LANGUAGE_STRATEGY.md)
  for what is shared vs. language-specific at the parser strategy
  level (error taxonomy, evaluation discipline, external benchmark
  slots).
- **FST migration**: [`docs/FST_LEMMATIZER.md`](FST_LEMMATIZER.md) for
  the change document, [`docs/FST_LEMMATIZER_ROADMAP.md`](FST_LEMMATIZER_ROADMAP.md)
  for the per-PR sequencing.
- **Public frequency baselines**: [`docs/FREQUENCY_BASELINES.md`](FREQUENCY_BASELINES.md)
  for inflected-form baselines used to compare against user-aggregated
  frequency. Bulk frequency files live under `localdata/frequency/`
  (gitignored), populated by `cmd/fetchfrequency`.
- **Data corpora ledger**: [`docs/data_enhancement.md`](data_enhancement.md)
  is the single source of truth for every external corpus pulled in.

## Current Decision

Finnish has no single Sonaveeb-equivalent. We combine open sources into
the same lemma/form/translation tables, tag rows by `source`, and resolve
at query time by `source_priority`.

| Source | Role | License |
| --- | --- | --- |
| **Kotus sanalista** | Authoritative lemma list with Kotus inflection class (51 nominal + 76 verb classes) | CC BY 4.0 |
| **Voikko / Giellalt-derived tables** | Offline source for generated factual morphology tables; current committed tables are smoke fixtures | Upstream analysers stay local; committed tables need provenance |
| **kaikki.org (Wiktionary)** | Translations, monolingual definitions, irregular forms not covered by Kotus classes | CC BY-SA 3.0 |

Deliberately not used:

- **Kielitoimiston sanakirja** — authoritative monolingual FI definitions,
  but redistribution is restricted. Not bulk-imported. Revisit only if
  Wikisanakirja coverage proves insufficient and only as a runtime
  lookup that respects the license.
- **fi.wiktionary directly** — kaikki.org already extracts and
  normalizes Wiktionary, including the Finnish edition. Use it.

## Why This Path

- Generated morphology tables let us consume rule-based analyser output
  as plain factual data without shipping the analyser blobs.
- Kotus is the canonical lemma-and-class authority and is openly
  licensed.
- kaikki.org is the only practical bulk source for Finnish↔English
  translations and Finnish-language definitions; it is already wired up
  as an importer source.
- All three sit behind the same `lemmas`/`forms` resolution layer, with
  source provenance and priority — the mechanism the ET track is
  building in [#67] and [#68].

Reference pages:

- Kotus sanalista: https://kaino.kotus.fi/sanat/nykysuomi/
- Voikko: https://voikko.puimula.org/
- kaikki.org Finnish dump: https://kaikki.org/dictionary/Finnish/

## Locked Decisions

_Moved 2026-05-07 to [`docs/DECISIONS.md`](DECISIONS.md) (Decisions 7–14).
DECISIONS.md is the canonical decision log; the lexical-layer locks are
preserved there latest-first with their original 2026-05-06 dates and
context. Cross-link from there back to this doc's Schema Delta /
Adapters / Resolution Layer sections for implementation detail._

## Schema Delta

The shared columns (`source`, `source_priority` on `lemmas`/`forms`;
extended `dict_metadata`) are introduced in [#67] and consumed by [#68].
The Finnish-specific schema additions are:

```sql
-- Kotus class (or equivalent paradigm identifier) for the lemma.
-- Drives the join from the Kotus adapter to the Voikko generator.
ALTER TABLE lemmas ADD COLUMN paradigm_class TEXT;

-- UD-style morph features encoded as JSON, e.g. {"Case":"Ela","Number":"Sing"}.
-- Populated by Voikko-generated rows; left NULL on kaikki.org rows.
ALTER TABLE forms ADD COLUMN feats TEXT;

-- Cross-language translations. Replaces the practice of stuffing
-- target-language glosses into lemmas.gloss.
CREATE TABLE translations (
    lemma       TEXT NOT NULL,
    pos         TEXT NOT NULL,
    lang        TEXT NOT NULL,    -- source language (FI, ET, ...)
    target_lang TEXT NOT NULL,    -- e.g. EN
    text        TEXT NOT NULL,
    sense_idx   INTEGER NOT NULL DEFAULT 0,
    source      TEXT NOT NULL,
    PRIMARY KEY (lemma, pos, lang, target_lang, sense_idx, source)
);

-- Monolingual definitions in the source language.
CREATE TABLE definitions (
    lemma     TEXT NOT NULL,
    pos       TEXT NOT NULL,
    lang      TEXT NOT NULL,
    sense_idx INTEGER NOT NULL DEFAULT 0,
    text      TEXT NOT NULL,
    source    TEXT NOT NULL,
    PRIMARY KEY (lemma, pos, lang, sense_idx, source)
);
```

`lemmas.gloss` stays as a denormalized "primary translation" cache for
fast paths and for backwards compatibility with the existing UI.

## Adapters

Three Finnish adapters: one new binary per rich source, plus the
existing `cmd/importdict/` for kaikki.org. All write `source` and
`source_priority` (mechanism from [#67]) and respect the attribution
fields in `dict_metadata`.

### `cmd/importkotus/` (source key `kotus`, default priority 10)

- Input: official Kotus sanalista distribution (CSV/XML, current year).
- Output: rows in `lemmas` with `source='kotus'`, `paradigm_class`
  filled, `gloss=NULL`.
- Does *not* write to `forms` — the Voikko generator is responsible.
- For lemmas already present from kaikki.org: upgrade `paradigm_class`
  in place rather than insert a duplicate row at lower priority.

### `cmd/genlemmatizertables/` (generated morphology tables)

The current generated-table path is intentionally narrower than the
original Voikko seed plan.

- The generator may read local upstream analysers such as Voikko
  `mor.vfst`, but those analyser files stay outside git.
- The output is a factual JSON table under
  `localdata/lemmatizer-fi-et/tables/` (gitignored); the runtime loads
  it from disk on `New()`.
- `make gen-lemmatizer-tables-fi VFST_PATH=/path/to/mor.vfst`
  regenerates the current FI smoke table from
  `cmd/genlemmatizertables/wordlists/fi_smoke.txt`.
- A production FI/ET table PR must add a production word list,
  provenance, generator command, row counts, and fresh eval.

### `cmd/importdict/` — kaikki.org (source key `kaikki`, default priority 10)

Today's `cmd/importdict/main.go` is this adapter, untagged. Changes
needed:

- Tag every row with `source='kaikki'` (via [#67] mechanism).
- Stop putting English glosses in `lemmas.gloss`. Instead:
  - extract `senses[*].glosses[*]` into `translations` with
    `target_lang='EN'`
  - extract Finnish-language senses (where the entry is from
    fi.wiktionary, not en.wiktionary) into `definitions`
- Continue writing `forms` rows. Voikko will outrank these on conflict.
- Continue skipping possessive-tagged forms; suffix stripping in
  [`internal/parserules/finnish.go`](../internal/parserules/finnish.go)
  still handles them at runtime.

## Estonian-specific source choices and adapter contract

_Consolidated 2026-05-07 from the legacy `docs/ESTONIAN_LEXICAL_PLAN.md`._

### Current ET decision

Use two complementary upstream paths:

1. **External analyzer baseline:** EstNLTK / Vabamorf through the
   `estnltk` parser adapter.
2. **Sanctioned lexical data:** EKI/Ekilex/Sõnaveeb-derived lexical data
   imported with explicit attribution, license text, source version,
   and change notices via `cmd/importekilex` and `cmd/importekilexdetails`.

The local custom parser stays useful as a fast deterministic baseline
and as the product parser. External analyzer output is comparison data
unless we explicitly choose to promote a behavior into local rules or
dictionary rows.

Reference pages:

- Sõnaveeb about/license: <https://sonaveeb.ee/about?uilang=en>
- Ekilex application/API entry point: <https://ekilex.ee/login>
- EstNLTK repository: <https://github.com/estnltk/estnltk>

### EstNLTK adapter contract

Parser mode: `estnltk`.

Command override:

```bash
export FINNESTDB_ESTNLTK_CMD="/path/to/python /path/to/scripts/estnltk_adapter_example.py"
```

Default discovery:

- `scripts/estnltk_adapter_example.py`
- `.venv-estnltk/bin/python` when created by `make setup-estnltk`
- nearest repo root containing `go.mod`
- executable directory fallback

Subprocess timeout: each call spawns a fresh Python process and reloads
Vabamorf, so cold start alone is roughly 1 second per call. The default
budget is `30s`, overridable with a Go duration string:

```bash
export FINNESTDB_ESTNLTK_TIMEOUT=1m
```

Setup:

```bash
make setup-estnltk
```

Evaluation:

```bash
make compare-parsers-et
# or directly
go run ./cmd/parsertest \
  -dataset testdata/parser-eval/et/gold/et-grammar-v1.json \
  -parsers basic,custom,estnltk
```

The adapter emits the same JSON shape as the Rust FFI parser and the
Omorfi adapter. The shared external-analyzer path then:

- records `source=analyzer:estnltk`
- preserves analyzer lemma, POS, and grammar label
- uses direct/custom dictionary overrides only when the analyzer returns
  an unresolved or `X` POS token
- attaches local grammar labels when they agree with analyzer lemma/POS

### ET lexical import sequence

Recommended path on a fresh machine (uses the bulk Ekilex pipeline; no
API key needed at deploy time, only during the offline scrape):

```bash
make import-dict-et            # kaikki.org Estonian
make import-ekilex-et          # tracked CC BY 4.0 public-word snapshot
make import-ekilex-details-et  # ~178k lemmas + ~6.2M form rows from
                                # the reduced Ekilex artifacts under
                                # localdata/ekilex/
make compare-parsers-et
```

The smoke-import path `make import-dict-et-ekilex` (which hits the live
Ekilex API per word, requires `EKILEX_API_KEY`) is intentionally configured
as a small smoke import. **Do not use it as the production data path** —
it is rate-limited, slow, and the bulk pipeline above is the canonical
loader. See `Makefile` and `cmd/fetchekilex` for the bulk-scrape recipe.

ET corrections follow the same shared correction path as Finnish — see
[`docs/PARSER_FEEDBACK_LOOP.md`](PARSER_FEEDBACK_LOOP.md).

## Resolution Layer

The parser's enrichment chain in
[`internal/store/dict.go`](../internal/store/dict.go) performs source-aware
form-to-lemma lookup:

1. Query all matching `forms` rows for a surface form, joined to
   row-level `source_priority`.
2. Pick the highest-priority row. Ties are broken deterministically by
   source name and surface plausibility.
3. For glosses, query `translations` and `definitions` ordered by
   priority; return the top N to the caller.

Default priority order for current sources:

- `custom_overrides` (1000, planned) > `ekilex` (20, ET) > `kaikki` (10)
  / `kotus` (10, FI)

`custom_overrides` is the existing `-custom-glosses` CSV path, promoted
to a real source.

### Risk to watch

Phase 2's gloss read path intentionally does **not** rank raw
`translations` rows in isolation. It joins each translation row back to
its co-written `lemmas` row on `(lemma, pos, lang, source)` and uses the
lemma row's `source_priority` to decide which translation wins.

That keeps one source of truth for priority and preserves the current
`-custom-glosses` behavior, but it also creates an invariant future
importers must preserve:

- If a source writes `translations`, it must also write or update the
  matching `lemmas` row with the same `source` and the intended
  `source_priority`.
- If a future adapter writes `translations` rows without a matching
  `lemmas.source`, `BatchLookupGlosses` will ignore those translations by
  design.
- A naive "translations first" query would regress custom overrides:
  stale `kaikki` translations would beat `lemmas.gloss` rows written by
  `-custom-glosses`.

Concrete follow-up: when `cmd/importekilexdetails` gains translations, it
must keep its `lemmas.source='ekilex'` rows in sync with the
`translations.source='ekilex'` rows it writes, or the new read path will
silently fall back to the wrong gloss source.

## Phasing

_Moved 2026-05-07 to [`docs/PARSER_EVOLUTION.md`](PARSER_EVOLUTION.md)
as dated entries. Phases 1–3 are the lexical-pipeline measurement
events recorded there; Phase 3.5 (Voikko generator spike) is captured
in [`experiments/2026-05-06-phase3.5-voikko-generator-spike.md`](../experiments/2026-05-06-phase3.5-voikko-generator-spike.md);
Phase 4 (Voikko seed) was superseded by the FST runtime path
(see [`docs/FST_LEMMATIZER.md`](FST_LEMMATIZER.md)); production
ET-table generation is tracked in [`TODO.md`](../TODO.md) "Parser
quality" section as the remaining piece._

## Migration Framework Plan

_Moved 2026-05-07 to [`TODO.md`](../TODO.md) "What's not in main yet"
as a research/engineering item. The current pattern (idempotent
`ALTER TABLE` + `EnsureXxx` helpers) is captured in
[Decision 12 in DECISIONS.md](DECISIONS.md) (2026-05-06)._

## Open Questions

Remaining items to confirm during implementation:

- **Production generated-table scope.** Pick the FI and ET word lists,
  table names, row-count targets, provenance format, and eval gates for
  the first production generated-table PR.
- **ET generation path.** Add a generator command for local Giellalt/HFST
  Estonian analyses, analogous to the current FI VFST smoke generator.
- **Exact morph-feature schema for `feats`.** UD features are the
  target, but upstream analysers expose native tag sets. The mapping
  tables should live next to the generator/parser code and be locked
  before production tables are promoted.
- **kaikki.org extraction of fi.wiktionary defs vs en.wiktionary
  glosses.** Phase 2 (#85) deferred this — the write path hard-codes
  `target_lang='EN'` because both FI and ET kaikki dumps are
  en.wiktionary extractions whose glosses are English. Loading
  fi.wiktionary's Finnish-language definitions into
  `definitions` (target_lang='FI') needs a separate kaikki dump
  (`https://kaikki.org/fiwiktionary/`) and a future import path —
  not blocking Phase 3/4.

## See Also

- [`docs/CROSS_LANGUAGE_STRATEGY.md`](CROSS_LANGUAGE_STRATEGY.md) — what
  is shared vs. language-specific
- [`docs/OMORFI_ADAPTER.md`](OMORFI_ADAPTER.md) — Omorfi as parser
  baseline; separate from generated-table shipping policy
- [`internal/parserules/finnish.go`](../internal/parserules/finnish.go)
  — runtime suffix stripping that complements stored forms
- [`cmd/importdict/main.go`](../cmd/importdict/main.go) — current
  single-source importer, refactored in Phase 2
- PRs in flight: [#66] (ET plan), [#67] (source priority), [#68]
  (Ekilex importer)
