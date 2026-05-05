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
   (`testdata/seed/fi-voikko-forms.jsonl.gz`), and ships it as a static
   artifact. The runtime importer reads it like any other JSONL source.
   No cgo, no libvoikko at runtime, same import shape as kaikki.org.
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
6. **Schema migration: minimal `PRAGMA user_version` block now, real
   migration framework deferred** — see [Migration Framework Plan](#migration-framework-plan)
   below.
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
  for every Kotus lemma, call `voikkospell --paradigm` (or libvoikko's
  generator API via the bundled adapter script under `scripts/`),
  produce every surface form with morph features, write a JSONL file.
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

- `voikko` (30) > `kaikki` (20) > `kotus` (10) > `custom_overrides`
  (1000, always wins)

`custom_overrides` is the existing `-custom-glosses` CSV path, promoted
to a real source.

## Phasing

Each phase ends in a working system; nothing is half-built across PR
boundaries. Phase 1 cannot start until [#67] merges.

- **Phase 1 — FI schema delta.** `paradigm_class` on `lemmas`, `feats`
  on `forms`, new `translations` and `definitions` tables. Stacked on
  [#67]. No behavior change. Tests: schema migration round-trip;
  parser eval baseline unchanged.
- **Phase 2 — kaikki.org refactor.** Move EN glosses to `translations`,
  FI defs to `definitions`. Update enrichment to read from new tables
  with fallback to `lemmas.gloss`. Tests: parser eval baseline
  unchanged; UI still renders glosses.
- **Phase 3 — Kotus adapter.** Pull sanalista, populate
  `paradigm_class` on existing FI lemmas, insert any Kotus lemmas not
  yet present. Tests: lemma count grows; eval baseline unchanged.
- **Phase 4 — Voikko seed.** Offline-generate
  `data/voikko/fi-voikko-forms-<version>.jsonl.gz`, commit it, import
  via the new `cmd/importvoikko/` binary. Tests:
  consonant gradation, vowel harmony, *-nen* class, partitive plural
  resolve correctly via Voikko-priority rows. Compare against frozen
  baseline in `docs/baselines/`.
- **Phase 5 — Resolution priority flip.** With `dict_metadata.priority`
  populated, enrichment naturally picks Voikko over kaikki for forms.
  Re-run eval; freeze new baseline.

## Migration Framework Plan

For now, schema changes ride on a minimal pattern that mirrors what
[#67] uses: `PRAGMA user_version` checks at startup with conditional
`ALTER TABLE` / `CREATE TABLE` blocks in
[`internal/store/db.go`](../internal/store/db.go)'s `ensureSchema`.
This is acceptable while migrations are infrequent and append-only.

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
- Track applied versions in a `schema_migrations` table.
- Run forward-only at startup; rollback handled out-of-band by ops.
- Be a single PR — not introduced lazily alongside a feature.

## Open Questions

All major decisions are locked above. Remaining items to confirm during
implementation:

- **Exact morph-feature schema for `feats`.** UD features are the
  obvious target, but Voikko's native output uses its own tag set.
  The adapter script will normalize; mapping table goes in
  [`scripts/`](../scripts/) and gets reviewed alongside the Voikko
  seed PR.
- **kaikki.org extraction of fi.wiktionary defs vs en.wiktionary
  glosses.** Need to confirm kaikki.org's Finnish dump exposes both
  cleanly via `senses[*].glosses` plus a language-of-edition flag.
  Verify during Phase 2.

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
