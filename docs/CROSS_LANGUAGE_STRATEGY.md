_Current as of 2026-07-03 — see [CHANGELOG.md](CHANGELOG.md) for revisions._

# Cross-Language Strategy: Finnish and Estonian

FinnEst ships Finnish and Estonian as equal-status, first-class languages from
the same codebase. The two languages are closely related but not
interchangeable. This document explains how we let them improve together without
making the parsers behave like clones of each other.

Public alpha must not present either language as experimental or secondary. If
one language is weaker in a concrete area, we track the specific asymmetry and
fix or classify it; we do not turn it into a broad product-status downgrade.

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

## Equal-Status Alpha Gate

Before public alpha, run the parity audit journey-first. Walk the same FI and
ET learner/admin paths, then attach parser/data/test/artifact metrics under
each step. The audit should cover:

- anonymous paste -> parse -> list -> explore
- signed-in Inspect
- embedded catalog coverage
- deck save/add-to-deck
- review
- known-word import
- parser feedback
- admin triage/quarantine
- parser/eval observability
- browser/regression tests
- production dictionary/FST/corpus artifacts

Classify every asymmetry as:

- **alpha-blocking**: breaks the equal-status learner journey and must be fixed;
- **language-specific**: valid because FI and ET differ linguistically or in
  licensed source availability; or
- **post-alpha**: visible internally but not a public product-status gap.

Sequencing can still be pragmatic. For example, a Finnish-first ambiguity eval
slice is acceptable when Finnish review is immediately available, but it must be
followed by ET parity cases before product UI decisions imply one language is
less supported.

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

## Measurable Divergences

_Added 2026-05-07._

This section records empirical observations about where Finnish and
Estonian diverge in measurable ways. These are not novel findings in
linguistics — register variation in token-frequency distributions and
the influence of inflectional richness on Zipfian decay are
well-studied. They are recorded here because (a) the specific
measurements on our specific corpora are useful project artifacts, and
(b) the observations have direct product implications for comprehension
prediction and deck construction.

### Register dominates language: top-N inflected-form coverage

Top-N inflected-form coverage of running text varies far more by
register than by language. Coverage curves measured 2026-05-07 against
public corpora (see [`docs/FREQUENCY_BASELINES.md`](FREQUENCY_BASELINES.md) for
methodology, license attribution, and reproduction recipe; bulk files
under `localdata/frequency/`):

| | Subtitle (OpenSubtitles 2018) | Written (UD treebank, words only) |
|---|---:|---:|
| **Finnish, top 1000** | 65.2% | 40.1% |
| **Estonian, top 1000** | 72.9% | 42.9% |
| **Finnish, top 5000** | 82.0% | 59.1% |
| **Estonian, top 5000** | 87.1% | 61.8% |

**Primary observation.** The subtitle-vs-written gap is roughly **25
percentage points** within each language at top-1000 forms. The
Finnish-vs-Estonian gap within either register is **3–7 percentage
points**. The register effect dominates the language effect by a factor
of ~5×.

**Secondary observation.** Within either register, Estonian is
consistently slightly easier than Finnish at every top-N level. This
gap is small but reproduces across both corpus sources we have, despite
opposite corpus-size relationships:

- OpenSubtitles ET has ~half the tokens of OpenSubtitles FI (smaller
  corpus, shorter long tail — would inflate ET's top-N coverage).
- UD-Estonian-EDT has ~2× the tokens of UD-Finnish-TDT (larger corpus,
  longer long tail — would deflate ET's top-N coverage).

The gap appears in both directions of the size confound, so at least
part of it is genuine. A plausible linguistic explanation is that
Estonian morphology is marginally less elaborate than Finnish (no vowel
harmony, weakened/limited consonant gradation, 14 cases vs 15), which
would compress the Zipfian tail of distinct forms slightly. But the
measurement is preliminary — see
[`TODO.md` "Re-test FI vs ET top-N coverage with corpus-size-comparable
data"](../TODO.md) for the size-matched re-measurement we should
do before committing to the linguistic interpretation.

### Product implications

1. **Don't promise a fixed comprehension % at a fixed form count
   without specifying the register.** "Learn 1000 words → understand
   80% of text" is roughly true for Estonian subtitles, very wrong for
   Finnish news/literature. This matters for any user-facing claim
   about deck size or learning targets.
2. **Comprehension prediction must condition on the source register**
   if we want predictions to be calibrated. A learner pasting Yle
   news will have a much lower comprehension number than a learner
   pasting subtitles, for the same known-form set. The user is likely
   to perceive this as a bug ("but I know all the conversational
   words") unless we explain it.
3. **The user-aggregated frequency from real pasted text should be
   register-tagged when feasible** so we can publish the curves split
   by register and see which our actual users are reading. See
   [`docs/ML_IDEAS.md` §2b](ML_IDEAS.md) and
   [`TODO.md` "Discover the most-frequent inflected forms in
   user-pasted text"](../TODO.md).

### Theoretical context (so we don't overclaim)

Register variation in token-coverage curves is documented in:

- Sinclair, J. (1991). *Corpus, Concordance, Collocation.* Oxford
  University Press — early work establishing register effects on
  frequency distributions.
- Biber, D. (1993). "Representativeness in Corpus Design." *Literary
  and Linguistic Computing* — register sampling and frequency
  estimation.
- Zipf, G. K. (1949). *Human Behavior and the Principle of Least
  Effort.* — the underlying distributional law.

The contribution of this measurement is **specific numbers on FI and
ET inflected-form coverage from these specific corpora using this
project's tokenization**. It is not a discovery; it is a calibration.

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
