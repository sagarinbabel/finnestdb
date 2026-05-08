# Corpus Pipeline Reports

This directory is tracked and append-only. Add one new timestamped report per
meaningful corpus run or schema decision. Do not overwrite historical reports.

Large generated artifacts stay out of Git:

- `localdata/{fi,et}-corpus/_derived/*.tsv`
- scratch SQLite databases
- downloaded corpora and EPUBs
- analyzer caches and Python virtual environments

Use reports here for durable, human-readable run summaries that both Codex and
Claude can read without copying context between tools.
