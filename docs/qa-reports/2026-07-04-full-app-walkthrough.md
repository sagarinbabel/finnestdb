# Full app walkthrough audit - 2026-07-04

Scope: local `main` after PR #285 was merged, using the real local SQLite DB,
real Go server, real parser artifacts, and Chromium/Playwright against
`http://127.0.0.1:8098`.

This is not a proof that every possible app state is perfect. I interpreted
"full coverage" as: every visible public/user/admin route, every primary
documented launch journey in `docs/GO_LIVE_CHECKLIST.md` and
`docs/USER_FLOWS.md`, all major visible buttons/controls on those routes, and
release-gate commands that can be run locally.

## Result

Not ready for go-live yet.

The core product flows run: anonymous parse/export, registration, sign-in/out,
dashboard catalog, signed-in inspect, deck save/detail/rename/review, review
ratings and secondary actions, vocab import, Anki setup/settings fallback,
language switching, parse history dialogs, admin workbench, admin feedback
review, safe correction-issue quarantine/restore, and admin user toggles.

The launch decision is blocked by release hygiene and checklist gaps:

- RC manifest is not fully automated: 5 pending cases remain and 13 generated
  RC Playwright cases are skipped.
- Embedded catalog difficulty review is incomplete: 3 of 6 shipped entries are
  still `difficulty_review: pending`, all Estonian.
- `make db-invariants` exits 0 but reports orphan deck rows:
  210 `occurrence_orphan_decks` and 24 `sentences_orphan_decks`.
- Corpus pipeline `govulncheck` does not load due a missing `go.sum` entry.
- There is no visible account-deletion UI; only the API endpoint was verified.
- Several docs/comments still state the anonymous cap is 20,000 characters
  while runtime/default UI are 300,000.

## Browser Evidence

Raw local artifacts:

- `localdata/qa/20260704141202-full-browser-walkthrough.json`
- `localdata/qa/20260704141801-targeted-browser-followup.json`
- `localdata/qa/20260704142036-admin-browser-followup.json`

The first pass had 10 pass / 9 fail / 1 gap. The failures after the Anki step
were harness-order failures: the expected unreachable-Anki setup modal remained
open and blocked later clicks. The targeted follow-ups re-ran those blocked
flows in fresh state.

Verified in browser:

- Anonymous landing/nav/theme/about.
- Anonymous FI paste -> parse -> Read popover -> Words filters/sort -> copy
  list -> CSV download.
- Anonymous ET paste -> parse.
- Anonymous gated `/inspect` and `/admin/workbench` redirect to sign-in.
- Fresh registration: `qa-full-20260704141202@example.com`.
- Dashboard cold-start catalog -> "Read this text" -> auto-parse.
- Signed-in Inspect FI -> read popover mark-known -> Words filter/sort/ignore
  -> parser feedback submit -> save deck.
- Deck list -> detail -> rename -> review.
- Review: skip, flag, mark known, ignore, Again, Hard, Good, Easy.
- Vocab: paste import, file import, remove one word, delete-all confirmation
  cancel.
- Anki: setup instructions, copy config, settings toggles/reset, unreachable
  AnkiConnect path.
- Languages page and nav language dropdown.
- Inspect mismatch warning and explicit switch.
- History page per-row delete dialog and delete-all dialog, both canceled.
- Mobile nav open and route.
- Sign out and sign back in.
- Admin login `qa-admin-full-audit@example.com`.
- Admin workbench Basic and Custom parser buttons.
- Admin feedback filters and follow-up review action.
- Admin correction issue quarantine and restore on safe unique surface
  `zzqaadmin20260704142036`.
- Admin users toggle for the QA user, restored to original state.

Not verified in browser:

- Account deletion by clicking UI: no visible profile/settings/delete-account
  route or button exists. `make live-api-smoke` verified `DELETE /api/me`.
- Full Anki import from a live Anki desktop collection: local AnkiConnect was
  not running. The UI setup/settings/error path was verified; mocked Playwright
  tests cover the import wizard logic.

## Automated Checks

| Check | Result | Notes |
|---|---:|---|
| `git diff --exit-code origin/main HEAD -- . ':!gencatalog'` | PASS | Main matched origin after PR #285 merge. |
| `make doctor` | PASS | DB, FI/ET forms, FST tables, libvoikko, ET hfstol, Omorfi, EstNLTK, Ekilex shards, UD cache, frequency baselines, Rust dylib present. |
| `go test ./...` | PASS | All Go packages passed. |
| `go vet ./...` | PASS | No vet findings. |
| `go test -race ./internal/api ./internal/auth ./internal/store` | PASS | macOS linker warnings only. |
| `cd web && npm run build` | PASS | TypeScript build completed. |
| `cd web && npm test` | PASS with skips | 174 passed, 13 skipped. |
| `cd web && npm audit --audit-level=moderate` | PASS | 0 vulnerabilities. |
| `cd parser && cargo audit` | PASS | 15 crate dependencies scanned; exit 0. |
| Root `govulncheck` | PASS | 0 reachable vulnerabilities; 18 required-module vulns not called. |
| `corpus_pipeline` `govulncheck` | FAIL | Package loading fails: missing `go.sum` entry for `github.com/open-spaced-repetition/go-fsrs/v3`. |
| `make gen-catalog-check` | PASS | Catalog is up to date. |
| `make first-experience-rc` | NOT CLEAN | Exit 0, but summary is 8 passed, 5 pending, 5 Playwright; generated Playwright run is 3 passed, 13 skipped. |
| `BASE_URL=http://127.0.0.1:8097 make live-api-smoke` | PASS | 15/15 against local production-mode server. |
| `make db-invariants` | NOT CLEAN | Integrity OK, but nonzero orphan deck rows. |
| `make purge-parse-context PURGE_PARSE_CONTEXT_FLAGS=-dry-run` | PASS | 5 purgeable source-text rows before 2026-06-04T14:02:11Z UTC. |
| `make compare-parsers` | PASS with known delta | Reports written under `reports/parser-eval/20260704T140404Z-*`. |
| `make compare-parsers-et` | PASS | Reports written under same timestamp; local UD ET sets included. |

## Parser Baseline Deltas

Compared current `custom` reports to frozen `docs/baselines/2026-05-12b-T1606Z`
where that baseline exists:

- ET committed sets: no lemma/POS/full/coverage deltas.
- FI core/grammar/manual: no lemma/POS/full deltas.
- FI analyzer traps: no lemma/POS/full deltas; coverage -0.78pp.
- UD FI FTB: lemma +0.02pp, POS +0.05pp, full +0.04pp.
- UD FI OOD: lemma -0.22pp, POS -0.01pp, full -0.06pp, coverage -0.11pp.
- UD FI PUD: no headline deltas; coverage -0.01pp.
- UD FI TDT: lemma +0.04pp, POS +0.03pp, full +0.02pp.
- Local-only UD ET reports have no `2026-05-12b` committed baseline to diff
  against.

The UD FI OOD dip matches the previously documented small dictionary-data drift
pattern, but it still needs explicit launch justification/re-freeze if this DB
is the production candidate.

## Findings

### P0 - RC pack is still incomplete

`testdata/first-experience-rc/manifest.json` has 18 cases:

- 8 `parser`
- 5 `playwright`
- 5 `pending`

Pending cases:

- `fi-anonymous-demo`
- `et-anonymous-demo`
- `fi-known-word-import`
- `et-known-word-import`
- `et-parser-feedback`

`make first-experience-rc` exits 0 despite these pending/skipped cases, so a
green command is easy to misread as a complete release candidate pass.

### P0 - Embedded catalog difficulty review is incomplete

`internal/catalog/data/catalog.json` currently has 6 entries:

- 3 `approved`
- 3 `pending`

Pending entries:

- `et-tallinn-vanalinn-article`
- `et-mesipuu-poem`
- `et-linnu-keel-story`

`docs/GO_LIVE_CHECKLIST.md` requires human difficulty review for every shipped
FI and ET text before go-live.

### P1 - DB invariant target reports orphan rows but still exits 0

`make db-invariants` reported:

- `integrity_check`: `ok`
- `occurrence_orphan_decks`: `210`
- `sentences_orphan_decks`: `24`

The orphan rows are concentrated on missing deck IDs 11-16:

- 35 orphan `occurrence` rows per deck ID.
- 4 orphan `sentences` rows per deck ID.

Earlier launch verification docs treat zero orphans as the pass condition.
Either the production-candidate DB needs cleanup or the invariant script needs
to fail nonzero on these counts so release automation cannot miss them.

### P1 - Account deletion has no clickable UI

The API is implemented and `make live-api-smoke` confirms account deletion and
session invalidation. I could not find or click a visible account deletion
flow. `web/index.html`/`web/app.ts` expose sign-out, but no profile/settings
route or delete-account button.

This is a product/readiness issue if go-live requires a self-serve data
deletion control. `docs/USER_FLOWS.md` currently marks the profile page as out
of scope, so the product decision needs to be explicit.

### P2 - Anonymous cap docs/comments are stale

Runtime and UI default to 300,000 characters:

- `internal/api/handlers.go`: `defaultAnonMaxChars = 300_000`
- `web/app.ts`: `DEFAULT_ANON_MAX_CHARS = 300_000`
- landing UI shows `0 / 300,000`

Stale references still say 20,000:

- `docs/FEATURES.md`
- `docs/USER_FLOWS.md`
- `docs/CHANGELOG.md`
- `docs/launch-readiness/2026-07-04-overnight-report.md`
- top comment in `web/app.ts`/built `web/app.js`

This is not a runtime bug, but it will mislead deploy/readiness decisions.

### P2 - `corpus_pipeline` vulnerability scan does not load

Root `govulncheck` is clean. Running the same scan inside `corpus_pipeline/`
fails before analysis:

```text
missing go.sum entry for module providing package github.com/open-spaced-repetition/go-fsrs/v3
```

The separate module has `replace finnestdb => ..` and reaches the root module
without carrying the root module's newer dependency checksums.

### P2 - Production-only launch work remains outside local verification

I did not and cannot truthfully mark these done from local browser QA:

- Re-run the 1,000-concurrent load test on the production host.
- Confirm production monitoring/alerting on parser latency, queue rejection,
  5xx, auth failures, and DB readiness.
- Run `make live-api-smoke` against the real release-candidate host, not only a
  local production-mode server.
- Complete the human ET catalog review.

## Conclusion

The app is much healthier than the checklist gaps suggest: the main learner and
admin journeys work end to end locally. But it is not deploy/go-live ready until
the RC manifest, catalog review, DB orphan rows, release scan failure, and
account-deletion product decision are resolved or explicitly accepted.
