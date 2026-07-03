# FinnEst Product Context

FinnEst helps learners pre-learn Finnish and Estonian vocabulary from real
texts they want to read. The product combines dictionary-backed parsing,
source-text deck creation, global learner vocabulary state, review, and an
admin-only parser-improvement loop.

## Language

**Learner**:
A signed-in person using FinnEst to inspect texts, create decks, import known words, and review vocabulary.
_Avoid_: customer, student, account

**Product Name**:
The user-facing product/brand is **FinnEst**. The capital `E` reflects the
Finnish + Estonian scope. The local repo folder, `finnestdb` module paths,
`finnestdb.db`, historical docs, and GitHub URLs can remain technical names
until an explicit engineering rename. User-facing copy and current product docs
should say FinnEst, not FinEstDB, Finnest, or FinnestDB.
_Avoid_: FinEstDB as product brand, Finnest, FinnestDB

**Equal-Status Languages**:
Finnish and Estonian are both first-class public-alpha languages. Do not label
one as experimental or secondary. If a gap exists, name the concrete
data/parser/catalog/UX/test asymmetry and classify it.
_Avoid_: Finnish-first product, Estonian add-on, experimental Estonian

**Non-Dangerous Rough Edge**:
A known alpha issue that does not affect privacy/security, retention/account
deletion, data integrity, review state, known-word truth, parser
feedback/quarantine safety, FI/ET equal status, overload behavior, or truthful
UI copy, and has an honest workaround/retry path. See
`docs/GO_LIVE_CHECKLIST.md` "Alpha Go/No-Go Rubric".
_Avoid_: anything we do not want to fix, vague launch risk

**First-Experience Quality Bar**:
The first anonymous or signed-in alpha journey should feel excellent about 95%
of the time in release-candidate testing. This means credible parser/card output
in the first screenful, truthful UI state, completed flow, and no latency/error
behavior that makes the product feel unreliable. Gate public alpha with a
checked-in, repeatable FI/ET release-candidate pack that combines automated
parser/browser checks with a short manual product walkthrough. The canonical
pack manifest should live at `testdata/first-experience-rc/manifest.json`.
Grade findings as `blocker`, `serious`, or `minor`; validate after launch with
privacy-preserving week-one telemetry when available.
_Avoid_: uptime-only metric, polish-only metric

**Release-Candidate Pack Manifest**:
The single checked-in source of truth for pre-alpha first-experience cases. It
should be consumed by parser checks, Playwright specs under `web/tests`, and the
manual product walkthrough so all three validate the same texts, journeys, and
expected outcomes. Create the manifest and a small skeleton runner first, even
if it fails initially, then make it pass as alpha flows land. The automated
portion should be exposed through `make first-experience-rc`, which runs parser
fixture checks and Playwright RC specs, then points at the manual walkthrough
instructions in `docs/GO_LIVE_CHECKLIST.md`. The manifest is data/fixtures
only; walkthrough instructions are not a separate planning document.
_Avoid_: separate manual checklist, duplicated Playwright-only fixtures

**Week-One Telemetry**:
Privacy-preserving post-launch metrics used to validate the
**First-Experience Quality Bar**. Aggregate by default. Per-user event trails
are allowed only for signed-in users, only for product events needed to debug
onboarding failures, and never for pasted source text.
_Avoid_: source-text analytics, anonymous user profiling

**Inspect**:
The signed-in learner surface for parsing pasted or uploaded Finnish or Estonian text.
_Avoid_: parser workbench, admin parser, full parser UI

**Parser Workbench**:
The admin-only surface for comparing parser modes and inspecting parser internals.
_Avoid_: Inspect, learner parse page

**Source Text**:
Finnish or Estonian text supplied by a learner for parsing, deck creation, or feedback context.
_Avoid_: corpus, document

**Embedded Text**:
A curated Finnish or Estonian text offered to signed-in learners who do not yet have their own source text ready. Prefer a complete coherent text from the redistributable subset of the corpus when license and size allow it; the UI may show a preview excerpt, but the product action should load the selected text itself.
_Avoid_: fixture, seed deck, corpus row

**Embedded Catalog**:
The checked-in catalog of curated **Embedded Texts**. It should be generated from local corpus tooling, store fast metadata separately from lazy-loaded full text, and cover FI/ET stories, articles, and poems across Easy/Medium/Hard buckets.
_Avoid_: broad corpus browser, runtime corpus dependency

**Global Difficulty**:
The catalog's fixed Easy/Medium/Hard label computed from text-level metrics and human-sanity-checked before launch.
_Avoid_: manual vibe label

**Personalized Text Fit**:
A learner-specific overlay for an **Embedded Text**, computed from the learner's known-word state when available. It answers "how much of this text do you likely already know?" while **Global Difficulty** remains the fallback when no known-word data exists. Current code computes this from lemma-backed state; the product direction is surface-first known vocabulary.
_Avoid_: personalized global level

**Anonymous Parser Demo**:
The unsigned public alpha surface (shipped 2026-07-04): paste text on the
landing form, parse it, get a parsed word list, and explore the list. It proves
parser quality before signup, but it is stateless and ephemeral. It uses a
stricter configurable text-size cap than signed-in parsing
(`FINNESTDB_ANON_MAX_CHARS`, default 20,000; signed-in stays 1,500,000).
_Avoid_: anonymous learning, anonymous deck, anonymous feedback

**Inspect Parse**:
An ephemeral parse result returned by `/api/parse`; it is not retained until the learner saves a deck or submits parser feedback.
_Avoid_: parse session

**Parse Session**:
A retained source-context record created only when a learner saves a deck or submits parser feedback.
_Avoid_: inspect parse

**Deck**:
A saved source-derived study object containing sentences and word occurrences.
_Avoid_: course, list

**Official Deck**:
A public deck published by an admin and visible in the official-decks tab.
_Avoid_: shared catalog deck, public list

**Card**:
A learner-level review item: a surface-form-in-context card, where the learner reviews the exact form they encountered or chose and lemma/POS/dictionary evidence is supporting metadata. Current implementation (2026-07-04): `cards` are keyed by `(user, language, surface_norm, lemma, POS)` — the normalized surface joins the key, with `(lemma, POS)` as the sense discriminator so homographs are distinct sense cards, not collapsed. FSRS memory (behind `FINNESTDB_FSRS_ENABLED`, default off) attaches to this surface-card id.
_Avoid_: deck word, flashcard copy

**Surface Form**:
The exact word form as it appears in source text.
_Avoid_: word when lemma identity matters

**Dictionary Entry**:
A `(language, lemma, POS)` identity resolved from dictionary, FST, lexical overlay, or custom override evidence.
_Avoid_: word, card

**Multi-Lemma Surface**:
A surface form that maps to more than one dictionary entry, producing multiple sense-aware review cards when safely supported by parser/dictionary evidence. The card should show sentence context and explain that the same-looking word can mean something else, such as a verb form versus a noun.
_Avoid_: duplicate word

**Known Surface Form**:
A learner-level assertion that the learner knows an exact **Surface Form** in a language. This comes only from explicit learner evidence: import, manual confirmation, test-out confirmation, or marking a card known. This is the product-facing model to use for import, top-N presets, and personalized fit because it preserves what the learner actually claimed to know.
_Avoid_: treating one known lemma as proof that every inflected form is known

**Known Surface Sense**:
A learner-level assertion that the learner knows a specific **Surface Form** with a specific resolved meaning/sense. This is created when a context-free known surface has multiple supported meanings and the learner confirms the intended meaning in context.
_Avoid_: assuming every same-looking word is known

**Meaning Check**:
A lightweight contextual prompt for an imported **Known Surface Form** that has multiple supported meanings. It shows a real sentence, the intended meaning, the same-looking alternative meaning, and lets the learner confirm "I know this meaning" or choose "Study this meaning." In parse results before a deck exists, "Study this meaning" means "include this meaning as a review card when I save this deck"; in deck/test-out/review contexts it creates or keeps the review card now.
_Avoid_: abstract disambiguation quiz

**Multiple Possible Meanings**:
The measured-ambiguity version of a **Meaning Check**. Use it when the parser can list candidate meanings but does not have a measured basis for safely presenting one contextual meaning as intended. It should show candidate meanings with per-candidate known/study actions and a separate "None of these looks right" parser-feedback action.
_Avoid_: pretending the parser knows the intended meaning

**Parser Confidence**:
The system's measured confidence that a surface in a specific sentence maps to one contextual **Dictionary Entry** rather than another. This is about parser disambiguation, not learner knowledge or review maturity. For alpha, calibrate it with deterministic eval slices before using it to simplify learner UI.
_Avoid_: learner confidence, FSRS confidence

**Parser Feedback**:
A learner-submitted correction or issue report attached to retained source context for admin triage. It is for "the app's analysis looks wrong", not for "I do not know this word".
_Avoid_: study choice, known-word confirmation

**Flag-Only Parser Feedback**:
A parser-feedback item where the learner says the analysis looks wrong but does not claim to know the correct lemma, POS, grammar, or meaning. Shipped 2026-07-04 (Phase 1b): the correction modal offers a flag-only path (default) alongside "propose a fix", `parse_feedback.flag_only` records it, and the admin queue can filter flag-only reports and show a badge. It does not write a **Custom Override**; an admin can supply a concrete lemma/POS on a flag-only row at accept time, converting it into a normal parser-identity correction that then writes the override. This backs the **None of these looks right** escape hatch in **Multiple Possible Meanings**.
_Avoid_: fake correction, known-word decision

**Correction Overlay**:
An accepted admin correction that changes the smallest learner-facing layer that was wrong: parser identity, meaning cue, contextual sense, phrase boundary, example quality, or card presentation. Parser-identity overlays can change future parsing; meaning/card overlays change rendering and study content without pretending old reviews were different.
_Avoid_: rewriting learner history, one giant manual note

**Correction Issue**:
A global, admin-triaged problem record created from one or more learner feedback reports. Shipped 2026-07-04 (Phase 1c) as the `correction_issues` table with a `(lang, parser, norm_surface, lemma, pos)` scope fingerprint, `status` (`open`/`quarantined`/`fixed`/`reopened`), report/distinct-reporter counts, first/last-reported timestamps, quarantine/fix metadata, `reopened_count`, and an alpha class. Feedback submission groups a report into a found-or-created issue and links it via `parse_feedback.correction_issue_id`. It answers "is this the same problem we already fixed, or a new uncovered case?"
_Avoid_: isolated per-user complaint

**Faulty Content Quarantine**:
An admin-approved suppression state for a known-bad deck occurrence or review card. Shipped 2026-07-04 (Phase 1c): quarantining a **Correction Issue** removes matching content from review/new-card queues, deck word/due/new-card counts, and comprehension coverage/unlocks for every matching learner, while `review_log` history and `card_state` scheduler state stay untouched. Restore is a status flip that returns the same content to circulation with scheduler state intact. Overlay/replacement fixes remain future work.
_Avoid_: deleting history, silently retconning reviews

**Trusted Quarantine Threshold**:
A future deterministic rule that could promote a global **Correction Issue** from reported to quarantined without manual review, usually based on multiple distinct authenticated reports against the same scoped issue. For alpha, the system collects the evidence — a `threshold_candidate` flag appears at ≥3 distinct reporters — but never auto-quarantines.
_Avoid_: one-click mob voting

**Emergency Quarantine**:
The admin **Quarantine now** action that immediately suppresses a scoped **Correction Issue** for all matching learners. Shipped 2026-07-04 (Phase 1c): it requires a prior alpha class and a non-empty reason. Use when leaving the content live is worse than temporary over-suppression.
_Avoid_: silent delete

**Learning History**:
The record of what a learner was shown, how they answered, and how scheduler state evolved at the time. This should be preserved even when later parser feedback proves the content was faulty.
_Avoid_: mutable card copy

**Review Maturity**:
A derived scheduler state from review history/FSRS, such as learning, retained, or mature. It can inform due dates and optional comprehension estimates, but it must not silently rewrite **Known Surface Form** evidence.
_Avoid_: known word, learned word

**Ignored Lemma**:
A learner-level suppression state for one `(language, lemma, POS)` entry that should not be added to review.
_Avoid_: deleted word

**Known Lemma**:
The current implementation's persisted known state for one `(language, lemma, POS)` entry in `user_known_lemmas`. Imports currently accept surface strings, resolve them through dictionary/FST fallback, and store the resolved lemma/POS. Treat this as existing implementation detail, not the final product abstraction.
_Avoid_: using this term as the product-facing synonym for known vocabulary

**Custom Override**:
The highest-priority lexical source written when an admin accepts a lemma/POS parser feedback item.
_Avoid_: manual fix, user dictionary

**Track A**:
Offline parser quality measured against frozen gold datasets and external analyzer baselines.
_Avoid_: parser tests

**Track B**:
Live parser quality measured from real learner feedback and accepted-correction rates.
_Avoid_: analytics, telemetry

**Alpha Step Scheduler**:
The current hand-rolled review scheduler behind the Again/Hard/Good/Easy buttons.
_Avoid_: FSRS

## Relationships

- A **Learner** creates **Decks** from **Source Text** through **Inspect**.
- An **Inspect Parse** becomes a **Parse Session** only when the learner saves a **Deck** or submits **Parser Feedback**.
- A **Deck** contains source sentences and occurrences of **Surface Forms**.
- A **Surface Form** can resolve to one or more **Dictionary Entries**.
- Product-target **Cards** belong to one **Learner** and a learner-visible **Surface Form** plus resolved sense/context, with **Dictionary Entries** linked as derived support. Current implementation-backed **Cards** belong to one **Learner** and one **Dictionary Entry**, and can be reviewed from multiple **Decks**.
- **Known Surface Forms** are the target learner-known vocabulary evidence.
- Ambiguous imported **Known Surface Forms** are resolved lazily through **Meaning Checks** into **Known Surface Senses** only when that surface appears in useful context.
- **Review Maturity** is derived from **Card** review history and stays separate from **Known Surface Forms**.
- Current implementation-backed **Known Lemmas** and **Ignored Lemmas** apply globally for the learner across decks.
- Accepted lemma/POS **Parser Feedback** writes a **Custom Override**.
- **Flag-Only Parser Feedback** is triage signal only until an admin turns it
  into a concrete accepted parser-identity correction.
- **Correction Issues** are global, not per-learner. Multiple reports can attach
  to one issue, and confirmed quarantine/fixes apply to every matching learner
  surface.
- **Correction Overlays** and **Faulty Content Quarantine** update what current
  and future learners see without rewriting **Learning History**.
- **Track A** guards parser releases before deployment; **Track B** tells whether real learner corrections show remaining parser pain.

## Example Dialogue

> **Dev:** "If a learner imports `kissoja`, should we only store `kissa/NOUN`?"
> **Domain expert:** "No. Preserve the submitted **Known Surface Form** as the evidence. Lemma/POS resolution is still useful for cards and coverage, but it should be derived from the surface claim rather than replacing it."

> **Dev:** "Can we show this parse in History?"
> **Domain expert:** "Only if the **Inspect Parse** became durable. Plain inspect output is ephemeral; **History** lists retained **Parse Sessions** from saved decks or feedback."

## Flagged Ambiguities

- "Parse page" has meant both learner **Inspect** and admin **Parser Workbench**. Use the precise term.
- "Known word" can mean a surface form, a dictionary entry, or review maturity. Product logic should move toward **Known Surface Form** as explicit learner evidence and keep **Review Maturity** as derived scheduler evidence. When describing current schema behavior, say **Known Lemma** explicitly and note that it is implementation-backed.
- "Deck word count" is not necessarily unique surface forms. **Multi-Lemma Surfaces** can expand into multiple **Dictionary Entries**.
- "Card identity" is now a launch-critical term. Public alpha should migrate review cards to the surface-form-in-context model before attaching real FSRS memory; do not build long-lived FSRS state on temporary lemma/POS card identity.
- "Study this meaning" is a card-creation/retention action. In parse results, the UI should label it as "creates a review card when you save"; in saved deck/review contexts, label it as "creates/keeps a review card."
- "Correction" or **Parser Feedback** should only be offered for "the app seems wrong." Unmeasured or low parser confidence should use **Multiple Possible Meanings**, not a fake known/not-known question.
- Current shipped parser feedback is exact-correction-only. Alpha ambiguity UX
  needs **Flag-Only Parser Feedback** before **None of these looks right** can
  be shipped honestly.
- "Do not rewrite learner history" does not mean "leave known-bad cards in
  circulation." The target is immutable **Learning History** plus mutable
  corrected/quarantined learner-facing content.
- "Global quarantine" in alpha means admin-confirmed quarantine by scoped
  **Correction Issue**. Trusted-threshold evidence is collected for traceability
  and future automation, but it does not auto-suppress content yet.
- **Emergency Quarantine** is deliberately global and immediate, but it must be
  admin-triggered, scoped, reasoned, and logged.
- The UI has Again/Hard/Good/Easy review buttons, but the current scheduler is the **Alpha Step Scheduler**, not FSRS.
- Public alpha access is **open signup**. Learners can create accounts without
  an invite or waitlist.
- Anonymous public alpha means **Anonymous Parser Demo**, not full anonymous
  learning. Durable/personalized/accountable actions require sign-in.
- Anonymous parsing should not use the full signed-in 1,500,000-character cap.
  Enforce a lower configurable demo cap before expensive parser work and prompt
  signup for longer texts.
- Email verification should not block the first learner loop after signup.
  Verification can gate higher-risk actions such as high-volume parsing,
  repeated feedback, exports if enabled, account recovery, and trust-weighted
  correction signals.
- Hosted alpha should be planned around roughly 1,000 concurrent users, with
  graceful degradation above that. Throttle anonymous and oversized parses
  first; preserve core signed-in review/deck actions where possible.
- Finnish and Estonian are **Equal-Status Languages** for public alpha. A
  Finnish-first implementation/eval sequence is only acceptable when followed by
  ET parity before product behavior or copy implies unequal support.
