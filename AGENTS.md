# Agent Instructions

These instructions are for LLM agents working in this repository.

## Current Focus

FinEstDB is pre-go-live. Prioritize work that helps ship and operate the
language-learning app:

- parser accuracy and dictionary-entry attachment
- learner flows: inspect, decks, review, known words, parser feedback
- production safety: auth/session hardening, abuse controls, data retention
- evaluation, baselines, and regression checks for parser changes

## Autoresearch Is Parked

`cmd/autoresearch` and `docs/AUTORESEARCH.md` are a parked idea for after the
app is shipped and live. Treat all autoresearch references as "future ideas to
remember", not current implementation work.

Do not focus on autoresearch, fix it, expand it, review PRs around it, or block
unrelated work because of it unless the user explicitly asks for autoresearch in
the current turn.

If a PR touches `cmd/autoresearch` incidentally, review only for compile/test
breakage. Do not make it a product-quality requirement.

## Python NLP tools (omorfi + estnltk)

A **single shared venv** lives at `.venv/` in the project root. It contains
both `omorfi` (Finnish morphological analyzer) and `estnltk` (Estonian
Vabamorf-backed analyzer). Never create separate `.venv-omorfi/` or
`.venv-estnltk/` directories — those are legacy names kept only as fallback
lookups in the Go code and scripts.

### Setup

```bash
make setup-nlp          # creates .venv/, installs omorfi + estnltk,
                        # downloads HFST models to ~/.cache/omorfi/
```

`make setup-omorfi` and `make setup-estnltk` are aliases for `make setup-nlp`.

### How discovery works

All Go code (`evalparsers.go`, `enrichgoldfeats`, `doctor`) and shell scripts
(`parser-comparison.sh`, `parser-comparison-et.sh`) auto-discover
`.venv/bin/python` at the repo root. No env-var exports are needed after
`make setup-nlp`. Override with `FINNESTDB_OMORFI_CMD` or
`FINNESTDB_ESTNLTK_CMD` only if you need a non-standard setup.

### In worktrees

The `.venv/` lives in the main repo root, not in worktrees. Symlink it in:

```bash
ln -s /path/to/finnestdb/.venv .venv
```

Same for `finnestdb.db` if the worktree needs the database.

### Do not

- Create per-tool venvs (`.venv-omorfi/`, `.venv-estnltk/`).
- Install omorfi or estnltk into system Python or any other venv.
- Download omorfi HFST models anywhere other than `~/.cache/omorfi/`.

## Build

```bash
make parser             # builds the Rust FFI tokenizer + Go binary
make setup-nlp          # one-time: Python NLP tools
make compare-parsers    # FI eval (needs setup-nlp + finnestdb.db)
make compare-parsers-et # ET eval (needs setup-nlp + finnestdb.db)
```

## Large corpus pipeline guardrails

For scraping, extraction, aggregation, and corpus-pipeline work, use the
`large-corpus-pipelines` Codex skill if available. The durable lessons from the
full ET/FI corpus work are:

- Keep fetch, extract, aggregate, and publish as separate stages.
- Reuse existing `localdata/{lang}-corpus/<source>/text.txt` extraction outputs
  unless extraction itself is wrong. Do not refetch or re-extract just because
  aggregation logic changed.
- Stream multi-GB inputs. Do not use whole-file `ReadFile`/split patterns for
  corpus text.
- Use scratch storage for high-volume intermediate state, and keep memory
  bounded with explicit flush thresholds.
- Budget final learner artifacts (`sentences_user_friendly.tsv`,
  `wordlist_user_friendly.tsv`), not raw source text, scratch DBs, WAL files,
  or occurrence tables.
- Pair byte budgets with quality-ordered source ingestion. Deduped rows can
  still be near-duplicate or low-value.
- Enforce TSV caps at row boundaries using exact encoded bytes. Never truncate
  a TSV mid-row to fit a budget.
- Log long phases, source progress, flushes, heap/sys memory, scratch/WAL size,
  budget estimates, and cap events before the job becomes hard to diagnose.
- Record source order, consumed/skipped/partial sources, budgets, actual output
  sizes, fingerprints, and phase durations in final metadata.

## Key paths

| Path | What |
|------|------|
| `.venv/` | Unified Python venv (omorfi + estnltk) |
| `~/.cache/omorfi/` | HFST model files for omorfi |
| `finnestdb.db` | 5+ GB SQLite dictionary database |
| `localdata/` | Gitignored runtime artifacts, corpora, tables |
| `corpus_pipeline/` | Tracked pipeline source (has its own `.venv/` — separate) |
