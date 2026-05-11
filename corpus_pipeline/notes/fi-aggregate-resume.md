# FI aggregate — resume handoff (Claude restart at 22:38 May 10)

## Process state at handoff
- PID 88631 (go run wrapper) → PID 88638 (compiled binary)
- PPID 1 (launchd) — fully detached from any terminal/Claude
- Elapsed: ~25h12m (started 2026-05-09 21:26)
- **Phase 2 active. Phase 1 complete (budget tripped on opensubtitles at 22:16).**

## What's running
```
go run ./cmd/aggregatecorpus -lang fi \
  -source-order quality \
  -max-user-friendly-sentences-bytes 6GB \
  -max-user-friendly-wordlist-bytes 4GB
```

Working directory: `/Users/sagar/Downloads/projects/finnestdb/corpus_pipeline`

## Live files
- **Log (append-only)**: `/tmp/finnestdb-aggregate-logs/fi-bounded.log`
- **Scratch DB**: `localdata/fi-corpus/_derived/_scratch.db` (26 GB at handoff)

## Current state at handoff (22:38)
- **Phase 2 enrichment**: 5.3M / 14.66M surfaces (~36%), ETA ~40 min
- Heap fluctuating 8-15 GB, sys 16.5 GB, **no swap**
- 14.66M unique surfaces in scratch, 5.3M wordlist rows written so far
- Budget tripped on opus-opensubtitles at 177,920 of 214,994 docs (~82%, +2.20 GB UF)
- Phase 1 final: **14.66M surfaces, uf-bytes-est = 6.01 GB**

## Sources completed in this run
1. fixture (0s)
2. manual (1s)
3. epub (1m33s, 1.04M surfaces)
4. yle-news-2011-2018 (3h24m45s, 4.65M surfaces, 1.60 GB UF)
5. yle-news-2022-2024 (1h44m, 797K surfaces, 493 MB UF)
6. leipzig-fi-news-2020 (3m3s)
7. leipzig-fi-newscrawl-2017 (3m6s)
8. leipzig-fi-wikipedia-2021 (3m13s)
9. wikipedia-fi (12h15m20s, 4M surfaces, 1.15 GB UF) ← the long pole
10. frequency-words-fi (5s)
11. **opus-opensubtitles (in progress, started 15:05)**

UF tally at last source-completion log line: **3.80 GB / 6 GB sentences cap** (after frequency-words-fi at 15:03:44). Opensubtitles has been adding to this in-memory since; budget should trip mid-source.

## Queue remaining after opus-opensubtitles
ALL 14 remaining sources will be marked `budgetSourcesSkipped` in the QA report (budget already tripped). They are: europarl, finlex, ecb, jrc-acquis, emea, eubookshop, bible, wikimatrix, paracrawl, multiparacrawl, hplt, ccmatrix, nllb, multihplt.

## ETA to full completion (from 22:38 handoff)
- Phase 2 enrichment remaining: ~40 min (9.3M more surfaces at 3935/s)
- Phase 3 mining: a few minutes (streams over tmp_wordlist)
- Phase 4 writers: ~25-30 min for ET-similar scale
- **Total ETA: ~75-90 min from handoff = around 23:50-00:10 May 10/11**

## How to verify still running after Claude restart
```sh
ps -ef | grep aggregatecorpus | grep -v grep
ls -lh /Users/sagar/Downloads/projects/finnestdb/localdata/fi-corpus/_derived/_scratch.db
tail -10 /tmp/finnestdb-aggregate-logs/fi-bounded.log
```

## How to re-attach Monitor (after Claude resumes)
```
Monitor:
  command: tail -F /tmp/finnestdb-aggregate-logs/fi-bounded.log 2>&1 | grep -E --line-buffered "phase[1-4]|errBudget|cap.*hit|budget.*reached|done in|Error|FAIL|panic|skipped|partial|completed|Finished|writing|flushing|wikipedia-fi:|opus-|leipzig-|yle-|epub:|tatoeba|sentences=|surfaces="
  persistent: true
  timeout_ms: 3600000
```

## When run finishes
Look for these terminal log lines:
- `[phase1] [budget] sentence UF cap reached at ...` (budget stop, expected)
- `[phase2] starting enrichment of N unique surfaces` (cleanly entered phase 2)
- `[phase3] mining ...` 
- `[phase4] writing wordlist.tsv ...`
- `[phase4] done — total wall clock = ...` (full success)

Then **before celebrating**:
1. `ls -lh localdata/fi-corpus/_derived/*.tsv localdata/fi-corpus/_derived/qa-report.json`
2. `wc -l localdata/fi-corpus/_derived/wordlist*.tsv localdata/fi-corpus/_derived/sentences*.tsv`
3. `jq . localdata/fi-corpus/_derived/qa-report.json | head -40` — check budget_audit fields, partial/skipped lists
4. Delete the now-stale scratch DB: `rm localdata/fi-corpus/_derived/_scratch.db*`
5. Document FI run results in `corpus_pipeline/notes/2026-05-10-fi-full-aggregate.md`

## After FI aggregate completes — next todo
- [ ] Re-enrich FI + ET (via `cmd/enrichcorpus`)
- [ ] Bootstrap tarballs (`make bootstrap-tarball`)

## Pre-existing context the user will have
- `/loop` was running with prompt: "continue checking on the FI scratch aggregate, then proceed with FI enrichment + final delta check + handoff once it's done"
- 4 reviewer fixes (P1a/P1b/P2a/P2b) are committed; ET bounded run validated them
- 1st full ET run (unbounded) and 2nd ET run (bounded) both complete; results in `corpus_pipeline/notes/2026-05-09-et-full-aggregate.md`
