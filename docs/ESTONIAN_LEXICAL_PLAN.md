# Estonian Lexical And Analyzer Plan

This is the working plan for making Estonian first-class without hand-rolling a
large unsourced lexicon.

## Current Decision

Use two complementary upstream paths:

1. External analyzer baseline: EstNLTK/Vabamorf through the `estnltk` parser
   adapter.
2. Sanctioned lexical data: EKI/Ekilex/Sonaveeb-derived lexical data when we
   can import it with explicit attribution, license text, source version, and
   change notices.

The local custom parser stays useful as a fast deterministic baseline and as
the product parser. External analyzer output is comparison data unless we
explicitly choose to promote a behavior into local rules or dictionary rows.

## Why This Path

- Sonaveeb says its lexical information comes from Ekilex and that Ekilex's
  standard license is CC BY 4.0, with freedom to share/adapt if credit and
  change notices are kept.
- EstNLTK provides Estonian NLP functionality including Vabamorf-backed
  morphological analysis, which makes it the practical analyzer adapter for ET.
- Keeping both behind the existing parser eval loop lets us measure changes
  against `testdata/parser-eval/et/gold/*.json` before promoting anything.

Reference pages:

- Sonaveeb about/license: https://sonaveeb.ee/about?uilang=en
- Ekilex application/API entry point: https://ekilex.ee/login
- EstNLTK repository: https://github.com/estnltk/estnltk

## Implemented Adapter Contract

Parser mode: `estnltk`

Command override:

```bash
export FINNESTDB_ESTNLTK_CMD="/path/to/python /path/to/scripts/estnltk_adapter_example.py"
```

Default discovery:

- `scripts/estnltk_adapter_example.py`
- `.venv-estnltk/bin/python` when created by `make setup-estnltk`
- nearest repo root containing `go.mod`
- executable directory fallback

Subprocess timeout: each call spawns a fresh Python process and reloads
Vabamorf, so cold start alone is roughly 1 second per call. The default
budget is `30s`, overridable with a Go duration string:

```bash
export FINNESTDB_ESTNLTK_TIMEOUT=1m
```

Setup:

```bash
make setup-estnltk
```

Evaluation:

```bash
go run ./cmd/parsertest \
  -dataset testdata/parser-eval/et/gold/et-manual-v1.json \
  -parsers basic,custom,estnltk
```

or:

```bash
make compare-parsers-et
```

The adapter emits the same JSON shape as the Rust FFI parser and Omorfi
adapter. The shared external-analyzer path then:

- records `source=analyzer:estnltk`
- preserves analyzer lemma, POS, and grammar label
- uses direct/custom dictionary overrides only when the analyzer returns an
  unresolved or `X` POS token
- attaches local grammar labels when they agree with analyzer lemma/POS

## Lexical Import Plan

1. Use the EKI/Ekilex API path for the selected dictionaries. Do not scrape
   Sonaveeb pages as the primary data path. Create an API key in the Ekilex
   user profile and run imports with `EKILEX_API_KEY` in the environment.
2. Use importer metadata fields before loading sanctioned EKI data:
   `source_name`, `source_url`, `source_version`, `license`, `attribution`,
   `imported_at`, and `changes_note`. These fields are available in
   `dict_metadata` and in `cmd/importdict` source flags.
3. Import into the existing `lemmas` and `forms` tables only after normalization
   rules are explicit:
   - language must be `ET`
   - POS must map to the app's UPOS-style labels
   - all forms must be lowercased NFC
   - gloss provenance must remain attributable
4. Keep Kaikki and EKI imports deterministic. The importer stores row-level
   `source` and `source_priority`, so overlapping forms/lemmas can be replaced
   intentionally by higher-priority sources rather than overwritten silently.
5. Run Estonian eval before and after each import:

```bash
make import-dict-et
export EKILEX_API_KEY=...
make import-dict-et-ekilex
make compare-parsers-et
```

6. Promote local parser rule changes only when the ET gold report improves with
   no priority regressions.

## Correction Flow

Estonian uses the same correction path as Finnish:

1. A logged-in user submits parse feedback for a token from a persisted
   `parse_sessions` row.
2. The feedback row stores `lang`, `parser`, surface form, original
   lemma/POS/grammar label, proposed lemma/POS/grammar label, and note.
3. Admins triage the shared queue at `/api/admin/parse-feedback`, filtering by
   status and reading the language column.
4. Accepted ET corrections are promoted through one of three paths:
   - add or update a dictionary row with preserved source metadata
   - add a focused Estonian parser rule
   - add the case to `testdata/parser-eval/et/gold/`
5. `make compare-parsers-et` must pass before a correction-driven parser or
   dictionary change is considered safe.

The correction path stays shared because the app contract is shared. Estonian
differs in analyzer and rule data, not in how users report mistakes or how
admins review them.

## Near-Term Work Items

- Expand ET gold datasets with cases where EstNLTK beats the custom parser.
- Freeze a checked-in baseline report comparing `basic`, `custom`, and
  `estnltk`.
- Run an Ekilex smoke import after a user-owned API key is available:

```bash
EKILEX_API_KEY=... go run ./cmd/importdict \
  -lang et \
  -source-key ekilex \
  -source-priority 20 \
  -ekilex-words suuline,maja,minema \
  -ekilex-datasets eki \
  -ekilex-limit 3
```

- Confirm the preferred production dataset code after inspecting
  `/api/datasets` with the same key. The default target is `eki` because the
  Ekilex API docs use it for public Wordweb-visible words.
