# Parser Evaluation Methodology

How `make compare-parsers` / `make compare-parsers-et` actually work, what the
numbers mean, and how to reproduce a baseline on a fresh machine. Companion to:

- [`PARSER_EVAL_DATASETS.md`](PARSER_EVAL_DATASETS.md) — how the gold sets were curated and what to add
- [`PARSER_EVOLUTION.md`](PARSER_EVOLUTION.md) — date-stamped trend of frozen baselines
- [`baselines/README.md`](baselines/README.md) — the JSON report field schema
- [`SYSTEM_VERSIONING.md`](SYSTEM_VERSIONING.md) — `parser-vN` and `parser-baseline-YYYY-MM-DD-N` conventions
- [`OMORFI_ADAPTER.md`](OMORFI_ADAPTER.md) — Finnish external analyzer adapter

This doc is about **the process**: what to run, what input it consumes, what
output it produces, and how to read that output.

## What we measure

Per dataset, per parser, on **non-PUNCT tokens** that the gold answer marks for
evaluation:

| Metric | Definition |
|---|---|
| **Lemma accuracy** | Fraction of evaluated tokens where the parser's lemma matches gold |
| **POS accuracy** | Fraction where Universal POS matches gold |
| **Grammar accuracy** | Fraction where `grammar_label` matches gold (only on tokens where gold *has* a label — e.g. case-name for nouns) |
| **Full accuracy** | Fraction where lemma AND POS AND grammar all match |
| **Resolved coverage** | Fraction of input tokens the parser resolved to a dictionary entry (not "unknown") |
| **Avg/p50/p95 case time** | Per-case wall time (sub-millisecond, nanosecond-precision since PR #103) |
| **Throughput** | Aggregate tokens/sec and chars/sec across the full dataset |

Schema details: [`baselines/README.md`](baselines/README.md).

## Parsers under comparison

| Parser | What it is | What it tests |
|---|---|---|
| `basic` | Tokenize + lowercase + direct SQLite form lookup | Pure dictionary recall |
| `custom` | Basic + multi-step enrichment (possessive strip, compound split, case-suffix matcher) plus parallel-FST scoring inside dict step 1 (post-PR #127) and FEATS-aware FST candidate merge (post-PR #129) | The actual finnestdb runtime |
| `omorfi` | Helsinki HFST analyzer for Finnish (external, via Python adapter) | Reference upper bound for FI |
| `estnltk` | Vabamorf via EstNLTK for Estonian (external, via Python venv adapter) | Reference upper bound for ET |

`omorfi` and `estnltk` are **required by default** in committed baselines so
dict-only numbers (basic / custom) are never read in isolation.

## What "a baseline" is

A baseline is a frozen measurement of `custom` (and the analyzers) on a
specific git commit, against a specific dictionary state, against a specific
gold set. It includes the raw JSON reports under [`baselines/`](baselines/) and
a markdown summary. The cross-baseline narrative — what changed and why —
lives in [`PARSER_EVOLUTION.md`](PARSER_EVOLUTION.md).

A baseline is reproducible only if all three inputs are reproducible:

1. **Code** — pinned to a git commit.
2. **Dictionary** — the SQLite DB, populated by the importers under `cmd/import*/`
   and Make targets like `make import-dict-fi-recommended`. Different sources
   loaded → different `(forms, lemmas)` tables → different numbers. The
   per-baseline summary records which sources were active at measurement time.
3. **FST tables** — by [`ARTIFACT_POLICY.md`](ARTIFACT_POLICY.md), upstream
   transducer blobs aren't vendored. After PR [#128](https://github.com/sagarinbabel/finnestdb/pull/128),
   the runtime disk-loads two-role JSON tables:
   - **Test fixtures (committed):** `testdata/lemmatizer/{fi,et}_min.json` —
     12-form smoke covers exactly the words exercised by lemmatizer unit
     tests. Used in tests; not the production lookup.
   - **Production tables (gitignored):** `localdata/lemmatizer-fi-et/tables/`
     — populated locally via `make gen-lemmatizer-tables-fi VFST_PATH=…`
     against an upstream Voikko `mor.vfst` (or equivalent for ET). The runtime
     calls `lemmatizer.New()` which loads from this directory by default
     (override with `LEMMATIZER_TABLES_DIR`).
   Maintainer machines with locally-regenerated production tables get
   FST-stage lifts that fresh clones do not — **the main reason the
   2026-05-06i FINAL baseline is not reproducible from a fresh clone** (see
   PARSER_EVOLUTION.md §2026-05-07j).

## Reproducing a baseline (end-to-end)

### 0. Prerequisites

- macOS or Linux. Linux apt path uses system `python3-hfst`; macOS uses
  per-tool venvs.
- Go (whatever the project's `go.mod` requires)
- Rust (for `cargo build --release` of the tokenizer)
- Python 3.10+
- ~10 GB free disk for SQLite DB + analyser models

### 1. Build the tokenizer

```
make parser
export DYLD_LIBRARY_PATH="$(pwd)/parser/target/release:$DYLD_LIBRARY_PATH"   # macOS
export LD_LIBRARY_PATH="$(pwd)/parser/target/release:$LD_LIBRARY_PATH"      # Linux
```

### 2. Populate the dictionary

```
make import-dict-fi-recommended
make import-dict-et-recommended    # includes Ekilex bulk drop if localdata is populated
make verify-dict                   # sanity check row counts per source
```

The recommended ET path needs `localdata/ekilex/{definitions,forms}/`
populated. On a fresh machine these are empty (per the artifact policy). Run
`make fetch-ekilex && make reduce-ekilex` first if you want the ~6.18M ET form
rows; expect a multi-hour scrape. Without it, ET uses kaikki + Ekilex public
headwords only (~390k forms). The summary `.md` of every committed baseline
records the dict state it was measured against.

### 3. Install external analyzers

**Finnish (omorfi 0.9.12, pure-Python via `pyhfst` since 2026-05):**

```
python3 -m venv .venv-omorfi
.venv-omorfi/bin/pip install omorfi
mkdir -p ~/.cache/omorfi
curl -sL -o ~/.cache/omorfi/models.tar.xz \
  https://github.com/flammie/omorfi/releases/download/v0.9.12/omorfi-hfst-models-0.9.12.tar.xz
( cd ~/.cache/omorfi && tar xf models.tar.xz )
export FINNESTDB_OMORFI_CMD="$(pwd)/.venv-omorfi/bin/python $(pwd)/scripts/omorfi_adapter_example.py"
```

The `make setup-omorfi` target does this on Linux but its apt-based step is a
no-op on macOS; the per-tool venv path above works cross-platform. omorfi
0.9.12 dropped the `hfst` C library in favour of `pyhfst` (pure Python) so
this no longer requires HFST C builds on macOS arm64.

**Estonian (estnltk via EstNLTK):**

```
make setup-estnltk
```

Creates `.venv-estnltk/` and downloads `nltk_data` to `.cache/nltk_data/`.
First call after install can take 30s+ as NLTK builds its font cache.

### 4. Run the comparison scripts

```
bash scripts/parser-comparison.sh -o reports/parser-eval/$(date +%Y-%m-%d)-fi.md
bash scripts/parser-comparison-et.sh -o reports/parser-eval/$(date +%Y-%m-%d)-et.md
```

Default discovery: every `testdata/parser-eval/{fi,et}/gold/*.json` not matching
`-dev-v` (held-out test discipline — dev sets are for per-commit "watch" eval,
not committed baselines). Each dataset gets its own JSON report under
`reports/parser-eval/${RUN_TS}-${name}.json`. The markdown summary aggregates
them.

### 5. Freeze a baseline

```
DATE=$(date +%Y-%m-%d)
TAG=post-fst       # or whatever you're naming this iteration
for f in reports/parser-eval/${RUN_TS}-fi-*.json reports/parser-eval/${RUN_TS}-et-*.json; do
    cp "$f" docs/baselines/${DATE}-${TAG}-$(basename "$f" | sed "s/^${RUN_TS}-//")
done
cp reports/parser-eval/${DATE}-fi.md docs/baselines/${DATE}-${TAG}-fi.md
cp reports/parser-eval/${DATE}-et.md docs/baselines/${DATE}-${TAG}-et.md
```

Then add a row to [`PARSER_EVOLUTION.md`](PARSER_EVOLUTION.md)'s trend table
and an `### YYYY-MM-DDx` entry per the convention there.

## Reading the JSON reports

Each JSON file is one dataset run with one parser set. Top-level keys:

```
run_id, generated_at, dataset, parsers, git_commit, benchmark
```

Where `benchmark.summary.{basic,custom,omorfi,estnltk}` carries the headline
metrics, `benchmark.cases[i].duration_ms.{parser}.samples_ns` carries the raw
nanosecond per-iteration timings, and `dataset.cases[i]` carries the gold token
expectations (so the report is fully self-contained for re-evaluation).

Full schema: [`baselines/README.md`](baselines/README.md).

## Held-out discipline

Test sets and dev sets are split:

- `gold/<name>-test-v*.json` — committed baselines run on these. They are the headline.
- `gold/<name>-dev-v*.json` — committed baselines **skip** these (filtered by the comparison scripts via `grep -v -- '-dev-v'`). Use them for per-commit "watch" eval (`-dataset` explicitly), so test-set numbers stay honest.
- `gold-train/` — UD train splits, gitignored regardless of license. Used only for OOV/coverage analysis, never for accuracy claims.

Don't tune against test sets. If you find yourself iterating on a fix until
test-set numbers move, you've crossed into overfitting; switch to dev for the
iteration loop, then re-run test for the final number.

## Plan: expand case counts

The smaller curated sets (~50–80 cases) flatter the parser. Real-world text is
much messier — proper nouns, foreign words, hyphenated compounds, numerals,
informal punctuation — which is why
[`LEARNINGS.md` §2026-05-07](LEARNINGS.md) documents the gap between curated
fi-grammar (97% lemma) and ud-fi-tdt-test (53% lemma).

The aim is to always include at least one large held-out test set per language
in committed baselines, so headline numbers reflect real-world hardness.

| Lang | Curated (today) | Held-out UD test (today) | Plan |
|---|---:|---:|---|
| FI | 4 sets, 112 cases | 4 sets, 6,527 cases (`ftb` 1867, `ood` 2106, `pud` 1000, `tdt` 1554) | Always run all 8 in committed baselines. With omorfi at ~400 ms/case, full 4-UD pass takes ~70 min — acceptable for a baseline freeze |
| ET | 2 sets, 54 cases | 0 (CC BY-NC-SA gitignored under `localdata/parser-eval/et/gold/`) | `make import-ud-gold-et` materializes `ud-et-edt-test-v1.json` and `ud-et-ewt-test-v1.json` into `localdata/parser-eval/et/gold/` (per PR [#131](https://github.com/sagarinbabel/finnestdb/pull/131) consolidation). The comparison scripts auto-discover gold sets from both `testdata/parser-eval/<lang>/gold/` and `localdata/parser-eval/<lang>/gold/`, so a local-only UD-ET freeze runs through `make compare-parsers-et` with no extra flags. We don't commit the resulting JSONs to public git but each maintainer can freeze their own local extended baseline. Estimated ~30 min for the analyzer pass |

Open follow-ups (not blockers):

- **Stratify the eval report by token category.** A 53% headline on UD-TDT hides 90%+ open-class accuracy buried under maybe 20% on proper nouns and numerals. [`LEARNINGS.md` §2026-05-07](LEARNINGS.md) sketches the per-attribute eval that would surface this.
- **~~Per-feature attribute eval~~ — landed 2026-05-07k.** Both gold and parser now carry FEATS end-to-end; the comparison script emits a per-attribute table, e.g. `Case 99.2% / Number 100% / Mood 100% / Tense 100% / Person 100% / VerbForm 100% / Voice 100%` for omorfi on `fi-grammar-v1`. See [`baselines/2026-05-07-feats-rich.md`](baselines/2026-05-07-feats-rich.md) for the methodology and the runbook for re-importing the live DB so `custom` picks up FEATS too.
- **Silver-tier corpora.** [`scripts/scrape-gutenberg-fi`](../scripts/scrape-gutenberg-fi) builds a 500k-token Gutenberg silver-tier corpus for OOV/coverage measurement. Not yet wired into the eval comparison.
- **Versioning reconciliation.** [`SYSTEM_VERSIONING.md`](SYSTEM_VERSIONING.md) proposes `parser-baseline-YYYY-MM-DD-N`; [`PARSER_EVOLUTION.md`](PARSER_EVOLUTION.md) uses `YYYY-MM-DDx` (lowercase letter). Pick one and update both docs.

## Common pitfalls

**Filename collision on `fi-manual` v1/v2.** Both gold sets have
`dataset.name == "fi-manual"`, so the comparison script's slugified filename
(`${RUN_TS}-fi-manual.json`) is the same for both — v2 overwrites v1 silently.
Workaround: pass datasets one at a time with `-out` set explicitly. Real fix:
have the script prefix the input file basename. Tracked in
[`PARSER_EVOLUTION.md`](PARSER_EVOLUTION.md) §2026-05-07j open issues.

**`omorfi` adapter dispatch on macOS arm64.** `internal/parsecore/parsecore.go`
auto-discovers `.venv-estnltk/bin/python` for estnltk but does NOT
auto-discover `.venv-omorfi/bin/python`. With omorfi 0.9.12's `pyhfst` backend,
`.venv-omorfi/` is the natural install path on macOS. Workaround: export
`FINNESTDB_OMORFI_CMD` (see step 3 above). Real fix: symmetric venv discovery.
Tracked in PARSER_EVOLUTION.md §2026-05-07j.

**FST table size mismatch with FINAL baselines.** The 2026-05-06i FINAL
baselines were measured with locally-regenerated `fi_min.json` / `et_min.json`
against full Voikko / Giellalt analysers. A fresh clone has the 12-form smoke
fixtures, so coverage on real-world tokens is lower than FINAL. Workaround:
regenerate the tables locally via `make gen-lemmatizer-tables-fi
VFST_PATH=/path/to/local/mor.vfst`. Real fix: ship a deterministic
local-table regeneration recipe (exact upstream analyser version + wordlist),
or have the runtime `mmap` an out-of-tree `mor.vfst` / `analyser-gt-desc.hfstol`
at startup. Tracked in PARSER_EVOLUTION.md §2026-05-07j.

**5-second omorfi timeout per case.** The default `FINNESTDB_OMORFI_TIMEOUT`
is 5 s. UD treebanks contain occasional very long sentences (>40 tokens) that
can push omorfi past that. If you see `omorfi parser timed out` errors, export
`FINNESTDB_OMORFI_TIMEOUT=60s` before the run.
