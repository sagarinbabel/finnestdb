# Corpus Pipeline Decisions

This file records durable decisions for the corpus pipeline. Keep it concise and
append-only: add a new dated section when the operating model changes instead
of rewriting history.

## 2026-05-08 - Track Pipeline Code, Keep Corpus Data Local

### Decision

Move the corpus pipeline source out of `localdata/corpus_pipeline/` into the
tracked repository folder `corpus_pipeline/`.

Track:

- Go command source
- shared internal packages
- Makefile targets
- Python batch adapter scripts
- operator docs
- smoke fixtures
- schemas
- small run reports and decision records

Do not track:

- downloaded corpora
- EPUBs
- generated TSVs
- scratch SQLite databases
- analyzer caches
- Python virtual environments
- omorfi/vabamorf binary data
- bootstrap tarballs

Runtime data remains under `localdata/`.

### Why

The first pipeline iteration lived entirely under `localdata/` to guarantee
zero Git pollution while the corpus design was unstable. That worked for the
pilot run, but it created a collaboration problem: Codex, Claude, and future
agents could only share context by copying large prompts or by reading
gitignored local files that may not exist in another checkout.

The pipeline is now useful enough to treat as product-supporting tooling. The
code should be reviewable, versioned, reproducible, and visible to any coding
agent working in this repo. The data should remain local-only because it is
large, partly license-sensitive, and already covered by the repository's
artifact policy.

Tracking the source but not the artifacts gives us both properties:

- agents can inspect the same implementation and docs without prompt-pasting
- local corpus runs remain disk-local and out of Git
- schema decisions survive across sessions
- run summaries can accumulate as historical evidence
- broken extractor/aggregator changes can be reviewed like normal code

### Report Policy

Reports in `corpus_pipeline/reports/` are append-only. Add a new timestamped
file for each meaningful run or schema decision. Do not overwrite historical
reports.

Large machine outputs stay in `localdata/{fi,et}-corpus/_derived/` and are not
committed. If a report needs numbers from those files, copy only compact
summaries into a timestamped report.

### Operational Consequences

Run the pipeline from:

```sh
cd corpus_pipeline
make <target>
```

By default, commands read and write corpus data under:

```sh
../localdata/{fi,et}-corpus/
```

The module path is `finnestdb/corpus_pipeline` with `replace finnestdb => ..`
so the commands can import the main repo's `internal/...` packages while living
outside `localdata/`.

The old local-only copy under `localdata/corpus_pipeline/` should be treated as
legacy once the tracked folder is in use.
