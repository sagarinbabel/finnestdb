# ML Ideas — FinEstDB

_Created: 2026-05-07. Status: ideas, not commitments. Refresh after the FST
lemmatizer migration (PRs #106–#112) lands and the post-FST baseline is
frozen._

This document records ML directions that fit the project's constraints:

- **Open source.** No proprietary models; nothing we can't redistribute.
- **Owned at serve time.** Models we trained and ship, not an external API
  on the request path.
- **Fast.** Sub-millisecond per token at parse time; sub-second per
  sentence for sentence-level features.
- **Free at inference.** No per-request cost.
- **Deterministic.** Same input → same output, run-to-run.
- **Measurable.** Plugs into the existing `internal/eval` framework.

Anything that fails one of these is either marked as such or excluded. The
sentence-level direction at the end intentionally bends the "free at
inference" rule and the doc explains the tradeoff.

---

## 1. Word-level ML for parser disambiguation

These plug into the parser/lemmatizer pipeline. Highest ROI sits here
because the parser is on the critical path for every other product
feature.

### 1a. CRF/HMM POS+lemma disambiguator (HIGHEST ROI)

**Problem solved.** Homonym disambiguation. The current FST lookup returns
multiple candidates per surface form (e.g. ET `linnas` → `linn/NOUN` "in
the city" vs `Linna/PROPN` proper name; ET `naeris` → `naeris/NOUN` turnip
vs `naerma/VERB` "laughed"). Picking the wrong candidate is what's costing
8–13pp lemma/POS accuracy on the Estonian gold set.

**Approach.** Train a Conditional Random Field tagger on the UD treebanks
(UD-Finnish-TDT and UD-Estonian-EDT). Features per token: surface form,
suffixes (last 1–4 chars), prefix, capitalization, sentence position, +/-1
context tokens, candidate POS tags from the FST. Output: argmax over
candidate (lemma, POS) pairs.

**Data.** UD-Finnish-TDT and UD-Estonian-EDT, already public, CC BY-SA
(EDT is BY-NC-SA — non-commercial only, watch this). Plus user-corrected
parses from the existing `parse_feedback` table — that's the moat.

**Cost.** Training: hours on a laptop CPU. Model: a few MB. Inference:
microseconds per token, deterministic given trained weights.

**Constraint check.** Open ✓ Owned ✓ Fast ✓ Free at inference ✓
Deterministic ✓ Measurable (plugs into `internal/eval`) ✓.

**Confidence: high.** This is the textbook fix for the residual accuracy
gap that the FST migration won't close on its own.

#### Sequencing and parallelization _(added 2026-05-07)_

This work is **parallel-safe with the FEATS-threading PR** (`Feats` field
on `parsecore.TokenResult`) because they touch different files and
different layers. Suggested layout:

1. **New package**: `internal/disambiguator/` (or
   `pkg/disambiguator-fi-et/` if we want to ship it as a reusable
   library alongside `pkg/lemmatizer-fi-et/`).
2. **Training pipeline.** Read CoNLL-U from `data/ud-treebanks/` (this
   is already populated locally if `scripts/fetch-and-import-ud.sh` ran
   per [PR #113](https://github.com/sagarinbabel/finnestdb/pull/113)).
   Extract per-token feature tuples:
   - surface form
   - suffixes (last 1–4 chars)
   - prefix (first 1–3 chars)
   - capitalization
   - sentence position (BOS, MOS, EOS bins)
   - context tokens (form/lemma/POS at -1 and +1)
   - candidate (lemma, POS) tags from the dictionary and FST
   Train one CRF per language using a Go CRF implementation.
   Candidates: vendor a ~500-line CRF (the algorithm is
   well-documented), or call `crfsuite` via subprocess for the first
   spike, then port to pure Go. Avoid Python at runtime — the deploy
   story stays single-binary.
3. **Model artifact**: ship as `data/models/disambiguator-fi.crfsuite`
   and `data/models/disambiguator-et.crfsuite` (or our own Go-native
   format if we vendor the trainer). A few MB each. Embed via
   `go:embed` if we want zero runtime filesystem coupling, or
   disk-load the same way `pkg/lemmatizer-fi-et` loads its tables —
   pick consistently with the table-policy direction.
4. **Integration point — gated on candidate merging.** The natural
   place to insert the disambiguator is *after* both dict and FST have
   surfaced their candidates, so the CRF picks among the union. That
   "candidate merging" architectural correction was the closed
   [PR #120](https://github.com/sagarinbabel/finnestdb/pull/120)'s
   plan and is **not** on main yet. Until candidate merging lands,
   the disambiguator can only operate on either-dict-or-FST output,
   which is strictly weaker.
5. **Decoupling strategy.** Build and evaluate the trained model
   *offline*, ahead of the merge architecture. Treat the trained
   `.crfsuite` (or equivalent) as a deliverable on its own. Wire it
   into the runtime in a follow-up PR once candidate merging is in.
   This means the work doesn't block on parser-architecture decisions
   and the model is ready the day the integration point is.
6. **Eval.** Per-language accuracy delta on the **homonym subset** of
   gold — filter `testdata/parser-eval/{fi,et}/gold/*` to cases where
   dict returned ≥2 candidates for the surface form. Report
   lemma/POS/full accuracy lift on that subset; aggregate-level lift
   will be small because most tokens are unambiguous. The interesting
   number is "of the cases that were ambiguous before, what fraction
   does the disambiguator resolve correctly."
7. **Risk to watch.** ET treebank is **CC BY-NC-SA**. A model trained on
   it inherits the non-commercial constraint by most license
   interpretations. If the project commercializes (paywall, paid
   tier), retrain on commercially-licensed data or accept the
   non-commercial restriction explicitly. Document the choice when
   the model ships.

**Confidence on this sequencing: high.** Parallel-safe is the load-
bearing claim — verified by inspection of the file boundaries between
this work and the FEATS PR. Confidence on the lift estimate (5–10pp
on the homonym subset): moderate; it depends on whether the homonyms
in the gold set are linguistically tractable from the local context
features above. Some homonyms (`naeris/NOUN` vs `naerma/VERB`) are
trivially context-disambiguable; some (`linnas/NOUN` vs `Linna/PROPN`)
require knowing the specific named-entity context, which CRF features
capture only partially.

### 1b. Neural lemmatizer for unknown words

**Problem solved.** When the FST and dict both miss (proper names,
neologisms, typos, code-switching), the parser currently returns the
surface form as a stub lemma. A small char-level seq2seq trained on
(form, lemma) pairs from UD treebanks can guess sensible lemmas for OOV.

**Approach.** Char-level BiLSTM or small transformer encoder-decoder.
Stanza ships exactly this for FI and ET — about 50 MB per language, ~99%
accuracy on UD test set, sub-millisecond per word.

**Cost.** Training: a few GPU-hours (or a day on CPU). Inference: ms per
word. Not a hot-path concern because dict + FST resolves >95% of forms
before this fires.

**Confidence: high.** Standard tech, well-benchmarked.

### 1c. Knowledge distillation: LLM → small student

**Problem solved.** Anywhere a small targeted model would help but
labeled data is thin (e.g. MWE detection, sentence-level grammar
features, idiom flagging).

**Approach.** Use a strong LLM to label a fixed corpus of ~100k–1M
FI/ET sentences with the target annotation. Train a small transformer
(~50 MB) on those labels. Ship the student. The teacher LLM is paid for
once at training time; serve time is local + deterministic.

**Cost.** LLM labeling cost ($100s–$1000s once), training a few hours.
Model fits on a phone.

**Confidence: moderate-high.** Pattern is settled; the variability is
how clean the LLM-as-labeler is for FI/ET morphology specifically.

---

## 2. Lexical-layer ML

These feed the lexical knowledge layer (Phase 4), not the parser hot
path.

### 2a. fastText embeddings on FI/ET

**Problem solved.** Semantic similarity, near-neighbor lookup, "words
related to X." Useful for the lexical knowledge graph (Phase 4) and
feature `4. Cross-deck comprehension gain` (rank words by what they
unlock across decks).

**Approach.** Train fastText skip-gram on Finnish + Estonian Wikipedia
+ pasted user text + UD treebanks. 100-dimensional vectors, sub-MB
quantized.

**Cost.** Training: a few hours on a laptop. Disk: <100 MB per language
quantized. Inference: microseconds.

**Confidence: high (settled tech).** This has been done many times for
both languages, but doing it on our specific corpus mix is what makes
it ours.

### 2b. User-text-aggregated frequency of inflected forms (RESEARCH GOAL)

**Problem solved.** Discover, from text users actually paste, which
**inflected surface forms** matter most for reading comprehension in
Finnish and Estonian. Existing public lists rank lemmas (wrong unit for
a learner reading running text) or rank forms on a fixed corpus
(subtitle, news, Wikipedia) that may not reflect what real learners
read.

**Approach.** As users paste text, aggregate form counts per language
into a running tally. Periodically recompute the ranked top-N. Surface
this both internally (deck construction, comprehension prediction) and
as a learner-facing data product ("the 1000 most-frequent Finnish
forms our users encounter, with comprehension coverage").

**Comparison anchors.** See
[`docs/FREQUENCY_BASELINES.md`](FREQUENCY_BASELINES.md) for public
baselines (OpenSubtitles 2018, UD-Finnish-TDT, UD-Estonian-EDT) and
their coverage curves. Bulk files in `localdata/frequency/` (gitignored,
populated by `cmd/fetchfrequency`). The user-aggregated ranking should be
compared against these to measure how different real-user text is from
those baselines.

**Cost.** Negligible compute. Privacy-relevant: aggregating across
users requires a documented retention/anonymization policy (already
flagged in `TODO.md`).

**Why this is novel.** Inflected-form frequency from
user-actual-reading-interest text doesn't exist as a public dataset for
either language, as far as we've checked. It's a real contribution if
we publish the resulting ranking + coverage curves.

**Confidence: moderate-high.** The idea works. The "novel" claim is
contingent on the public-list survey being thorough; revisit once the
ranking is computed and compared against the public baselines documented in `docs/FREQUENCY_BASELINES.md`.

### 2c. Comprehension prediction (CORE FEATURE — already in spec)

**Problem solved.** Given any text, predict the user's comprehension %
of it before they read.

**Approach.** Token-weighted coverage: known-form-tokens / total-tokens.
Use the form-level frequency aggregation from 2b plus the user's known-form
set. Output is a percentage per text or per deck. Marginal gain per
candidate word is the gradient ("if you learn N more words, your
comprehension on this deck goes from X% to Y%").

**Status.** Already specified in
[`docs/srs-deck-spec.md` §3](srs-deck-spec.md). Tracked in `TODO.md`
Implementation Backlog §13. The infrastructure exists; the UI layer is
what's missing.

**Confidence: high.** Settled approach, settled formula. Execution is
UI work plus the frequency-aggregation backend from 2b.

---

## 3. Sentence-level ML — the "cached corpus" strategy

This is the direction where the project intentionally bends the "free
at inference" rule.

### 3a. The two flows

There are two distinct kinds of sentence-level work:

| Flow | Input | Cache hit rate | Approach |
|---|---|---|---|
| **Curated corpus** | a fixed library of ~100k learner-friendly sentences chosen by the project | 100% — every sentence is precomputed | precompute LLM translations, context labels, difficulty ratings; serve from cache |
| **User-pasted text** | arbitrary text the user pastes | ~0% — combinatorial sentence space | either small on-device model, API at request time, or graceful degradation to word-level only |

Conflating these two flows is the failure mode. They need different
infrastructure.

### 3b. Curated-corpus sentence cards (PRIMARY DIRECTION)

**Problem solved.** Generate sentence-based learning cards with
contextual translations, grammar notes, and difficulty ratings, without
hitting an external API at request time.

**Approach.**

1. Curate a fixed corpus of ~100k FI sentences and ~100k ET sentences
   chosen for learner value: simple grammar at the low end, diverse
   vocabulary at the high end, real-world sources (news, fiction,
   conversational), filtered for offensive content and proper-noun
   density.
2. Run each sentence through a strong LLM offline to generate:
   English translation, grammar notes, difficulty score (CEFR-ish),
   key-vocabulary callouts, and a short "why this sentence is useful"
   tag.
3. Store the LLM output keyed on the sentence hash.
4. At serve time, look up by hash. Deterministic, sub-millisecond,
   no API call on the request path.

**Constraint check.** Open ✗ (LLM at training time is not open;
output is) Owned ✓ (cache is ours forever) Fast ✓ Free at inference ✓
Deterministic ✓ Measurable ✓.

**Cost.** LLM cost at curation time: ~$1k-$5k for 200k sentences total
on Claude/GPT-4. Storage: <500 MB compressed.

**Why this works.** The project doesn't need to translate arbitrary
sentences. It needs to *show learners good sentences*. A curated 200k
corpus with rich annotations is a stronger product than translating
arbitrary user-pasted sentences badly.

**Confidence: high.** This is the direction.

### 3c. Sentence-level analysis on user-pasted text

**Problem solved.** When the user pastes their own text and we want to
show contextual translations, related sentences, or paragraph-level
context summaries.

**Approach options, ranked:**

1. **Graceful degradation.** Word-level cards from user-pasted text;
   no sentence-level translation. The user's pasted text is for
   *parsing and deck construction*, not for full LLM translation.
   This is the safest default. Confidence: high.
2. **Distilled local model (1c).** Train a small student model offline
   on LLM-labeled data; ship the student. Sentence-level translation
   gets a ~85–92% quality version of LLM output, deterministic and
   local. Cost: training run + 50 MB per language. Confidence:
   moderate.
3. **Optional API call at request time.** Allow logged-in users to opt
   in to "rich mode" that hits an LLM for sentence-level translation.
   Charge for it or rate-limit. Violates the "free at inference"
   constraint but gives users the option. Confidence: moderate (only
   if the project monetizes).

The right default is (1) for the alpha. Add (2) once the parser and
deck flow are stable; add (3) only if monetization needs it.

---

## 4. Review-flow ML (Phase 5)

Not the immediate priority but worth recording.

### 4a. Contextual bandit for new-card selection

**Problem solved.** "Which word should the learner study next?" Today
this is heuristic (token frequency in the active deck, with manual
tweaks). A contextual bandit can learn each user's optimal review
ordering from their answer outcomes.

**Approach.** Standard LinUCB-style bandit. Features: form frequency,
days-since-introduced, number of decks the form appears in, recent
answer accuracy, time-of-day. Reward: post-review confidence
improvement.

**Cost.** Trains continuously per user, ~kilobytes of state per user,
microseconds per decision.

**Confidence: moderate.** Depends on having clean reward signal from
the review flow, which doesn't exist yet (Phase 5).

### 4b. FSRS or successor for spaced-repetition scheduling

Already in the project plan
([`docs/srs-deck-spec.md`](srs-deck-spec.md)). Not "ML" in the
training sense, but a learned scheduling model — relevant when this
doc is refreshed.

---

## Recommended sequence (subject to refresh)

1. **Land PRs #106–#112** (FST migration). Refresh this doc with
   post-FST eval baselines.
2. **CRF disambiguator (1a).** Highest expected accuracy gain. Targets
   the residual ~8–13pp ET POS gap and FI homonym cases the FST won't
   solve alone.
3. **User-text frequency aggregation (2b)** alongside Phase 5 work.
   Cheap to add, compounds with usage, feeds comprehension prediction
   (2c).
4. **Curated-corpus sentence cards (3b).** Strongest sentence-level
   product story; fully owned at serve time.
5. **fastText embeddings (2a)** for the lexical knowledge graph
   (Phase 4).
6. **Neural lemmatizer (1b)** when OOV rate becomes a measured
   problem.
7. **Bandit ranker (4a)** after the review flow has reward signal.

Refresh this doc after each step lands. Confidence on the order: high
through step 3, moderate after.

## Constraints on this doc

- Re-evaluate after PRs #106–#112 merge. The post-FST baselines may
  shift relative ROIs.
- Confidence levels are mine, not consensus. Push back if you disagree
  before committing engineering time.
- Anything tagged "moderate" needs a small spike before scoping. Don't
  green-light a full implementation off this doc alone.
