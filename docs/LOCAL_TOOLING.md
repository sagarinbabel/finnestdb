# Local Tooling Inventory

_Current as of 2026-05-18._

This is the first stop when an agent or maintainer thinks a parser tool,
analyzer, model, table, or local data file is missing.

## Rule For Agents

Before saying "I need `analyser-gt-desc.hfstol`", "omorfi is missing",
"estnltk is missing", or "the FST tables are unavailable", verify the local
state from this repo:

```sh
make doctor
```

If `make doctor` reports a path, use that path. If it reports a legacy path,
either pass that path explicitly to the generator or canonicalize it under
`localdata/`. Do not ask the user to provide a tool until this check has been
run and the relevant lines have been read.

## Canonical Local Paths

| Purpose | Canonical path or command | Notes |
|---|---|---|
| Setup verifier | `make doctor` | Single command for DB, FST tables, analyzer venvs, source analyzers, Ekilex, UD cache, frequency baselines, and parser library. |
| Python NLP venv | `.venv/bin/python` | Shared venv for both `omorfi` and `estnltk`. Do not create `.venv-omorfi/` or `.venv-estnltk/`. |
| Omorfi adapter | `scripts/omorfi_adapter_example.py` | Auto-discovered by eval tooling. |
| EstNLTK adapter | `scripts/estnltk_adapter_example.py` | Auto-discovered by eval tooling. |
| Omorfi HFST model | `~/.cache/omorfi/omorfi.analyse.hfst` | Installed by `make setup-nlp`; repo-local fallback is `.cache/omorfi/omorfi.analyse.hfst`. |
| FI source analyzer | Voikko `mor.vfst` | `make doctor` checks common Homebrew/Linux locations. Pass as `VFST_PATH=...` when regenerating FI tables. |
| ET source analyzer | `localdata/lemmatizer-fi-et/analyser-gt-desc.hfstol` | Canonical local-only location for Giellalt `lang-est` optimized lookup. Pass as `HFSTOL_PATH=...` only if using a noncanonical local copy. |
| FST runtime tables | `localdata/lemmatizer-fi-et/tables/{fi_min.json,et_min.json}` | Loaded by runtime. If present, parser FST lookup works even if the original source analyzer is not present. |
| Dictionary DB | `finnestdb.db` | Large local SQLite database, gitignored. In worktrees, symlink this from the main repo. |
| Runtime artifacts | `localdata/` | Single gitignored root for generated tables, corpora, caches, and local-only data. |
| Corpus pipeline venv | `corpus_pipeline/.venv/` | Separate from the repo-root NLP venv; used only by the tracked corpus pipeline. |

## Known Legacy Locations

The canonical data root is `localdata/`, but older agent worktrees may still
contain local-only analyzer blobs in package-data paths. These are not
tracked by git and must not be committed. If `make doctor` finds one, it is
usable as an input path.

Known fallback patterns:

```text
pkg/lemmatizer-fi-et/data/et/analyser-gt-desc.hfstol
data/lemmatizer-fi-et/analyser-gt-desc.hfstol
.claude/worktrees/*/pkg/lemmatizer-fi-et/data/et/analyser-gt-desc.hfstol
```

If a fallback exists and you want the canonical path:

```sh
mkdir -p localdata/lemmatizer-fi-et
ln -s /absolute/path/to/analyser-gt-desc.hfstol \
  localdata/lemmatizer-fi-et/analyser-gt-desc.hfstol
```

Use a symlink rather than copying when the source file is already on the same
machine. This keeps one physical source of truth and avoids stale blobs.

## Regeneration Commands

Finnish FST runtime table:

```sh
make gen-lemmatizer-tables-fi VFST_PATH=/absolute/path/to/mor.vfst
```

If `localdata/lemmatizer-fi-et/wordlists/fi.txt` is missing, this target
derives it from `finnestdb.db` before writing
`localdata/lemmatizer-fi-et/tables/fi_min.json`. Both outputs are
gitignored bootstrap artifacts.

Estonian FST table, using the canonical HFSTOL path:

```sh
make gen-lemmatizer-tables-et
```

The current ET target still uses the tracked smoke wordlist until a
production ET wordlist is chosen.

Estonian FST table, using a fallback path reported by `make doctor`:

```sh
make gen-lemmatizer-tables-et HFSTOL_PATH=/absolute/path/to/analyser-gt-desc.hfstol
```

External parser baselines:

```sh
make setup-nlp
make compare-parsers
make compare-parsers-et
```

`make setup-nlp` creates `.venv/`, installs both Python analyzers, downloads
the omorfi model to `~/.cache/omorfi/`, and validates both imports.

## Interpreting Missing Pieces

- Missing source analyzer but present `fi_min.json` or `et_min.json`: runtime
  FST lookup can still work; only table regeneration is blocked.
- Missing `fi_min.json` or `et_min.json`: the corresponding runtime FST step
  is disabled, and parsing falls back to dictionary and rule-based paths.
- Missing `omorfi` or `estnltk`: production parsing can still run; parser
  comparison baselines are unavailable until `make setup-nlp` succeeds.
- Missing `finnestdb.db`: local app and dictionary-backed parser work are not
  fully runnable. Use `scripts/setup-local.sh` or the targeted import targets.
