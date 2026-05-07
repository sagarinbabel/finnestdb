# Estonian parser comparison (20260507T110614Z, parsers: basic,custom,estnltk)

| Dataset | Cases | Parser | Lemma | POS | Grammar | Full | Coverage | Avg ms |
|---|---:|---|---:|---:|---:|---:|---:|---:|
| et-grammar | 50 | basic | 88.6% | 96.2% | 0.0% | 0.0% | 97.8% | 0.1 |
| et-grammar | 50 | custom | 88.6% | 96.2% | 19.6% | 0.0% | 98.9% | 0.1 |
| et-grammar | 50 | estnltk | 98.1% | 97.1% | 92.2% | 92.4% | 100.0% | 1253.1 |
| et-manual | 4 | basic | 77.8% | 77.8% | 0.0% | 0.0% | 83.3% | 0.1 |
| et-manual | 4 | custom | 77.8% | 77.8% | 0.0% | 0.0% | 91.7% | 0.1 |
| et-manual | 4 | estnltk | 100.0% | 100.0% | 100.0% | 100.0% | 100.0% | 1294.2 |

## Head-to-head deltas (parser - first parser, in pts)

| Dataset | Parser | Δ Lemma | Δ POS | Δ Coverage |
|---|---|---:|---:|---:|
| et-grammar | custom vs basic | +0.0 | +0.0 | +1.1 |
| et-grammar | estnltk vs basic | +9.5 | +1.0 | +2.2 |
| et-manual | custom vs basic | +0.0 | +0.0 | +8.3 |
| et-manual | estnltk vs basic | +22.2 | +22.2 | +16.7 |

## Per-FEATS-attribute accuracy

Each row scores one UD FEATS attribute (Case, Number, Tense, ...) on the
subset of gold tokens whose FEATS contained that attribute. Useful for
seeing where the parser is silent vs. wrong on richer morphology than the
single-attribute Grammar metric covers.

| Dataset | Attribute | Parser | Eligible | Correct | Accuracy |
|---|---|---|---:|---:|---:|
| et-grammar | Case | basic | 51 | 0 | 0.0% |
| et-grammar | Case | custom | 51 | 1 | 2.0% |
| et-grammar | Case | estnltk | 51 | 48 | 94.1% |
| et-grammar | Mood | basic | 24 | 0 | 0.0% |
| et-grammar | Mood | custom | 24 | 0 | 0.0% |
| et-grammar | Mood | estnltk | 24 | 24 | 100.0% |
| et-grammar | Number | basic | 102 | 0 | 0.0% |
| et-grammar | Number | custom | 102 | 0 | 0.0% |
| et-grammar | Number | estnltk | 102 | 102 | 100.0% |
| et-grammar | Person | basic | 23 | 0 | 0.0% |
| et-grammar | Person | custom | 23 | 0 | 0.0% |
| et-grammar | Person | estnltk | 23 | 23 | 100.0% |
| et-grammar | Tense | basic | 24 | 0 | 0.0% |
| et-grammar | Tense | custom | 24 | 0 | 0.0% |
| et-grammar | Tense | estnltk | 24 | 24 | 100.0% |
| et-grammar | VerbForm | basic | 25 | 0 | 0.0% |
| et-grammar | VerbForm | custom | 25 | 0 | 0.0% |
| et-grammar | VerbForm | estnltk | 25 | 25 | 100.0% |
| et-grammar | Voice | basic | 24 | 0 | 0.0% |
| et-grammar | Voice | custom | 24 | 0 | 0.0% |
| et-grammar | Voice | estnltk | 24 | 24 | 100.0% |
| et-manual | Case | basic | 6 | 0 | 0.0% |
| et-manual | Case | custom | 6 | 0 | 0.0% |
| et-manual | Case | estnltk | 6 | 6 | 100.0% |
| et-manual | Mood | basic | 1 | 0 | 0.0% |
| et-manual | Mood | custom | 1 | 0 | 0.0% |
| et-manual | Mood | estnltk | 1 | 1 | 100.0% |
| et-manual | Number | basic | 9 | 0 | 0.0% |
| et-manual | Number | custom | 9 | 0 | 0.0% |
| et-manual | Number | estnltk | 9 | 9 | 100.0% |
| et-manual | Person | basic | 1 | 0 | 0.0% |
| et-manual | Person | custom | 1 | 0 | 0.0% |
| et-manual | Person | estnltk | 1 | 1 | 100.0% |
| et-manual | Tense | basic | 1 | 0 | 0.0% |
| et-manual | Tense | custom | 1 | 0 | 0.0% |
| et-manual | Tense | estnltk | 1 | 1 | 100.0% |
| et-manual | VerbForm | basic | 1 | 0 | 0.0% |
| et-manual | VerbForm | custom | 1 | 0 | 0.0% |
| et-manual | VerbForm | estnltk | 1 | 1 | 100.0% |
| et-manual | Voice | basic | 1 | 0 | 0.0% |
| et-manual | Voice | custom | 1 | 0 | 0.0% |
| et-manual | Voice | estnltk | 1 | 1 | 100.0% |
