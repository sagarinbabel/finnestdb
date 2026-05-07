# Generated-table lemmatizer

This document describes the Finnish + Estonian lemmatizer package after
the artifact-policy cleanup. The runtime does **not** ship upstream
transducer blobs. It embeds generated factual tables under
`pkg/lemmatizer-fi-et/tables/`.

See also:

- [docs/ARTIFACT_POLICY.md](ARTIFACT_POLICY.md) for the repository policy.
- [docs/FST_LEMMATIZER_ROADMAP.md](FST_LEMMATIZER_ROADMAP.md) for the
  historical migration plan and what changed.
- [experiments/2026-05-06-phase3.5-voikko-generator-spike.md](../experiments/2026-05-06-phase3.5-voikko-generator-spike.md)
  for the generator spike that led to this package.

## Current status

`pkg/lemmatizer-fi-et/` provides:

- Runtime loading of generated JSON analysis tables:
  - `tables/fi_min.json`
  - `tables/et_min.json`
- Offline reader/parser packages for upstream analyser formats:
  - `vfst/` for local `mor.vfst` files.
  - `hfstol/` for local HFST optimised-lookup files.
- Mapping packages that normalize native analyser tags into the parser's
  `Analysis` shape:
  - `voikkomap/`
  - `giellaltmap/`
- `cmd/genlemmatizertables`, currently a Finnish VFST table generator
  that reads a local `mor.vfst` plus `tables/fi_wordlist.txt` and writes
  generated JSON.

The checked-in FI and ET tables are intentionally small smoke fixtures.
They prove the runtime, parser integration, and generated-table policy,
but they are **not** production replacements for full Voikko or Giellalt
coverage.

## Artifact policy

Allowed in git:

- Generated factual tables such as JSON, TSV, or SQLite dumps containing
  derived analyses.
- Word lists or manifests used to regenerate those tables.
- Generator code and tests.
- Upstream license and attribution text.

Not allowed in git:

- `mor.vfst`
- `analyser-gt-desc.hfstol`
- `.hfst` files
- Any other upstream analyser blob that directly ships the transducer.

Maintainers may keep upstream analysers locally, for example under
`localdata/` or via an absolute path passed to a generator command. The
generated table is the reviewable artifact that may be committed.

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
        |-- embed tables/fi_min.json
        |-- embed tables/et_min.json
        `-- return []Analysis for exact surface-form matches
```

The runtime path is deliberately simple:

1. `lemmatizer.New()` unmarshals the embedded JSON tables into maps.
2. `Lemmatize("FI", word)` returns entries from the FI table.
3. `Lemmatize("ET", word)` returns entries from the ET table.
4. Unknown languages or unknown words return no analyses.

No transducer file is opened at runtime, and no binary analyser data is
embedded in the Go binary.

## Offline generation

The current Finnish generator is:

```sh
make gen-lemmatizer-tables-fi VFST_PATH=/absolute/path/to/mor.vfst
```

That target runs:

```sh
go run ./cmd/genlemmatizertables \
  -lang fi \
  -vfst "$VFST_PATH" \
  -wordlist pkg/lemmatizer-fi-et/tables/fi_wordlist.txt \
  -out pkg/lemmatizer-fi-et/tables/fi_min.json
```

`VFST_PATH` must point to a local Voikko `mor.vfst`. The file itself must
not be committed.

The ET production path still needs a matching generated-table command
for a local Giellalt/HFST analyser. Until that exists and a production
word list is selected, `tables/et_min.json` remains a smoke fixture.

## What the current tables prove

The checked-in tables prove that:

- The parser can consume generated factual analyses without shipped
  transducer blobs.
- FI and ET can share one runtime package and `Analysis` shape.
- The runtime stays deterministic and testable.
- Future production tables can be reviewed as plain generated data.

They do **not** prove broad runtime coverage, grammar-label gains, or
final eval deltas. Any accuracy or coverage claim must be generated from
the exact production tables committed on the branch making the claim.

## Production-table promotion checklist

Before promoting this package as a production replacement for the older
dictionary/rule path:

1. Choose the production input word list for each language.
2. Generate FI and ET factual tables from local upstream analysers.
3. Commit the generated tables and provenance notes, but no analyser
   blobs.
4. Record the generator command, upstream source/version, and table row
   counts.
5. Re-run parser eval on the exact committed tables.
6. Update `docs/baselines/` and `docs/PARSER_EVOLUTION.md` only with
   those new results.

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

- Add a production-sized FI word list and regenerate `fi_min.json` into
  a properly named production table.
- Add an ET generator path from a local Giellalt/HFST analyser.
- Store table provenance in machine-readable metadata next to the JSON
  tables.
- Rebaseline parser eval only after production generated tables land.
