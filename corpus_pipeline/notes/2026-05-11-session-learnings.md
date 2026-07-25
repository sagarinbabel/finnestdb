# Session learnings - bounded FI/ET corpus build (2026-05-09 → 2026-05-11)

## What we shipped

| Artifact | FI | ET |
|---|---:|---:|
| Unique surfaces | 14,661,583 | 14,086,382 |
| Unique sentences | 68,820,256 | 58,781,765 |
| Sentence occurrences | 110,398,854 | 89,267,441 |
| Documents | 1,694,544 | 454,649 |
| Tokens | 763.5M | (similar order) |
| Sentences UF size | 5.80 GB / 6 GB cap | 5.5 GB / 6 GB cap |
| Wordlist UF size | 2.82 GB / 4 GB cap | 2.6 GB / 4 GB cap |
| Silver candidates (external-analyzer agreement) | 1,242,398 (omorfi) | 4,440,232 (vabamorf) |
| Wall clock - aggregate | 27h 30m | 4h 54m |
| Wall clock - enrich | 49 min | ~30 min |

Plus 4 reviewer fixes (P1a/P1b/P2a/P2b), all four exercised at scale and verified to hold.

---

## Session shape

- **Wall-clock span of the work:** 2026-05-09 ~17:50 → 2026-05-11 14:30 (~45 hours)
- **Wall-clock spent in long-running processes:** roughly 40 hours (most of it the FI aggregate)
- **Background processes managed:** 4 (ET bounded aggregate, FI bounded aggregate, FI enrich, ET enrich), plus bootstrap attempts
- **Monitor events received:** ~570 in the FI bounded log alone, plus more across the other runs. Each was a tail-line that the wrapped grep filter promoted to a notification.
- **Claude session restarts survived without breaking the pipeline:** 2 (both clean, because the aggregate process had `PPID 1` after `nohup` and persisted through Claude exits).

I don't have access to my own token-usage counter or wall-clock chat-time meter, so I can't quote exact numbers there. What I can report is what the system gives me: timestamps in log files, file mtimes, and the count of monitor events that crossed the conversation boundary. Treating those as floor estimates: hundreds of in-loop iterations, dominated by `ScheduleWakeup` re-arms during the long phase-1 stretch.

---

## What worked (keep doing)

### Bounded-memory design held under load
The reviewer fixes (P1a–P2b) all earned their place at scale:

- **P1a (encode-then-measure capped TSV writer)** - final file sizes hit `5.80 GB / 6.00 GB` and `2.82 GB / 4.00 GB` with `cap_hit=false` for both languages. The "no truncation" invariant held over 80M+ rows.
- **P1b (tmp_sentence_id + streaming JOIN)** - phase 4 step 3 streamed 68.8M FI sentences without bulk-loading into RAM. Heap peaked at 18.7 GB during the *sort*, then dropped immediately. Without P1b that would have been a ~10 GB hashToText map.
- **P2a (intra-source errBudgetReached)** - tripped cleanly at 6.01 GB inside opus-opensubtitles mid-source, recorded the partial source in `sources_partial_by_budget`, and continued to phase 2.
- **P2b (tmp_surface_order + UPDATE FROM JOIN)** - 14.66M surfaces' example_final_id resolved in one JOIN pass at the end of phase 4 step 5c. Phase 3 stayed sub-minute.

### `PPID 1` detachment via `nohup bash -c 'exec …'`
Every long-running launch used the same incantation. Result: the Claude restart at 22:04 on the FI run was a non-event. Recommended forever for any process the user might want surviving session boundaries.

### Persistent Monitor with a wide grep alternation
The grep filter included terminal-state words (`Error`, `FAIL`, `panic`, `errBudget`, `cap.*hit`, `done in`) alongside progress markers. Silence-as-success would have been the easy bug to write here; the wide filter caught budget trips and step transitions reliably.

### Manual checkpoint queries to scratch SQLite
At one point I mis-attributed WAL growth to phase 4 when we were actually still in phase 1. Lesson: never trust a single signal - `sqlite3 _scratch.db 'SELECT count(*) FROM tmp_wordlist'` is the source of truth when "what phase are we in?" is at stake.

### Documenting handoff state mid-run
The `fi-aggregate-resume.md` note (written right before the user restarted Claude) made the second-session pickup trivial. Doing that as a routine before any planned interruption pays for itself.

---

## What I'd do differently

### Sanity-check the tarball recipe against the user's mental model BEFORE running it
The default `bootstrap-tarball-fi` recipe wrapped the entire `localdata/fi-corpus/` directory (72 GB), which would have produced a 25-30 GB tarball loaded with raw downloads and extracted text the receiver doesn't need. The user caught it after the FI tarball was already 6 GB in. Lesson: when a recipe touches a multi-GB target, do a `tar tzf` or `--exclude=…` dry-run first.

(The lean recipe is now wired in - `BOOTSTRAP_EXCLUDES` in `corpus_pipeline/Makefile`, documented in `docs/CORPUS_PIPELINE.md`. Tarballs are paused; see manifest file for what's preserved.)

### Estimate enrichment throughput from a benchmark, not from the plan
The v1 plan said "12-24 hours per language" for enrichcorpus. The reality at 3400-3950 surfaces/sec was ~50 min for FI and ~30 min for ET - orders of magnitude faster. I should have benchmarked with `-limit 20000` first, which I did the second time around. That's now my default for any "how long will this take" question.

### Log the running UF tally during phase 1
The `uf-bytes-est` was only printed at source completion, so during the multi-hour opus-opensubtitles ingestion I couldn't tell the user how close we were to budget without guessing. A flush-time log line (`[phase1] [running uf-bytes-est=X.XX GB]`) would have made the live state visible.

### Don't burn cache TTL on tight `ScheduleWakeup` intervals when Monitor is firing reliably
Early on I was re-arming at 1200s heartbeats while the Monitor was already firing every ~2-15 min. Per the loop guidance, the safety net should be a *fallback*, not a primary tick. 1800s would have been fine; sometimes lower wasn't.

### Be explicit about "in-memory in-process" vs "on-disk" for tracking values
The user asked at one point "what are the sentences and wordlist user-friendly at right now?" Stale on-disk TSVs from a previous run misled the question. The right answer was "the live tally only lives in the running process's `uf-bytes-est` field; the on-disk file isn't written until phase 4." Worth saying up-front when a query is mid-run.

---

## Documented going-forward

- **Tarball build instructions** - see `corpus_pipeline/docs/CORPUS_PIPELINE.md` (new "Bootstrap tarballs (handoff to another machine)" subsection) for the lean recipe, what ships vs. excludes, sizing, and sha256 sidecar advice.
- **Derived-artifact manifest** - `corpus_pipeline/notes/2026-05-11-derived-artifacts-manifest.md` with paths, sizes, row counts, and sha256 checksums for every FI + ET TSV/JSON, plus per-source `manifest.json`/`documents.jsonl`, plus code-tarball inputs.
- **Per-file sha256 dumps** - `corpus_pipeline/notes/2026-05-11-{fi,et,code}-checksums.txt`.
- **FI bounded run record** - `corpus_pipeline/notes/2026-05-11-fi-full-aggregate.md`.
- **Earlier ET unbounded + bounded runs** - `corpus_pipeline/notes/2026-05-09-et-full-aggregate.md`.

---

## One-line answer to "what to do when tarballs are needed"

```sh
cd corpus_pipeline && make bootstrap-tarball
```

Outputs to `localdata/bootstraps/finnestdb-bootstrap-{code,fi,et}.tgz`. ~25 GB total compressed, ~30-45 min serial (faster with `-j3`). Then write sha256 sidecars: `cd localdata/bootstraps && for f in *.tgz; do shasum -a 256 "$f" > "$f.sha256"; done`. The receiving side verifies against this manifest's row counts + checksums.
