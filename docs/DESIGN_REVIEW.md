# Design Review — Aalto Edition (2026-05-07)

Review of `branding.html` and `finnest aalto.html` from the design bundle.

---

## branding.html — The FinnEst mark

### Strengths

- **Superscript-as-lemma-tag is the core insight.** The `fi`/`et` superscripts
  read exactly like the morphological annotations the parser produces — the
  wordmark literally *is* the product. Teaches what the tool does in a single
  glance.

- **Flag-blue differentiation.** Finnish blue (#003580) is darker/heavier,
  Estonian blue (#0072CE) is brighter/lighter. They read as a pair without
  labels.

- **Size scale with practical rules.** "Drop sups below 24px" is the kind of
  guidance that separates a real brand guide from a mood board.

- **Split-tile app icon (F|E)** is recognizable at small sizes. The ink/birch
  warm variant is the most distinctive — doesn't depend on knowing which blue
  is which.

### Issues to address

1. **`.lockup .book` is styled but never shown in any artboard.** Either show
   it with a concrete use case or remove the dead CSS.

2. **Fraunces variant defined (`.lockup.fraunces`) but never used.** Either add
   a card showing how it compares to Newsreader or drop the variant. Fraunces'
   wonky serifs would read quite differently.

3. **"Sanatorium" / "Paimio" naming is unexplained.** Needs a one-liner
   linking to Aalto's Paimio Sanatorium for anyone unfamiliar with the
   reference.

4. **No favicon artboard.** Section 02 says "use the two-tone tile below
   24px" but never proves it works at 16x16 / 32x32. The rounded corners will
   eat into glyph space at those sizes.

5. **Superscript spacing is fragile.** `margin-left: 0.55em` on `.est` is a
   magic number tuned to clear the floating `fi` sup. Breaks if the font or
   sup text changes. Document the "why" or calculate from actual width.

6. **No clear-space rules or don'ts section.** Show what NOT to do: squishing,
   recoloring to random colors, dropping sups at large sizes, etc.

---

## finnest aalto.html — App design system

### Strengths

- **Two-intensity system** (`data-aalto` restrained vs bold) is smart product
  thinking. Ship restrained, dial up Aalto-ness later without redesigning.

- **Birch wood-grain in pure CSS** — layered `repeating-linear-gradient` +
  `radial-gradient` is impressive. Dark-mode smoked oak variant is a nice
  touch.

- **Role token system** (`--bg`, `--bg-deep`, `--bg-raise`, `--bg-hover`)
  abstracts light/dark cleanly. Adding a third theme would be straightforward.

- **Vocabulary status colors** — known (sage), learning (warm wheat), new
  (blue) — are semantically clear AND colorblind-safe (differ in lightness,
  not just hue).

- **Paste box in Newsreader at 20px** is a bold serif-for-input choice that
  says "this is a reading/language tool" before you type a word.

### Issues to address

1. **Flag-blue inconsistency with branding.html.** Branding defines two
   distinct flag blues, but the app collapses them into one `--blue`. And
   `--et` maps to `--ink` (black), not Estonian blue. If the brand's premise
   is Finnish/Estonian duality, the app's FI/ET tags should use the actual
   flag blues.

2. **`--red-acc` defined but never used.** Either cut it or assign it to error
   states / destructive actions / overdue review items. The birch palette
   needs a warm interrupt color.

3. **No responsive breakpoints.** No `@media` queries. The separate
   `finnest mobile.html` exists but shares no responsive strategy with this
   file. Risk of two diverging design systems.

4. **`body { overflow: hidden }` means all scrolling is inside `.main`.**
   Verify the landing page works on shorter screens (laptop 768px height).

5. **`.form-pill.more` with dashed border** reads as "incomplete" or "drag
   target" in modern UIs. A `+` icon in a solid ghost pill would communicate
   "add more" more cleanly.

6. **Savoy-vase SVG** is styled (`.vase-svg`) but the actual path isn't in
   this file. Make sure the silhouette is abstract enough to read as
   decoration, not a literal vase.

---

## finnest prototype.html — Clickable prototype

Interactive prototype adds flows on top of the Aalto design system:
SignupRibbon, SaveAsModal, CorrectionModal, ColdStart, KnownWordsImport,
EphemeralToggle, ReviewDoneCard. Wired via proto-app.jsx + proto-screens.jsx
+ proto-flows.jsx.

### Strengths

- **Cold-start onboarding** ("Three ways to seed your first deck") is
  well-structured: paste text (recommended), import known words (Anki/CSV),
  seed top-1000 (gated/coming soon). Clear progressive disclosure.

- **CorrectionModal** has a smart two-path design: "I don't know the answer"
  (flag-only) vs "Right answer is…" (propose lemma + POS + grammar + notes).
  Flag-only is the recommended path, which is correct — most users won't know
  the right answer but can still signal a problem.

- **SignupRibbon** for anonymous users is well-placed — appears above results
  after a parse, not before. The user gets value first, then sees the save
  prompt. Good funnel design.

- **Tweaks panel** for toggling demo state (signed in/out, cold/warm start,
  ephemeral, Aalto intensity, theme) makes the prototype self-documenting for
  stakeholder walkthroughs.

### Missing flows — TODO

1. **Signed-in user correction submission.** The CorrectionModal exists and
   renders for signed-in users, but the submission flow is incomplete:
   - Where does the correction go after "Submit"? The toast says
     "→ /admin/feedback queue" but there's no queue view or triage screen.
   - Signed-in corrections should attach the user's identity so the submitter
     gets credit / notification when the correction is accepted or rejected.
   - Need a "My submissions" view (or at least a list in the user profile)
     showing pending / accepted / rejected corrections.
   - Consider: should corrections be per-parse (tied to the specific sentence
     context) or per-lemma (global)? The modal currently captures the sentence
     but the data model isn't shown.
   - **Action:** Add a corrections history panel accessible from the user
     avatar menu, and show submission status (pending / accepted / rejected)
     as a badge or inline in the parse results where the user flagged it.

2. **Leverage page — filter by deck.** The current leverage view shows words
   ranked across all 5 active decks with a single "+14% comprehension gain"
   summary. Missing:
   - **Deck filter / selector** at the top of the leverage page — let the user
     pick a single deck (or "all decks") and see leverage ranked within that
     scope. Each deck has different text, so the highest-leverage words differ.
   - **Current → target comprehension projection.** Instead of just "+14%",
     show: "You currently understand **62%** of *Kalevala ch.1*. Learn these
     15 words → **78%**." The user needs to see where they are now AND where
     they'd get to. This is the core motivation loop.
   - **Goal threshold.** Let the user set a target comprehension % (e.g. 90%)
     and show exactly how many words they need to learn to reach it. The number
     of words is the gap between current known coverage and the target.
   - **Auto-create deck from leverage words.** When viewing leverage for a
     specific deck, add a "Create study deck" button that bundles the top N
     leverage words (up to the target %) into a new FSRS deck automatically.
     Flow: user picks a deck → sees leverage ranking → sets target (e.g. 85%)
     → clicks "Create deck for these 23 words" → SaveAsModal opens with the
     words pre-selected → deck appears in Decks view, ready for review.
   - **Action:** Add deck filter dropdown, current/target comprehension bar,
     and "Create leverage deck" CTA to the leverage page.

3. **Leverage → Deck creation flow (end-to-end).** The "Add top 10 to queue"
   button exists but is ambiguous — which queue? The user should be able to:
   - Select specific leverage words (checkboxes or shift-click)
   - Click "Create deck from selected" or "Add to existing deck"
   - Reuse the SaveAsModal with the selected words pre-populated

---

## Cross-cutting recommendations

1. **Commit to the superscript lockup in the app topbar.** The `Finn^fi Est^et`
   mark is the strongest design element in the system. The branding guide
   section 04 shows it in the topbar at 26px — use it. The current app topbar
   shows `finnest.` in italic serif, which wastes the brand's best asset.

2. **Unify flag blues.** The branding guide defines Finnish blue (#003580) and
   Estonian blue (#0072CE) as distinct colors. The app should use both — not
   collapse them into one `--blue`. This matters especially on the leverage
   page where FI and ET deck tiles are shown side by side.

3. **Correction flow is the highest-priority gap.** The prototype has the
   modal but no backend flow. For a language tool, user corrections are the
   primary quality signal — this needs a complete round-trip: submit → triage
   → accept/reject → notify user → update parse data.
