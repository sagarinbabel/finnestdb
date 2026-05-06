# Silver-tier Finnish corpus — Project Gutenberg

This directory holds Finnish-language texts scraped from Project
Gutenberg (https://www.gutenberg.org/) for use as a silver-tier eval
corpus. Plan C / PR 3 — see [`docs/CHANGELOG.md`](../../docs/CHANGELOG.md)
entry dated 2026-05-07.

## Provenance

- Source: https://www.gutenberg.org/ebooks/search/?query=l.fi
  ("ebooks in language: Finnish")
- Scraped via `cmd/scrapegutenberg` on 2026-05-07
- Each book's `*** START OF` … `*** END OF` body is preserved; the
  Project Gutenberg trademark / boilerplate header and footer are
  stripped before saving.

## License

Project Gutenberg ebooks in this directory are public-domain works in
the United States. Each individual `<id>.txt` was originally distributed
under the Project Gutenberg License, which restricts redistribution
*with the Project Gutenberg trademark or trademark-bearing header
intact*. Stripping the boilerplate is permitted; the resulting plain
text body is public-domain.

If you redistribute these files outside the finnestdb repo, please
attribute the source URL recorded in `manifest.jsonl` and confirm the
public-domain status in your jurisdiction (the books are PD in the US;
some are still under copyright in EU jurisdictions where authors died
fewer than 70 years ago).

## Files

- `manifest.jsonl` — one JSON record per book. Fields:
  `id`, `title`, `author`, `language`, `tokens` (whitespace count),
  `source_url`, `encoding`, `fetched_at`, `path`.
- `raw/<id>.txt` — cleaned plain-text body. UTF-8.

## Reproducing

```bash
go run ./cmd/scrapegutenberg \
    -lang fi \
    -target-tokens 500000 \
    -out data/silver-fi/raw \
    -manifest data/silver-fi/manifest.jsonl
```

Re-runs are idempotent — books already in the manifest are skipped.

## What this is *not*

This is the **raw** silver corpus. Token-level morphological annotations
(silver gold) are produced by a separate pipeline that runs
Voikko + Omorfi over each book and keeps only tokens where both
analyzers agree. That pipeline lands in Plan C / PR 4.
