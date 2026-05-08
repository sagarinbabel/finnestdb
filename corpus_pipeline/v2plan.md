# Corpus pipeline — v2 follow-ups + v1 handoff notes

This file is the durable record of what's done, what's deferred, and the
trigger conditions for picking each item back up. Per the plan (v1plan.md
§"v2 follow-ups"), this now lives in tracked source at
`corpus_pipeline/v2plan.md` so Codex, Claude, and human reviewers can share the
same roadmap. Runtime corpus data stays local-only under `localdata/`.

## v1 status snapshot — 2026-05-08 (updated 2026-05-08 evening)

### What landed in this session

**Core pipeline (all green):**
- ✅ Module skeleton at `corpus_pipeline/` — module path `finnestdb/corpus_pipeline`, replace directive validates, smoke-test imports `finnestdb/internal/parserffi` and runs.
- ✅ `internal/cli` — preflight (FST + dict), lang code normalization, roots resolver
- ✅ `internal/sources` — manifest discovery
- ✅ `internal/fetcher` — HTTP downloader with sha256 sidecars, idempotent
- ✅ `cmd/fetchcorpus` — registry-driven; FI registry has 4 OPUS sources, ET has 4. ~200 MB fetched in 19 s.
- ✅ `cmd/extractcorpus` with extractors: `fixture`, `text`, `md_lingq_parallel`, `epub` (pure-Go zip+XHTML, 230 EPUBs → 176 MB text in 14 s), `gz` (OPUS .txt.gz with pseudo-doc grouping)
- ✅ `cmd/aggregatecorpus` — 4-phase in-memory aggregator: phase 1 parserffi tokenize + dedup, phase 2 parsecore.Analyze + Lemmatizer.Lemmatize multi-analysis, phase 3 mining outputs (5 TSVs initial), phase 4 deterministic ID assignment + writers.
- ✅ `cmd/corpusverify` — preflight + hard/soft gates + fixture probes (FI Talossa→talo/Ine, ET joon→jooma/VERB+joon/NOUN), error_report.txt + error-history.jsonl on fail.
- ✅ `cmd/corpuspromote` — chains extract→aggregate→verify, updates promotion-state.json.
- ✅ Makefile with all targets: fetch/extract/aggregate/verify/promote (-fi/-et/both), bootstrap-tarball (3-way split), corpus-cache-clear.
- ✅ `docs/CORPUS_PIPELINE.md` — operator manual with quick reference, profiles, repair loop, troubleshooting.

**Reorg done:**
- ✅ 230 FI EPUBs → `fi-corpus/epub/raw/` (with manifest.json kind=prose format=epub)
- ✅ 49 FI manual .txt files → `fi-corpus/manual/raw/` (with manifest.json)
- ✅ Old scattered subfolders (`raw/`, `text/`, `clean/`, `lists/`, `meta/`) → `manual/aux/` for provenance
- ✅ 2 ET LingQ md files → `et-corpus/lingq-parallel/raw/` (with manifest.json format=md_lingq_parallel)
- ✅ FI fixture + ET fixture sources for smoke gate

**Real data verified:**
- FI smoke (fixture only): 10 sentences → 47 surfaces, 0% unresolved, fixture probes pass
- ET smoke (fixture + LingQ): 5080 sentences → 4099 surfaces, joon multi-analysis confirmed
- FI opus-tatoeba alone: 5.4 MB → 145K sentences, 84K surfaces, 0% unresolved, 7K ambiguous, 20 s wall clock
- FI pilot full aggregate (7 sources, ~550 MB): in progress at session-end, see v2plan.md§"Pilot status" below

### Full results — FI ✅ (54 min wall clock with -scratch + docs-count fix, all 14 sources)

```
14 sources: fixture, manual, epub, gutenberg, opus-tatoeba, opus-bible,
            opus-emea, opus-ecb, opus-europarl, opus-finlex, opus-eubookshop,
            opus-wikimatrix, opus-jrc-acquis, opus-ted2020 (opus-opensubtitles
            URL registered but not fetched, so empty)

sentences_unique:    15,078,951
unique_surfaces:      7,505,796
unresolved_rate:      0.0019%
wordlist.tsv:         2.5 GB
sentences.tsv:        2.1 GB

Top FI surfaces (occurrences):
  ja          9,068,233    ja/CCONJ      (and)
  on          5,528,373    on/VERB       (is)
  että       2,019,266    että/CCONJ   (that)
  tai         1,532,581    tai/CCONJ     (or)
  oli         1,524,867    olla/VERB     (was)
  ei          1,472,709    ei/VERB       (no)
  Euroopan      967,568    euroopan/NOUN (Europe-gen)
  myös         856,495    myös/ADV     (also)
  sekä         828,990    sekä/CCONJ   (as well as)
  ovat          796,586    olla/VERB     (are)

Mining outputs:
  parser-disagreements.tsv:           80 MB (huge signal)
  internal-consensus-candidates.tsv:  3.1 MB
  high-frequency-ambiguous.tsv:       2.1 MB
```

### (Earlier) Pilot results — FI ✅ (5m 15s wall clock — pre-expanded set)

```sh
$ make aggregate-corpus-fi    # 7 sources, 550 MB extracted text
{
  "sources": 7,
  "sentences_unique": 2813008,
  "unique_surfaces": 1395466,
  "unresolved_rate_prose": 0.0000115   # essentially zero
}
$ make corpus-verify-fi -- PROFILE=pilot
VERIFY PASS (lang=fi profile=pilot)
```

Output sizes:
- `wordlist.tsv`: 624 MB / 1,443,757 rows (multi-analysis enabled)
- `sentences.tsv`: 352 MB / 2,813,008 deduped sentences
- `sentence_occurrences.tsv`: 160 MB / per-occurrence provenance
- Top FI surfaces: `ja` (1.9M), `on` (881K, multi-analysis), `oli` (513K),
  `ei` (405K, multi-analysis VERB+AUX), `että` (403K), `tai` (382K)

**The in-memory aggregator handled 550 MB without OOM** — peak RSS was
~12 GB. v2.4 (SQLite scratch + concurrent workers) is NOT urgent yet;
trigger remains "when full corpus aggregation OOMs."

### Pilot results — ET (initial 6 sources, 4m 14s)

```
6 sources: fixture, lingq-parallel, 4 OPUS small
sentences_unique:   1,123,769
unique_surfaces:    1,448,978
unresolved_rate:    0.0036%
wordlist.tsv:       704 MB
```

### Expanded results — ET (13 sources, 11 min)

```
13 sources: above + 6 more OPUS + hf-err-newsroom (registry only — not yet fetched)
sentences_unique:   4,037,716
unique_surfaces:    3,352,756
total_tokens:       96,090,602
ambiguous_surfaces: 41,551
unresolved_rate:    0.0018%
wordlist.tsv:       1.46 GB
```

### Enriched results — ET (vabamorf, ~7 min)

```
external_analyzer:        vabamorf (via estnltk.vabamorf.morf)
unique_surfaces_enriched: 3,352,756
silver_candidates:        1,438,782 (43% agreement with parser_choice)
wordlist-enriched.tsv:    225 MB
silver-candidates.tsv:    53 MB
```

### What's working (smoke-green end-to-end)

```sh
cd corpus_pipeline
make corpus-promote-fi   # smoke profile passes
make corpus-promote-et   # smoke profile passes
```

The vertical slice is validated:
- `parserffi.AnalyzeText` tokenization
- `parsecore.Analyze` (custom mode) parser_choice enrichment
- `Lemmatizer.Lemmatize` multi-analysis enumeration
- Wordlist collapsing on identical (lemma, pos, feats), separating ambiguous
- Deterministic sentence IDs (sorted by source, document_id, sentence_ix, hash)
- Mining outputs: unresolved, poetry-unresolved, high-frequency-ambiguous
- Anti-circularity: silver-candidates.tsv NOT written by aggregator
- Hard preflight gates: FST tables + dict DB schema + dict counts
- Soft gates: unresolved_rate threshold
- Per-language fixture probes (FI Talossa→talo/Ine, ET joon→jooma/VERB+joon/NOUN)

### Verified outputs (per language)

```
localdata/{fi,et}-corpus/_derived/
├── wordlist.tsv                     # 20 columns including 3-way counts
├── sentences.tsv                    # deduped, deterministic IDs
├── sentences_user_friendly.tsv      # filtered learner-facing sentence bank
├── sentence_occurrences.tsv         # full provenance
├── poems.tsv                        # empty in v1 (no poetry sources active)
├── documents.tsv
├── manifest.tsv
├── build_metadata.json              # parser_version, fst_tables_sha, dict_fingerprint
├── qa-report.json                   # split prose/poetry totals
├── qa-gate.json                     # hard/soft gate results
├── promotion-state.json             # last-completed profile
├── errors/                          # populated only on failure
└── mining/
    ├── unresolved.tsv               # sorted by surface_count_prose desc
    ├── poetry-unresolved.tsv        # threshold: count_poetry≥10 AND ≥5×prose
    ├── parser-disagreements.tsv     # header-only in v1 (basic-vs-custom comparison TODO)
    ├── high-frequency-ambiguous.tsv
    └── internal-consensus-candidates.tsv  # header-only in v1 (TODO)
```

### Architecture decisions honored

- ✅ Module path `finnestdb/corpus_pipeline` (Go internal/ visibility)
- ✅ `replace finnestdb => ..` for imports
- ✅ `LEMMATIZER_TABLES_DIR` env var set before parsecore/store calls
- ✅ Lang code normalization at the boundary (lowercase paths, uppercase parser/FST)
- ✅ Dictionary DB preflight uses real schema (forms/lemmas/dict_metadata)
- ✅ Cache key includes parser_version + fst_tables_sha + dict_fingerprint
- ✅ Phase 4 deterministic ID assignment after sort key
- ✅ TSV via encoding/csv with Comma='\t' (handles tab/quote escaping)
- ✅ Wordlist surface_count_prose / _poetry / _total trio (poetry contributes, doesn't dominate)
- ✅ example_ref_type / example_ref_id / example_text triple (no dangling refs).
  *Updated 2026-05-09 (PR #2 of roadmap):* `example_text` removed from
  canonical `wordlist.tsv` and the mining TSVs. The pair
  `example_ref_type` + `example_ref_id` is the durable reference; readers
  recover the example body by joining against `sentences.tsv` (or
  `poems.tsv` when the type is `poem`). The user-friendly export
  `wordlist_user_friendly.tsv` carries the same ref pair plus meaning,
  parsed morphology, and the standard count/provenance columns.

## Near-term tracked PR roadmap

The detailed PR sequence lives in
[`docs/PR_ROADMAP.md`](docs/PR_ROADMAP.md). Each PR should explain **why** it
exists, what generated artifacts change, and how downstream code reconstructs
any denormalized data that is removed.

Planned work (four items — first three are active, fourth is a later spike):

1. **Meaning sources research** (ACTIVE — prerequisite for item 2).
   Audit DB gloss coverage, identify external meaning sources (Wiktionary,
   Kielitoimiston sanakirja, Ekilex public data), record license constraints,
   and build/extend an import path so meanings reach the dictionary DB.
   - **Effort:** ~6-10 hours.
   - **Trigger:** Next implementation PR after this roadmap lands.
2. **Canonical cleanup + user-friendly wordlist** (depends on item 1).
   Drop `example_text` from canonical outputs, document reconstruction via
   `example_ref_id` joins, and add `wordlist_user_friendly.tsv` with meanings
   from the dictionary DB.
   - **Effort:** ~3-5 hours.
   - **Trigger:** After item 1 delivers adequate gloss coverage.
3. **Sentence export + EPUB extraction cleanup** (independent).
   Improve `extract_epub` to skip nav/front-matter junk and add
   `sentences_user_friendly.tsv` as a filtered derived export. These are coupled
   because better extraction reduces what the sentence filter has to catch.
   - **Effort:** ~4-6 hours.
   - **Trigger:** Sentence-bank examples are consumed by the app or by manual
     quality review.
4. **Later: interlinear glossing prototype.**
   Investigation/prototype work, not a normal shipping PR. Prototype a tool
   that takes a sentence and emits lemma/gloss/FEATS rows per surface.
   - **Effort:** ~8-16 hours (spike + prototype).
   - **Trigger:** Items 2 and 3 are complete, and the app needs
     morphology-aware learner explanations beyond lemma + meaning.

## Deferred to v2 — with triggers

The hard work below didn't ship in this session. Each item has a **trigger
condition** that should make a future session pick it up. Until a trigger
fires, the item stays parked.

### v2.1 — ✅ DONE 2026-05-08

230 FI EPUBs moved to `fi-corpus/epub/raw/`. 49 .txt files in
`manual/raw/` (28 originals + 20 from old text/final + 1 Pintaremontti).
Scattered subfolders (`raw/`, `text/`, `clean/`, `lists/`, `meta/`)
consolidated into `manual/aux/{raw_html,old_text_versions,clean_archive,lists,meta}/`.
`manifest.json` written for both `epub/` and `manual/`. ET LingQ md
files → `et-corpus/lingq-parallel/raw/`.

### v2.2 — ✅ PARTIALLY DONE 2026-05-08 (expanded later in session)

Done:
- `internal/fetcher/fetcher.go` (~100 LoC HTTP downloader with sha256
  sidecars, HEAD probes, atomic .part renames, idempotent re-runs).
- `cmd/fetchcorpus/main.go` + `sources_fi.go` + `sources_et.go`.
- **11 verified FI sources** (HEAD-probed): opus-tatoeba (1.9 MB),
  opus-bible (1.3 MB), opus-emea (20 MB), opus-ecb (72 MB),
  opus-europarl (102 MB), opus-finlex (99 MB), opus-eubookshop (135 MB),
  opus-wikimatrix (218 MB), opus-jrc-acquis (29 MB),
  opus-opensubtitles (1.07 GB), opus-ted2020 (1.5 MB).
  Total compressed: ~1.95 GB.
- **10 verified ET sources** (HEAD-probed): opus-tatoeba (63 KB),
  opus-bible (329 KB), opus-emea (18 MB), opus-jrc-acquis (87 MB),
  opus-europarl (30 MB), opus-eubookshop (24 MB), opus-wikimatrix (85 MB),
  opus-opensubtitles (401 MB), opus-ted2020 (795 KB),
  opus-paracrawl (369 MB).
  Total compressed: ~1.02 GB.
- All URLs HEAD-probe OK. Pilot fetched 4 of FI's 11 + 4 of ET's 10
  (~200 MB) in 19 seconds.

To pull the rest: `cd corpus_pipeline && make fetch-corpus`.
Estimated wall clock: ~3-5 min on a typical connection.

Still deferred (was originally ~25 sources per language):
- ~21 more FI sources: Yle Kielipankki s-vrt (4.8 GB), Wikipedia,
  Parliament Eduskunta, Leipzig newscrawl/news/wikipedia, Parsebank,
  remaining OPUS (Europarl, Finlex, JRC-Acquis, WikiMatrix, EUbookshop,
  TED2020, Bible, Tatoeba), SKVR, FrequencyWords, runosto.net,
  Selkouutiset, Gutenberg-FI, infopankki, opus-cc100.
- ~21 more ET sources: OPUS DocHPLT/CC-100/NLLB/CCMatrix/HPLT/MultiHPLT/
  OpenSubtitles/ParaCrawl/MultiParaCrawl/Wikipedia/Europarl/EMEA/Bible/
  Tatoeba, Leipzig, Wikipedia dump, Gutenberg-ET, FrequencyWords,
  TalTechNLP/err-newsroom (HF), Riigikogu, ERAB, EEVA.
- **Trigger**: When the 4 sources per lang aren't enough register
  diversity for the parser/word-list quality the user wants.
- **Effort**: ~2 hours per ~5 sources (mostly URL verification +
  registry entries; existing `extract_gz.go` handles all OPUS sources).

### v2.3 — Format extractors — ✅ MOSTLY DONE 2026-05-08

- **Status**: ✅ DONE: `fixture`, `text`, `md_lingq_parallel`,
  `epub` (pure-Go zip+XHTML, 230 EPUBs → 176 MB in 14 s),
  `gz` (OPUS .txt.gz with synthetic-period for sentence-splitting),
  `csv`/`tsv` (with column-picking heuristic for Eduskunta-shaped data),
  `leipzig` (tar+TSV with `-sentences.txt` extraction),
  `skvr` (XML with `<runo>`/`<line>` event-based decoder, routes to poems.jsonl),
  `html` (regex tag stripper, polite scrape input),
  `huggingface` (jsonl + jsonl.gz with `text_fields=` hint or longest-string heuristic),
  `riigikogu` (delegates to html for v1; specialize later),
  `erab` (delegates to skvr for v1),
  `eeva` (delegates to html for v1).
- **Stubbed**: `vrt` (Yle Kielipankki — needs zip+VRT-format walker, not in active source list),
  `wiki` (MediaWiki XML — relies on OPUS-Wikipedia .txt.gz instead which uses `gz`).
  Both stubs return nil (no error) so the pipeline keeps working when registry includes them.
- **Missing**: `vrt` (Yle Kielipankki), `leipzig` (tar+TSV),
  `wiki` (MediaWiki XML — needs wikiextractor or pure-Go port),
  `csv` (Eduskunta), `skvr` (XML), `html` (polite scrape),
  `huggingface` (parquet/jsonl — needs HF hub or direct API),
  `riigikogu` (HTML scrape), `erab` (TXT/XML),
  `eeva` (HTML scrape with prose/poetry routing).
- **Trigger**: Each individual extractor's trigger is "we want this
  source ingested." Easy to do one-at-a-time.

#### v2.3 follow-up — extract_gz refinement (known issue)

- **Status**: Working but produces some monster sentences when input
  lines lack standard sentence-ending punctuation (e.g. EU legal
  chemistry text in OPUS jrc-acquis emits 100 KB single "sentences"
  because parserffi can't find boundaries).
- **Workaround in v1**: `example_text` column in wordlist.tsv is
  capped at 400 chars. Underlying `sentences.tsv` keeps full text
  (no info loss).
- **Real fix (v2)**: extract_gz should treat each .gz line as a
  separate paragraph (OPUS convention is "one sentence per line"),
  not bundle 500 lines into one pseudo-doc. That eliminates the
  monster-sentence problem at the source.
- **Effort**: ~30 min refactor of extract_gz.go.
- **Trigger**: When `mining/poetry-unresolved.tsv` or qa-report soft
  warnings flag suspicious very_long sentence rates above thresholds
  the pilot/full runs care about.

### v2.4 — `cmd/aggregatecorpus` scale features — ✅ WIRED + TESTED 2026-05-08

`-scratch` flag wires SQLite scratch DB for sentences + occurrences +
documents (surfaces stay in-memory). Per-source flush in phase 1
prevents RSS blow-up during single-source ingestion. Phase 4 reads via
in-memory hash→id and bulk-loaded hash→text maps (no per-row UPDATE,
which was the WAL-write bottleneck at scale).

Empirical bottlenecks fixed during this work (all live in
learnings-from-the-first-run.md):
1. **Per-surface `docsProse map[string]struct{}`** — replaced with
   counter + last-seen-doc-id. Saved ~7 GB RSS at 3M surfaces (L29).
2. **9.5M individual UPDATE statements** to set final_id — replaced
   with bulk-load of `(hash, text)` into a Go map and write-time
   resolution via in-memory hash→id (L38).

Tested: FI fixture + smoke (sub-second), FI full-scratch (~14 min in
flight at session-end-2 with 6.99M sentences flushed; will finish
phase 2/4 and produce wordlist).

### v2.5 — Mining: parser-disagreements + internal-consensus — ✅ DONE 2026-05-08

Phase 2 of `cmd/aggregatecorpus` now also calls `parsecore.Analyze(... "basic")`
alongside the `"custom"` call, captures both choices, and:
- Emits a row to `mining/parser-disagreements.tsv` when basic and custom
  disagree on (lemma, pos)
- Emits a row to `mining/internal-consensus-candidates.tsv` when basic +
  custom + FST top hit all agree on (lemma, pos)
Both sorted by surface_count_prose desc.

### v2.6 — `cmd/enrichcorpus` — ✅ DONE 2026-05-08 (graceful skip, untested at scale)

`cmd/enrichcorpus -lang fi|et` runs.
- FI: looks for `omorfi-disamb-cmdline` on PATH (or `FINNESTDB_OMORFI_CMD`).
  Pipes surfaces through long-lived subprocess.
- ET: invokes `python3 scripts/estnltk-batch.py` (which loads estnltk once
  and reads/writes TSV per surface).
- **Both gracefully skip** with helpful install messages if the analyzer
  isn't available. Exit 0 so promotion ladder stays green.

Tested: graceful-skip path (omorfi + estnltk both missing on this machine).
**Not tested**: actual enrichment with installed analyzer — requires
`brew install omorfi` (FI) or `pip install estnltk` (ET).

### v2.7 — `cmd/epubdeck` — ✅ DONE 2026-05-08

Per-book wordlist extractor. Reads `localdata/{lang}-corpus/epub/per-book/<slug>.txt`
(produced by extract_epub), runs aggregator-style parserffi tokenize +
parsecore + Lemmatizer enrichment scoped to one book, writes
`epub/decks/<slug>.tsv` with same column shape as wordlist.tsv plus
example_text per row.

Tested: 1 book ("Kostaja" by Alastair Reynolds) → 5.4 MB deck in 9 seconds.
Top entries reveal real ambiguity (`on` → olla VERB / `kuin` → mistakenly
NOUN-instrumental). Standalone — doesn't pollute main wordlist.

`make epub-deck-fi EPUB=<filename>` runs one specific book; without
`EPUB=` runs all 230.

### v2.8 — Gutenberg scraper — ✅ DONE 2026-05-08 (Makefile delegation)

Main repo's `cmd/scrapegutenberg/main.go` already supports `-lang fi|et`.
Added Makefile targets `scrape-gutenberg-fi` / `scrape-gutenberg-et` that
invoke it with `-out localdata/{lang}-corpus/gutenberg/raw/` and
`-manifest localdata/{lang}-corpus/gutenberg/manifest.jsonl`. No new Go
code. Honors the cleanliness invariant.

### v2.4 — `cmd/aggregatecorpus` scale features — ✅ DONE 2026-05-08

`-scratch` flag wires SQLite scratch DB for sentences + occurrences +
documents (surfaces stay in-memory). Per-source flush in phase 1
prevents RSS blow-up. Phase 4 reads via streaming SELECT cursors;
sentences.tsv and occurrences.tsv built without loading all in memory
at once (except a hash→id Go map, ~600 MB at full FI scale).

Two empirical bottlenecks fixed during this work:
1. **`docsProse map[string]struct{}` per surface** — replaced with
   counter + last-seen-doc-id. Saved ~7 GB RSS at 3M surfaces.
2. **9.5M individual UPDATE statements** to set final_id — replaced
   with bulk-load of `(hash, text)` into a Go map and write-time
   resolution via in-memory hash→id. Saved 20+ min wall-clock.

Tested at scale: FI fixture + smoke. Full FI scratch run in flight
(see "Pilot results" below for numbers).

### v2.4 — `cmd/aggregatecorpus` scale features — original notes preserved

- **Status**: Scaffolding written: `cmd/aggregatecorpus/scratch_db.go`
  has the schema + `openScratch()` helper, ready to wire. Not yet used
  by phases 1-4 — the in-memory implementation handles current pilot
  (12 GB RSS for 2.8M sentences = ~4 KB/sentence empirical).
- **Trigger fired**: tried to run FI expanded aggregate (22.7M lines
  extracted from 14 sources). Estimated peak ~80-90 GB RSS on a 64 GB
  machine without swap. **Won't fit.**
- **Workaround in v1**: `-skip` flag added to aggregator. Aggregate
  the smaller subset (skip opus-wikimatrix, opus-eubookshop, opus-finlex,
  opus-opensubtitles, opus-paracrawl) for now. Captures ~750 MB FI
  text instead of 2.4 GB.
- **Real fix (v2.4 wiring)**:
  - Replace `state.surfaces`, `state.sentences`, `state.occurrences`,
    `state.documents`, `state.wordlistRows` maps/slices with SQL queries
    against `_scratch.db`.
  - Phase 1: `INSERT INTO tmp_*` per token / sentence / occurrence as
    parserffi emits them. Single writer goroutine; N parser-workers.
  - Phase 2: `SELECT DISTINCT surface FROM tmp_surfaces`, enrich each,
    `INSERT INTO tmp_wordlist`.
  - Phase 4: `SELECT ... ORDER BY ... ` to drive deterministic-ID
    assignment + TSV writes.
  - Surface-analyses cache: `cache/surface-analyses.tsv` keyed by
    `(surface, lang, parser_version, fst_tables_sha, dict_fingerprint)`.
- **Effort**: ~5-8 hours of careful refactor. Schema is ready; the
  work is rewriting the four phase functions to use SQL instead of
  in-memory maps.
- **Trigger**: When user wants to aggregate the full source set
  (including opus-opensubtitles 1.07 GB, opus-paracrawl 369 MB,
  hf-err-newsroom 169 MB) without -skip.

### v2.5 — Mining: parser-disagreements + internal-consensus

- **Status**: Header-only TSVs written. No comparison logic.
- **Missing**: For each surface, run `parsecore.Analyze(... "basic")`
  alongside the `"custom"` call already in phase 2. Compare chosen
  analyses; emit row when they disagree. Internal-consensus when basic
  + custom + FST top hit all agree.
- **Why deferred**: Smoke doesn't need them populated.
- **Effort**: ~2 hours.
- **Trigger**: When parser-improvement workflows want this signal.

### v2.6 — `cmd/enrichcorpus` (omorfi + estnltk batch adapters)

- **Status**: Not started.
- **Missing**: Long-lived omorfi-disamb-cmdline subprocess (FI),
  long-lived Python subprocess running `scripts/estnltk-batch.py` (ET),
  JSON-line protocol over stdin/stdout, resumable cache. Reads
  `wordlist.tsv`, emits `wordlist-enriched.tsv` + `mining/silver-candidates.tsv`.
- **Why deferred**: User explicitly said "not part of the initial run.
  Have it ready, I'll run it myself when I want richer FEATS."
- **Effort**: ~7 hours.
- **Trigger**: When user wants richer FEATS than the runtime parser
  emits, OR when silver-tier parser-eval data is needed.

### v2.7 — `cmd/epubdeck`

- **Status**: Not started.
- **Missing**: Per-book wordlist extractor. Reads
  `localdata/{lang}-corpus/epub/per-book/<slug>.txt`, runs aggregator
  logic scoped to one book, writes `epub/decks/<slug>.tsv`.
- **Why deferred**: Smoke doesn't need it.
- **Effort**: ~2 hours (mostly reuses aggregatecorpus internals,
  scoped to one source).
- **Trigger**: User wants per-book vocabulary deck for a specific EPUB.

### v2.8 — `cmd/scrapegutenberg-corpus`

- **Status**: Not started. Main repo's `cmd/scrapegutenberg/` exists for
  FI but writes to wrong path and only does FI.
- **Missing**: Local re-impl supporting both FI and ET, writing into
  `localdata/{fi,et}-corpus/gutenberg/raw/`.
- **Effort**: ~1 hour (mostly copy main repo's code, fix output path,
  add ET search).
- **Trigger**: When Gutenberg sources need to be in pilot/full.

### v2.9 — Pilot + full profile runs

- **Status**: Profiles exist as concepts; only smoke runs end-to-end.
- **Missing**: `cmd/corpuspromote` doesn't yet drive the fetcher with
  `-limit-bytes` for pilot. Full needs the entire fetcher driving.
- **Effort**: ~2 hours after fetchcorpus + extractors are in place.
- **Trigger**: Once enough format extractors exist that pilot has
  meaningful coverage (~3-5 sources per language minimum).

### v2.10 — Bootstrap tarball (3-way split)

- **Status**: Not started.
- **Missing**: `make bootstrap-tarball` target that produces
  `finnestdb-bootstrap-{code,fi,et}.tgz`.
- **Effort**: ~30 minutes (just tar invocations).
- **Trigger**: User wants to hand off the corpus to another machine.

### v2.11 — Operator doc `docs/CORPUS_PIPELINE.md`

- **Status**: Stub only (this file). The full operator doc — table of
  every make target with what/when/wall-clock/outputs, plus
  troubleshooting playbook — is not written.
- **Effort**: ~1.5 hours.
- **Trigger**: When someone other than the original implementer needs
  to operate the pipeline.

### v2.12 — ET corpus parity — ⚠️ PARTIALLY DONE 2026-05-08

Done:
- ✅ HF err-newsroom (TalTechNLP/err-newsroom train.json.gz, 169 MB)
  added to ET registry. HEAD-probed OK. Will pull into the corpus next
  fetch.
- ✅ Format extractors built/delegated: `huggingface` (real),
  `riigikogu`/`erab`/`eeva` (delegated to html/skvr respectively).
- ✅ Format dispatcher routes all four via known slugs.

Still missing:
- ⚠️ Riigikogu, ERAB, EEVA are scrape-based not URL-bulk-download.
  They need:
  - Riigikogu: per-sitting HTML scraper (~1.5s polite delay) walking
    https://www.riigikogu.ee/...stenogramm pages, accumulate
    `<source>/raw/*.html`, then `extract_html` does the rest.
  - ERAB: manual web-interface export (CLARIN ACA gated). User
    registers at https://www.folklore.ee/regilaul/, requests, downloads
    TXT/XML, drops into `et-corpus/erab-regilaulud/raw/`.
  - EEVA: HTML scraper that walks https://www.eeva.ee/ document index
    + downloads pages, inspects metadata to route prose/poetry to
    `eeva-prose/` vs `eeva-poetry/` source folders.

These extractors EXIST in `cmd/extractcorpus/` and the corresponding
formats route via the dispatcher. The blocker is the data acquisition
step (scraper + manual export). No code blocker.

- **Effort remaining**: ~3-4 hours per scraper.
- **Trigger**: ET frequency curves show register imbalance hurting
  users, OR there's appetite for more ET parser-improvement signal.

## How to resume

When picking this up in a future session, start with:

1. `cd corpus_pipeline && make corpus-promote-fi corpus-promote-et`
   to confirm the smoke gate is still green (validates nothing rotted).
2. Read `v2plan.md` (this file) to pick the next item by trigger.
3. Implement the item on a scoped branch and open a draft PR when the change
   affects tracked pipeline source, schemas, docs, or reports.
4. Add a "✅ done <date>" note to its section here.

## Cleanliness invariant

After every session, `git status --porcelain` from the repo root should show
**no unintended deltas outside `corpus_pipeline/` or `localdata/`** vs. the
pre-session snapshot.
Pre-existing unrelated state (e.g. `design/*.jsx` untracked files) is
fine — only the delta matters.

The `corpus_pipeline/` folder is tracked source. Everything under
`localdata/{fi,et}-corpus/` is gitignored and stays purely local.
