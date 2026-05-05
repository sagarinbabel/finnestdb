_Current as of 2026-04-29 — see [CHANGELOG.md](CHANGELOG.md) for revisions._

# Cross-Language Strategy: Finnish and Estonian

FinEstDB ships Finnish and Estonian as first-class languages from the
same codebase. The two languages are closely related but not
interchangeable. This document explains how we let them improve
together without making the parsers behave like clones of each other.

## The Goal

The Finnish and Estonian parsers should:

- share infrastructure that is genuinely shared (pipeline shape,
  evaluation harness, dictionary plumbing, error taxonomy)
- specialize where the languages actually differ (morphology,
  morphophonology, lexicon, partitive vs. partitive-like patterns,
  consonant gradation rules, vowel harmony, etc.)
- be measured against the same kinds of metrics, so a regression in
  one language is comparable to a regression in the other

Improvements in one language should make the other language easier to
improve, but they should never silently change linguistic behavior in
the other.

## What Is Shared

### Pipeline shape

Both languages run through the same parsecore pipeline:

- NFC normalization
- sentence/token segmentation
- morphological analysis
- dictionary resolution
- gloss lookup
- word/sentence aggregation
- structured parse-stage observability

When we add a new stage or observability counter, both languages get
it on the same day.

### Evaluation harness

Both languages use:

- the same `cmd/parsertest` runner
- the same gold-dataset format under `testdata/parser-eval/<lang>/gold`
- the same metric definitions (lemma accuracy, POS accuracy, grammar
  accuracy, full accuracy, resolved coverage, latency percentiles)
- the same baseline-freezing convention under `docs/baselines/`
- `make eval`, `make compare-parsers`, and `make eval-check` cover
  both languages

If a metric only makes sense in one language, it is documented as
language-specific rather than silently dropped from the other.

### External benchmark slots

Each language has a single canonical external benchmark that we
compare our `basic` and `custom` parsers against:

- **Finnish**: Omorfi
- **Estonian**: EstNLTK / Vabamorf

These adapters live behind a common interface so the comparison
reports look identical structurally for FI and ET. The benchmark
itself is not "the answer" — it is a reference signal, the same way
gold datasets are a reference signal.

### Error taxonomy

Parser errors are classified using a shared vocabulary:

- compound segmentation error
- consonant-gradation / morphophonological alternation error
- case/feature tagging error
- unknown lemma / guesser fallback
- MWE boundary error
- tokenization or sentence-split error

When we triage feedback (Track B) or audit gold mismatches (Track A),
we tag findings with the same categories regardless of language. That
lets us see, for example, that "compound segmentation" is the top
error class in both FI and ET, even though the actual splitting rules
are language-specific.

### Dictionary and feedback plumbing

- The lemma/form/gloss schema is the same for both languages.
- Dictionary import metadata records source name, source URL, source version,
  license, attribution, and change notes so EKI/Ekilex data can be imported
  without losing provenance.
- The known-words and parser-feedback subsystems treat language as a
  filter, not as a different data model.
- Admin triage of corrections uses one queue and one workflow, with a
  language column.

## What Is Not Shared

### Morphology rules

We do not copy morphology between languages. Estonian rules live in
`internal/parserules/estonian.go`; Finnish rules live in
`internal/parserules/finnish.go`. When auditing parity, we ask "does
the equivalent phenomenon exist in this language?" and either:

- implement the language-appropriate handling, or
- mark the rule explicitly N/A with a short justification.

Examples of phenomena that look superficially similar but are not the
same rule:

- consonant gradation (FI has it; ET has weakened/limited variants)
- vowel harmony (FI has it; ET does not)
- partitive plural and stem alternations (different in ET)
- compound formation patterns (overlap, but productivity differs)

A rule that "works" in Finnish is never auto-promoted to Estonian. It
is treated as evidence that the *category* is worth investigating in
Estonian, not as a portable rule.

### Lexicon

Each language has its own lexical sources, frequency data, and gold
datasets. There is no shared word list. Cross-language cognates are
not modeled in alpha.

### Disambiguation priors

Frequency priors and any future disambiguation models are trained per
language on per-language corpora. We do not share trained weights.

## How They Improve Together

The mechanism for cross-language improvement is the shared
infrastructure, not shared linguistic content.

1. A real user reports a parser error in language A through the
   feedback loop.
2. The error is classified using the shared taxonomy.
3. Triage and aggregation across both languages reveal which error
   categories matter most overall.
4. We invest in the parts of the pipeline that affect that category
   (e.g., the compound splitter, the morphology analyzer harness, the
   feature tagger) — and the investment is shared.
5. Each language's specific rules are then updated independently
   inside that shared mechanism.

In other words: the parsers learn from each other through which
*problems* are worth solving, not through which *strings* to emit.

## Parity Checklist

When we ship a parser-quality release, both languages must pass:

- gold dataset evaluation against the frozen baseline
- external benchmark comparison (Omorfi for FI, EstNLTK/Vabamorf for
  ET)
- the same `make eval`, `make compare-parsers`, and `make eval-check`
  commands
- comparable manual gold dataset scale and density
- Track B (live accepted-correction rate) at acceptable levels per
  language and per parser mode

A regression in one language is treated the same as a regression in
the other. We do not ship one language while waiting for the other.

## See Also

- [`docs/FEATURES.md`](FEATURES.md) — product framing
- [`docs/baselines/`](baselines/) — frozen evaluation reports
- [`docs/PARSER_EVAL_DATASETS.md`](PARSER_EVAL_DATASETS.md) — gold
  dataset structure
- [`docs/OMORFI_COMPARISON.md`](OMORFI_COMPARISON.md) — Finnish
  external benchmark methodology
- [`TODO.md`](../TODO.md) — execution plan and open work
