# Parser audit (20260507T125057Z, parsers: basic,custom,omorfi)

| Dataset | Cases | Parser | Lemma | POS | Lemma+POS | Grammar | Full | Coverage | Avg ms |
|---|---:|---|---:|---:|---:|---:|---:|---:|---:|
| fi-core | 6 | basic | 85.0% | 90.0% | 85.0% | 100.0% | 45.0% | 91.3% | 0.1 |
| fi-core | 6 | custom | 85.0% | 90.0% | 85.0% | 100.0% | 45.0% | 95.7% | 0.1 |
| fi-core | 6 | omorfi | 85.0% | 85.0% | 80.0% | 100.0% | 80.0% | 95.7% | 384.8 |
| fi-grammar | 80 | basic | 96.8% | 98.1% | 96.8% | 98.6% | 50.6% | 99.7% | 0.1 |
| fi-grammar | 80 | custom | 96.8% | 98.1% | 96.8% | 98.6% | 50.6% | 99.7% | 0.1 |
| fi-grammar | 80 | omorfi | 95.5% | 96.2% | 94.2% | 100.0% | 93.6% | 100.0% | 394.5 |
| fi-manual | 22 | basic | 55.7% | 78.6% | 52.9% | 60.0% | 30.0% | 75.3% | 0.2 |
| fi-manual | 22 | custom | 81.4% | 85.7% | 80.0% | 60.0% | 32.9% | 91.2% | 0.3 |
| fi-manual | 22 | omorfi | 74.3% | 77.1% | 74.3% | 60.0% | 54.3% | 87.6% | 404.5 |
| fi-manual | 4 | basic | 88.9% | 100.0% | 88.9% | 100.0% | 44.4% | 91.7% | 0.1 |
| fi-manual | 4 | custom | 88.9% | 100.0% | 88.9% | 100.0% | 44.4% | 100.0% | 0.1 |
| fi-manual | 4 | omorfi | 100.0% | 100.0% | 100.0% | 100.0% | 100.0% | 100.0% | 403.3 |

## Head-to-head deltas (parser - first parser, in pts)

| Dataset | Parser | Δ Lemma | Δ POS | Δ Coverage |
|---|---|---:|---:|---:|
| fi-core | custom vs basic | +0.0 | +0.0 | +4.3 |
| fi-core | omorfi vs basic | +0.0 | -5.0 | +4.3 |
| fi-grammar | custom vs basic | +0.0 | +0.0 | +0.0 |
| fi-grammar | omorfi vs basic | -1.3 | -1.9 | +0.3 |
| fi-manual | custom vs basic | +25.7 | +7.1 | +16.0 |
| fi-manual | omorfi vs basic | +18.6 | -1.4 | +12.4 |
| fi-manual | custom vs basic | +0.0 | +0.0 | +8.3 |
| fi-manual | omorfi vs basic | +11.1 | +0.0 | +8.3 |

## Per-FEATS-attribute accuracy

Each row scores one UD FEATS attribute (Case, Number, Tense, ...) on the
subset of gold tokens whose FEATS contained that attribute. Useful for
seeing where the parser is silent vs. wrong on richer morphology than the
single-attribute Grammar metric covers.

| Dataset | Attribute | Parser | Eligible | Correct | Accuracy |
|---|---|---|---:|---:|---:|
| fi-core | Case | basic | 17 | 14 | 82.4% |
| fi-core | Case | custom | 17 | 14 | 82.4% |
| fi-core | Case | omorfi | 17 | 17 | 100.0% |
| fi-core | Degree | basic | 3 | 0 | 0.0% |
| fi-core | Degree | custom | 3 | 0 | 0.0% |
| fi-core | Degree | omorfi | 3 | 3 | 100.0% |
| fi-core | Mood | basic | 2 | 2 | 100.0% |
| fi-core | Mood | custom | 2 | 2 | 100.0% |
| fi-core | Mood | omorfi | 2 | 2 | 100.0% |
| fi-core | Number | basic | 19 | 15 | 78.9% |
| fi-core | Number | custom | 19 | 15 | 78.9% |
| fi-core | Number | omorfi | 19 | 19 | 100.0% |
| fi-core | Number[psor] | basic | 1 | 0 | 0.0% |
| fi-core | Number[psor] | custom | 1 | 0 | 0.0% |
| fi-core | Number[psor] | omorfi | 1 | 1 | 100.0% |
| fi-core | Person | basic | 3 | 2 | 66.7% |
| fi-core | Person | custom | 3 | 2 | 66.7% |
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
| fi-core | Tense | basic | 2 | 2 | 100.0% |
| fi-core | Tense | custom | 2 | 2 | 100.0% |
| fi-core | Tense | omorfi | 2 | 2 | 100.0% |
| fi-core | VerbForm | basic | 3 | 0 | 0.0% |
| fi-core | VerbForm | custom | 3 | 0 | 0.0% |
| fi-core | VerbForm | omorfi | 3 | 3 | 100.0% |
| fi-core | Voice | basic | 3 | 0 | 0.0% |
| fi-core | Voice | custom | 3 | 0 | 0.0% |
| fi-core | Voice | omorfi | 3 | 3 | 100.0% |
| fi-grammar | Case | basic | 130 | 101 | 77.7% |
| fi-grammar | Case | custom | 130 | 101 | 77.7% |
| fi-grammar | Case | omorfi | 130 | 129 | 99.2% |
| fi-grammar | Degree | basic | 13 | 0 | 0.0% |
| fi-grammar | Degree | custom | 13 | 0 | 0.0% |
| fi-grammar | Degree | omorfi | 13 | 13 | 100.0% |
| fi-grammar | InfForm | basic | 1 | 0 | 0.0% |
| fi-grammar | InfForm | custom | 1 | 0 | 0.0% |
| fi-grammar | InfForm | omorfi | 1 | 1 | 100.0% |
| fi-grammar | Mood | basic | 25 | 24 | 96.0% |
| fi-grammar | Mood | custom | 25 | 24 | 96.0% |
| fi-grammar | Mood | omorfi | 25 | 25 | 100.0% |
| fi-grammar | Number | basic | 154 | 125 | 81.2% |
| fi-grammar | Number | custom | 154 | 125 | 81.2% |
| fi-grammar | Number | omorfi | 154 | 154 | 100.0% |
| fi-grammar | Number[psor] | basic | 8 | 0 | 0.0% |
| fi-grammar | Number[psor] | custom | 8 | 0 | 0.0% |
| fi-grammar | Number[psor] | omorfi | 8 | 8 | 100.0% |
| fi-grammar | PartForm | basic | 2 | 0 | 0.0% |
| fi-grammar | PartForm | custom | 2 | 0 | 0.0% |
| fi-grammar | PartForm | omorfi | 2 | 2 | 100.0% |
| fi-grammar | Person | basic | 24 | 23 | 95.8% |
| fi-grammar | Person | custom | 24 | 23 | 95.8% |
| fi-grammar | Person | omorfi | 24 | 24 | 100.0% |
| fi-grammar | Person[psor] | basic | 14 | 0 | 0.0% |
| fi-grammar | Person[psor] | custom | 14 | 0 | 0.0% |
| fi-grammar | Person[psor] | omorfi | 14 | 14 | 100.0% |
| fi-grammar | Style | basic | 1 | 0 | 0.0% |
| fi-grammar | Style | custom | 1 | 0 | 0.0% |
| fi-grammar | Style | omorfi | 1 | 1 | 100.0% |
| fi-grammar | Tense | basic | 21 | 19 | 90.5% |
| fi-grammar | Tense | custom | 21 | 19 | 90.5% |
| fi-grammar | Tense | omorfi | 21 | 21 | 100.0% |
| fi-grammar | VerbForm | basic | 28 | 0 | 0.0% |
| fi-grammar | VerbForm | custom | 28 | 0 | 0.0% |
| fi-grammar | VerbForm | omorfi | 28 | 28 | 100.0% |
| fi-grammar | Voice | basic | 28 | 1 | 3.6% |
| fi-grammar | Voice | custom | 28 | 1 | 3.6% |
| fi-grammar | Voice | omorfi | 28 | 28 | 100.0% |
| fi-manual | Case | basic | 57 | 24 | 42.1% |
| fi-manual | Case | basic | 7 | 6 | 85.7% |
| fi-manual | Case | custom | 57 | 24 | 42.1% |
| fi-manual | Case | custom | 7 | 6 | 85.7% |
| fi-manual | Case | omorfi | 57 | 30 | 52.6% |
| fi-manual | Case | omorfi | 7 | 7 | 100.0% |
| fi-manual | Clitic | basic | 1 | 0 | 0.0% |
| fi-manual | Clitic | custom | 1 | 0 | 0.0% |
| fi-manual | Clitic | omorfi | 1 | 1 | 100.0% |
| fi-manual | Degree | basic | 4 | 0 | 0.0% |
| fi-manual | Degree | custom | 4 | 0 | 0.0% |
| fi-manual | Degree | omorfi | 4 | 4 | 100.0% |
| fi-manual | InfForm | basic | 2 | 0 | 0.0% |
| fi-manual | InfForm | custom | 2 | 0 | 0.0% |
| fi-manual | InfForm | omorfi | 2 | 2 | 100.0% |
| fi-manual | Mood | basic | 3 | 1 | 33.3% |
| fi-manual | Mood | basic | 1 | 1 | 100.0% |
| fi-manual | Mood | custom | 1 | 1 | 100.0% |
| fi-manual | Mood | custom | 3 | 1 | 33.3% |
| fi-manual | Mood | omorfi | 3 | 3 | 100.0% |
| fi-manual | Mood | omorfi | 1 | 1 | 100.0% |
| fi-manual | Number | basic | 8 | 7 | 87.5% |
| fi-manual | Number | basic | 34 | 24 | 70.6% |
| fi-manual | Number | custom | 34 | 24 | 70.6% |
| fi-manual | Number | custom | 8 | 7 | 87.5% |
| fi-manual | Number | omorfi | 8 | 8 | 100.0% |
| fi-manual | Number | omorfi | 34 | 34 | 100.0% |
| fi-manual | Number[psor] | basic | 3 | 0 | 0.0% |
| fi-manual | Number[psor] | basic | 2 | 0 | 0.0% |
| fi-manual | Number[psor] | custom | 3 | 0 | 0.0% |
| fi-manual | Number[psor] | custom | 2 | 0 | 0.0% |
| fi-manual | Number[psor] | omorfi | 3 | 3 | 100.0% |
| fi-manual | Number[psor] | omorfi | 2 | 2 | 100.0% |
| fi-manual | PartForm | basic | 1 | 0 | 0.0% |
| fi-manual | PartForm | custom | 1 | 0 | 0.0% |
| fi-manual | PartForm | omorfi | 1 | 1 | 100.0% |
| fi-manual | Person | basic | 1 | 1 | 100.0% |
| fi-manual | Person | basic | 3 | 1 | 33.3% |
| fi-manual | Person | custom | 3 | 1 | 33.3% |
| fi-manual | Person | custom | 1 | 1 | 100.0% |
| fi-manual | Person | omorfi | 1 | 1 | 100.0% |
| fi-manual | Person | omorfi | 3 | 3 | 100.0% |
| fi-manual | Person[psor] | basic | 5 | 0 | 0.0% |
| fi-manual | Person[psor] | basic | 3 | 0 | 0.0% |
| fi-manual | Person[psor] | custom | 3 | 0 | 0.0% |
| fi-manual | Person[psor] | custom | 5 | 0 | 0.0% |
| fi-manual | Person[psor] | omorfi | 3 | 3 | 100.0% |
| fi-manual | Person[psor] | omorfi | 5 | 5 | 100.0% |
| fi-manual | Tense | basic | 2 | 0 | 0.0% |
| fi-manual | Tense | basic | 1 | 1 | 100.0% |
| fi-manual | Tense | custom | 1 | 1 | 100.0% |
| fi-manual | Tense | custom | 2 | 0 | 0.0% |
| fi-manual | Tense | omorfi | 1 | 1 | 100.0% |
| fi-manual | Tense | omorfi | 2 | 2 | 100.0% |
| fi-manual | VerbForm | basic | 6 | 0 | 0.0% |
| fi-manual | VerbForm | basic | 1 | 0 | 0.0% |
| fi-manual | VerbForm | custom | 1 | 0 | 0.0% |
| fi-manual | VerbForm | custom | 6 | 0 | 0.0% |
| fi-manual | VerbForm | omorfi | 6 | 6 | 100.0% |
| fi-manual | VerbForm | omorfi | 1 | 1 | 100.0% |
| fi-manual | Voice | basic | 1 | 0 | 0.0% |
| fi-manual | Voice | basic | 6 | 0 | 0.0% |
| fi-manual | Voice | custom | 1 | 0 | 0.0% |
| fi-manual | Voice | custom | 6 | 0 | 0.0% |
| fi-manual | Voice | omorfi | 6 | 6 | 100.0% |
| fi-manual | Voice | omorfi | 1 | 1 | 100.0% |

## Stratified accuracy

Each row is one (dataset, parser, bucket) slice. ExpectedTokens is the
denominator for accuracy; empty buckets are omitted. See stratify.go for
the bucket definitions.

### By UPOS bucket

| Dataset | Run | Parser | Bucket | Tokens | Lemma | POS | Lemma+POS | Full |
|---|---|---|---|---:|---:|---:|---:|---:|
| fi-core | 20260507T125105Z | basic | open | 18 | 94.4% | 94.4% | 94.4% | 50.0% |
| fi-core | 20260507T125105Z | basic | closed | 1 | 0.0% | 100.0% | 0.0% | 0.0% |
| fi-core | 20260507T125105Z | basic | propn | 1 | 0.0% | 0.0% | 0.0% | 0.0% |
| fi-core | 20260507T125105Z | custom | open | 18 | 94.4% | 94.4% | 94.4% | 50.0% |
| fi-core | 20260507T125105Z | custom | closed | 1 | 0.0% | 100.0% | 0.0% | 0.0% |
| fi-core | 20260507T125105Z | custom | propn | 1 | 0.0% | 0.0% | 0.0% | 0.0% |
| fi-core | 20260507T125105Z | omorfi | open | 18 | 94.4% | 88.9% | 88.9% | 88.9% |
| fi-core | 20260507T125105Z | omorfi | closed | 1 | 0.0% | 100.0% | 0.0% | 0.0% |
| fi-core | 20260507T125105Z | omorfi | propn | 1 | 0.0% | 0.0% | 0.0% | 0.0% |
| fi-grammar | 20260507T125127Z | basic | open | 154 | 98.1% | 99.4% | 98.1% | 51.3% |
| fi-grammar | 20260507T125127Z | basic | propn | 2 | 0.0% | 0.0% | 0.0% | 0.0% |
| fi-grammar | 20260507T125127Z | custom | open | 154 | 98.1% | 99.4% | 98.1% | 51.3% |
| fi-grammar | 20260507T125127Z | custom | propn | 2 | 0.0% | 0.0% | 0.0% | 0.0% |
| fi-grammar | 20260507T125127Z | omorfi | open | 154 | 96.8% | 96.1% | 95.5% | 94.8% |
| fi-grammar | 20260507T125127Z | omorfi | propn | 2 | 0.0% | 100.0% | 0.0% | 0.0% |
| fi-manual | 20260507T125513Z | basic | open | 67 | 58.2% | 82.1% | 55.2% | 31.3% |
| fi-manual | 20260507T125513Z | basic | propn | 3 | 0.0% | 0.0% | 0.0% | 0.0% |
| fi-manual | 20260507T125620Z | basic | open | 9 | 88.9% | 100.0% | 88.9% | 44.4% |
| fi-manual | 20260507T125513Z | custom | open | 67 | 85.1% | 89.6% | 83.6% | 34.3% |
| fi-manual | 20260507T125513Z | custom | propn | 3 | 0.0% | 0.0% | 0.0% | 0.0% |
| fi-manual | 20260507T125620Z | custom | open | 9 | 88.9% | 100.0% | 88.9% | 44.4% |
| fi-manual | 20260507T125513Z | omorfi | open | 67 | 77.6% | 79.1% | 77.6% | 56.7% |
| fi-manual | 20260507T125513Z | omorfi | propn | 3 | 0.0% | 33.3% | 0.0% | 0.0% |
| fi-manual | 20260507T125620Z | omorfi | open | 9 | 100.0% | 100.0% | 100.0% | 100.0% |

### By OOV (in-dict vs unresolved)

| Dataset | Run | Parser | Bucket | Tokens | Lemma | POS | Lemma+POS | Full |
|---|---|---|---|---:|---:|---:|---:|---:|
| fi-core | 20260507T125105Z | basic | in-dict | 19 | 89.5% | 94.7% | 89.5% | 47.4% |
| fi-core | 20260507T125105Z | basic | oov | 1 | 0.0% | 0.0% | 0.0% | 0.0% |
| fi-core | 20260507T125105Z | custom | in-dict | 20 | 85.0% | 90.0% | 85.0% | 45.0% |
| fi-core | 20260507T125105Z | omorfi | in-dict | 20 | 85.0% | 85.0% | 80.0% | 80.0% |
| fi-grammar | 20260507T125127Z | basic | in-dict | 155 | 97.4% | 98.7% | 97.4% | 51.0% |
| fi-grammar | 20260507T125127Z | basic | oov | 1 | 0.0% | 0.0% | 0.0% | 0.0% |
| fi-grammar | 20260507T125127Z | custom | in-dict | 155 | 97.4% | 98.7% | 97.4% | 51.0% |
| fi-grammar | 20260507T125127Z | custom | oov | 1 | 0.0% | 0.0% | 0.0% | 0.0% |
| fi-grammar | 20260507T125127Z | omorfi | in-dict | 156 | 95.5% | 96.2% | 94.2% | 93.6% |
| fi-manual | 20260507T125513Z | basic | in-dict | 33 | 97.0% | 97.0% | 97.0% | 54.5% |
| fi-manual | 20260507T125513Z | basic | oov | 37 | 18.9% | 62.2% | 13.5% | 8.1% |
| fi-manual | 20260507T125620Z | basic | in-dict | 8 | 87.5% | 100.0% | 87.5% | 37.5% |
| fi-manual | 20260507T125620Z | basic | oov | 1 | 100.0% | 100.0% | 100.0% | 100.0% |
| fi-manual | 20260507T125513Z | custom | in-dict | 60 | 93.3% | 95.0% | 93.3% | 38.3% |
| fi-manual | 20260507T125513Z | custom | oov | 10 | 10.0% | 30.0% | 0.0% | 0.0% |
| fi-manual | 20260507T125620Z | custom | in-dict | 9 | 88.9% | 100.0% | 88.9% | 44.4% |
| fi-manual | 20260507T125513Z | omorfi | in-dict | 55 | 94.5% | 98.2% | 94.5% | 69.1% |
| fi-manual | 20260507T125513Z | omorfi | oov | 15 | 0.0% | 0.0% | 0.0% | 0.0% |
| fi-manual | 20260507T125620Z | omorfi | in-dict | 9 | 100.0% | 100.0% | 100.0% | 100.0% |

### By Compoundness

| Dataset | Run | Parser | Bucket | Tokens | Lemma | POS | Lemma+POS | Full |
|---|---|---|---|---:|---:|---:|---:|---:|
| fi-core | 20260507T125105Z | basic | simple | 20 | 85.0% | 90.0% | 85.0% | 45.0% |
| fi-core | 20260507T125105Z | custom | simple | 20 | 85.0% | 90.0% | 85.0% | 45.0% |
| fi-core | 20260507T125105Z | omorfi | simple | 20 | 85.0% | 85.0% | 80.0% | 80.0% |
| fi-grammar | 20260507T125127Z | basic | compound | 1 | 100.0% | 100.0% | 100.0% | 0.0% |
| fi-grammar | 20260507T125127Z | basic | simple | 155 | 96.8% | 98.1% | 96.8% | 51.0% |
| fi-grammar | 20260507T125127Z | custom | compound | 1 | 100.0% | 100.0% | 100.0% | 0.0% |
| fi-grammar | 20260507T125127Z | custom | simple | 155 | 96.8% | 98.1% | 96.8% | 51.0% |
| fi-grammar | 20260507T125127Z | omorfi | compound | 1 | 100.0% | 100.0% | 100.0% | 0.0% |
| fi-grammar | 20260507T125127Z | omorfi | simple | 155 | 95.5% | 96.1% | 94.2% | 94.2% |
| fi-manual | 20260507T125513Z | basic | compound | 2 | 50.0% | 100.0% | 50.0% | 50.0% |
| fi-manual | 20260507T125513Z | basic | simple | 68 | 55.9% | 77.9% | 52.9% | 29.4% |
| fi-manual | 20260507T125620Z | basic | simple | 9 | 88.9% | 100.0% | 88.9% | 44.4% |
| fi-manual | 20260507T125513Z | custom | compound | 27 | 88.9% | 96.3% | 88.9% | 22.2% |
| fi-manual | 20260507T125513Z | custom | simple | 43 | 76.7% | 79.1% | 74.4% | 39.5% |
| fi-manual | 20260507T125620Z | custom | compound | 1 | 100.0% | 100.0% | 100.0% | 100.0% |
| fi-manual | 20260507T125620Z | custom | simple | 8 | 87.5% | 100.0% | 87.5% | 37.5% |
| fi-manual | 20260507T125513Z | omorfi | compound | 2 | 50.0% | 50.0% | 50.0% | 50.0% |
| fi-manual | 20260507T125513Z | omorfi | simple | 68 | 75.0% | 77.9% | 75.0% | 54.4% |
| fi-manual | 20260507T125620Z | omorfi | simple | 9 | 100.0% | 100.0% | 100.0% | 100.0% |

