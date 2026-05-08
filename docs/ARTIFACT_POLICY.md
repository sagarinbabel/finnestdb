# Artifact / licensing policy

This repo does **not** track third-party data artifacts in git, even when their
license technically permits redistribution. The motivation is twofold:

1. **Distribution clarity.** Upstream transducer blobs (`.vfst`, `.hfstol`,
   `.hfst`) are often distributed under copyleft licenses that make our own
   distribution terms ambiguous when redistributed alongside our code.
2. **Repo hygiene.** Even permissively-licensed corpora (kaikki dumps, Ekilex
   CC BY 4.0 shards, Kotus sanalista, Gutenberg silver) inflate the repo
   substantially and rot independently of the code that consumes them. They
   belong in a layer the runtime fetches, not in `git history`.

## What is **not** allowed in git

- Upstream transducer blobs (`.vfst`, `.hfstol`, `.hfst`) and any other
  binary runtime artifacts that directly embed/ship an upstream analyser.
- **Derived factual tables** generated offline from upstream analysers
  (e.g. `pkg/lemmatizer-fi-et/tables/*.json` produced by
  `cmd/genlemmatizertables` from Voikko's `mor.vfst` or Giellalt's
  `analyser-gt-desc.hfstol`). These are upstream content reshaped, not
  ours.
- **Bulk corpora** scraped/downloaded from external sources, even when
  CC BY / public-domain (Gutenberg silver, kaikki.org dumps, Kotus
  sanalista, Ekilex public-word/details shards).
- Generated SQLite databases (`finnestdb.db` and friends).

All of the above live under [`localdata/`](../localdata/), which is
**gitignored**. Generator/fetcher code lives in `cmd/` and writes there
exclusively.

### Single-folder bootstrap rule (2026-05-07)

`localdata/` is the **only** place gitignored runtime data lives. The
UD treebank cache, NC-licensed ET parser-eval gold, and FI/ET train
splits all moved here on 2026-05-07. There is no second data root.
This guarantees:

- A teammate handoff is always one tarball:
  `tar czf finnestdb-bootstrap.tgz localdata/ finnestdb.db`.
- A fresh `setup-local.sh` run leaves the working tree with `git status`
  empty (only `localdata/` and `finnestdb.db` change, both gitignored).
- The data ledger in [`docs/data_enhancement.md`](data_enhancement.md)
  has a single root path to track per source — no scattered exceptions.

When adding a new corpus or generator, write to a subdirectory of
`localdata/` from day one. Do NOT introduce a sibling like `data/`
or `corpora/`.

## What is allowed in git

- **Generator code** that, given local access to upstream analysers, can
  reproduce the factual tables (e.g. [`cmd/genlemmatizertables`](../cmd/genlemmatizertables/)).
- **Fetcher code** that can download/scrape source data into `localdata/`
  (e.g. [`cmd/fetchekilex`](../cmd/fetchekilex/), [`cmd/scrapegutenberg`](../cmd/scrapegutenberg/),
  [`cmd/importud`](../cmd/importud/)).
- **License / attribution text** for upstream sources (for auditability),
  in directories like `pkg/lemmatizer-fi-et/data/{fi,et}/LICENSE-*.txt`.
- **Tiny, hand-authored test fixtures** under `testdata/`. These must be
  unquestionably ours — never extracts from upstream content. Golden files
  for parser/reducer tests are fine; full grammar baselines are fine
  (they're our measurement output, not third-party content).
- **Eval baseline reports** under [`docs/baselines/`](baselines/). These
  are our measurement output (`cmd/parsertest` → `.json.gz`, plus markdown
  summaries). They're frozen per-PR for regression detection without keeping
  pretty-printed raw JSON as hundreds of thousands of docs lines — see
  [`docs/PARSER_EVOLUTION.md`](PARSER_EVOLUTION.md) for the cross-PR
  narrative.

## Reproducibility workflow

A new contributor (or a fresh CI environment) bootstraps via:

```sh
make parser              # builds the Rust shared library
scripts/setup-local.sh   # fetches/generates everything into localdata/, populates finnestdb.db
```

`scripts/setup-local.sh` is the single entry point. It chains every
fetch/generate/import target so a clean clone reaches a runnable state in
one command. See its banner for the per-source detail and for which
steps require an `EKILEX_API_KEY`.

For sharing the populated state with a teammate (e.g. so they can skip a
multi-hour Ekilex scrape), zip up `localdata/` and `finnestdb.db` and
deliver them out of band; both are gitignored and the receiver drops them
in place. In production this goes away — the server hosts the populated
DB and the repo carries no runtime data.

## Why "tracked factual tables" are now forbidden too

Earlier versions of this policy allowed offline-generated factual tables
(JSON/TSV with derived lemma/UD-features) to be committed since they were
"plain data, not blobs". Tightened on 2026-05-07 because:

- They are still upstream content, just reshaped — same redistribution
  question on a smaller scale.
- They rot when the upstream analyser version changes; tracking them
  hides the rot behind a green CI.
- The runtime is identical whether the tables are loaded from `go:embed`
  (build-time) or from disk (boot-time). Nothing about the design needs
  them in git.

The `pkg/lemmatizer-fi-et/tables/*.json` migration completed alongside
this policy: the generator writes to
[`localdata/lemmatizer-fi-et/tables/`](../localdata/) (gitignored),
[`pkg/lemmatizer-fi-et`](../pkg/lemmatizer-fi-et/) loads from disk on
`New()` (default `localdata/lemmatizer-fi-et/tables`, override via
`LEMMATIZER_TABLES_DIR`), and hand-authored test fixtures live under
[`testdata/lemmatizer/`](../testdata/lemmatizer/). When tables are
absent at runtime (e.g. fresh clone without
`scripts/setup-local.sh`), the FST step is disabled and the parser
falls back to the dict + case-suffix path with a single log line.

As of `2026.05.07k`, every analysis in those table JSONs carries an
explicit `Feats` field (e.g. `"Feats": "Case=Ine|Number=Sing"`)
composed by `pkg/lemmatizer-fi-et/udfeats::Compose` at parse time. The
runtime composer prefers the persisted field; older table snapshots
without `Feats` still load and recompose on the fly, so the policy
doesn't force regeneration. When a maintainer regenerates against
their local upstream analyser, the new `Feats` field falls out for
free — `genlemmatizertables` doesn't need a new flag.
