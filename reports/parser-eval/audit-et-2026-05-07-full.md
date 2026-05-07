# Estonian parser comparison (20260507T125228Z, parsers: basic,custom,estnltk)

| Dataset | Cases | Parser | Lemma | POS | Lemma+POS | Grammar | Full | Coverage | Avg ms |
|---|---:|---|---:|---:|---:|---:|---:|---:|---:|
| et-grammar | 50 | basic | 90.5% | 94.3% | 87.6% | 82.4% | 81.0% | 100.0% | 0.1 |
| et-grammar | 50 | custom | 86.7% | 91.4% | 83.8% | 78.4% | 79.0% | 100.0% | 0.1 |
| et-grammar | 50 | estnltk | 98.1% | 97.1% | 96.2% | 94.1% | 93.3% | 100.0% | 1256.4 |
| et-manual | 4 | basic | 88.9% | 88.9% | 88.9% | 83.3% | 66.7% | 91.7% | 0.1 |
| et-manual | 4 | custom | 100.0% | 100.0% | 100.0% | 83.3% | 77.8% | 100.0% | 0.1 |
| et-manual | 4 | estnltk | 100.0% | 100.0% | 100.0% | 100.0% | 100.0% | 100.0% | 1235.9 |

## Head-to-head deltas (parser - first parser, in pts)

| Dataset | Parser | Δ Lemma | Δ POS | Δ Coverage |
|---|---|---:|---:|---:|
| et-grammar | custom vs basic | -3.8 | -2.9 | +0.0 |
| et-grammar | estnltk vs basic | +7.6 | +2.9 | +0.0 |
| et-manual | custom vs basic | +11.1 | +11.1 | +8.3 |
| et-manual | estnltk vs basic | +11.1 | +11.1 | +8.3 |

## Per-FEATS-attribute accuracy

Each row scores one UD FEATS attribute (Case, Number, Tense, ...) on the
subset of gold tokens whose FEATS contained that attribute. Useful for
seeing where the parser is silent vs. wrong on richer morphology than the
single-attribute Grammar metric covers.

| Dataset | Attribute | Parser | Eligible | Correct | Accuracy |
|---|---|---|---:|---:|---:|
| et-grammar | Case | basic | 51 | 42 | 82.4% |
| et-grammar | Case | custom | 51 | 40 | 78.4% |
| et-grammar | Case | estnltk | 51 | 48 | 94.1% |
| et-grammar | Mood | basic | 24 | 23 | 95.8% |
| et-grammar | Mood | custom | 24 | 24 | 100.0% |
| et-grammar | Mood | estnltk | 24 | 24 | 100.0% |
| et-grammar | Number | basic | 102 | 98 | 96.1% |
| et-grammar | Number | custom | 102 | 98 | 96.1% |
| et-grammar | Number | estnltk | 102 | 102 | 100.0% |
| et-grammar | Person | basic | 23 | 20 | 87.0% |
| et-grammar | Person | custom | 23 | 21 | 91.3% |
| et-grammar | Person | estnltk | 23 | 23 | 100.0% |
| et-grammar | Tense | basic | 24 | 23 | 95.8% |
| et-grammar | Tense | custom | 24 | 24 | 100.0% |
| et-grammar | Tense | estnltk | 24 | 24 | 100.0% |
| et-grammar | VerbForm | basic | 25 | 23 | 92.0% |
| et-grammar | VerbForm | custom | 25 | 24 | 96.0% |
| et-grammar | VerbForm | estnltk | 25 | 25 | 100.0% |
| et-grammar | Voice | basic | 24 | 23 | 95.8% |
| et-grammar | Voice | custom | 24 | 24 | 100.0% |
| et-grammar | Voice | estnltk | 24 | 24 | 100.0% |
| et-manual | Case | basic | 6 | 5 | 83.3% |
| et-manual | Case | custom | 6 | 5 | 83.3% |
| et-manual | Case | estnltk | 6 | 6 | 100.0% |
| et-manual | Mood | basic | 1 | 0 | 0.0% |
| et-manual | Mood | custom | 1 | 1 | 100.0% |
| et-manual | Mood | estnltk | 1 | 1 | 100.0% |
| et-manual | Number | basic | 9 | 8 | 88.9% |
| et-manual | Number | custom | 9 | 8 | 88.9% |
| et-manual | Number | estnltk | 9 | 9 | 100.0% |
| et-manual | Person | basic | 1 | 0 | 0.0% |
| et-manual | Person | custom | 1 | 1 | 100.0% |
| et-manual | Person | estnltk | 1 | 1 | 100.0% |
| et-manual | Tense | basic | 1 | 0 | 0.0% |
| et-manual | Tense | custom | 1 | 1 | 100.0% |
| et-manual | Tense | estnltk | 1 | 1 | 100.0% |
| et-manual | VerbForm | basic | 1 | 0 | 0.0% |
| et-manual | VerbForm | custom | 1 | 1 | 100.0% |
| et-manual | VerbForm | estnltk | 1 | 1 | 100.0% |
| et-manual | Voice | basic | 1 | 0 | 0.0% |
| et-manual | Voice | custom | 1 | 1 | 100.0% |
| et-manual | Voice | estnltk | 1 | 1 | 100.0% |
