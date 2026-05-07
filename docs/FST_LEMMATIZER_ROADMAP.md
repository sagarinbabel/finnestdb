# Generated-table lemmatizer roadmap

Status: policy-adjusted after PRs #107, #108, #110, and #112. The repo
now follows [docs/ARTIFACT_POLICY.md](ARTIFACT_POLICY.md): generated
factual tables may be committed; upstream transducer blobs may not.

Date: 2026-05-06 to 2026-05-07
Driver: Phase 3.5 spike, recorded in
[experiments/2026-05-06-phase3.5-voikko-generator-spike.md](../experiments/2026-05-06-phase3.5-voikko-generator-spike.md)

## What changed in plan

The original Finnish lexical plan had a Phase 4 that would
offline-generate millions of Finnish forms via a Voikko paradigm
generator and import them into SQLite. The Phase 3.5 spike found that
the released Voikko tooling exposes analysis, not paradigm generation,
through the practical public interfaces available to this project.

The first implementation pass then explored pure-Go readers for Voikko
VFST and HFST optimised-lookup analyser formats. That code remains
useful for local generation and test fixtures, but the shipping policy
changed before merge: the repository should not commit or embed
transducer blobs.

The accepted architecture is therefore:

1. Keep analyser readers and tag mappers as generator support.
2. Generate factual analysis tables offline from locally available
   upstream analysers.
3. Commit only those factual tables and their provenance.
4. Embed generated tables in the runtime.

## Current architecture

`pkg/lemmatizer-fi-et/` contains:

- `lemmatizer.go` - runtime loader that reads FI/ET JSON tables from
  `localdata/lemmatizer-fi-et/tables/` (default) or
  `LEMMATIZER_TABLES_DIR`. The tables themselves are not tracked.
- `vfst/` - reader for local Voikko VFST files, used by generators and
  tests.
- `hfstol/` - reader for local HFST optimised-lookup files, used by
  generators and tests.
- `voikkomap/` and `giellaltmap/` - tag normalization into the parser's
  `Analysis` struct.

Adjacent inputs (committed, not under `pkg/`):

- [`testdata/lemmatizer/{fi,et}_min.json`](../testdata/lemmatizer/) -
  hand-authored unit-test fixtures; `lemmatizer_test.go` loads these
  via `NewFromDir`.
- [`cmd/genlemmatizertables/wordlists/fi_smoke.txt`](../cmd/genlemmatizertables/wordlists/fi_smoke.txt) -
  seed word list for the current FI smoke generator run.

No `.vfst`, `.hfstol`, or `.hfst` files are committed.

## PR sequence after policy cleanup

| PR | Scope after cleanup | Merge gate |
|---|---|---|
| #107 | FI table-backed lemmatizer scaffold, VFST reader/generator support, minimal FI smoke table | No vendored blobs; no production eval claims unless production table is committed |
| #108 | HFST optimised-lookup reader and Giellalt FI tag mapping support for offline generation | No vendored blobs; no claim that runtime uses a production Giellalt FI table |
| #110 | ET table runtime path and minimal ET smoke table | No vendored blobs; ET eval claims deferred until production ET table exists |
| #112 | Documentation aligned with generated-table policy | No stale references to shipped transducers, embedded binary growth, or old final deltas |

## Baseline policy

The pre-FST baselines remain useful as historical reference points.
Post-PR and final eval snapshots from the earlier blob-backed runtime
were removed or deferred because they were not generated from production
tables committed under the current policy.

Future eval snapshots must state:

- exact table file names;
- table row counts;
- upstream source/version;
- generator command;
- commit SHA used for the eval.

## Promotion path

The next production-oriented PR should not change policy again. It
should add actual generated tables:

1. Choose production word lists for FI and ET.
2. Generate tables from local upstream analysers.
3. Commit generated tables plus provenance metadata.
4. Run `go test ./...` and `cargo test`.
5. Re-run parser comparison scripts.
6. Add new baseline docs that explicitly name the generated tables.

Until then, the current runtime is a scaffold with smoke fixtures.

**2026-05-07k status**: smoke fixtures regenerated with the new `Feats`
field on every analysis (composed by `pkg/lemmatizer-fi-et/udfeats`).
Future regenerations of the production tables pick up the field
automatically — no flag, no schema migration. The runtime composer at
[`internal/store::featsFromFSTAnalysis`](../internal/store/dict.go)
prefers the persisted field, so once production tables ship the
parser will short-circuit the on-the-fly composition for direct hits.

## Attribution

The generator path may depend on these upstream projects:

- **libvoikko / voikko-fi** for Finnish analyses.
- **HFST** for optimised-lookup tooling and format reference.
- **GiellaLT lang-fin / lang-est-x-utee** for Finnish and Estonian
  morphology.

License and attribution files under `pkg/lemmatizer-fi-et/data/` are
kept for auditability. They do not mean the corresponding transducer
blobs are committed.
