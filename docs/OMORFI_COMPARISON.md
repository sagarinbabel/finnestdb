# Omorfi Comparison Workflow

This page describes how to run a side-by-side accuracy comparison
between our **basic** and **custom** parsers and the **omorfi** FST
analyser. The goal is to know how close (or far) our parser is from
the academic-quality FST baseline, and where the gaps are.

## One-time setup

```bash
make setup-nlp
```

This target:

1. Creates the unified `.venv/` for NLP tooling
2. `pip install`s the `omorfi` and `estnltk` Python packages
3. Downloads the omorfi v0.9.12 HFST model (~26 MB compressed,
   ~120 MB uncompressed) into `~/.cache/omorfi/`

After it completes, the omorfi parser is callable end-to-end with
no environment variables to set. `cmd/parsertest` and the comparison
script auto-detect `.venv/bin/python`, the model, and the bundled
adapter at `scripts/omorfi_adapter_example.py`.

## Run a comparison

```bash
make compare-parsers
```

or invoke the script directly:

```bash
scripts/parser-comparison.sh                   # all gold datasets, stdout
scripts/parser-comparison.sh -o report.md      # write to file
scripts/parser-comparison.sh DS1.json DS2.json # specific datasets
```

The script:

1. Auto-detects whether omorfi is available (looks for
   `~/.cache/omorfi/omorfi.analyse.hfst` or the local repo cache)
2. Runs `cmd/parsertest` against every Finnish gold dataset under
   `testdata/parser-eval/fi/gold/`
3. Pipes the JSON reports through `cmd/parser-compare`, which emits
   a markdown comparison table and head-to-head deltas

## Reading the output

`cmd/parser-compare` produces two tables:

1. **Per-row metrics** - lemma, POS, grammar, full, coverage,
   average per-case latency for each (dataset, parser) cell.
2. **Head-to-head deltas** - every parser after the first is
   shown as `Δ` against the first parser in points.

Look for cells where omorfi is well ahead of custom - those are
specific weaknesses to target with new rules in
`internal/parserules/`.

## Latest captured baseline

See `docs/baselines/2026-04-28-fi-3way-comparison.md` for the
output captured on the day omorfi was first wired up automatically.

## Why we still maintain a custom parser

See `docs/DECISIONS.md` for the rationale (speed, deployability,
licensing, customisability). The comparison here is a quality
benchmark, not a deprecation plan.

## Internals - how the auto-discovery works

| Layer | Lookup |
|---|---|
| `scripts/omorfi_adapter_example.py` | `$OMORFI_ANALYSE_HFST` → `./.cache/omorfi/` → `~/.cache/omorfi/` |
| `internal/evalparsers` (`runExternalOmorfi`) | `$FINNESTDB_OMORFI_CMD` → `.venv/bin/python scripts/omorfi_adapter_example.py` when available → `python3 scripts/omorfi_adapter_example.py` |
| `scripts/parser-comparison.sh` | Sets `parsers=basic,custom,omorfi` if any of those candidates is satisfied |

Override any layer by exporting the matching env var explicitly.
