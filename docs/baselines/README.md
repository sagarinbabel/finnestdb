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
| `lemma_pos_accuracy` | Fraction where lemma AND POS *both* match — the dictionary-entry attachment metric. First-class for language-learning quality (entries are keyed by `(lemma, POS)`). Added 2026-05-07; baselines older than that have this field at 0 in JSON, but `cmd/parser-compare` recomputes it from per-case data when comparing against an older baseline directory. See [`../PARSER_EVAL_METHODOLOGY.md`](../PARSER_EVAL_METHODOLOGY.md) for the framing |
| `grammar_accuracy`| Fraction with correct grammar_label (only counts cases where gold has a label) |
| `full_accuracy`   | Fraction with all of lemma + POS + grammar + every gold FEATS attribute correct |
| `resolved_coverage` | Fraction of input tokens the parser resolved to a dictionary entry |
| `avg/p50/p95_case_duration_ms` | Per-case latency in milliseconds, sub-ms float (PR #103: nanosecond-precision timer) |
| `words_per_second` / `chars_per_second` | Aggregate throughput across the dataset (sum of non-PUNCT tokens or runes, divided by total wall time across all repeats; PR #103) |
| `avg_unique_forms` | Average distinct non-punctuation surface forms per case |
| `avg_resolved_tokens` / `avg_unresolved_tokens` | Average resolved vs unresolved non-punctuation tokens per case |
| `avg_*_ms` timing fields | Average analyzer, form lookup, gloss lookup, sentence resolution, and word enrichment time per case (sub-ms float) |

The per-case raw samples live under `cases[].duration_ms[<parser>].samples_ns`
as `int64` nanoseconds — those are what the summary `_ms` floats are
derived from. Older baselines (pre-PR #103) used a `samples` field in
integer milliseconds; reading the unit off the field name is the
forward-compatible way to interpret these.

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

## Current reference set

The newest committed baselines are dated **2026-05-07** with two suffixes:

- **`2026-05-07-post-fst-*`** (j) — pre-FEATS-data state. The `feats_attributes`
  arrays in these reports are empty because the manual gold sets had no
  FEATS to score against; the FEATS comparison in `internal/eval/eval.go`
  is a no-op when gold has no FEATS.
- **`2026-05-07-feats-rich-*`** (k) — first baseline where every committed
  gold set carries UD FEATS and the per-FEATS-attribute table fires for
  all parsers. See [`2026-05-07-feats-rich.md`](2026-05-07-feats-rich.md)
  for the methodology and the omorfi/estnltk reference numbers (≥99% on
  every attribute on every dataset). The `custom` parser scores 0% on
  FEATS in this entry because the live SQLite DB hasn't been re-imported
  with the new FEATS mappers yet — see the methodology doc for the
  re-import runbook.

Both sets are valid references; pick `feats-rich` for any FEATS-related
comparison and `post-fst` when you specifically want the pre-FEATS state.
