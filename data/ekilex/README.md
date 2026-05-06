# Ekilex Data Snapshots

`eki-public-words-2026-et.jsonl` is a compact, keyless snapshot derived from
Ekilex `/api/public_word/eki` for Estonian public headwords in EKI
ühendsõnastik 2026.

The snapshot intentionally contains only the fields needed for a reproducible
local import:

- `word_id`
- `lemma`
- `lang`
- `source_dataset`
- `morph_exists`

It does not contain the Ekilex API key. Import it into the local SQLite
dictionary with:

```bash
make import-ekilex-et
```

The importer preserves existing dictionary rows and only adds missing direct
headword lookups with POS `X`.
