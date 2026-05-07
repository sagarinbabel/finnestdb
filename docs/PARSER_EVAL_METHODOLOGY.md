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

## Framing: two first-class parser outputs

For a language-learning tool, the parser has two jobs that matter equally to a
learner reading text:

1. **Dictionary-entry attachment** — given an inflected surface form, which
   dictionary headword should this learner be sent to? (`pankkiin → pankki /
   NOUN / "bank"`). This is what makes the click-to-define UX work at all.
2. **Grammatical analysis** — what *did* this form become in this sentence,
   and why? (`pankkiin = pankki in the illative singular`). This is what makes
   the parser educational — it explains, not just translates.

Treat these as peer metrics, not as a primary plus a footnote. A parser that
attaches `pankkiin` to `pankki` but cannot say it is illative singular is
useful but incomplete. A parser that labels illative singular but attaches to
the wrong lemma is also incomplete. The product goal is the joint result.

The two outputs also feed back into each other: morphological evidence (case,
number, tense, person, mood) helps disambiguate between otherwise plausible
lemma/POS candidates — Finnish `kuusi` is `NOUN` ("spruce") in inessive but
`NUM` ("six") in nominative, and the FEATS in context decide which dictionary
entry is right. So the FST/FEATS work is not ornamental; it is part of making
attachment more accurate too.

The metric table below reflects this: lemma+POS attachment and grammar are
both reported; "Full" stays as the all-correct ceiling.

## What we measure

Per dataset, per parser, on **non-PUNCT tokens** that the gold answer marks for
evaluation:

| Metric | Tier | Definition |
|---|---|---|
| **Lemma accuracy** | attachment (component) | Fraction of evaluated tokens where the parser's lemma matches gold |
| **POS accuracy** | attachment (component) | Fraction where Universal POS matches gold |
| **Lemma+POS accuracy** | attachment (joint, **first-class**) | Fraction where lemma AND POS *both* match — the actual "did this surface form land on the right dictionary entry?" metric, since entries are keyed by `(lemma, POS)`. Watch this over `Lemma accuracy` alone, especially on languages with `NOUN`/`NUM`/`PROPN` homographs |
| **Grammar accuracy** | grammar (single attribute, **first-class**) | Fraction where `grammar_label` matches gold (only on tokens where gold *has* a label — e.g. case-name for nouns) |
| **Per-FEATS-attribute accuracy** | grammar (full UD FEATS) | One row per UD FEATS attribute (Case, Number, Tense, Mood, Person, VerbForm, Voice, …); accuracy on the gold subset that supplied that attribute. Landed 2026-05-07k |
| **Full accuracy** | joint ceiling | Fraction where lemma AND POS AND grammar AND every gold FEATS attribute match. Useful as a single "everything correct" headline, but movement should be diagnosed against the two first-class metrics, not used to mask which side is slipping |
| **Resolved coverage** | reach | Fraction of input tokens the parser resolved to a dictionary entry (not "unknown") |
| **Avg/p50/p95 case time** | speed | Per-case wall time (sub-millisecond, nanosecond-precision since PR #103) |
| **Throughput** | speed | Aggregate tokens/sec and chars/sec across the full dataset |

Schema details: [`baselines/README.md`](baselines/README.md).

> When reading a report, eyes go to **Lemma+POS** (attachment) and **Grammar
> + per-FEATS** (analysis) first. If both move together, the parser got
> better. If only one moved, name which side, and avoid leaning on a moving
> Full% headline that doesn't tell you which dimension changed.

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
make setup-omorfi
```

Creates `.venv-omorfi/` and downloads the HFST models to `~/.cache/omorfi/`.
`scripts/parser-comparison.sh` auto-detects the venv + adapter and
constructs `FINNESTDB_OMORFI_CMD` for itself, so no env vars need to be
exported. omorfi 0.9.12 dropped the `hfst` C library in favour of
`pyhfst` (pure Python) so this no longer requires HFST C builds on
macOS arm64.

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
# RUN_TS is the YYYYMMDDTHHMMSSZ stamp printed by the comparison scripts
# (and visible as the prefix in reports/parser-eval/${RUN_TS}-*.json).
scripts/freeze-baseline.sh "$RUN_TS"
```

The script reads the parser-version letter from `parsecore.ParserVersion`,
derives the date + UTC HHMM from `RUN_TS`, and writes per-dataset JSONs
and cross-language summaries to `docs/baselines/` under the canonical
filename:

```
docs/baselines/YYYY-MM-DD<rev>-T<HHMM>Z-<dataset>.<ext>
```

— for example `docs/baselines/2026-05-07k-T1118Z-fi-core.json`. **Append-only**:
the script refuses to overwrite existing targets, so older baselines stay
referenceable forever (cross-references like "see `2026-05-06-final-fi-core`"
in old PR descriptions remain valid). Filename spec, examples, and rationale:
[`baselines/README.md` §Filename convention](baselines/README.md).

Then add a row to [`PARSER_EVOLUTION.md`](PARSER_EVOLUTION.md)'s trend table
and a `### YYYY-MM-DD<rev>-T<HHMM>Z` entry per the convention there, plus
a row in [`SYSTEM_VERSIONING.md` § Parser evaluation baseline history](SYSTEM_VERSIONING.md).

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

**Filename collision on `fi-manual` v1/v2 — fixed 2026-05-07.** Both gold sets
have `dataset.name == "fi-manual"`. Pre-fix, the comparison script slugified
the JSON `name` field, so two datasets collapsed to one report path and v2
overwrote v1 silently. Now `scripts/parser-comparison{,-et}.sh` slug from the
input *file basename* (which is unique by definition), so `fi-manual-v1.json`
and `fi-manual-v2.json` produce distinct report files.

**`omorfi` adapter dispatch on macOS arm64 — fixed 2026-05-07.**
`scripts/parser-comparison.sh` now mirrors the EstNLTK side: when
`.venv-omorfi/bin/python` and `scripts/omorfi_adapter_example.py` both
exist, `FINNESTDB_OMORFI_CMD` is auto-constructed. `make setup-omorfi`
also creates `.venv-omorfi/` symmetrically with `make setup-estnltk`'s
`.venv-estnltk/`, instead of pip-installing into the active interpreter.

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
