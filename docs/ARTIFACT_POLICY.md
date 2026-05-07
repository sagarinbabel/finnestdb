# Artifact / licensing policy (FST stack)

This repo does **not** commit or embed upstream transducer blobs (examples:
`.vfst`, `.hfstol`, `.hfst`). Those artifacts are often distributed under
copyleft licenses and make the repository's distribution terms ambiguous.

Instead, we allow upstream analysers to be used **offline** to generate
**factual tables** (plain data) that the runtime consumes.

## What is allowed in git

- **Generated factual tables**: JSON/TSV/SQLite dumps that contain derived
  analyses (lemma, UD-features, etc.), plus provenance (source + version +
  generator command).
- **Licenses / attribution text** for upstream sources (for auditability).
- **Generator code** that can reproduce the tables when given local access
  to upstream analysers.

## What is not allowed in git

- Any upstream transducer blobs (`.vfst`, `.hfstol`, `.hfst`) or other binary
  runtime artifacts that directly embed/ship the upstream analyser.

## Regeneration workflow (local-only)

The generator targets are intended for maintainers who have installed or
downloaded upstream analysers locally (kept under `localdata/` or referenced
via env vars). Generated tables are then committed; the blobs are not.

