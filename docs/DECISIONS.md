# Project Decisions and Roadmap

_Current as of 2026-04-29 — see [CHANGELOG.md](CHANGELOG.md) for revisions._

> **Note (2026-04-29):** The consumer-alpha execution plan in
> [`../TODO.md`](../TODO.md) and the product framing in
> [`FEATURES.md`](FEATURES.md) take precedence over older decisions
> recorded here where they conflict. Older entries remain for historical
> context.

This document tracks key architectural decisions, the reasoning behind them, and the
project roadmap. It serves as a journal of how the project evolves and why we made
the choices we did.

---

## Product Vision

FinEstDB is a **JPDB clone for Finnish and Estonian**. The core user flow:

1. **Paste text** — User submits Finnish (or Estonian) text
2. **Parse perfectly** — System extracts lemmas, POS, definitions
3. **Review word list** — User sees all unique words with meanings
4. **Mark known/unknown** — User indicates which words they already know
5. **Create deck** — User saves unknown words as a study deck
6. **SRS study** — Spaced repetition moves words from "learning" to "known"
7. **Loop** — Next time user pastes text, known words are dimmed, focus is on new vocabulary

The value proposition: **pre-mine vocabulary before reading** so the reading experience
is more enjoyable and comprehension is higher.

---

## Decision 1: Build a Custom Parser (Not Use Omorfi Directly)

**Date:** 2026-04-28

### Context

Omorfi (Open Morphology of Finnish) is the gold standard for Finnish morphological
analysis. It uses finite-state transducers (FST) built over 15+ years of linguistic
work. The question: should we use Omorfi directly, or build our own parser?

### Decision

**Build our own custom parser**, using Omorfi as a quality benchmark.

### Reasoning

| Factor | Our Parser | Omorfi |
|--------|-----------|--------|
| **Speed** | Fast (Rust + dictionary lookup) | Slower (FST traversal + Python) |
| **Deployment** | Self-contained Go binary | Requires Python + HFST + .hfst files |
| **Customization** | Add a rule in Go, redeploy | Fork FST project, recompile transducers |
| **Licensing** | Permissive (we control it) | GPL-3.0 (copyleft implications) |
| **Estonian support** | Same architecture, different rules | Would need separate tool (EstNLTK) |

### Trade-off Accepted

We accept that our parser may not match Omorfi's accuracy on edge cases. The goal is:

> "95%+ of Omorfi accuracy at 10x the speed, with full control over rules."

### How We Measure Success

- **Accuracy:** Compare lemma/POS output against gold-annotated test cases
- **Speed:** Tokens per second (must stay fast as rules grow)
- **Coverage:** Percentage of tokens resolved to dictionary entries

### Omorfi's Role

Omorfi is used as:
- A **benchmark** to measure our parser's accuracy against
- A **tool for generating gold annotations** when building test cases
- Not a production runtime dependency

---

## Decision 2: Parser Architecture

**Date:** 2026-04-28

### Current Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│  Rust Parser (parser/src/lib.rs)                                │
│  - NFC normalization                                            │
│  - Sentence splitting                                           │
│  - Tokenization                                                 │
│  - Heuristic POS guessing                                       │
└─────────────────────────────────────────────────────────────────┘
                              │
                              │ FFI (JSON)
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  Go Parse Core (internal/parsecore/parsecore.go)                │
│  - Parser registry (basic, custom, omorfi)                      │
│  - Dictionary resolution                                        │
│  - Enrichment rules (possessive, compound, case suffix)         │
│  - Gloss lookup                                                 │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  SQLite Dictionary (internal/store/dict.go)                     │
│  - forms table: surface form → lemma + POS                      │
│  - lemmas table: lemma + POS → gloss                            │
│  - Source: kaikki.org (Wiktionary-derived)                      │
└─────────────────────────────────────────────────────────────────┘
```

### Parser Modes

| Mode | Description | Use Case |
|------|-------------|----------|
| `basic` | Dictionary lookup only, no enrichment | Speed baseline |
| `custom` | Dictionary + possessive/compound/case rules | Production parser |
| `omorfi` | External adapter for comparison | Evaluation only |

### Enrichment Rules (in `custom` mode)

1. **Possessive suffix stripping** (Finnish)
   - `kirjassani` → strip `-ni` → `kirjassa` → lookup → `kirja`

2. **Compound word splitting** (Finnish + Estonian)
   - `pankkiautomaatti` → `pankki` + `automaatti` → lookup both

3. **Case suffix stripping** (Finnish + Estonian)
   - `kirjassa` → strip `-ssa` → `kirja` + grammar label "inessive"

### Future Consideration: FST

We may eventually need FST-like tooling for **novel/unseen words** that aren't in
the dictionary. For now, the dictionary + heuristic rules cover most cases.

---

## Decision 3: Evaluation-Driven Development

**Date:** 2026-04-28

### Approach

Parser improvements are driven by **measured evaluation** against gold test cases,
not intuition.

### Infrastructure Built

- **Gold datasets:** `testdata/parser-eval/fi/gold/fi-manual-v1.json` (22 cases)
- **Eval CLI:** `cmd/parsertest` — runs parsers against datasets, outputs accuracy metrics
- **Metrics:** Lemma accuracy, POS accuracy, grammar label accuracy, coverage, timing

### Evaluation Workflow

```
1. Run eval:  go run ./cmd/parsertest -dataset fi-gold.json -parsers custom
2. Review failures: Which tokens did we get wrong?
3. Add rule: Fix the failure pattern
4. Re-run eval: Confirm improvement, no regressions
```

### Future: Automatic Improvement

Inspired by [karpathy/autoresearch](https://github.com/karpathy/autoresearch), we plan
to build an automated improvement loop:

```
1. Agent modifies parser rules
2. Run eval automatically
3. If accuracy improves: commit and continue
4. If accuracy drops: revert and try different approach
5. Repeat overnight → 100 experiments vs 5 manual attempts
```

This requires:
- Larger gold dataset (100+ sentences, currently 22)
- Consolidated rules in one file (clear edit scope for agent)
- Automated git workflow

---

## Project Roadmap

### Track A: Core Product (User-Facing)

| Phase | Work | Status |
|-------|------|--------|
| A1 | Parse Experience — results table polish, coverage gauge | Planned |
| A2 | Deck Creation — "Save as Deck" CTA from results | Planned |
| A3 | Known Words — mark known/ignored in results table | Planned |
| A4 | Navigation Shell — nav bar, dark theme alignment | Planned |
| A5 | SRS Core — review queue, card scheduling, session UI | Planned |
| A6 | Known Words Loop — SRS → known list → dims in future parses | Planned |
| A7 | Import Known Words — upload CSV of already-known vocabulary | Planned |

### Track B: Parser Quality (Development Infrastructure)

| Phase | Work | Status |
|-------|------|--------|
| B1 | Gold Data Expansion — 100+ annotated Finnish sentences | Planned |
| B2 | Baseline Benchmark — record current accuracy/speed | Planned |
| B3 | Rule Consolidation — all rules in one file | Planned |
| B4 | Omorfi Comparison — side-by-side accuracy measurement | Planned |
| B5 | Auto-Improvement Loop — autoresearch-style experiments | Planned |

### Track C: Estonian (Parallel Path)

| Phase | Work | Status |
|-------|------|--------|
| C1 | Estonian Gold Data — expand from 1 to 50+ cases | Planned |
| C2 | Estonian Dictionary — verify kaikki.org coverage | Planned |
| C3 | Estonian Rules — case suffixes, compounds | Planned |

### Execution Order

```
Priority 1: A1, A4      (UI foundation)
Priority 2: B1-B5       (Parser quality infrastructure)
Priority 3: C1, C2, C3  (Estonian support)
Priority 4: A2, A3      (Deck + known words)
Priority 5: A5-A7       (Full SRS loop)
```

---

## Open Questions

1. **FST for novel words:** At what accuracy level do we need FST-like morphological
   analysis for unseen words? Current heuristics may plateau at ~95%.

2. **Gold data source:** Should we use Omorfi to generate candidate annotations, then
   human-verify? Or fully manual annotation?

3. **Auto-improvement scope:** Which files should the agent be allowed to modify?
   Just rules? Or also the Rust tokenizer?

---

## Changelog

| Date | Change |
|------|--------|
| 2026-04-28 | Initial decisions documented: custom parser rationale, architecture, evaluation approach, roadmap |
