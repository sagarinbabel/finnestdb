# 2026-05-08 Update - FI Aggregate + Enrich Both Complete

End-of-loop verification snapshot. Both languages are in finished pilot state;
nothing is in flight.

## FI - Verified Complete

- aggregate: 14 sources processed, run_end_utc 2026-05-07T21:54:35Z
- 7,505,796 unique surfaces, 7,586,831 wordlist rows (multi-analysis)
- 15,078,951 unique sentences, 18,529,752 occurrences
- 267,130,893 prose tokens
- 41,111 documents
- unresolved rate: 0.0020% (149 surfaces - date-codes like `heina-2003`)
- ambiguous surfaces: 63,151
- enrichment: wordlist-enriched.tsv 432 MB / 7,505,796 rows (1:1 with surfaces)
- silver candidates (omorfi-agreement): 808,600 rows
- mining/parser-disagreements: 1,208,920 rows
- mining/internal-consensus: 47,694 rows
- qa-gate: pilot PASS (utc 2026-05-07T22:23:05Z, no hard failures, no soft warnings)

## ET - Verified Complete

- aggregate: 13 sources processed, run_end_utc 2026-05-07T20:03:03Z
- 3,352,756 unique surfaces, 3,482,701 wordlist rows
- 4,037,716 unique sentences, 4,758,176 occurrences
- 96,090,602 prose tokens
- 16,450 documents
- unresolved rate: 0.0018% (60 surfaces - chemical formulas like `amino-2`)
- ambiguous surfaces: 41,551
- enrichment: wordlist-enriched.tsv 236 MB / 3,352,756 rows
- silver candidates (vabamorf-agreement): 1,438,782 rows
- mining/parser-disagreements: 310,109 rows
- mining/internal-consensus: 41,277 rows
- qa-gate: pilot PASS (utc 2026-05-07T22:23:16Z, no hard failures, no soft warnings)

## Disk Footprint

- localdata/fi-corpus: 9.7 GB (raw 700 MB + derived 6.1 GB + intermediate)
- localdata/et-corpus: 3.6 GB (raw 250 MB + derived 2.6 GB + intermediate)

## Cleanliness Invariant

`git status --porcelain | grep -v "localdata/"` showed only the 10
pre-existing `design/*` untracked files. Zero new deltas from this workstream
outside `localdata/`.

The original local-only report lived at
`localdata/corpus_pipeline/notes/RESULTS-2026-05-08.md`; this tracked report is
the durable run snapshot.

## Outstanding - Registered But Not Fetched

These sources are registered in `cmd/fetchcorpus/sources_{fi,et}.go` but did not
pull in this run.

- FI heavy OPUS: hplt, ccmatrix, nllb, multihplt, paracrawl, multiparacrawl
- FI Yle Kielipankki: yle-news-2022-2024 (892 MB), yle-news-2011-2018 (2.81 GB)
- FI Wikipedia dump (968 MB), Leipzig 1M news/newscrawl/wikipedia, FrequencyWords
- FI big OPUS that errored to 0B: opus-opensubtitles
- ET heavy OPUS: dochplt (5.07 GB), ccmatrix, nllb, hplt, multihplt, multiparacrawl
- ET Wikipedia dump (286 MB), Leipzig 300K-1M news/newscrawl/wikipedia, FrequencyWords
- ET big sources that errored to 0B: opus-opensubtitles, opus-paracrawl, hf-err-newsroom

To pull them, the registry is in place: rerun `make fetch-corpus`, then extract
and aggregate. `extract_vrt.go` is implemented for Yle; `extract_wiki.go` for
Wikipedia dumps is still a stub.

## Hard-Walled - Not Fetchable In V1

- ERAB (CLARIN-academic-gated, not on public web)
- Parsebank (URLs returning 404)
- SKVR XML download (web-only DB, no bulk endpoint discovered)
- Eduskunta (no direct download - API-only)

## Ready-To-Consume Artifacts

- `localdata/{fi,et}-corpus/_derived/wordlist.tsv` - multi-analysis word list
- `localdata/{fi,et}-corpus/_derived/wordlist-enriched.tsv` - plus omorfi/vabamorf
- `localdata/{fi,et}-corpus/_derived/sentences.tsv` - sentence bank
- `localdata/{fi,et}-corpus/_derived/sentence_occurrences.tsv` - provenance
- `localdata/{fi,et}-corpus/_derived/mining/silver-candidates.tsv` - true silver

Both languages pass the same pilot QA gate. Pipeline is in a clean state for
the v1 word-list and sentence-bank features.
