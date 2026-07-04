# Parser Eval Baselines

This directory contains **frozen baseline measurements** from
`cmd/parsertest`. They establish the reference accuracy/coverage/speed
the parser had at a given date so we can detect regressions or measure
improvements over time.

For the cross-measurement narrative — what changed between dates and
why — see [`../PARSER_EVOLUTION.md`](../PARSER_EVOLUTION.md). This
directory is the data; that doc is the story.

## Filename convention

Every committed baseline filename follows this canonical format:

```
YYYY-MM-DD<rev>-T<HHMM>Z-<dataset>.<ext>
```

Field-by-field:

| Component | Meaning | Example |
|---|---|---|
| `YYYY-MM-DD` | UTC date of the comparison-script run-start (`scripts/parser-comparison.sh`'s `RUN_TS`) | `2026-05-07` |
| `<rev>` | The single-letter parser-version stamp at measurement time, from [`internal/parsecore/parsecore.go`](../../internal/parsecore/parsecore.go)'s `ParserVersion` (e.g. `2026.05.07k` → `k`). Bumped manually when the *parser* changes; the time component disambiguates same-`<rev>` re-measures. | `k` |
| `-T<HHMM>Z-` | UTC hour+minute of run-start, dashes around | `-T1118Z-` |
| `<dataset>` | Gold-set name. For per-dataset JSONs: the slug derived from the input file basename ([`scripts/parser-comparison.sh`](../../scripts/parser-comparison.sh) post-PR [#146](https://github.com/sagarinbabel/finnestdb/pull/146) — earlier the JSON `name` field was used and v1/v2 collided). For cross-language summaries: `fi` or `et`. | `fi-grammar`, `ud-fi-tdt-test`, `fi`, `et` |
| `<ext>` | `.json.gz` for the raw `cmd/parsertest` reports; `.md` for the cross-language wide-format summaries | `.json.gz` / `.md` |

Full examples:

```
2026-05-07k-T1118Z-fi-core.json.gz       ← per-dataset raw report
2026-05-07k-T1118Z-ud-fi-tdt-test.json.gz ← per-dataset on UD test split
2026-05-07k-T1118Z-fi.md                 ← cross-language FI summary
2026-05-07k-T1118Z-et.md                 ← cross-language ET summary
```

The same convention applies to `cmd/ambiguityeval` / `make compare-ambiguity`
reports (see [`PARSER_EVAL_METHODOLOGY.md` §"Ambiguity and meaning-check
calibration"](../PARSER_EVAL_METHODOLOGY.md#ambiguity-and-meaning-check-calibration)):
a frozen ambiguity baseline follows `<dataset>` = `fi-ambiguity` / `et-ambiguity`,
e.g. `2026-07-04a-T0730Z-fi-ambiguity.json.gz`, so it sits alongside the headline
baselines under the same append-only discipline. As of this doc, no ambiguity
baseline has been frozen yet — `make compare-ambiguity` runs are ad hoc until a
maintainer runs the formal freeze step.

**Append-only at the baseline-ID level.** Older baseline IDs are never
renamed or removed from the history. Raw per-dataset JSON reports are stored as
`.json.gz` so the repo does not carry hundreds of thousands of pretty-printed
JSON lines in normal docs diffs. Tagged-style names (`-final-`,
`-feats-rich-`, `-post-fst-`) committed before the canonical convention landed
are left as-is apart from the `.gz` compression suffix. The cross-reference of
every baseline ID is maintained in [`SYSTEM_VERSIONING.md` § Parser evaluation baseline history](../SYSTEM_VERSIONING.md), which is also append-only ([#141](https://github.com/sagarinbabel/finnestdb/pull/141)).

**Why time, not just date.** Multiple baselines on the same day at the same
parser-version `<rev>` happen routinely — first measure pre-change, second
post-change, third after a fix. Without `T<HHMM>Z` they'd collide on
filename and the freeze step would silently overwrite (the same class of bug
as the v1/v2 dataset slug collision). With it, every freeze is uniquely
identified by its run-start moment.

The freeze step in [`PARSER_EVAL_METHODOLOGY.md §5`](../PARSER_EVAL_METHODOLOGY.md) is automated by
[`scripts/freeze-baseline.sh`](../../scripts/freeze-baseline.sh), which reads
the parser-version letter from `parsecore.ParserVersion`, derives the date
+ time from the comparison script's `RUN_TS`, and refuses to overwrite an
existing target file (so append-only is enforced mechanically, not just by
convention).

## How to read a baseline file

Each baseline `.json.gz` is the raw report emitted by `cmd/parsertest`,
compressed with deterministic gzip headers. `cmd/parser-compare` reads both
`.json` and `.json.gz`, and shell inspection works with `gunzip -c`:

```bash
gunzip -c docs/baselines/2026-05-07k-T1118Z-fi-core.json.gz | jq '.summary.custom'
```

The keys you care about are under `summary.<parser>`:

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

When a report is generated with `parsertest -stratified`, each parser
summary also carries a `stratification` object with three axis breakdowns
(`by_upos`, `by_oov`, `by_compound`). Each entry has `bucket`,
`expected_tokens`, `lemma_correct`, `pos_correct`, `full_correct`, and the
matching `*_accuracy` floats. The field is omitted from JSON when the flag
is off, so legacy baselines remain byte-identical. See
[`../PARSER_EVAL_METHODOLOGY.md`](../PARSER_EVAL_METHODOLOGY.md) §Stratified
breakdown for the bucket definitions and motivation.

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
freeze the new floor. Use [`scripts/freeze-baseline.sh`](../../scripts/freeze-baseline.sh)
so the canonical filename convention (above) is applied automatically:

```bash
# After a comparison script run that wrote to reports/parser-eval/
# (where RUN_TS is the YYYYMMDDTHHMMSSZ timestamp visible in those filenames)
scripts/freeze-baseline.sh "$RUN_TS"
```

The script compresses each per-dataset JSON into `.json.gz` and copies the
cross-language summaries into `docs/baselines/` under the canonical name. It
refuses to overwrite an existing target, so baseline IDs remain append-only.

## Current reference set

The newest committed baseline family is **`2026-05-12b-T1606Z`**. It is the
current logical reference set for parser-regression comparisons unless a task
explicitly asks for an older historical point.

Earlier **2026-05-07** sets remain useful historical references:

- **`2026-05-07-post-fst-*`** (j) — pre-FEATS-data state. The
  `feats_attributes` arrays in these reports are empty because the manual gold
  sets had no FEATS to score against; the FEATS comparison in
  `internal/eval/eval.go` is a no-op when gold has no FEATS.
- **`2026-05-07-feats-rich-*`** (k) — first baseline where every committed gold
  set carries UD FEATS and the per-FEATS-attribute table fires for all parsers.
  See [`2026-05-07-feats-rich.md`](2026-05-07-feats-rich.md) for the
  methodology and the omorfi/estnltk reference numbers.

Use the 2026-05-07 reports only when you need a specific before/after anchor
for the FEATS migration or early FST work.
