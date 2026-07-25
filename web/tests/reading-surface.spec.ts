import { expect, test, type Page } from '@playwright/test';

// Reading surface (Read / Words tabs, the living text). The Read view tokenizes
// the source text client-side and colors each parsed word by its learner state;
// tapping a word opens a popover with Known / Study / Ignore (or the Multiple-
// possible-meanings candidate list for a homograph). These specs mock /api/parse
// so they run deterministically without the FFI parser.

const readText = 'Kissa juoksee pihalla. Koira nukkuu.';

async function mockUser(page: Page): Promise<void> {
  await page.route('**/api/me', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        authenticated: true,
        user: { id: 1, email: 'alice@example.com', is_admin: false },
        dashboard: { known_count: 0, due_count: 0, new_capacity_today: 5, decks: [] },
        languages: { learning: ['FI', 'ET'], active: 'FI', stats: { FI: { decks: 0, known_words: 0 }, ET: { decks: 0, known_words: 0 } } },
      }),
    });
  });
}

async function mockAnon(page: Page): Promise<void> {
  await page.route('**/api/me', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ authenticated: false, user: null, anon_max_chars: 20000 }),
    });
  });
}

// A parse where "Kissa" is already known and "koira" is not, so the Read view
// has a mix of known/new coloring to assert on. Forms carry the exact source
// surfaces (case-preserved) so the client can match them back.
async function mockParse(page: Page): Promise<void> {
  await page.route('**/api/parse', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        lang: 'FI',
        total_tokens: 5,
        parse_duration_ms: 6,
        words: [
          { lemma: 'kissa', pos: 'NOUN', forms: ['Kissa'], count: 1, gloss: 'cat', learning_state: 'known' },
          { lemma: 'juosta', pos: 'VERB', forms: ['juoksee'], count: 1, gloss: 'to run' },
          { lemma: 'piha', pos: 'NOUN', forms: ['pihalla'], count: 1, gloss: 'yard' },
          { lemma: 'koira', pos: 'NOUN', forms: ['Koira'], count: 1, gloss: 'dog' },
          { lemma: 'nukkua', pos: 'VERB', forms: ['nukkuu'], count: 1, gloss: 'to sleep' },
        ],
      }),
    });
  });
}

async function parse(page: Page, text = readText): Promise<void> {
  await page.goto('/#/inspect');
  await page.locator('#inspect-text').fill(text);
  await page.getByRole('button', { name: 'Parse text' }).click();
  await expect(page.locator('#results-page')).toHaveClass(/active/);
}

// ── Tabs: default + memory ─────────────────────────────────────────────────

test('results open on the Read tab by default; the living text renders', async ({ page }) => {
  await mockUser(page);
  await mockParse(page);
  await parse(page);

  await expect(page.locator('#read-view')).toBeVisible();
  await expect(page.locator('#results-tab-read')).toHaveAttribute('aria-selected', 'true');
  await expect(page.locator('#words-view')).toBeHidden();
  // Every parsed surface is a tappable span; unparsed punctuation is plain text.
  await expect(page.locator('.read-token', { hasText: 'Kissa' })).toBeVisible();
  await expect(page.locator('.read-token', { hasText: 'juoksee' })).toBeVisible();
});

test('the last-used tab is remembered across parses via localStorage', async ({ page }) => {
  await mockUser(page);
  await mockParse(page);
  await parse(page);

  // Switch to Words.
  await page.locator('#results-tab-words').click();
  await expect(page.locator('#words-view')).toBeVisible();
  await expect(page.locator('#read-view')).toBeHidden();

  // A fresh parse (same session) lands on Words, honoring the remembered choice.
  await page.locator('#results-back').click();
  await parse(page);
  await expect(page.locator('#words-view')).toBeVisible();
  await expect(page.locator('#results-tab-words')).toHaveAttribute('aria-selected', 'true');
});

// ── Live token coloring ────────────────────────────────────────────────────

test('token coloring reflects the known state hydrated from the parse', async ({ page }) => {
  await mockUser(page);
  await mockParse(page);
  await parse(page);

  // "Kissa" came back learning_state=known → is-known; "Koira" is new.
  await expect(page.locator('.read-token', { hasText: 'Kissa' })).toHaveClass(/is-known/);
  await expect(page.locator('.read-token', { hasText: 'Koira' })).toHaveClass(/is-new/);
});

test('marking a word ignored from the popover updates its color live to neutral', async ({ page }) => {
  await mockUser(page);
  await mockParse(page);
  await page.route('**/api/lemma-state', async (route) => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: '{"status":"ignored"}' });
  });
  await parse(page);

  const koira = page.locator('.read-token', { hasText: 'Koira' });
  await expect(koira).toHaveClass(/is-new/);
  await koira.click();

  const pop = page.locator('#read-popover');
  await expect(pop).toBeVisible();
  await pop.getByRole('button', { name: 'Ignore' }).click();

  // Ignored → neutral (uncolored), live, without a re-parse.
  await expect(page.locator('.read-token', { hasText: 'Koira' })).toHaveClass(/is-neutral/);
});

// ── Popover open / action / close ──────────────────────────────────────────

test('the popover shows lemma, POS, gloss and closes on ESC', async ({ page }) => {
  await mockUser(page);
  await mockParse(page);
  await parse(page);

  await page.locator('.read-token', { hasText: 'juoksee' }).click();
  const pop = page.locator('#read-popover');
  await expect(pop).toBeVisible();
  await expect(pop).toContainText('juoksee');
  await expect(pop).toContainText('juosta');
  await expect(pop).toContainText('to run');
  await expect(pop.getByRole('button', { name: 'Mark known' })).toBeVisible();
  await expect(pop.getByRole('button', { name: 'Study' })).toBeVisible();
  await expect(pop.getByRole('button', { name: 'Ignore' })).toBeVisible();

  await page.keyboard.press('Escape');
  await expect(pop).toBeHidden();
});

test('Study in an unsaved parse marks the pending sense and sends it on save', async ({ page }) => {
  await mockUser(page);
  await mockParse(page);
  let deckBody: any = null;
  await page.route('**/api/decks', async (route) => {
    if (route.request().method() !== 'POST') { await route.continue(); return; }
    deckBody = route.request().postDataJSON();
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ deck_id: 3 }) });
  });
  await parse(page);

  await page.locator('.read-token', { hasText: 'juoksee' }).click();
  const pop = page.locator('#read-popover');
  // The chip's helper copy is reused verbatim.
  await expect(pop).toContainText('Creates a review card when you save.');
  await pop.getByRole('button', { name: 'Study' }).click();
  await expect(pop.getByRole('button', { name: 'Studying' })).toBeVisible();
  // The word is now colored as learning in the text.
  await expect(page.locator('.read-token', { hasText: 'juoksee' })).toHaveClass(/is-learning/);
  await page.keyboard.press('Escape');

  // Save from the shared CTA (above the tabs) and confirm the sense rides along.
  await page.getByRole('button', { name: 'Save as deck' }).click();
  await page.locator('#results-deck-title').fill('Read deck');
  await page.locator('#results-save-submit').click();
  await expect.poll(() => deckBody).not.toBeNull();
  expect(deckBody.selected_senses).toContainEqual({ surface: 'juoksee', lemma: 'juosta', pos: 'VERB' });
});

test('tapping outside the popover closes it', async ({ page }) => {
  await mockUser(page);
  await mockParse(page);
  await parse(page);

  await page.locator('.read-token', { hasText: 'juoksee' }).click();
  await expect(page.locator('#read-popover')).toBeVisible();
  await page.locator('#read-popover-backdrop').click();
  await expect(page.locator('#read-popover')).toBeHidden();
});

// ── Ambiguous surface in the popover ───────────────────────────────────────

const kuusiText = 'Pihalla kasvaa kuusi.';
async function mockKuusiParse(page: Page): Promise<void> {
  await page.route('**/api/parse', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        lang: 'FI',
        total_tokens: 3,
        parse_duration_ms: 8,
        words: [
          { lemma: 'kuusi', pos: 'NUM', forms: ['kuusi'], count: 1, gloss: 'six', example_sentence: kuusiText },
          { lemma: 'kasvaa', pos: 'VERB', forms: ['kasvaa'], count: 1, gloss: 'to grow' },
          { lemma: 'piha', pos: 'NOUN', forms: ['Pihalla'], count: 1, gloss: 'yard' },
        ],
        ambiguous_surfaces: [
          {
            surface: 'kuusi',
            example: kuusiText,
            candidates: [
              { lemma: 'kuusi', pos: 'NUM', gloss: 'six', source: 'dict' },
              { lemma: 'kuusi', pos: 'NOUN', gloss: 'spruce', source: 'fst' },
            ],
          },
        ],
      }),
    });
  });
}

test('an ambiguous word opens the Multiple-possible-meanings panel in the popover', async ({ page }) => {
  await mockUser(page);
  await mockKuusiParse(page);
  await parse(page, kuusiText);

  const kuusi = page.locator('.read-token', { hasText: 'kuusi' });
  await expect(kuusi).toHaveClass(/is-ambiguous/);
  await kuusi.click();

  const panel = page.locator('#read-popover .ambiguity-panel');
  await expect(panel).toBeVisible();
  await expect(panel).toContainText('Multiple possible meanings for');
  await expect(panel).toContainText('six');
  await expect(panel).toContainText('spruce');
  await expect(panel.locator('.ambiguity-candidate')).toHaveCount(2);
});

test('None-of-these in the popover opens the flag-only correction path', async ({ page }) => {
  await mockUser(page);
  await mockKuusiParse(page);
  await parse(page, kuusiText);

  await page.locator('.read-token', { hasText: 'kuusi' }).click();
  await page.locator('#read-popover').getByRole('button', { name: 'None of these looks right' }).click();

  const modal = page.locator('#correction-modal');
  await expect(modal).not.toHaveClass(/hidden/);
  await expect(page.locator('#correction-mode-flag')).toBeChecked();
  await expect(page.locator('#correction-mode-propose')).toBeDisabled();
});

// ── Anonymous read-only popover ────────────────────────────────────────────

test('anonymous Read popover shows gloss only with a sign-in nudge, no actions', async ({ page }) => {
  await mockAnon(page);
  await mockParse(page); // learning_state is ignored for anon (no state axis)
  await page.goto('/');
  await page.locator('#landing-text').fill(readText);
  await page.locator('#landing-submit').click();
  await expect(page.locator('#results-page')).toHaveClass(/active/);

  // Neutral coloring - no learner state exists for anonymous parses.
  await expect(page.locator('.read-token', { hasText: 'Kissa' })).toHaveClass(/is-neutral/);

  await page.locator('.read-token', { hasText: 'juoksee' }).click();
  const pop = page.locator('#read-popover');
  await expect(pop).toContainText('to run');
  await expect(pop.getByRole('button', { name: 'Mark known' })).toHaveCount(0);
  await expect(pop.getByRole('button', { name: 'Study' })).toHaveCount(0);
  await expect(pop).toContainText('Create an account');
});

// ── 375px bottom sheet ─────────────────────────────────────────────────────

test('at 375px the popover renders as a bottom sheet with ≥44px tap targets', async ({ page }) => {
  await mockUser(page);
  await mockParse(page);
  await page.setViewportSize({ width: 375, height: 812 });
  await parse(page);

  await page.locator('.read-token', { hasText: 'juoksee' }).click();
  const pop = page.locator('#read-popover');
  await expect(pop).toBeVisible();

  // Bottom sheet: pinned to the viewport bottom, full width.
  const box = await pop.boundingBox();
  expect(box).not.toBeNull();
  expect(Math.round(box!.x)).toBe(0);
  expect(Math.round(box!.width)).toBe(375);
  expect(box!.y + box!.height).toBeGreaterThan(700);

  // Action buttons keep a ≥44px touch height.
  const btnBox = await pop.getByRole('button', { name: 'Mark known' }).boundingBox();
  expect(btnBox!.height).toBeGreaterThanOrEqual(44);
});

// ── Catalog → reader flow ──────────────────────────────────────────────────

const CATALOG_TEXT = 'Kissa istui ikkunalla ja katseli lintuja pihalla. Aurinko paistoi kirkkaasti.';

test('choosing a catalog text auto-parses and lands on the Read tab', async ({ page }) => {
  await page.route('**/api/me', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        authenticated: true,
        user: { id: 1, email: 'alice@example.com', is_admin: false },
        dashboard: { known_count: 0, due_count: 0, new_capacity_today: 5, decks: [] },
        languages: { learning: ['FI'], active: 'FI', stats: { FI: { decks: 0, known_words: 0 } } },
      }),
    });
  });
  await page.route('**/api/catalog', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        entries: [{
          id: 'fi-sample-story', language: 'fi', title: 'Kissa ikkunalla', genre: 'story',
          difficulty: 'easy', difficulty_review: 'pending', corpus_source: 'test',
          license: 'CC0', text_length: CATALOG_TEXT.length, word_count: 11,
        }],
        has_known_words: { fi: false },
      }),
    });
  });
  await page.route('**/api/catalog/fi-sample-story/text', async (route) => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ text: CATALOG_TEXT }) });
  });
  await page.route('**/api/parse', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        lang: 'FI',
        total_tokens: 11,
        parse_duration_ms: 9,
        words: [
          { lemma: 'kissa', pos: 'NOUN', forms: ['Kissa'], count: 1, gloss: 'cat' },
          { lemma: 'istua', pos: 'VERB', forms: ['istui'], count: 1, gloss: 'to sit' },
        ],
      }),
    });
  });

  await page.goto('/#/inspect');
  // Picking the text opens it like a book: it auto-parses (no intermediate Parse
  // click) and lands straight on the results page's Read tab.
  await page.locator('#inspect-catalog [data-catalog-id="fi-sample-story"]').click();
  await expect(page.locator('#results-page')).toHaveClass(/active/);
  // Lands in the reader: Read tab active, the catalog text rendered as the
  // living text (not a cramped box).
  await expect(page.locator('#results-tab-read')).toHaveAttribute('aria-selected', 'true');
  await expect(page.locator('#read-view')).toBeVisible();
  await expect(page.locator('#read-text')).toContainText('Kissa istui ikkunalla');
  await expect(page.locator('.read-token', { hasText: 'Kissa' })).toBeVisible();

  // The text is still in the Inspect textarea so re-parse / edit stays possible.
  await page.goto('/#/inspect');
  await expect(page.locator('#inspect-text')).toHaveValue(CATALOG_TEXT);
});
