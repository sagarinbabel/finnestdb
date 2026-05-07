# Generated-table lemmatizer

This document describes the Finnish + Estonian lemmatizer package after
the artifact-policy cleanup. The runtime does **not** ship upstream
transducer blobs, and as of the
`pkg/lemmatizer-fi-et/tables/*` → `localdata/` migration it does not
ship the derived JSON tables either. Tables live under
`localdata/lemmatizer-fi-et/tables/` (gitignored) and are loaded from
disk on `New()`.

See also:

- [docs/ARTIFACT_POLICY.md](ARTIFACT_POLICY.md) for the repository policy.
- [docs/FST_LEMMATIZER_ROADMAP.md](FST_LEMMATIZER_ROADMAP.md) for the
  historical migration plan and what changed.
- [experiments/2026-05-06-phase3.5-voikko-generator-spike.md](../experiments/2026-05-06-phase3.5-voikko-generator-spike.md)
  for the generator spike that led to this package.

## Current status

[`pkg/lemmatizer-fi-et/`](../pkg/lemmatizer-fi-et/) provides:

- Disk-based loading of generated JSON analysis tables. `New()` resolves
  the directory from `LEMMATIZER_TABLES_DIR` (env), falling back to
  `localdata/lemmatizer-fi-et/tables/`. Per-language coverage is
  independent: a deployment that has only `fi_min.json` works for FI
  and returns no analyses for ET.
- `NewFromDir(dir string)` — explicit-path constructor used by tests
  (against [`testdata/lemmatizer/`](../testdata/lemmatizer/)) and by
  callers that don't want to touch env.
- Offline reader/parser packages for upstream analyser formats:
  - `vfst/` for local `mor.vfst` files.
  - `hfstol/` for local HFST optimised-lookup files.
- Mapping packages that normalize native analyser tags into the parser's
  `Analysis` shape:
  - `voikkomap/`
  - `giellaltmap/`
- [`cmd/genlemmatizertables`](../cmd/genlemmatizertables/), currently a
  Finnish VFST table generator that reads a local `mor.vfst` plus a
  seed wordlist and writes generated JSON to
  `localdata/lemmatizer-fi-et/tables/`.

Test fixtures under [`testdata/lemmatizer/`](../testdata/lemmatizer/)
are hand-authored and intentionally tiny — they cover the words used
by `pkg/lemmatizer-fi-et/lemmatizer_test.go` and nothing else. They
are **not** production tables; they are the unit-test ground truth.

## Artifact policy

Allowed in git:

- Generator code and tests.
- Hand-authored seed wordlists for the generator (e.g.
  [`cmd/genlemmatizertables/wordlists/fi_smoke.txt`](../cmd/genlemmatizertables/wordlists/fi_smoke.txt)).
- Hand-authored unit-test fixtures under
  [`testdata/lemmatizer/`](../testdata/lemmatizer/).
- Upstream license and attribution text under
  `pkg/lemmatizer-fi-et/data/{fi,et}/` (no transducer blobs).

Not allowed in git:

- `mor.vfst`, `analyser-gt-desc.hfstol`, `.hfst` files, or any other
  upstream analyser blob that directly ships the transducer.
- The factual JSON tables generated from those analysers
  (`{fi,et}_min.json` and any future production tables) — these belong
  under `localdata/lemmatizer-fi-et/tables/`, gitignored.

Maintainers may keep upstream analysers locally, for example under
`localdata/` or via an absolute path passed to a generator command. The
generated table is a runtime asset that is regenerated rather than
reviewed.

## Runtime architecture

```
parser query
  |
  v
internal/store.BatchLookupForms()
  |
  |-- existing SQLite dictionary and fallback rules
  |
  `-- pkg/lemmatizer-fi-et
        |
        |-- New() reads localdata/lemmatizer-fi-et/tables/fi_min.json
        |-- New() reads localdata/lemmatizer-fi-et/tables/et_min.json
        `-- return []Analysis for exact surface-form matches
```

The runtime path is deliberately simple:

1. `lemmatizer.New()` (or `NewFromDir`) reads each language's table from
   disk. Missing files degrade the language to "no analyses"; both
   files missing is a hard error.
2. `Lemmatize("FI", word)` returns entries from the FI table.
3. `Lemmatize("ET", word)` returns entries from the ET table.
4. Unknown languages or unknown words return no analyses.

If `New()` fails (for example, on a fresh clone where setup-local.sh
hasn't generated tables yet), `internal/store` logs a single warning
and falls back to the dict + case-suffix path. No transducer blob is
opened at runtime.

### Store-level candidate merge

`internal/store.BatchLookupForms` uses generated FST tables in two
places when `parserMode == "custom"`:

1. **Direct dictionary hits.** The store now treats dictionary rows and
   generated-table FST analyses as one candidate set for the surface
   form. Candidates are keyed by `(lemma, POS)`.
2. **Fallback misses.** If dictionary, possessive, compound, and
   suffix-strip paths all miss, the FST table can still provide a
   standalone resolution.

The direct-hit merge is deliberately conservative:

- When dictionary and FST agree on `(lemma, POS)`, the dictionary
  candidate remains the resolution and the FST analysis enriches it
  with `Feats` and the legacy `GrammarLabel` projection.
- When dictionary and FST disagree, the dictionary candidate normally
  wins. A disagreeing FST candidate can win only when the dictionary row
  is weak legacy data (no source priority and no morphology) and the FST
  has stronger case/POS/morphology evidence. A dictionary row with any
  FEATS or projected grammar label is treated as morphologically
  authoritative for now, even if the FST analysis is richer.
- If local FST tables are missing, behavior degrades to the dictionary
  path plus the existing case-suffix label stopgap.

FST morphology is projected into UD-style FEATS before it leaves the
store. For example, `GrammarLabel=inessive` plus `Number=Sing` becomes
`Case=Ine|Number=Sing`; verb analyses can carry
`Number=Sing|Mood=Ind|Tense=Pres|Person=1`. The legacy
`GrammarLabel` field remains for older grammar-label metrics, and is
back-projected from `Case=` when possible.

## Offline generation

Generated-table values use canonical alphabetical UD FEATS ordering, so
all producers emit the same string shape for the same morphology.

The Finnish generator:

```sh
make gen-lemmatizer-tables-fi VFST_PATH=/absolute/path/to/mor.vfst
```

That target runs:

```sh
mkdir -p localdata/lemmatizer-fi-et/tables
go run ./cmd/genlemmatizertables \
  -lang fi \
  -vfst "$VFST_PATH" \
  -wordlist cmd/genlemmatizertables/wordlists/fi_smoke.txt \
  -out localdata/lemmatizer-fi-et/tables/fi_min.json
```

`VFST_PATH` must point to a local Voikko `mor.vfst`. The file itself
must not be committed.

`scripts/setup-local.sh` invokes the FI generator best-effort — if
`VFST_PATH` is set, it generates; otherwise it skips with a warning
and the FST step in custom-mode parsing is disabled until tables exist.

The ET production path still needs a matching generated-table command
for a local Giellalt/HFST analyser. Until that exists and a production
word list is selected, the ET FST is disabled at runtime (no
`et_min.json` under `localdata/`).

## What the test fixtures prove

The fixtures under [`testdata/lemmatizer/`](../testdata/lemmatizer/)
prove that:

- The runtime can consume generated factual analyses without shipped
  transducer blobs.
- FI and ET can share one runtime package and `Analysis` shape.
- The runtime stays deterministic and testable on a hermetic file set.
- Future production tables can be reviewed as plain generated data.

They do **not** prove broad runtime coverage, grammar-label gains, or
final eval deltas. Any accuracy or coverage claim must be generated
from the exact production tables under `localdata/` on the branch
making the claim, with the generator command and upstream version
recorded alongside.

## Production-table promotion checklist

Before promoting this package as a production replacement for the older
dictionary/rule path:

1. Choose the production input word list for each language.
2. Generate FI and ET factual tables from local upstream analysers
   into `localdata/lemmatizer-fi-et/tables/`.
3. Record the generator command, upstream source/version, and table
   row counts in `docs/PARSER_EVOLUTION.md`.
4. Re-run parser eval against those local tables.
5. Update `docs/baselines/` only with results from runs that loaded
   production tables.

Until then, the package should be described as a generated-table
scaffold with smoke fixtures and offline analyser-reader support.

## Upstream attribution

The offline generation path may use:

- **libvoikko / voikko-fi** for Finnish VFST analyses.
- **HFST** for optimised-lookup tooling and format reference.
- **GiellaLT lang-fin / lang-est-x-utee** for Finnish and Estonian
  morphological resources.

License text and provenance notes live under
`pkg/lemmatizer-fi-et/data/{fi,et}/`. They are retained for auditability
because those upstream projects may be used during local table
generation, even though their transducer blobs are not committed.

## Follow-up work

- Add a production-sized FI word list and regenerate
  `localdata/lemmatizer-fi-et/tables/fi_min.json` into a properly named
  production table.
- Add an ET generator path from a local Giellalt/HFST analyser.
- Store table provenance in machine-readable metadata next to the JSON
  tables.
- Rebaseline parser eval only after production generated tables land.
