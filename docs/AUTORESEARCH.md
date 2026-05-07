# Autoresearch — Automated Rule Tuning

> **Status: parked post-live idea.** This document preserves an experiment for
> after FinEstDB is shipped and live. Do not treat `cmd/autoresearch` as active
> roadmap work, do not prioritize fixes here, and do not block unrelated parser
> or product changes on autoresearch behavior unless a user explicitly asks for
> autoresearch in the current task.

Inspired by Karpathy's [autoresearch](https://github.com/karpathy/autoresearch),
`cmd/autoresearch` is a small loop that mechanically asks the question:
*"what happens to parser accuracy if I change this rule?"*

It mutates one entry in `internal/parserules/` at a time, runs the parser
eval, records the result, and reverts. The output is a JSONL log of every
attempt — a paper trail of which rules carry weight, which are redundant,
and which are ripe for refinement.

## Why this exists

Manual rule tuning has two failure modes:

1. **You change a rule, accuracy drops 0.3pts, and you don't notice** —
   regressions are silent without an automated check.
2. **You spend an evening tuning, run 5 experiments, learn very little** —
   slow iteration loops produce shallow conclusions.

A tireless loop that runs 100 experiments overnight finds insights that 5
careful experiments cannot.

## Quick start for later

```bash
make parser  # build the Rust shared library

# Dry run — list candidate mutations without running them
go run ./cmd/autoresearch -dry-run

# Real run on the smallest gold dataset (a few minutes)
go run ./cmd/autoresearch \
    -dataset testdata/parser-eval/fi/gold/fi-core-v1.json \
    -log     experiments/autoresearch-fi-core.jsonl

# Read the log
cat experiments/autoresearch-fi-core.jsonl | jq -r '
  "iter \(.iteration) line \(.mutation.line): Δ\(.metric)=\(.delta)pts → \(.verdict)"'
```

## How it works

1. **Read** `internal/parserules/finnish.go` from disk and keep a copy of
   the original bytes in memory.
2. **Find candidate lines** — every line that looks like a suffix entry
   (a string literal at the start of the line).
3. **For each candidate:**
   - Comment the line out (`"ssa" → // "ssa"`)
   - Run `cmd/parsertest` against the target gold dataset as a
     subprocess (process boundary keeps a botched mutation from
     poisoning the rest of the run).
   - Compare the chosen metric (default: `lemma_accuracy`) against the
     baseline.
   - Verdict:
     - `kept` if the delta is `≥ -min-delta` (default: any non-regression)
     - `reverted` if the metric dropped more than the threshold
   - Restore the original bytes before the next iteration.
4. **Log** every experiment to a JSONL file.
5. **Restore** the rule file on exit, including on Ctrl-C.

## Mutation strategies

Currently implemented:

- **`comment-out-suffix`** — ablation. Comments out one suffix entry at
  a time. Reveals which rules are actually firing, and which are
  covered by other paths or never trigger.

Parked future ideas (do not implement before go-live unless explicitly asked):

- **`reorder`** — swap two entries in a longest-first list to find
  ordering-sensitive bugs.
- **`introduce`** — insert a candidate suffix from a hand-curated list
  and see whether it lifts grammar coverage.
- **`tighten-min-stem-length`** — vary the `len(stem) < 3` guard in
  `tryCaseSuffixStrip` to find the optimal cut-off.

If this work is explicitly resumed later, add a strategy by extending
`findCandidateLines` and `commentOutLine` with a new mutation type, and
dispatching on a new `-strategy` flag.

## Reading the log

Each line of the JSONL log has this shape:

```json
{
  "iteration": 7,
  "timestamp": "2026-04-28T20:35:35Z",
  "mutation": {
    "strategy": "comment-out-suffix",
    "file": "internal/parserules/finnish.go",
    "line": 38,
    "before": "{\"sta\", \"elative\"}, {\"stä\", \"elative\"},",
    "after":  "// {\"sta\", \"elative\"}, {\"stä\", \"elative\"},"
  },
  "baseline_metric": {"lemma": 0.729, "pos": 0.857, ...},
  "candidate_metric": {"lemma": 0.700, "pos": 0.829, ...},
  "delta": -2.9,
  "metric": "lemma",
  "verdict": "reverted"
}
```

Useful queries:

```bash
# Which mutations were reverted (= rules that matter)?
jq 'select(.verdict == "reverted") | {line: .mutation.line, before: .mutation.before, delta: .delta}' \
   experiments/autoresearch-fi-manual.jsonl

# Which mutations were kept (= rules that may be redundant)?
jq 'select(.verdict == "kept" and .delta == 0) | .mutation.before' \
   experiments/autoresearch-fi-manual.jsonl
```

## Safety

- Each iteration is bracketed by a write-restore pair on the rule file.
- The original file bytes are saved before any mutation is applied.
- A signal handler catches SIGINT/SIGTERM and triggers the deferred
  restore, so Ctrl-C does not leave the file in a mutated state.
- The eval is a subprocess — a panic in the parser does not kill the
  loop or the rule file.
- The default mode is **measure-only**: even a "kept" mutation is
  reverted before the next iteration. We are not greedily improving
  the file in place — we're collecting an ablation map. Future work
  could add a `-greedy` mode that keeps each accepted mutation.

## Future direction

Once the mutation set grows beyond ablations, this becomes a true
search loop. Possible extensions:

- A small genetic algorithm over the rule table
- LLM-proposed rule edits with the eval as the critic
- Joint optimisation across multiple gold datasets

The harness already gives us the foundation: a process-isolated eval,
a structured log, and safe revert semantics.
