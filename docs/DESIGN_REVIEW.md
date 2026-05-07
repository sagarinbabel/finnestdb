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

## Cross-cutting recommendation

The superscript lockup (`Finn^fi Est^et`) is the strongest design element in
the system. The branding guide section 04 shows it in the topbar at 26px — commit
to that in the app. The current app topbar just shows `finnest.` in italic serif,
which wastes the brand's best asset. The superscripts at topbar size are small
but legible, and they make every page feel like the product is demonstrating
what it does.
