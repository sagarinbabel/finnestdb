# Parser Eval Testdata

This directory contains parser evaluation datasets and annotation working files.

Structure:

- `fi/`
  Finnish datasets and notes
- `et/`
  Estonian datasets and notes

Recommended workflow:

1. collect source snippets under `sources/`
2. annotate working files under `drafts/`
3. move reviewed evaluator-ready JSON into `gold/`

Only files in `gold/` should be treated as stable benchmark inputs.
