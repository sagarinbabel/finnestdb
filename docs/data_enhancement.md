# Data enhancement log

Running ledger of every external data source the project pulls in for
parser-eval, dictionary loading, or silver-tier supervision. Use this
file as the single place to record:

- **what** the corpus is (one-line description),
- **where** it came from (canonical URL or upstream repo),
- **license** (and whether the derived artefact can be committed),
- **size** (cases / tokens / files / bytes — whichever applies),
- **when** it was added or last refreshed (ISO date),
- **how** it lives in the tree (committed path vs. gitignored localdata path),
- **how to regenerate** it (script or `make` target).

Add a new row whenever a corpus is imported, refreshed, or replaced.
Bump the **last refreshed** column on a re-pull so we can spot when a
locally-cached corpus has fallen behind upstream.

Sizes are measured directly with `grep -c '"id":' / '"surface":'` on
the JSON outputs, or `wc -c` / `du -sh` on raw files. They are point-in-time
and approximate — re-run the count if you need fresh numbers.

> **License compatibility rule** (per
> [`docs/ARTIFACT_POLICY.md`](ARTIFACT_POLICY.md)): public-domain and
> non-NC permissive licenses are committable as eval gold; NC-licensed
> derivatives stay under `localdata/` or gitignored gold paths. Bulk
> source corpora always stay in `localdata/`.

---

## 1. Parser-eval gold

Hand-checked or treebank-derived per-token gold used by `make compare-parsers`
and `make compare-parsers-et`. Committed gold lives under
[`testdata/parser-eval/{fi,et}/gold/`](../testdata/parser-eval/);
gitignored gold (NC-licensed ET UD dev/test, FI/ET train splits) lives
under [`localdata/parser-eval/{fi,et}/{gold,gold-train}/`](../localdata/)
per the single-folder bootstrap rule introduced 2026-05-07. Both
locations are auto-discovered by `scripts/parser-comparison{,-et}.sh`.

### 1a. Finnish — committed

| Dataset | Source | License | Cases | Tokens | Path | Added | Last refreshed |
|---|---|---|---:|---:|---|---|---|
| `fi-core-v1` | hand-authored, project | (project) | 6 | 20 | `testdata/parser-eval/fi/gold/fi-core-v1.json` | early (pre-Plan-C) | — |
| `fi-grammar-v1` | hand-authored, project | (project) | 80 | 156 | `testdata/parser-eval/fi/gold/fi-grammar-v1.json` | early (pre-Plan-C) | — |
| `fi-manual-v1` | hand-authored, project | (project) | 22 | 70 | `testdata/parser-eval/fi/gold/fi-manual-v1.json` | early (pre-Plan-C) | — |
| `fi-manual-v2` | hand-authored, project | (project) | 4 | 9 | `testdata/parser-eval/fi/gold/fi-manual-v2.json` | early (pre-Plan-C) | — |
| `ud-fi-tdt-dev-v1` | UD\_Finnish-TDT (Turku Dependency Treebank) | CC BY-SA 4.0 | 1,358 | 15,588 | `testdata/parser-eval/fi/gold/ud-fi-tdt-dev-v1.json` | 2026-05-06 (PR #113) | 2026-05-07 |
| `ud-fi-tdt-test-v1` | UD\_Finnish-TDT | CC BY-SA 4.0 | 1,554 | 17,951 | `testdata/parser-eval/fi/gold/ud-fi-tdt-test-v1.json` | 2026-05-06 (PR #113) | 2026-05-07 |
| `ud-fi-ftb-dev-v1` | UD\_Finnish-FTB (FinnTreeBank, Helsinki) | CC BY 4.0 | 1,875 | 13,536 | `testdata/parser-eval/fi/gold/ud-fi-ftb-dev-v1.json` | 2026-05-06 (PR #113) | 2026-05-07 |
| `ud-fi-ftb-test-v1` | UD\_Finnish-FTB | CC BY 4.0 | 1,867 | 13,973 | `testdata/parser-eval/fi/gold/ud-fi-ftb-test-v1.json` | 2026-05-06 (PR #113) | 2026-05-07 |
| `ud-fi-pud-test-v1` | UD\_Finnish-PUD (Parallel UD test set) | CC BY-SA 3.0 | 1,000 | 13,474 | `testdata/parser-eval/fi/gold/ud-fi-pud-test-v1.json` | 2026-05-06 (PR #113) | 2026-05-07 |
| `ud-fi-ood-test-v1` | UD\_Finnish-OOD (out-of-domain — poetry/dialogue) | CC BY-SA 4.0 | 2,106 | 16,151 | `testdata/parser-eval/fi/gold/ud-fi-ood-test-v1.json` | 2026-05-06 (PR #113) | 2026-05-07 |
| **FI gold subtotal (committed)** | | | **9,872** | **90,928** | | | |

Re-pull on 2026-05-07 produced byte-identical outputs to what's already
on `main`; no upstream drift detected since the original 2026-05-06 import.

### 1b. Finnish — local-only train splits (gitignored)

UD train splits are 12k–15k cases each. They live under
`localdata/parser-eval/fi/gold-train/` so headline
`parser-comparison.sh` runs don't accidentally include 30k-sentence
files. Used for OOV/coverage analysis with explicit `-dataset` flags.

| Dataset | Source | License | Cases | Tokens | Path | Added | Last refreshed |
|---|---|---|---:|---:|---|---|---|
| `ud-fi-tdt-train-v1` | UD\_Finnish-TDT | CC BY-SA 4.0 | 12,204 | 138,700 | `localdata/parser-eval/fi/gold-train/ud-fi-tdt-train-v1.json` | 2026-05-07 | 2026-05-07 |
| `ud-fi-ftb-train-v1` | UD\_Finnish-FTB | CC BY 4.0 | 14,972 | 109,445 | `localdata/parser-eval/fi/gold-train/ud-fi-ftb-train-v1.json` | 2026-05-07 | 2026-05-07 |
| **FI train subtotal (local-only)** | | | **27,176** | **248,145** | | | |

### 1c. Estonian — committed

| Dataset | Source | License | Cases | Tokens | Path | Added | Last refreshed |
|---|---|---|---:|---:|---|---|---|
| `et-grammar-v1` | hand-authored, project | (project) | 50 | 105 | `testdata/parser-eval/et/gold/et-grammar-v1.json` | early (pre-Plan-C) | — |
| `et-manual-v1` | hand-authored, project | (project) | 4 | 9 | `testdata/parser-eval/et/gold/et-manual-v1.json` | early (pre-Plan-C) | — |
| **ET gold subtotal (committed)** | | | **54** | **114** | | | |

### 1d. Estonian — local-only (gitignored due to **CC BY-NC-SA**)

Both Estonian UD treebanks ship under CC BY-NC-SA 4.0. The license's
non-commercial clause is incompatible with redistribution from a
permissively-licensed code repo, so derived gold JSON is gitignored
permanently and lives entirely under `localdata/parser-eval/et/`.
All six ET UD eval files below are local-only and must be regenerated
from a fresh checkout via `make import-ud-gold-et`.

| Dataset | Source | License | Cases | Tokens | Path | Added | Last refreshed |
|---|---|---|---:|---:|---|---|---|
| `ud-et-edt-dev-v1` | UD\_Estonian-EDT (Estonian Dependency Treebank) | CC BY-NC-SA 4.0 | 3,110 | 37,014 | `localdata/parser-eval/et/gold/ud-et-edt-dev-v1.json` | 2026-05-07 | 2026-05-07 |
| `ud-et-edt-test-v1` | UD\_Estonian-EDT | CC BY-NC-SA 4.0 | 3,190 | 40,496 | `localdata/parser-eval/et/gold/ud-et-edt-test-v1.json` | 2026-05-07 | 2026-05-07 |
| `ud-et-edt-train-v1` | UD\_Estonian-EDT | CC BY-NC-SA 4.0 | 24,419 | 284,915 | `localdata/parser-eval/et/gold-train/ud-et-edt-train-v1.json` | 2026-05-07 | 2026-05-07 |
| `ud-et-ewt-dev-v1` | UD\_Estonian-EWT (Estonian Web Treebank) | CC BY-NC-SA 4.0 | 823 | 8,074 | `localdata/parser-eval/et/gold/ud-et-ewt-dev-v1.json` | 2026-05-07 | 2026-05-07 |
| `ud-et-ewt-test-v1` | UD\_Estonian-EWT | CC BY-NC-SA 4.0 | 910 | 10,759 | `localdata/parser-eval/et/gold/ud-et-ewt-test-v1.json` | 2026-05-07 | 2026-05-07 |
| `ud-et-ewt-train-v1` | UD\_Estonian-EWT | CC BY-NC-SA 4.0 | 5,375 | 55,583 | `localdata/parser-eval/et/gold-train/ud-et-ewt-train-v1.json` | 2026-05-07 | 2026-05-07 |
| **ET UD subtotal (local-only)** | | | **37,827** | **436,841** | | | |

### 1e. Totals available *locally* after 2026-05-07 import

| Lang | Source | Cases | Tokens |
|---|---|---:|---:|
| FI | committed gold (manual + UD test/dev) | 9,872 | 90,928 |
| FI | local-only train (gold-train/) | 27,176 | 248,145 |
| FI | **total locally available** | **37,048** | **339,073** |
| ET | committed gold (manual only) | 54 | 114 |
| ET | local-only UD dev/test/train (NC) | 37,827 | 436,841 |
| ET | **total locally available** | **37,881** | **436,955** |
| **Both** | **grand total locally available** | **74,929** | **776,028** |

For comparison, before the 2026-05-07 import the working tree had
exactly the **committed** rows (9,872 FI + 54 ET = 9,926 cases /
91,042 tokens). The local-only files multiply effective gold ~7.5×.

### How to regenerate (1a–1d)

```sh
make import-ud-gold        # both languages, all splits
# or
make import-ud-gold-fi
make import-ud-gold-et
# or, lower-level
bash scripts/fetch-and-import-ud.sh both [--no-fetch]
```

Treebanks are cloned shallow into `localdata/ud-cache/`
(gitignored, ~50 MB each, ~97 MB total for FI+ET). Re-running with
the cache populated takes ~1 min wall-clock. Output is
byte-deterministic for a given upstream commit, so re-running is
safe and idempotent.

---

## 2. Dictionary corpora

Bulk source data used by `cmd/importdict`, `cmd/importekilex*`, and
`cmd/importkotus` to populate `finnestdb.db` (lemmas, forms, source
attribution). Per [`docs/ARTIFACT_POLICY.md`](ARTIFACT_POLICY.md) all of
these stay in [`localdata/`](../localdata/) (gitignored) — generator
code is committed; data is fetched fresh.

| Dataset | Lang | Source | License | Local size | Path | Notes |
|---|---|---|---|---|---|---|
| Ekilex public-words snapshot | ET | https://ekilex.ee | CC BY 4.0 | small JSONL (~10 MB) | `localdata/ekilex/eki-public-words-2026-et.jsonl` | loaded by `make import-ekilex-et` |
| Ekilex `/api/word/details` shards | ET | https://ekilex.ee API | CC BY 4.0 | ~1.0 GB / ~177k files | `localdata/ekilex/details/raw/00xxx/` | scraped by `make fetch-ekilex`; reduced via `make reduce-ekilex` |
| Kotus *Nykysuomen sanalista* | FI | https://kaino.kotus.fi/sanat/nykysuomi/ | CC BY 4.0 | ~104k headwords | `localdata/kotus/` | loaded by `make import-kotus-fi` (paradigm class for Voikko-style inflection) |
| kaikki.org Wiktionary FI dump | FI | https://kaikki.org | CC BY-SA 3.0 / GFDL | (varies) | `localdata/kaikki/fi/` | optional silver source for alt-forms; see `docs/LEXICAL_PLAN.md` |

### How to regenerate

The single entry point is `scripts/setup-local.sh`, which chains every
fetch/generate/import target. It is the canonical bootstrap path (see
[`README.md`](../README.md) Quickstart). For partial refreshes use the
individual `make` targets in section 2 of [`Makefile`](../Makefile).

---

## 3. Silver-tier corpora

Large unsupervised text corpora used (or planned) for silver-tagged
training data. Per [`docs/ARTIFACT_POLICY.md`](ARTIFACT_POLICY.md) raw
text and silver-tagged outputs stay in `localdata/` regardless of license.

| Dataset | Lang | Source | License | Tokens | Path | Added | Status |
|---|---|---|---|---:|---|---|---|
| Gutenberg-FI silver | FI | https://www.gutenberg.org/ebooks/search/?query=l.fi | public domain (US) | ~511,000 | `localdata/silver-fi/` (gitignored) | 2026-05-07 (PR #115) | committed scraper, raw text local-only |
| `fi-corpus` (HS / IS / YLE / IL home + articles) | FI | various FI news sites | site terms-of-use; non-redistributable | ~21 sources | `localdata/fi-corpus/` (gitignored) | (project) | hand-curated test material; not for redistribution |
| Gutenberg-FI poetry (runosto.net) | FI | https://www.runosto.net/ | public domain (FI) | ~195,000 (target) | `localdata/silver-fi-runosto/` (gitignored) | — | proposed (closed PR #122); revive on a fresh branch if adversarial-poetry coverage is wanted |
| Gutenberg-ET silver | ET | https://www.gutenberg.org/ebooks/search/?query=l.et | public domain (US) | — (target ~250k) | `localdata/silver-et/` (gitignored) | — | not yet fetched; same scraper, `-lang et` |

### How to regenerate

```sh
# Gutenberg-FI silver — 14 books, ~511k tokens, ~3 min wall-clock with
# 1.5 s polite delay between requests. Idempotent; re-runs append to
# the manifest and skip already-fetched IDs.
make scrape-gutenberg-fi
```

For a fresh teammate clone, the existing `localdata/silver-fi/` tree
can be zipped and copied out-of-band to skip the scrape entirely (see
[`docs/ARTIFACT_POLICY.md`](ARTIFACT_POLICY.md) §Reproducibility).

---

## 4. Treebank cache

`localdata/ud-cache/` is gitignored and holds shallow clones of the UD
treebank repos, ~50 MB each. After the 2026-05-07 import:

| Repo | License | Repo size on disk |
|---|---|---|
| UD\_Finnish-TDT | CC BY-SA 4.0 | (in cache) |
| UD\_Finnish-FTB | CC BY 4.0 | (in cache) |
| UD\_Finnish-PUD | CC BY-SA 3.0 | (in cache) |
| UD\_Finnish-OOD | CC BY-SA 4.0 | (in cache) |
| UD\_Estonian-EDT | CC BY-NC-SA 4.0 | (in cache) |
| UD\_Estonian-EWT | CC BY-NC-SA 4.0 | (in cache) |
| **`localdata/ud-cache/` total** | | **~97 MB** |

These are intermediate caches — derived gold JSON is what's actually
consumed by the eval. A periodic `git -C localdata/ud-cache/<repo>
pull` will pick up upstream fixes; re-import after pulling.

---

## 5. Frequency baselines

Per-form / per-lemma frequency lists used to compare against this
project's user-aggregated frequency. Bulk files live under
`localdata/frequency/` (gitignored); methodology, coverage curves, and
license attribution live in
[`docs/FREQUENCY_BASELINES.md`](FREQUENCY_BASELINES.md).

| Dataset | Source | License | Forms (top-N) | Path | Added | Last refreshed |
|---|---|---|---:|---|---|---|
| `opensubtitles-2018-fi-50k` | [hermitdave/FrequencyWords](https://github.com/hermitdave/FrequencyWords) | CC BY-SA 4.0 | 50,000 | `localdata/frequency/fi/opensubtitles-2018-fi-50k.txt` | 2026-05-07 | 2026-05-07 |
| `opensubtitles-2018-et-50k` | [hermitdave/FrequencyWords](https://github.com/hermitdave/FrequencyWords) | CC BY-SA 4.0 | 50,000 | `localdata/frequency/et/opensubtitles-2018-et-50k.txt` | 2026-05-07 | 2026-05-07 |
| `UD_Finnish-TDT-forms` | [UD\_Finnish-TDT](https://github.com/UniversalDependencies/UD_Finnish-TDT) train | CC BY-SA 4.0 | 49,179 unique | `localdata/frequency/fi/UD_Finnish-TDT-forms.tsv` | 2026-05-07 | 2026-05-07 |
| `UD_Finnish-TDT-forms-words` | UD\_Finnish-TDT, PUNCT/SYM filtered | CC BY-SA 4.0 | 48,964 unique | `localdata/frequency/fi/UD_Finnish-TDT-forms-words.tsv` | 2026-05-07 | 2026-05-07 |
| `UD_Finnish-TDT-lemmas` | UD\_Finnish-TDT (col 3) | CC BY-SA 4.0 | 23,025 unique | `localdata/frequency/fi/UD_Finnish-TDT-lemmas.tsv` | 2026-05-07 | 2026-05-07 |
| `UD_Estonian-EDT-forms` | [UD\_Estonian-EDT](https://github.com/UniversalDependencies/UD_Estonian-EDT) train | **CC BY-NC-SA 4.0** | 73,176 unique | `localdata/frequency/et/UD_Estonian-EDT-forms.tsv` | 2026-05-07 | 2026-05-07 |
| `UD_Estonian-EDT-forms-words` | UD\_Estonian-EDT, PUNCT/SYM filtered | **CC BY-NC-SA 4.0** | 73,040 unique | `localdata/frequency/et/UD_Estonian-EDT-forms-words.tsv` | 2026-05-07 | 2026-05-07 |
| `UD_Estonian-EDT-lemmas` | UD\_Estonian-EDT (col 3) | **CC BY-NC-SA 4.0** | 36,324 unique | `localdata/frequency/et/UD_Estonian-EDT-lemmas.tsv` | 2026-05-07 | 2026-05-07 |

The ET-EDT-derived files are CC BY-NC-SA — non-commercial only. Stays
in `localdata/` per the license-compatibility rule. Watch this if the
project ever monetizes.

### How to regenerate

```bash
make fetch-frequency-baselines
# or
go run ./cmd/fetchfrequency
```

Idempotent — re-running with no upstream change produces byte-identical
files.

---

## 6. Open follow-ups

Things that would expand the corpus further and were considered during
this pass but **not** done:

- **ET Gutenberg silver** — same scraper as FI, just `-lang et`. Useful
  to balance the FI silver tier and unblock per-language silver tagging.
- **runosto.net poetry-FI** — adversarial domain (verse, archaic forms);
  revives closed PR #122. Estimated ~195k tokens.
- **kaikki.org ET dump** — Wiktionary-derived alt-forms for ET. Same
  pipeline as kaikki-FI; would feed the dict tier rather than gold/silver.
- **UD\_Estonian-PUD** — listed in the user message as a potential
  source, but no `UD_Estonian-PUD` repo exists in the
  UniversalDependencies GitHub org as of 2026-05-07. Skipped. (PUD only
  exists for ~20 languages including FI; ET is not in that set.)

When any of these are added, append a row to the relevant section
above with the same column shape so future readers can see at a glance
how the corpus has grown.
