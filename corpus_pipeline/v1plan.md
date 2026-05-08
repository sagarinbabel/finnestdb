# Corpus acquisition pipeline — fetch, extract, aggregate, mine

## Context

`finnestdb` is a JPDB-clone for Finnish/Estonian. The product needs a broad,
register-diverse corpus to:

1. **Improve parser quality** — surface OOV words, expose dictionary gaps,
   feed mining workflows that flag candidates for hand-labeled gold and
   high-confidence silver. Today: parser eval lives on ~37K FI / ~437K ET
   UD-gold tokens. Goal: ~10 GB per language, plus a stream of mined
   parser-improvement candidates.
2. **Build a giant inflected-form word list** — every unique surface form
   across all corpus, deduplicated, with POS / lemma / case / mood / FEATS /
   counts. Surface-form frequency table is the core v1 deliverable for the
   "JPDB word list" feature. Today: doesn't exist.
3. **Build a sentence bank** — every sentence from every source with
   document/sentence-index provenance, available as flashcard-context
   fodder. Today: doesn't exist.
4. **Keep poetry separate** — preserved with line breaks, source-tagged,
   for distinct learning-UX experiments. Today: doesn't exist.

Existing state: `localdata/fi-corpus/` has 22 MB of hand-pasted articles in
ad-hoc structure. `et-corpus/` doesn't exist. `cmd/scrapegutenberg` writes
to a stale path (`localdata/silver-fi/raw`) that violates the single-folder
bootstrap rule. There's no end-to-end fetch/extract/aggregate pipeline.

Local-only by policy: per [`docs/ARTIFACT_POLICY.md`](docs/ARTIFACT_POLICY.md),
nothing in `localdata/` ships in the app or in git. Licensing is tracked
for hygiene but not gated — no commercial/noncommercial allowlists.

## Language conventions — FI and ET are first-class

Every command and Makefile target has three variants, no exceptions:

| Variant | Behavior |
|---|---|
| `make <target>` | Both languages (runs FI then ET, or in parallel where safe) |
| `make <target>-fi` | Finnish only |
| `make <target>-et` | Estonian only |

This applies to: `fetch-corpus`, `extract-corpus`, `aggregate-corpus`,
`add-epub`, `epub-to-text`, `epub-deck`, `enrich-corpus`,
`corpus-verify`, `corpus-promote`. When this plan shows examples in
just FI for brevity, the ET equivalent ALWAYS exists.

Same in CLI: every `cmd/` binary takes `-lang fi|et` (or `-lang both`
where applicable, default = FI for parity with existing
`cmd/parser-compare` convention).

**Smoke fixtures cover both languages.** `testdata/corpus-fixtures/fi/`
+ `testdata/corpus-fixtures/et/` with hand-authored sentences containing
known ambiguities per language:
- **FI**: `tulen` (verb *tulla* 1sg vs noun *tuli* gen), `kuusen`
  (noun *kuusi* gen), `alusta` (noun *alus* part vs verb *aloittaa*
  imperative), `talossa` (noun *talo* inessive)
- **ET**: `joon` (verb *jooma* 1sg vs noun *joon* nominative line),
  `kuus` (numeral six) vs `kuusk` (fir tree), `aitan` (verb *aitama*
  1sg vs noun *ait* partitive)

## Output artifact families

After the pipeline runs, `localdata/{fi,et}-corpus/_derived/` contains:

### Family A — App data exports (3 TSVs)

| File | Purpose |
|---|---|
| `wordlist.tsv` | Giant deduped inflected-form list, one row per (surface, analysis). Multi-analysis per surface enabled. |
| `sentences.tsv` | Sentence bank, deduped across sources, with document/sentence-index provenance. |
| `poems.tsv` | Poetry, line breaks preserved (JSON-escaped), source-tagged. |

### Family B — Parser-improvement mining (5 initial TSVs + enrichment-only silver TSV + report)

| File | Purpose |
|---|---|
| `mining/unresolved.tsv` | High-frequency surfaces the parser couldn't resolve, **prose-first**. Sorted by `surface_count_prose` desc, then `surface_count_total` desc. Feeds dictionary expansion. |
| `mining/parser-disagreements.tsv` | Surfaces where `basic` and `custom` modes disagree. Reuses `cmd/corpusmine` logic. |
| `mining/high-frequency-ambiguous.tsv` | High-frequency surfaces with ≥2 FST analyses. Candidates for hand-labeled gold. |
| `mining/poetry-unresolved.tsv` | Subset of unresolved where poetry count dominates — archaisms, dialect, folk-poetry formulas. Kept separate from main unresolved.tsv so prose-focused parser work isn't noisy. |
| `mining/internal-consensus-candidates.tsv` | Surfaces where `basic` ⊕ `custom` ⊕ FST top hit all agree. NOT silver — basic and custom share dictionary plumbing, so this isn't independent enough. Useful prioritization signal only. |
| `mining/silver-candidates.tsv` | **Silver file policy** (canonical wording, used everywhere): `silver-candidates.tsv` is absent before `cmd/enrichcorpus` runs. The verifier accepts either absent OR header-only as valid initial state. `cmd/aggregatecorpus` **never writes it** — only `cmd/enrichcorpus` does, after parser_choice agrees with external analyzer (omorfi/estnltk). |
| `qa-report.json` | Per-run snapshot — parser version, source row counts, token counts, unique surface count, unresolved rate, ambiguous-surface count, license mix, top unresolved/disagreement examples. |

### Family C — Provenance sidecars

| File | Purpose |
|---|---|
| `manifest.tsv` | Per-source bytes / lines / sentences / tokens / license / fetch URL / fetched_utc / sha256. |
| `documents.tsv` | Per-document_id mapping — source, doc_id, title, author, raw_path, extracted_path. Used by sentences/poems to resolve `document_id`. |
| `sentence_occurrences.tsv` | Full provenance — every (sentence_id, source, document_id, sentence_ix, quality_flags) tuple. Lets sentences.tsv stay text-only and dedup-clean while preserving "where exactly did this sentence come from?". |
| `build_metadata.json` | Pipeline version, parser version, FST table version, dict fingerprint, run start/end, host, cli args. |

### Family D — Promotion gates and error tracking

| File | Purpose |
|---|---|
| `qa-gate.json` | Machine-readable hard/soft gate results from `cmd/corpusverify`. Hard failures cause nonzero exit; soft warnings logged. |
| `promotion-state.json` | Tracks which profile (smoke/pilot/full) last passed verification per language. Updated by `cmd/corpuspromote`. |
| `errors/error-history.jsonl` | Append-only error log. Each entry: `{utc, profile, stage, error_class, message, detail, file_refs}`. Never overwritten. |
| `errors/repaired.jsonl` | Append-only repair log. Each entry: `{utc, fix_id, error_class, original_error_utc, fix_description, git_head_sha?}`. `fix_id` is mandatory; `git_head_sha` is optional (only present if the fix landed as a local commit — which is rare in this no-PR workflow). Lets the pipeline know "this error was already addressed, watch for regression." |

### Plus per-source files

- `localdata/{fi,et}-corpus/<source>/raw/` — verbatim downloaded files (with `.sha256` sidecars)
- For **prose** sources: `localdata/{fi,et}-corpus/<source>/text.txt` — extracted plain text (one paragraph per line, empty lines = doc boundaries)
- For **poetry** sources: `localdata/{fi,et}-corpus/<source>/poems.jsonl` — one JSON object per poem, line breaks preserved as escaped `\n` (not paragraph-normalized)
- `localdata/{fi,et}-corpus/<source>/manifest.json` — per-source metadata (slug, kind=prose|poetry, license, format, format_version)
- `localdata/{fi,et}-corpus/<source>/documents.jsonl` — per-document metadata (id, title, author, raw_path, byte offsets)

### Plus bootstrap tarball

- `finnestdb-bootstrap-{code,fi,et}.tgz` — 3-way split tarball (see Stage 4)

## Pipeline — seven sequential stages

### Stage 1 — `cmd/fetchcorpus`

Downloads all sources to `localdata/{fi,et}-corpus/<source>/raw/`. Writes
`<source>/manifest.json` with metadata. Idempotent (checks SHA256, skips
if matches). Single binary with `-lang fi|et`, `-source <name>`, `-force`,
`-dry-run`.

**Duplicates** the ~30-line `downloadFile()` pattern from main repo's
`cmd/fetchfrequency/main.go` into `corpus_pipeline/internal/fetcher/`
so we don't modify main repo's tracked files. (Yes, this is intentional
DRY-violation in service of the cleanliness rule — main repo's
`cmd/fetchfrequency` is left untouched.)

### Stage 2 — `cmd/extractcorpus`

For each `<source>/raw/`, dispatches to a format-specific extractor.
**Output depends on whether the source is prose or poetry:**

- **Prose sources** (`kind: prose` in manifest.json): writes
  `<source>/text.txt` (one paragraph per line, empty lines mark
  document boundaries) + `<source>/documents.jsonl`.
- **Poetry sources** (`kind: poetry` — SKVR, runosto, ERAB, **eeva-poetry** (only the curated poetry subset of EEVA),
  hand-curated Gutenberg poetry IDs): writes `<source>/poems.jsonl`
  with one JSON object per poem, line breaks **escaped** (`\n` literal,
  preserved through JSON encoding). NO paragraph-normalized text.txt
  for poetry sources — line breaks are part of the data.

Phase 1 of aggregator only reads `text.txt` from prose sources;
phase 1 reads `poems.jsonl` from poetry sources and routes them
straight to the poems buffer.

Format extractors needed:

| Format | Sources | Approach |
|---|---|---|
| Plain text gzip (`.txt.gz`) | OPUS OpenSubtitles, Europarl, Finlex, DocHPLT, … | `gzip.NewReader` → write |
| Yle VRT zip | Yle News s-vrt | `archive/zip` → parse VRT (TAB-sep, blank line = sentence, `<text>` = doc) → reconstruct paragraphs |
| Leipzig tar+TSV | Leipzig newscrawl/news/wikipedia | `archive/tar` → `*-sentences.txt` → strip `id\t` |
| MediaWiki XML bz2 | Wikipedia full dumps | Shell out to wikiextractor (Python); document setup-local pulls it via pip |
| Parliament CSV | Eduskunta speeches | `encoding/csv` → speech column |
| EPUB | User's local EPUBs | Shell out to `pandoc -f epub -t plain`; falls back to inline unzip+XHTML if pandoc absent |
| SKVR XML | SKVR folk poems | XML parse → poem records (kind=poetry → routed to poems.tsv in Stage 3) |
| HTML scrape | runosto.net (FI poetry), infopankki, selkouutiset, **Riigikogu stenograms (ET parl)**, **eeva-prose / eeva-poetry (ET old literature, split by document type)** | Polite scrape (~1.5s delay), copying `cmd/scrapegutenberg` rate-limiter pattern. EEVA scraper inspects each document's metadata tags to route prose vs poetry into the right source folder. |
| HuggingFace dataset | **TalTechNLP/err-newsroom (ET news)** | Pull via HuggingFace Hub Python lib OR direct parquet download. Parse parquet/jsonl, extract `heading + lead-in + text` columns. |
| ERAB export | **ERAB regilaulud (ET poetry)** | TXT/XML export interface — parse song records (these are poetry, route to poems.tsv via `kind: poetry` in manifest.json) |
| Markdown (LingQ parallel) | **lingq-parallel (ET)** | Parse `.md`, recognize ENG/EST blocks, extract Estonian-only sentences, drop English equivalents. New extractor `extract_md.go`. |

Each extractor emits **quality flags** per paragraph/sentence: `has_url`,
`has_email`, `mostly_digits`, `very_short`, `very_long`, `non_target_lang_chars`.
These propagate into `sentences.tsv` and let the QA report detect
contamination.

Dispatch is per-source — the source's `manifest.json` declares `format`.

### Stage 3 — `cmd/aggregatecorpus`

(Naming note: "phases" inside a single aggregate run, vs. the separate
"v2 enrichment pass" run via `cmd/enrichcorpus` later — see Stage 5.)

**Four-phase architecture (correctness fix from review)**:

**Phase 1: Surface counting + sentence dedup (parserffi-tokenized, SQLite-backed)**

Walks every `<source>/text.txt`, calls
`internal/parserffi.AnalyzeText(langUpper, paragraph)` directly — **NOT
`parsecore.Analyze`**. parsecore does dictionary lookup + gloss
enrichment + has `MaxTextChars = 300_000`; that's wasteful for phase-1
counting and would chunk paragraphs unnecessarily. parserffi
(internal/parserffi/bindings.go:35) is the raw Rust-FFI
tokenizer/sentence-splitter without enrichment — exactly what phase 1
needs.

**Sentence text reconstruction note**: `parserffi.Sentence` returns
**tokens**, not the original sentence text. To produce
`tmp_sentences.text` we must reconstruct the sentence string from the
token list. Copy the reconstruction logic from
`internal/parsecore.toParsedSentences` (which already does this
correctly for the runtime parser path — handles whitespace, joins
tokens correctly across punctuation). Hash the reconstructed text for
dedup; same hash function as parsecore uses for sentence keys, so the
sentence bank stays consistent with what the app emits.

For each sentence and token it returns:

- Pipe (surface, source) tuples to a single SQLite-writer goroutine
  (see "Concurrency model" below).
- Compute sentence content-hash → writer does
  `INSERT OR IGNORE INTO tmp_sentences(hash, text, lang)` plus
  unconditional `INSERT INTO tmp_sentence_occurrences(sentence_hash,
  source, document_id, sentence_ix, quality_flags)`.
- For **poetry** sources, ingest `<source>/poems.jsonl` into `tmp_poems`
  (preserving full poems with line breaks). **Also tokenize the poem
  text via `parserffi.AnalyzeText`** to feed surface counts — we don't
  ignore the words. The crucial difference: poem-derived counts go
  into a *separate column* (`surface_count_poetry`) rather than mixing
  into the prose default. This honors goal #2 ("every unique surface
  form across all corpus") AND avoids letting an archaic refrain
  repeated 200 times in folk songs outrank a common modern prose word.

  Prose tokenization reads `<source>/text.txt` and counts go into
  `surface_count_prose`. Aggregator maintains both columns per surface.

**SQLite scratch DB**: aggregation runs against
`localdata/{lang}-corpus/_derived/_scratch.db` so 50-80M sentences and
~5M unique surfaces don't have to fit in RAM. The scratch DB is
deleted after phase 4 writers complete (or kept with `-keep-scratch`
for debugging). It is NOT a canonical artifact — TSVs are.

Output of phase 1: SQLite tables `tmp_surface_counts`,
`tmp_sentences`, `tmp_sentence_occurrences`, `tmp_poems`.

**Phase 2: Enrich unique surfaces (with cache)**

For each unique surface in `tmp_surface_counts`:

- Check `_derived/cache/surface-analyses.tsv` keyed by
  `(surface, lang, parser_version, fst_tables_sha, dict_fingerprint)`.
  Cache hit → reuse rows (no parser call). Cache miss → enrich.
- **No new lemmatizer API needed.** Existing
  `pkg/lemmatizer-fi-et.Lemmatizer.Lemmatize(lang, word) []Analysis`
  (verified at lemmatizer.go:114) already returns every loaded analysis
  without picker logic. Aggregator uses this directly.
- Enrichment per surface (using `langUpper` = "FI" or "ET"):
  - `parsecore.Analyze(db, langUpper, surface, "custom")` → record the
    parser's chosen (lemma, pos, feats), tag `is_parser_choice=1`
  - `Lemmatizer.Lemmatize(langUpper, surface)` → all FST analyses
  - **Collapse** rows where (lemma, pos, feats) match across sources
    into a single row with `analysis_sources` = `;`-joined list. So
    when parser_choice matches FST top hit, you get one row with
    `analysis_sources="parser_choice;fst"`, not two rows. Avoids the
    "same analysis duplicated" smell the review flagged.
- Pick one example **key** for the surface (NOT a final ID — those
  don't exist yet):
  - If the surface has prose occurrences → store
    `example_sentence_hash` = the content-hash of the first prose
    sentence containing the surface (resolves to `sentences.tsv.id`
    in phase 4).
  - Else (poetry-only surface) → store
    `example_poem_key = (source, document_id)` of the first poem
    containing the surface (resolves to `poems.tsv.id` in phase 4).
  - Phase 4 walks the wordlist after final ID assignment and converts
    each key into `example_ref_type` / `example_ref_id` / `example_text`.
    Avoids dangling references AND respects deterministic-ID ordering.

Cache is invalidated by bumping `parser_version`, `fst_tables_sha`
(SHA256 of the FST table files), or `dict_fingerprint` (hash of
relevant `dict_metadata` rows + table row counts in `finnestdb.db`).
All three are recorded in `build_metadata.json`. Cache lives under
`_derived/cache/` and ships in bootstrap tarballs.

Output of phase 2: wordlist rows (collapsed per (surface, lemma, pos, feats)).

**Phase 3: Mining outputs (poetry-aware)**

Same phase-2 loop also emits:

- `mining/unresolved.tsv` — surfaces with no FST hit AND no dict hit,
  **prose-first sort**: `surface_count_prose` desc, then
  `surface_count_total` desc. **Includes register breakdown columns**
  (`surface_count_prose`, `surface_count_poetry`) — a surface
  unresolved in poetry but rare in prose may be archaic/dialectal,
  while one unresolved in prose is a modern dictionary gap. The
  distinction matters for parser improvement priorities.
- `mining/poetry-unresolved.tsv` — subset of unresolved surfaces meeting
  the poetry-dominant threshold (`surface_count_poetry >= 10 AND
  surface_count_poetry >= 5 * max(surface_count_prose, 1)`).
  Curated separately so parser-improvement
  workflows targeting modern Finnish/Estonian don't get noise from
  Kalevala archaisms or regilaul formulas (and vice versa — folk-poetry
  research can target this file specifically).
- `mining/high-frequency-ambiguous.tsv` — surfaces with ≥2 FST
  analyses, sorted by `surface_count_prose` desc (prose-first because
  ambiguity in modern usage is the parser-improvement priority).
- `mining/parser-disagreements.tsv` — surfaces where `basic` and
  `custom` modes produce different chosen analyses. Sort by
  `surface_count_prose` desc.
- `mining/internal-consensus-candidates.tsv` — emit row when `basic`
  agrees with `custom` agrees with FST top hit. **NOT silver** —
  basic and custom share dictionary plumbing, so they're not
  independent. True silver requires external-analyzer agreement
  (omorfi/estnltk), which only `cmd/enrichcorpus` adds.

**Phase 4: Writers (deterministic IDs)**

Concurrent phase-1 workers produce non-deterministic sentence-insertion
order. Phase 4 fixes this by **assigning final `sentences.tsv.id`
values deterministically** using a stable sort key from the
`tmp_sentence_occurrences` table:

```
ORDER BY MIN(source_order),                  -- alphabetical source slug → stable
         MIN(document_id),                    -- stable per source
         MIN(sentence_ix),                    -- stable per document
         sentence_hash                        -- final tie-breaker
```

So `sentences.tsv.id=1` is always the sentence whose first occurrence
(across all sources) sorts smallest by `(source_order, document_id,
sentence_ix, hash)`. Re-running aggregation on identical inputs
produces identical sentence IDs. (Hard gate verifies this.)

Writers — **strict ordering**, because example keys must resolve to
final IDs:

1. `sentences.tsv` — sorted by deterministic id asc (assigns final IDs).
2. `poems.tsv` — sorted by source, title (assigns final poem IDs).
3. `documents.tsv`, `manifest.tsv` — provenance sidecars.
4. `sentence_occurrences.tsv` — sorted by sentence_id asc, then source asc.
5. **Resolve example keys** — walk wordlist and mining tables, look up
   each surface's `example_sentence_hash` → final `sentences.tsv.id`
   (or `example_poem_key` → `poems.tsv.id`), populate
   `example_ref_type` / `example_ref_id` / `example_text`.
6. `wordlist.tsv` — sorted by `surface_count_prose` desc, then
   `surface_count_total` desc, then surface asc, then `analysis_rank`
   asc.
7. All 5 mining TSVs.
8. `build_metadata.json`, `qa-report.json` — last so they reflect final
   row counts.

**Concurrency model — single SQLite writer**

SQLite + concurrent goroutines requires care. The model:

- **Phase 1**: N parser-worker goroutines (one per CPU core) each read
  one source's text.txt sequentially, call `parserffi.AnalyzeText`,
  and stream `(surface, lang, source, sentence_hash, text, document_id,
  sentence_ix, quality_flags)` rows over a buffered channel.
- **One** SQLite-writer goroutine drains the channel and does batched
  inserts inside a single `BEGIN ... COMMIT` per ~10K rows. WAL mode
  enabled. No `sync.Map` — SQLite is the single source of truth for
  cross-source aggregation in phase 1.
- **Phase 2**: M enrichment-worker goroutines (one per CPU core) pull
  unique surfaces from `tmp_surface_counts` (read-only) and call
  `parsecore.Analyze` + `lemmatizer.Lemmatize` + cache lookups, write
  results back via the same single-writer channel.

This is simpler and safer than mixing `sync.Map` with SQLite. One
writer, many readers/parsers.

Writer: `encoding/csv` with `w.Comma = '\t'`, copying the pattern from
[`cmd/corpusmine/main.go`](cmd/corpusmine/main.go) lines 262-299. The
csv package handles tab-in-content escaping correctly (rare but possible),
which is the safety belt for "TSV is fine as long as you use the library."

### Stage 4 — `make bootstrap-tarball` (split into 3)

```sh
make bootstrap-tarball
# produces:
#   finnestdb-bootstrap-code.tgz   ~2 GB    (finnestdb.db + small derived files)
#   finnestdb-bootstrap-fi.tgz     ~25-40 GB (localdata/fi-corpus/)
#   finnestdb-bootstrap-et.tgz     ~25-40 GB (localdata/et-corpus/)
```

3 tarballs, not 1 — total ~50-80 GB compressed is impractical as a
single file. The user (or teammate) can pull just one if they only
need one language.

The split is documented in `corpus_pipeline/docs/CORPUS_PIPELINE.md`
(local doc, NOT main repo's `docs/ARTIFACT_POLICY.md` — that file
stays untouched per cleanliness rule).

### Stage 5 — `cmd/enrichcorpus` (built in v1, run on demand later)

**Built now, not run during the initial pipeline.** Sits ready for the
user to run whenever they want richer FEATS:

```sh
make enrich-corpus-fi    # ~12-24 hrs, runs omorfi over all unique surfaces
make enrich-corpus-et    # ~12-24 hrs, runs estnltk
make enrich-corpus       # both
```

**Persistent batch adapter, not per-surface shell-out.** Per-surface
shell-out is infeasible at 5M surfaces (estnltk startup ~1s × 5M = ~58
days). Instead:

- omorfi: long-lived `omorfi-disamb-cmdline` subprocess; aggregator
  pipes surfaces on stdin (one per line), reads analyses on stdout.
- estnltk: long-lived Python subprocess running an estnltk-loop script
  (added under `scripts/estnltk-batch.py`); aggregator speaks JSON-line
  protocol over stdin/stdout.

Reads `_derived/wordlist.tsv`, queries the analyzer on each unique
surface, emits `_derived/wordlist-enriched.tsv` with extra columns:
`omorfi_lemma` (or `estnltk_lemma`), `omorfi_pos`, `omorfi_feats`,
`external_analysis_count`.

**Adds true silver labels**: surfaces where parser_choice agrees with
external analyzer (omorfi or estnltk) get appended to
`mining/silver-candidates.tsv` (a separate file from
`internal-consensus-candidates.tsv`). This is the only path to a real
silver tier — anti-circular discipline.

Resumable: writes its own cache at `_derived/cache/external-analyses.tsv`
keyed by `(surface, lang, analyzer_version)`. If it dies halfway, re-run
picks up where it left off.

Independent of the main pipeline. Doesn't touch sentences.tsv or
poems.tsv. Doesn't gate anything else. Runs on user's schedule.

### Stage 6 — `cmd/corpusverify` (promotion gate)

Runs after every aggregate. Reads the derived TSVs, applies hard and
soft gates, writes `_derived/qa-gate.json`, exits nonzero on hard
failure.

```sh
make corpus-verify-fi PROFILE=smoke    # or pilot, full
```

**Hard gates** (fail = stop):
- All required files exist (wordlist.tsv, sentences.tsv, poems.tsv,
  documents.tsv, sentence_occurrences.tsv, manifest.tsv,
  build_metadata.json, qa-report.json, all 5 initial mining TSVs:
  unresolved.tsv, poetry-unresolved.tsv, parser-disagreements.tsv,
  high-frequency-ambiguous.tsv, internal-consensus-candidates.tsv.
  silver-candidates.tsv must be absent or header-only —
  `cmd/aggregatecorpus` never writes it; only `cmd/enrichcorpus` does
  (Silver file policy in Family B above).
- All TSVs parse with `encoding/csv`, `Comma='\t'`.
- All text fields are valid UTF-8.
- Every `wordlist.example_ref_id` resolves to either `sentences.id`
  (when `example_ref_type=sentence`) or `poems.id` (when
  `example_ref_type=poem`).
- Every `sentences.id` referenced by `sentence_occurrences` exists.
- Every `sentence_occurrences.document_id` resolves to `documents.document_id`.
- No duplicate primary keys: sentences.id, documents.document_id,
  wordlist `(surface, lang, lemma, pos, feats)`.
- Every row for the same `(surface, lang)` repeats the same trio:
  `surface_count_prose`, `surface_count_poetry`, `surface_count_total`.
- `sentences.tsv` has no raw newlines inside text.
- `poems.tsv` preserves line breaks as escaped `\n`.
- **FST tables present** for both languages — missing FST tables is
  a HARD smoke failure (the entire ambiguity-enumeration story
  collapses without them). Verifier checks
  `localdata/lemmatizer-fi-et/tables/fi_min.json` and
  `localdata/lemmatizer-fi-et/tables/et_min.json` (the actual paths used
  by this project — see `pkg/lemmatizer-fi-et` runtime), or
  `LEMMATIZER_TABLES_DIR` env var override if set. Non-empty file size
  required (the smoke fixtures are 12-form mini-tables; full tables are
  much larger).
- **Rust parser shared library present** — `parser/target/release/libparser*`
  must exist. `internal/parserffi` links against this at runtime; if
  missing, every `parserffi.AnalyzeText` call fails. The verifier's
  preflight runs `make parser` from main repo if the library is
  missing, then re-checks. If `make parser` itself fails, that's a
  HARD smoke failure with a clear error_class
  (`preflight.rust_parser.build_failed`).
- Fixture probes pass: FI `tulen` returns ≥2 analyses (verb tulla / noun tuli),
  FI `talossa` resolves to `talo/NOUN/Case=Ine`, FI `kuusen` resolves to
  `kuusi/NOUN/Case=Gen`, ET `joon` returns ≥2 analyses (verb jooma / noun joon),
  ET `kuus` resolves to numeral, ET `aitan` returns ≥2 analyses.
- Re-aggregate on identical input is byte-identical (or hash-identical
  modulo intentionally varying metadata like timestamps).
- Peak RSS during aggregation under profile cap.

**Soft gates** (warn, don't fail):
- `unresolved_surface_rate` above per-profile threshold.
- `very_short` / `very_long` sentence rate above thresholds.
- URL/email/digit-heavy contamination rate above thresholds.
- Top unresolved contains obvious extraction junk (`<div`, `</`,
  `&nbsp;`, `http`, `www.`).
- Single source contributes >X% of total tokens (suggests an extractor
  bug or runaway dump).
- Language-ID contamination above threshold.

### Stage 7 — `cmd/corpuspromote` (3-tier promotion ladder)

Simplified from the reviewer's 5-level ladder. Three profiles:

| Profile | Data slice | Wall clock |
|---|---|---|
| `smoke` | Hand-authored fixture (`testdata/corpus-fixtures/{fi,et}/`) + 1 tiny real source (opus-tatoeba ~10 MB) | ~2 min |
| `pilot` | ~500 MB per language (largest sources truncated to fit) | ~30-60 min |
| `full` | Everything (~10 GB per language) | ~10-12 hrs |

```sh
make corpus-promote-fi    # runs smoke → if pass → pilot → if pass → full
make corpus-promote-et
make corpus-promote       # both
```

Stops on the first hard-gate failure. Records state in
`_derived/promotion-state.json` so you can resume after fixing.

### Error report system (your specific request)

When any hard gate fails, the runner appends to:

- `_derived/errors/error-history.jsonl` — one JSON object per error,
  appended forever:
  ```json
  {"utc": "2026-05-08T08:31:00Z", "profile": "pilot", "stage": "verify",
   "error_class": "wordlist.example_sentence_id.orphan",
   "message": "42 orphan example_sentence_id references",
   "detail": ["12345", "67890", "..."], "file_refs": ["wordlist.tsv:line 1234"]}
  ```
- `_derived/errors/error_report.txt` — human-readable summary of the
  most recent failure + suggested next steps. Overwritten each run
  (history is in the JSONL).

When a fix lands, append to `_derived/errors/repaired.jsonl`:

```json
{"utc": "2026-05-08T11:42:00Z",
 "fix_id": "fix-2026-05-08-001",
 "error_class": "wordlist.example_sentence_id.orphan",
 "original_error_utc": "2026-05-08T08:31:00Z",
 "fix_description": "aggregator now writes example_sentence_id only after sentence is committed",
 "git_head_sha": "abc123def"}
```

`fix_id` is mandatory (durable identifier — works even with no commits).
`git_head_sha` is **optional** — included if the fix was a local commit
on some branch, omitted if the fix was just a file edit (which is the
common case for this no-PR workflow).

Next runs check both: if a known error_class re-appears within ~7
days of a `repaired.jsonl` entry, the verifier flags it as **regression**
in the report ("you fixed this on YYYY-MM-DD, it's back"). Catches
silent reverts and pipeline drift.

Both files are append-only. Never overwritten. Ship in bootstrap
tarball.

## Incremental updates — adding sources after the initial run

The pipeline is designed to be re-run safely. When you add new sources
(EPUBs, a new scrape, an updated dump), the workflow is:

### EPUB workflow — three named make commands

The `epub` source is **folder-driven, not URL-driven**: its
`manifest.json` declares `format: epub` with no download URL — so
`cmd/fetchcorpus` skips it. Three Makefile targets handle the rest:

#### `make epub-to-text` — just extract EPUBs to plain text

```sh
make epub-to-text          # both languages
make epub-to-text-fi
make epub-to-text-et
```

Walks `localdata/{lang}-corpus/epub/raw/*.epub`, shells out to
`pandoc -f epub -t plain` per file, writes:

- `localdata/{lang}-corpus/epub/text.txt` — concatenated, one paragraph
  per line, empty lines = book boundaries (used by aggregate)
- `localdata/{lang}-corpus/epub/per-book/<slug>.txt` — one file per
  book, plain text, full content (handy for reading separately)
- `localdata/{lang}-corpus/epub/documents.jsonl` — per-book metadata

Idempotent (skips books whose mtime + size match the manifest).
Standalone — useful if you just want plain-text versions of your EPUBs
for some other reason. Doesn't touch wordlist/sentences/poems.

#### `make add-epub` — fold EPUBs into the giant corpus

```sh
make add-epub              # both languages
make add-epub-fi
make add-epub-et
```

Internally runs `make epub-to-text` first (as a Make dependency), then
`cmd/aggregatecorpus` to refresh wordlist.tsv / sentences.tsv with
EPUB content folded in. Phase 2 cache hits on existing ~5M surfaces;
only the ~50K new surfaces from your EPUBs get parser-enriched. Wall
clock: ~5-15 minutes (vs. ~6 hours from scratch).

This is the one to run when you want EPUBs to "be part of the giant
list."

#### `make epub-deck` — strip an EPUB into a per-book word list

```sh
make epub-deck                                # all books in epub/raw/, both langs
make epub-deck-fi
make epub-deck-et
make epub-deck EPUB=tuntematon-sotilas.epub   # one specific book
```

For each EPUB, produces a standalone per-book word list at:

```
localdata/{lang}-corpus/epub/decks/<book-slug>.tsv
```

Same column shape as the main wordlist.tsv (surface, surface_count_prose, surface_count_poetry, surface_count_total,
lemma, pos, feats, …) but counts are scoped to that one book. Useful
as a flashcard-deck source — you can manually import a single book's
vocab into Anki / the app, or drop it somewhere else.

This is independent of `add-epub`. You can run `epub-deck` without
ever running `add-epub` (e.g. you want a deck for a book but don't
want it polluting the giant aggregated wordlist).

### Generalizing to any new source

Any future "I want to add this" workflow uses the same shape:

```
localdata/{fi,et}-corpus/<my-source>/
├── raw/                 # files you put here (or fetcher downloads here)
└── manifest.json        # declares slug, kind=prose|poetry, license, format
```

Then `make extract-corpus aggregate-corpus`. The pipeline discovers
sources by walking `localdata/{fi,et}-corpus/*/manifest.json`, so a
new directory just shows up.

### Surface enrichment cache

`_derived/cache/surface-analyses.tsv` — maps `(surface, lang,
parser_version, fst_tables_sha, dict_fingerprint)` to its enriched rows. Phase 2 reads
this on every run; cache hits skip the parser call entirely. New
sources only pay the parser cost for surfaces that are genuinely new
to the corpus.

Cache invalidation:
- Bumping `parser_version` in code → affected entries stale, phase 2 re-enriches.
- Changing FST tables (new `fst_tables_sha`) → affected entries stale.
- Changing the dictionary in `finnestdb.db` (new `dict_fingerprint`,
  computed from `dict_metadata` table + relevant table row counts hashed) → affected entries stale.
- Manual `make corpus-cache-clear` → wipes the cache entirely.

Cache size: ~500 MB-1 GB (5M surfaces × ~100 bytes/row). Ships in the
bootstrap tarball so a teammate gets fast incremental runs out of the
box.

## Directory layout (after reorg)

```
localdata/
├── fi-corpus/
│   ├── manual/                     # 27 hand-pasted FI .txt articles
│   │   ├── raw/                    # original .txt + HTML scrapes
│   │   ├── text.txt                # concatenated for aggregator
│   │   ├── documents.jsonl         # per-doc metadata
│   │   └── manifest.json
│   ├── epub/                       # 230 FI EPUBs from main repo's manual/
│   │   ├── raw/                    # *.epub files
│   │   ├── per-book/<slug>.txt     # pandoc output per book
│   │   ├── text.txt                # concatenated
│   │   ├── decks/<slug>.tsv        # produced by epub-deck on demand
│   │   ├── documents.jsonl
│   │   └── manifest.json
│   ├── gutenberg/                  # FI Gutenberg scrape
│   ├── opus-opensubtitles/
│   ├── yle-news-s-vrt/
│   ├── wikipedia/
│   ├── leipzig-newscrawl/
│   ├── leipzig-news/
│   ├── leipzig-wikipedia/
│   ├── opus-europarl/
│   ├── opus-finlex/
│   ├── parliament-eduskunta/
│   ├── parsebank/
│   ├── opus-wikimatrix/
│   ├── opus-eubookshop/
│   ├── opus-ecb/
│   ├── opus-emea/
│   ├── skvr/                       # poetry — kind=poetry
│   ├── runosto/                    # poetry — kind=poetry
│   ├── frequency-words/
│   ├── selkouutiset/
│   ├── opus-bible/
│   ├── opus-ted2020/
│   ├── opus-tatoeba/
│   ├── infopankki/
│   ├── opus-cc100/
│   ├── opus-jrc-acquis/
│   └── _derived/
│       ├── wordlist.tsv
│       ├── sentences.tsv
│       ├── poems.tsv
│       ├── documents.tsv
│       ├── manifest.tsv
│       ├── build_metadata.json
│       ├── qa-report.json
│       └── mining/                       # initial aggregate writes 5 files
│           ├── unresolved.tsv
│           ├── poetry-unresolved.tsv
│           ├── parser-disagreements.tsv
│           ├── high-frequency-ambiguous.tsv
│           ├── internal-consensus-candidates.tsv
│           └── silver-candidates.tsv     # ONLY populated by cmd/enrichcorpus
└── et-corpus/
    ├── opus-dochplt/
    ├── opus-nllb/
    ├── opus-ccmatrix/
    ├── opus-hplt/
    ├── opus-multihplt/
    ├── opus-cc100/
    ├── opus-opensubtitles/
    ├── opus-paracrawl/
    ├── opus-multiparacrawl/
    ├── leipzig-newscrawl/
    ├── leipzig-news/
    ├── leipzig-wikipedia/
    ├── wikipedia/
    ├── gutenberg/
    ├── frequency-words/
    ├── opus-europarl/
    ├── opus-jrc-acquis/
    ├── opus-emea/
    ├── opus-bible/
    ├── opus-tatoeba/
    ├── hf-err-newsroom/                       # HuggingFace ERR news 2016-2022
    ├── riigikogu-stenograms/                  # ET parliamentary
    ├── erab-regilaulud/                       # ET folk poetry — kind=poetry
    ├── eeva-prose/                            # EEVA older literature, prose
    ├── eeva-poetry/                           # EEVA older literature, poetry — kind=poetry
    ├── lingq-parallel/                        # 2 LingQ ENG_EST parallel-text .md files
    ├── epub/                                  # folder-driven, populated when user drops EPUBs
    └── _derived/
        ├── wordlist.tsv
        ├── sentences.tsv
        ├── poems.tsv
        ├── documents.tsv
        ├── manifest.tsv
        ├── build_metadata.json
        ├── qa-report.json
        └── mining/...
```

## TSV schemas

### wordlist.tsv

```
surface  surface_count_prose  surface_count_poetry  surface_count_total  doc_count_prose  doc_count_poetry  source_counts_json                                              lang  lemma  pos    case  number  mood  tense  person  voice  verbform  feats                                              analysis_sources    analysis_rank  is_parser_choice  parser_version  example_ref_type  example_ref_id  example_text
tulen    8421                 12                    8433                 193              5                 {"opus-opensubs":7000,"gutenberg":1421,"skvr":12}              fi    tulla  VERB         Sing    Ind   Pres   1                        Mood=Ind|Number=Sing|Person=1|Tense=Pres|VerbForm=Fin  parser_choice;fst   1              1                 2026.05.07k     sentence          12345           Tulen kotiin myöhään.
tulen    8421                 12                    8433                 193              5                 {"opus-opensubs":7000,"gutenberg":1421,"skvr":12}              fi    tuli   NOUN   Gen   Sing                                          Case=Gen|Number=Sing                               fst                 2              0                 2026.05.07k     sentence          12345           Tulen kotiin myöhään.
mieleni  18                   2403                  2421                 11               387               {"skvr":2403,"manual":18}                                       fi    mieli  NOUN         Sing                                            Case=Nom|Number=Sing|Number[psor]=Sing|Person[psor]=1  parser_choice;fst   1              1                 2026.05.07k     sentence          54321           Mieleni teki tehdä jotain.
arkanen  0                    87                    87                   0                12                {"skvr":87}                                                     fi    arka   ADJ          Sing                                            Case=Nom|Number=Sing|Number[psor]=Sing|Person[psor]=1  fst                 1              0                 2026.05.07k     poem              448             Arkanen sydämeni…
```

- One row per **distinct** `(surface, lemma, pos, feats)`. Collapses
  duplicate analyses where parser_choice and FST agree.
- **Three count columns**:
  - `surface_count_prose` — occurrences from `kind: prose` sources
    (news, subtitles, parliament, encyclopedia, web, EPUBs, etc.)
  - `surface_count_poetry` — occurrences from `kind: poetry` sources
    (SKVR, runosto, ERAB, eeva-poetry)
  - `surface_count_total` = sum of both
- All three repeat across rows for the same surface. **Don't sum
  across rows — that double-counts.**
- `doc_count_prose` / `doc_count_poetry` = distinct doc counts per register.
- `source_counts_json` = per-source breakdown across both registers.
- **Default sort: `surface_count_prose` desc, then `surface_count_total`
  desc, then surface asc, then `analysis_rank` asc.** Poetry counts
  visible but not rank-dominant — a rare archaic refrain repeated 200
  times in folk songs won't outrank a common modern prose word. The
  `mieleni` example above (Kalevala-famous "mieleni minun tekevi" line)
  has high poetry count but low prose count — appears far down in
  the prose-default ranking, where it belongs for learners reading
  modern text.
- `analysis_sources` = `;`-joined list (`parser_choice`, `fst`, future:
  `omorfi`, `estnltk`).
- `analysis_rank` per-(surface) ordering; rank 1 = parser-preferred.
- `is_parser_choice` = 1 only on the parser-chosen row.
- `example_ref_type` ∈ {`sentence`, `poem`}: tells you which table to
  resolve `example_ref_id` against. Prose-first surfaces get
  `sentence`; poetry-only surfaces (where `surface_count_prose=0`)
  get `poem` pointing into `poems.tsv`. Avoids dangling references.
- `example_ref_id` is the primary key of the chosen table.
- `example_text` is the literal text excerpt (denormalized — saves the
  consumer a join, and survives if sentences/poems IDs ever shift).
- Empty cells = `""`, not `-`.

### sentences.tsv (text only, deduped)

```
id   lang  text
1    fi    Hän sanoi: "Älä mene sinne."
2    fi    Kokoomus voitti vaalit ylivoimaisesti.
3    fi    Talossa on viisi huonetta.
4    fi    (talo: 1234.fi)
```

- `id` is sequential, stable within a single run.
- One row per unique sentence text.
- `text` has `\n`/`\r` collapsed to space (single-line sentences).

### sentence_occurrences.tsv (full provenance)

```
sentence_id  source                document_id              sentence_ix  quality_flags
1            opus-opensubtitles    opus-opensubs:doc-00001  3
2            yle-news-s-vrt        yle-news:74-20223215     0
3            opus-opensubtitles    opus-opensubs:doc-00042  17
3            leipzig-newscrawl     leipzig:doc-12345        4
4            wikipedia             wikipedia:Helsinki       128          mostly_digits;has_url
```

- One row per (sentence_id × source × document × position) — many-to-one with sentences.tsv.
- Lets you grab adjacent sentences from the same document, debug
  extractor bugs, sample by source for QA.
- `quality_flags` is `;`-joined (`has_url`, `has_email`, `mostly_digits`,
  `very_short`, `very_long`, `non_target_lang_chars`); empty if clean.

### poems.tsv

```
id  lang  source       document_id     title          author       line_count  text
1   fi    skvr         skvr:I.1.1      Kalevala-XI    trad         128         Mieleni minun tekevi,\naivoni ajattelevi…
2   fi    runosto      runosto:1234    Aamu           Eino Leino   16          Aamun koitto kirkas…\n…
```

- `text` preserves line breaks as literal `\n` (`encoding/csv` handles this with quoting; downstream consumers `text.replace("\\n", "\n")`).
- `line_count` is non-blank line count.

### Mining TSVs

```
mining/unresolved.tsv (sorted by surface_count_prose desc, then surface_count_total desc):
surface       surface_count_prose  surface_count_poetry  surface_count_total  source_counts_json                  example_ref_type  example_ref_id  example_text
örkkimörkkö   42                   0                     42                   {"gutenberg":42}                     sentence          999888           Ja sitten örkkimörkkö tuli paikalle.

mining/poetry-unresolved.tsv (sorted by surface_count_poetry desc):
surface         surface_count_prose  surface_count_poetry  surface_count_total  source_counts_json     example_ref_type  example_ref_id  example_text
mielenmoinen    0                    412                   412                  {"skvr":412}            poem              7723             …mielenmoinen suru…

mining/parser-disagreements.tsv (sorted by surface_count_prose desc):
surface  surface_count_prose  surface_count_poetry  basic_lemma  basic_pos  custom_lemma  custom_pos  example_ref_type  example_ref_id
…

mining/high-frequency-ambiguous.tsv (sorted by surface_count_prose desc):
surface  surface_count_prose  surface_count_poetry  fst_analyses_count  top_two_lemmas       top_two_pos       example_ref_type  example_ref_id
tulen    8421                 12                    2                   tulla;tuli           VERB;NOUN          sentence          12345

mining/internal-consensus-candidates.tsv (sorted by surface_count_prose desc):
surface  surface_count_prose  surface_count_poetry  agreed_lemma  agreed_pos  agreed_feats                                          example_ref_type  example_ref_id  agreement_kind
talossa  3201                 8                     talo          NOUN        Case=Ine|Number=Sing                                  sentence          67890            basic+custom+fst

mining/silver-candidates.tsv  (only populated by cmd/enrichcorpus, absent/empty in v1 initial run):
surface  surface_count_prose  surface_count_poetry  agreed_lemma  agreed_pos  agreed_feats                                          example_ref_type  example_ref_id  external_analyzer
talossa  3201                 8                     talo          NOUN        Case=Ine|Number=Sing                                  sentence          67890            omorfi
```

**Poetry-dominant threshold for `mining/poetry-unresolved.tsv`** —
hard rule, not vibes:

```
surface_count_poetry >= 10 AND surface_count_poetry >= 5 * max(surface_count_prose, 1)
```

So a surface qualifies as poetry-dominant if it has at least 10
poetry occurrences AND its poetry count is at least 5× its prose count
(treating prose=0 as prose=1 for the multiplier). Excludes noise from
1-2 incidental poetry occurrences while catching real archaisms /
formulas / dialectal forms.

### qa-report.json

```json
{
  "lang": "fi",
  "run_start_utc": "2026-05-08T03:14:15Z",
  "run_end_utc":   "2026-05-08T11:42:01Z",
  "parser_version": "2026.05.07k",
  "fst_tables_sha": "abc123def456…",
  "dict_fingerprint": "789ghi012jkl…",
  "totals": {
    "sources": 25,
    "documents": 1234567,
    "sentences_unique": 78901234,
    "sentences_total_occurrences": 142345678,
    "tokens_total": 412345678,
    "tokens_prose": 408000000,
    "tokens_poetry": 4345678,
    "unique_surfaces": 4892341,
    "unique_surfaces_prose": 4612000,
    "unique_surfaces_poetry": 612000,
    "poetry_only_surfaces": 280341,
    "prose_only_surfaces": 4280000,
    "unresolved_surfaces_total": 234567,
    "unresolved_rate_total": 0.048,
    "unresolved_surfaces_prose": 198000,
    "unresolved_rate_prose": 0.043,
    "unresolved_surfaces_poetry": 89000,
    "unresolved_rate_poetry": 0.145,
    "ambiguous_surfaces": 612345,
    "ambiguous_rate": 0.125,
    "poems": 87654
  },
  "license_mix": {
    "public_domain": 12,
    "cc_by": 8,
    "cc_by_sa": 3,
    "cc_by_nc_sa": 1,
    "tos": 1
  },
  "quality_flags_distribution": {
    "has_url": 12345,
    "mostly_digits": 6789,
    "very_short": 234567,
    …
  },
  "top_unresolved": [
    {"surface": "…", "count": 1234},
    …
  ],
  "top_disagreements": [
    {"surface": "tulen", "basic_lemma": "tuli", "custom_lemma": "tulla", "count": 8421},
    …
  ]
}
```

### manifest.tsv

```
source              kind     license             url                                                    bytes_raw   bytes_text   sentences   tokens     fetched_utc            sha256
opus-opensubtitles  prose    OpenSubtitles ToS   https://object.pouta.csc.fi/OPUS-OpenSubtitles/v2018/mono/fi.txt.gz  1124000000  1820000000  61234567   412345678  2026-05-08T03:14:15Z  abc123…
yle-news-s-vrt      prose    Yle Areena terms    https://www.kielipankki.fi/download/YLE/fi/2022-2024-s-vrt/ylenews-fi-2022-2024-s-vrt.zip  4800000000  6800000000  102345678  654321987  2026-05-08T03:42:18Z  def456…
skvr                poetry   public domain       https://www.skvr.fi/…                                  99000000    78000000    345678      4567890    2026-05-08T04:01:33Z  789abc…
```

### documents.tsv

```
document_id              source           title         author       raw_path                                                       extracted_offset
opus-opensubs:doc-00001  opus-opensubs    -             -            localdata/fi-corpus/opus-opensubtitles/raw/fi.txt.gz#L1-L128    0
yle-news:74-20223215     yle-news         Lehti.t.t…    Yle          localdata/fi-corpus/yle-news-s-vrt/raw/ylenews-fi-2022-2024-s-vrt.zip#article=74-20223215  142
skvr:I.1.1               skvr             Kalevala-XI   trad         localdata/fi-corpus/skvr/raw/skvr-i.xml#poem=I.1.1               0
```

## Tiering for parser improvement (anti-circularity)

| Tier | Source | Use | File |
|---|---|---|---|
| **Gold** | Human-labeled or trusted upstream (UD treebanks, hand-authored cases, parser-feedback fix-ups) | Gates parser changes. **Not produced by this pipeline.** | `testdata/parser-eval/{fi,et}/gold/` (committed) and `localdata/parser-eval/{fi,et}/gold/` (NC-licensed UD) |
| **Silver** | External-analyzer agreement: parser_choice agrees with omorfi (FI) or estnltk (ET) | Real silver — independent enough to support regression smoke tests and bootstrapping. | `_derived/mining/silver-candidates.tsv` (only populated by `cmd/enrichcorpus`) |
| **Internal consensus** | basic ⊕ custom ⊕ FST top hit all agree | Useful signal but not independent (basic and custom share dictionary plumbing). Treat as priority hints, not correctness claims. | `_derived/mining/internal-consensus-candidates.tsv` |
| **Bronze / mining** | Raw corpus signals (unresolved, parser-disagreements, high-frequency-ambiguous) | Prioritization, not correctness claims. | `_derived/mining/*.tsv` |

**Critical anti-circularity rule:** the v1 initial run populates
`internal-consensus-candidates.tsv`, NOT `silver-candidates.tsv`. The
basic and custom parsers share too much dictionary plumbing to count as
independent — internal consensus is a useful signal, not a silver label.
True silver only appears after `cmd/enrichcorpus` runs (omorfi or
estnltk = independent analyzer). Don't claim parser accuracy against
internal-consensus data.

## Source list — v1 hard commitment (verified, point-in-time sizes)

**This is the v1 commitment.** Every source listed below WILL be
fetched. Sources outside this list are out of scope for v1 (added in
a follow-up if/when the user requests them). Sizes were taken from the
OPUS API and Kielipankki HEAD probes during prior research turn — they
are point-in-time snapshots; the fetcher re-verifies via HEAD at run
time and updates `manifest.tsv` with actual byte counts.

### FI ~10 GB (25 sources)

| Source | Kind | Bytes (compressed) | Format |
|---|---|---:|---|
| Yle News 2022-2024 s-vrt | news | 4.8 GB | VRT zip |
| OPUS OpenSubtitles fi | conv | 1.07 GB | txt.gz |
| OPUS Wikipedia fi | encyc | 923 MB | txt.gz |
| Parliament Eduskunta | parl | 619 MB | csv |
| Leipzig newscrawl fi | news | ~800 MB | tar+tsv |
| Leipzig news fi | news | ~800 MB | tar+tsv |
| Leipzig wikipedia fi | encyc | ~700 MB | tar+tsv |
| Wikipedia fi dump | encyc | ~600 MB | xml.bz2 |
| Parsebank | web | 219 MB | txt.gz |
| OPUS WikiMatrix fi | encyc | 213 MB | txt.gz |
| OPUS EUbookshop fi | legal | 132 MB | txt.gz |
| SKVR | poetry | 99 MB | xml |
| OPUS Europarl fi | parl | 97 MB | txt.gz |
| OPUS Finlex fi | legal | 97 MB | txt.gz |
| OPUS JRC-Acquis fi | legal | ~80 MB | txt.gz |
| OPUS ECB fi | finance | 71 MB | txt.gz |
| FrequencyWords fi | freq | 37 MB | txt |
| Gutenberg fi | literary | ~30 MB | scrape (existing cmd) |
| infopankki | gov | ~30 MB | scrape |
| OPUS TED2020 fi | talks | ~25 MB | txt.gz |
| runosto.net | poetry | ~20 MB | scrape |
| OPUS EMEA fi | medical | 20 MB | txt.gz |
| OPUS Bible fi | religious | ~15 MB | txt.gz |
| OPUS Tatoeba fi | conv | ~10 MB | txt.gz |
| Selkouutiset | news (easy) | 9.8 MB | scrape |

### ET ~10 GB (25 sources, parity-closing additions in **bold**)

| Source | Kind | Bytes (compressed) | Format |
|---|---|---:|---|
| OPUS DocHPLT et | web | 1.88 GB | txt.gz |
| OPUS CC-100 et | web | 1.62 GB | txt.gz |
| OPUS NLLB et | mixed | 904 MB | txt.gz |
| OPUS CCMatrix et | web | 909 MB | txt.gz |
| OPUS HPLT et | web | 820 MB | txt.gz |
| OPUS MultiHPLT et | web | 700 MB | txt.gz |
| OPUS OpenSubtitles et | conv | 391 MB | txt.gz |
| Leipzig newscrawl et | news | ~400 MB | tar+tsv |
| Leipzig news et | news | ~400 MB | tar+tsv |
| OPUS ParaCrawl et | web | 360 MB | txt.gz |
| Leipzig wikipedia et | encyc | ~300 MB | tar+tsv |
| OPUS MultiParaCrawl et | web | 295 MB | txt.gz |
| Wikipedia et dump | encyc | 286 MB | xml.bz2 |
| OPUS Wikipedia et | encyc | ~150 MB | txt.gz |
| OPUS Europarl et | parl | ~80 MB | txt.gz |
| OPUS JRC-Acquis et | legal | ~70 MB | txt.gz |
| OPUS EMEA et | medical | ~18 MB | txt.gz |
| FrequencyWords et | freq | 13 MB | txt |
| OPUS Bible et | religious | ~12 MB | txt.gz |
| OPUS Tatoeba et | conv | ~8 MB | txt.gz |
| Gutenberg et | literary | ~5 MB | scrape (1 book today) |
| **HF TalTechNLP/err-newsroom** | **news (broadcaster)** | **173 MB** | **HuggingFace dataset (parquet/jsonl)** — 187,187 ERR articles 2016-2022 |
| **Riigikogu stenograms** | **parliamentary** | **~200-500 MB est** | **HTML scrape of sitting-by-sitting index, polite (~1.5s delay)** |
| **ERAB (Eesti regilaulude andmebaas)** | **poetry (folk)** | **~100-200 MB** | **TXT/XML export from web interface (CLARIN ACA NC — local-only OK per policy)** — 92K regilaul texts + 6K newer rhymed folk songs. v1 = public web export. v2 = contact for full corpus. |
| **eeva-prose** (EEVA prose subset) | **literary (older)** | **~50-100 MB est** | **HTML scrape, prose only** |
| **eeva-poetry** (EEVA poetry subset) | **poetry (literary)** | **~20-50 MB est** | **HTML scrape, curated poetry-tagged documents only — line breaks preserved, routed to poems.tsv** |

### ET parity status — closed vs. still open

After these additions, the previous gaps are mostly closed:

| Gap | FI has | ET v1 has (after these adds) | Status |
|---|---|---|---|
| Parliamentary | Eduskunta CSV (619 MB), Europarl ET (~80 MB) | **Riigikogu stenograms (~200-500 MB) + Europarl ET** | ✅ **Closed** |
| Public-broadcaster news | Yle Kielipankki s-vrt (4.8 GB) | **HF TalTechNLP/err-newsroom (173 MB) + Leipzig (~1.1 GB) + Wikipedia (~280 MB)** | ⚠️ **Partial** — total ~1.5 GB vs. FI's 4.8 GB; v2 could scrape uudised.err.ee / kultuur.err.ee / arhiiv.err.ee for more |
| Poetry | SKVR folk (99 MB), runosto.net literary | **ERAB regilaul (~100-200 MB) + eeva-poetry (~20-50 MB)** + eeva-prose for non-poetry old literature | ✅ **Closed** |

The v1 ET corpus now has solid register coverage. v2 gap-closing
narrows to: more ERR portals (currently scraping the HF static dump
only), full ERAB corpus via CLARIN academic contact, and modern
authored Estonian poetry (luuletus.ee / Luuleleid — license-messy,
needs case-by-case research).

## Critical files to create — ALL UNDER `corpus_pipeline/`

Every path below is **relative to `corpus_pipeline/`** unless
explicitly noted. Nothing tracked by git is modified.

| File | Action | Notes |
|---|---|---|
| `go.mod` | **create** | `module finnestdb/corpus_pipeline`, `go 1.21`, `replace finnestdb => ..`. The module path MUST start with `finnestdb/` so we can import `finnestdb/internal/...` (Go internal-visibility rule). |
| `go.sum` | **create** | Generated by `go mod tidy`. |
| `Makefile` | **create** | All targets: `fetch-corpus[-fi/-et]`, `extract-corpus[-fi/-et]`, `aggregate-corpus[-fi/-et]`, `bootstrap-tarball`, `epub-to-text[-fi/-et]`, `add-epub[-fi/-et]`, `epub-deck[-fi/-et]`, `enrich-corpus[-fi/-et]`, `corpus-verify[-fi/-et]`, `corpus-promote[-fi/-et]`, `corpus-cache-clear`. Operate via `cd corpus_pipeline && make <target>`. |
| `cmd/fetchcorpus/main.go` | **create** | Orchestrator binary, `-lang`, `-source`, `-force` flags |
| `cmd/fetchcorpus/sources_fi.go` | **create** | FI source registry (URL, kind, format, license) |
| `cmd/fetchcorpus/sources_et.go` | **create** | ET source registry |
| `cmd/extractcorpus/main.go` | **create** | Format dispatcher, emits prose `<source>/text.txt` OR poetry `<source>/poems.jsonl` (depending on source kind) plus `<source>/documents.jsonl` |
| `cmd/extractcorpus/extract_*.go` | **create** | One file per format: `_vrt`, `_gz`, `_leipzig`, `_wiki`, `_csv`, `_epub`, `_skvr`, `_html`, `_huggingface`, `_riigikogu`, `_erab`, `_eeva`, `_md` (for ET LingQ parallel-text) |
| `cmd/extractcorpus/quality.go` | **create** | Quality-flag detectors (URL, mostly-digits, etc) |
| `cmd/aggregatecorpus/main.go` | **create** | Four-phase aggregator emitting all derived outputs |
| `cmd/aggregatecorpus/phase1_count.go` | **create** | Sentence dedup + surface counting (parser-tokenized). Maintains separate `surface_count_prose` and `surface_count_poetry` columns. |
| `cmd/aggregatecorpus/phase2_enrich.go` | **create** | Enrich unique surfaces via parser + FST, with cache |
| `cmd/aggregatecorpus/phase3_mining.go` | **create** | Emit mining/*.tsv |
| `cmd/aggregatecorpus/writers.go` | **create** | TSV writers |
| `cmd/aggregatecorpus/cache.go` | **create** | Surface-analyses cache (key = `(surface, lang, parser_version, fst_tables_sha, dict_fingerprint)`) |
| `cmd/aggregatecorpus/scratch_db.go` | **create** | SQLite scratch DB schema + helpers |
| `cmd/enrichcorpus/main.go` | **create** | Persistent batch adapter to omorfi (FI) / estnltk (ET) |
| `cmd/enrichcorpus/omorfi_adapter.go` | **create** | Long-lived omorfi-disamb-cmdline subprocess, line-protocol I/O |
| `cmd/enrichcorpus/estnltk_adapter.go` | **create** | Long-lived Python subprocess running `scripts/estnltk-batch.py`, JSON-line protocol |
| `cmd/epubdeck/main.go` | **create** | Per-book word-list extractor |
| `cmd/corpusverify/main.go` | **create** | Promotion-gate verifier |
| `cmd/corpusverify/gates.go` | **create** | Hard/soft gate definitions + per-profile thresholds |
| `cmd/corpuspromote/main.go` | **create** | Runs smoke → pilot → full ladder |
| `cmd/scrapegutenberg-corpus/main.go` | **create** | Local re-impl of Gutenberg scraper supporting both FI and ET. Starts from main repo's `cmd/scrapegutenberg/main.go` as a copy, adds ET search support. |
| `internal/fetcher/fetcher.go` | **create** | Duplicate ~30-line `downloadFile()` from main repo's `cmd/fetchfrequency/main.go`. (Don't lift — keep main untouched. Intentional DRY-violation.) |
| `internal/parserglue/parserglue.go` | **create** | Thin wrapper around `parsecore.Analyze` exposing only what aggregator needs. |
| `scripts/estnltk-batch.py` | **create** | Python entry point. Loads estnltk once, reads surfaces from stdin, writes JSON analyses to stdout. |
| `testdata/corpus-fixtures/fi/` | **create** | Hand-authored FI fixture for smoke gate. |
| `testdata/corpus-fixtures/et/` | **create** | Hand-authored ET fixture for smoke gate. |
| `docs/CORPUS_PIPELINE.md` | **create** | Operator doc — table of every make target with what it does, when to run, wall clock, outputs. |
| `v1plan.md` | **create** | Copy of this plan, durable local record. |
| `v2plan.md` | **create** | 12 v2 follow-up items with `Why deferred / Effort / Trigger`. |
| `notes/` | **create** | Empty folder for operator notes / error-report autopsies. |

| ~~`pkg/lemmatizer-fi-et/list_analyses.go`~~ | **NOT NEEDED** | `Lemmatizer.Lemmatize(lang, word) []Analysis` already returns all FST hits (verified at lemmatizer.go:114). |
| ~~Modify `cmd/scrapegutenberg/main.go`~~ | **NOT MODIFIED** | We write `corpus_pipeline/cmd/scrapegutenberg-corpus/` instead. Main repo's scraper is untouched. |
| ~~Lift to `internal/fetcher/`~~ | **NOT MODIFIED** | Duplicate the ~30 lines into our local module. |
| ~~Modify root `Makefile`~~ | **NOT MODIFIED** | Local Makefile at `corpus_pipeline/Makefile`. |
| ~~Modify `scripts/setup-local.sh`~~ | **NOT MODIFIED** | Pipeline is invoked manually, not from setup-local. |
| ~~Modify `docs/data_enhancement.md`~~ | **NOT MODIFIED** | Operator doc lives at `corpus_pipeline/docs/CORPUS_PIPELINE.md` instead. |
| ~~Modify `docs/ARTIFACT_POLICY.md`~~ | **NOT MODIFIED** | Existing policy already covers `localdata/`. |
| ~~Modify `TODO.md`~~ | **NOT MODIFIED** | v2 items live at `corpus_pipeline/v2plan.md`. |
| `localdata/fi-corpus/` | **reorg** | Move existing folders into per-source layout. `localdata/` is gitignored, so reorganizing within it doesn't show in `git status`. |

## Decisions taken (current — supersedes any earlier text)

1. **Multi-analysis enumeration uses existing `Lemmatizer.Lemmatize`.**
   No new API. `pkg/lemmatizer-fi-et.Lemmatize(lang, word) []Analysis`
   already returns every loaded analysis without picker logic
   (lemmatizer.go:114). Aggregator calls it directly.

2. **`cmd/enrichcorpus` ships ready in v1, runs on demand.** Not part
   of the initial pipeline run. User invokes it via `make
   enrich-corpus[-fi/-et]`. Resumable cache, persistent batch adapter
   for both omorfi (FI) and estnltk (ET).

3. **Sentences are text-only deduped; full provenance lives in
   `sentence_occurrences.tsv`.** sentences.tsv has just `(id, lang,
   text)`. occurrences has `(sentence_id, source, document_id,
   sentence_ix, quality_flags)`.

4. **TSV with `encoding/csv`, `Comma='\t'`.** csv package handles
   tab-in-content, quote escaping, line-break preservation correctly.

5. **Wordlist rows collapse on identical (lemma, pos, feats).** Same
   analysis from parser_choice + FST top hit becomes one row with
   `analysis_sources="parser_choice;fst"`, not two rows. Different
   analyses (verb vs. noun reading of "tulen") stay as separate rows.
   `surface_count_prose`, `surface_count_poetry`, and `surface_count_total` all repeat across rows for the same surface — column
   name makes this clear.

6. **`document_id` and `sentence_ix` preserved end-to-end** via
   `documents.tsv` and `sentence_occurrences.tsv`. Wordlist's
   `example_ref_type` / `example_ref_id` / `example_text` triple
   resolves through these (sentence rows for prose-first surfaces,
   poem rows for poetry-only surfaces).

7. **Source-tagged poetry only — no auto-detection in v1.** Routing
   happens via `<source>/manifest.json` `kind: poetry`. SKVR, runosto,
   ERAB, **eeva-poetry**, hand-curated Gutenberg poetry IDs go to
   `poems.tsv`. **eeva-prose** stays prose (older Estonian literature
   that isn't poetry — the EEVA extractor inspects per-document
   metadata to route).

8. **Phase 1 uses `internal/parserffi.AnalyzeText` (NOT
   `parsecore.Analyze`).** parserffi is the raw FFI tokenizer/sentence-
   splitter; parsecore adds dictionary lookup + enrichment + 300K char
   limit which we don't need for counting. parsecore is reserved for
   phase 2 enrichment of unique surfaces.

9. **Two-phase aggregation.** Phase 1 = parserffi-tokenize all sources
   + count surfaces + dedup sentences (~hours, dominated by I/O and
   tokenization). Phase 2 = parsecore + FST + cache lookup per *unique*
   surface (~5M, not 200M tokens). Single SQLite writer goroutine
   coordinates all writes.

10. **Anti-circularity rule.** Initial mining writes
    `internal-consensus-candidates.tsv` (basic + custom + FST agree).
    Silver file policy: `silver-candidates.tsv` is absent before
    `cmd/enrichcorpus` runs; verifier accepts absent OR header-only;
    `cmd/aggregatecorpus` never writes it.

11. **Mining outputs are first-class.** `_derived/mining/` ships in v1
    alongside app exports — the corpus serves both the word-list /
    sentence-bank features and the parser-improvement loop.

12. **Bootstrap split into 3 tarballs.** `finnestdb-bootstrap-{code,fi,et}.tgz`.
    Total ~50-80 GB compressed; single-file handoff is impractical.

13. **Licensing is tracked, not gated.** `manifest.tsv` records license
    for hygiene. No commercial-use allowlists. `localdata/` is local-only
    by policy.

14. **TSV is the v1 canonical user-facing artifact; SQLite is scratch
    only.** Aggregator uses `_scratch.db` (SQLite) during phase 1-3 to
    avoid OOM on 50-80M sentences; deletes it after writers complete
    (or keeps with `-keep-scratch` for debugging). Final outputs are TSVs.
    Promoting SQLite to a canonical query DB is v2.

15. **Pipeline is re-runnable; cache key includes dict fingerprint.**
    `_derived/cache/surface-analyses.tsv` keyed by
    `(surface, lang, parser_version, fst_tables_sha, dict_fingerprint)`.
    Bumping any of the four invalidates the relevant cache entries.
    Adding EPUBs after initial run = ~5-15 min, not 6 hours.

16. **EPUB source is folder-driven.** No download URL; user drops files
    into `localdata/{fi,et}-corpus/epub/raw/`. Same pattern for any
    future "I have local files" source.

17. **Three EPUB commands**: `make epub-to-text` (extract only),
    `make add-epub` (extract + fold into giant list), `make epub-deck`
    (per-book wordlist, standalone). All built in v1.

18. **Promotion ladder: 3 tiers** — smoke (fixture, ~2 min), pilot
    (~500 MB/lang, ~30-60 min), full (everything, ~10-12 hrs). Each
    gate is `cmd/corpusverify`; promotion is `cmd/corpuspromote`.

19. **Local-only, zero impact on main repo.** Everything in
    `corpus_pipeline/` (own Go module with `replace
    finnestdb => ../..`). No tracked-file modifications, no commits, no
    PRs. After v1 build, `git status` shows no NEW changes outside
    pre-existing untracked files (e.g. `design/*.jsx` already present).

20. **Lemmatizer tables path is explicit, not defaulted.**
    `pkg/lemmatizer-fi-et` defaults its tables dir to
    `localdata/lemmatizer-fi-et/tables` relative to **cwd**. Our commands
    run from `corpus_pipeline/`, so the default resolves to
    the WRONG path and FST silently disables. The fix:
    - At command startup, compute `tablesDir =
      filepath.Join(dataRoot, "lemmatizer-fi-et", "tables")` (absolute,
      resolved from `-data-root` flag).
    - Set `os.Setenv("LEMMATIZER_TABLES_DIR", tablesDir)` BEFORE any
      `store.DB` open or `parsecore.Analyze` call (these resolve tables
      via the env var when set).
    - Aggregator's direct FST enumeration uses
      `lemmatizer.NewFromDir(tablesDir)` explicitly, not `New()`.
    - Verifier preflight asserts `<resolved tablesDir>/fi_min.json` and
      `et_min.json` exist and non-empty (or whatever filenames the
      runtime expects — `lemmatizer.NewFromDir` will error if they're
      wrong, which is the actual ground truth).

21. **Phase-2 example keys are stable hashes, not final IDs.**
    Phase 4 is where deterministic `sentences.tsv.id` and `poems.tsv.id`
    are assigned. Phase 2 cannot write `example_ref_id` yet — the IDs
    don't exist. The fix:
    - Phase 2 stores `example_sentence_hash` (for prose-first surfaces)
      or `example_poem_key = (source, document_id)` (for poetry-only
      surfaces) into the scratch DB row for each unique surface.
    - Phase 4, AFTER assigning final IDs to `sentences.tsv` and
      `poems.tsv`, walks the wordlist + mining tables and resolves
      each example key into the final `example_ref_type`,
      `example_ref_id`, `example_text` triple before writing the
      output TSVs.
    - Order of phase-4 writers matters: write `sentences.tsv` and
      `poems.tsv` FIRST (final IDs assigned), then `wordlist.tsv` and
      mining TSVs (resolve example keys against the now-stable IDs).

22. **Lang code normalization at the boundary.** CLI accepts `-lang fi|et`
    (lowercase, matches user expectation and existing tooling like
    `parser-comparison.sh`). Internal calls into `parsecore.Analyze`,
    `store.DB`, and `lemmatizer` expect `FI|ET` (uppercase, matches the
    runtime API). The fix: each command does ONE normalization at
    startup — `langUpper := strings.ToUpper(lang)` — and uses
    `lang` (lowercase) for filesystem paths/TSV column values, `langUpper`
    for parser/FST calls. No mixing the two later.

23. **Dictionary DB path is explicit and validated.** `parsecore.Analyze`
    depends on the main repo's `finnestdb.db`. Commands run from
    `corpus_pipeline/`, so a default DB path would point at
    the wrong place. Worse, `store.NewDB` will silently initialize a
    fresh empty database if handed a bad path — which means parser
    runs would succeed against an empty dict and emit catastrophically
    bad results without erroring. The fix:
    - At command startup, compute
      `repoRoot = filepath.Clean(filepath.Join(dataRoot, ".."))`.
    - Every command takes a `-db <path>` flag with default
      `filepath.Join(repoRoot, "finnestdb.db")`.
    - **Before** calling `store.NewDB`, the command asserts (using
      the actual schema — no `dict` table exists, the real dictionary
      tables are `forms`, `lemmas`, `dict_metadata`):
      (a) DB file exists and is non-empty.
      (b) Required tables exist: `forms`, `lemmas`, `dict_metadata`.
      (c) `SELECT COUNT(*) FROM forms WHERE lang = ?` > 0 with
          `langUpper` (FI/ET).
      (d) `SELECT COUNT(*) FROM lemmas WHERE lang = ?` > 0 with
          `langUpper`.
      Failure = HARD error with `preflight.dict.missing_or_empty`
      error_class. Catches the silent-empty-DB trap.
    - That DB is used for all `parsecore.Analyze` calls AND for
      computing `dict_fingerprint` (hash of `dict_metadata` rows +
      relevant table row counts).
    - `build_metadata.json` records `db_path` (absolute) AND
      `dict_fingerprint` so re-runs can detect "wait, the DB changed
      under us."
    - This is also a hard smoke gate — verifier rejects runs against
      empty/wrong-DB before any expensive parsing happens.

## Verification plan

End-to-end test on a *small subset* (1 source per language) before the
full 20 GB run:

1. `go run ./cmd/fetchcorpus -lang fi -source opus-tatoeba`
   → `localdata/fi-corpus/opus-tatoeba/raw/fi.txt.gz` exists, size matches
   HEAD probe, `manifest.json` written, `.sha256` sidecar written.
2. `go run ./cmd/extractcorpus -lang fi -source opus-tatoeba`
   → `text.txt` exists, ≥10K non-empty lines, `documents.jsonl` non-empty,
   valid UTF-8 (`file --mime` confirms).
3. `go run ./cmd/aggregatecorpus -lang fi -only opus-tatoeba`
   → All 8 derived files exist (`wordlist.tsv`, `sentences.tsv`,
   `sentence_occurrences.tsv`, `poems.tsv`, `manifest.tsv`,
   `documents.tsv`, `build_metadata.json`, `qa-report.json`) + 5
   initial mining TSVs (unresolved, poetry-unresolved,
   parser-disagreements, high-frequency-ambiguous,
   internal-consensus-candidates). `silver-candidates.tsv` is
   absent (or header-only) — initial aggregator never writes it; only
   `cmd/enrichcorpus` does.
4. Spot-check ambiguity: `awk -F'\t' '$1=="tulen"' wordlist.tsv` → expect
   ≥2 rows (verb + noun analyses), one with `is_parser_choice=1`.
5. Spot-check sentence-bank provenance: pick id=1, follow `document_id`
   into `documents.tsv`, confirm `raw_path` resolves.
6. **Spot-check anti-circularity**: `wc -l mining/internal-consensus-candidates.tsv`
   should be < wordlist row count (consensus requires agreement, so
   proper subset). `mining/silver-candidates.tsv` must be **absent or
   header-only** per the Silver file policy — `cmd/aggregatecorpus`
   never writes it; only `cmd/enrichcorpus` does.
7. Repeat 1-6 for ET with `opus-tatoeba`.
8. `make bootstrap-tarball` → 3 tarballs exist:
   `finnestdb-bootstrap-code.tgz`, `finnestdb-bootstrap-fi.tgz`,
   `finnestdb-bootstrap-et.tgz`. Verify each with `tar tzf <file> | head`:
   code tarball lists `finnestdb.db` + small derived files; fi/et
   tarballs list `localdata/{fi,et}-corpus/` content.

Then run the full pipeline:

9. `make fetch-corpus` (~6 hours wall clock for 20 GB)
10. `make extract-corpus` (~30-60 min)
11. `make aggregate-corpus` (~3-6 hours; pass 1 fast, pass 2 dominated by
    parser at ~5K tok/s × N cores on ~5M unique surfaces)
12. `make bootstrap-tarball`

Final verification:

- `wc -l _derived/wordlist.tsv` → expect 5–15M rows
- `wc -l _derived/sentences.tsv` → expect 50–80M rows for FI
- `wc -l _derived/poems.tsv` → expect 5K–50K rows (SKVR has ~85K poems)
- `du -sh localdata/{fi,et}-corpus` → ~25-30 GB each (raw + extracted text + sentence_occurrences + scratch.db before deletion)
- `du -sh finnestdb-bootstrap-*.tgz` → likely **3 split tarballs**:
  - `finnestdb-bootstrap-code.tgz` — just `finnestdb.db` + small derived files (~2 GB)
  - `finnestdb-bootstrap-fi.tgz` — `localdata/fi-corpus/` (~25-40 GB)
  - `finnestdb-bootstrap-et.tgz` — `localdata/et-corpus/` (~25-40 GB)
- `jq '.totals.unresolved_rate_prose' _derived/qa-report.json` → expect
  <0.10 (else flag for parser improvement)
- `jq '.totals.unresolved_rate_poetry' _derived/qa-report.json` → expect
  <0.20 (poetry tolerates higher unresolved due to archaisms; values
  above 0.20 suggest extractor or tokenization bugs in poetry sources)
- `jq '.hard_failures' _derived/qa-gate.json` → expect `[]` (empty)
- `wc -l _derived/sentence_occurrences.tsv` → ≥ `wc -l _derived/sentences.tsv` (occurrences ≥ unique sentences)

## Cleanliness rule — zero impact on the main repo

**Hard constraint: nothing modifies tracked files, nothing creates new
untracked files outside `localdata/`.** No commits, no future-PR
pollution.

Caveat on "clean": the current worktree already has *unrelated*
pre-existing changes (`internal/store/db.go` modified, `design/*.jsx`
and `design/*.html` untracked from prior work). Those are not part of
this workstream and we leave them alone. The cleanliness rule for
this workstream is more precisely:

> No new modifications to tracked files outside `localdata/`. No new
> untracked files outside `localdata/`. Pre-existing unrelated worktree
> state is ignored.

A `git status` snapshot before the v1 build is captured and compared
against the post-build snapshot — any *delta* outside `localdata/` is
a bug.

**Everything lives in `corpus_pipeline/`** — under the
already-gitignored `localdata/` umbrella. This includes:

- The plan docs (v1plan.md, v2plan.md, notes/)
- The Go code itself (its own module via replace directive)
- The Makefile for the corpus pipeline (separate from main repo's)
- The operator documentation
- Test fixtures
- Any helper scripts

**How the Go code can still import `internal/parsecore` etc**:
`corpus_pipeline/go.mod` declares its own module
(`finnestdb/corpus_pipeline`) with a `replace` directive:

```go
// corpus_pipeline/go.mod
module finnestdb/corpus_pipeline

go 1.21    // matches main repo's go.mod

require finnestdb v0.0.0

replace finnestdb => ..
```

**Why this exact module path**: Go's `internal/` visibility rule —
`x/y/internal/z` is importable only from packages whose import path
starts with `x/y/`. So `finnestdb/internal/parsecore` is importable
from any `finnestdb/...` package. Our module path
`finnestdb/corpus_pipeline` satisfies this; a generic
`corpus-pipeline.local` would NOT satisfy it and our imports of
`finnestdb/internal/parsecore` would fail to compile.

Then commands import `finnestdb/internal/parsecore`,
`finnestdb/pkg/lemmatizer-fi-et`, `finnestdb/internal/store`,
`finnestdb/internal/parserffi` just fine. `cd
/path/to/main-repo/corpus_pipeline && go run
./cmd/fetchcorpus` resolves all dependencies.

**Operate from the main repo's localdata, not the worktree.**
Worktrees have separate gitignored `localdata/` per worktree (the
worktree's localdata is mostly empty). The user's actual data —
including the 230 FI EPUBs found 2026-05-08 — lives at
`/Users/sagar/Downloads/projects/finnestdb/localdata/`. Place
`corpus_pipeline/` there. Execution happens from the main repo path.

**Data-root flag.** Every command takes `-data-root <path>` (default
`..`, resolved at startup to absolute path). Outputs land at
`{data-root}/{fi,et}-corpus/...`. With default `-data-root=..`, running
from `corpus_pipeline/` resolves to `localdata/{fi,et}-corpus/`
which is what we want.

**Implications for the design** (things I previously had as "modify main
repo" → now "duplicate locally" or "write our own"):

| Originally planned | Replacement |
|---|---|
| Edit `cmd/scrapegutenberg/main.go` to add `-lang et` | Write our own `corpus_pipeline/cmd/scrapegutenberg-corpus/main.go` supporting both FI and ET. ~80% of the original code is reusable as a starting point. |
| Lift `downloadFile()` from `cmd/fetchfrequency/main.go` into `internal/fetcher/` | Inline ~30 lines into `corpus_pipeline/internal/fetcher/fetcher.go` (local module). |
| Edit root `Makefile` to add corpus targets | Local Makefile at `corpus_pipeline/Makefile`. Operate via `cd corpus_pipeline && make fetch-corpus`. |
| Edit `docs/data_enhancement.md`, `docs/ARTIFACT_POLICY.md` | Skip. Keep `docs/CORPUS_PIPELINE.md` local at `corpus_pipeline/docs/CORPUS_PIPELINE.md`. |
| Edit `scripts/setup-local.sh` | Skip. Corpus pipeline is invoked manually, not from setup-local. |
| Edit `TODO.md` | Skip. v2 items live in `corpus_pipeline/v2plan.md`. |
| Reorg existing `localdata/fi-corpus/` (move folders) | This still happens — but `localdata/` is already gitignored, so reorganizing within it doesn't show in `git status`. Safe. |

**Result**: After the entire build is done, `git status` shows nothing
new. The corpus pipeline is invisible to git. Deleting it = `rm -rf
corpus_pipeline/` (and reclaim 80 GB by also `rm -rf
localdata/{fi,et}-corpus/`).

## v2 follow-ups — where they live, how they get done

**All local. No PRs. No GitHub.** The whole pipeline is local-only by
explicit user policy.

**Plan & tracking docs folder**: `corpus_pipeline/` (the same
folder as everything else):

- `corpus_pipeline/v1plan.md` — copy of this plan, kept with
  the project so future-you can read it without digging through
  `~/.claude/plans/`.
- `corpus_pipeline/v2plan.md` — v2 follow-up tracker (the
  table below, expanded with `Why deferred / Effort / Trigger` blocks).
- `corpus_pipeline/notes/` — any operator notes,
  troubleshooting recipes, error-report autopsies you accumulate.

**v2 item structure** in `v2plan.md`:

```markdown
- [ ] **<title>** — one-line summary
  - **Why deferred:** <reason — usually scope or scale or unverified>
  - **Effort:** <hrs estimate>
  - **Trigger:** <condition that means "do this now">
```

The trigger is the operative part — without it, v2 lists rot.

**How items get done**: when a trigger fires, you (or Claude) just open
`v2plan.md`, pick the item, write code locally. No PR, no review
ceremony. Tick the checkbox when done; record what shipped.

### Items committed to TODO.md (with triggers)

| # | Item | Why deferred | Effort | Trigger |
|---|---|---|---|---|
| 1 | omorfi/estnltk persistent server — replace per-batch subprocess with a long-lived gRPC/HTTP server | v1 batch adapter works for ~1-day runs; server is overkill until we re-enrich frequently | ~6 hrs | `make enrich-corpus` is being run >1×/month and the subprocess startup dominates |
| 2 | SQLite mirror as canonical (reviewer's full schema: corpus_sources, corpus_documents, corpus_sentences, corpus_poems, surface_counts, surface_analyses) | TSVs work for "more for inspiration"; SQLite needed when query patterns demand it | ~8 hrs | App needs runtime queries against corpus data (e.g. "show all Essive forms for talo sorted by frequency") |
| 3 | Sentence-bank UI integration (surface sentences as flashcard context) | UI work, separate workstream | ~16 hrs | Flashcard MVP exists and needs example sentences |
| 4 | Register tagging (auto-classify each sentence: conversational / news / parl / literary) | Useful but not blocking; needs a classifier | ~8 hrs | Comprehension-coverage curve UX needs per-register splits |
| 5 | Sentence-level minhash dedup across registers | v1 dedups exact matches only | ~6 hrs | OpenSubtitles or similar is dominating frequency in a way that hurts learners |
| 6 | Incremental refresh — `make corpus-refresh` only re-fetches sources whose HEAD changed | v1 is full re-fetch on demand | ~4 hrs | Re-fetch wall clock is too long to repeat |
| 7 | Auto-poetry-detection heuristic — capture poetry quoted in prose sources | v1 routes by source manifest only | ~6 hrs | We notice prose corpora have meaningful poetry content (Wikipedia, Gutenberg) being mis-routed |
| 8 | Quality scoring per source — language-ID confidence + encoding cleanliness, auto-demote low-quality sources | v1 reports quality flags but doesn't act on them | ~8 hrs | One source's contamination shows up disproportionately in mining outputs |
| 9 | Living pipeline — scheduled refresh, drift detection, alert on URL 404 | v1 is manual-trigger only | ~12 hrs | We commit to running this routinely instead of one-shot |
| 10 | **Expand ET coverage further** — full ERAB corpus via CLARIN academic contact; scrape additional ERR portals (uudised, kultuur, arhiiv); modern authored ET poetry (luuletus.ee / Luuleleid case-by-case license review) | v1 closes parl + folk-poetry gaps and adds 173 MB of ERR news; broadcaster total still 1/3 of FI's Yle, modern poetry still missing | ~10 hrs | ET frequency curves show register imbalance still hurting users after v1 ships, OR comprehension data motivates more ET news depth |
| 11 | Token-level analysis provenance — `token_occurrences.tsv` mapping every token to (sentence_id, ix, analysis_id) | v1 keeps example_sentence_id only — sufficient for "show me in context" but not full provenance | ~6 hrs | We need to train a model or compute statistics that require token-by-token provenance |
| 12 | Local model training (compound segmentation, parser tie-breaker) | Needs the corpus first | ~40 hrs | After v1 ships and we can reason about model quality vs. parser quality |

`corpus_pipeline/v2plan.md` is canonical — this table in the
plan is just a snapshot. The v1 build's last step writes these entries
to `v2plan.md` (with the structured `Why deferred / Effort / Trigger`
blocks). After that, the plan file in `~/.claude/plans/` is throwaway;
`corpus_pipeline/v1plan.md` + `v2plan.md` are the durable
local records.

## Monitoring during execution + auto-recover loop

When I (Claude) run the pipeline (`make corpus-promote` or its
sub-targets), I do not just kick it off and walk away. The execution
loop is:

```
loop:
  run `make corpus-promote-{fi|et}`
  if exit code 0 AND _derived/qa-gate.json shows no hard failures:
    → SUCCESS, break
  else:
    → read _derived/errors/error_report.txt + qa-gate.json
    → spot-check the relevant TSV files (mining/unresolved.tsv, etc.)
    → diagnose: extractor bug? aggregator race? schema mismatch?
                source-specific data quirk? missing dependency?
    → fix the bug in corpus_pipeline/ source
    → append a `repaired.jsonl` entry with fix_id + fix_description
    → restart:
        - if pilot/full failed: `make corpus-promote-{fi|et}` resumes
          via promotion-state.json (smoke / pilot already passed → skip)
        - if smoke failed: full restart from scratch
    → loop
```

Concrete behaviors:
- **Read the error before fixing.** Don't blindly re-run — the
  error_report.txt + qa-gate.json tell me what failed and where.
- **Watch the soft-gate warnings too.** Even if hard gates pass, soft
  warnings (URL contamination, very-long sentences, single-source
  dominance) flag extractor bugs that should be fixed before promoting
  to the next profile.
- **Track repeated errors.** If the same error_class appears in
  error-history.jsonl across multiple runs without a corresponding
  repaired.jsonl entry between them, that's a regression — flag it
  loudly in the QA report.
- **Use Monitor for long-running profiles.** For pilot (~30-60 min) and
  full (~10-12 hrs), I attach a Monitor to the make process so I get
  notified when it exits without polling.

This is execution discipline, not pipeline code. The pipeline emits
the artifacts; I (or future-me, or another Claude session) interpret
and act on them.

## Existing data on disk — re-audited 2026-05-08

(Earlier I said "0 EPUBs found" — that was wrong. My `find` ran from
the worktree which has its own gitignored `localdata/`. The main repo
has substantial existing data.)

What IS on disk in the main repo (`/Users/sagar/Downloads/projects/finnestdb/localdata/`):

| Path | Content | v1 treatment |
|---|---|---|
| `localdata/fi-corpus/manual/` | **230 FI EPUBs** (Witcher 8 books, Antti Tuomainen crime, Tove Jansson Muumi, Lars Kepler, Lee Child Jack Reacher, Robin Hobb, Hugh Howey, Andy Weir, Alastair Reynolds, lots more) **+ 27 hand-pasted `.txt` files** (articles, book excerpts, Harry Potter translation, Murakami parallel text) **+ misc** | **Reorg splits this**: EPUBs → `fi-corpus/epub/raw/`. `.txt` files stay → `fi-corpus/manual/raw/`. The first full run processes BOTH (EPUBs via `pandoc`, txt via passthrough). |
| `localdata/fi-corpus/raw/` | Original HTML scrapes from earlier work | Move into `manual/raw/` (provenance). |
| `localdata/fi-corpus/text/final/` | 20 extracted text files (~119K words) | Already cleaned — fold into `manual/text.txt`. |
| `localdata/fi-corpus/lists/` | URL selection lists | Archive into `manual/lists/`. |
| `localdata/fi-corpus/meta/manifest-final.tsv` | Existing manifest of 20 hand-curated articles | Seed `documents.jsonl` for manual source. |
| `localdata/fi-corpus/clean/` | Whatever's there | Audit during reorg. |
| `localdata/et-corpus/` | **2 Estonian `.md` files** (`LingQ 1-30 ENG_EST.md`, `LingQ 31-60 ENG_EST.md`) — parallel-text language-learning material | Reorg moves them → `et-corpus/lingq-parallel/raw/`. New extractor needed: `extract_md.go` that strips Markdown structure and Estonian-only sentences (drop English equivalents). |

**The first full run includes the 230 FI EPUBs and 2 ET md files
automatically** — they're discovered by walking
`localdata/{lang}-corpus/*/manifest.json`, no source-list edit needed.
EPUB extraction wall clock: ~30-60 min for 230 books via pandoc.

**Format extractors gain `extract_md.go`** (Markdown stripping + ET
sentence isolation from parallel text).

## Stage-by-stage execution order

All work happens inside `corpus_pipeline/`. Main repo work
tree stays clean throughout.

1. **Preflight**: from main repo root, ensure `make parser` has run
   recently — verify `parser/target/release/libparser.{dylib,so}`
   exists and is non-empty. If missing, run `make parser`. Failure here
   means parserffi can't link, so smoke gate would fail anyway. Catch
   it before scaffolding (~5 min, or up to ~5 min for cold cargo build).
2. **Create `corpus_pipeline/` skeleton at the main repo's
   localdata path**: `go.mod` with `module finnestdb/corpus_pipeline`
   + `go 1.21` + `replace finnestdb => ..`. Empty `cmd/`, `internal/`,
   `scripts/`, `testdata/`, `docs/`, `notes/` folders. Stub `Makefile`
   (~30 min).
3. **Verify replace directive works**: tiny test main.go that imports
   `finnestdb/internal/parsecore` AND `finnestdb/internal/parserffi`,
   calls `parserffi.AnalyzeText("FI", "Hei maailma.")`, prints result.
   Run via `go run ./cmd/_smoke/` (~15 min). De-risks both the module
   path AND the Rust FFI link.
4. **Reorg existing `localdata/{fi,et}-corpus/`** to per-source layout
   (~1 hr — bigger now that we know about 230 EPUBs + 2 ET md files):
   - Audit (absolute path) `find /path/to/main/localdata -iname "*.epub"`
     → 230 FI EPUBs in `fi-corpus/manual/`. Move them to
     `fi-corpus/epub/raw/` and write `epub/manifest.json` with
     `format: epub, kind: prose, source: epub-personal`.
   - Move FI `.txt` files in `fi-corpus/manual/` → `fi-corpus/manual/raw/`
     (preserve as raw text). Write `manual/manifest.json` with
     `format: text, kind: prose`.
   - Move `fi-corpus/raw/` HTML scrapes → `manual/raw/` (consolidate).
   - Move `fi-corpus/text/final/*.txt` → `manual/text.txt` (or
     per-document text files).
   - Move `fi-corpus/lists/` → `manual/lists/`.
   - Convert `meta/manifest-final.tsv` → `manual/documents.jsonl`.
   - Move `et-corpus/*.md` (2 LingQ parallel-text files) →
     `et-corpus/lingq-parallel/raw/`. Write `lingq-parallel/manifest.json`
     with `format: md_lingq_parallel, kind: prose`.
   - Create empty per-source folders for the ~25 FI + 25 ET sources
     the fetcher will populate.
   - All operations within gitignored `localdata/`, so `git status`
     shows no delta on tracked files.
5. **Write `corpus_pipeline/cmd/scrapegutenberg-corpus/`**
   from a copy of main repo's scraper, add `-lang et` (~1 hr).
6. **Inline `internal/fetcher/fetcher.go`** (~20 min, ~30 LoC).
7. **Build `cmd/fetchcorpus` + source registries** (~3 hrs).
8. **Build `cmd/extractcorpus` + 13 format extractors + quality flags** (~7.5 hrs — VRT, gz, Leipzig tar, MediaWiki XML, CSV, EPUB, SKVR XML, generic HTML scrape, HuggingFace, Riigikogu, ERAB, EEVA-prose/poetry split, Markdown LingQ).
9. **Build `cmd/aggregatecorpus` (4 phases + cache + SQLite scratch + deterministic ID assignment)** (~7 hrs).
10. **Build `cmd/epubdeck`** (~2 hrs).
11. **Build `cmd/enrichcorpus` with persistent batch adapters** (~7 hrs).
12. **Build `cmd/corpusverify` (preflight + hard/soft gates)** (~4 hrs).
13. **Build `cmd/corpuspromote`** (~3 hrs).
14. **Fill out `corpus_pipeline/Makefile` with all targets** (~1.5 hr).
15. **Author hand-built smoke fixtures** at `testdata/corpus-fixtures/{fi,et}/` (~1 hr).
16. **Write `docs/CORPUS_PIPELINE.md` operator doc** at `corpus_pipeline/docs/CORPUS_PIPELINE.md` (~1.5 hr).
17. **Write `v1plan.md` (copy of this plan) and `v2plan.md` (12 items with `Why deferred / Effort / Trigger`)** at `corpus_pipeline/` (~45 min).
18. **Verify clean work tree (delta-based)**: snapshot `git status
    --porcelain` before step 1, snapshot it again after step 17. The
    diff must be empty for paths outside `localdata/`. Pre-existing
    unrelated state (e.g. `design/*` untracked from prior work)
    is irrelevant — only the *delta* matters (~5 min sanity check).
19. **Run smoke profile** (`cd corpus_pipeline && make corpus-promote-fi corpus-promote-et` with PROFILE=smoke) — must pass before any real fetch. Includes the FST-tables + Rust-parser preflight.
20. **Run pilot profile** (PROFILE=pilot, ~500 MB/lang) — must pass before full run.
21. **Run full pipeline** (`make corpus-promote`) — wall clock ~10-12 hours. Full run includes the 230 FI EPUBs + 2 ET LingQ md files automatically (discovered by walking `*/manifest.json`). Monitor + auto-recover loop on hard-gate failures.
22. **Verify incremental EPUB workflow** — drop one fresh EPUB into
    `localdata/fi-corpus/epub/raw/`, run `make add-epub-fi`, confirm
    only the new book's surfaces enrich (cache-hit count is high).
    Test `make epub-deck-fi` on that one file. Repeat for ET if any
    EPUBs available there.
23. **Smoke-test `cmd/enrichcorpus`** on a 100-surface slice (don't
    run the full 24-hour pass — confirm batch adapter starts up,
    speaks the protocol, writes a sane wordlist-enriched.tsv slice).
24. **Final delta check vs. pre-build snapshot**: `git status
    --porcelain` diff outside `localdata/` must be empty. Pre-existing
    unrelated state (e.g. `design/*` untracked) is fine; only NEW
    deltas outside `localdata/` are bugs.

Total dev time: **~42 hours of code** (added ~2 hrs for the new ET
extractors: HuggingFace dataset puller, Riigikogu HTML scraper, ERAB
TXT/XML extractor, EEVA HTML scraper). Plus wall clock for the 20 GB
run. That's a real ~5-day week. If you want a tighter v1, the items
I'd cut first are: `cmd/corpuspromote` ladder (run levels manually
instead), `cmd/epubdeck` (do per-book extraction by hand from
epub/per-book/), `sentence_occurrences.tsv` (collapse back into
sentences.tsv `sources` column). That gets us to ~30 hours.

## What happens *after* the initial run

The initial run produces the directory layout, the cache, all derived
TSVs, and the bootstrap tarball. Then:

- **You add EPUBs.**
  ```sh
  cp ~/book1.epub localdata/fi-corpus/epub/raw/
  cp ~/book2.epub localdata/fi-corpus/epub/raw/
  make add-epub-fi              # ~5-15 min, EPUBs folded into giant list
  ```
  EPUBs land in `wordlist.tsv` (new surfaces), `sentences.tsv` (new
  sentences), `documents.tsv` (new doc IDs).

- **You want a per-book deck for one EPUB.**
  ```sh
  make epub-deck-fi EPUB=tuntematon-sotilas.epub
  # → localdata/fi-corpus/epub/decks/tuntematon-sotilas.tsv
  ```
  Standalone per-book wordlist, doesn't pollute the giant list.

- **You just want EPUBs as plain text (not in the giant list).**
  ```sh
  make epub-to-text-fi
  # → localdata/fi-corpus/epub/per-book/<slug>.txt
  ```

- **You want richer FEATS via omorfi/estnltk.**
  ```sh
  make enrich-corpus-fi    # ~12-24 hrs
  # → localdata/fi-corpus/_derived/wordlist-enriched.tsv
  ```
  Resumable. Doesn't touch sentences.tsv / poems.tsv / mining outputs.

- **A source releases new data.**
  ```sh
  go run ./cmd/fetchcorpus -lang fi -source opus-opensubtitles -force
  make extract-corpus-fi aggregate-corpus-fi
  ```

- **The parser improves.** Bump `parser_version` in code, run
  `make corpus-cache-clear aggregate-corpus`. Cache wipes, all surfaces
  re-enrich, mining outputs reflect the new parser. Wall clock ~3-6 hrs.

- **You want a fresh handoff tarball.**
  ```sh
  make bootstrap-tarball
  ```

All commands are idempotent. None of them push data to git
(`localdata/` is gitignored).
