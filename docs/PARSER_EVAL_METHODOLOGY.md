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

### Ambiguity and meaning-check calibration

Product meaning checks need a narrower measurement than headline parser
accuracy: can the parser choose the intended dictionary entry for an ambiguous
surface in sentence context? This section is the spec for the **ambiguity eval
slice** — the measurement foundation the "Ambiguous meaning flow" launch gate
(see [`TODO.md`](../TODO.md)) is blocked on. It is implementation-ready: an agent
can start from here cold.

The `custom` parser today emits exactly **one** `(lemma, POS)` pick per surface
(`parsecore.TokenResult`) and has **no numeric confidence score**. The product,
however, wants to branch between a single **Meaning Check** and **Multiple
possible meanings** (see [`CONTEXT.md`](../CONTEXT.md)). This slice measures
whether that branch can be made safely, per ambiguity class, from deterministic
eval — not from an invented confidence number.

#### 1. What is measured

On the *target token* of each case (the ambiguous surface, marked `target: true`
in gold), three metrics:

1. **Candidate inclusion** — is the contextually-correct `(lemma, POS)` present
   in the candidate set the product can offer for this surface? The candidate
   set is `store.BatchLookupAllForms(form, lang, "custom")` — the exact
   dict-candidate list that powers Multi-Lemma Surface expansion
   (`internal/api/handlers.go::expandTokenLemmas`). If the correct sense is not
   in this set, the product literally cannot show it, so **Multiple possible
   meanings** is impossible and even a correct single pick is unverifiable.
2. **Selection accuracy** — does the parser's single top pick
   (`parsecore.Analyze(..., "custom")`) match the contextually-correct
   `(lemma, POS)`? This is the gate metric: the single **Meaning Check** UI is
   only safe when selection is reliable on the class.
3. **Calibration (confidence proxy)** — see below. Because there is no numeric
   confidence, calibration is measured against a *structural proxy* and reported
   as "when the proxy says high-confidence, how often is selection right?"

FEATS is recorded where applicable (as in the main gold sets) but is not part of
the gate — the meaning branch is a lemma/POS-sense decision, not a case decision.
External analyzers (`omorfi` FI, `estnltk` ET) stay available as the reference
upper bound, exactly as in the headline eval.

#### Confidence proxy (there is no numeric confidence today — stated honestly)

The `custom` ranking (`internal/store/dict.go::pickBestResolutionCandidate` +
`mergeAndRankDictFSTCandidates`) is a `sort.SliceStable` over **discrete,
non-numeric** signals: case-match score, POS-sanity score, source priority,
dict/FST agreement, FST emission order, and a handful of morphology biases. It
produces an *ordering*, not a probability. So we do **not** claim a confidence
number. Instead the slice starts with a **structural proxy** built only from
signals that already exist:

- **`single`** — the candidate set (`BatchLookupAllForms`) has exactly one
  `(lemma, POS)` for the surface → treated as high-confidence.
- **`multi`** — two or more distinct `(lemma, POS)` candidates → low-confidence
  by default; the parser is choosing among genuine homographs.
- **`dict_fst_agree`** — the winning pick was corroborated by both a dictionary
  row and an FST analysis (`Source` contains `dict` and an `fst_*` tag) →
  raises confidence within the `multi` bucket.

The slice reports selection accuracy *stratified by proxy bucket*. That table is
what tells us whether `single` is actually a trustworthy high-confidence signal
(it may not be — see the baseline, where FI `single` includes cross-POS
homographs whose second sense is missing from the dict). If and when a real
numeric confidence lands, this proxy is replaced and the same slice re-measures
it; the gate rule (below) is written against *measured* accuracy per class, so it
survives the proxy swap.

#### 2. Gold slice format

Minimal extension of the committed gold shape
(`internal/eval/eval.go`: `Dataset` → `DatasetCase` → `ExpectedTokenRef`). One
sentence = one case with exactly one scored target token; a case-level
`ambiguity_class` groups sentences of the same homograph, and the target token
carries an `expected_candidates` sense set. Full field docs and a worked example
live in [`testdata/parser-eval/fi-ambiguity/README.md`](../testdata/parser-eval/fi-ambiguity/README.md).
Shape:

```jsonc
{
  "name": "fi-ambiguity", "version": "v1", "language": "FI", "slice": "ambiguity",
  "cases": [{
    "id": "fi-amb-kuusi-1",
    "text": "Pihalla kasvaa suuri kuusi.",
    "ambiguity_class": "kuusi",
    "tokens": [{
      "surface": "kuusi", "occurrence": 1, "target": true,
      "lemma": "kuusi", "pos": "NOUN", "feats": "Case=Nom|Number=Sing",
      "expected_candidates": [
        {"lemma": "kuusi", "pos": "NOUN", "gloss_hint": "spruce"},
        {"lemma": "kuusi", "pos": "NUM",  "gloss_hint": "six"}
      ]
    }]
  }]
}
```

The committed slice data is
[`testdata/parser-eval/fi-ambiguity/fi-ambiguity-v1.json`](../testdata/parser-eval/fi-ambiguity/fi-ambiguity-v1.json)
(48 FI cases, 21 classes) and
[`et-ambiguity-v1.json`](../testdata/parser-eval/fi-ambiguity/et-ambiguity-v1.json)
(13 ET cases, 6 classes). They live under `testdata/parser-eval/`, deliberately
*not* under `fi/gold` or `et/gold`, so they never inflate the headline
lemma/POS sweep with a curated, deliberately-hard homograph mix.

#### 3. FI case inventory + current-parser baseline (evidence)

The 48 FI cases cover: cross-POS homographs where the two senses differ in UD POS
(`kuusi` NOUN/NUM, `tuli` NOUN/VERB, `voi` NOUN/VERB); single-POS homographs
(`kurkku` cucumber/throat, `vuori` mountain/lining, `selkä` back/open-water);
noun-vs-verb *form* collisions (`sanoi`/`sanoin`, `palaa`, `tuulet`, `sain`); and
case-form surfaces the FST handles cleanly as controls (`maassa`, `kylään`,
`rannalle`, …). Two natural CC0 sentences per sense minimum.

Baseline below is **actual `custom`-mode output** on the production DB + full FST
tables (`localdata/lemmatizer-fi-et/tables/`, 243 MB FI / 75 MB ET), parser
version `2026.05.15a`, measured 2026-07-04. "cand set" is the raw
`BatchLookupAllForms` result; "pick" is the single `Analyze` result. This table
IS the evidence section — the formal baseline freeze is left to the maintainer
(see §6).

| class | sel. acc | candidate set the product can offer | failure mode |
|---|---|---|---|
| kuusi | 2/4 | `kuusi/NUM` only | **NOUN "spruce" absent from dict forms** — FST knows it, candidate API doesn't merge FST |
| tuli | 2/4 | `tulla/VERB` only | **NOUN "fire" absent from dict forms** — same gap |
| voi | 2/4 | `voi/NOUN` only | **VERB "voida" absent from dict forms** — same gap |
| alusta | 0/2 | `alus/NOUN` only | wrong single reading; neither `alku` (ela) nor `alusta` (chassis) reachable |
| palaa | 0/2 | `pala/NOUN` only | picks noun `pala` (par) over both verb readings |
| sanoi | 1/2 | `sanoa/VERB` (lex-overlay) | overlay forces VERB; `sanoin` "with words" (`sana` NOUN instr.) unreachable |
| tie | 1/2 | `tie` / `tien` | `tien` self-keys as its own lemma (genitive not stemmed) |
| kurkku, vuori, selka, sain, laulun, tuulet, ilta, kyla, ranta, puku, kayda, maa, alkoi, sade | 2/2 each | correct single reading | control cases pass |

**FI headline: selection accuracy 36/48 = 75.0%; candidate inclusion 35/48 =
72.9%.**

The dominant FI failure is not bad ranking — it is that **kaikki.org's `forms`
table stores only one reading per surface for the classic cross-POS homographs**,
so the second sense never enters `BatchLookupAllForms` at all. The FST *does*
return both readings (verified: `kuusi` → NOUN+NUM, `tuli` → NOUN+VERB, `voi` →
NOUN+VERB+INTJ), but the candidate API is dict-only. So for FI, **candidate
inclusion is the blocker before selection** — the product can't even honestly
show "Multiple possible meanings" for `kuusi`/`tuli`/`voi` today.

#### 4. Threshold → UI rule

The gate operates **per ambiguity class**, using the slice's per-class numbers:

> An ambiguity class may use the **single Meaning Check** UI only when, on its
> slice cases: **selection accuracy ≥ 90%** AND **candidate inclusion = 100%**
> AND **N ≥ 4** target cases (≥ 2 per sense). Otherwise the surface uses
> **Multiple possible meanings** (list candidates, per-candidate known/study,
> plus the "None of these looks right" flag-only feedback path).

Rationale for the numbers:

- **selection ≥ 90%** — the single check asserts one meaning as intended; below
  ~90% the learner is corrected against a wrong sense often enough to erode the
  First-Experience trust bar. Deliberately strict for alpha; can loosen with a
  real confidence signal that lets low-confidence *within* a passing class fall
  through to the multi-UI.
- **candidate inclusion = 100%** — if the correct sense can be missing from the
  candidate set (the FI kaikki gap), the multi-UI would omit the right answer,
  which is worse than asking. A class that can't even enumerate its senses is not
  eligible for *either* confident UI until the candidate set is fixed.
- **N ≥ 4, ≥ 2/sense** — two sentences per sense is the floor for the accuracy
  number to mean anything; a class with one sentence per sense can hit 100% by
  luck.

Applied to today's baseline: the 14 control classes at 2/2 do **not** yet
qualify (N < 4 per class) — they need their case count raised before they can be
promoted, which is exactly the point of the "expand case counts" discipline. The
cross-POS classes (`kuusi`/`tuli`/`voi`) fail candidate inclusion outright and
must stay on **Multiple possible meanings** regardless of selection until the
candidate API merges FST readings. This is the honest launch posture: **no class
is single-Meaning-Check-eligible on the v1 slice; everything defaults to Multiple
possible meanings**, and classes graduate as case counts rise and the FI
candidate gap is closed.

#### 5. ET parity plan (follow-up slice)

ET fails *differently*, which is why parity is mandatory before any product copy
implies equal support. Real ET homographs (verified against the DB + Ekilex, not
invented): `tee` road/tea vs `tegema` do, `viis` five(NUM)/melody(NOUN)/`viima`,
`sai` white-bread(NOUN)/`saama` got, `pea` head(NOUN)/`pidama` must, `kuu`
moon/month, `palk` salary/log. The committed `et-ambiguity-v1.json` (13 cases, 6
classes) baseline:

**ET headline: selection accuracy 7/13 = 53.8%; candidate inclusion 13/13 =
100.0%.**

The inversion is the finding: **Ekilex populates the full candidate set** (ET
candidate inclusion is perfect — `tee` → both NOUN and `tegema/VERB`; `pea` →
five candidates), but the parser's single pick prefers the VERB reading on
cross-POS collisions and mis-selects on `tee`, `viis`, `sai`, `pea`. So ET's
blocker is **selection ranking**, while FI's blocker is **candidate inclusion**.
A single language-agnostic threshold would hide this; the per-class rule surfaces
it. The ET slice is scoped as the follow-up: expand to ~20-30 cases and wire the
same runner, before ET ambiguity UI ships.

#### 6. Runner integration + baseline discipline

Add a focused `cmd/ambiguityeval` (rather than overloading `cmd/parsertest`): it
loads a `slice: "ambiguity"` gold file, for each case runs `custom`-mode
`Analyze` for the pick and `BatchLookupAllForms` for the candidate set, computes
the three metrics + the proxy-stratified table, and writes a report keyed by
`ambiguity_class`. Rationale for a separate command: the ambiguity metrics
(candidate inclusion, per-class stratification) are not token-accuracy metrics
and would distort the `parsertest` summary schema; keeping them apart also keeps
the ambiguity slice out of the headline sweep by construction.

Wire it into a `make compare-ambiguity` target (parallel to
`make compare-parsers`), and freeze its output with the existing
`scripts/freeze-baseline.sh` naming convention
(`YYYY-MM-DD<rev>-T<HHMM>Z-<dataset>`, see
[`baselines/README.md`](baselines/README.md)) so a `2026-07-04a-fi-ambiguity`
report is append-only and referenceable. The committed baseline file is the
per-class selection/candidate table plus the proxy-stratified accuracy, so the
threshold rule can be re-evaluated against any frozen point. Re-freeze on any
parser-affecting PR, same as the headline baselines.

Until `cmd/ambiguityeval` lands, the evidence table in §3–§5 above is the
reference baseline (produced by a throwaway harness over these exact gold files,
parser `2026.05.15a`, 2026-07-04).

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

### Stratified breakdown (`-stratified`)

Even Lemma+POS hides structural variation. On UD-FTB-test the `custom` parser
scores 71.4% lemma overall — but that splits into ~80% open-class, ~54%
closed-class, ~62% PROPN, ~93% NUM (see [`LEARNINGS.md`](LEARNINGS.md)
§2026-05-07 — UD-TDT for the original observation). A regression in any one
slice (PROPN dropping from 60% → 30%) can be invisible in the rolled-up
number when the dominant slice (open-class NOUN/VERB) hasn't moved.

Pass `-stratified` to `cmd/parsertest` to attach a three-axis breakdown to
each parser summary in the JSON report and write a `<run>.stratified.md`
sidecar:

1. **UPOS bucket** — `open` (NOUN/VERB/ADJ/ADV), `closed` (DET/PRON/CCONJ/...),
   `propn`, `num`, `punct`, `other`.
2. **OOV** — `in-dict` (parser resolved the surface) vs `oov` (didn't).
3. **Compoundness** — `compound` (surface contains a hyphen, or parser used
   the compound-split rule) vs `simple`.

`cmd/parser-compare -stratified` prints the same tables alongside the
headline. The compare-side computation is on-the-fly from the report's
case-level comparisons, so it works on historical baseline JSONs that predate
the flag.

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
make setup-nlp
```

Creates `.venv/`, installs both omorfi and estnltk, and downloads the HFST
models to `~/.cache/omorfi/`. No environment variables need to be exported:
`scripts/parser-comparison.sh` auto-detects the venv + adapter and constructs
`FINNESTDB_OMORFI_CMD` for itself, and `internal/evalparsers` independently
discovers `.venv/bin/python` at runtime, so direct callers like
`cmd/parsertest` work without any export either. The repo-local venv is the
default because system Python on macOS Homebrew enforces PEP 668
(`externally-managed-environment`), which would otherwise block
`pip install omorfi`. omorfi 0.9.12 dropped the `hfst` C library in favour
of `pyhfst` (pure Python) so this no longer requires HFST C builds on
macOS arm64.

If you must override the venv (different python version, alternative omorfi
fork), set:

```
export FINNESTDB_OMORFI_CMD="$(pwd)/.venv/bin/python $(pwd)/scripts/omorfi_adapter_example.py"
```

**Estonian (estnltk via EstNLTK):**

```
make setup-nlp
```

Uses the same `.venv/` and downloads `nltk_data` to `.cache/nltk_data/`.
First call after install can take 30s+ as NLTK builds its font cache.

### 4. Run the comparison scripts

```
bash scripts/parser-comparison.sh -o reports/parser-eval/$(date +%Y-%m-%d)-fi.md
bash scripts/parser-comparison-et.sh -o reports/parser-eval/$(date +%Y-%m-%d)-et.md
```

Default discovery: every `.json` or `.json.gz` dataset under
`testdata/parser-eval/{fi,et}/gold/` and `localdata/parser-eval/{fi,et}/gold/`
not matching `-dev-v` (held-out test discipline — dev sets are for per-commit
"watch" eval, not committed baselines). Each dataset gets its own JSON report
under `reports/parser-eval/${RUN_TS}-${name}.json`. The markdown summary
aggregates them.

### 5. Freeze a baseline

```
# RUN_TS is the YYYYMMDDTHHMMSSZ stamp printed by the comparison scripts
# (and visible as the prefix in reports/parser-eval/${RUN_TS}-*.json).
scripts/freeze-baseline.sh "$RUN_TS"
```

The script reads the parser-version letter from `parsecore.ParserVersion`,
derives the date + UTC HHMM from `RUN_TS`, and writes compressed per-dataset
JSONs plus cross-language summaries to `docs/baselines/` under the canonical
filename:

```
docs/baselines/YYYY-MM-DD<rev>-T<HHMM>Z-<dataset>.<ext>
```

— for example `docs/baselines/2026-05-07k-T1118Z-fi-core.json.gz`. **Append-only**:
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

- `gold/<name>-test-v*.json[.gz]` — committed baselines run on these. They are the headline.
- `gold/<name>-dev-v*.json[.gz]` — committed baselines **skip** these (filtered by the comparison scripts via `grep -v -- '-dev-v'`, and by `make eval` via the same case match). Use them for per-commit "watch" eval — either `make eval-watch` (test + dev sweep) or `cmd/parsertest -dataset <dev-set>` explicitly — so test-set numbers stay honest.
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
- **Silver-tier corpora.** [`cmd/scrapegutenberg`](../cmd/scrapegutenberg/main.go) builds a 500k-token Gutenberg silver-tier corpus for OOV/coverage measurement via `make scrape-gutenberg-fi`. Not yet wired into the eval comparison.
- **Versioning reconciliation.** [`SYSTEM_VERSIONING.md`](SYSTEM_VERSIONING.md) proposes `parser-baseline-YYYY-MM-DD-N`; [`PARSER_EVOLUTION.md`](PARSER_EVOLUTION.md) uses `YYYY-MM-DDx` (lowercase letter). Pick one and update both docs.

## Common pitfalls

**Filename collision on `fi-manual` v1/v2 — fixed 2026-05-07.** Both gold sets
have `dataset.name == "fi-manual"`. Pre-fix, the comparison script slugified
the JSON `name` field, so two datasets collapsed to one report path and v2
overwrote v1 silently. The same collision reached `make eval` because
`cmd/parsertest`'s default report path also slugified `dataset.name`. Now
`scripts/parser-comparison{,-et}.sh` slug from the input *file basename*
(which is unique by definition), and `cmd/parsertest` derives its default
slug from the input filename via `EvaluateOptions.RunIDSlug`, so
`fi-manual-v1.json` and `fi-manual-v2.json` produce distinct report files
through every code path.

**`omorfi` adapter dispatch on macOS arm64 — fixed 2026-05-07.**
`scripts/parser-comparison.sh` now mirrors the EstNLTK side: when
`.venv/bin/python` and `scripts/omorfi_adapter_example.py` both exist,
`FINNESTDB_OMORFI_CMD` is auto-constructed. `internal/evalparsers`
independently auto-discovers `.venv/bin/python` at runtime, so direct callers
like `cmd/parsertest` also work without an `FINNESTDB_OMORFI_CMD` export.
`make setup-nlp` creates the shared `.venv/` instead of pip-installing into
the active interpreter.

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
