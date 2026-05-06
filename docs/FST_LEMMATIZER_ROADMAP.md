# FST lemmatizer migration — roadmap

Status: **all four PRs merged 2026-05-06.** See
[docs/FST_LEMMATIZER.md](FST_LEMMATIZER.md) for the as-built
architecture, attribution, performance, and final eval delta.
This roadmap remains as the historical planning record.

Date: 2026-05-06
Driver: Phase 3.5 spike, recorded in
[experiments/2026-05-06-phase3.5-voikko-generator-spike.md](../experiments/2026-05-06-phase3.5-voikko-generator-spike.md)

## What changed in plan

The original [Finnish lexical plan](FINNISH_LEXICAL_PLAN.md) had a Phase 4
that would offline-generate ~6M Finnish forms via a Voikko paradigm
generator and import them into SQLite as static rows. The Phase 3.5
spike shut that path down: libvoikko ships an *analyzer* runtime, not a
generator, and the public C API has no generation function.

Investigating the alternative paths surfaced a better architecture:
**port the runtime FSTs themselves to Go and look up surface forms
directly**, instead of offline-enumerating every paradigm into SQLite.
This is faster (microsecond lookups vs SQLite hash hits), lighter (~35
MB of FST data vs millions of rows), and naturally combines two
lexicons (Voikko's curated VFST + Giellalt's HFST optimised-lookup).

## Final architecture

A new package `pkg/lemmatizer-fi-et/` provides:

- `vfst/` — Go port of libvoikko's
  [`UnweightedTransducer`](https://github.com/voikko/corevoikko/blob/master/libvoikko/src/fst/UnweightedTransducer.cpp)
  runtime. Reads `mor.vfst` (Voikko's compiled Finnish morphology
  analyzer) directly. ~5 MB vendored data. Memory-mapped, sub-µs
  lookups.
- `hfstol/` — Go port of HFST's
  [optimised-lookup runtime](https://github.com/hfst/hfst/tree/master/libhfst/src/implementations/optimized-lookup).
  Reads `analyser-gt-desc.hfstol` files compiled from
  [`giellalt/lang-fin`](https://github.com/giellalt/lang-fin) and
  [`giellalt/lang-est`](https://github.com/giellalt/lang-est).
  ~26 MB per language.
- A unified `Lemmatize(word string) []Analysis` that queries both
  backends per Finnish lookup, merges results, and dedupes by
  `(lemma, tag-set)`. Voikko-priority on overlap (more curated);
  Giellalt fills coverage gaps. For Estonian, Giellalt-only (Voikko
  doesn't cover ET).

Runtime closure: pure Go, no cgo, no shared library, no SQLite query on
the hot path. Existing dictionary tables (`forms`, `lemmas`,
`paradigm_class`) are still consulted for gloss/translation lookup but
not for morphological analysis.

## Why this beats the original plan

| Dimension | Original Phase 4 (SQLite seed) | New FST runtime |
|---|---|---|
| Lookup time | ~5 µs (SQLite hash) | ~1 µs (memory-mapped FST) |
| Disk footprint | ~250 MB seed table + indexes | ~35 MB per language |
| Build dependency | Voikko paradigm generator (does not exist as released tool) | Giellalt source build (one-time) |
| Coverage | Voikko only | Voikko + Giellalt union |
| Runtime deps | None new | None |
| Refresh cost | Re-run Voikko build pipeline | Bump vendored `.vfst` / `.hfstol` files |

## PR sequence

| PR | Scope | Acceptance |
|---|---|---|
| **0** (this PR) | Pre-change FI + ET baselines, Phase 3.5 spike report, this roadmap | Baselines committed; reviewers understand the plan |
| **1** | `pkg/lemmatizer-fi-et/vfst/`, vendored `mor.vfst`, golden tests, Voikko-tag → UD-features mapping, `Lemmatize` interface, parser integration for FI | FI eval ≥ baseline on all four datasets; new package has unit tests; integration through `internal/parsecore` |
| **2** | `pkg/lemmatizer-fi-et/hfstol/`, vendored Giellalt FI `.hfstol`, merged `Lemmatize` results with dedup | FI eval coverage ↑ on at least one dataset; no regressions |
| **3** | Build `giellalt/lang-est`, vendor ET `.hfstol`, ET integration | ET eval coverage ↑ on at least one dataset |
| **4** | Final delta vs baseline, `docs/FST_LEMMATIZER.md` (architecture + attribution + perf numbers), update `FINNISH_LEXICAL_PLAN.md` to mark Phase 4 superseded | Numbers documented, plan updated, attribution complete |

Reviewer for the series: **@chickendude**.

## Pre-change baselines (snapshot before any FST work)

Captured by `make compare-parsers` and `make compare-parsers-et`,
parser commit `265dd21` (current `main`):

- FI summary: [docs/baselines/2026-05-06-pre-fst-comparison-fi.md](baselines/2026-05-06-pre-fst-comparison-fi.md)
- ET summary: [docs/baselines/2026-05-06-pre-fst-comparison-et.md](baselines/2026-05-06-pre-fst-comparison-et.md)
- Per-dataset JSONs at `docs/baselines/2026-05-06-pre-fst-*.json`

Headline numbers (custom parser):

| Lang | Dataset | Lemma | POS | Coverage |
|---|---|---:|---:|---:|
| FI | fi-core | 85.0% | 90.0% | 95.7% |
| FI | fi-grammar | 96.8% | 98.1% | 99.7% |
| FI | fi-manual-v1 | 81.4% | 85.7% | 91.2% |
| FI | fi-manual-v2 | 88.9% | 100.0% | 100.0% |
| ET | et-grammar | 88.6% | 96.2% | 98.9% |
| ET | et-manual | 77.8% | 77.8% | 91.7% |

The biggest headroom is on the manual datasets (fi-manual-v1 lemma
81.4%; et-manual lemma 77.8%) — those are hand-curated harder cases
where the current SQLite-rule path under-resolves. The FST path should
particularly help there.

## Attribution

This work depends on three independently-developed open-source
projects. Their licences will be reproduced verbatim alongside the
vendored data files in PR 1+:

- **libvoikko** (MPL 1.1 / GPLv2 / LGPLv2.1 tri-license) — runtime
  algorithm reference and `mor.vfst` data file.
- **HFST** (GPLv3) — runtime algorithm reference for the
  optimised-lookup format.
- **GiellaLT lang-fin / lang-est** (GPLv3) — `.hfstol` data files
  compiled from their lexc sources.

Vendored data inherits the upstream licences. The Go runtime code is
ours and lives under the project's existing licence.
