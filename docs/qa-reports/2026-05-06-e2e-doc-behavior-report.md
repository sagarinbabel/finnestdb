# FinnEst E2E + Documentation Behavior Report

Date: 2026-05-06
Author: Codex (QA pass requested in chat)

## Scope

This report compiles:

1. Full browser E2E test results run today.
2. Documentation-to-runtime behavior checks.
3. A feature-focused validation matrix based on `docs/FEATURES.md`.
4. Additional findings and recommended follow-up actions.

Docs reviewed for expected behavior:

- `README.md`
- `docs/FEATURES.md`
- `docs/GETTING_STARTED.md`
- `docs/IMPLEMENTATION.md`
- `docs/CHANGELOG.md`
- `docs/GO_LIVE_CHECKLIST.md`

## Test Environment

- OS: macOS (darwin 25.3.0)
- Repository: `finnestdb`
- Browser automation: Playwright (`web/tests/parse-results.spec.ts`)
- Command: `cd web && npx playwright test`
- Test web server: auto-started by Playwright config on `http://localhost:8081`
- Backend startup logs observed:
  - FI dictionary loaded: 26,826,071 forms
  - ET dictionary loaded: 392,863 forms

## Full End-to-End Test Run (Today)

Run timestamp: 2026-05-06 20:04 local time (UTC+3)

- Total tests: 24
- Passed: 23
- Failed: 1
- Duration: ~45s

Failure:

- `tests/parse-results.spec.ts` -> `successful sign-in lands on dashboard`
- Failure reason: test only fills email, but current sign-in form requires password length >= 8. The dashboard never becomes active because form validation blocks submit.

Revalidation run after fixing test input (timestamp: 2026-05-06 20:14 local time):

- Total tests: 24
- Passed: 24
- Failed: 0
- Duration: ~39s

Applied test fix:

- `web/tests/parse-results.spec.ts`
  - In `successful sign-in lands on dashboard`, added `Password` input before clicking `Sign in`.

## Documentation Behavior Parity Findings

### Mismatch 1: Language warning semantics

Status: mismatch between docs and runtime.

- Docs currently describe the language mismatch warning as advisory and parse can continue.
- Runtime behavior blocks inspect submit until language is switched to detected language.

Impact:

- User-facing docs may mislead on expected Inspect behavior.

Recommendation:

- Update `README.md` and any related docs to state that mismatch is blocking in current alpha UX (with one-click switch CTA).

### Mismatch 2: Sign-in requirements wording

Status: partial mismatch between docs and runtime.

- Some docs say "Sign in with an email address".
- Runtime requires email plus password (>= 8 chars) in sign-in/create-account UI.

Impact:

- Onboarding docs under-specify required input; test assumptions can drift.

Recommendation:

- Update `README.md` and `docs/GETTING_STARTED.md` wording to "email + password (8+ chars)".

### Mismatch 3: Anonymous inspect expectations across docs

Status: inconsistent narrative.

- `docs/FEATURES.md` includes language implying anonymous paste/parse can be ephemeral.
- Runtime route guard redirects anonymous users trying `/#/inspect` to sign-in.

Impact:

- Product expectation is ambiguous for anonymous users.

Recommendation:

- Align docs to current implementation (or change implementation intentionally, then test and document that behavior).

### Mismatch 4: Documented ET ambiguity example (`joon`) not reproduced

Status: mismatch for the concrete example used in docs.

- `docs/FEATURES.md` uses ET `joon` as the canonical "multi-sense" example (noun + verb).
- Live API checks today produced only one candidate (`jooma`/VERB) in parse and deck-detail outputs for text `joon`.

Impact:

- The documentation example appears stronger than current runtime data behavior.

Recommendation:

- Either update docs to say this behavior is dictionary-coverage dependent, or investigate ET dictionary rows for `joon` noun mapping and restore the expected dual-candidate behavior.

## Feature Deep-Dive (Live API Checks)

To extend beyond mocked browser tests, additional live checks were run against a local server on `http://localhost:8080`:

1. Anonymous parse session persistence check: PASS
   - `POST /api/parse` as anonymous (`lang=ET`, `text=joon`) returned no `parse_id`.
2. Signed-in parse session persistence check: PASS
   - Registered test account and repeated parse with session cookie; response included `parse_id`.
3. Global cards across decks check: PASS
   - Created deck A (`joon`), marked its review card known, created deck B with same text.
   - `GET /api/me` showed both decks with `known=1` and `dashboard.known_count=1`, confirming global known-state propagation.
4. Multi-sense ambiguity check using documented sample (`joon`): FAIL
   - Created ET deck from `joon`; deck detail returned one word (`jooma`/VERB), not dual candidates.
   - This specifically conflicts with the featured docs example.

## Feature-Focused Validation Matrix (`docs/FEATURES.md`)

Legend:

- PASS: behavior confirmed by test evidence from today.
- FAIL: behavior contradicted by runtime.
- PARTIAL: not fully proven by current E2E test set.
- N/A: not directly testable from current UI/test harness.

1. Role-aware product surface (anonymous/user/admin): PASS
   - Anonymous restricted from inspect/admin routes; user/admin route guards validated.
2. Core loop `paste -> inspect -> correct -> deck -> review`: PASS
   - Inspect parse, correction modal, save deck, review flow all covered.
3. Inspect output includes lemmas/forms/definitions/token counts/grammar labels/examples: PASS
   - Results rendering and core fields validated in user/admin inspect tests.
4. Row-level Known/Ignore actions for logged-in users: PASS
   - State changes and request payloads validated.
5. Save parsed vocabulary as deck: PASS
   - Deck creation and subsequent deck listing validated.
6. Spaced-repetition review flow: PASS
   - Next card fetch + answer progression to empty state validated.
7. Known-word tracking and management: PASS
   - Import/remove known words validated in decks surface.
8. Progress counters (known/due): PASS
   - Dashboard known/due counters asserted for logged-in user.
9. Admin-only parser workbench access: PASS
   - Admin visibility and user restriction both validated.
10. Admin feedback triage queue: PASS
    - Admin feedback list loading and review action payload validated.
11. Language mismatch warning and switch action: PASS
    - Blocking behavior and switch-to-detected-language CTA validated.
12. Mobile responsiveness direction (375px): PASS
    - Mobile correction visibility and landing nav behavior validated.
13. "Cards are global across decks": PASS
    - Live check confirmed known-state carries across multiple decks containing same lemma.
14. Multi-sense ambiguous word handling (e.g., ET `joon`): FAIL
    - Live check with `joon` produced only one candidate card/lemma, not multiple senses.
15. Anonymous parse is ephemeral while signed-in parse is stored: PASS
    - Live check confirmed anonymous parse has no `parse_id`, signed-in parse includes `parse_id`.

## Additional Test-Suite Quality Finding

- The previous failing sign-in test was stale versus current UI validation.
- Fix has been applied and revalidated in the same day:
  - Fill password in `successful sign-in lands on dashboard` before submit.
  - Suite now passes 24/24.

## Overall Assessment

- The app behavior is mostly aligned with role-aware alpha docs and feature intent.
- Main gaps are documentation drift and one stale E2E test.
- Critical runtime regressions were not observed in most tested paths, but one featured ambiguity behavior is currently not reproducible.

## Action List

1. Update docs to match current language-warning behavior (blocking + switch CTA).
2. Update docs to specify password requirement in sign-in instructions.
3. Resolve anonymous Inspect expectation mismatch (docs vs implementation).
4. Update failing sign-in Playwright test to include password input.
5. Investigate ET dictionary coverage for `joon` noun sense and verify expected multi-candidate expansion path.
6. Add/keep dedicated E2E/API checks for:
   - ambiguous multi-sense lemma card creation
   - signed-in vs anonymous parse-session persistence
