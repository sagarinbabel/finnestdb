# Learnings from the first run

Captured live as the corpus pipeline gets built and exercised on real
data. Pair with `v1plan.md` (the approved plan) and `v2plan.md` (status
+ deferred items). This file is the journal — what surprised me, what I
got wrong, what I changed mid-flight.

Local-only, gitignored under `localdata/`. Append-only.

---

## Session 1 (smoke profile)

### L1 — Worktrees have separate gitignored `localdata/`

I assumed I could `find localdata -name "*.epub"` from the worktree and
get the user's data. **Wrong.** Worktrees share `.git` but the working
tree (including gitignored files) is per-worktree. The user's 230 FI
EPUBs lived in the main repo's `localdata/`, NOT the worktree's. My
first audit returned 0 EPUBs and I told the user "no EPUBs found
anywhere," which was wrong.

**Take-away**: When operating on the user's data from a worktree,
always use absolute paths to the main repo (`/Users/sagar/Downloads/projects/finnestdb/...`)
and never trust relative paths (`localdata/...`) for data lookups.

### L2 — Go internal/ visibility forces module path

Initial plan had module path `corpus-pipeline.local`. The reviewer
caught this: Go's `internal/` rule says `x/y/internal/z` is importable
only from packages whose import path starts with `x/y/`. So importing
`finnestdb/internal/parserffi` from a `corpus-pipeline.local/...`
package would fail.

**Fix**: module path must be `finnestdb/corpus_pipeline` so
the `finnestdb/` prefix matches. Combined with `replace finnestdb =>
../..` in go.mod, the local module behaves as a sub-module of the
main repo without modifying tracked files.

### L3 — `parserffi.AnalyzeText` field is `Form`, not `Surface`

Trivial but cost me a build cycle on the smoke test. Always grep the
struct definition before referencing fields by guessed names.

### L4 — `pkg/lemmatizer-fi-et.Lemmatize` already returns multi-analysis

I planned to add a new `ListAnalyses` API to expose all FST hits. The
reviewer pointed out `Lemmatize(lang, word) []Analysis` already does
this without picker logic. Verified at `pkg/lemmatizer-fi-et/lemmatizer.go:114`.

**Take-away**: search for existing API before designing a new one.
Even when the plan says "add this method," check first.

### L5 — Lemmatizer tables path is cwd-relative by default

`pkg/lemmatizer-fi-et` defaults its tables dir to
`localdata/lemmatizer-fi-et/tables` **relative to cwd**. Commands run
from `corpus_pipeline/` — the default would resolve to the
WRONG path and the FST would silently disable. The reviewer caught
this; fix is to compute `tablesDir = filepath.Join(dataRoot,
"lemmatizer-fi-et", "tables")` at startup and `os.Setenv(
"LEMMATIZER_TABLES_DIR", tablesDir)` BEFORE any parsecore/store call.

### L6 — `store.NewDB` silently initializes empty DB on bad path

If you pass `store.NewDB("./missing.db")`, it creates a fresh empty
database file and returns happily. Then your parser queries succeed
against zero data and emit catastrophic results without erroring.

**Fix**: hard preflight before opening — assert file exists, non-empty,
required tables (`forms`, `lemmas`, `dict_metadata`) exist, count > 0
for target lang. The dict actually has tables `forms`, `lemmas`,
`dict_metadata` — there is NO `dict` table (the original plan said
`dict`, which would have failed silently).

### L7 — Lang code mismatch between layers

CLI takes `-lang fi|et` (lowercase, matches existing project tooling
like `parser-comparison.sh`). Internal `parsecore.Analyze`,
`store.DB`, `lemmatizer` all expect `FI|ET` (uppercase). Without a
boundary normalization, you'd get either silently-empty results or
failed lookups.

**Fix**: every command does ONE normalization at startup —
`langUpper := strings.ToUpper(lang)` — and uses lowercase for paths/
TSV column values, uppercase for parser/FST calls.

### L8 — Aggregator architecture: in-memory vs SQLite scratch

The plan called for SQLite scratch DB + single-writer goroutine + worker
pool from the start. I built a simpler in-memory version for v1 smoke
to validate the architecture quickly. Worked great:
- 5.4 MB OPUS Tatoeba: 145K sentences, 84K surfaces, 20s wall clock
- 550 MB FI pilot (7 sources): 2.8M sentences, 1.4M surfaces, 5m 15s wall clock, ~12 GB peak RSS

**Take-away**: the approved plan said in-memory wouldn't scale. In
reality, in-memory worked all the way through pilot. SQLite scratch is
deferred until something actually OOMs. The plan's caution was prudent
but not the v1 critical path.

---

## Session 2 (pilot profile + extractor expansion)

### L9 — Pure-Go EPUB extraction beats pandoc

Plan said "shell out to pandoc -f epub -t plain". pandoc isn't installed
on this machine. Rather than asking the user to install it, I built a
~120-line pure-Go extractor using `archive/zip` + a regex tag stripper.
**Result: 230 EPUBs → 176 MB clean text in 14 seconds.**

The pure-Go version is also more portable (no shell dep), more debuggable
(everything in Go), and idempotent (cached per-book/<slug>.txt files).

**Take-away**: when a plan calls for a shell-out and the dep isn't
installed, often a 100-line Go alternative is faster overall (no install
friction, no per-call subprocess overhead). Default to in-process unless
the external tool genuinely does something Go can't.

### L10 — OPUS .txt.gz needs synthetic-period for sentence splitting

`extract_gz` originally bundled 500 lines into one paragraph and fed
the bundle to parserffi. For most OPUS data this was fine — sentences
end with `.!?` and parserffi splits cleanly. But OPUS jrc-acquis (EU
legal corpus) has chemistry text like "Karboksüülhappeanhüdriidil
põhinev epoksüvaigu vedel kõvendi, mahukaaluga..." with commas but no
periods. parserffi couldn't split it, returning a SINGLE 100KB
"sentence" per pseudo-doc.

This blew up the wordlist's `example_text` column to 105 KB strings,
making ET wordlist 7.8 GB instead of ~700 MB.

**Two-layer fix:**
1. **Immediate** (band-aid): cap `example_text` at 400 runes in the
   wordlist writer. Sentences.tsv keeps full text (no info loss).
   Result: ET wordlist 7.8 GB → 704 MB.
2. **Root cause**: in `extract_gz`, append `.` to lines that don't end
   with sentence-ending punctuation `.!?:`. This lets parserffi
   sentence-split per-line. Underlying sentences.tsv quality improves
   on next extract+aggregate cycle.

**Take-away**: when `parserffi.AnalyzeText` returns suspiciously few
sentences for a paragraph, suspect the input lacks sentence boundaries
that the FFI splitter needs. Synthetic punctuation insertion is a
cheap fix that respects the format expectation.

### L11 — `tar` errors don't always block tarball creation

First `make bootstrap-tarball-code` errored with `tar: localdata/parser-eval:
Cannot stat: No such file or directory`, but tar exited nonzero AFTER
creating a 1.9 GB tarball. Make reported failure but the artifact existed.

**Fix**: shell out a loop that builds the include-list dynamically from
existing paths only. Avoids the misleading partial-success state.

### L12 — Bootstrap tarballs need to live under `localdata/`

I initially had `bootstrap-tarball` write to repo root next to
`finnestdb.db` (matching `docs/ARTIFACT_POLICY.md`'s example). That
created a new untracked file outside `localdata/`, violating the
cleanliness invariant.

**Fix**: target `localdata/bootstraps/`. Still a single-folder handoff
(same `tar -xzf` works), just inside the gitignored zone. The
`docs/ARTIFACT_POLICY.md` example path was the original plan; the
local-only policy supersedes it.

### L13 — Pilot timing & memory benchmarks (in-memory aggregator)

Real measurements, not estimates:

| Lang | Input bytes | Sentences | Surfaces | Wall clock | Peak RSS |
|---|---:|---:|---:|---:|---:|
| FI | 550 MB extracted (7 sources) | 2,813,008 | 1,395,466 | 5m 15s | ~12 GB |
| ET | 395 MB extracted (6 sources) | 1,123,769 | 1,448,978 | 4m 14s | ~8 GB |

Phase 1 (parserffi tokenize + dedup): dominated by I/O + CGO calls per
paragraph.
Phase 2 (parsecore + Lemmatizer per unique surface): dominated by
dictionary lookups. ~1.4M surfaces × ~150 µs = ~3.5 minutes.

**Extrapolation to full corpus** (10 GB extracted text per lang ≈ 18×
pilot): wall clock ~95 min/lang, RSS ~70-80 GB peak — **OOMs on
typical machines.** That's the v2.4 trigger. SQLite scratch + worker
pool becomes mandatory above ~2-3 GB extracted text.

### L14 — `parsecore.Analyze` on a single token reconstructs ambiguity correctly

For phase 2, I call `parsecore.Analyze(db, langUpper, surface, "custom")`
on each unique surface (just the surface as the whole "text"). This works
because:
- Single-token "sentence" goes through tokenizer + analyzer
- The chosen analysis comes back as `result.Sentences[0].Tokens[0]`
- Combined with `lemmatizer.Lemmatize(langUpper, surface)` for FST
  alternates, we get parser_choice + all-FST-hits without writing a
  custom multi-analysis API

Cleaner than the planned `ListAnalyses` API would have been.

### L15 — `unresolved_rate_prose ≈ 0.001%` is a real signal

Both pilots showed essentially zero unresolved surfaces:
- FI: 16 / 1,395,466 surfaces unresolved (0.00115%)
- ET: 52 / 1,448,978 surfaces unresolved (0.00359%)

This means the parser+FST cover virtually every Finnish/Estonian word
in real text. The unresolved entries are mostly chemistry compound
abbreviations from EU legal text (e.g. "EQ-01", "amino-2",
"heptüül-2-(3-heptüül-4-metüül-4") — not valid words.

**Implication**: the parser is in much better shape than the corpus-
mining workflow expected. The `mining/unresolved.tsv` file is more
useful for catching extractor bugs than for finding parser gaps.

### L17 — `cmd/enrichcorpus` design: graceful skip when analyzer missing

Neither `omorfi-disamb-cmdline` nor `estnltk` was installed on this
machine. Rather than failing the pipeline or asking the user to
install something, `cmd/enrichcorpus` detects the missing tool, prints
a clear install hint (`brew install omorfi` for FI, `pip install
estnltk` for ET), and **exits cleanly with code 0**. wordlist-enriched.tsv
and silver-candidates.tsv stay absent. The verifier already accepts
"absent OR header-only" for silver-candidates per the Silver file
policy, so `make corpus-verify` keeps passing.

**Take-away**: external-tool integrations should default to graceful
skip with helpful install messages, not hard failure. The pipeline's
core promise (smoke profile passing) is independent of optional
enrichment passes.

### L18 — `cmd/scrapegutenberg` already had `-lang et` support

The plan said "write `cmd/scrapegutenberg-corpus` from a copy of main
repo's scraper, add `-lang et`." When I checked the actual main repo
source, `-lang fi|et` was already a flag. I just needed Makefile
targets that invoke it with the right `-out` paths. No new Go code.

**Take-away**: re-read existing code carefully before assuming a fork
is needed. The plan was based on a hazy memory of the existing
scraper; reality was simpler.

### L19 — Most v2.3 extractors collapse into ~3 patterns

The 11 "remaining" format extractors looked daunting in the plan, but:
- `csv` / `tsv`: stdlib `encoding/csv` with column-picking heuristic
- `leipzig`: `archive/tar` + `gzip` + strip "id\\t" prefix
- `skvr`: `encoding/xml` event-based decoder
- `html` / `riigikogu` / `eeva`: regex tag stripper from extract_epub
- `huggingface`: `bufio.Scanner` over jsonl[.gz], `encoding/json` per line
- `erab`: same as skvr (XML poetry)
- `vrt`, `wiki`: stubbed (deferred — neither is in the active source list)

Pure Go, no Python deps for any of them. Total dispatch table covers
13 formats now plus a graceful `default` warning for anything new.

### L21 — TalTechNLP/err-newsroom is JSONL.gz, not parquet

The plan said HuggingFace ERR newsroom would be parquet. Probing the
actual repo via `huggingface.co/api/datasets/.../tree/main` shows
`train.json.gz` (169 MB), `dev.json.gz` (1.9 MB), `test.json.gz` (1.8 MB).
JSONL.gz is much easier than parquet — `bufio.Scanner` over `gzip.Reader`
with `encoding/json` per line, no parquet library needed.

**Take-away**: when a plan says "this format" for an external dataset,
verify via the platform's API before writing the extractor. HF's API
gives a clean file listing.

### L22 — In-memory aggregator: ~4 KB RSS per sentence

Empirical from FI pilot: 12 GB RSS / 2.8M sentences ≈ 4.3 KB per
sentence. That's much higher than the data structures alone (~300-500
bytes per sentence for the maps + slices). The bulk likely comes from:
- Per-CGO call buffers in parserffi (1.4M calls in phase 2)
- parsecore.Analyze internal allocations (dictionary lookups,
  intermediate sentence structs, glue strings)
- Lemmatizer's loaded FST tables (~137 MB JSON)
- Go runtime + GC overhead (~2x typical)

**Implication**: each 2x corpus growth roughly 2x memory. 10 GB
extracted text → ~50-80 GB RSS, which OOMs typical machines.
SQLite scratch + concurrent workers becomes mandatory above ~3-4 GB
extracted text. Triggering threshold confirmed empirically.

### L24 — Expanded ET pilot results (13 sources, 4 GB extracted)

ET expanded aggregate completed in 11 minutes wall clock. 13 sources
(was 6), ~720 MB extracted text (was ~395 MB), 4 GB extracted line count
~8M lines:
- 4,037,716 unique sentences (was 1,123,769 = 3.6x growth)
- 3,352,756 unique surfaces (was 1,448,978 = 2.3x growth)
- 96M total tokens
- 41,551 ambiguous surfaces (was 13K = 3x growth)
- 0.0018% unresolved (essentially zero — parser still nails it)
- Wordlist: 1.46 GB
- Peak RSS: ~10 GB (vs 8 GB at smaller scale — only modest growth
  thanks to surface dedup)

**Take-away**: surface count growth is sublinear with corpus size. Most
new sentences contain words that were already in the surface set. 2x
new sources only added ~2x surfaces, not 3x as I feared.

This means the 4 KB/sentence empirical from L22 was inflated by sentence
diversity. With more redundant sentences (subtitle dialog, EU legal
boilerplate), each "new sentence" costs less memory because it shares
surfaces with prior sentences. The OOM extrapolation from L22 was too
conservative.

### L26 — Expanded FI pilot results

11 FI sources (skipped opus-wikimatrix, opus-eubookshop, opus-finlex
to stay under 64 GB RSS without swap):
- 5,284,887 unique sentences (was 2.8M → 1.9x growth)
- 2,282,908 unique surfaces (was 1.4M → 1.6x growth)
- 110M total tokens
- 43,039 ambiguous surfaces (was 7K → 6x growth — much richer
  ambiguity signal from EU legal corpus)
- 0.0011% unresolved (24 surfaces unresolved out of 2.3M)
- Wordlist: 828 MB
- mining/parser-disagreements.tsv: **25 MB** of real disagreement signal
- Wall clock: ~10 minutes

The disagreements file is the v2.5 work paying off — 25 MB of
candidates for parser improvement. Each row is a surface where basic
mode and custom mode of the parser pick different lemma+POS. Good
input for hand-labeling gold or refining FST priorities.

### L27 — Parser-disagreements size scales with corpus, not with parser quality

25 MB of disagreements from 1.6M surfaces (vs. ~0 from 47-surface
fixture) doesn't mean the parser got worse — it means the corpus is
diverse enough that basic mode (no FST priority) and custom mode
(FST-aware) hit different choices on more surfaces. Each disagreement
is roughly 100 bytes of TSV; 25 MB / 100 = ~250K disagreement rows.
That's ~10% of all surfaces showing some basic-vs-custom mismatch.

Real number for parser-eval workflows: only the parser_choice
disagreements where downstream evaluation cares (frequency-weighted)
need attention. The mining file gives you the raw set; downstream
mining workflows can join with surface_count_prose to prioritize.

### L28 — Final session 3 disk + cleanliness state

```
localdata/
├── fi-corpus/                         3.6 GB (data + extracted + derived)
│   └── _derived/                      1.9 GB (wordlist, sentences, mining, etc.)
├── et-corpus/                         3.4 GB
│   └── _derived/                      2.4 GB
├── corpus_pipeline/                   7.3 MB (the code + plans + this file)
└── bootstraps/                        1.9 GB (existing code-only tarball)

git status --porcelain (main repo):    no new deltas outside localdata/
```

Started session 3 with 10 untracked design/* files in main repo. Ended
session 3 with the same 10 files. Zero pollution.

### L25 — `-skip` flag is a useful pragmatic tool

Added `-skip slug1,slug2` flag to `cmd/aggregatecorpus`. Lets you
exclude specific sources without removing them from the registry or
filesystem. Useful when:
- A source's data is malformed and you want to aggregate around it
- Memory pressure forces partial runs
- You want to do an A/B comparison (with vs. without source X)

This is a v1 pragmatic — not a substitute for v2.4 SQLite scratch, but
buys time before that refactor is mandatory.

### L29 — Memory hog wasn't sentences, it was per-surface doc-sets

I assumed in-memory aggregator's 12 GB peak came from the sentences map.
Profiling under scratch mode revealed the truth: the memory hog was
`surfaceStats.docsProse map[string]struct{}` — for very-frequent surfaces
like `ja` (1.9M occurrences across 13K pseudo-docs), the docsProse set
held 13K entries. With 3M unique surfaces averaging ~50 doc memberships
each, that's ~150M map entries × ~50 bytes = **7-10 GB just for
doc-counting**.

**Fix**: replace `docsProse map[string]struct{}` with two fields:
`docCountProse int` + `lastDocProse string`. During ingestion, increment
the counter only when docID changes from lastDoc. We don't actually need
the SET of doc IDs — we just need the count.

Same fix for `docsPoetry`. Memory drops from O(surfaces × avg-docs) to
O(surfaces × constant).

**Take-away**: when memory profiling, don't assume the obvious data
structure (sentences) is the hot one. Per-surface auxiliary maps that
look small individually can blow up at scale.

### L30 — Pure-Python omorfi vs system C++ omorfi

The plan said `omorfi-disamb-cmdline`. That's the C++ omorfi binary —
not available via Homebrew on this machine. The Python `pip install
omorfi` provides:
- `omorfi-conllu.py` (analyser CLI)
- `omorfi-tokenise.py`
- `omorfi-disamparsulate.py`
- etc.

Plus the `omorfi` Python module with `Omorfi.analyse(token)` that
returns `Analysis` objects with `.raw` strings like
`[WORD_ID=tulla][UPOS=VERB][VOICE=ACT][...]`.

**Fix**: instead of shelling to `omorfi-disamb-cmdline`, write a Python
batch script (`scripts/omorfi-batch.py`) that:
1. Imports Python omorfi
2. Calls `o.load_analyser(<path>)` once
3. Reads surfaces from stdin, calls `o.analyse()`, parses raw output
4. Writes TSV: surface\tlemma\tUPOS\tfeats\tcount

`cmd/enrichcorpus` invokes via `python3 scripts/omorfi-batch.py` (using
the local `.venv/bin/python` if present).

`omorfi-download.py` populates ~50 MB of `omorfi.*.hfst` model files.
**Caveat**: it dumps them to cwd by default — needs to be run from
`corpus_pipeline/omorfi-data/` (or via OMORFI_HOME env).
Otherwise hfst files end up in worktree root and pollute git status.
Caught and moved to localdata/.

### L31 — estnltk POS labels need UPOS translation

estnltk emits single-letter POS codes (`J`/`V`/`S`/`A`) instead of UD's
three-letter UPOS (`CCONJ`/`VERB`/`NOUN`/`ADJ`). Without translation,
silver-candidates matching (which compares parser_choice POS == external
POS) would be ALWAYS-EMPTY for Estonian.

Fix: `POS_MAP` dict in `scripts/estnltk-batch.py` translates each
estnltk code to the corresponding UPOS before emitting.

**Validation on ET 100-surface slice**: 60 silver candidates emitted
(60% agreement with parser_choice). Top entries: `ja`/CCONJ, `on`/olema/VERB,
`Euroopa`/PROPN, `et`/CCONJ, `ning`/CCONJ — exactly the function-word
agreement we'd expect from independent analyzers.

**Validation on FI 47-surface slice**: omorfi → 26 silver candidates
(55% agreement). Top entries: `asiakkaansa`/asiakas/NOUN, `huonetta`/huone/NOUN,
`hyödyllistä`/hyödyllinen/ADJ, `istui`/istua/VERB, `joessa`/joki/NOUN.
Real Finnish lemmas matching the runtime parser. End-to-end silver
pipeline working.

### L33 — vabamorf is bundled inside estnltk

`pip install vabamorf` fails — vabamorf isn't a separate pypi package.
But `import estnltk.vabamorf.morf` works: it's the underlying C++ FST
that estnltk wraps in its higher-level Text-layer pipeline.

**Implication**: I can call vabamorf directly via:
```python
from estnltk.vabamorf.morf import Vabamorf
morph = Vabamorf()
result = morph.analyze(['joon'])  # → list of dicts with root, ending, lemma, partofspeech, form
```

This is a clean parallel to FI's omorfi-direct path. Both:
- Pure Python wrappers around C++ FSTs
- Lower overhead than higher-level NLP pipelines (parsecore-equivalent)
- Reachable from the local venv

`scripts/vabamorf-batch.py` uses this directly. ET silver candidates
from 100-surface slice: **63** (vs estnltk-batch's 60). Modest win but
cleanly demonstrates ET parity with FI's omorfi adapter.

### L34 — `cmd/enrichcorpus` prefers vabamorf, falls back to estnltk

ET analyzer detection now tries vabamorf first (lower-overhead). If
the `from estnltk.vabamorf.morf import Vabamorf; Vabamorf()` Python
check passes, use `vabamorf-batch.py`. Otherwise fall back to
`estnltk-batch.py`. Both produce the same TSV protocol.

This keeps the dependency footprint the same (pip install estnltk) but
gets us the cleaner direct-FST analyzer for ET. Symmetric with FI's
omorfi (also pip-installed, also direct-FST via Python wrapper).

### L36 — enrichcorpus loads all enriched results before writing

Current Go design: read all surfaces, send them to Python subprocess
via stdin (in goroutine), accumulate responses into `map[string]externalAnalysis`,
write at end. No streaming write.

This means `wordlist-enriched.tsv` shows zero progress until ET
enrichment completes — even though Python is processing surfaces
steadily.

For ~3.35M surfaces at ~14K/sec vabamorf throughput: ~4 minutes Python
time. Go process holds 2.8 GB RSS during accumulation (3.35M map
entries with strings).

**v3 idea**: stream-write enriched.tsv as Python emits results.
Reduces RSS from O(N) to O(1) and gives operators progress visibility.
For now, single-shot write is fine at this scale.

### L39 — FI omorfi vs ET vabamorf: very different agreement rates

Final enrichment numbers:
- ET vabamorf: 1,438,782 silver / 3,352,756 surfaces = **42.9% agreement**
- FI omorfi: 808,600 silver / 7,505,796 surfaces = **10.8% agreement**

The FI rate is much lower despite both being "direct FST analyzer
agrees with parser_choice" mining. Hypotheses:
1. **FI has more morphological complexity**. Finnish has ~15 cases × number ×
   possessive × clitic — each surface has more analysis paths. omorfi
   and parsecore can pick differently from a wider menu.
2. **FI parser favors compound segmentation** (custom mode preference)
   while omorfi's first-analysis often picks the non-compound reading.
3. **FI omorfi's first analysis is its disambiguation choice**, but
   without context (single-surface input), disambiguation defaults to
   most frequent — which may not match parsecore's FST priority order.
4. **POS_MAP differences**: I parse `[UPOS=...]` directly from omorfi's
   raw output. omorfi emits UD-style UPOS like VERB/NOUN/ADJ, but for
   ambiguous-POS forms like "kuin" (CCONJ "as/like" but parsed as
   kuu/NOUN), both analyzers picked the wrong path together → silver
   row with wrong agreement.

**Take-away**: agreement rates aren't a parser-quality metric. They
measure how independently two analyzers reach the same choice on
ambiguous inputs. FI's lower rate doesn't mean parsecore is worse —
it reflects that FI ambiguity is denser. The 808K silver entries that
DID agree are still high-confidence silver-tier data.

### L40 — Wall-clock timings at full FI/ET scale

```
Operation                    FI                ET
fetch (10 sources)           ~2 min            ~30 sec
extract (~3 GB compressed)   ~13 sec           ~13 sec
aggregate (-scratch)         54 min            11 min
enrich (omorfi/vabamorf)     26 min            7 min
verify                       ~2 sec            ~2 sec
─────────────────────────────────────────────────────
Total per language           ~85 min           ~20 min
```

FI is ~4x slower throughout because the corpus is genuinely larger
(15M sentences vs 4M) AND because phase 2 calls parsecore.Analyze
twice (basic + custom) for parser-disagreement mining.

ET vabamorf is ~3.7x faster than FI omorfi per-surface — likely
because the analyzer itself is faster and the per-surface CGo overhead
matters less for ET's fewer surfaces.

### L38 — `UPDATE WHERE id = ?` × 9.5M is way too slow

Phase 4 of the SQLite-scratch aggregator originally ran:
```go
stmt, _ := tx.Prepare(`UPDATE tmp_sentences SET final_id = ? WHERE hash = ?`)
for h, id := range hashToID {
    stmt.Exec(id, h)
}
tx.Commit()
```

For 9.5M sentences this took **>20 minutes** before commit even
started. The bottleneck wasn't SQLite per se — WAL was growing at ~15
MB/sec, but each `Exec` requires a CGo call (~10 µs overhead), so
9.5M × 10µs = ~95 sec just in CGo. Plus per-row WAL frame writes plus
B-tree page splits at this scale.

**Fix**: skip the UPDATE entirely. Keep the `hash → final_id` map in
Go memory (already built for sort + ID assignment). At write-time:
- For sentences.tsv: bulk-load `(hash, text)` rows into a Go map
  (`hashToText`, ~1.5 GB), then iterate the sorted (id, hash) slice
  and emit lines using the map.
- For occurrences.tsv: SELECT FROM tmp_sentence_occurrences ORDER BY
  source/doc/ix, look up final_id from the in-memory map per row, write.
- For example_text in wordlist: same lookups, no SELECT.

Trade-off: ~1.5 GB more RSS at phase 4. With 64 GB host that's fine.

**Take-away**: SQLite + per-row UPDATE in transactions is fast in
isolation but degenerates at 10M scale because WAL frame writes and
CGo overhead compound. Bulk-loading to in-memory + writing from there
is faster when the data fits.

### L37 — Riigikogu scraper + extract_html: pure-Go, no scraping libs

Built `cmd/scraperiigikogu` — ~120 lines pure Go using stdlib
`net/http` + `regexp` for extracting sitting IDs from
`https://stenogrammid.riigikogu.ee/`. Polite 1.5s delay. Saves to
`localdata/et-corpus/riigikogu/raw/<id>.html`. Idempotent (skips
existing pages).

Existing `extract_html` then strips tags. **Caught a real bug there:
the regex stripper preserved CSS/JS content from `<style>`/`<script>`
blocks**, since they don't get caught by the simple
`<[^>]+>` tag-killer (the inner content is plain text from the regex's
view). Fix: pre-strip `<style>...</style>`, `<script>...</script>`,
and HTML comments BEFORE general tag stripping.

Test: 5 Riigikogu sittings → 1.1 MB clean Estonian parliamentary text.

`make scrape-riigikogu RIIGIKOGU_LIMIT=100` for routine use; set
LIMIT=0 for everything the index lists.

### L35 — Run two heavy jobs in parallel without contention

FI aggregate (in-memory + scratch) takes ~4-6 GB RSS in optimized
mode. ET enrichment (Python subprocess + Go reader) takes ~2-4 GB.
Combined: well under 32 GB on a 64 GB box. Parallel execution is fine
as long as neither fully saturates I/O — and SQLite scratch + Python
stdin/stdout aren't disk-bound.

### L32 — Per-source flush is enough; phase 2 doesn't need scratch

Originally I planned full SQLite scratch (sentences + occurrences +
wordlist on disk). Empirically:
- Sentences + occurrences: ~3 GB on disk after FI flush. Big enough to
  matter.
- Wordlist: ~1.4M rows × ~300 bytes = ~450 MB. Fits comfortably.
- Surfaces map (with doc-set fix above): ~3M entries × ~150 bytes = ~450 MB.

So scratch only needs to hold sentences + occurrences. Wordlist can
stay in-memory through phases 2-4. Phase 4 reads sentences via streaming
SELECT cursors (memory O(1) for the dump), but joins with in-memory
wordlistRows.

This reduced complexity vs. the original plan substantially. Keeps most
phase 2/3 logic in Go without SQL queries.

### L23 — opus-opensubtitles / paracrawl deferred from expanded pilot

Fetched: 6 new FI sources + 4 new ET sources (medium-sized) → expanded
extracted text from ~550 MB FI → ~2.4 GB FI; ~395 MB ET → ~720 MB ET.

Skipped (too big for in-memory aggregator without v2.4):
- opus-opensubtitles FI (1.07 GB compressed)
- opus-opensubtitles ET (391 MB compressed)
- opus-paracrawl ET (369 MB compressed)
- hf-err-newsroom ET (169 MB compressed)

These stay in the source registry. They'll process once SQLite
scratch is wired in.

### L20 — Embedded `extract_misc.go` for niche-source delegations

Riigikogu, ERAB, EEVA each have their own format slug in the registry,
but underlying file format is HTML or XML respectively. So I wrote
trivial delegating wrappers in `extract_misc.go` rather than 3 separate
extractor files. Keeps the dispatch table clean while letting future
sessions refine each one if needed (e.g. Riigikogu has structured
speaker metadata worth capturing — that goes in the wrapper later).

### L16 — Top-N surfaces sanity check

FI top: `ja` (1.9M, "and"), `on` (881K, "is"), `oli` (513K, "was"), `ei`
(405K, "no" — multi-analysis VERB+AUX), `että` (403K), `tai` (382K).

ET top: `ja` (1.3M, "and"), `on` (687K, "is"), `või` (478K, "or" —
5-way ambiguity ADV/CCONJ/NOUN/VERB), `ei` (290K), `et` (276K), `mis`
(250K), `ning` (249K), `kui` (220K), `Euroopa` (178K — corpus-skew
toward EU legal text).

Function words dominate as expected. Multi-analysis works correctly
where ambiguity exists.
