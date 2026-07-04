# FI/ET contextual-ambiguity eval slice

Gold cases for the **ambiguity and meaning-check calibration** slice. This is
the measurement foundation the "Ambiguous meaning flow" launch gate depends on:
before the product may show a single **Meaning Check** (vs **Multiple possible
meanings**) for an ambiguity class, that class must clear a measured selection
bar on this slice. See
[`docs/PARSER_EVAL_METHODOLOGY.md` §Ambiguity and meaning-check calibration](../../../docs/PARSER_EVAL_METHODOLOGY.md#ambiguity-and-meaning-check-calibration)
for the full spec, the confidence-proxy definition, the threshold rule, and the
committed baseline table.

## Files

- `fi-ambiguity-v1.json` — Finnish slice (headline; the first-language slice).
- `et-ambiguity-v1.json` — Estonian parity slice (follow-up; smaller).

Both live under `testdata/parser-eval/` (data, correctly committed), NOT under a
language `gold/` dir, because they are consumed by a focused ambiguity runner,
not by the generic `make compare-parsers` accuracy sweep. Keeping them out of
`fi/gold` and `et/gold` also prevents them from silently inflating the headline
lemma/POS numbers with a curated, deliberately-hard homograph mix.

## Format

The file reuses the committed gold `Dataset`/`DatasetCase`/`ExpectedTokenRef`
shape from `internal/eval/eval.go`, extended with a small ambiguity block. A
case is one sentence with exactly one *scored* target token (the ambiguous
surface); non-target tokens are omitted because this slice measures
disambiguation of the target, not whole-sentence accuracy.

```jsonc
{
  "name": "fi-ambiguity",
  "version": "v1",
  "language": "FI",
  "slice": "ambiguity",              // marks this as an ambiguity slice, not a plain gold set
  "cases": [
    {
      "id": "fi-amb-kuusi-noun-1",
      "text": "Pihalla kasvaa suuri kuusi.",
      "ambiguity_class": "kuusi",    // groups sentences that share a homograph surface
      "tokens": [
        {
          "surface": "kuusi",
          "occurrence": 1,            // 1-based; which occurrence of the surface in the sentence
          "target": true,             // the token scored for candidate-inclusion / selection
          "lemma": "kuusi",           // contextually correct lemma
          "pos": "NOUN",              // contextually correct UD POS
          "feats": "Case=Nom|Number=Sing",
          "expected_candidates": [    // the sense set the parser SHOULD be able to offer
            {"lemma": "kuusi", "pos": "NOUN", "gloss_hint": "spruce"},
            {"lemma": "kuusi", "pos": "NUM",  "gloss_hint": "six"}
          ]
        }
      ]
    }
  ]
}
```

Field notes:

- `ambiguity_class` — every sentence for one homograph shares this key, so the
  runner can report per-class selection accuracy (the unit the threshold rule
  operates on).
- `target: true` — the single scored token. Runner metrics (candidate inclusion,
  selection accuracy, calibration) are computed only on target tokens.
- `expected_candidates` — the *full* sense set for this surface, independent of
  which sentence it appears in. Same list across all sentences of a class. Used
  for the candidate-inclusion metric (is the correct sense reachable at all?) and
  to detect the FI kaikki-gap failure mode where a real sense is absent from the
  dictionary forms table entirely.
- `gloss_hint` is a human-readability aid for reviewers; it is NOT matched
  against parser output.
- `feats` is optional and follows the same UD convention as the main gold sets.

## Provenance

All Finnish and Estonian sentences are original CC0-trivial examples authored
for this slice (simple, unambiguous except for the target surface). No corpus
text is copied. The verified current-parser baseline for every case is recorded
in the methodology doc's baseline table, produced by parsing each sentence in
`custom` mode against the production DB + FST tables.
