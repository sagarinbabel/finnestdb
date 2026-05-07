> **Provenance.** Snapshot taken on the head of `claude/pr1-vfst-go-runtime` before merge to main, capturing the FST stack at "PR 1/4 (VFST runtime, FI) just landed." Restored to main on 2026-05-07 from git via `git show d6d82a7^:docs/baselines/<file>` so the FST stack lift is attributable per-PR — see [`../PARSER_EVOLUTION.md`](../PARSER_EVOLUTION.md) §2026-05-06f.
> Cleanup commits between this snapshot and PR4 merge did not change measured numbers (verified against `2026-05-06-final-*.json`).

# Parser comparison (20260506T201010Z, parsers: basic,custom)

| Dataset | Cases | Parser | Lemma | POS | Grammar | Full | Coverage | Avg ms |
|---|---:|---|---:|---:|---:|---:|---:|---:|
| fi-core | 6 | basic | 85.0% | 90.0% | 0.0% | 35.0% | 91.3% | 0.0 |
| fi-core | 6 | custom | 85.0% | 90.0% | 0.0% | 35.0% | 95.7% | 0.0 |
| fi-grammar | 80 | basic | 96.8% | 98.1% | 0.0% | 51.3% | 99.7% | 0.0 |
| fi-grammar | 80 | custom | 97.4% | 98.7% | 1.4% | 51.9% | 100.0% | 0.0 |
| fi-manual | 4 | basic | 88.9% | 100.0% | 0.0% | 55.6% | 91.7% | 0.0 |
| fi-manual | 4 | custom | 88.9% | 100.0% | 0.0% | 55.6% | 100.0% | 0.0 |
| fi-manual | 4 | basic | 88.9% | 100.0% | 0.0% | 55.6% | 91.7% | 0.0 |
| fi-manual | 4 | custom | 88.9% | 100.0% | 0.0% | 55.6% | 100.0% | 0.0 |

## Head-to-head deltas (parser - first parser, in pts)

| Dataset | Parser | Δ Lemma | Δ POS | Δ Coverage |
|---|---|---:|---:|---:|
| fi-core | custom vs basic | +0.0 | +0.0 | +4.3 |
| fi-grammar | custom vs basic | +0.6 | +0.6 | +0.3 |
| fi-manual | custom vs basic | +0.0 | +0.0 | +8.3 |
| fi-manual | custom vs basic | +0.0 | +0.0 | +8.3 |
