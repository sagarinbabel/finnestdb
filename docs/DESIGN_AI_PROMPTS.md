# Design AI prompts for FinnEst

_Created 2026-05-07. Templates for handing FinnEst screens to design
AIs (v0.dev, Lovable, Bolt, Cursor's design canvas, Figma Make, etc.)
and getting back UI that lands inside our existing CSS, doesn't
re-invent the type/colour system, and respects our actual user flows._

The current design source of truth is:

- [`web/index.html`](../web/index.html) — the live shell, all real
  pages (anonymous landing, sign-in, dashboard, inspect, decks,
  review, admin surfaces), and the correction modal.
- [`web/styles.css`](../web/styles.css) — design tokens and component
  styles. **Heads-up:** `web/styles.css` defines an early hex-based
  `:root` block at the top, then later (line ~2339) overrides those
  same tokens with the **Design v2 OKLCH** values for both default
  and `[data-theme="light"]`. The OKLCH block wins. The token table
  below is the *effective* one — match that, not the early hex.
- [`mockup.html`](../mockup.html) — older, single-file design mockup
  kept around for reference but **not** the current visual direction.
- [`docs/USER_FLOWS.md`](USER_FLOWS.md) — screen-by-screen behaviour
  spec.
- [`design/`](../design/) — checked-in design directions: HTML
  prototypes (`finnest v2.html`, `flow-diagram.html`,
  `wireframe-clickthrough.html`, …) and JSX views (`v2-*.jsx`,
  `view-*.jsx`). The Design v2 OKLCH values in `web/styles.css` come
  from this direction.

When prompting a design AI, attach `web/index.html`, `web/styles.css`,
and `docs/USER_FLOWS.md` if the tool accepts file context. Otherwise,
paste the **System block** below verbatim before the per-screen
prompt.

## System block (paste first, every time)

```
You are designing screens for FinnEst (finne.st), a consumer
language-learning web app for Finnish and Estonian readers. Users
paste real text — articles, lyrics, book chapters — and the app
returns dictionary-backed lemmas, definitions, examples, token
counts, and review cards built from the words in their source.

DESIGN CONSTRAINTS, NON-NEGOTIABLE:

1. The app is a single responsive web app. Mobile is a first-class
   surface — every screen must work at 375 px wide and look
   intentional, not like a desktop layout that collapsed.

2. Light and dark themes both ship. Use CSS variables for every color.

3. Typography:
   - Body / UI: 'IBM Plex Sans', system-ui, -apple-system,
     BlinkMacSystemFont, sans-serif.
   - Display / hero / deck titles: 'Fraunces', serif (variable font,
     opsz 9..144, wght 400/600/800).
   - Code / form data / morphology labels: 'IBM Plex Mono', monospace.
   Do not introduce additional families.

4. Color tokens (CSS vars — DO NOT recolour). Default is the dark INK
   theme; `[data-theme="light"]` is the PAPER override. Values are
   OKLCH because the live tokens in `web/styles.css` are OKLCH from
   line ~2339; the hex block earlier in that file is overridden.
     --bg-primary       oklch(0.165 0.008 250) / oklch(0.97 0.005 85)  /* INK / PAPER */
     --bg-secondary     oklch(0.21 0.008 250)  / oklch(1 0 0)
     --bg-tertiary      oklch(0.25 0.010 250)  / oklch(0.92 0.005 85)
     --text-primary     oklch(0.96 0.005 90)   / oklch(0.18 0.008 250)
     --text-secondary   oklch(0.72 0.008 90)   / oklch(0.42 0.010 250)
     --border           oklch(0.30 0.010 250)  / oklch(0.82 0.008 85)
     --card-bg          oklch(0.165 0.008 250) / oklch(0.97 0.005 85)
     --accent           oklch(0.72 0.19 38)    / oklch(0.55 0.19 35)   /* vermillion */
     --accent-hover     oklch(0.80 0.14 85)    / oklch(0.62 0.14 80)
     --success          oklch(0.78 0.13 145)
     --warning          oklch(0.80 0.14 85)
     --danger           oklch(0.64 0.18 28)
     --shadow           rgba(0,0,0,0.35)       / rgba(25,23,19,0.10)
   Part-of-speech colour set (used in the results table chips):
     --pos-noun  #3b82f6   --pos-verb  #10b981
     --pos-adj   #8b5cf6   --pos-adv   #14b8a6
     --pos-pron  #f59e0b   --pos-other #6b7280

5. Spacing scale: 0.25 / 0.5 / 0.75 / 1 / 1.5 / 2 / 3 rem.
   Border radius: 4 px (inputs/buttons), 8 px (cards), full (pills).
   Shadows: only on cards and modals — never on flat surfaces.

6. Components already exist; reuse them. Do not redesign:
   - Top nav (`.global-nav`, `.nav-link`, `.nav-mobile-overlay`).
   - Buttons: `.btn` + `.btn-primary` / `.btn-outline` / `.btn-link`,
     sizes `.btn-sm` / `.btn-lg`, plus `.btn-block`.
   - Inputs: `.form-group` with stacked label + control. No floating
     labels.
   - Coverage gauge: `.coverage-gauge` with `.coverage-gauge-bar` and
     `.coverage-gauge-fill` (semantic class `high` / `medium` / `low`
     — `medium`, not `mid`; the runtime in `web/app.ts` uses
     `medium`).
   - Word table: `.word-table`, sortable headers `.sort-btn`, POS
     filter chips `.pos-filter-chip`, language pills `.results-pill`.
   - Modals: `.modal` + `.modal-card` + `.modal-actions` (see the
     correction modal in `web/index.html` for the exact shape).
   - Toasts: `#toast-container` is mounted globally; just call `toast()`.

7. Voice and tone: plainspoken, never gamified. The product is for
   adult learners who already read in their L1 and want to read in
   Finnish or Estonian. No streaks, no badges, no XP, no fire emojis.
   "Review" is the verb — never "Practice" or "Learn".

8. Accessibility: every interactive element keyboard-focusable;
   label every input; aria-live on result regions and toasts; honour
   prefers-reduced-motion (no flips, no parallax, no auto-rotating
   carousels).

9. NEVER use Tailwind utility classes in output. The codebase uses
   plain CSS with the variables above. If you must produce a React
   component for v0/Lovable, write `style={{}}` against CSS variables
   or a single `styles.module.css` file that uses `var(--accent)` etc.

10. NEVER fabricate Finnish or Estonian text in output. If a screen
    needs example text, use this fixed sample:

    Finnish:  "Aurinko paistaa, ja koirat juoksevat puistossa."
    Estonian: "Päike paistab ja koerad jooksevad pargis."

OUTPUT FORMAT:
- One artifact per screen, self-contained HTML or JSX.
- No external libraries beyond Google Fonts (already loaded in
  `web/index.html`).
- Include a comment block at the top citing which existing class
  selectors you reused.
```

## Per-screen prompts

Each block below is a **delta** to the system block. Paste the system
block, then the screen block, then any sample data the tool needs.

### Landing + inline parse (anonymous)

```
Design the anonymous landing for finne.st.

The page IS the parse tool. There is no separate "marketing landing"
above the fold — the value prop is "paste text, get vocabulary," so
the textarea is the hero.

Layout, top to bottom:
1. Sticky `.global-nav` with logo on the left and "Sign in" on the
   right (theme toggle next to it). On mobile, collapse to hamburger.
2. Hero block: an h1 in Fraunces ("Learn Finnish & Estonian by reading
   what you love."), a one-line subtitle, then immediately the parse
   form.
3. Parse form is a single card (background `var(--card-bg)`, 8px
   radius, shadow). Note `--card-bg` is a CSS variable, not a class —
   the actual card classes in the codebase are `.signin-card`,
   `.action-card`, and `.placeholder-card`; pattern-match on whichever
   is closest, or a plain `<section>` with the variable applied:
   - language toggle (Auto / FI / ET) as a segmented control
   - textarea with 10 rows, 1,500,000 char max
   - file picker accepting .txt, .md, .epub (drag-and-drop too); .apkg is a
     separate future import flow
   - LIVE stats strip below the textarea: "Detected: Finnish · 4,213
     chars · 612 tokens · 287 unique forms · 0 numbers". Updates
     debounced as the user types.
   - language-mismatch banner that appears when detection conflicts
     with the selector, with a one-click "Switch to Finnish" button.
   - Primary "Parse text →" button right-aligned.
4. Two sample-text buttons under the form: "Try a Finnish news
   headline" / "Try an Estonian children's poem" — clicking populates
   the textarea.
5. Footer with three links: How it works / Privacy / About.

Do NOT include:
- Marketing testimonials.
- A separate "features" grid (we have one on /about; not on /).
- A pricing section. There is no pricing.

Reference the existing inspect-form structure in
`web/index.html` lines 244–283. The new element vs. existing: the live
stats strip and the segmented language control with Auto.
```

### Results page (shared between anonymous and signed-in)

```
Redesign the results page so the same component serves anonymous and
signed-in users, with conditional UI for each.

Required regions, top to bottom:
1. Back button (top-left) and the deck title (or "Results" for an
   unsaved parse).
2. Coverage gauge — reuse `.coverage-gauge`. Width animates from 0 on
   mount.
3. Sign-up ribbon, ANONYMOUS ONLY: a single-row card with a sparkle
   icon, headline "Want to remember these words?", body "Sign up free
   to turn this list into a review deck — we'll teach you the words
   you don't know in 5 min/day", and two buttons (Create account /
   Later). Dismissable for the session.
4. Meta strip: "287 unique words · Custom parser · 342ms".
5. POS filter chips (`.pos-filter-chip`) — sticky on scroll on mobile.
6. Word table (`.word-table`) with columns:
   #  Form  Definition  Count  Status
   - Form column shows surface form, with a small POS chip in the
     POS color set.
   - Status column: for anonymous, a multi-select checkbox; for
     signed-in, a `[ Known ▾ ]` dropdown (Known / Studying /
     Ignored).
   - Each row has a hover-revealed ✎ "Wrong?" link to open the
     correction modal (see correction-flow prompt below). On touch
     devices the ✎ is always visible.
7. Bulk-action bar at the bottom of the table: "12 selected · [Export
   CSV] [Mark known]". Floats above the bottom edge on scroll.
8. Save-as-deck panel, SIGNED-IN ONLY: collapsed by default, expands
   to show the radio choice "A new deck / Add to existing deck" with
   the relevant inputs. See docs/USER_FLOWS.md §6 for the exact copy.

Sample data (use exactly):
Row 1:  olla  · VERB · be, exist · 34 occurrences
Row 2:  hän   · PRON · he, she   · 22 occurrences
Row 3:  koira · NOUN · dog       · 18 occurrences

Reference the existing `#results-page` block in `web/index.html`
lines 492–551. New elements: sign-up ribbon, ✎ link per row, bulk
selection bar, "Add to existing deck" radio inside the save panel.
```

### Correction modal (lighter version)

```
Redesign the correction modal in `web/index.html` lines 553–603 to
add a "flag-only" path. The current modal forces the user to provide
a corrected lemma and POS — losing signal from users who notice a
parse is wrong but don't know the right answer.

New shape:
- Heading: "Suggest a correction" (unchanged).
- Read-only block: surface form (mono), parser's current answer
  (lemma · POS · grammar label).
- Two radios:
  ◉ This is wrong (I don't know the right answer)
  ○ This is wrong, and the right answer is:
- Selecting the second radio reveals the existing fields (proposed
  lemma, POS, grammar label, notes).
- The proposed-lemma input pre-fills with the SURFACE FORM lowercased,
  not the current lemma — if the parser was wrong, its lemma is the
  wrong starting point.
- Footer: Cancel / Send.

Match the existing `.modal-card` styling. Don't change the modal's
focus-trap or escape-to-close behaviour.

For tablet/desktop, keep modal width 480 px. For mobile (<640 px),
full-screen the modal with a slide-up animation honouring
prefers-reduced-motion.
```

### Sign-in / Create account

```
Redesign the auth screen at `#/signin` to add Google OAuth alongside
email+password. Tabs at the top: "Sign in" / "Create account".

Top: a single Google button — full-width on mobile, max 360px on
desktop, "Continue with Google" with the standard G logo.

Below: a horizontal rule with "or with email" centred.

Below that: the form. For "Create account" mode show First name +
Email + Password (8+ chars). For "Sign in" mode show Email + Password.

Below the submit: legal copy "By creating an account you agree to our
Terms and Privacy. We don't sell your data and don't train external
models on it."

Reference the existing `#signin-page` block in `web/index.html` lines
170–194. Keep its email/password form intact; the new elements are the
Google button, the divider, and the First-name field.
```

### Dashboard (signed-in landing)

```
Redesign the dashboard at `#/dashboard`. The app is hash-routed —
generated links MUST use the hash form (`#/dashboard`, `#/inspect` for
parse, `#/decks`, `#/review`, `#/signin`, `#/`). Bare paths like
`/parse` will leave the SPA or 404.

Top section: greeting using first name, sub-line "Pick up where you
left off, or read something new."

Stats row: three stat cards in a responsive grid (3 cols desktop, 1
col mobile). Each card has a label (small caps, text-secondary) and
a value (very large, accent color). Stats: Words known (split
"FI / ET"), Due to review, New today (formatted "0 / 20" — N learned
of cap).

Action grid: three large clickable cards (`.action-card`) — Read a
new text / Review due words / Your decks. Each shows count or hint
inline ("0 due — start by parsing a text").

Recent decks: list of up to 5 most-recently-touched decks. Each row
shows title (Fraunces), language pill (FI/ET in --pos-noun/--pos-adj
colours), word count, "X% known" mini-bar, and a Review button.

EMPTY STATE (zero decks, zero parses): replace the recent-decks list
with a card titled "Just getting started?" listing three options:
"Paste a text" (link to `#/inspect`), "Import known words" (link to
`#/decks` known-words section), "Start with the top 1000 Finnish
words" (link to `#/decks/top-1000-fi` — note this route is **new**
and does not exist yet, gated on the seed-deck research project). The
third one is gated on a feature flag —
hide if the seed deck doesn't exist for the user's preferred lang.

Reference the existing `#dashboard-page` block in `web/index.html`
lines 196–241.
```

### Decks list

```
Redesign `#/decks` to surface deck-level coverage and a global "review
all" affordance.

Top: header "Your decks" + button "+ New from text" (links to
`#/inspect`).

Then a list of deck cards. Each card:
- Title (Fraunces, 1.5rem) + language pill on the right.
- Sub-row of three stats joined by middots: "842 words · 68% known ·
  +14 new ready".
- Right-side primary "Review" button.
- Triple-dot menu (`⋯`) on the far right with: Rename, Open detail,
  Export CSV, Delete.

After the list of decks, a single full-width card titled "Review all
due words across both languages" with the cross-deck due count and a
Review → button.

Below that, the "Known words" panel (collapsed accordion by default).
Reference the existing `.known-words-panel` in `web/index.html` lines
300–327 — keep its language selector, import textarea, and unresolved
list. Just present it under a heading instead of the current
always-expanded layout.
```

### Deck detail (with comprehension prediction)

```
Design `#/decks/:id`. Reuse the results-page word table at the bottom;
the top of the page is new.

Top band:
- Deck title (Fraunces) and `⋯` menu (Rename / Export CSV / Delete).
- Sub-line: "842 unique words · 68% token-weighted coverage".
- Coverage gauge full width.
- Below the gauge, three-line "comprehension projection":
    Learn the next 20 words → 81%
    Learn the next 50 words → 89%
- Two buttons: primary "Review this deck", secondary "Add 20 to
  today's queue".

Then the word table, with the deck-specific Known/Studying/Ignored
status per row.

The comprehension projection numbers come from
`GET /api/decks/:id/comprehension` (already on the TODO list).
Render placeholder values "—" while the request is in flight; do not
animate from 0%.
```

### Review session

```
Refine `#/review`. The screen already exists at
`web/index.html` lines 331–376.

Changes:
1. The deck filter at the top should default to "All decks · Finnish"
   or "All decks · Estonian" (auto-picked based on which language has
   more due cards). User can switch language; "Mixed languages" is an
   option but never the default.
2. The four FSRS rating buttons (Again / Hard / Good / Easy) should
   be a single horizontal row on desktop and a 2x2 grid on mobile.
   Color: outlined for all four; primary fill ONLY on Good. Keyboard
   shortcuts: 1 / 2 / 3 / 4.
3. A small "✎ Wrong?" link below the meaning on the back of the card,
   opening the same correction modal as the results table.
4. After the four ratings, a row of three small `.btn-link`s:
   "Mark known", "Ignore word", "Load another". Same as today.

Sample card front: a sentence "Aurinko paistaa, ja [koirat] juoksevat
puistossa." with the target form highlighted. Card back: lemma
"koira", POS "NOUN", meaning "dog", grammar "nominative plural".
```

## Tool-specific notes

### v0.dev

v0 likes Tailwind by default — disable it for these prompts. In v0:
"Generate without Tailwind, plain CSS modules referencing CSS vars."
Drop the System block in chat first; it stays in scope across
follow-ups.

### Lovable

Lovable is good at flow/state but tends to over-add features. Prefix
each per-screen prompt with: "Implement EXACTLY the elements listed
below. Do not add a hero image, testimonials, FAQ, or pricing."

Lovable's React output uses inline styles freely — that's fine; we'll
extract to `web/styles.css` on review.

### Bolt.new

Bolt scaffolds whole apps. Tell it: "Treat
`web/index.html` as the routing root. Add new routes as new
`<div id="…-page" class="page">` blocks inside that file, NOT as
separate React Router routes." This keeps it inside the existing
hash-router structure (`#/`, `#/dashboard`, `#/decks` …).

### Cursor / Claude design canvas

Most reliable. Attach `web/index.html`, `web/styles.css`, and
`docs/USER_FLOWS.md` as context, paste the System block, then per-screen
block. Cursor will produce a full HTML diff that drops into the existing
file.

### Figma Make / Figma AI

Skip the System block; Figma can't read CSS vars. Instead, set up a
Figma file with the colour and type styles by hand once (or import
from `web/styles.css` via a token plugin), then paste only the
per-screen prompt into Figma Make.

## Anti-patterns to call out in any prompt

- Do not generate Tailwind. Period.
- Do not invent a sidebar navigation. The app uses a top nav only.
- Do not add a "tour" or "onboarding modal" that interrupts on first
  load. Empty states do that work.
- Do not animate page transitions. The app uses hash routing with
  instant page swaps.
- Do not put the language toggle inside a settings menu. It belongs
  on the parse form, every time.
- Do not gamify (streaks, levels, badges, hearts, gems, fire emojis).
- Do not invent a brand voice. Plainspoken, no marketing breathlessness.

## Verifying generated UI

Before merging anything from a design AI:

1. Drop the HTML/JSX into `web/` (or a sandbox file) and load it.
2. Resize to 375 px and check every screen renders without horizontal
   scroll.
3. Toggle dark mode and check every colour comes from CSS vars.
4. Tab through the page — every interactive element must focus
   visibly.
5. Run the Playwright suite that covers landing, sign-in, parse,
   deck save, and review:

   ```sh
   cd web && npx playwright test
   ```

   `web/package.json` doesn't define a `test` script — run Playwright
   directly via the local install. The suite in `web/tests/` boots
   the Go server itself (see `web/playwright.config.ts`) so make sure
   port 8081 is free.
