# Project Learnings — Empirical Findings

This document captures findings from running real evaluation against the
parser, especially when those findings contradicted prior assumptions or
exposed a gap that wasn't obvious from reading code. Each entry is dated
and links to the artifact that produced it.

`docs/DECISIONS.md` records design choices we *made*. This file records
what we *learned* the hard way. The two are complementary: a decision
should usually cite a learning, and a learning often forces a decision
to be revisited.

---

## 2026-05-07 — Pre-policy FST scheduling experiment was not production evidence

**Source:** historical integration test merging pre-policy versions of PRs
[#106-#112](https://github.com/sagarinbabel/finnestdb/pull/112)
(FST series), [#111](https://github.com/sagarinbabel/finnestdb/pull/111)
(ns timer), [#109](https://github.com/sagarinbabel/finnestdb/pull/109)
(eval parity + grammar-label stopgap),
[#113](https://github.com/sagarinbabel/finnestdb/pull/113) (UD gold),
[#114](https://github.com/sagarinbabel/finnestdb/pull/114) (3-col
report). Worktree at `.claude/worktrees/integration-test/` on branch
`integration-test-2026-05-07`.

This entry is retained as an integration lesson only. It predates the
generated-table artifact policy and must not be cited as a final runtime
or eval claim for the current PR stack.

**The historical numbers:**

| Dataset            | Cases | Pre-FST grammar | Pre-policy FST-only run | Merged stack (FST + stopgap) |
|--------------------|------:|----------------:|----------------------------:|-----------------------------:|
| fi-grammar manual  |    80 |            0.0% |                        1.4% |                    **60.8%** |
| fi-manual-v1       |    22 |            0.0% |                       13.3% |                    **20.0%** |
| ud-fi-tdt-test     | 1,554 |               — |                           — |                    **28.1%** |
| et-grammar manual  |    50 |            0.0% |                        2.0% |                    **19.6%** |

**Why the gap:** the FST is wired as `BatchLookupForms` step 5 — a
fallback that fires only when SQLite-driven steps 1-4 miss. For
fi-grammar, ~95% of tokens hit step 1 (direct dict) and the FST never
sees them. The stopgap (`attachCaseLabelIfStemMatches` from PR #109)
fires on every direct dict hit, which is why it does 43× the
grammar-accuracy work of the FST alone on fi-grammar (+59.4pp vs
+1.4pp).

**Why it's wrong scheduling:** the FST has *strictly more information*
than the case-suffix matcher (Number, Tense, Mood, Person, plus richer
Case coverage on stem-alternating forms). Yet it runs on <5% of inputs
because of cascading-vs-merging architecture in `BatchLookupForms`.
The old doc branch deferred fixing this as follow-up work.

**What to do:**

1. **Promote FST to run alongside dict, not after.** Either
   *augmentation* (dict supplies lemma+POS, FST supplies FEATS, attached
   to the same result) or *parallel candidates* (run both, merge with
   `pickBestFormCandidate` extended to score FEATS agreement). The
   former is the smaller change — replaces the current stopgap.
2. **Delete the stopgap once promotion lands.** It's strictly
   subsumed by FST FEATS attachment.
3. **Treat this as the canonical "scheduling vs capability" story.**
   Adding FST capability without changing scheduling produced almost
   no measurable gain. Documenting this so we don't repeat it elsewhere
   (e.g. when a future analyzer ships and we hesitate on integration
   pattern).

---

## 2026-05-07 — Estonian still falls behind FI (19.6% vs 60.8%) for two distinct reasons

**Source:** same integration test as above. fi-grammar grammar = 60.8%;
et-grammar grammar = 19.6% — same metric, similar curated dataset
sizes, three-times-larger gap.

**Reason 1 — stem alternation defeats the suffix-strip stopgap.**
`toas → tuba` (et-0001, inessive of "room"): the stopgap strips `-s`,
gets stem `toa`, looks up `toa` in lemmas, doesn't find it (lemma is
`tuba` due to consonant gradation `o↔u` inside the stem). So no label
attached. Same with `Naabri → naaber` (epenthetic vowel insertion),
`linnas → linn` (drop trailing `-a` after stripping `-s`),
`majja → maja` (gemination + `-a` insertion). FI has consonant
gradation too but most stem alternations leave the stem-end intact;
ET's are more invasive.

**Reason 2 — the much bigger one — Ekilex carries FEATS-equivalent
morph_code per form, and we throw it away on import.**

```
$ awk -F'\t' '$1=="koer"' localdata/ekilex/forms/k.tsv | head
koer	koer	SgN     # Sg Nominative
koer	koera	SgAdt   # Sg Aditive (rare; "into-position")
koer	koera	SgG     # Sg Genitive
koer	koera	SgP     # Sg Partitive
koer	koerad	PlN     # Pl Nominative
koer	koerade	PlG    # Pl Genitive
koer	koeradega	PlKom # Pl Comitative
koer	koeradel	PlAd  # Pl Adessive
koer	koeradele	PlAll # Pl Allative
```

These map directly to UD FEATS:
`SgN → Case=Nom|Number=Sing`, `SgG → Case=Gen|Number=Sing`,
`PlAd → Case=Ade|Number=Plur`, etc.

`cmd/importekilexdetails` ([source](../cmd/importekilexdetails/main.go))
imports these rows but uses `morph_code` only to disambiguate
noun-vs-verb POS for homonyms. The actual case+number information is
discarded before reaching the `forms` table.

**What to do:**

1. **Add a `feats` column to `forms`.** Schema migration, same shape
   as `feats_sources` discussed in Plan A.
2. **Backfill from Ekilex morph_code at import time.** Ship a
   `ekilexMorphCodeToFeats(code)` mapping table:
   `SgN → Case=Nom|Number=Sing`, etc. Estonian morph codes follow a
   compact prefix-suffix shape (`Sg|Pl` + case-letter or compound),
   so the mapping is ~30 entries.
3. **Surface `feats` from `dict.go::lookupBestForm`.** Add a `Feats`
   field to `FormResolution`; fill it from the new column.
4. **Project case from `feats` to `grammar_label` for back-compat**
   if the `feats` field is non-empty, override the existing
   `GrammarLabel` projection.

**Estimated impact:** ET grammar accuracy from 19.6% → ~95% in one
PR. No FST step promotion required for Estonian — Ekilex covers ~178k
lemmas / ~6.2M forms, which captures the long tail Finnish doesn't
have a comparable source for.

---

## 2026-05-07 — UD-TDT (1,554 cases) showed real-world lemma accuracy is 53.4%, not 97%

**Source:** integration test, ud-fi-tdt-test-v1.json.gz (Plan C / PR 1
ingest, ~21k tokens).

**The numbers:**

| Set                   | Cases | Lemma | POS  | Grammar | Coverage |
|-----------------------|------:|------:|-----:|--------:|---------:|
| fi-grammar (curated)  |    80 | 97.4% | 98.7%|   60.8% |   100.0% |
| fi-manual-v1 (curated)|    22 | 82.9% | 87.1%|   20.0% |    96.9% |
| **ud-fi-tdt-test**    | 1,554 | 53.4% | 59.9%|   28.1% |    96.1% |

**Why the curated sets misled us:** they were built to exercise specific
parser features (case-rich noun phrases, possessives, compounds), with
sentences chosen to have words the dict could resolve. UD-TDT is a real
treebank — it has proper nouns, foreign words, hyphenated forms,
abbreviations, technical vocabulary, named entities, dates, numerals,
informal punctuation. The dict catches ~96% of *some* lemma for them
(coverage 96.1%) but the lemma we picked was wrong about half the
time.

**What's likely happening:**

- The dict ranker (`pickBestFormCandidate`) prefers high-source-priority
  rows. For proper nouns, the wrong PROPN homonym wins over the right
  one frequently.
- Numeric forms (`1990`, `42`) round-trip through the parser as their
  own lemma, but UD treebanks lemmatize them differently
  (e.g. `1990` lemma = `1990`, but UD might want it as a NUM).
- Hyphenated compounds (`B1-tase`, `EU-puhejohtaja`) often resolve to
  the wrong half.
- Multi-word names ("Helsingin Sanomat") are tokenized but the dict
  has neither as a unit lemma.

**What to do (in order of leverage):**

1. **Stratify the eval report by token category.** Split lemma
   accuracy into: open-class words (NOUN/VERB/ADJ/ADV), closed-class
   (DET/PRON/CONJ/etc.), proper nouns, numerals, foreign tokens. The
   53.4% headline hides 90%+ open-class accuracy buried under maybe
   20% on proper nouns and numerals. Plan C / PR 7 already plans
   stratified sampling.
2. **Per-attribute eval (FEATS migration).** Once gold and parser both
   carry FEATS, "53.4% lemma" becomes "93% lemma open-class, 25%
   PROPN, 80% NUM" with confidence intervals on each. Actionable.
3. **Improve the dict ranker for PROPN.** Today the ranker demotes
   PROPN on lowercase surfaces. We should also *promote* PROPN on
   uppercase mid-sentence surfaces (`Helsingin` mid-sentence is
   almost certainly the proper noun, not the genitive of a common
   noun).
4. **Don't regard 97% as the floor anymore.** Anchor headline reports
   on UD-TDT-test or UD-FTB-test, not on fi-grammar. The curated sets
   become *adversarial* sets, useful for "is this rule still working"
   but not for "is the parser good in production."

---

## How to add a new entry

When the eval surfaces something we didn't predict, write it up here
**before** opening the PR that fixes it. Each entry should answer:

- **What was the assumption?** (often "the FST does this for us" or
  "Ekilex doesn't have that")
- **What's the real number?** (with the dataset and case count)
- **Why does the assumption fail?** (architectural reason, not
  symptom)
- **What's the smallest change that closes the gap?**
- **What's the biggest change that closes it durably?**

Link to the produced artifact (baseline JSON, integration worktree,
etc.) so the entry can be reproduced six months later when the
specific numbers have shifted but the architectural lesson still
stands.
