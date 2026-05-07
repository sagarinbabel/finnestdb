# Design Review (2026-05-07)

Review of the FinnEst `design/` folder. Files checked into the repo are
referenced as `design/<name>`; files marked **(local only)** exist in the
working copy but are not committed — they were reviewed from the design
bundle delivered separately.

**Checked in:** `wireframe-clickthrough.html`, `flow-diagram.html`,
`finnest.html`, `finnest v2.html`, `finnest mobile.html`, `app.jsx`,
`view-*.jsx`, `v2-*.jsx`, `m-*.jsx`, `design-canvas.jsx`,
`tweaks-panel.jsx`, `components.jsx`, `data.jsx`.

**Local only (not in repo):** `branding.html`, `finnest aalto.html`,
`finnest prototype.html`, `proto-app.jsx`, `proto-screens.jsx`,
`proto-flows.jsx`, `aalto-app.jsx`, `aalto-landing.jsx`,
`finnest-aalto-standalone.html`.

---

## branding.html — The FinnEst mark (local only)

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

## finnest aalto.html — App design system (local only)

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

## finnest prototype.html — Clickable prototype (local only)

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

See consolidated TODO list at the end of this document.

---

## wireframe-clickthrough.html — Alternate design direction

Self-contained HTML/CSS/vanilla JS clickthrough (80KB, no React). Represents a
distinctly different design direction from the Aalto prototype: dark-first INK
theme, vermillion accent (`oklch(0.72 0.19 38)`), Fraunces/IBM Plex stack,
hash-based routing with `data-go` click delegation.

### Strengths

- **Comprehension projection on Deck Detail is exactly what was missing from
  the Aalto prototype.** Shows "current → if you learn top N words → target %"
  with three concrete tiers (20, 50, 100 words). This is the core motivation
  loop — port this pattern back to the Aalto leverage page.

- **Coverage bar per deck** ("71% · based on your 1,204 known FI words")
  anchors the user's position before showing what they could gain. Much
  stronger than the Aalto prototype's "+14% across all decks" which lacks a
  baseline.

- **Mobile responsive breakpoints** (`@media max-width: 720px`) — the only
  file in the design folder that actually has them. Stacks grid layouts,
  resizes the paste box, and hides the topnav behind a burger.

- **Distinct FI/ET colors** — FI as cyan (`oklch(0.78 0.13 200)`) and ET as
  magenta (`oklch(0.74 0.16 320)`). These hue-differentiated tags are better
  than the Aalto approach of collapsing both into one `--blue`, and better than
  the branding hex blues because they work in both light and dark mode via
  OKLCH lightness adjustment.

- **Flowstrip UI** across the top with 11 labeled screens + keyboard nav
  (← →) + hotspot toggle. Excellent for stakeholder walkthroughs — shows all
  flows in linear order without needing to discover them.

- **Correction modal** with both flag-only and propose-answer paths matches
  the proto-flows.jsx design. Good consistency across the two codebases.

### Issues to address

1. **Two diverging design systems.** The wireframe and the Aalto prototype
   share no code, no CSS tokens, and only partially overlap in structure. This
   is the biggest risk in the design folder: decisions made in one don't
   propagate to the other. Need to converge on ONE token system and extract
   shared components.

2. **Comprehension projection uses hardcoded tiers** (20/50/100 words). Should
   be data-driven from the actual leverage ranking: "Learn the next N words to
   reach X%" where X is a user-configurable target (default 85%).

3. **No review flow.** The wireframe has a Review screen (screen 11) but it
   only shows a single static card. The Aalto prototype's FSRS-driven review
   loop with scope selection, progress bar, and ReviewDoneCard is much further
   along.

4. **Flowstrip numbering starts at 0.** "00 Landing" reads as a developer
   index, not a user-facing label. Start at 1 or drop the numbers.

5. **`data-go` routing is fragile.** Every clickable element needs a manual
   `data-go="screen-name"` attribute. No URL structure, no back button support
   beyond browser history. Fine for a throwaway clickthrough, but any further
   investment should move to hash-routes (which the file partially does) or
   React Router.

6. **No dark/light toggle.** The wireframe is INK-only. The Aalto prototype
   supports both Paimio (light) and Sanatorium (dark) via `data-theme`. The
   wireframe should at minimum include the PAPER token set for parity.

---

## View components (view-*.jsx) — Modular screen implementations

Five standalone React components (`view-parse.jsx`, `view-library.jsx`,
`view-deck.jsx`, `view-leverage.jsx`, `view-review.jsx`) extracted from
the app shells. These are the most production-ready pieces in the folder.

### Strengths

- **view-deck.jsx has the comprehension projection panel** ("Surasura-inspired"
  per the comment). Shows NOW % → arrow → IF YOU LEARN TOP 100 → TARGET % with
  a HeatStrip visualization. This is the best implementation of the feature and
  should be the canonical reference.

- **view-leverage.jsx has a proper scope filter** (All decks / Active only /
  Finnish / Estonian) — partially addresses the "filter by deck" TODO from the
  prototype review. The four scope options are a good start; adding per-deck
  filtering would complete it.

- **Leverage table has per-row deck presence indicators** — colored squares
  showing which of the user's 5 decks contain each word. Smart information
  density.

- **view-parse.jsx** (19KB) is the most complete parse UI — text input with
  language auto-detect, word filtering by status/MWE, sort by order/leverage/
  alpha, selected-word detail panel, and sentence highlighting.

### Issues to address

1. **Comprehension projection lives in DeckView but not LeverageView.** The
   deck detail shows "now 62% → learn top 100 → 80%" but the leverage page
   only shows the delta ("+14%"). The leverage page should show the same
   now→target projection, scoped to whichever deck is selected.

2. **"Add top 10 to queue" button on leverage is still ambiguous.** Which
   queue? The view-leverage.jsx has "+ Queue" per row plus "Add top 10 to
   queue" in the header, but neither specifies which deck the words go into.
   Need: "Create deck from top N" or "Add to [deck name]" with a deck picker.

3. **view-review.jsx** has FSRS card rendering but no correction affordance —
   the "✎ Wrong?" button only exists in proto-screens.jsx as a DOM injection.
   Should be built into the review component natively.

---

## Mobile design (finnest mobile.html + m-*.jsx)

Mobile prototype runner using `DesignCanvas` to showcase three artboards:
Landing, Wordlist, Review. Components in `m-shared.jsx` and `m-web.jsx`.

### Strengths

- **SwipeReviewCard** with drag-to-review gesture and particle burst animation
  is a genuinely native-feeling interaction. Better than tap-only for mobile.

- **PWA frame** (`MWFrame`) simulates iOS status bar and browser chrome,
  making the prototype look like a real app during walkthroughs.

- **`useStaggerReveal` hook** for animate-in transitions on the wordlist gives
  the mobile version more polish than the desktop prototype.

### Issues to address

1. **Only 3 screens.** Desktop has 11+ screens/routes; mobile has 3 (Landing,
   Wordlist, Review). Missing: Decks/Library, Deck Detail, Leverage, Settings,
   Cold Start, Sign-in. These are the screens that matter most for retention.

2. **No shared responsive strategy.** The mobile prototype is a completely
   separate codebase from the desktop views. The wireframe-clickthrough.html
   already has `@media` breakpoints that handle mobile layout — those should be
   the canonical responsive approach, not a parallel mobile app.

3. **design-canvas.jsx** (31KB) is a Figma-like inspection framework used only
   by the mobile prototype. Impressive engineering but heavy infrastructure for
   3 artboards. Worth keeping as a tool, but shouldn't be on the critical path.

---

## Design folder meta-observations

### File inventory by generation

| Generation | Files | Direction |
|---|---|---|
| **Wireframe** (INK, Fraunces, vermillion) | `finnest.html`, `wireframe-clickthrough.html`, `flow-diagram.html`, `app.jsx`, `view-*.jsx` | Dark-first, accent-driven, IBM Plex stack |
| **Aalto** (Newsreader, birch, flag-blues) | `finnest aalto.html`, `aalto-app.jsx`, `aalto-landing.jsx`, `branding.html` | Warm naturalistic, serif-heavy, light-first |
| **Prototype** (Aalto + flows) | `finnest prototype.html`, `proto-*.jsx` | Aalto tokens + interactive flows |
| **Mobile** | `finnest mobile.html`, `m-*.jsx` | Swipe gestures, PWA frame |
| **Tooling** | `design-canvas.jsx`, `tweaks-panel.jsx` | Inspection infrastructure |
| **Standalone** | `finnest-aalto-standalone.html` (2.5MB) | Distribution bundle |

### Convergence plan

The wireframe and Aalto directions need to merge. The wireframe has better
structural thinking (responsive breakpoints, comprehension projection, distinct
FI/ET colors). The Aalto direction has better surface polish (birch texture,
Newsreader serif warmth, two-intensity system). Recommended convergence:

1. **Token system:** Use the wireframe's OKLCH token structure (it already has
   paper/ink variants) but adopt the Aalto role names (`--bg-raise`, `--bg-deep`).
   Both already use these — just align the values.

2. **FI/ET colors:** Use the wireframe's hue-differentiated approach (cyan FI,
   magenta ET in OKLCH) rather than the branding hex blues. They're more
   versatile across themes.

3. **Typography:** The wireframe's Fraunces + IBM Plex Sans + IBM Plex Mono
   stack is more cohesive than Aalto's Newsreader + Inter Tight + IBM Plex Mono.
   Fraunces has optical sizing and weight axes that Newsreader lacks.

4. **Components:** The `view-*.jsx` files are the most production-ready. Build
   from these, adding the Aalto prototype's flow components (CorrectionModal,
   SaveAsModal, ColdStart, SignupRibbon) on top.

5. **Responsive:** Use the wireframe-clickthrough's `@media` breakpoints as
   the canonical mobile strategy. Retire the separate mobile prototype.

---

## Updated cross-cutting recommendations

1. **Commit to the superscript lockup in the app topbar.** The `Finn^fi Est^et`
   mark is the strongest design element in the system. The branding guide
   section 04 shows it in the topbar at 26px — use it. The current app topbar
   shows `finnest.` in italic serif, which wastes the brand's best asset.

2. **Unify flag blues.** The branding guide defines Finnish blue (#003580) and
   Estonian blue (#0072CE) as distinct colors. The app should use both — not
   collapse them into one `--blue`. This matters especially on the leverage
   page where FI and ET deck tiles are shown side by side. The wireframe's
   OKLCH hue-differentiated colors (cyan FI, magenta ET) are the best
   implementation of this principle.

3. **Correction flow is the highest-priority gap.** The prototype has the
   modal but no backend flow. For a language tool, user corrections are the
   primary quality signal — this needs a complete round-trip: submit → triage
   → accept/reject → notify user → update parse data.

4. **Port comprehension projection to the leverage page.** The DeckView
   (view-deck.jsx) and wireframe deck detail both have now→target projection.
   The leverage page needs the same treatment: show current coverage, show
   what learning the top N words would achieve, let the user set a target %.

5. **Converge the two design directions.** The wireframe and Aalto prototype
   are diverging. Pick one token system, one type stack, one responsive
   strategy. See "Convergence plan" above.

---

## Consolidated design TODO

All open items collected from the reviews above, grouped by priority.

### P0 — Core user flows (missing or broken)

1. **Leverage → word list → study deck (end-to-end).** The comprehension
   projection says "Learn 20 words → 81%" but the user can't see *which* 20
   words, can't review them, can't act on the number. Full flow:

   a. **Deck Detail or Leverage page** shows the projection: "You know 62%.
      Learn these 20 words → 81%." The "these 20 words" text is a link /
      expandable section.
   b. **Clicking it reveals the ranked word list** — the actual 20 lemmas,
      with gloss, POS, frequency bar, and a checkbox per row (all checked by
      default). The user can uncheck words they don't want or adjust the
      target % slider to grow/shrink the list.
   c. **"Create study deck" button** at the bottom of the list. Opens
      SaveAsModal with the selected words pre-populated. Deck name defaults
      to something like "Kalevala ch.1 — top 20 leverage words".
   d. **Deck appears in the Decks view**, tagged as a leverage-derived deck
      (e.g. a small ⚡ badge). User can start reviewing immediately.
   e. **As the user learns words from the leverage deck**, the comprehension
      projection on the source deck updates in real time: "You know 68%.
      Learn these 14 remaining words → 81%." The feedback loop closes.
   f. **When all leverage words are learned**, the projection tier collapses
      and the next tier appears: "You know 81%. Learn 30 more → 89%."

   Current state: view-deck.jsx shows the now→target numbers. The wireframe
   shows three hardcoded tiers. Neither lets the user see the words or create
   a deck. The "Add top 10 to queue" button in view-leverage.jsx is ambiguous
   (which queue?). This is the highest-priority feature gap.

2. **Signed-in user correction submission round-trip.** The CorrectionModal
   exists and renders, but the flow after "Submit" is incomplete:
   - Where does the correction go? The toast says "→ /admin/feedback queue"
     but there's no queue view or triage screen.
   - Signed-in corrections should attach user identity so the submitter gets
     credit / notification when the correction is accepted or rejected.
   - Need a "My submissions" view (or at least a list in the user profile)
     showing pending / accepted / rejected corrections.
   - Should corrections be per-parse (tied to the specific sentence context)
     or per-lemma (global)? The modal captures the sentence but the data
     model isn't shown.
   - **Action:** Add corrections history panel accessible from the user avatar
     menu. Show submission status (pending / accepted / rejected) as a badge
     or inline in the parse results where the user flagged it.

3. **Leverage page — deck filter + comprehension projection.** The leverage
   page (view-leverage.jsx, v2-leverage.jsx) shows words ranked across all
   decks with "+14% comprehension gain" but no baseline. Missing:
   - **Deck filter / selector** — let the user pick a single deck (or "all
     decks") and see leverage ranked within that scope. view-leverage.jsx has
     a scope filter (All/Active/Finnish/Estonian) but not per-deck.
   - **Now → target projection** (same as view-deck.jsx has): "You know 62%
     of *Kalevala ch.1*. Learn these 15 words → 78%."
   - **Goal threshold slider** — user sets target % (default 85%), list shows
     exactly how many words to learn. The word list below updates live.
   - **"Create study deck" CTA** — bundles the leverage words into a new FSRS
     deck (see P0 item 1 above).

### P1 — Design system convergence

4. **Unify wireframe and Aalto token systems.** Two parallel CSS architectures
   (wireframe OKLCH + Aalto hex/OKLCH hybrid) that don't share code. Pick one
   and retire the other. Recommendation: wireframe OKLCH tokens with Aalto
   role names. See "Convergence plan" section above.

5. **Unify FI/ET colors.** Three different FI/ET color schemes across files:
   - Branding: hex flag blues (#003580 / #0072CE)
   - Aalto: collapsed into one `--blue`, `--et` maps to `--ink` (wrong)
   - Wireframe: OKLCH hue-differentiated (cyan FI / magenta ET)
   Recommendation: wireframe approach. Works in both themes, clearly distinct.

6. **Commit to the superscript lockup in the app topbar.** The `Finn^fi Est^et`
   mark is the strongest brand element. The branding guide section 04 shows it
   in the topbar at 26px. All app shells currently show `finnest.` in italic
   serif instead.

7. **Pick one type stack.** Aalto uses Newsreader + Inter Tight + IBM Plex
   Mono. Wireframe uses Fraunces + IBM Plex Sans + IBM Plex Mono.
   Recommendation: Fraunces + IBM Plex (optical sizing, weight axes, more
   cohesive mono pairing).

8. **Unify responsive strategy.** The wireframe-clickthrough has `@media`
   breakpoints at 720px. The Aalto prototype has none. The mobile prototype
   (`finnest mobile.html` + `m-*.jsx`) is a third, separate codebase.
   Recommendation: use the wireframe's breakpoints as canonical; retire the
   separate mobile prototype.

### P2 — Component gaps

9. **"✎ Wrong?" button should be native to view-review.jsx.** Currently only
   exists as a DOM injection in proto-screens.jsx via setInterval polling.
   Build it into the review component properly.

10. **Review flow in wireframe.** Screen 11 (Review) only shows a static card.
    The Aalto prototype's FSRS-driven review loop with scope selection,
    progress bar, and ReviewDoneCard is much further along. Port it.

11. **Mobile prototype coverage.** Only 3 of 11+ screens (Landing, Wordlist,
    Review). Missing: Decks/Library, Deck Detail, Leverage, Settings, Cold
    Start, Sign-in. If staying with the responsive-breakpoint strategy (P1
    item 8), these are covered by the desktop views collapsing.

### P3 — Polish / brand guide gaps

12. **Branding: `.lockup .book` styled but never shown.** Either add artboard
    or remove dead CSS.

13. **Branding: Fraunces variant defined but never used.** Either add a
    comparison card or drop `.lockup.fraunces`.

14. **Branding: "Sanatorium" / "Paimio" naming unexplained.** Add a one-liner
    linking to Aalto's Paimio Sanatorium.

15. **Branding: No favicon artboard.** Prove the two-tone tile works at
    16x16 / 32x32.

16. **Branding: Superscript spacing fragile.** `margin-left: 0.55em` on `.est`
    is a magic number. Document or calculate.

17. **Branding: No clear-space rules or don'ts section.**

18. **Aalto: `--red-acc` defined but never used.** Assign to error states /
    destructive actions or cut it.

19. **Aalto: `body { overflow: hidden }`.** Verify on shorter screens (768px).

20. **Aalto: `.form-pill.more` dashed border reads as drag target.** Use a `+`
    icon in a solid ghost pill instead.

21. **Aalto: Savoy-vase SVG styled but path not in file.** Ensure silhouette
    is abstract enough.

22. **Wireframe: Flowstrip numbering starts at 0.** Start at 1 or drop
    numbers.

23. **Wireframe: No dark/light toggle.** Add PAPER token set for parity.

24. **Wireframe: Comprehension projection uses hardcoded tiers** (20/50/100).
    Should be data-driven from the actual leverage ranking.

25. **Wireframe: `data-go` routing is fragile.** Fine for throwaway
    clickthrough; any further investment should use hash routes or React
    Router.
