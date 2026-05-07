# Parser comparison (20260507T102649Z, parsers: basic,custom,omorfi)

| Dataset | Cases | Parser | Lemma | POS | Grammar | Full | Coverage | Avg ms |
|---|---:|---|---:|---:|---:|---:|---:|---:|
| fi-core | 6 | basic | 85.0% | 90.0% | 0.0% | 0.0% | 91.3% | 0.1 |
| fi-core | 6 | custom | 85.0% | 90.0% | 30.0% | 0.0% | 95.7% | 0.1 |
| fi-core | 6 | omorfi | 85.0% | 85.0% | 100.0% | 80.0% | 95.7% | 395.4 |
| fi-grammar | 80 | basic | 96.8% | 98.1% | 0.0% | 0.0% | 99.7% | 0.1 |
| fi-grammar | 80 | custom | 96.8% | 98.1% | 59.5% | 0.0% | 99.7% | 0.1 |
| fi-grammar | 80 | omorfi | 95.5% | 96.2% | 100.0% | 93.6% | 100.0% | 388.1 |
| fi-manual | 4 | basic | 88.9% | 100.0% | 0.0% | 11.1% | 91.7% | 0.1 |
| fi-manual | 4 | custom | 88.9% | 100.0% | 33.3% | 11.1% | 100.0% | 0.1 |
| fi-manual | 4 | omorfi | 100.0% | 100.0% | 100.0% | 100.0% | 100.0% | 389.0 |
| fi-manual | 4 | basic | 88.9% | 100.0% | 0.0% | 11.1% | 91.7% | 0.1 |
| fi-manual | 4 | custom | 88.9% | 100.0% | 33.3% | 11.1% | 100.0% | 0.1 |
| fi-manual | 4 | omorfi | 100.0% | 100.0% | 100.0% | 100.0% | 100.0% | 389.0 |

## Head-to-head deltas (parser - first parser, in pts)

| Dataset | Parser | Δ Lemma | Δ POS | Δ Coverage |
|---|---|---:|---:|---:|
| fi-core | custom vs basic | +0.0 | +0.0 | +4.3 |
| fi-core | omorfi vs basic | +0.0 | -5.0 | +4.3 |
| fi-grammar | custom vs basic | +0.0 | +0.0 | +0.0 |
| fi-grammar | omorfi vs basic | -1.3 | -1.9 | +0.3 |
| fi-manual | custom vs basic | +0.0 | +0.0 | +8.3 |
| fi-manual | omorfi vs basic | +11.1 | +0.0 | +8.3 |
| fi-manual | custom vs basic | +0.0 | +0.0 | +8.3 |
| fi-manual | omorfi vs basic | +11.1 | +0.0 | +8.3 |

## Per-FEATS-attribute accuracy

Each row scores one UD FEATS attribute (Case, Number, Tense, ...) on the
subset of gold tokens whose FEATS contained that attribute. Useful for
seeing where the parser is silent vs. wrong on richer morphology than the
single-attribute Grammar metric covers.

| Dataset | Attribute | Parser | Eligible | Correct | Accuracy |
|---|---|---|---:|---:|---:|
| fi-core | Case | basic | 17 | 0 | 0.0% |
| fi-core | Case | custom | 17 | 0 | 0.0% |
| fi-core | Case | omorfi | 17 | 17 | 100.0% |
| fi-core | Degree | basic | 3 | 0 | 0.0% |
| fi-core | Degree | custom | 3 | 0 | 0.0% |
| fi-core | Degree | omorfi | 3 | 3 | 100.0% |
| fi-core | Mood | basic | 2 | 0 | 0.0% |
| fi-core | Mood | custom | 2 | 0 | 0.0% |
| fi-core | Mood | omorfi | 2 | 2 | 100.0% |
| fi-core | Number | basic | 19 | 0 | 0.0% |
| fi-core | Number | custom | 19 | 0 | 0.0% |
| fi-core | Number | omorfi | 19 | 19 | 100.0% |
| fi-core | Number[psor] | basic | 1 | 0 | 0.0% |
| fi-core | Number[psor] | custom | 1 | 0 | 0.0% |
| fi-core | Number[psor] | omorfi | 1 | 1 | 100.0% |
| fi-core | Person | basic | 3 | 0 | 0.0% |
| fi-core | Person | custom | 3 | 0 | 0.0% |
| fi-core | Person | omorfi | 3 | 3 | 100.0% |
| fi-core | Person[psor] | basic | 1 | 0 | 0.0% |
| fi-core | Person[psor] | custom | 1 | 0 | 0.0% |
| fi-core | Person[psor] | omorfi | 1 | 1 | 100.0% |
| fi-core | PronType | basic | 1 | 0 | 0.0% |
| fi-core | PronType | custom | 1 | 0 | 0.0% |
| fi-core | PronType | omorfi | 1 | 1 | 100.0% |
| fi-core | Style | basic | 1 | 0 | 0.0% |
| fi-core | Style | custom | 1 | 0 | 0.0% |
| fi-core | Style | omorfi | 1 | 1 | 100.0% |
| fi-core | Tense | basic | 2 | 0 | 0.0% |
| fi-core | Tense | custom | 2 | 0 | 0.0% |
| fi-core | Tense | omorfi | 2 | 2 | 100.0% |
| fi-core | VerbForm | basic | 3 | 0 | 0.0% |
| fi-core | VerbForm | custom | 3 | 0 | 0.0% |
| fi-core | VerbForm | omorfi | 3 | 3 | 100.0% |
| fi-core | Voice | basic | 3 | 0 | 0.0% |
| fi-core | Voice | custom | 3 | 0 | 0.0% |
| fi-core | Voice | omorfi | 3 | 3 | 100.0% |
| fi-grammar | Case | basic | 130 | 0 | 0.0% |
| fi-grammar | Case | custom | 130 | 0 | 0.0% |
| fi-grammar | Case | omorfi | 130 | 129 | 99.2% |
| fi-grammar | Degree | basic | 13 | 0 | 0.0% |
| fi-grammar | Degree | custom | 13 | 0 | 0.0% |
| fi-grammar | Degree | omorfi | 13 | 13 | 100.0% |
| fi-grammar | InfForm | basic | 1 | 0 | 0.0% |
| fi-grammar | InfForm | custom | 1 | 0 | 0.0% |
| fi-grammar | InfForm | omorfi | 1 | 1 | 100.0% |
| fi-grammar | Mood | basic | 25 | 0 | 0.0% |
| fi-grammar | Mood | custom | 25 | 0 | 0.0% |
| fi-grammar | Mood | omorfi | 25 | 25 | 100.0% |
| fi-grammar | Number | basic | 154 | 0 | 0.0% |
| fi-grammar | Number | custom | 154 | 0 | 0.0% |
| fi-grammar | Number | omorfi | 154 | 154 | 100.0% |
| fi-grammar | Number[psor] | basic | 8 | 0 | 0.0% |
| fi-grammar | Number[psor] | custom | 8 | 0 | 0.0% |
| fi-grammar | Number[psor] | omorfi | 8 | 8 | 100.0% |
| fi-grammar | PartForm | basic | 2 | 0 | 0.0% |
| fi-grammar | PartForm | custom | 2 | 0 | 0.0% |
| fi-grammar | PartForm | omorfi | 2 | 2 | 100.0% |
| fi-grammar | Person | basic | 24 | 0 | 0.0% |
| fi-grammar | Person | custom | 24 | 0 | 0.0% |
| fi-grammar | Person | omorfi | 24 | 24 | 100.0% |
| fi-grammar | Person[psor] | basic | 14 | 0 | 0.0% |
| fi-grammar | Person[psor] | custom | 14 | 0 | 0.0% |
| fi-grammar | Person[psor] | omorfi | 14 | 14 | 100.0% |
| fi-grammar | Style | basic | 1 | 0 | 0.0% |
| fi-grammar | Style | custom | 1 | 0 | 0.0% |
| fi-grammar | Style | omorfi | 1 | 1 | 100.0% |
| fi-grammar | Tense | basic | 21 | 0 | 0.0% |
| fi-grammar | Tense | custom | 21 | 0 | 0.0% |
| fi-grammar | Tense | omorfi | 21 | 21 | 100.0% |
| fi-grammar | VerbForm | basic | 28 | 0 | 0.0% |
| fi-grammar | VerbForm | custom | 28 | 0 | 0.0% |
| fi-grammar | VerbForm | omorfi | 28 | 28 | 100.0% |
| fi-grammar | Voice | basic | 28 | 0 | 0.0% |
| fi-grammar | Voice | custom | 28 | 0 | 0.0% |
| fi-grammar | Voice | omorfi | 28 | 28 | 100.0% |
| fi-manual | Case | basic | 7 | 0 | 0.0% |
| fi-manual | Case | basic | 7 | 0 | 0.0% |
| fi-manual | Case | custom | 7 | 0 | 0.0% |
| fi-manual | Case | custom | 7 | 0 | 0.0% |
| fi-manual | Case | omorfi | 7 | 7 | 100.0% |
| fi-manual | Case | omorfi | 7 | 7 | 100.0% |
| fi-manual | Mood | basic | 1 | 0 | 0.0% |
| fi-manual | Mood | basic | 1 | 0 | 0.0% |
| fi-manual | Mood | custom | 1 | 0 | 0.0% |
| fi-manual | Mood | custom | 1 | 0 | 0.0% |
| fi-manual | Mood | omorfi | 1 | 1 | 100.0% |
| fi-manual | Mood | omorfi | 1 | 1 | 100.0% |
| fi-manual | Number | basic | 8 | 0 | 0.0% |
| fi-manual | Number | basic | 8 | 0 | 0.0% |
| fi-manual | Number | custom | 8 | 0 | 0.0% |
| fi-manual | Number | custom | 8 | 0 | 0.0% |
| fi-manual | Number | omorfi | 8 | 8 | 100.0% |
| fi-manual | Number | omorfi | 8 | 8 | 100.0% |
| fi-manual | Number[psor] | basic | 3 | 0 | 0.0% |
| fi-manual | Number[psor] | basic | 3 | 0 | 0.0% |
| fi-manual | Number[psor] | custom | 3 | 0 | 0.0% |
| fi-manual | Number[psor] | custom | 3 | 0 | 0.0% |
| fi-manual | Number[psor] | omorfi | 3 | 3 | 100.0% |
| fi-manual | Number[psor] | omorfi | 3 | 3 | 100.0% |
| fi-manual | Person | basic | 1 | 0 | 0.0% |
| fi-manual | Person | basic | 1 | 0 | 0.0% |
| fi-manual | Person | custom | 1 | 0 | 0.0% |
| fi-manual | Person | custom | 1 | 0 | 0.0% |
| fi-manual | Person | omorfi | 1 | 1 | 100.0% |
| fi-manual | Person | omorfi | 1 | 1 | 100.0% |
| fi-manual | Person[psor] | basic | 3 | 0 | 0.0% |
| fi-manual | Person[psor] | basic | 3 | 0 | 0.0% |
| fi-manual | Person[psor] | custom | 3 | 0 | 0.0% |
| fi-manual | Person[psor] | custom | 3 | 0 | 0.0% |
| fi-manual | Person[psor] | omorfi | 3 | 3 | 100.0% |
| fi-manual | Person[psor] | omorfi | 3 | 3 | 100.0% |
| fi-manual | Tense | basic | 1 | 0 | 0.0% |
| fi-manual | Tense | basic | 1 | 0 | 0.0% |
| fi-manual | Tense | custom | 1 | 0 | 0.0% |
| fi-manual | Tense | custom | 1 | 0 | 0.0% |
| fi-manual | Tense | omorfi | 1 | 1 | 100.0% |
| fi-manual | Tense | omorfi | 1 | 1 | 100.0% |
| fi-manual | VerbForm | basic | 1 | 0 | 0.0% |
| fi-manual | VerbForm | basic | 1 | 0 | 0.0% |
| fi-manual | VerbForm | custom | 1 | 0 | 0.0% |
| fi-manual | VerbForm | custom | 1 | 0 | 0.0% |
| fi-manual | VerbForm | omorfi | 1 | 1 | 100.0% |
| fi-manual | VerbForm | omorfi | 1 | 1 | 100.0% |
| fi-manual | Voice | basic | 1 | 0 | 0.0% |
| fi-manual | Voice | basic | 1 | 0 | 0.0% |
| fi-manual | Voice | custom | 1 | 0 | 0.0% |
| fi-manual | Voice | custom | 1 | 0 | 0.0% |
| fi-manual | Voice | omorfi | 1 | 1 | 100.0% |
| fi-manual | Voice | omorfi | 1 | 1 | 100.0% |
