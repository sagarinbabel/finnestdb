# Parser comparison (20260506T203804Z, parsers: basic,custom)

| Dataset | Cases | Parser | Lemma | POS | Grammar | Full | Coverage | Avg ms |
|---|---:|---|---:|---:|---:|---:|---:|---:|
| fi-core | 6 | basic | 85.0% | 90.0% | 0.0% | 35.0% | 91.3% | 0.0 |
| fi-core | 6 | custom | 85.0% | 90.0% | 0.0% | 35.0% | 100.0% | 0.0 |
| fi-grammar | 80 | basic | 96.8% | 98.1% | 0.0% | 51.3% | 99.7% | 0.0 |
| fi-grammar | 80 | custom | 97.4% | 98.7% | 1.4% | 51.9% | 100.0% | 0.0 |
| fi-manual | 4 | basic | 88.9% | 100.0% | 0.0% | 55.6% | 91.7% | 0.0 |
| fi-manual | 4 | custom | 88.9% | 100.0% | 0.0% | 55.6% | 100.0% | 0.0 |
| fi-manual | 4 | basic | 88.9% | 100.0% | 0.0% | 55.6% | 91.7% | 0.0 |
| fi-manual | 4 | custom | 88.9% | 100.0% | 0.0% | 55.6% | 100.0% | 0.0 |

## Head-to-head deltas (parser - first parser, in pts)

| Dataset | Parser | Δ Lemma | Δ POS | Δ Coverage |
|---|---|---:|---:|---:|
| fi-core | custom vs basic | +0.0 | +0.0 | +8.7 |
| fi-grammar | custom vs basic | +0.6 | +0.6 | +0.3 |
| fi-manual | custom vs basic | +0.0 | +0.0 | +8.3 |
| fi-manual | custom vs basic | +0.0 | +0.0 | +8.3 |
