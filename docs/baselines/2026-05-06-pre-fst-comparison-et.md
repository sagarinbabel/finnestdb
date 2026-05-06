# Estonian parser comparison (20260506T195202Z, parsers: basic,custom)

| Dataset | Cases | Parser | Lemma | POS | Grammar | Full | Coverage | Avg ms |
|---|---:|---|---:|---:|---:|---:|---:|---:|
| et-grammar | 50 | basic | 88.6% | 96.2% | 0.0% | 42.9% | 97.8% | 0.0 |
| et-grammar | 50 | custom | 88.6% | 96.2% | 2.0% | 42.9% | 98.9% | 0.0 |
| et-manual | 4 | basic | 77.8% | 77.8% | 0.0% | 11.1% | 83.3% | 0.0 |
| et-manual | 4 | custom | 77.8% | 77.8% | 0.0% | 11.1% | 91.7% | 0.0 |

## Head-to-head deltas (parser - first parser, in pts)

| Dataset | Parser | Δ Lemma | Δ POS | Δ Coverage |
|---|---|---:|---:|---:|
| et-grammar | custom vs basic | +0.0 | +0.0 | +1.1 |
| et-manual | custom vs basic | +0.0 | +0.0 | +8.3 |
