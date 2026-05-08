# Corpus pipeline — operator manual

Reference card for the corpus pipeline that lives at
`corpus_pipeline/`. All paths are relative to the main
finnestdb repo's `localdata/` directory.

The pipeline source is tracked so agents and humans share one reproducible
implementation. Corpus data remains local-only under `localdata/`. See
`docs/DECISIONS.md` for the rationale and report policy. See
`docs/PR_ROADMAP.md` for the near-term PR sequence and the reason each
planned schema/export change exists.

---

## Architecture in 30 seconds

```
                ┌─── localdata/{fi,et}-corpus/<source>/raw/   (downloads)
fetch ─────────►│
                └─── <source>/manifest.json                    (per-source metadata)

                ┌─── <source>/text.txt              (prose, paragraph per line)
extract ───────►│
                └─── <source>/poems.jsonl           (poetry, line breaks preserved)
                └─── <source>/documents.jsonl       (per-doc metadata)

aggregate ─────►_derived/wordlist.tsv               (giant inflected-form list, one row per analysis)
                _derived/sentences.tsv              (text-only, deduped, deterministic IDs)
                _derived/sentences_user_friendly.tsv (filtered learner-facing sentence bank)
                _derived/sentence_occurrences.tsv   (full provenance)
                _derived/poems.tsv                  (line breaks preserved)
                _derived/documents.tsv
                _derived/manifest.tsv
                _derived/build_metadata.json
                _derived/qa-report.json
                _derived/mining/                    (parser-improvement candidates)
                  ├── unresolved.tsv                (prose-first sort)
                  ├── poetry-unresolved.tsv         (count_poetry≥10 AND ≥5×prose)
                  ├── parser-disagreements.tsv      (basic vs custom)
                  ├── high-frequency-ambiguous.tsv  (≥2 FST analyses)
                  ├── internal-consensus-candidates.tsv (basic+custom+FST agree)
                  └── silver-candidates.tsv         (ONLY by enrichcorpus, omorfi/estnltk agreement)

verify ────────►_derived/qa-gate.json               (hard/soft gate results, exit nonzero on hard fail)

promote ───────►runs the smoke→pilot→full ladder, stops on first hard fail,
                writes _derived/promotion-state.json + errors/error_report.txt
                + appends to errors/error-history.jsonl
```

---

## Quick reference — every make target

Run from `cd corpus_pipeline/`. Every target has `-fi`, `-et`,
and no-suffix (= both) variants.

| Target | What | When to run | Wall clock | Outputs |
|---|---|---|---|---|
| `make build` | `go build ./...` | After code edits | <5 s | binaries cached |
| `make fetch-corpus[-fi/-et]` | Download all registered sources to `raw/` | When sources change or you want a refresh | ~2-30 min depending on registry size | `<source>/raw/*` + `.sha256` sidecars |
| `make fetch-corpus-dry-run` | HEAD probe each URL without downloading | Verifying URLs before a real fetch | <1 min | stderr report |
| `make extract-corpus[-fi/-et]` | Walk all sources, run format-specific extractor | After fetch, or when extractors change | ~1-5 min for all | `<source>/text.txt` (prose) or `poems.jsonl` (poetry) + `documents.jsonl` |
| `make aggregate-corpus[-fi/-et]` | 4-phase aggregation → canonical and user-facing derived TSVs + QA report | After extract | ~minutes for smoke, ~hours for full | `_derived/*.tsv` + `_derived/mining/*.tsv` + JSON metadata |
| `make corpus-verify[-fi/-et] PROFILE=…` | Hard/soft gate check, exit nonzero on fail | After aggregate | <5 s | `_derived/qa-gate.json` (+ `errors/` on fail) |
| `make corpus-promote[-fi/-et] PROFILE=…` | Chain extract→aggregate→verify; smoke→pilot→full ladder | When advancing through profiles | depends on profile | promotion-state.json updated |
| `make corpus-cache-clear` | Wipe `_derived/cache/` (when v2.4 caching lands) | After parser/dict changes | <1 s | clears cache |
| `make bootstrap-tarball` | 3-way split tarball (code, fi, et) | Before handoff to another machine | ~5-30 min depending on data size | `finnestdb-bootstrap-{code,fi,et}.tgz` in repo root |

The `PROFILE` Makefile variable defaults to `smoke`. Override with
`PROFILE=pilot` or `PROFILE=full`.

Migration note: `corpus-verify` requires `_derived/sentences_user_friendly.tsv`.
Existing `_derived/` directories built before that export existed must rerun
`make aggregate-corpus[-fi|-et]` before verify will pass.

---

## Profiles

| Profile | Data slice | Purpose |
|---|---|---|
| `smoke` | Hand-authored fixture (`testdata/corpus-fixtures/{fi,et}/`) + tiny real source. Probes verify FI `Tulen→tulla/VERB`, FI `Talossa→talo/Ine`, FI `Kuusen→kuusi/Gen`, ET `joon` produces ≥2 analyses including `jooma/VERB`. | Fast architecture validation. Run after any code change. |
| `pilot` | ~500 MB per language (currently 4 small OPUS sources per lang) | Full pipeline shape on real data, not toy fixtures. |
| `full` | Everything from the source registry | The actual corpus build. ~10 GB per language. ~10-12 hours wall clock. |

---

## Repair loop on hard-gate failure

`cmd/corpusverify` exits nonzero on hard-gate fail and writes:

- `_derived/qa-gate.json` — machine-readable gate results
- `_derived/errors/error_report.txt` — human-readable summary
- Appends to `_derived/errors/error-history.jsonl` (timestamped, append-only)

Workflow when you (or Claude) hit a hard fail:

1. **Read** `_derived/errors/error_report.txt` — names the specific gate that failed.
2. **Read** `_derived/qa-gate.json` — machine-readable detail.
3. **Diagnose**: extractor bug? Aggregator schema mismatch? Source data quirk?
4. **Fix** in `corpus_pipeline/` source.
5. **Append** a `repaired.jsonl` entry with `{utc, fix_id, error_class,
   original_error_utc, fix_description}` (omit `git_head_sha` —
   we're no-commit local).
6. **Restart**: `make corpus-promote-{fi|et}` resumes via
   `promotion-state.json` (skips passed profiles).
7. **Loop** until clean.

If a known `error_class` re-appears within ~7 days of a `repaired.jsonl`
entry, the verifier flags it as **regression** in the report — catches
silent reverts and pipeline drift.

---

## Cleanliness invariant

`corpus_pipeline/` is tracked source. Runtime corpus data is gitignore-only
under `localdata/`. After every session:

```sh
# from the repo root:
git status --porcelain | grep -v '^?? localdata/' | grep -v '^?? design/'
# expect: empty
```

The corpus pipeline should not create new tracked changes outside
`corpus_pipeline/` unless a PR explicitly says why. Pre-existing unrelated
state (`design/*` untracked, etc.) is fine — only the intended delta matters.

---

## Common operations

### Add a new source (URL-driven)

1. Add a `Source` entry to `cmd/fetchcorpus/sources_fi.go` (or `_et.go`):
   ```go
   {Slug: "opus-foo", Lang: "fi", Kind: "prose", Format: "gz",
    URL: "https://...", Filename: "fi.txt.gz", License: "..."},
   ```
2. `make fetch-corpus-fi -- -dry-run` to verify the URL.
3. `make fetch-corpus-fi` to actually fetch.
4. `make extract-corpus-fi aggregate-corpus-fi corpus-verify-fi`.

### Add a new source (folder-driven, e.g. EPUBs)

1. Drop files into `localdata/{lang}-corpus/<your-slug>/raw/`.
2. Write `localdata/{lang}-corpus/<your-slug>/manifest.json` with
   `format: epub` (or whatever).
3. `make extract-corpus aggregate-corpus`. (Skip fetch — folder-driven.)

### Add an EPUB to the existing FI corpus

```sh
cp ~/some-book.epub localdata/fi-corpus/epub/raw/
cd corpus_pipeline
make extract-corpus-fi aggregate-corpus-fi corpus-verify-fi
```

### Inspect the wordlist for a specific surface

```sh
awk -F'\t' '$1=="tulen"' localdata/fi-corpus/_derived/wordlist.tsv
```

### Top-N most-frequent prose surfaces

```sh
# (header is row 1, sort is by surface_count_prose desc)
tail -n +2 localdata/fi-corpus/_derived/wordlist.tsv | head -100
```

### Find unresolved high-frequency surfaces (parser-improvement priorities)

```sh
head -50 localdata/fi-corpus/_derived/mining/unresolved.tsv
```

---

## Architecture decisions enforced

These are the v1 decisions baked into the code. Don't change them lightly:

1. **Phase 1 uses `parserffi.AnalyzeText`**, not `parsecore.Analyze` (avoids dictionary lookup + 300K char limit during cheap counting).
2. **Phase 2 uses `parsecore.Analyze` + `Lemmatizer.Lemmatize`** for parser_choice + multi-FST analyses on each unique surface.
3. **Wordlist rows collapse on identical (lemma, pos, feats)** across analysis sources. `analysis_sources` is `;`-joined.
4. **Default sort: `surface_count_prose` desc, then `surface_count_total` desc.** Poetry counts visible but not rank-dominant.
5. **Poetry sources contribute to surface counts** (in a separate column), they're not silently dropped.
6. **Deterministic sentence IDs** assigned in phase 4 by sorting first-occurrence (source, document_id, sentence_ix, hash).
7. **Lang code normalization at the boundary**: lowercase for paths/TSV, uppercase for parser/FST.
8. **Dictionary DB preflight uses real schema**: `forms`, `lemmas`, `dict_metadata` tables; counts > 0 for target lang.
9. **`silver-candidates.tsv` is enrichcorpus-only.** `aggregatecorpus` never writes it. Verifier rejects if aggregator emitted rows.
10. **Module path** is `finnestdb/corpus_pipeline` (Go internal/ visibility).

---

## What's not yet built

See [`v2plan.md`](../v2plan.md) for the full deferred-items list with
trigger conditions, and [`PR_ROADMAP.md`](PR_ROADMAP.md) for the planned PR
sequence. Highlights:

- SQLite scratch DB + concurrent worker pool (v2.4) — needed when full
  corpus aggregation OOMs in-memory. Pilot probably fits.
- `cmd/enrichcorpus` (v2.6) — omorfi/estnltk batch adapters for true
  silver labels.
- `cmd/epubdeck` (v2.7) — per-book wordlist extractor.
- 11 more format extractors (v2.3) — vrt, leipzig, wiki, csv, skvr,
  html, huggingface, riigikogu, erab, eeva, plus the parliament csv.
- More OPUS / Yle / parliament sources in the registry (v2.2).
- ET parity: Riigikogu, ERR, ERAB, EEVA (v2.12).

---

## Troubleshooting

### "missing FST tables" hard fail at preflight

```
preflight.fst.missing: /…/localdata/lemmatizer-fi-et/tables/fi_min.json
```

The FST tables aren't in the expected path. Either:
- The main repo's `make parser` was never run, OR
- `localdata/lemmatizer-fi-et/tables/` got moved/deleted, OR
- `LEMMATIZER_TABLES_DIR` env var override is wrong.

Fix: `cd <repo-root> && make parser` and confirm
`localdata/lemmatizer-fi-et/tables/{fi,et}_min.json` exist non-empty.

### "preflight.dict.missing_or_empty" hard fail

```
preflight.dict.missing_or_empty: forms has 0 rows for lang=FI
```

`finnestdb.db` was opened against a fresh/empty file. Either:
- `finnestdb.db` got deleted/moved, OR
- The `-db` flag points at the wrong path.

Fix: confirm `<repo-root>/finnestdb.db` exists, is multi-GB, and has
non-empty `forms` and `lemmas` tables for both FI and ET. If not, run
the main repo's `scripts/setup-local.sh` to rebuild.

### EPUB extractor fails on a specific book

```
[extract_epub] WARN: <book>.epub: <error>
```

Pure-Go zip + XHTML strip. If a book has unusual encoding (e.g. nested
zip, or DRM), it may fail. The book is logged and skipped — other books
continue. Failed-extraction books appear in stderr; check
`<source>/per-book/` for missing entries.

### Aggregator runs out of memory

```
runtime: out of memory
```

The v1 aggregator is in-memory. Trigger v2.4 (SQLite scratch DB +
concurrent workers). For now, work around by aggregating per-source:
`make aggregate-corpus-fi -- -only opus-tatoeba` etc.

### Pipeline runs but `wordlist.tsv` looks empty/wrong

Check `_derived/qa-report.json` totals — `unresolved_rate_prose` near
1.0 means the parser isn't resolving anything (FST tables missing? db
empty?). Also check `mining/unresolved.tsv` for what's slipping through.
