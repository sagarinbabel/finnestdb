# FinEstDB Product Notes and Roadmap

## Purpose of This Document

This file is no longer a strict “current alpha spec.”

It now serves two purposes:

- preserve what the team thought the product would be at an earlier stage
- record how the plan evolved once parser quality became the main bottleneck

For the **current implementation on `main`**, treat the README as the source of
truth. This document is the historical planning record plus the current forward
roadmap.

## Historical Product Direction

### Earlier product idea

At an earlier planning stage, the intended alpha looked like this:

- upload long Finnish or Estonian texts
- parse them into deck-ready vocabulary
- create decks
- learn and review with FSRS
- support accounts, known-word tracking, and review scheduling
- include richer example sentences and eventually MWE support

That earlier direction assumed the parser would become “good enough” quickly and
that the app could move into learning and account features soon after.

### Why the plan changed

As implementation progressed, it became clear that parser quality is the
critical dependency for almost everything else:

- if lemma resolution is weak, deck quality is weak
- if definitions are missing or wrong, review quality is weak
- if parser behavior is hard to measure, improvements are hard to trust

Because of that, the project focus shifted from “finish the full learning app”
to “build a parser workbench and evaluation loop first.”

## Current State on `main`

The current shipped product surface is:

- parse Finnish or Estonian text
- compare two parser modes: `basic` and `custom`
- inspect sortable results with definitions, grammar labels, token counts, and example sentences
- see parser mode, coverage proxy score, and parse duration

Current parser modes:

- **Basic parser**
  - Rust tokenization and heuristic lemma/POS fallback
  - direct dictionary lookup only

- **Custom parser**
  - the same Rust parser output
  - plus enrichment rules for possessives, compounds, and case suffixes

Current text limit:

- **300,000 Unicode characters**

This is the current product reality. It is intentionally narrower than the
earlier end-to-end language-learning plan.

## Current Main Focus: Parser Quality

The immediate work is to improve the parser in a measurable way.

### 1. Decide how parser improvement is measured

We are currently deciding how to judge whether one parser mode is better than
another.

The current direction is to measure:

- quality against labeled examples
- coverage on real text
- performance on a fixed dataset
- debuggability of parser decisions

That means future parser work should be guided by explicit metrics, not only by
manual spot checks.

### 2. Measure speed and performance

Parser quality is not the only goal.

We also care about:

- parse duration on a fixed benchmark set
- throughput for larger text inputs
- whether improvements increase cost too much for practical use

The goal is not just “better than the stub,” but “better while staying fast
enough to remain interactive and maintainable.”

### 3. Build annotation and evaluation workflows

We will be doing annotation work to create a small, high-value test set.

That work includes:

- selecting representative Finnish and Estonian input
- annotating expected lemma/POS/definition behavior
- defining what counts as a successful output
- creating a repeatable benchmark that can compare parser modes over time

### 4. Create a fast parser test setup

A major current goal is to make parser testing easy and repeatable.

That includes:

- quick local runs
- stable benchmark inputs
- visible comparison between parser modes
- simple regression checking after rule changes

The project should make it easy to answer:

- did this parser change improve quality?
- what kind of errors did it reduce?
- how much did it cost in performance?

## Next Main Focus After Parser Quality

Once parser quality and evaluation are in a strong place, the next major system
to build is a richer lexical knowledge layer.

This may end up looking like:

- a self-improving dictionary
- a lexical knowledge graph
- a provenance-aware lexical database

The exact final term is less important than the goal:

- definitions should become easier to improve
- lexical relationships should become easier to store
- parser outputs should connect to richer lexical knowledge over time

This is the next major platform layer after parser evaluation.

## Planned Omorfi Baseline

We do not currently have Omorfi integrated.

The planned role for Omorfi is:

- provide a stronger Finnish morphology baseline
- give us a benchmark target for the custom parser
- help us compare quality and speed against a more complete analyzer

The intended sequence is:

1. make parser evaluation trustworthy
2. improve the custom parser against those metrics
3. integrate Omorfi as a comparison baseline
4. decide whether Omorfi is only a benchmark, a fallback, or part of production

## Later Product Work

Only after the parser and lexical layer are in good shape should the project
return to broader product features such as:

- user accounts
- known-word tracking
- review scheduling
- dashboard metrics
- learning and review flows

Those features still matter, but they are intentionally downstream of parser
quality.

## Timeline Intent

This document is meant to be a useful record of intended sequence:

1. parser evaluation and benchmarking
2. parser quality improvements
3. richer lexical knowledge layer
4. accounts and learning features

That way, the project can look back later and compare:

- what we originally thought
- what changed
- whether implementation followed the intended order
