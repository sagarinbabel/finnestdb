# FST lemmatizer — architecture and rationale

This document describes finnestdb's Finnish + Estonian morphological
analysis layer as it stands after the four-PR migration completed
2026-05-06. It supersedes the original
[LEXICAL_PLAN.md](LEXICAL_PLAN.md) Phase 4 (Voikko
SQLite seed) and Phase 5 (resolution priority flip).

For the migration journey itself — what was tried, what failed, what
worked — see the spike report at
[experiments/2026-05-06-phase3.5-voikko-generator-spike.md](../experiments/2026-05-06-phase3.5-voikko-generator-spike.md).

## TL;DR

`pkg/lemmatizer-fi-et/` is a **pure-Go** morphological analyzer for
Finnish and Estonian. It loads two open-source finite-state
transducer formats from embedded binary data files and merges their
results.

- For Finnish, it queries both **libvoikko's `mor.vfst`** (curated
  Finnish lexicon, MPL/GPL/LGPL tri-licensed) and **Giellalt
  `lang-fin`'s `analyser-gt-desc.hfstol`** (broader coverage, GPLv3+),
  preferring Voikko on overlap.
- For Estonian, it queries Giellalt **`lang-est-x-utee`**'s
  `analyser-gt-desc.hfstol` (the only published HFST analyzer for
  Estonian).

Net effect on the parser, vs the SQLite-only baseline frozen on the
same day:

| Lang | Dataset | lemma | grammar | full | coverage |
|---|---|---:|---:|---:|---:|
| FI | fi-grammar | +0.6 | **+1.4** | +0.6 | +0.3 |
| FI | fi-manual-v1 | **+1.5** | **+13.3** | **+21.5** | **+5.7** |
| FI | fi-core | +0.0 | +0.0 | +0.0 | **+4.3** |
| ET | et-grammar | +0.0 | **+2.0** | +0.0 | **+1.1** |
| ET | et-manual | **+11.1** | **+16.7** | **+11.1** | **+8.3** |

Zero regressions. The runtime adds ~40 MB to the compiled binary
(embedded data files), no cgo, no shared library, no external
filesystem coupling. Lookup latency is ~35–43 µs per word.

## Why this exists

The original [LEXICAL_PLAN.md](LEXICAL_PLAN.md) called
for a 6M-row SQLite "seed" of every Finnish form generated offline by
Voikko. The Phase 3.5 spike (2026-05-06) shut that path down: the
released `voikkospell` binary ships only the *analyzer*, not a
generator, and libvoikko's public C API has no generation function
either. The internal C++ classes (`UnweightedTransducer`,
`WeightedTransducer`) are local-symbol-only in `libvoikko.dylib`, so
they can't be reached from cgo.

While investigating alternatives (the Giellalt source-build path and
linking against HFST), the spike surfaced a **better** approach
overall: instead of pre-computing every paradigm into a giant SQLite
seed, **port the FST runtimes themselves to Go** and run the analyzers
directly on every parser query. Faster lookups, smaller artifacts,
and we get the union of two lexicons "for free" rather than committing
to one.

That's what this PR series shipped.

## Architecture

```
                  ┌─────────────────────┐
parser query  ──→ │ internal/store      │
                  │ BatchLookupForms()  │
                  │  Step 1-4: SQLite   │  (existing rule-based path)
                  │  Step 5: FST   ─────┼─→ pkg/lemmatizer-fi-et
                  └─────────────────────┘            │
                                                     ▼
              ┌──────────────────────────────────────────────────┐
              │ Lemmatize(lang, word) []Analysis                 │
              │                                                  │
              │   FI:  vfst.Analyze() ─→ voikkomap.Parse()       │
              │        ──────merge by (lemma, UPOS) ──────       │
              │        hfstol.Analyze(FI) ─→ giellaltmap.Parse() │
              │                                                  │
              │   ET:  hfstol.Analyze(ET) ─→ giellaltmap.Parse() │
              │                                                  │
              │  embedded: mor.vfst (FI), analyser-gt-desc.hfstol│
              │            (FI + ET) — total ~40 MB              │
              └──────────────────────────────────────────────────┘
```

### Components

- **`vfst`** — Go port of [libvoikko's `UnweightedTransducer`](https://github.com/voikko/corevoikko/blob/master/libvoikko/src/fst/UnweightedTransducer.cpp)
  runtime. Reads the Voikko VFST binary format directly: 16-byte
  header + symbol table + 8-byte transition records + flag-diacritic
  state machine (P/C/U/R/D operators). Memory-mapped, sub-µs lookups.
  ~500 LOC. **Verified bit-identical to `voikkospell -m`** on a
  representative sample.

- **`hfstol`** — Go port of [HFST's `Transducer::get_analyses`](https://github.com/hfst/hfst/blob/master/libhfst/src/implementations/optimized-lookup/transducer.cc)
  runtime. Reads the HFST3 optimised-lookup binary format: HFST3
  magic + property bag, 56-byte TransducerHeader, alphabet, sparse
  index table (6-byte entries), transition target table (8 bytes
  unweighted / 12 bytes weighted), with cycle protection during
  epsilon-only walks. ~700 LOC. **Verified bit-identical to
  `hfst-lookup`** on the same sample.

- **`voikkomap`** — parses libvoikko's FSTOUTPUT tag stream
  (`[Ln][Ica][Xp]talo[X]talo[Sn][Ny]`) into the parser's `Analysis`
  struct (Lemma, UPOS, GrammarLabel, Number, Tense, Mood, Person).
  Compound-aware: concatenates all `[Xp]...[X]` segments. Tag mapping
  derived from `FinnishVfstAnalyzer.cpp::parseBasicAttributes` upstream.

- **`giellaltmap`** — parses Giellalt's tag strings (`talo+N+Sg+Nom`,
  `olla+V+Act+Ind+Prs+Sg1`, `pankki+N+Cmp/SgNom+Cmp#automaatti+N+Sg+Ela`)
  into the same `Analysis` shape. Compound-aware via `#` delimiters.
  Handles Finnish AND Estonian tag conventions (most tags overlap;
  Estonian-specific tags like `+Pers`/`+Imprs` are silently ignored
  for now).

- **`Lemmatize(lang, word)`** — top-level entry point. For Finnish,
  queries both VFST and HFSTOL and merges by `(lemma, UPOS)`:
  Voikko-priority on overlap, Giellalt fills coverage gaps,
  compound HFSTOL analyses are deferred so non-compound readings win
  for the same `(lemma, UPOS)`. For Estonian, Giellalt-only.

### Where the ideas came from

- **libvoikko** by Harri Pitkänen (https://voikko.puimula.org) — the
  curated Finnish morphology used by LibreOffice, Firefox, and most
  open-source Finnish tooling. It IS the de facto Finnish FST.
  We use its compiled `mor.vfst` data file and we ported its
  `UnweightedTransducer` runtime algorithm to Go.

- **HFST** (Helsinki Finite-State Toolkit) by University of Helsinki
  (https://hfst.github.io) — the academic-standard FST toolchain for
  Finno-Ugric languages. We use its `.hfstol` (optimized-lookup)
  binary format and we ported its `Transducer::get_analyses`
  algorithm to Go.

- **Giellalt** by UiT The Arctic University of Norway and partners
  (https://giellalt.github.io) — the multilingual Sami / Finno-Ugric
  morphological-resources collective. We use their `lang-fin` and
  `lang-est-x-utee` to build the `.hfstol` data files for Finnish
  and Estonian respectively. They produce the source lexc/twolc files;
  HFST compiles them; we read the result.

The combination — running both Voikko (curated) and Giellalt (broader)
through pure-Go format readers, embedded data, no native dependency —
is, as far as we can tell, novel in the Finnic-NLP ecosystem.

## Why this beats the original Phase 4 plan

| Dimension | Original Phase 4 (SQLite seed) | New FST runtime |
|---|---|---|
| **Lookup time** | ~5 µs (SQLite hash) — depends on caching | ~35–43 µs/word — runs full morphological analysis |
| **Disk footprint (data)** | ~250 MB seed table + indexes | ~40 MB embedded |
| **Build dependency** | Voikko paradigm generator (which doesn't exist as a released tool) | Giellalt source build (one-time, by maintainers; reproducible) |
| **Coverage** | Voikko-only or Giellalt-only — pick one | Voikko + Giellalt union, dedupe-merged |
| **Runtime deps** | None new (pure SQLite) | None new (pure Go, no cgo, no shared lib) |
| **Refresh cost** | Re-run Voikko build pipeline + re-import | Bump vendored `.vfst` / `.hfstol` files (and rebuild Go binary) |
| **Distribution** | Ship the .db file (or have users build it) | Ship one Go binary; embedded data inside |
| **Languages** | One per phase | FI + ET in three PRs |

Note that the FST path is **slower per query** than a pure SQLite hash
hit, because it runs real morphological analysis instead of a single
hashtable lookup. For us this is a net win because:

1. The FST result includes *more* than a SQLite row would — it gives
   you Lemma, UPOS, GrammarLabel, Number, Tense, Mood, Person all in
   one pass, where the SQLite path would need multiple joins.
2. The FST handles forms the SQLite forms-table doesn't have rows for
   — including productive compounds, derivations, and rare inflections.
3. The eval shows the FST is the *only* way we get grammar_label
   coverage above 0% on most datasets — because the SQLite path was
   just storing surface→lemma pairs without grammatical features.
4. 35–43 µs is well below user-perceptible thresholds. A typical
   sentence (10–20 words) lemmatizes in well under 1 ms.

## Performance

Measured 2026-05-06 on Apple Silicon (M-series), Go 1.21:

```
FI Lemmatize: 180000 calls in 7.69s = 42.72 µs/call
ET Lemmatize: 180000 calls in 6.33s = 35.19 µs/call
```

FI is slower than ET because each FI call queries *both* the VFST and
HFSTOL backends and merges results. ET is single-backend.

Memory: each Transducer holds its file contents + a small symbol
table. For the FI runtime, that's the 4 MB `mor.vfst` + the 26 MB
`analyser-gt-desc.hfstol` + ~1 MB of derived structure = ~31 MB
resident. Add the ET 11 MB and we're at ~42 MB total.

Embedded binary size growth vs pre-FST:
- mor.vfst: 4 MB
- FI analyser-gt-desc.hfstol: 26 MB
- ET analyser-gt-desc.hfstol: 11 MB
- Total: ~41 MB added to the Go binary

The embedded data is read directly without a copy on `New()`; we
`hold` slices over the embedded bytes. So the 41 MB is paid once
(at binary load) and reused across all goroutines.

## Eval results

All numbers are the **custom** parser mode against the
`testdata/parser-eval/{fi,et}/gold/` datasets, captured by
`scripts/parser-comparison{,-et}.sh`. See:

- Pre-FST baseline (PR 0): [docs/baselines/2026-05-06-pre-fst-comparison-{fi,et}.md](baselines/)
- Post-FST final (PR 4): [docs/baselines/2026-05-06-final-{fi,et}.md](baselines/)

### Finnish

| Dataset | Cases | Metric | Pre-FST | Post-FST | Δ |
|---|---:|---|---:|---:|---:|
| fi-core | 6 | lemma | 85.0% | 85.0% | +0.0 |
|  |  | pos | 90.0% | 90.0% | +0.0 |
|  |  | full | 35.0% | 35.0% | +0.0 |
|  |  | coverage | 95.7% | **100.0%** | **+4.3** |
| fi-grammar | 80 | lemma | 96.8% | **97.4%** | **+0.6** |
|  |  | pos | 98.1% | **98.7%** | **+0.6** |
|  |  | grammar | 0.0% | **1.4%** | **+1.4** |
|  |  | full | 51.3% | **51.9%** | **+0.6** |
|  |  | coverage | 99.7% | **100.0%** | **+0.3** |
| fi-manual-v1 | 22 | lemma | 81.4% | **82.9%** | **+1.5** |
|  |  | pos | 85.7% | **87.1%** | **+1.4** |
|  |  | grammar | 0.0% | **13.3%** | **+13.3** |
|  |  | full | 41.4% | **62.9%** | **+21.5** |
|  |  | coverage | 91.2% | **96.9%** | **+5.7** |
| fi-manual-v2 | 4 | (unchanged — already at 100% coverage pre-FST) |  |  |  |

### Estonian

| Dataset | Cases | Metric | Pre-FST | Post-FST | Δ |
|---|---:|---|---:|---:|---:|
| et-grammar | 50 | lemma | 88.6% | 88.6% | +0.0 |
|  |  | pos | 96.2% | 96.2% | +0.0 |
|  |  | grammar | 0.0% | **2.0%** | **+2.0** |
|  |  | full | 42.9% | 42.9% | +0.0 |
|  |  | coverage | 98.9% | **100.0%** | **+1.1** |
| et-manual | 4 | lemma | 77.8% | **88.9%** | **+11.1** |
|  |  | pos | 77.8% | 77.8% | +0.0 |
|  |  | grammar | 0.0% | **16.7%** | **+16.7** |
|  |  | full | 11.1% | **22.2%** | **+11.1** |
|  |  | coverage | 91.7% | **100.0%** | **+8.3** |

The biggest gains are on the manual datasets (fi-manual-v1 lemma +1.5,
full +21.5; et-manual lemma +11.1, full +11.1). Those are the
hand-curated harder cases where the rule-based SQLite path was
under-resolving — exactly the cases the FST path was supposed to help
with.

The grammar-label improvements (+1.4 to +16.7 across datasets, all
from a 0.0% pre-FST baseline) are entirely new coverage — the SQLite
path stored only `(form, lemma, pos)` triples without grammatical
features, while the FST returns Case/Number/Person/Tense/Mood for
free.

## Distribution and licensing

The vendored data files inherit upstream licenses; the Go runtime
code is ours under the project's existing license.

- **`mor.vfst`** (~4 MB): tri-licensed MPL 1.1 / GPLv2+ / LGPLv2.1+.
  Verbatim license at `pkg/lemmatizer-fi-et/data/fi/LICENSE-libvoikko.txt`.
  Source: copied from `/opt/homebrew/Cellar/libvoikko/4.3.3/lib/voikko/5/mor-standard/mor.vfst`.
  sha256 `5d3bfa40…0d0f957d`.

- **`pkg/lemmatizer-fi-et/data/fi/analyser-gt-desc.hfstol`** (~26 MB):
  GPLv3+. Verbatim licenses at
  `pkg/lemmatizer-fi-et/data/fi/LICENSE-hfst.txt` (HFST runtime
  algorithm) and `LICENSE-giellalt-lang-fin.txt` (linguistic data).
  Built locally from `giellalt/lang-fin@HEAD` via HFST 3.17.1.
  sha256 `3bc4802d…0b540326`.

- **`pkg/lemmatizer-fi-et/data/et/analyser-gt-desc.hfstol`** (~11 MB):
  GPLv3+. Verbatim licenses at
  `pkg/lemmatizer-fi-et/data/et/LICENSE-hfst.txt` and
  `LICENSE-giellalt-lang-est.txt`. Built locally from
  `giellalt/lang-est-x-utee@HEAD` via HFST 3.17.1.
  sha256 `fd3e5ec6…ba1150e17`.

To refresh the FI data, rebuild `giellalt/lang-fin` against a current
HFST toolchain (the spike report has the bootstrap recipe) and replace
the file. To refresh the Voikko data, copy a newer `mor.vfst` from a
libvoikko release. To refresh ET, rebuild `giellalt/lang-est-x-utee`.

## Future work (not in scope here)

- **Promote the FST step earlier in the resolution chain.** Currently
  it runs as step 5 (after SQLite forms / lemmas / case-suffix), which
  is the safest integration. Promoting it ahead of case-suffix
  stripping would likely improve both speed and correctness, but
  requires a larger eval to confirm no regressions. Worth a
  follow-up PR.
- **ET-specific feature mapping.** Estonian-specific tags
  (`+Pers`/`+Imprs` → UD `Voice=Act/Pass`, `+Aff`/`+Neg` → UD
  `Polarity=Pos/Neg`) are silently ignored by `giellaltmap` today.
  Adding them is mechanical; gated on the eval graders caring about
  those features.
- **Memory-mapped instead of embedded.** For deployments that ship the
  data alongside the binary, mmap'd files would let multiple binary
  instances share the same physical memory pages. Embedding is
  simpler and sufficient at our current scale.
- **More languages.** The `hfstol` runtime is language-agnostic — any
  Giellalt language with a published `.hfstol` (Sami variants, Kven,
  Greenlandic, etc.) can be added with a few lines.
