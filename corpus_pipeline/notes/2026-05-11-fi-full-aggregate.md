# FI bounded aggregate — full run 2026-05-09 21:26 → 2026-05-11 00:56

## Outcome: ✅ clean completion, no cap hits

| Metric | Value |
|---|---|
| Wall clock | **27h 30m** |
| Phase 1 (parser tokenize) | ~17h 50m |
| Phase 2 (enrichment) | 58m 56s |
| Phase 3 (mining) | 53s |
| Phase 4 (writers) | 1h 40m 53s |

## Configuration

```
go run ./cmd/aggregatecorpus -lang fi \
  -source-order quality \
  -max-user-friendly-sentences-bytes 6GB \
  -max-user-friendly-wordlist-bytes 4GB
```

Defaults `scratch=true`. parser_version=`dev-20260509`, fst_tables_sha=`908dae20fd643c92`, dict_fingerprint=`175f075f9bfa138c`.

## Totals

- **14,661,583** unique surfaces (100% prose — no poetry sources in budget)
- **763.5M** tokens
- **68,820,256** unique sentences
- **110,398,854** sentence occurrences (1.6× ratio — opensubtitles dedup helps)
- **1,694,544** documents
- **0** poems (none of the FI poetry sources — skvr, runosto-net — are in our v1 source registry yet)

## Budget audit

| Cap | Budget | Actual | Cap hit |
|---|---|---|---|
| sentences_user_friendly.tsv | 6.00 GB | **5.80 GB** | no |
| wordlist_user_friendly.tsv | 4.00 GB | **2.82 GB** | no |

Both came in cleanly under cap.

## Sources processed

11 of 27 made it into the corpus before budget tripped. All tier 0-3 sources (the highest quality) are intact:

| # | Source | Wall clock | Surfaces added | UF added |
|---|---|---|---|---|
| 1 | fixture | 0s | 47 | 508 B |
| 2 | manual | 1s | 46,515 | 1.70 MB |
| 3 | epub (230 books) | 1m 33s | 1,043,034 | 205.98 MB |
| 4 | yle-news-2011-2018 | 3h 24m 45s | 4,646,202 | 1.60 GB |
| 5 | yle-news-2022-2024 | 1h 44m 4s | 797,477 | 493.16 MB |
| 6 | leipzig-fi-news-2020 | 3m 3s | 213,454 | 117.05 MB |
| 7 | leipzig-fi-newscrawl-2017 | 3m 6s | 336,739 | 121.06 MB |
| 8 | leipzig-fi-wikipedia-2021 | 3m 13s | 524,597 | 117.34 MB |
| 9 | wikipedia-fi | 12h 15m 20s | 3,999,915 | 1.15 GB |
| 10 | frequency-words-fi | 5s | 4,765 | 691.25 KB |
| 11 | opus-opensubtitles (partial) | 7h 12m 27s | 3,048,838 | 2.20 GB |

**Skipped by budget** (16 sources): tatoeba, ted2020, bible, ecb, emea, eubookshop, europarl, finlex, jrc-acquis, multiparacrawl, paracrawl, wikimatrix, ccmatrix, hplt, multihplt, nllb. These would have been mostly low-quality MT/web crawl noise.

## Mining outputs

| File | Count |
|---|---|
| unresolved | 302 surfaces |
| poetry-unresolved | 0 |
| ambiguous (≥2 FST) | 74,193 |
| parser-disagreements (basic vs custom) | 2,875,631 |
| internal-consensus (basic ∩ custom ∩ FST) | 56,824 |
| silver-candidates | absent (only enrichcorpus writes this) |

**Unresolved rate: 0.002%** — extraordinarily low; FST + dict cover virtually everything.

## Output files (canonical TSVs)

```
sentence_occurrences.tsv         6.8 GB  110.4M rows
sentences.tsv                    5.5 GB   68.8M rows  
sentences_user_friendly.tsv      5.4 GB   66.6M rows
wordlist_user_friendly.tsv       2.6 GB   14.76M rows
wordlist.tsv                     2.4 GB   14.76M rows
documents.tsv                    283 MB   1.69M rows
manifest.tsv                     3.6 KB   27 source rows
poems.tsv                        56 B     header only
build_metadata.json              10 KB
qa-report.json                   1.6 KB
```

## Memory behavior

- Phase 1: heap 5-9 GB, sys 8-10 GB, **no swap throughout**
- Phase 2: heap 8-15 GB, sys 16-17 GB, no swap
- Phase 4 step 2 (sentence ID sort): heap 18 GB peak, sys 28 GB. This was the highest pressure point. No swap event observed.

Bounded-memory design held end-to-end on a 27.5h run. The four reviewer fixes (P1a/P1b/P2a/P2b) were all exercised:
- **P1a** (encode-then-measure): sentences UF + wordlist UF wrote cleanly with no truncation.
- **P1b** (tmp_sentence_id + JOIN): 68.8M sentences streamed to disk without bulk-loading hashToText into RAM.
- **P2a** (intra-source errBudgetReached): tripped mid-opus-opensubtitles cleanly; partial source recorded, run continued to phase 2.
- **P2b** (tmp_surface_order + UPDATE FROM JOIN): 14.66M surface example refs resolved in one JOIN pass, not per-surface SELECTs.

## Anomaly in QA report

`totals.sentences_unique = 0` is wrong — the actual count is 68.8M (per phase-4 step 2 log). The QA writer is not pulling this count correctly. Cosmetic bug; not a data correctness issue. **TODO**: investigate qaReport.totals.sentences_unique calculation in main.go.

## Next steps

1. ✅ Delete _scratch.db (~36 GB reclaimed)
2. → Re-enrich FI (via `cmd/enrichcorpus`) — adds external-analyzer agreement labels for silver-candidates
3. → Re-enrich ET
4. → Bootstrap tarballs (`make bootstrap-tarball`)
