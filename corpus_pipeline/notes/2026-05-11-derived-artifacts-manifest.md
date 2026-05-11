# Derived artifacts manifest — 2026-05-11

Bootstrap tarballs were intentionally **not built** this session — the user
requested we leave the derived artifacts in place and record exact
paths/sizes/checksums/metadata so tarballs can be built later on demand.

The lean tarball recipe is already wired in `corpus_pipeline/Makefile`
(targets `bootstrap-tarball-fi`, `bootstrap-tarball-et`, `bootstrap-tarball-code`)
with `BOOTSTRAP_EXCLUDES` excluding raw/, text.txt, scratch DB/WAL/SHM,
per-book/, decks/, poems.jsonl, and .DS_Store. To build later:

```sh
cd corpus_pipeline && make bootstrap-tarball
# or just one:
cd corpus_pipeline && make bootstrap-tarball-fi
```

Outputs go to `localdata/bootstraps/finnestdb-bootstrap-{code,fi,et}.tgz`.
The directory was emptied at the end of this session.

---

## Run identity

| Field | FI | ET |
|---|---|---|
| parser_version | `dev-20260509` | `dev-20260509` |
| fst_tables_sha | `908dae20fd643c92` | (same FST table file) |
| dict_fingerprint | `175f075f9bfa138c` | `9781b1d6890b9144` (different — ET ekilex import) |
| run_start_utc | 2026-05-09T18:27:26Z | 2026-05-09T17:51:46Z |
| run_end_utc | 2026-05-10T21:56:54Z | 2026-05-09T18:18:43Z |
| budget mode | `quality`-ordered, 6 GB/4 GB caps | `quality`-ordered, 6 GB/4 GB caps |

---

## FI derived artifacts — `/Users/sagar/Downloads/projects/finnestdb/localdata/fi-corpus/_derived/`

| File | Size | Rows | sha256 |
|---|---:|---:|---|
| sentences.tsv | 5.5 GB | 68,820,256 | `ebea2cc867a10d8c25c507570a0d8c190820726e18be144dcbfd34ef5b9ebe13` |
| sentences_user_friendly.tsv | 5.4 GB | 66,591,173 | `9df4f39ca173385d0d91f7f5a35e87502f7a347574dece0556c94bade135433d` |
| sentence_occurrences.tsv | 6.8 GB | 110,398,854 | `71abd95a1bbcc2c8bde03525a1169386643cf1b2b8ebf77b3794ec87441e2503` |
| wordlist.tsv | 2.4 GB | 14,756,412 | `da564c11bb86f92642ac83fef779fc96fb34c8ca9d3bb57ba4178d7e22cdb476` |
| wordlist_user_friendly.tsv | 2.6 GB | 14,756,412 | `5da87a3f709b7b1904b6d403021ccebb6d56e15eeeb2f1d82d7532357e6e043f` |
| wordlist-enriched.tsv | 786 MB | 14,661,583 | `26fa6875a0c8b316868efb3dab9ad36a0046b13a8865fc6214dec624d30a2d5c` |
| documents.tsv | 283 MB | 1,694,544 | `d4a0e5e88fca7440771bbdd66319e4d0d477a2c7d1e068ad6553f5b7f6829db4` |
| manifest.tsv | 3.6 KB | 27 | `7f0c2c84b6af740fd393f44923d24808ec7b28509b8e2186f6039418add12a09` |
| poems.tsv | 56 B | 0 (header only) | `1d37c5bb3df6f415cc53a8dab110a889a3a0c99eeb7e697cd7b955684bd44a81` |
| qa-report.json | 1.6 KB | — | `3dc3a247ecf18bb7266680b9c04349de51ec217e48cf604a28bc32f003217fd2` |
| build_metadata.json | 10 KB | — | `7c5af76bbbecbec0a997a843ea24be2081b85dc6e15d2fea492271f40b087daf` |
| qa-gate.json | 363 B | — | `a1a9b9039beedaa9d7729c18db1a7f717d6bbb2f662c963ec10ccec540dafd90` |

### FI mining outputs — `/_derived/mining/`

| File | Size | Rows | sha256 |
|---|---:|---:|---|
| silver-candidates.tsv | 44 MB | 1,242,398 | `17246646a53e9a3dab633e74bfd46928d3ee7e45168d0eef13ce349b8df607af` |
| parser-disagreements.tsv | 179 MB | 2,875,631 | `87efe0224d53cb697207e0ec4ddec1fdd6f5be271a34532ec4064d548ac43678` |
| internal-consensus-candidates.tsv | 3.4 MB | 56,824 | `db0439b89dff17580ecc0998e08df5af7dbfab506b8890c125a963cc7a2adbbe` |
| high-frequency-ambiguous.tsv | 1.5 MB | 74,193 | `c9e66398a810af5aca60f37fb8d30c996f98a56fc1005e8ccabc609b025a2379` |
| unresolved.tsv | 4.1 KB | 302 | `1b13a972e0bdc6871753046ab440fb7c4ebb3112bd7844b796c0dfd6c039a3ff` |
| poetry-unresolved.tsv | 69 B | 0 (header only) | `41930f8b81050040b394991b0e2e0bb6ab1430051b4d5a0b297e5c7f9b3d829a` |

### FI per-source provenance (kept for documents.tsv lookups)

Per `/_derived/cache/`, `localdata/fi-corpus/*/manifest.json` (27 files) and
`localdata/fi-corpus/*/documents.jsonl` (27 files). Full sha256 list in
`/tmp/fi-checksums.txt` — sample below for the largest:

| File | Size | sha256 |
|---|---:|---|
| yle-news-2011-2018/documents.jsonl | 142 MB | `888514b3c280aaa75b7a0c9e6ae35d84adac2090856672ddd8af4b337092daad` |
| yle-news-2022-2024/documents.jsonl | 34 MB | `c731018b5524bd8ed7abb355de59b93a74e364a1d6fce8002864e80f5b8e3113` |
| opus-paracrawl/documents.jsonl | 11 MB | `4f463155d7254a9968334f8823736e64bd7373b37cdb41ac5e94089392fad980` |

Total FI per-source manifest+documents.jsonl: ~250 MB.

---

## ET derived artifacts — `/Users/sagar/Downloads/projects/finnestdb/localdata/et-corpus/_derived/`

| File | Size | Rows | sha256 |
|---|---:|---:|---|
| sentences.tsv | 5.8 GB | 58,781,765 | `8e4e623ba8fb02065c209bec5571a14a3297c7da7fd14b333a146de1f0d6f2e6` |
| sentences_user_friendly.tsv | 5.5 GB | 57,132,206 | `5e18fb046750026cc39af65fc0a4a534efbfe897d6f78bd9eb408fc962e43c55` |
| sentence_occurrences.tsv | 5.3 GB | 89,267,441 | `f88b7df9c561fa8c220a1a89e3c76e26ece1ea6ef2cee2f487d86b2e709352f2` |
| wordlist.tsv | 2.4 GB | 14,086,382 | `91b717730efc933ae3c102579d99d6582f5f103db178703d18447b0e353a643b` |
| wordlist_user_friendly.tsv | 2.6 GB | 14,086,382 | `8298c046136b6fb7db2c918237c66a51e8d95e09ce268d1752d7f3ea515ed612` |
| wordlist-enriched.tsv | 988 MB | 13,894,841 | `ac0d82e57cfcfc9de6044e8414f68b2ec8cd939f7142f05ad6b20c5d3d2d1676` |
| documents.tsv | 70 MB | 454,649 | `87c840e04bc76249f7aa750e71a52f691c4f781674885030ce92e8de61470583` |
| manifest.tsv | 3.3 KB | 25 | `becee260696720efd49bd20ff31926048b99b5920f328d12c2797303c6b7ac28` |
| poems.tsv | 56 B | 0 (header only) | `1d37c5bb3df6f415cc53a8dab110a889a3a0c99eeb7e697cd7b955684bd44a81` |
| qa-report.json | 1.4 KB | — | `8c4f418498113cb1299ffde105231577242dd8985547c7282896dcfa53e0a31a` |
| build_metadata.json | 9.3 KB | — | `9781b1d6890b9144a1d88499ece599b8493f7e87f9d3e480d8139682c4bc84f2` |
| qa-gate.json | 364 B | — | `207717c7ebf388ebb925688c1c88a43c3741c99530151e733de4b4d3c975d511` |

### ET mining outputs — `/_derived/mining/`

| File | Size | Rows | sha256 |
|---|---:|---:|---|
| silver-candidates.tsv | 172 MB | 4,440,232 | `60c978c32ac1cedb06d2767399e0ba8a6a230639525647c184876285fb1b4fc2` |
| parser-disagreements.tsv | 79 MB | 1,375,878 | `466c7c023d885223d89b9bf0b53a33cb203f3a0ea90e244a8de3cc5d68ec0caa` |
| internal-consensus-candidates.tsv | 3.8 MB | 58,097 | `1d3699f7c1027a5b3992014e4cb78647be6429b895560f2f61d53586adb920cf` |
| high-frequency-ambiguous.tsv | 1.3 MB | 64,987 | `ac6392300d7edbb3552bf973d603d0eba9faeab8a70e2240b309ab67919ddd43` |
| unresolved.tsv | 8 KB | 517 | `dd384e451e3cd1dd253514de18627926021e80ec21b6c20600e645c78038d226` |
| poetry-unresolved.tsv | 69 B | 0 (header only) | `41930f8b81050040b394991b0e2e0bb6ab1430051b4d5a0b297e5c7f9b3d829a` |

---

## Code-side artifacts (for `bootstrap-tarball-code`)

| File | Size | sha256 |
|---|---:|---|
| finnestdb.db | 4.9 GB | `20de4cabd02e676a52b92a76c26641e39c312bde5f751eb03038794cf57455fb` |
| localdata/lemmatizer-fi-et/tables/fi_min.json | 12 MB (approx) | `908dae20fd643c92608b49c7b1ecd1d06208d05e6823512ec474505ef05376e3` |
| localdata/lemmatizer-fi-et/tables/et_min.json | 8 MB (approx) | `76c53ac7dc18ab07145c10ac420209322632c0b1cdeb7e55f5166df918c17b86` |

Other code-tarball inputs (file lists in `/tmp/code-checksums.txt`):
- `localdata/lemmatizer-fi-et/` — 131 MB total
- `localdata/ekilex/` — 1.3 GB total (40+ TSV form/lemma files)
- `localdata/kaikki/` — 237 MB total
- `corpus_pipeline/` — Go code, scripts, fixtures (excluding .venv)

A `code` tarball was started but **deleted** when we paused (510 MB before
cancellation). On a re-run, expect ~2 GB compressed.

---

## Estimated tarball sizes on re-run (with lean excludes)

| Tarball | Source size | Estimated compressed |
|---|---:|---:|
| code | ~6.5 GB | ~2 GB |
| fi (lean) | ~24 GB | ~10-15 GB |
| et (lean) | ~22 GB | ~10-12 GB |
| **Total** | **~52 GB** | **~25 GB** |

vs. the original "include everything" recipe which would have been
~50-80 GB compressed. The lean recipe is the canonical going-forward
default.

---

## What gets EXCLUDED by the lean recipe (intentional)

These are large but reproducible from source on a fresh machine:

- `_derived/_scratch.db`, `_scratch.db-wal`, `_scratch.db-shm` (deleted after each run; could be 30+ GB during a run)
- `_derived/cache/` (parser cache — auto-rebuilds; ~few hundred MB)
- `<source>/raw/` — original downloads (50+ GB on FI side; re-fetchable via `cmd/fetchcorpus`)
- `<source>/text.txt` — extracted plain text (regeneratable from raw via `cmd/extractcorpus`)
- `<source>/per-book/` (epub-specific)
- `<source>/decks/` (epub-specific)
- `<source>/poems.jsonl` (poetry sources — empty for FI/ET v1)
- `.DS_Store`

---

## Live working copy paths (for app integration)

App developers wanting to consume the corpus *without* a tarball can point
at these paths directly (file:// or sqlite imports):

```
/Users/sagar/Downloads/projects/finnestdb/finnestdb.db
/Users/sagar/Downloads/projects/finnestdb/localdata/fi-corpus/_derived/wordlist.tsv
/Users/sagar/Downloads/projects/finnestdb/localdata/fi-corpus/_derived/sentences.tsv
/Users/sagar/Downloads/projects/finnestdb/localdata/fi-corpus/_derived/sentence_occurrences.tsv
/Users/sagar/Downloads/projects/finnestdb/localdata/fi-corpus/_derived/wordlist-enriched.tsv
/Users/sagar/Downloads/projects/finnestdb/localdata/fi-corpus/_derived/mining/silver-candidates.tsv
… (and the ET equivalents under et-corpus/)
```

---

## Verification command (when tarballs are eventually built)

To re-verify checksums against this manifest after extracting a tarball:

```sh
cd /path/to/extracted
shasum -a 256 -c <(grep ^[0-9a-f] /path/to/this/notes/2026-05-11-derived-artifacts-manifest.md)
```

Or pre-compute and store sha256 sidecars beside each .tgz at build time —
recommended for handoff.

---

## Full per-file checksum dumps

Verbatim sha256 output is preserved at:

- `/tmp/fi-checksums.txt`
- `/tmp/et-checksums.txt`
- `/tmp/code-checksums.txt`

These are session-temp; if you need them durable, move into `notes/`.
