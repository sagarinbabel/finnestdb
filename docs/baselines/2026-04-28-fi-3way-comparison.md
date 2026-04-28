# Parser comparison (20260428T203047Z, parsers: basic,custom,omorfi)

| Dataset | Cases | Parser | Lemma | POS | Grammar | Full | Coverage | Avg ms |
|---|---:|---|---:|---:|---:|---:|---:|---:|
| fi-core | 6 | basic | 85.0% | 90.0% | 0.0% | 35.0% | 91.3% | 0.0 |
| fi-core | 6 | custom | 85.0% | 90.0% | 0.0% | 35.0% | 95.7% | 0.0 |
| fi-core | 6 | omorfi | 85.0% | 85.0% | 100.0% | 80.0% | 95.7% | 450.7 |
| fi-manual | 22 | basic | 44.3% | 75.7% | 0.0% | 32.9% | 69.6% | 0.0 |
| fi-manual | 22 | custom | 72.9% | 85.7% | 0.0% | 57.1% | 93.8% | 0.0 |
| fi-manual | 22 | omorfi | 71.4% | 81.4% | 60.0% | 68.6% | 89.7% | 466.5 |

## Head-to-head deltas (parser - first parser, in pts)

| Dataset | Parser | Δ Lemma | Δ POS | Δ Coverage |
|---|---|---:|---:|---:|
| fi-core | custom vs basic | +0.0 | +0.0 | +4.3 |
| fi-core | omorfi vs basic | +0.0 | -5.0 | +4.3 |
| fi-manual | custom vs basic | +28.6 | +10.0 | +24.2 |
| fi-manual | omorfi vs basic | +27.1 | +5.7 | +20.1 |
