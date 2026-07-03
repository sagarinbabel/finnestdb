# Decisions Log

_Reverse-chronological — newest decisions at the top. See [CHANGELOG.md](CHANGELOG.md) for revisions._

> **Note:** DECISIONS.md and CHANGELOG.md overlap by design — CHANGELOG records
> *what* changed; DECISIONS records *why* we chose to change it that way.
> Where a decision and a changelog entry describe the same event, both files
> cross-link.
>
> The consumer-alpha execution plan in [`../TODO.md`](../TODO.md) and the
> product framing in [`FEATURES.md`](FEATURES.md) take precedence over older
> decisions recorded here where they conflict. Older entries remain for
> historical context.
>
> The project roadmap formerly tracked in this file (Tracks A/B/C) has moved
> to TODO.md's "What's in main" / "What's not in main yet" sections. The
> Track A/B/C breakdown is preserved at the bottom of this file as historical
> record only.

This document tracks key architectural decisions and the reasoning behind
them. It serves as a journal of how the project evolves and why we made the
choices we did.

---

## Decision 29: Alpha go/no-go allows non-dangerous rough edges only

**Date:** 2026-07-03

### Context

The product-readiness grill settled that public alpha should not wait for a
fully polished product, but "rough edges are okay" is too vague to guide launch
decisions. The team needs a concrete definition of what is launch-blocking,
what can ship with documentation, and how future agents should know the bar.

### Decision

Public alpha can launch when the core journeys work end-to-end and every known
rough edge is classified as non-dangerous.

The canonical quality rubric is `docs/GO_LIVE_CHECKLIST.md` under **Alpha
Go/No-Go Rubric**. The launch issue ledger itself lives inside `TODO.md` under
**Alpha launch issue ledger**. Do not create a separate launch-issues document
unless the table becomes too large for `TODO.md` to remain readable.

The first experience should be excellent about 95% of the time before public
alpha. This is a product-quality bar, not only an uptime target. A first-run
path should usually feel credible, fast-enough, and trustworthy: no broken flow,
no misleading state, no obvious high-severity parser/card issue in the learner's
first screenful, and no latency/error behavior that makes the app feel
unreliable. Gate launch through a journey-first FI/ET release-candidate pack
covering anonymous demo, embedded text, own-text Inspect, save deck, first
review, known-word import, and parser feedback. The release-candidate pack
should be a checked-in, repeatable artifact with explicit FI/ET cases for
curated embedded texts, realistic pasted texts, known-word imports,
ambiguity/homograph handling, parser-feedback flows, deck save, and first
review. It should have one canonical manifest at
`testdata/first-experience-rc/manifest.json`; parser checks, `web/tests`
Playwright specs, and the manual walkthrough should all consume that same
manifest and fixtures. Build the manifest and a small skeleton runner as the
first alpha implementation task, before waiting for all missing flows to exist;
it may fail initially, but it should become the concrete launch bar as features
land. Expose the automated portion through one top-level command,
`make first-experience-rc`, which runs parser fixture checks and Playwright RC
specs, then prints the manual walkthrough path/instructions. Run it in two
parts: automated parser/browser checks for deterministic
behavior, plus a short manual product walkthrough for judgment calls about
trustworthiness, clarity, and first-screen credibility. Grade findings as
`blocker`, `serious`, or `minor`: blockers and serious trust-breaking findings
stop launch unless fixed or explicitly reclassified with evidence; minor
findings can ship only if they satisfy the non-dangerous rough-edge rubric and
are tracked in the launch issue ledger. After launch, validate the same bar with
privacy-preserving week-one alpha telemetry when available: journey
completion/drop-off, parse/deck/review errors, latency, retry/429/503 rates,
feedback/flag rate, quarantine-triggering reports, and language split. Do not
retain pasted source text merely to compute these metrics. Telemetry is
aggregate by default. Per-user event trails are allowed only for signed-in users,
only for product events needed to debug onboarding failures, and never for
pasted source text.

Minimal telemetry is not itself a public-alpha blocker. If it is not ready at
launch, record it in the post-launch roadmap and use server logs plus manual
feedback/admin review as the week-one fallback until telemetry lands.

A rough edge is **non-dangerous** only if all of these are true:

1. it does not create privacy, security, account, retention, or abuse risk;
2. it does not lose or corrupt learner data, review state, known-word state,
   source-retention state, or feedback/quarantine history;
3. it does not mislead the learner about what is known, saved, retained,
   deleted, reviewed, quarantined, or parser-confident;
4. it does not break an agreed core journey for either FI or ET;
5. it has a clear workaround, retry path, or honest UI explanation; and
6. it is documented in the launch issue list with owner, severity, affected
   journey/language, evidence, workaround, and revisit condition.

A **no-go blocker** is any issue that violates one of those conditions. Examples:

- auth/session/admin isolation bug;
- account deletion or retention behavior that does not match the product copy;
- source text stored when the UI says it is ephemeral;
- parser-feedback or quarantine failure that leaves confirmed-bad study content
  in circulation;
- deck/review/FSRS state loss or duplicated/incorrect due state;
- known-word import that silently records the wrong durable knowledge claim;
- FI/ET asymmetry that makes one language fail the equal-status journey;
- anonymous parse load that can starve signed-in review/deck usage;
- outage behavior that times out or corrupts state instead of returning clear
  retry behavior; or
- misleading parser-confidence/meaning-check UI.

Acceptable rough edges include cosmetic polish gaps, wording that is clear but
not elegant, minor extra clicks, limited non-core admin conveniences, incomplete
post-alpha roadmap features, or isolated parser imperfections that are
reportable and do not undermine the parser-feedback/quarantine safety path.

### Why

This lets alpha launch to learn from real users without allowing preventable
trust failures. The bar is not "beautiful"; the bar is "core journeys work,
truthful state is preserved, and known risk is bounded."

### Trade-off

The rubric creates more release discipline than a casual alpha. That is
intentional because the app handles user accounts, pasted source text, learning
history, and parser corrections.

### How to revisit

After the first hosted alpha cohort, revisit the categories using real incident
data and support feedback. Do not weaken privacy, data-integrity, or FI/ET
equal-status gates without an explicit decision.

---

## Decision 28: Finnish and Estonian launch with equal product status

**Date:** 2026-07-03

### Context

FinnEst has repeatedly invested in Finnish and Estonian as first-class
languages: shared parser infrastructure, shared dictionary schema, shared deck
and review model, external benchmark slots for both languages, corpus/frequency
work for both languages, and product flows that name both languages together.

The remaining product-readiness question is whether public alpha may present
one language as the primary language and the other as experimental if parser,
catalog, corpus, or review quality differs.

### Decision

Public alpha launches Finnish and Estonian with equal product status.

The product should not label one language as experimental, secondary, or
best-effort. Both languages must meet the same minimum public-alpha journey:

1. anonymous paste -> parse -> word list -> explore;
2. signed-in Inspect;
3. curated embedded texts;
4. deck save/add-to-deck;
5. review;
6. known-word import;
7. parser feedback;
8. admin triage/quarantine path; and
9. parser/eval observability.

If one language is weaker in a concrete area, track and fix the concrete
asymmetry. Do not convert it into a broad product label. For example, it is
acceptable to say internally "ET needs more curated embedded poems" or "FI
ambiguity cases are measured first because Sagar can validate Finnish"; it is
not acceptable to make Estonian feel like an add-on language in the product.

Before public alpha, run a journey-first parity audit. Start by walking the
same public-alpha learner/admin journeys in FI and ET, then attach metrics and
artifact checks under each step. Do not start with abstract metric tables that
can pass while one learner journey still feels weaker.

The audit should compare FI and ET across data, parser quality, corpus/catalog
readiness, known-word import behavior, deck/review behavior,
feedback/quarantine, UX copy, tests, and production artifacts. Every found
asymmetry should be classified as one of:

- **must fix before alpha** because it breaks the equal-status learner journey;
- **acceptable language-specific difference** because the languages genuinely
  differ; or
- **post-alpha improvement** because it does not change public product status.

### Why

Equal status is part of the product identity. FinnEst is not a Finnish app with
Estonian support bolted on; it is a Finnish-and-Estonian reading product.

### Trade-off

Equal status raises the launch bar. It may force extra ET/FI catalog work,
evaluation work, or UX cleanup before alpha. That is preferable to shipping a
product whose positioning contradicts years of first-class dual-language work.

### How to revisit

Only revisit if a concrete blocker makes one language impossible to support in
the same learner journey. Even then, prefer narrowing a specific feature for
both languages over demoting one language globally.

---

## Decision 27: Public alpha includes an anonymous parser demo with signed-in durability

**Date:** 2026-07-03

### Context

The product needs to prove that the parser is excellent, not only that the
signed-in learner loop exists. After Decision 26 settled open signup, the
follow-up question was how much value an unsigned visitor should receive before
creating an account, and how aggressively the hosted alpha should be planned
for load.

### Decision

Public alpha should include an anonymous, stateless parser demo:

1. paste text;
2. parse text;
3. get the parsed word list; and
4. explore the list.

That is the anonymous scope. All durable, personalized, or accountable actions
require sign-in: saving a deck, reviewing, importing known words, marking
known/ignored state, parser feedback/corrections, parse history, retained source
management, and account-level settings.

Anonymous parsing should have a stricter text-size limit than signed-in
parsing. The current signed-in cap is 1,500,000 characters. The anonymous demo
cap should be lower, configurable, enforced before expensive parser work, and
tuned through the 1,000-concurrent-user load test. If an unsigned visitor pastes
more than the anonymous demo cap, the UI should explain the limit and prompt
them to sign up for longer-text workflows.

Email verification should not block the first learner loop after signup. A new
user should be able to parse, save a deck, and start review immediately after
account creation. Verification can gate higher-risk or higher-trust actions:
high-volume parsing, repeated feedback submissions, exports if enabled, account
recovery, and any trust-weighted correction signal.

The initial hosted phase should be planned for roughly 1,000 concurrent users.
If load exceeds that, the app should degrade gracefully rather than fail
chaotically: throttle anonymous and oversized parses first, preserve core
signed-in review/deck actions as long as possible, return clear retry behavior,
and keep enough monitoring to know whether parser CPU, memory, database writes,
or feedback volume is the pressure point. The deployment should be easy to scale
with more servers and funding: keep anonymous parse state ephemeral, avoid
server-local user session assumptions, make parser concurrency explicit, and
prefer horizontally repeatable runtime artifacts.

### Why

The anonymous demo is the proof point for the "state-of-the-art parser" claim.
It lets a learner experience the parser before committing to an account, while
still keeping every feature that needs memory, accountability, or privacy
controls behind sign-in.

Planning for 1,000 concurrent users is a realistic alpha target without a
marketing budget. It also forces the right architecture questions early:
backpressure, rate limits, graceful overload, and scale-out shape.

### Trade-off

Anonymous parsing increases abuse and load risk compared with a fully
auth-gated product. The mitigation is a deliberately narrow anonymous surface:
paste/parse/list/explore only, lower text-size cap than signed-in parsing,
rate-limited, ephemeral, and first to be throttled under pressure.

### How to revisit

If anonymous parse abuse or infrastructure cost dominates real learner usage,
reduce anonymous limits, require signup after a small number of parses, or
temporarily gate the demo behind lightweight friction. Do not remove the
signed-in learner loop to compensate for anonymous load.

---

## Decision 26: Public alpha account access is open signup

**Date:** 2026-07-03

### Context

After settling that public alpha is signed-in learner alpha rather than an
anonymous-first browser parser, the remaining access question was whether the
hosted alpha should be invite-only, waitlist/manual allowlist, or open signup.

Open signup increases operational risk compared with an allowlist: abuse,
parser load, low-quality feedback, privacy support, account deletion, and admin
visibility all matter sooner. But the product is meant to be user-facing, and
the alpha needs to test real self-serve learner journeys rather than only a
small controlled cohort.

### Decision

Public alpha should allow self-serve open signup.

This does not change the signed-in learner product center: Decks, Review,
known-word state, and parser feedback remain signed-in product surfaces.
Decision 27 later adds a narrow anonymous browser parser demo; durable and
personalized behavior still requires an account.

Because signup is open, the public-alpha launch gates must include production
safety work that an invite-only alpha might otherwise defer:

- rate limiting and abuse controls around parse, auth, and feedback endpoints;
- account deletion and retention jobs;
- admin visibility into feedback/quarantine issues;
- email/password and OAuth hardening;
- privacy copy that matches actual parse-source retention behavior; and
- basic monitoring for parser load and feedback volume.

### Why

Open signup tests whether the product can explain itself and activate a learner
without manual onboarding. That is the product risk FinnEst needs to retire for
a real learner-facing alpha.

### Trade-off

Open signup raises support and abuse risk before the funnel is polished. The
mitigation is to make the safety work launch-gating, not to silently treat the
alpha as a private test.

### How to revisit

If hosted usage creates uncontrolled cost, abuse, or moderation load before the
safety gates are complete, temporarily close signup behind a waitlist. That is a
fallback operational mode, not the target product posture.

---

## Decision 25: Preserve learning history while removing faulty content from circulation

**Date:** 2026-07-03

### Context

The parser-feedback loop now has two related but different responsibilities:

1. improve future parses when admins accept concrete parser-identity fixes; and
2. protect learners from content already created from faulty analysis.

Current implementation only handles part of the first responsibility. Authenticated
learners can submit exact corrections, admins can triage them, and accepted
lemma/POS corrections write `custom_overrides` rows that affect future parser
lookups. The current schema does not have flag-only feedback, source-agnostic
correction overlay tables, a weekly Track B admin report/job, AI-assisted
review tooling, or a way to quarantine/re-render existing faulty deck/card
content.

### Decision

Do not rewrite learner or learning history. Past review events, ratings, due
state provenance, and "what the learner was shown at the time" should remain
auditable.

Do remove known-faulty learner-facing content from circulation after admin
acceptance. A minimal quarantine path is required before public alpha; rich
overlays and weekly AI-assisted triage can mature after that. Accepted feedback
should be able to:

1. write parser-identity fixes for future parser output;
2. suppress a bad occurrence/card/sentence/cue from review and new-card queues
   for all learners whose content matches the confirmed issue scope;
3. replace a faulty cue, sentence, explanation, or contextual sense through a
   correction overlay; or
4. mark affected content as needing reparse or manual rebuild.

These actions must not pretend the learner reviewed different content in the
past. They change what is shown from now on.

Feedback should roll up into global correction issues, not isolated per-user
complaints. One report creates or appends to an issue with reporter, timestamp,
source context, affected scope, status, quarantine action, fix action, fix
version, and reopen/regression events. Once an issue is confirmed, quarantine or
fixes apply globally to all matching learner content. Raw unreviewed reports
should normally not suppress content for everyone without admin confirmation or
a trusted threshold, but they should still be tracked as global evidence.

The default flow is report-first, quarantine-after-confirmation. A report
creates/updates the global issue immediately and appears in admin triage, but
content stays live until an admin takes one of these actions:

1. an admin chooses an emergency **Quarantine now** action with reason and
   explicit scope;
2. an admin accepts the correction and chooses a quarantine or overlay action.

For alpha, trusted thresholds are traceability-only. The system should collect
duplicate counts, distinct reporter counts, and threshold-candidate events such
as "X people reported this same thing," but it must not auto-quarantine globally
until real feedback quality is observed and the threshold policy is revisited.

For alpha, avoid a full correction platform. Keep `parse_feedback` as raw report
intake and add the smallest global issue layer needed for grouping and admin
state: a `correction_issues` table plus a `correction_issue_id` link from
`parse_feedback`. Defer separate `quarantine_targets` and rich event tables
unless the implementation needs them to preserve agreed traceability. The
minimum traceability can live on the issue row plus linked report rows:
report/duplicate counts, status, scope fingerprint, admin action fields, fix
version, and reopened/regression marker.

### Reasoning

Learners should not keep studying a card once the team knows it is faulty. But
mutating historical review records would make scheduler state, audit trails, and
learner trust worse. The right model is append-only/immutable history plus a
mutable rendering/circulation layer.

This also keeps parser fixes honest. A global `custom_overrides` row is only
safe for accepted parser-identity corrections. Meaning cues, contextual senses,
phrase boundaries, example quality, and card presentation problems belong in
their own overlay layers.

AI can help admins triage weekly feedback by summarizing context, comparing
dictionary/analyzer evidence, drafting candidate classifications, and preparing
proposed fixes. It must not directly author core linguistic truth. Human admins
accept, reject, or edit the final correction before any overlay, quarantine, or
lexical writeback is applied.

### Consequences

- Add a minimal faulty-content quarantine/suppression path before public alpha,
  not merely after launch.
- Add a correction-issue ledger so repeated reports can be identified as
  duplicate, fixed, missed-scope, or regression cases.
- Keep the alpha schema small: `parse_feedback` for raw intake plus a minimal
  `correction_issues` global state table linked from feedback. Do not introduce
  separate quarantine-target or rich event tables unless the implementation needs
  them for traceability.
- Add an emergency admin quarantine action with required reason, explicit scope,
  append-only event logging, and a rollback/fix path.
- Collect threshold evidence for future automation, but keep global quarantine
  admin-only for public alpha.
- Add source-agnostic correction overlays as described in
  [`CORRECTION_TAXONOMY.md`](CORRECTION_TAXONOMY.md).
- Weekly Track B reporting remains planned, not shipped. The first version can
  be an admin report/job, not a polished analytics dashboard.
- AI-assisted admin triage is allowed as draft support only; deterministic code
  and human approval own routing, writes, and acceptance.
- Review/deck queries should eventually skip quarantined content or render the
  accepted overlay, while preserving existing card/review history.
- Learner-facing quarantine UX should be quiet. Quarantined content disappears
  from review/new-card queues globally. Admins keep full traceability; learners
  only see neutral deck-detail copy such as "Removed from study after review" if
  the absence would otherwise be confusing.
- Current learner-facing stats should exclude quarantined content: deck word
  counts, due counts, new-card counts, comprehension estimates, and marginal
  unlocks should reflect only currently studyable content. Historical/admin
  views can preserve quarantined content for audit.
- Quarantined content should be restored after fix by default. Create a new
  study item only when the learning target identity changes, such as wrong
  lemma/POS, wrong sense, homograph split, phrase/MWE replacement, or invalid
  target retirement.
- Restored content keeps its existing scheduler state when the learning target
  identity is unchanged. Reset/reintroduce scheduling only when a fix creates a
  new learning target identity.
- Admin triage should require a simple alpha classification, not the full
  correction taxonomy: `parser issue`, `bad card content`,
  `source/extraction issue`, or `not sure`. Rich taxonomy labels can be optional
  until the admin workflow has real data.
- Alpha admin UI should not include a broad in-app fix editor. It should support
  classification, notes, report grouping, and global quarantine. Parser-identity
  fixes can use the existing accepted lemma/POS `custom_overrides` path; richer
  fixes remain manual code/data changes or future overlay work.
- Alpha should use one combined admin feedback/issues queue, not a separate
  Issues page. Add issue-aware filters/statuses such as `submitted`,
  `needs review`, `quarantined`, `fixed`, and `reopened`; split the UI later if
  volume or workflow complexity requires it.

### Source

See `docs/grill-sessions/2026-07-03-product-readiness.md` questions 26-38 for the
working discussion.

---

## Decision 24: Promote grill decisions into the durable doc set

**Date:** 2026-07-03

### Context

The product-readiness grill process is useful because it preserves the exact
question trail and the reasoning behind product decisions. But FinnEst already
has several durable documentation surfaces with different jobs, now summarized
in [`docs/INDEX.md` "Canonical doc roles"](INDEX.md#canonical-doc-roles). Future
agents should not need to mine git history or reconstruct document ownership
from scattered prose.

Leaving all product direction only in a grill-session transcript would make
future implementation agents depend on conversational context. Merging
everything into one giant document would make decisions harder to find and
maintain.

### Decision

Use this promotion workflow for grill sessions and future product-scope
expansion sessions:

1. Record question-by-question exploration in the active
   `docs/grill-sessions/` file.
2. Promote stable decisions to `docs/DECISIONS.md` after each 10-question batch,
   or sooner when a decision changes product direction, schema, or launch scope.
3. Use the canonical role table in `docs/INDEX.md` to decide which other durable
   docs to update.
4. Update README links and `docs/INDEX.md` when document roles or navigation
   change.
5. Before implementation handoff, consolidate navigation into existing entry
   points: `TODO.md` for execution and launch issue classification,
   `docs/INDEX.md` for doc roles, `README.md` for top-level navigation, and
   `AGENTS.md` for agent instructions. Do not create a new document when an
   existing source-of-truth section can carry the information clearly.
6. Keep grill-session docs as archived audit trails. Do not execute directly
   from them after promotion; use `DECISIONS.md`, `CONTEXT.md`, `TODO.md`, and
   the relevant specs for current direction.

### Reasoning

This keeps the useful parts of a live product grill without turning a transcript
into the execution system. The grill doc answers "how did we get here?"
`DECISIONS.md` answers "what is settled and why?" `TODO.md` answers "what
should be built?" `CONTEXT.md` answers "what language and mental model should
agents use?"

The workflow also reduces context loss for future model sessions. Agents can
start from `AGENTS.md`, read `CONTEXT.md` and the relevant `DECISIONS.md`
entries, then use `TODO.md` and specs for implementation. The grill session
remains available when they need the original question trail or exact user
wording.

### Consequences

- `AGENTS.md` now records this workflow for future agents.
- The active grill session records this workflow in its protocol.
- Stable decisions from the 2026-07-03 product-readiness grill are promoted
  here rather than being left only in the grill table.
- The current public-alpha handoff starts from `TODO.md` "LLM handoff read
  order" and avoids adding a separate launch-issues document.

---

## Decision 23: Public alpha learner model is surface-first with guided cold start and narrow FSRS

**Date:** 2026-07-03

### Context

The 2026-07-03 product-readiness grill re-evaluated whether FinnEst is ready as
a learner-facing product, especially around signed-in onboarding, embedded
texts, known-word import, starter decks, review cards, and scheduling.

Current implementation reality:

- The browser alpha is signed-in first.
- Inspect, deck creation, known/ignored state, review, AnkiConnect import, and
  parse-history UI exist.
- Known-word imports accept submitted surface strings but persist resolved
  `(lemma, POS)` rows in `user_known_lemmas`.
- Review cards and deck creation are currently lemma-backed.
- Review scheduling is a hand-rolled step scheduler stored through
  `card_state.fsrs_json`, not real FSRS.
- Frequency baseline artifacts already support top-N inflected surface-form
  analysis, but the cold-start starter experience is not built.

### Decision

Use this alpha product direction:

1. **Public alpha centers the signed-in learner loop.** Optimize the durable
   product for Inspect -> deck -> review -> known/ignored state -> feedback.
   Decision 27 adds a narrow anonymous parser demo, but durable learning remains
   signed-in.
2. **Cold start offers both learner-owned text and curated embedded text.**
   The embedded catalog should be a small checked-in catalog, generated from
   local corpus tooling, with lazy-loaded full text fixtures. Target shape:
   FI/ET x stories/articles/poems x Easy/Medium/Hard x two texts per bucket
   when fully populated. Prefer redistributable full coherent texts when size
   and license allow it.
3. **Difficulty is computed, then human sanity-checked.** Use deterministic
   parser/corpus features such as length, sentence complexity, frequency
   profile, dictionary coverage, unresolved rate, ambiguity, FEATS variety, and
   genre-relative scoring. Finnish gets Sagar review; Estonian gets an Estonian
   reviewer.
4. **Known vocabulary is surface-first.** A known surface form is learner
   evidence. It does not automatically imply the lemma or other inflected forms
   are known. Lemma/POS analysis is derived support for lookup, grouping,
   explanation, and implementation transition, not the learner-facing knowledge
   unit.
5. **Alpha review cards are surface-form-in-context cards.** Show the
   lemma/dictionary entry as supporting explanation, not as the primary thing the
   learner is claiming to know. Migrate this card identity before attaching real
   FSRS memory. When the same-looking surface has distinct supported meanings,
   use separate sense-aware cards with sentence context and an explicit homograph
   note, such as noun versus verb form.
6. **Common-frequency starter content uses milestones.** Build a ranked
   top-1000 surface-form artifact per language/register, but expose it as
   250/500/1000 milestones. Default cold-start CTA is Top 250; 500 and 1000 are
   expansion milestones.
7. **Top-N starter decks are personalized, not fixed.** If a learner imports or
   confirms known words first, the milestone deck should include only the
   remaining unknown surface forms. Learners may skip the path, test out with
   fast controls, or start the milestone.
8. **No silent bulk mark-as-known for top-N.** Known state should record
   individually confirmed surface forms. A tier can be skipped or tested out,
   but the app should not mark a whole tier known without per-item confirmation.
9. **Public alpha should ship real FSRS, narrowly scoped.** Replace the
   hand-rolled step scheduler before public alpha using Go runtime FSRS with
   default parameters, the current four-button UI, feature flag, conservative
   migration/fallback, and regression tests. Defer personal optimization,
   `fsrs-rs`, rescheduling tools, simulation dashboards, mature-card analytics,
   and broad review UX redesign.
10. **Known vocabulary and FSRS maturity are separate.** Known surface state
    comes from explicit learner evidence: import, manual confirmation, test-out,
    or marking a card known. FSRS maturity can power due dates and derived
    retained/learning coverage views, but it must not silently write known-word
    evidence.
11. **Ambiguous known-word imports resolve lazily in context.** If an imported
    surface has multiple supported meanings, do not ask the learner to
    disambiguate during import. Store surface evidence, then ask a meaning check
    only when the surface appears in a real sentence. The **Study this meaning**
    action must state whether it creates/keeps a review card now or creates one
    when the deck is saved.
12. **Parse-results meaning checks are non-blocking and pending until save.**
    In parse results, the learner can resolve an ambiguous imported surface in
    context, but no review card exists until they save/add the deck. **Study
    this meaning** must say "Creates a review card when you save." Ignored
    unresolved meanings stay conservative and remain study candidates.
13. **Low-confidence parser ambiguity uses multiple-meaning UI, not a false
    meaning check.** If the parser can list candidates but cannot confidently
    choose the intended sentence meaning, show **Multiple possible meanings**
    with per-candidate known/study actions. **None of these looks right** opens
    parser feedback and should use the planned flag-only feedback path; it is
    for "the app seems wrong", not "I do not know this word."
14. **Parser confidence must be measured, Finnish-first.** The parser should use
    context to choose meanings, and the repo already has FI/ET eval tooling.
    Before the learner UI collapses ambiguity into one intended meaning, add a
    focused ambiguity eval slice, starting with Finnish examples such as
    `kuusi`, `tuli`, and `voi`, then add Estonian parity cases. Parser
    confidence is contextual sense-selection confidence, not learner knowledge.
15. **Flag-only parser feedback is required for alpha ambiguity UX.** Current
    parser feedback is correction-only: learners must provide proposed lemma/POS,
    and admin acceptance writes `custom_overrides` for future parses. The
    **None of these looks right** action in low-confidence meaning UI needs a
    separate `flag_only` path with nullable proposed fields, admin filtering, and
    no lexical writeback until an admin supplies and accepts a concrete
    parser-identity correction.

### Reasoning

The product promise is "learn the words you need for the text you want to
read." For that promise, surface forms matter because they are what the learner
actually encounters and recognizes. Lemma-backed implementation details are
still useful, but using the lemma as the primary known state overstates
knowledge in morphologically rich languages and does not generalize to future
languages where lemmas may be stems, dictionary conventions, or otherwise poor
learner-facing units.

The milestone starter path solves a different cold-start problem: a new learner
may have no text, no Anki deck, and no imported known-word list. Top 250 is small
enough to be non-threatening, while 500 and 1000 provide meaningful expansion.
Making the path personalized avoids wasting effort on words the learner already
confirmed.

Narrow FSRS is the pragmatic launch choice. The app already exposes a review
surface and the four-button rating model. Shipping public alpha with a
hard-coded scheduler undermines the learning loop, but full FSRS ecosystem work
would distract from core launch readiness. Runtime default-parameter FSRS gives
better scheduling without requiring optimization research or a review-system
rewrite.

### Consequences

- Future known-word work should preserve submitted surface forms as first-class
  evidence and treat lemma/POS as derived.
- Card-identity work should migrate from current lemma-backed cards to
  surface-form-in-context cards before the FSRS cutover.
- Homographs should not collapse into one pure-surface card when
  parser/dictionary evidence supports distinct meanings; the card presentation
  should teach the contrast explicitly.
- Top-N artifacts should be built as source-audited, register-aware,
  surface-form ranked artifacts, then exposed as 250/500/1000 milestones.
- FSRS implementation should attach scheduler state to stable surface-card IDs,
  not temporary lemma/POS card IDs.
- FSRS maturity should remain derived scheduler evidence. It must not silently
  promote a surface/sense card into explicit known vocabulary.
- Ambiguous imported surfaces need a lazy meaning-check flow. The UI must make
  card creation explicit when the learner chooses to study the meaning.
- Parse results should center the learner journey: meaning checks are inline,
  optional, and reversible before save; deck save is the durable card-creation
  moment.
- Parser confidence must shape the meaning-check UX. Low-confidence ambiguity
  should not ask the learner to confirm a guessed meaning as if it were known.
- Confidence thresholds should be backed by focused parser eval, not vibe.
  Finnish is first-class and should get the first ambiguity slice.
- Feedback implementation must distinguish "issue report" from "accepted
  parser-identity correction". Flag-only reports are valuable Track B signal, but
  they are not lexicon updates by themselves.
- The active grill session remains the detailed Q/A trail; this decision is the
  stable summary for future work.

### Source

See `docs/grill-sessions/2026-07-03-product-readiness.md` questions 1-25 for the
full working discussion.

---

## Decision 22: Source-backed ET learner corrections stay deterministic

**Date:** 2026-05-12

### Context

A manual audit of Estonian parse rows flagged cases where the learner
surface was misleading even though the row could be traced to local
Ekilex-derived data:

- `olema/VERB` could show the translation `accompany` instead of the
  learner-primary `be`.
- `see/PRON`, `väike/ADJ`, `et/CCONJ`, and `kui/CCONJ` could expose
  low-value source translations such as `current`, `delicate`,
  `as if`, or `albeit` ahead of the expected learner gloss.
- `ei`, `kui`, and other invariant closed-class rows could inherit
  nominal case FEATS from duplicate Ekilex form rows, producing labels
  like genitive or illative on words where a case label is not
  learner-meaningful.
- Special-capitalized dictionary entries such as `mA` and `MA` were
  indexed under lowercase `ma`, so the pronoun `ma` could collide with
  unit or degree abbreviations.
- ET-only source definitions such as `sina/NOUN` described an Estonian
  color noun, not the high-frequency pronoun the learner expects.

The important source rule is that the parser must not invent
Sõnaveeb/Ekilex content. If a weird translation is displayed, we need
to know whether it is present in the source artifacts and then decide
whether it belongs in the learner-primary slot.

### Decision

Keep this correction path deterministic and auditable:

1. Treat reduced Ekilex artifacts and Sõnaveeb pages as provenance for
   source claims. A translation being present in the local Ekilex shard
   is not by itself enough to make it the learner-primary gloss.
2. Use small learner gloss overrides for high-frequency ET closed-class
   rows where source ordering is not suitable for the app:
   `ei/ADV -> "no; not"`, `et/CCONJ -> "that"`,
   `kui/CCONJ -> "when; if; as; than"`, `olema/VERB -> "be"`,
   plus the existing `see/PRON` and `väike/ADJ` overrides.
3. Require exact surface capitalization for bare special-capitalized
   dictionary lemmas. `ma` and sentence-initial `Ma` resolve to the
   pronoun; exact `mA` or `MA` may resolve to their abbreviation rows.
   This direct-dictionary filter applies in both `basic` and `custom`
   modes; basic mode remains dictionary-only but its candidate set is
   source-integrity-filtered. Exact all-caps forms such as `TA` also
   bypass lowercase lexical overlays and may reach exact source entries.
4. Sanitize misleading FEATS at runtime for already-imported DBs:
   nominal case-only FEATS on invariant closed-class exact rows are
   cleared, and exact ET verb dictionary forms with
   `Case=Ill|VerbForm=Sup` display as `VerbForm=Inf` regardless of
   FEATS attribute order or harmless extra nominal attributes.
5. Change future Ekilex form imports so `ID` rows keep empty FEATS and
   same-key `SgN` rows can overwrite earlier stale case duplicates with
   nominative FEATS.
6. Filter known ET source-language-only trap alternatives by exact
   `(surface, lemma, POS)`, for example `kui/NOUN`, so stale nominal
   FEATS cannot outrank the real closed-class readings.
7. Add ET lexical-overlay entries for `ei`, `ma`, and `sina` in custom
   mode. These are deterministic closed-class corrections, not
   generated guesses.

### Reasoning

This keeps source fidelity separate from learner presentation. The
source artifacts can contain long-tail translations, abbreviations,
symbols, and source-language-only definitions that are real data but
bad first choices for a language-learning parse row. The parser should
record and use those sources, but the learner-primary gloss should be a
small curated decision when high-frequency function words are involved.

Importer fixes are the durable answer for future DB builds, but runtime
sanitization is still necessary because developers and deployments can
already have a 5+ GB SQLite DB built from the previous importer. The
runtime guard is deliberately narrow: it only clears nominal case-only
FEATS from invariant closed-class exact rows, and it normalizes exact
special-capitalized dictionary forms to bare nominative display instead
of exposing stale illative/genitive labels.

Exact capitalization is also a source-fidelity rule. If Ekilex has
separate `mA` or `MA` entries, lowercasing them into the same learner
surface as `ma` loses information and lets abbreviations beat the
pronoun. Matching special-capitalized lemmas exactly preserves those
entries for exact input while protecting ordinary text.

No LLM call belongs in this path. The failures are exact lookup,
capitalization, morphology-code, and source-priority problems; each has
a deterministic rule and a focused regression test.

### Source

The audited values were checked against the local reduced Ekilex shards
under `localdata/ekilex/{definitions,forms}` and the public Sõnaveeb
lookup pages for the reported words. The importer behavior is pinned by
`cmd/importekilexdetails` tests; runtime behavior is pinned by
`internal/store` and `pkg/lemmatizer-fi-et/lexadverbs` tests.

**See also:** [CHANGELOG.md §2026-05-12 — Source-backed ET learner
cleanup](CHANGELOG.md), [PARSER_EVOLUTION.md §2026-05-12c](PARSER_EVOLUTION.md).

---

## Product Vision

FinnEst is a **JPDB clone for Finnish and Estonian**. The core user flow:

1. **Paste text** — User submits Finnish (or Estonian) text
2. **Parse perfectly** — System extracts lemmas, POS, definitions
3. **Review word list** — User sees all unique words with meanings
4. **Mark known/unknown** — User indicates which words they already know
5. **Create deck** — User saves unknown words as a study deck
6. **SRS study** — Spaced repetition moves words from "learning" to "known"
7. **Loop** — Next time user pastes text, known words are dimmed, focus is on new vocabulary

The value proposition: **pre-mine vocabulary before reading** so the reading experience
is more enjoyable and comprehension is higher.

---

## Decision 21: Source priority outranks generic FST support and morphology ties

**Date:** 2026-05-12

### Context

PR #187 revisited the custom dict+FST merge ranker after PR #185's
learner-facing alternative filtering. The parser already used
dictionary `source_priority`, but in the merged candidate path that
priority came after generic support and morphology tie-breaks. A
lower-priority kaikki row could therefore beat a higher-priority
Ekilex row if it had same-lemma FST support, or if it carried FEATS
while the Ekilex row did not.

That ordering was too weak for cross-source dictionary overlap. The
source priority column is the row-level authority signal: Ekilex bulk
rows are intended to beat kaikki rows once candidates are otherwise
equal on the sanity checks that protect learner-facing regressions.

### Decision

In `pickBestResolutionCandidate`, keep this rank order:

1. Surface/lemma case sanity.
2. Lowercase-surface `PROPN` sanity.
3. The narrow `fstBeatsWeakDict` escape hatch for legacy dict rows
   with no priority and no morphology.
4. Dictionary `source_priority`.
5. Generic dict/FST support score.
6. Morphology presence (`FEATS` or projected grammar label).
7. Deterministic tie-breaks.

This means higher-priority Ekilex rows beat lower-priority kaikki rows
even when the lower-priority row has FST support or FEATS, provided
case/POS sanity ties and the dict row is not the specific weak legacy
case handled by `fstBeatsWeakDict`.

### Reasoning

This is not an "Ekilex always wins" rule. The case/POS sanity checks
still run first because proper-name homonyms and lowercase common-word
regressions are known, user-visible failures. The FST escape hatch
also remains before priority so generated morphology can still repair
truly weak legacy rows.

Once those guards tie, source priority is the better authority signal
than generic support or morphology density. Ekilex is the stronger
source for Estonian cross-source overlaps, and the 2026-05-07 bulk
import normally supplies dense FEATS; an Ekilex row without FEATS is
the exception, not a reason for a lower-priority kaikki row to win.
The change was eval-checked on committed FI/ET gold and local UD-ET
test sets, and pinned with regression tests for both lower-priority
FST support and lower-priority FEATS.

---

## Decision 20: Lexical-overlay short-circuit and curated bad-lemma blocklists

**Date:** 2026-05-12

### Context

PR #183 imported five learner-quality fixes from `yle_subs` (the
downstream Anki-deck builder that consumes finnestdb's exports). Each
fix targeted a specific recurring failure where the productive
dictionary or FST analysis was defensibly wrong for a closed-class
form, a frozen lexicalised adverb, or a known kaikki-import artefact.

The first ship of the PR fired the overlay inside
`Lemmatizer.Lemmatize` only — but `BatchLookupForms` consults the
forms table BEFORE asking the FST, and the dict candidate won the
merge-layer support-score tiebreak. The overlay was effectively
dead code on every surface where kaikki had a row. Review feedback
forced a re-think of where these corrections belong in the
resolution flow.

A subsequent review pass on the first cut of the bad-lemma blocklist
found it also dropped legitimate `varsi → varsi/NOUN` dict lookups
in basic mode because the flat lemma set treated `varsi` as always
wrong. The fix needed a finer-grained distinction.

### Decision

Two design choices:

1. **Lexical-overlay short-circuit runs at Step 0 of
   `BatchLookupForms`, custom mode only.** Not inside
   `Lemmatize()`. When the surface hits the overlay, the curated
   analysis is returned outright and Steps 1 (dict) + 5 (FST) are
   skipped. Source tag `lex-overlay` marks the path for eval
   attribution. Custom-mode-only because the overlay is part of
   the "custom enhancements" suite — basic-mode baselines stay
   stable.

2. **Bad-lemma blocklist is two-tiered**:
   - `alwaysBadDictLemmasFI` for lemmas that are NEVER legitimate
     standalone words. Short fragments (`as`, `taa`, `ku`),
     compound-clip prefixes (`sisä-`, `ylä-`), and documented
     kaikki-import bugs (`poli` as the lemma for poliisi inflected
     forms). Filtered regardless of surface or parser mode — no
     learner asks for the bare surface `sisä-` expecting that
     prefix as the lemma.
   - `badSurfaceLemmaFI` for (surface, lemma) pairs where the
     lemma is legitimate elsewhere but wrong for the specific trap
     surface (`(varsin, varsi)`, `(vuotta, vuo)`,
     `(siitä, siittää)`, etc.). The lemma is preserved for
     OTHER surfaces — `varsi → varsi/NOUN` keeps working in both
     modes.

### Reasoning

**Why Step 0 over Lemmatize-internal short-circuit.** The dict
layer's merge ranker (`pickBestResolutionCandidate`) prefers dict
candidates over FST candidates on supportScore alone (dict=3 vs
FST=2). An overlay analysis injected as an FST analysis loses that
tiebreak even when it's right. A Step 0 short-circuit bypasses the
ranking entirely for surfaces the maintainer has explicitly
asserted are bugs.

**Why custom-mode-only.** Basic-mode is the dict-only baseline we
measure other parsers against. Adding overlay rewrites to basic
mode would shift baselines for every comparison. Custom mode is
the "what we ship to learners" path and the right place for
curated enhancements.

**Why two-tier blocklist instead of flat.** A single lemma set
forces an all-or-nothing choice: either filter `varsi` everywhere
(losing the legitimate noun lookup) or nowhere (keeping the trap
on `varsin`). yle_subs's own
`SUSPICIOUS_SURFACE_LEMMAS` is keyed on pairs for exactly this
reason. The split keeps the never-legitimate fragments globally
blocked (where there's no legitimate lookup to protect) and
preserves the lemma on its real surfaces.

**What the overlay does NOT do.** It does not generate cards, edit
the user-friendly wordlist, or train any classifier. It is a
runtime correction layer for a handful of catalogued surfaces
(~14 FI entries, ~11 ET entries). Productive morphology stays in
the FST tables.

### Source

PR [#183](https://github.com/sagarinbabel/finnestdb/pull/183). The
yle_subs source files for each rule are referenced from code
comments at the entry point in
[`pkg/lemmatizer-fi-et/lexadverbs/lexadverbs.go`](../pkg/lemmatizer-fi-et/lexadverbs/lexadverbs.go)
and the bad-lemma definitions in
[`internal/store/dict.go`](../internal/store/dict.go).

**See also:** [PARSER_EVOLUTION.md §2026-05-12a](PARSER_EVOLUTION.md),
[FST_LEMMATIZER.md "Store-level candidate merge"](FST_LEMMATIZER.md).

---

## Decision 19: Filter low-value dict alternatives in deck/parse expansion

**Date:** 2026-05-12

### Context

`BatchLookupAllForms` returns every `(lemma, pos)` candidate the dictionary
has for a surface form. Wiktionary-imported form-of rows (e.g.
`olen → "first-person singular present indicative of olema"`) live alongside
the base lemma (`olema → "be"`) and, until PR #185, both produced their own
card or word-list entry during deck/parse expansion. Some surfaces also had
candidates with empty glosses — `liiga/X` next to `liiga/ADV → "too"` — which
similarly bloated the deck with rows the learner can't act on.

### Decision

When a surface has multiple dict candidates and at least one has a non-empty
gloss, suppress:

1. candidates with empty glosses, and
2. Wiktionary form-of alternatives, when a lexical-base alternative exists for
   the same surface.

Form-of detection is structural, not substring-based:

- `candidate.Lemma == form` (case-insensitive, trimmed) — Wiktionary form-of
  rows are imported with the surface form as their own lemma.
- Gloss contains no `;` or `,` — form-of glosses are single-clause.
- Gloss parses as `<allowed morphology terms> of <single-word target>` after
  normalizing `-` and `/` to spaces. The allowed vocabulary covers
  case names, person/number, tense/mood/voice, infinitive/participle/gerund,
  comparative/superlative degree, connegative and potential moods, and the
  bare `form` / `inflection` markers.

When no lexical alternative exists for a surface, all candidates are preserved
— genuine unresolved / gap cases still surface to the learner.

### Reasoning

A v1 marker-substring heuristic produced false positives on common lexical
glosses whose body text happens to mention grammatical terms:
`vana/ADJ "old; ancient; ...; out of order; ...; past; ..."`,
`oma/ADJ "(my/...) own; ...; one of a kind; ...; singular; ..."`,
`mennä/VERB "to go [with illative of third infinitive ...]"`. The structural
signals are language-independent and robust: `form == lemma` identifies
Wiktionary form-of-as-lemma rows directly, and the `;`/`,` rejection rules
out multi-sense lexical glosses without inspecting their body.

The filter operates on `BatchLookupAllForms` output before deck-ingest and
before the parse-overview word list, so the unique-lemma count of an import
overview still matches the count of the deck the user would save.

### Source

PR [#185](https://github.com/sagarinbabel/finnestdb/pull/185).

**See also:** [CHANGELOG.md §2026-05-12 — Deck/parse low-value dict-alternative
filter (PR #185)](CHANGELOG.md).

---

## Decision 18: IMPLEMENTATION.md split

**Date:** 2026-05-07

### Context

`docs/IMPLEMENTATION.md` overlapped substantially with
[`ARCHITECTURE.md`](../ARCHITECTURE.md) and was hard to keep in sync. Updates
landed in one file but not the other; readers couldn't tell which doc was
canonical for a given topic.

### Decision

`docs/IMPLEMENTATION.md` becomes a redirect stub. Unique content moved to
its canonical home:

- "Suggest fix" UX → new
  [`docs/PARSER_FEEDBACK_LOOP.md`](PARSER_FEEDBACK_LOOP.md).
- Build/tooling → [`README.md`](../README.md) "Build & Start" section.
- Current limitations → [`README.md`](../README.md) "Known Limitations"
  section.

### Reasoning

Minimize cross-doc drift. One canonical home per topic, with stubs that
redirect rather than mirror.

### Source

PR #135.

**See also:** [CHANGELOG.md §2026-05-07 — Runtime docs parity pass](CHANGELOG.md).

---

## Decision 17: ESTONIAN_LEXICAL_PLAN consolidation

**Date:** 2026-05-07

### Context

We had two lexical-layer plan documents — `docs/LEXICAL_PLAN.md` (FI) and
`docs/ESTONIAN_LEXICAL_PLAN.md` (ET) — and they had drifted independently.
The ET plan still recommended a smoke import path (`make
import-dict-et-ekilex`) that the bulk Ekilex pipeline had superseded.

### Decision

Consolidate into one [`docs/LEXICAL_PLAN.md`](LEXICAL_PLAN.md). ET-specific
source choices and the EstNLTK adapter contract now live in a section
("Estonian-specific source choices and adapter contract") inside that file.
`docs/ESTONIAN_LEXICAL_PLAN.md` is deleted.

### Reasoning

Duplicate plans rot independently. One canonical doc per language pair —
shared architecture in the body, language-specific deltas in clearly
labelled sections.

### Source

PR #135.

**See also:** [CHANGELOG.md §2026-05-06 — Lexical pipelines](CHANGELOG.md)
for the original two-doc landing.

---

## Decision 16: FST as parallel scorer in dict step 1

**Date:** 2026-05-07

### Context

Pre-PR-#127, the FST runtime in
[`pkg/lemmatizer-fi-et/`](../pkg/lemmatizer-fi-et/) was wired in as a
step-5 fallback only — it fired on dict miss. This made morphology a
fallback rather than an evidence source: the parser couldn't surface FEATS
for forms whose lemma resolved cleanly through the dict.

### Decision

The FST contributes candidates in parallel with dict step 1
([PR #127](https://github.com/sagarinbabel/finnestdb/pull/127)).
`FormResolution.Feats` is enriched from FST candidate merge
([PR #129](https://github.com/sagarinbabel/finnestdb/pull/129)).
Per-attribute FEATS evaluation is added so regressions on individual
morphological attributes are visible
([PR #130](https://github.com/sagarinbabel/finnestdb/pull/130)).

### Reasoning

Dict-only resolution can't surface FEATS for forms whose lemma resolves
cleanly through dict — the lemma is right but the morphology bucket is
empty. Parallel scoring + candidate merge gives FEATS coverage on dict
hits without sacrificing dict's lemma accuracy. Per-attribute eval
prevents silent regressions on individual features (Case, Number, Person,
Tense) hiding behind a stable composite "grammar accuracy" number.

### How to Revisit

If the smoke-fixture FST starts dragging dict accuracy down (e.g. emits
wrong-case homonyms that win the merged ranking), re-litigate the merge
order. The current ranking gives dict the lemma vote and FST the FEATS
vote; that asymmetry is intentional but tunable.

---

## Decision 15: Single-folder bootstrap rule

**Date:** 2026-05-07

### Context

Legacy `data/ud-cache/` and gitignored carve-outs under
`testdata/parser-eval/` made bootstrap require multiple archives or a
custom recipe that knew the carve-outs. Handing a teammate a "fast
bootstrap" zip was a foot-gun.

### Decision

Every gitignored runtime artifact lives under
[`localdata/`](../localdata/). The `data/` directory is disallowed for new
artifacts; `.gitignore` carries belt-and-braces guards for legacy paths.
`localdata/` covers UD cache, parser-eval gold/train carve-outs, frequency
baselines, lemmatizer tables, Kotus distribution, Ekilex artifacts, etc.

### Reasoning

One tarball captures the entire bootstrap state:

```bash
tar czf finnestdb-bootstrap.tgz localdata/ finnestdb.db
```

No carve-outs to remember. `make setup-local` summarizes the tree on
completion and emits the bootstrap-tar instruction.

### Source

PR #131. See [`docs/ARTIFACT_POLICY.md`](ARTIFACT_POLICY.md) "Single-folder
bootstrap rule" section for the full policy and rationale.

**See also:** [CHANGELOG.md §2026-05-07 — Single-folder data root](CHANGELOG.md).

---

## Decision 14: `feats` column not backfilled on existing kaikki.org rows — REVERSED 2026-05-07k

**Date:** 2026-05-06 · **Reversed:** 2026-05-07 (PR #139)

### Context

When the `feats` column was added to `forms` (Phase 1 schema delta), an
implicit question was whether to retroactively populate it on existing
kaikki.org-sourced rows.

### Original decision (2026-05-06)

Leave `feats=NULL` on existing kaikki.org form rows. Voikko-generated /
FST-derived rows fill in features at higher priority going forward.

### Original reasoning

kaikki.org form rows don't carry UD-style FEATS in the source data, and
mining them post-hoc from the surface/lemma pair is exactly what the FST
does at runtime. Backfilling synthetically would create a second,
lower-quality FEATS source competing with the FST output. Cleaner to
keep kaikki.org rows feature-less and let higher-priority sources
contribute FEATS.

### Reversal (2026-05-07k)

The original reasoning was wrong: kaikki form rows DO carry the
morphology, just not in UD-FEATS shape. Each `forms[]` entry has a
`Tags []string` field with exactly the lowercase English vocabulary
that maps deterministically to UD FEATS — `["illative","singular"]`
→ `Case=Ill|Number=Sing`. PR [#139](https://github.com/sagarinbabel/finnestdb/pull/139) implements this projection in
`cmd/importdict/feats.go::kaikkiTagsToFeats`, covering Case (15 entries),
Number, Person, Tense, Mood, Voice, VerbForm, Degree, Reflex (lexical-static),
PronType. The translation is lossless — we're not synthesising FEATS we
can't defend; we're reading the same morphological annotation from a
different field of the same source.

The "competing FEATS source" concern is also resolved by the precedence
already in place: dict-layer FEATS yield to FST-layer FEATS via
`enrichResolutionWithFST` when the FST has a richer analysis, and
`featsFromFSTAnalysis` always wins when both fire. So kaikki-derived
FEATS act as the floor (better than NULL) without competing with the
FST ceiling.

---

## Decision 13: Wikisanakirja for monolingual FI definitions; Kielitoimiston deferred

**Date:** 2026-05-06

### Context

Finnish has two practical sources for monolingual definitions:
Kielitoimiston sanakirja (authoritative, restricted redistribution) and
Wikisanakirja (Wiktionary's Finnish edition, openly licensed).

### Decision

Use Wikisanakirja (via kaikki.org's Finnish edition extract) for
monolingual FI definitions. Kielitoimiston is **not** in scope for alpha.

### Reasoning

Kielitoimiston's redistribution restrictions make bulk import infeasible.
Wikisanakirja coverage is sufficient for alpha; kaikki.org already extracts
and normalizes the Finnish-edition Wiktionary data. Revisit Kielitoimiston
only if Wikisanakirja proves insufficient and only as a runtime lookup
that respects the license.

### How to Revisit

Track Wikisanakirja coverage gaps. If a meaningful fraction of high-frequency
lemmas come back without definitions, add Kielitoimiston as a runtime
(non-redistributed) lookup with explicit license-compliant attribution.

---

## Decision 12: Idempotent ALTER TABLE migration pattern; real framework deferred

**Date:** 2026-05-06

### Context

We need to add columns and tables as the lexical schema evolves. A real
migration framework (numbered SQL files, version table) is the textbook
answer, but it's also a one-shot up-front investment.

### Decision

Schema migrations stay on the established codebase pattern: each migration
is an idempotent `ALTER TABLE ... ADD COLUMN` (or `CREATE TABLE IF NOT
EXISTS`) that tolerates the SQLite "duplicate column name" error on
re-run. Grouped backfills get exported helpers named `EnsureXxx` in
[`internal/store/db.go`](../internal/store/db.go), called by both the
server's `ensureSchema` and any standalone importer that needs the same
shape. No `PRAGMA user_version`. References:
`EnsureDictionarySourceColumns` (#67), `EnsureLexicalEnrichmentColumns`
(Phase 1).

A real migration framework is deferred until at least one of these is
true: a non-additive migration is needed (column rename, stateful
backfill); >5 versioned migrations and merge conflicts start; rollback
support is required.

### Reasoning

The codebase already established the idempotent-ALTER convention.
Introducing a parallel mechanism (PRAGMA user_version, numbered SQL
files) before it's needed would just give us two patterns to maintain.
Migrations are infrequent and append-only today.

### How to Revisit

When the trigger conditions hit (non-additive migration, merge-conflict
pressure, rollback need), introduce the real framework as a single PR —
not lazily alongside a feature.

---

## Decision 11: Kotus distribution as authoritative lemma source

**Date:** 2026-05-06

### Context

There are two practical paths for Kotus class data: the official Kotus
sanalista distribution (https://kaino.kotus.fi/sanat/nykysuomi/) and
Voikko's `joukahainen` re-export.

### Decision

Use the official Kotus distribution. The 2024 distribution is fetched into
the gitignored `localdata/kotus/` via `make setup-local` (CC BY 4.0; see
[`docs/ARTIFACT_POLICY.md`](ARTIFACT_POLICY.md)) and imported via
`cmd/importkotus`.

### Reasoning

The official distribution is more likely to be maintained and is the
canonical authority for Kotus class assignments. Voikko's re-export adds
a layer of indirection without adding signal.

---

## Decision 10: Adapter packaging — separate cmd/ binaries per rich source

**Date:** 2026-05-06

### Context

PRs #67 and #68 added Ekilex import via a `-source-key` flag on the
existing `cmd/importdict/` binary. Subsequent rich sources (Kotus, Voikko)
needed a packaging decision.

### Decision

Each rich source gets its own `cmd/import<source>/` (or
`cmd/gen<artifact>/`) binary, matching the precedent set when
`cmd/importekilex/` landed on main:

- `cmd/importkotus/` — Kotus sanalista TSV
- `cmd/genlemmatizertables/` — generated FST tables (FI smoke today)
- `cmd/importekilex/` — Ekilex public-headword snapshot
- `cmd/importekilexdetails/` — bulk Ekilex reduced JSONL
- `cmd/importdict/` — kaikki.org / Wiktionary (the original)

### Reasoning

Each binary handles its own input shape (XML, TSV, JSONL, generated
lookups) and shares only the schema bootstrap pattern. Keeping
`cmd/importdict/` as a multi-source dispatcher would mix unrelated input
parsers in one entry point and dilute its responsibility.

### Note

PRs #67 and #68 use the `-source-key` flag inside `cmd/importdict/` —
that work predates `cmd/importekilex/` landing on main and will likely
rebase against the separate-binary pattern.

---

## Decision 9: Production morphology tables deferred; smoke fixtures only

**Date:** 2026-05-06

### Context

The lemmatizer-fi-et package needs morphology tables to function. Generating
production-scale FI/ET tables requires running upstream analysers
(VFST/HFST) locally and generating large derived outputs. Smoke fixtures
(9 FI keys, 7 ET keys) are enough to prove the integration path.

### Decision

The repository commits only smoke fixtures under
[`testdata/lemmatizer/`](../testdata/lemmatizer/) plus focused package
tests; it does not commit production FI/ET tables. These fixtures prove
the integration and the artifact policy, not production morphology
coverage. Production tables are generated locally via
`make gen-lemmatizer-tables-fi VFST_PATH=...` and written to
`localdata/lemmatizer-fi-et/tables/` (gitignored). Broad
runtime/eval claims wait until production tables, provenance, and fresh
baselines all land together in a single PR.

### Reasoning

Committing production tables prematurely would mean shipping artifacts
without provenance and gating eval claims on data nobody else can
reproduce. Holding the smoke fixtures separate from production tables
makes the boundary explicit.

### How to Revisit

A production FI/ET table PR adds a production word list, provenance,
generator command, row counts, and fresh eval baselines as a single unit.

---

## Decision 8: Translations and definitions tables ship before Sõnaveeb

**Date:** 2026-05-06

### Context

Both the FI and ET lexical plans need separate `translations` and
`definitions` tables (rather than overloading `lemmas.gloss`). The question
was whether to land them with the FI plan, or hold for Sõnaveeb integration.

### Decision

Land `translations` and `definitions` tables with the FI Phase 1 schema
delta. Both languages benefit; landing them once avoids two parallel
migrations.

### Reasoning

The Finnish plan needs translations and definitions; the Estonian plan
benefits from them. Sequential landings would mean either ET ships
without these tables and migrates later, or FI waits on ET — both worse
than landing once.

---

## Decision 7: Generated-table deployment policy

**Date:** 2026-05-06

### Context

The generated-table runtime (`pkg/lemmatizer-fi-et/`) loads JSON tables
generated from upstream analysers (Voikko VFST, Giellalt HFST). The
question is what gets committed to git: just the runtime code, the
generated tables, or also the upstream analyser blobs.

### Decision

The build/generation pipeline may run local upstream analysers, but the
repository ships **neither** analyser blobs **nor** the derived factual
tables. Both live under `localdata/lemmatizer-fi-et/` (gitignored).
The runtime loads tables from disk on `New()`. Smoke fixtures (small
hand-checked tables) live in
[`testdata/lemmatizer/`](../testdata/lemmatizer/) and focused package
tests — those exist purely to prove the integration path and are tiny.

See [`docs/ARTIFACT_POLICY.md`](ARTIFACT_POLICY.md) for the full policy.

### Reasoning

Upstream analyser blobs are large, license-constrained, and are
reproducibly generated from public source. Committing them would bloat
the repo and create license-compatibility questions. Generated tables
have the same problem at smaller scale: regenerable from upstream
analysers, not human-readable, and license-derivative.

The runtime-loads-from-disk choice means a fresh checkout has a working
package (using smoke fixtures) and a production-quality install requires
running the generator locally and pointing the runtime at
`localdata/lemmatizer-fi-et/tables/`.

**See also:** [CHANGELOG.md §2026-05-06 — Lexical pipelines](CHANGELOG.md).

---

## Decision 6: Numeric-Hyphen Tokenization Lives in the Shared Tokenizer

**Date:** 2026-05-06

### Context

A user pasted Estonian text containing `65-aastane` ("65-year-old") into the
parser during manual testing and noticed neither `65` nor `aastane` showed up
as separate words. Pure numbers like `65` weren't tagged `NUM` either. The
same construction is just as productive in Finnish (`65-vuotias`,
`1990-luvulla`, etc.), and the tokenizer at
[`parser/src/lib.rs`](../parser/src/lib.rs) takes an unused `_lang`
parameter — Finnish was guaranteed to have the identical bug.

### Decision

Fix this in the shared Rust tokenizer with four pure-tokenizer rules. Do **not**
add per-language entries to
[`internal/parserules/finnish.go`](../internal/parserules/finnish.go) or
[`internal/parserules/estonian.go`](../internal/parserules/estonian.go).

- **R1** — split a chunk at the first hyphen where one side is pure digits and
  the other starts with a letter (`65-aastane`, `65-vuotias`, `1990-luvulla`).
  Skip mixed-prefix abbreviations (`B1-tase`, `well-known`).
- **R2** — `guess_pos` returns `NUM` for `^\d+$`, `^\d+\.\d+$`,
  `^\d+,\d+$`, with internal whitespace stripped.
- **R3** — post-pass that merges `\d{1,3}( \d{3})+` runs into one NUM token.
  Form keeps spaces (`"250 000"`); lemma drops them (`"250000"`) so SI-spaced
  and unspaced numbers group as one entry in the words list.
- **R4** — split a chunk at the only hyphen if both sides are pure digits
  (`1990-2020`). Multi-hyphen forms (ISO dates `2026-05-06`) stay whole because
  R4 requires exactly one hyphen.

### Reasoning

[`docs/CROSS_LANGUAGE_STRATEGY.md`](CROSS_LANGUAGE_STRATEGY.md) lists
*tokenization or sentence-split error* and *compound segmentation error* as
shared error categories, and prescribes investing in shared infrastructure
when one language surfaces a problem the other has too. This is exactly that
case: the four rules are language-agnostic (digits, letters, hyphens, spaces
are universals across FI/ET) and any per-language inflection of the freed
stems (`aastane`, `vuotias`, `luku`) is handled by the existing language-
specific lookup chain. Splitting the tokenizer fix into two per-language
implementations would have been duplicate work with a high risk of drift.

### Trade-off Accepted

- Conservative R4 (one-hyphen-only) leaves ISO dates whole as a single
  unresolved NOUN literal, which is the same as the pre-fix state for those
  forms (no regression). A dedicated date detector can layer on later.
- ET `65-aastast` (partitive of `65-aastane`) splits cleanly via R1 but
  `aastast` doesn't lemmatize back to `aastane` without a `-ne` ADJ
  inflection table — separate piece of work.
- Negative numbers like `-5` stay as one token. Acceptable for alpha;
  negation is usually written `miinus 5` in both languages.

### How We Measure Success

- 13 new Rust unit tests added; full suite: 41 passed, 0 failed.
- Zero regression on all 6 existing gold datasets (et-grammar-v1,
  et-manual-v1, fi-core-v1, fi-manual-v1, fi-manual-v2, fi-grammar-v1) at the
  Phase-2 baseline DB. Numbers identical to
  [2026-05-06b](baselines/2026-05-06b-summary.md).
- A direct probe of 13 sentences across both languages (5 FI + 5 ET fix
  cases + 3 regression cases) confirms R1–R4 produce the intended tokens
  and freed stems (`aastane`, `luku`) hit the dictionary cleanly. Full
  trace in
  [`docs/qa-reports/2026-05-06-numeric-hyphen-tokenization.md`](qa-reports/2026-05-06-numeric-hyphen-tokenization.md).

**See also:** [CHANGELOG.md §2026-05-06 — Numeric-hyphen tokenization](CHANGELOG.md).

---

## Decision 5: Don't Extend the Case-Suffix Table; Generated Morphology Tables Are the Real Answer

**Date:** 2026-05-06

### Context

`internal/parserules/{finnish,estonian}.go` defines suffix→case-label tables
(15 Finnish entries, 17 Estonian entries) used by
`internal/store/dict.go::tryCaseSuffixStrip`. The matcher strips a suffix off
the surface form, looks up the residual stem in the `lemmas` table, and on
hit returns `(lemma, pos, grammar_label)`.

The natural-feeling reaction to the 0% grammar-accuracy result on
`grammar_label` (see `docs/baselines/2026-05-06b-summary.md`) is to grow this
table — add more suffix entries, encode consonant gradation, handle ternary
compounds, etc. Existing TODO items #15 (three-part compound splitting) and
#16 (consonant gradation rules) point in that direction. We are choosing
**not** to.

### Decision

**Freeze the case-suffix table at its current size.** Further morphology
investment goes into generated factual morphology tables loaded by
[`pkg/lemmatizer-fi-et`](../pkg/lemmatizer-fi-et/) from
`localdata/lemmatizer-fi-et/tables/` (gitignored), plus the offline
generator/reader code that can reproduce them from local upstream
analysers. Per [docs/ARTIFACT_POLICY.md](ARTIFACT_POLICY.md), upstream
transducer blobs and the derived tables are both local-only and are
not committed.

Two near-term exceptions are in scope:

1. **The stopgap label-attach pass on dict hits**
   (`attachCaseLabelIfStemMatches` in `internal/store/dict.go`). Lifts grammar
   accuracy off zero on tokens whose stem doesn't change under inflection.
   Explicitly stopgap; removed once production generated tables emit FEATS
   for direct hits. **Updated 2026-05-07k**: the stopgap path now also
   projects UD FEATS via `featsFromCaseLabel` (a small map lookup against
   `pkg/lemmatizer-fi-et/udfeats::LegacyLabelToUDCase`). The `Case=` it
   emits is the only attribute it can safely commit to from a stripped
   suffix; Number/Tense/Mood/Person stay empty. The suffix table itself
   is unchanged — the addition is a projection on top, not an extension.
2. **Bug fixes** to existing entries if a wrong label is being attached.

### Reasoning

Suffix-stripping is the wrong *shape* of operation for Finnish/Estonian
morphology. Five reasons, each grounded in real tokens from our gold sets:

1. **Stem alternation can't be expressed by end-of-string rules.**
   `toas → tuba` (et-grammar-v1, inessive of "room"): suffix-strip removes
   `s`, leaves `toa`, but the lemma is `tuba` — `o ↔ u` flips inside the
   stem with consonant alternation. A suffix table operates only on the
   suffix; it has no way to encode "after stripping `s`, also rewrite
   `oa → uba` for stems in grade-alternation class III." Encoding that is
   reimplementing an FST with worse abstractions. Same with `Naabri →
   naaber` (epenthetic vowel), `linnas → linn` (final-`a` deletion after
   `s`-stripping), `majja → maja` (gemination + `a`-insertion produced
   the illative; reverse isn't a suffix strip at all).

2. **Suffix-shaped lemmas trigger false positives.** `aas` (meadow), `mees`
   (man), `loss` (castle) all end in `s`. Stripping `s` gives `aa` / `mee` /
   `los` — none of which are lemmas. The table has no way to know which
   `-s` is paradigmatic and which is part of the lemma. An FST knows
   because it has the lexicon and the inflectional paradigm together.

3. **Genuine ambiguity needs a candidate set, not a single answer.**
   `linnas` is `Case=Ine|Number=Sing` of `linn` ("in the city") AND, in
   some readings, `Case=Gen|Number=Sing` of a personal name `linnas`. A
   suffix table emits one tuple `(lemma, label)`; the alternative is
   discarded. Real morphology produces a candidate set and lets the
   disambiguator pick. `pickBestFormCandidate` already exists for direct
   dict — case-suffix-strip output should be folded into the same ranker
   in the FST world, not the suffix world.

4. **Compound interaction.** Estonian compounds extensively. Suffix-strip
   over-fires on compounds: `linnasüda` ("city heart") ending in
   suffix-shaped `a` parses as essive of a fake lemma. Compounds need to
   be split *before* suffix logic, and the split needs paradigm-class
   awareness — FST territory.

5. **We are already building a table-backed morphology path.** PRs
   [#107](https://github.com/sagarinbabel/finnestdb/pull/107),
   [#108](https://github.com/sagarinbabel/finnestdb/pull/108), and
   [#110](https://github.com/sagarinbabel/finnestdb/pull/110) add the
   generated-table runtime and offline analyser readers. The suffix table
   should remain fallback code while production tables are generated.

### Trade-off Accepted

The stopgap will not produce grammar labels for stem-alternating forms
(`toas`, `Naabri`, `linnas`-as-inessive). That's acceptable because:

- Stem-alternating forms are exactly what generated analyser-derived
  tables are meant to cover;
- The existing 15+17 entries are sufficient to lift grammar accuracy off
  zero on the easy majority case (Finnish has cleaner suffixation than
  Estonian; Estonian's harder cases were always going to need the FST).

### What This Closes

- **TODO #15** (three-part compound splitting) — DEFER to FST migration.
  The VFST handles compounds natively via concatenated `[Xp]...[X]`
  segments; see `pkg/lemmatizer-fi-et/voikkomap/` parser in PR #107.
- **TODO #16** (consonant gradation rules in suffix-strip) — REJECT.
  Gradation lives in the FST's lexicon-aware paradigm tables, not in
  string-rewrite rules over the surface.

Both items are restated in `TODO.md` under the
"FST migration supersedes" section.

### How to Revisit

If the FST migration stalls or is reversed, this decision should be
re-litigated. Until then, PRs that add suffix-table entries or implement
gradation/ternary-compound logic in `internal/parserules/` or
`internal/store/dict.go` should be redirected to
`pkg/lemmatizer-fi-et/` instead.

**See also:** [CHANGELOG.md §2026-05-06b — Eval harness parity + grammar-label stopgap](CHANGELOG.md).

---

## Decision 4: Parse Feedback Requires Login (v1)

**Date:** 2026-04-29

### Context

PR #53 introduces a parse-feedback flow: a user reviewing a parse result can flag
that the parser tagged a token incorrectly (wrong lemma/POS/grammar label) and
propose a correction. The endpoint that accepts this feedback (`POST
/api/parse/feedback`) needed an auth model.

### Decision

**Require login to submit parse feedback in v1.** No anonymous feedback path.

### Reasoning

- **Spam control.** Without an authenticated identity, the feedback queue is open
  to drive-by submissions with no rate-limit anchor and no way to hold a reporter
  accountable.
- **Admin signal-to-noise.** Admins reviewing the queue need to be able to
  weight reporters (returning user vs. one-time submitter). Anonymous breaks that.
- **Follow-up.** If a correction is ambiguous, admins need a way to ask the
  reporter for context. Anonymous feedback can't be followed up on.
- **Scope.** A "light anonymous feedback" path requires its own design (one-shot
  feedback tokens scoped to a parse session, separate rate limiting, separate
  admin UI). Out of scope for alpha.

### Trade-off Accepted

Some users will hit a parser bug, want to flag it, and won't bother creating an
account to do so — that signal is lost. We accept this for v1 in exchange for a
clean, low-noise feedback queue.

### Related Source-Text Retention Decision

The original product call here was **Option B**:

- do not persist parse sessions during `/api/parse`
- persist parse-session context only when a logged-in user explicitly submits
  feedback
- treat feedback submission as the consent boundary for storing source text

Rationale from a user perspective:

- The parse UI feels ephemeral; users will paste personal/sensitive content
  (private messages, work documents, copyrighted material) without expecting it
  to be stored.
- Storing only on feedback-submit aligns persistence with consent: the user has
  actively asked us to look at this parse, so retaining the context is
  justified.
- Eliminates unbounded growth from anonymous parse traffic.

### Amendment (2026-04-30): alpha shipped as Option A

Alpha shipped as **Option A** instead:

- authenticated `/api/parse` calls create `parse_sessions` rows
- anonymous `/api/parse` calls do **not** create `parse_sessions` rows
- anonymous parses do **not** persist source text
- parse feedback still requires login and still references a server-issued
  `parse_id`

We accepted the deviation from the original Option B decision for alpha because
it:

- solves the immediate unbounded-growth concern from anonymous parse traffic
- keeps the frontend and backend contract simpler
- preserves a clean path to a future parse-history UI for logged-in users

### Trade-off Accepted

This originally left a privacy gap for alpha because logged-in Inspect parses
were stored automatically. The current alpha closes that gap by making
`/api/parse` ephemeral by default, including for signed-in users. Source text is
stored only when a user saves a deck or submits parser feedback.

The remaining retention work is:

- a parse-history/deletion UI for saved deck and feedback source context
- per-user delete controls for stored parse sessions
- clear UI copy on the parse form, not only in documentation

### Post-v1 Reconsideration

If parser-quality work outgrows the volunteer feedback signal, revisit anonymous
"light feedback" as a separate, rate-limited path with its own queue.

**See also:** [CHANGELOG.md §2026-04-29 — Consumer alpha execution plan](CHANGELOG.md).

---

## Decision 3: Evaluation-Driven Development

**Date:** 2026-04-28

### Approach

Parser improvements are driven by **measured evaluation** against gold test cases,
not intuition.

### Infrastructure Built

- **Gold datasets:** `testdata/parser-eval/fi/gold/fi-manual-v1.json` (22 cases)
- **Eval CLI:** `cmd/parsertest` — runs parsers against datasets, outputs accuracy metrics
- **Metrics:** Lemma accuracy, POS accuracy, grammar label accuracy, coverage, timing

### Evaluation Workflow

```
1. Run eval:  go run ./cmd/parsertest -dataset fi-gold.json -parsers custom
2. Review failures: Which tokens did we get wrong?
3. Add rule: Fix the failure pattern
4. Re-run eval: Confirm improvement, no regressions
```

### Future: Automatic Improvement

**Status:** parked post-live idea. This section is historical context and
should not be treated as active roadmap work before FinnEst is shipped and
live. Do not block parser or product changes on autoresearch behavior unless a
user explicitly asks for it.

Inspired by [karpathy/autoresearch](https://github.com/karpathy/autoresearch), we plan
to build an automated improvement loop:

```
1. Agent modifies parser rules
2. Run eval automatically
3. If accuracy improves: commit and continue
4. If accuracy drops: revert and try different approach
5. Repeat overnight → 100 experiments vs 5 manual attempts
```

This requires:
- Larger gold dataset (100+ sentences, currently 22)
- Consolidated rules in one file (clear edit scope for agent)
- Automated git workflow

**Update 2026-05-06c:** the gold dataset expanded from 22 cases to ~14k
committed FI cases via UD treebank ingestion (Plan C / PR 1) — see
[CHANGELOG.md §2026-05-06c — UD treebank gold expansion](CHANGELOG.md).

---

## Decision 2: Parser Architecture

**Date:** 2026-04-28

### Current Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│  Rust Parser (parser/src/lib.rs)                                │
│  - NFC normalization                                            │
│  - Sentence splitting                                           │
│  - Tokenization                                                 │
│  - Heuristic POS guessing                                       │
└─────────────────────────────────────────────────────────────────┘
                              │
                              │ FFI (JSON)
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  Go Parse Core (internal/parsecore/parsecore.go)                │
│  - Production parser registry (basic, custom)                   │
│  - Dictionary resolution                                        │
│  - Enrichment rules (possessive, compound, case suffix)         │
│  - Gloss lookup                                                 │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  SQLite Dictionary (internal/store/dict.go)                     │
│  - forms table: surface form → lemma + POS                      │
│  - lemmas table: lemma + POS → gloss                            │
│  - Source: kaikki.org (Wiktionary-derived)                      │
└─────────────────────────────────────────────────────────────────┘
```

Eval-only baselines live in `internal/evalparsers`, which registers
`omorfi`/`estnltk`, owns adapter subprocess discovery and timeouts, and feeds
the normalized FFI-shaped result through parsecore's external-analyzer result
builder. The server-facing parser registry does not expose those lab modes.

### Parser Modes

| Mode | Description | Use Case |
|------|-------------|----------|
| `basic` | Dictionary lookup only, no enrichment | Speed baseline |
| `custom` | Dictionary + possessive/compound/case rules | Production parser |
| `omorfi` | External FI adapter for comparison | Evaluation only |
| `estnltk` | External ET adapter for comparison | Evaluation only |

### Enrichment Rules (in `custom` mode)

1. **Possessive suffix stripping** (Finnish)
   - `kirjassani` → strip `-ni` → `kirjassa` → lookup → `kirja`

2. **Compound word splitting** (Finnish + Estonian)
   - `pankkiautomaatti` → `pankki` + `automaatti` → lookup both

3. **Case suffix stripping** (Finnish + Estonian)
   - `kirjassa` → strip `-ssa` → `kirja` + grammar label "inessive"

### Update 2026-05-07: FST migration shipped

The original "Future Consideration: FST" note has been superseded.
The FST migration shipped via PRs
[#106](https://github.com/sagarinbabel/finnestdb/pull/106) /
[#107](https://github.com/sagarinbabel/finnestdb/pull/107) /
[#127](https://github.com/sagarinbabel/finnestdb/pull/127) /
[#129](https://github.com/sagarinbabel/finnestdb/pull/129) /
[#130](https://github.com/sagarinbabel/finnestdb/pull/130). The FST runtime
in [`pkg/lemmatizer-fi-et/`](../pkg/lemmatizer-fi-et/) is now wired in
parallel with dict step 1 — see Decision 16 above. Production tables are
generated locally; smoke fixtures are committed (Decision 9).

---

## Decision 1: Build a Custom Parser (Not Use Omorfi Directly)

**Date:** 2026-04-28

### Context

Omorfi (Open Morphology of Finnish) is the gold standard for Finnish morphological
analysis. It uses finite-state transducers (FST) built over 15+ years of linguistic
work. The question: should we use Omorfi directly, or build our own parser?

### Decision

**Build our own custom parser**, using Omorfi as a quality benchmark.

### Reasoning

| Factor | Our Parser | Omorfi |
|--------|-----------|--------|
| **Speed** | ~40–50k words/s on FI on `custom`, single core ([baseline](baselines/2026-05-06-fi-summary.md#measured-throughput)) | FST traversal + Python subprocess startup; not yet benchmarked under our harness |
| **Deployment** | Self-contained Go binary | Requires Python + HFST + .hfst files |
| **Customization** | Add a rule in Go, redeploy | Fork FST project, recompile transducers |
| **Licensing** | Permissive (we control it) | GPL-3.0 (copyleft implications) |
| **Estonian support** | Same architecture, different rules | Would need separate tool (EstNLTK) |

### Trade-off Accepted

We accept that our parser may not match Omorfi's accuracy on edge
cases. The goal is **comparable lemma/POS accuracy with deployment
and licensing properties Omorfi can't give us** — speed is a
property we own, not the headline argument.

### How We Measure Success

- **Accuracy:** Compare lemma/POS output against gold-annotated test cases.
- **Speed:** `cmd/parsertest` reports per-case latency (avg / p50 / p95
  in ms, ns-precision under the hood) and aggregate `words/s` and
  `chars/s` per parser per dataset. Current floor on Finnish is
  ~40–50k words/s on `custom` against the 2026-05-06 dictionary state;
  treat anything below that on the same datasets as a regression to
  triage. Speed claims must always cite a measurement — comparing
  finnestdb against external baselines requires running both under the
  same harness, not eyeballing numbers from external papers.
- **Coverage:** Percentage of tokens resolved to dictionary entries.

> **Speed claims policy:** never quote a "we're faster than X" number
> in this repo without a `cmd/parsertest` run on a comparable dataset
> and a link to the JSON report. The 2026-05-06 timer fix (PR #103)
> exists because we previously couldn't.

### Omorfi's Role

Omorfi is used as:
- A **benchmark** to measure our parser's accuracy against
- A **tool for generating gold annotations** when building test cases
- Not a production runtime dependency

---

## Open Questions

_Questions are date-tagged with the date they were first recorded._

1. **(2026-04-28) FST for novel words:** At what accuracy level do we need
   FST-like morphological analysis for unseen words? Current heuristics may
   plateau at ~95%. _Partial answer 2026-05-07: FST runtime now contributes
   in parallel with dict (Decision 16); the question evolves to "production
   table coverage targets" — see LEXICAL_PLAN.md "Production generated-table
   scope" open question._

2. **(2026-04-28) Gold data source:** Should we use Omorfi to generate
   candidate annotations, then human-verify? Or fully manual annotation?
   _Partial answer 2026-05-06c: UD treebanks now provide ~14k committed
   FI cases / ~8k local-only ET cases of human-checked morphology — see
   CHANGELOG.md §2026-05-06c. Manual gold remains for targeted regression
   probes._

3. **(2026-04-28) Auto-improvement scope:** Which files should the agent be
   allowed to modify? Just rules? Or also the Rust tokenizer?

4. **(2026-05-06) Production generated-table scope:** Pick the FI and ET
   word lists, table names, row-count targets, provenance format, and
   eval gates for the first production generated-table PR.

5. **(2026-05-06) ET generation path:** Add a generator command for local
   Giellalt/HFST Estonian analyses, analogous to the current FI VFST
   smoke generator.

---

## Historical: Project Roadmap (preserved)

> **Note:** The project roadmap moved to TODO.md's "What's in main" /
> "What's not in main yet" sections. The Track A/B/C breakdown below is
> preserved as historical context for how we framed the work in
> 2026-04-28, not as the current roadmap.

### Track A: Core Product (User-Facing)

| Phase | Work | Status |
|-------|------|--------|
| A1 | Parse Experience — results table polish, coverage gauge | Largely shipped |
| A2 | Deck Creation — "Save as Deck" CTA from results | Shipped |
| A3 | Known Words — mark known/ignored in results table | Shipped |
| A4 | Navigation Shell — nav bar, dark theme alignment | Shipped |
| A5 | SRS Core — review queue, card scheduling, session UI | Shipped |
| A6 | Known Words Loop — SRS → known list → dims in future parses | Shipped |
| A7 | Import Known Words — upload CSV of already-known vocabulary | See TODO.md |

### Track B: Parser Quality (Development Infrastructure)

| Phase | Work | Status |
|-------|------|--------|
| B1 | Gold Data Expansion — 100+ annotated FI sentences | Shipped (~14k via UD) |
| B2 | Baseline Benchmark — record current accuracy/speed | Shipped |
| B3 | Rule Consolidation — all rules in one file | Shipped |
| B4 | Omorfi Comparison — side-by-side accuracy measurement | Shipped |
| B5 | Auto-Improvement Loop — autoresearch-style experiments | Parked post-live idea |

### Track C: Estonian (Parallel Path)

| Phase | Work | Status |
|-------|------|--------|
| C1 | Estonian Gold Data — expand from 1 to 50+ cases | Shipped (~8k via UD-ET) |
| C2 | Estonian Dictionary — verify kaikki.org coverage | Shipped (Ekilex bulk) |
| C3 | Estonian Rules — case suffixes, compounds | Shipped |

---

## Changelog

| Date | Change |
|------|--------|
| 2026-04-28 | Initial decisions documented: custom parser rationale, architecture, evaluation approach, roadmap (Decisions 1–3) |
| 2026-04-29 | Decision 4 added: parse feedback requires login in v1; source_text persisted only on feedback submit |
| 2026-04-30 | Recorded parse-feedback persistence amendment: alpha ships authenticated parse-session storage as Option A |
| 2026-05-06 | Decision 5 added: freeze the case-suffix table; further morphology work goes into generated morphology tables under `pkg/lemmatizer-fi-et/tables/` |
| 2026-05-06 | Decision 6 added: numeric-hyphen tokenization (R1–R4) lives in the shared Rust tokenizer, no per-language rule tables |
| 2026-05-06 | Decisions 7–14 absorbed from `LEXICAL_PLAN.md` "Locked Decisions" (generated-table deployment, translations/definitions tables, production-table deferral, adapter packaging, Kotus distribution, ALTER TABLE migrations, Wikisanakirja for FI defs, kaikki.org `feats` not backfilled) |
| 2026-05-07 | Decision 15 added: single-folder bootstrap rule — every gitignored runtime artifact lives under `localdata/` |
| 2026-05-07 | Decision 16 added: FST contributes candidates in parallel with dict step 1 (PRs #127/#129/#130) |
| 2026-05-07 | Decision 17 added: ESTONIAN_LEXICAL_PLAN consolidated into `docs/LEXICAL_PLAN.md` (PR #135) |
| 2026-05-07 | Decision 18 added: IMPLEMENTATION.md split into PARSER_FEEDBACK_LOOP.md + README sections (PR #135) |
| 2026-05-07 | Document reordered latest-first; roadmap moved to TODO.md (preserved here as historical) |
| 2026-05-07 | Decision 5 amended (PR #139): case-suffix stopgap also projects UD `Case=` into `forms.feats` via `featsFromCaseLabel`; suffix table itself stays frozen. **Decision 14 (kaikki `feats` not backfilled) reversed**: `cmd/importdict/feats.go::kaikkiTagsToFeats` now projects Wiktionary tag arrays into UD FEATS at import time, populating `forms.feats` for every kaikki row |
| 2026-05-12 | Decision 20 added: lexical-overlay short-circuit and curated bad-lemma blocklists (PR #183) |
| 2026-05-12 | Decision 21 added: source priority outranks generic FST support and morphology ties in the merged dict+FST ranker (PR #187) |
| 2026-05-12 | Decision 22 added: source-backed ET learner corrections stay deterministic and source-audited |
| 2026-07-03 | Decision 23 added: public alpha learner model is surface-first with guided cold start and narrowly-scoped FSRS |
| 2026-07-03 | Decision 23 amended: alpha review card identity is surface-form-in-context before FSRS |
| 2026-07-03 | Decision 23 amended: homographs use sense-aware surface cards with sentence context and explicit contrast notes |
| 2026-07-03 | Decision 23 amended: explicit known vocabulary and FSRS maturity stay separate |
| 2026-07-03 | Decision 23 amended: ambiguous known-word imports resolve lazily through contextual meaning checks |
| 2026-07-03 | Decision 23 amended: parse-results meaning checks are non-blocking and pending until deck save |
| 2026-07-03 | Decision 23 amended: low-confidence ambiguity uses multiple-meaning UI plus flag-only parser feedback |
| 2026-07-03 | Decision 23 amended: parser confidence for meaning checks must be calibrated with a Finnish-first ambiguity eval slice |
| 2026-07-03 | Decision 24 added: product-grill decisions are promoted into `DECISIONS.md`, `CONTEXT.md`, `TODO.md`, and specs according to document role |
| 2026-07-03 | Decision 24 amended: before implementation handoff, consolidate navigation into TODO/INDEX/README/AGENTS instead of creating new execution docs |
| 2026-07-03 | Decision 25 added: preserve learning history while removing known-faulty content from circulation |
| 2026-07-03 | Decision 25 amended: minimal faulty-content quarantine is a public-alpha gate |
| 2026-07-03 | Decision 25 amended: correction issues are global and need report/fix/regression lifecycle tracking |
| 2026-07-03 | Decision 25 amended: reports create global issues first; quarantine follows admin action or trusted threshold |
| 2026-07-03 | Decision 25 amended: alpha quarantine is admin-only; trusted thresholds collect evidence but do not auto-suppress |
| 2026-07-03 | Decision 25 amended: alpha uses minimal issue schema, not a full correction platform |
| 2026-07-03 | Decision 25 amended: learner-facing quarantine quietly removes content from study queues |
| 2026-07-03 | Decision 25 amended: current learner-facing stats exclude quarantined content |
| 2026-07-03 | Decision 25 amended: quarantined content restores after fix by default unless target identity changes |
| 2026-07-03 | Decision 25 amended: restored content resumes scheduler state when target identity is unchanged |
| 2026-07-03 | Decision 25 amended: alpha admin triage uses simple required classification, full taxonomy optional |
| 2026-07-03 | Decision 25 amended: alpha admin UI supports quarantine/notes, not a broad in-app fix editor |
| 2026-07-03 | Decision 25 amended: alpha uses one combined admin feedback/issues queue with filters |
| 2026-07-03 | Decision 26 added: public alpha account access is open signup |
| 2026-07-03 | Decision 27 added: public alpha includes an anonymous parser demo with signed-in durability and a 1,000-concurrent-user planning target |
| 2026-07-03 | Decision 27 amended: anonymous parsing uses a stricter configurable text-size limit than signed-in parsing |
| 2026-07-03 | Decision 28 added: Finnish and Estonian launch with equal product status |
| 2026-07-03 | Decision 28 amended: FI/ET parity audit is journey-first, with metrics attached under each journey step |
| 2026-07-03 | Decision 29 added: public alpha go/no-go allows only classified non-dangerous rough edges |
| 2026-07-03 | Decision 29 amended: launch issue ledger lives in TODO.md, not a new document |
| 2026-07-03 | Decision 29 amended: first experience should be excellent about 95% of the time before public alpha |
| 2026-07-03 | Decision 29 amended: first-experience bar is measured by both release-candidate pack and week-one telemetry |
| 2026-07-03 | Decision 29 amended: week-one telemetry is aggregate by default, with signed-in product-event trails only and no pasted text |
| 2026-07-03 | Decision 29 amended: minimal telemetry is post-launch roadmap, not a public-alpha blocker if logs plus manual review are available |
| 2026-07-03 | Decision 29 amended: pre-alpha first-experience RC pack should be a checked-in repeatable FI/ET artifact |
| 2026-07-03 | Decision 29 amended: first-experience RC pack requires both automated checks and manual product walkthrough |
| 2026-07-03 | Decision 29 amended: manual RC walkthrough findings are severity-graded rather than binary |
| 2026-07-03 | Decision 29 amended: RC pack uses one shared manifest consumed by automated and manual checks |
| 2026-07-03 | Decision 29 amended: RC pack manifest and skeleton runner should be the first alpha implementation task |
| 2026-07-03 | Decision 29 amended: RC pack automated portion should have one top-level `make first-experience-rc` command |
| 2026-07-03 | Decision 29 amended: product name capitalization corrected to FinnEst |
