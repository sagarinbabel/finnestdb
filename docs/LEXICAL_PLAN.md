# Finnish Lexical Plan

This is the working plan for making Finnish lexical coverage first-class.
It is the Finnish counterpart to
[`docs/ESTONIAN_LEXICAL_PLAN.md`](ESTONIAN_LEXICAL_PLAN.md). Schema-level
groundwork (multi-source rows, source priority, attribution metadata) is
shared and lives in the ET track — see [#66] and [#67]. This doc covers
only what is Finnish-specific.

[#66]: https://github.com/sagarinbabel/finnestdb/pull/66
[#67]: https://github.com/sagarinbabel/finnestdb/pull/67
[#68]: https://github.com/sagarinbabel/finnestdb/pull/68

## Current Decision

Finnish has no single Sonaveeb-equivalent. We combine three open sources
into the same lemma/form/translation tables, tagged by `source`, and
resolve at query time by `source_priority` (mechanism landing in [#67]).

| Source | Role | License |
| --- | --- | --- |
| **Kotus sanalista** | Authoritative lemma list with Kotus inflection class (51 nominal + 76 verb classes) | CC BY 4.0 |
| **Voikko** | Generator: expands `(lemma, Kotus class)` into the full surface paradigm with morph features | GPL/LGPL — generator binary; output is data |
| **kaikki.org (Wiktionary)** | Translations, monolingual definitions, irregular forms not covered by Kotus classes | CC BY-SA 3.0 |

Deliberately not used:

- **Kielitoimiston sanakirja** — authoritative monolingual FI definitions,
  but redistribution is restricted. Not bulk-imported. Revisit only if
  Wikisanakirja coverage proves insufficient and only as a runtime
  lookup that respects the license.
- **fi.wiktionary directly** — kaikki.org already extracts and
  normalizes Wiktionary, including the Finnish edition. Use it.

## Why This Path

- Voikko is rule-based, so we *compute* the Finnish paradigm rather than
  trust scraped tables. This is more reliable than any single online
  dictionary for inflection, including for words Wiktionary covers
  poorly (rare nouns, neologisms, derivations).
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

These were decided before implementation began.

1. **Voikko deployment: precompute paradigms offline.** The build pipeline
   runs Voikko once, produces a JSONL seed file
   (`data/voikko/fi-voikko-forms-<version>.jsonl.gz`), and ships it as a
   static artifact. The runtime importer reads it like any other JSONL
   source. No cgo, no libvoikko at runtime, same import shape as
   kaikki.org.
2. **Translations and definitions tables land now**, not after Sonaveeb
   integration. The Finnish plan needs them; the Estonian plan benefits
   from them; landing them once avoids two parallel solutions.
3. **Storage budget acceptable** for the ~6M Voikko-generated form rows
   (~300–500MB on disk for FI). No on-demand generation.
4. **Adapter packaging: separate `cmd/` binaries per rich source**, matching
   the precedent set by `cmd/importekilex/` on main. New binaries:
   `cmd/importkotus/` and `cmd/importvoikko/`. `cmd/importdict/` stays
   the kaikki.org/Wiktionary importer. Each binary handles its own
   input shape (XML, JSONL, generated lookups) and shares only the
   schema bootstrap pattern. (Note: PRs [#67] and [#68] use a
   `-source-key` flag inside `cmd/importdict/` instead — that work
   predates `cmd/importekilex/` landing on main and will likely rebase
   against the separate-binary pattern.)
5. **Kotus data source: official Kotus distribution**
   (https://kaino.kotus.fi/sanat/nykysuomi/), not Voikko's joukahainen
   re-export. The official distribution is more likely to be maintained
   and is the canonical authority for Kotus class assignments.
6. **Schema migration: idempotent `ALTER TABLE` with duplicate-column
   error tolerance** — the established codebase pattern (see
   `EnsureDictionarySourceColumns` in `internal/store/db.go`, landed in
   #67). Real migration framework deferred — see
   [Migration Framework Plan](#migration-framework-plan) below. (The
   original draft of this doc proposed `PRAGMA user_version`; aligning
   with the existing convention is cleaner than introducing a parallel
   mechanism.)
7. **Wikisanakirja for monolingual FI definitions** (via kaikki.org's
   Finnish edition extract). Kielitoimiston not in scope for alpha.
8. **`feats` column not backfilled for existing kaikki.org form rows.**
   Voikko-generated rows will fill in features at higher priority;
   leaving kaikki rows with `feats=NULL` is acceptable.

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

### `cmd/importvoikko/` (source key `voikko`, default priority 30)

The bit that makes Finnish actually nicer than Estonian: rule-based
paradigm computation rather than scraped tables.

- **Offline build step** (run once per Voikko/sanalista version):
  for every Kotus lemma, call into Voikko to enumerate the inflected
  forms. The exact entry point is the open question Phase 3.5 (the
  Voikko generator spike) closes — the installed `voikkospell` ships
  morphological *analysis* (`-m` / `-M`), not a `--paradigm`
  *generator* despite some older docs implying otherwise. Likely
  candidates: libvoikko's C API via Python (`pyvoikko` /
  `python3-libvoikko`), the standalone `voikkogen` tool when
  available, or driving `vislcg3` / Giellatekno-style FSTs over the
  same morphological data Voikko is built from. The spike commits
  the chosen path before this phase produces the full seed.
- Ship the JSONL under `data/voikko/` (mirroring `data/ekilex/`),
  e.g. `data/voikko/fi-voikko-forms-<version>.jsonl.gz`.
- `cmd/importvoikko/` reads it and writes rows to `forms` with
  `source='voikko'` and `feats` populated.

Volume estimate:

- ~70k nominals × ~28 productive forms ≈ 2M rows
- ~25k verbs × ~150 forms ≈ 3.7M rows
- Total ≈ 6M form rows, ~300–500MB on disk.

### `cmd/importdict/` — kaikki.org (source key `kaikki`, default priority 20)

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

## Resolution Layer

The parser's enrichment chain in
[`internal/store/dict.go`](../internal/store/dict.go) currently does
form-to-lemma lookup with no source awareness. Once [#67] lands and the
Finnish adapters are populating rows, enrichment is extended to:

1. Query all matching `forms` rows for a surface form, joined to
   `dict_metadata.priority` via `(lang, source)`.
2. Pick the highest-priority row. Ties broken deterministically by
   source name.
3. For glosses, query `translations` and `definitions` ordered by
   priority; return the top N to the caller.

Default priority order for FI:

- `custom_overrides` (1000, always wins) > `voikko` (30) > `kaikki` (20)
  > `kotus` (10)

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

Each phase ends in a working system; nothing is half-built across PR
boundaries.

- **Phase 1 — FI schema delta.** `paradigm_class` on `lemmas`, `feats`
  on `forms`, new `translations` and `definitions` tables. No behavior
  change. Shipped in #76 once #67 merged. Tests: schema migration
  round-trip; parser eval baseline unchanged.
- **Phase 2 — kaikki.org refactor.** Shipped 2026-05-06 across four
  PRs. Move EN glosses to `translations` (#85 for kaikki, #89 for
  Ekilex extending the same pattern to ET). Update enrichment to
  read from `translations` with fallback to `lemmas.gloss` (#86).
  Eval baseline unchanged on the post-#84 reference DB.
  FI definitions (target_lang='FI') still pending an fi.wiktionary
  import path; not blocking subsequent phases. See
  [`docs/PARSER_EVOLUTION.md`](PARSER_EVOLUTION.md) for the
  measurement entry.
- **Phase 3 — Kotus adapter.** Shipped 2026-05-06 across two PRs.
  PR 3.1 ([#92](https://github.com/sagarinbabel/finnestdb/pull/92))
  landed the `cmd/importkotus` binary against an assumed XML schema.
  PR 3.2 replaced the parser with the real TSV format from the
  official 2024 distribution
  (`https://kaino.kotus.fi/lataa/nykysuomensanalista2024.txt`) and
  committed the file as a tracked artifact under
  [`data/kotus/`](../data/kotus/) (CC BY 4.0). After import,
  ~49k FI lemmas carry a populated `paradigm_class`. Eval baseline
  unchanged — Phase 3 is metadata enrichment; Phase 4 (Voikko) is
  what makes it pay off. See
  [`docs/PARSER_EVOLUTION.md`](PARSER_EVOLUTION.md) for the
  measurement entry.
- **Phase 3.5 — Voikko generator spike. (Done, 2026-05-06.)** Confirmed
  that none of the candidate generation paths (`voikkospell --paradigm`,
  `voikkogen`, libvoikko Python wrapper, libvoikko C API) actually
  exposes a generator: the runtime is hardcoded analyzer-direction.
  The Giellalt source-build path (HFST + lang-fin) does work and
  produces correct paradigm forms, but the spike also surfaced a
  better architecture overall — see Phases 4–5 below. Full report:
  [experiments/2026-05-06-phase3.5-voikko-generator-spike.md](../experiments/2026-05-06-phase3.5-voikko-generator-spike.md).
- **Phase 4 — Voikko seed.** ~~Offline-generate
  `data/voikko/fi-voikko-forms-<version>.jsonl.gz`, commit it, import
  via the new `cmd/importvoikko/` binary.~~
  **Superseded by the FST-runtime architecture below.** A 6M-row
  static seed turned out to be the wrong artifact — porting the FST
  *runtime* to Go gives faster lookups, smaller disk footprint, and
  union coverage of Voikko + Giellalt in one move. See
  [docs/FST_LEMMATIZER_ROADMAP.md](FST_LEMMATIZER_ROADMAP.md).
- **Phase 4 (replacement) — FST lemmatizer in Go. (Done, 2026-05-06.)**
  New package `pkg/lemmatizer-fi-et/` with the Voikko VFST and HFST
  optimised-lookup runtimes ported to Go, embedded data files for both
  FI (Voikko + Giellalt lang-fin) and ET (Giellalt lang-est-x-utee),
  and a unified `Lemmatize(lang, word)` entry point wired into
  `internal/store/dict.go::BatchLookupForms` step 5. Verified
  bit-identical to upstream `voikkospell -m` and `hfst-lookup` on
  representative samples. Eval delta vs the pre-FST baseline:
  fi-manual-v1 lemma +1.5 / full +22.9 / coverage +5.7 pts;
  et-manual lemma +11.1 / full +11.1 / coverage +8.3 pts; first-time
  grammar_label coverage on every dataset. See
  [docs/FST_LEMMATIZER.md](FST_LEMMATIZER.md) for full architecture,
  attribution, performance numbers, and rationale.
- **Phase 5 — Estonian via Giellalt. (Done, 2026-05-06.)** Subsumed
  into the Phase 4 work above — same FST runtime, same merge logic;
  Estonian uses Giellalt's `lang-est-x-utee` morphology since Voikko
  has no Estonian model.

## Migration Framework Plan

For now, schema changes ride on the established codebase pattern: each
migration is an idempotent `ALTER TABLE ... ADD COLUMN` (or
`CREATE TABLE IF NOT EXISTS`) that tolerates the SQLite "duplicate
column name" error on re-run. Grouped backfills get exported helpers
named `EnsureXxx` in [`internal/store/db.go`](../internal/store/db.go),
called by both the server's `ensureSchema` and any standalone importer
that needs the same shape. See `EnsureDictionarySourceColumns` (from
[#67]) and `EnsureLexicalEnrichmentColumns` (from Phase 1) as
references. No `PRAGMA user_version` is used. This is acceptable while
migrations are infrequent and append-only.

A real migration framework is deferred until at least one of these is
true:

- We need a non-additive migration (column rename, backfill that
  cannot be expressed as an idempotent `ALTER`, or a data
  transformation that depends on prior state).
- We have more than ~5 versioned migrations and the conditional-block
  pattern starts producing merge conflicts.
- We need rollback support.

When that happens, the framework should:

- Live under `internal/store/migrations/` as numbered SQL files
  (`0001_initial.sql`, `0002_source_priority.sql`, ...).
- Track applied versions in a `schema_migrations` table (or `PRAGMA
  user_version`, decided at framework introduction).
- Run forward-only at startup; rollback handled out-of-band by ops.
- Be a single PR — not introduced lazily alongside a feature.

## Open Questions

All major decisions are locked above. Remaining items to confirm during
implementation:

- **Voikko paradigm-generation entry point.** The installed
  `voikkospell` ships `-m` / `-M` for morphological analysis, not a
  `--paradigm` generator. Phase 3.5 spike resolves the actual
  generation path (libvoikko via Python, `voikkogen`, or FST-based
  alternative) before Phase 4 commits to the full seed.
- **Exact morph-feature schema for `feats`.** UD features are the
  obvious target, but Voikko's native output uses its own tag set.
  The adapter script will normalize; mapping table goes in
  [`scripts/`](../scripts/) and is locked alongside the Phase 3.5
  spike (so Phase 4 only writes; it doesn't decide).
- **kaikki.org extraction of fi.wiktionary defs vs en.wiktionary
  glosses.** Phase 2 (#85) deferred this — the write path hard-codes
  `target_lang='EN'` because both FI and ET kaikki dumps are
  en.wiktionary extractions whose glosses are English. Loading
  fi.wiktionary's Finnish-language definitions into
  `definitions` (target_lang='FI') needs a separate kaikki dump
  (`https://kaikki.org/fiwiktionary/`) and a future import path —
  not blocking Phase 3/4.

## See Also

- [`docs/ESTONIAN_LEXICAL_PLAN.md`](ESTONIAN_LEXICAL_PLAN.md) — sibling
  plan; schema groundwork lives there
- [`docs/CROSS_LANGUAGE_STRATEGY.md`](CROSS_LANGUAGE_STRATEGY.md) — what
  is shared vs. language-specific
- [`docs/OMORFI_ADAPTER.md`](OMORFI_ADAPTER.md) — Omorfi as parser
  baseline; uses the same FSTs Voikko does, but is a separate concern
  from data seeding
- [`internal/parserules/finnish.go`](../internal/parserules/finnish.go)
  — runtime suffix stripping that complements stored forms
- [`cmd/importdict/main.go`](../cmd/importdict/main.go) — current
  single-source importer, refactored in Phase 2
- PRs in flight: [#66] (ET plan), [#67] (source priority), [#68]
  (Ekilex importer)
