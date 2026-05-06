# Parser Eval Baselines

This directory contains **frozen baseline measurements** from
`cmd/parsertest`. They establish the reference accuracy/coverage/speed
the parser had at a given date so we can detect regressions or measure
improvements over time.

For the cross-measurement narrative — what changed between dates and
why — see [`../PARSER_EVOLUTION.md`](../PARSER_EVOLUTION.md). This
directory is the data; that doc is the story.

## How to read a baseline file

Each `YYYY-MM-DD-<dataset>.json` is the raw report emitted by
`cmd/parsertest`. The keys you care about are under `summary.<parser>`:

| Field | Meaning |
|---|---|
| `expected_tokens` | Total annotated tokens in the dataset |
| `lemma_accuracy`  | Fraction with correct lemma |
| `pos_accuracy`    | Fraction with correct POS |
| `grammar_accuracy`| Fraction with correct grammar_label (only counts cases where gold has a label) |
| `full_accuracy`   | Fraction with all of lemma + POS + grammar correct |
| `resolved_coverage` | Fraction of input tokens the parser resolved to a dictionary entry |
| `avg/p50/p95_case_duration_ms` | Per-case latency in milliseconds |
| `avg_unique_forms` | Average distinct non-punctuation surface forms per case |
| `avg_resolved_tokens` / `avg_unresolved_tokens` | Average resolved vs unresolved non-punctuation tokens per case |
| `avg_*_ms` timing fields | Average analyzer, form lookup, gloss lookup, sentence resolution, and word enrichment time per case |

## How to reproduce

```bash
make parser  # builds the Rust shared library
export LD_LIBRARY_PATH="$(pwd)/parser/target/release:$LD_LIBRARY_PATH"

# Requires a populated SQLite dictionary (see `make import-dict-fi`)
go run ./cmd/parsertest \
    -dataset testdata/parser-eval/fi/gold/<dataset>.json \
    -parsers basic,custom \
    -warmup 2 -repeat 5
```

## How to compare

When you change the parser or rules, re-run the same command, then diff
your fresh report against the matching baseline file in this directory.
Any drop in accuracy is a regression and needs justification.

## Updating a baseline

Only commit a new baseline when the parser improves and you want to
freeze the new floor. Always include the date in the filename and
**keep older baselines** so we have a history.
