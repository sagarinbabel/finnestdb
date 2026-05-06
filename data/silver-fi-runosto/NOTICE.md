# Silver-tier Finnish-poetry corpus — runosto.net

This directory holds Finnish-language poems scraped from runosto.net
for use as a silver-tier eval corpus. Plan C / PR 7 — see
[`docs/CHANGELOG.md`](../../docs/CHANGELOG.md) entry dated 2026-05-07.

## Provenance

- Source: https://runosto.net/ via the WordPress REST API at
  https://runosto.net/wp-json/wp/v2/posts
- Scraped via `cmd/scraperunosto` on 2026-05-07
- Each post is one poem. WP-rendered HTML is stripped to plain text
  (paragraph and `<br/>` boundaries preserved as newlines so the silver
  tagger's sentence splitter behaves).

## Why this corpus

Adversarial domain. Poetry uses archaic case forms, inverted syntax,
abbreviated words, and rare vocabulary. Tagging accuracy here is a
floor; if the parser does well on Gutenberg novels and badly on
runosto verse, that's exactly the diagnostic signal we want.

## License

The site doesn't carry a single corpus license. The vast majority of
listed authors died well over 70 years ago (Aleksis Kivi, Eino Leino,
J. L. Runeberg, traditional anonymous folk verse from the Kanteletar,
Larin Paraske, Otto Manninen) and their work is in the public domain
in Finland (life + 70 years).

Some posts have modern authors with works still under copyright. The
public-domain assumption is per-author; the manifest records the
post `id`, `slug`, and `source_url` so a reviewer can trace any
specific poem back to runosto.net's listing.

If you redistribute these files outside the finnestdb repo, please
verify the public-domain status of each author in your jurisdiction.

## Files

- `manifest.jsonl` — one JSON record per poem. Fields: `id`, `title`,
  `slug`, `tokens`, `source_url`, `fetched_at`, `path`.
- `raw/<id>.txt` — cleaned poem text. UTF-8.

## Reproducing

```bash
go run ./cmd/scraperunosto \
    -target-tokens 200000 \
    -out data/silver-fi-runosto/raw \
    -manifest data/silver-fi-runosto/manifest.jsonl
```

Re-runs are idempotent — poems already in the manifest are skipped.

## Tagging into silver gold

```bash
make silvertag-runosto
```

Same `cmd/silvertag` tool that produces gutenberg-fi-silver-v1, run
against this corpus.
