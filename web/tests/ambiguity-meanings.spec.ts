import { expect, test, type Page } from '@playwright/test';

// Multiple possible meanings flow (ambiguous surfaces). The parse mock returns
// the `kuusi` homograph with two senses - NUM "six" (dict) and NOUN "spruce"
// (FST-only) - exactly the shape /api/parse produces for a signed-in learner.

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

const kuusiSentence = 'Pihalla kasvaa kuusi.';

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
          {
            lemma: 'kuusi',
            pos: 'NUM',
            forms: ['kuusi'],
            count: 1,
            gloss: 'six',
            example_sentence: kuusiSentence,
          },
          { lemma: 'kasvaa', pos: 'VERB', forms: ['kasvaa'], count: 1, gloss: 'to grow' },
        ],
        ambiguous_surfaces: [
          {
            surface: 'kuusi',
            example: kuusiSentence,
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

async function parseKuusi(page: Page): Promise<void> {
  await page.goto('/#/inspect');
  await page.locator('#inspect-text').fill(kuusiSentence);
  await page.getByRole('button', { name: 'Parse text' }).click();
  await expect(page.locator('#results-page')).toHaveClass(/active/);
  // The ambiguity chip lives in the lemma table (Words tab); the results page
  // now defaults to the Read tab. Switch to Words for the chip-based specs.
  const wordsTab = page.locator('#results-tab-words');
  if (await wordsTab.isVisible()) await wordsTab.click();
}

test('ambiguous row shows the chip, expands to candidates and sentence context', async ({ page }) => {
  await mockUser(page);
  await mockKuusiParse(page);
  await parseKuusi(page);

  const chip = page.locator('.ambiguity-chip').first();
  await expect(chip).toContainText('Multiple possible meanings');

  // Collapsed by default - no panel yet.
  await expect(page.locator('.ambiguity-panel')).toHaveCount(0);

  await chip.click();
  const panel = page.locator('.ambiguity-panel');
  await expect(panel).toBeVisible();
  await expect(panel).toContainText('Multiple possible meanings for');
  await expect(panel).toContainText('kuusi');
  await expect(panel).toContainText('six');
  await expect(panel).toContainText('spruce');
  // Sentence context is shown.
  await expect(panel.locator('.ambiguity-example')).toContainText('Pihalla kasvaa');
  // Two candidates, each with all three actions.
  await expect(panel.locator('.ambiguity-candidate')).toHaveCount(2);
  await expect(panel.getByRole('button', { name: 'Study this meaning' }).first()).toBeVisible();
  await expect(panel.getByRole('button', { name: 'I know this meaning' }).first()).toBeVisible();
  await expect(panel.getByRole('button', { name: 'Not sure' }).first()).toBeVisible();
  // The exact grill-approved helper copy.
  await expect(panel).toContainText('Creates a review card when you save.');
});

test('study-this selects the FST sense and the deck save sends it in selected_senses', async ({ page }) => {
  await mockUser(page);
  await mockKuusiParse(page);
  let deckBody: any = null;
  await page.route('**/api/decks', async (route) => {
    if (route.request().method() !== 'POST') { await route.continue(); return; }
    deckBody = route.request().postDataJSON();
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ deck_id: 7 }) });
  });

  await parseKuusi(page);
  await page.locator('.ambiguity-chip').first().click();

  // Select the FST-only NOUN sense (spruce) to study.
  const spruce = page.locator('.ambiguity-candidate', { hasText: 'spruce' });
  await spruce.getByRole('button', { name: 'Study this meaning' }).click();
  await expect(spruce.getByRole('button', { name: 'Selected to study' })).toBeVisible();

  // Save as deck.
  await page.getByRole('button', { name: 'Save as deck' }).click();
  await page.locator('#results-deck-title').fill('Kuusi deck');
  await page.locator('#results-save-submit').click();

  await expect.poll(() => deckBody).not.toBeNull();
  expect(deckBody.selected_senses).toContainEqual({ surface: 'kuusi', lemma: 'kuusi', pos: 'NOUN' });
});

test('know-this records the sense known and excludes it from study selection', async ({ page }) => {
  await mockUser(page);
  await mockKuusiParse(page);
  const lemmaStateCalls: any[] = [];
  await page.route('**/api/lemma-state', async (route) => {
    lemmaStateCalls.push(route.request().postDataJSON());
    await route.fulfill({ status: 200, contentType: 'application/json', body: '{"status":"known"}' });
  });
  await page.route('**/api/decks', async (route) => {
    if (route.request().method() !== 'POST') { await route.continue(); return; }
    (page as any)._deckBody = route.request().postDataJSON();
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ deck_id: 8 }) });
  });

  await parseKuusi(page);
  await page.locator('.ambiguity-chip').first().click();

  const six = page.locator('.ambiguity-candidate', { hasText: 'six' });
  await six.getByRole('button', { name: 'I know this meaning' }).click();

  // The lemma-state POST records the chosen (lemma, pos) known.
  await expect.poll(() => lemmaStateCalls).toContainEqual(
    expect.objectContaining({ lang: 'FI', lemma: 'kuusi', pos: 'NUM', status: 'known' }),
  );

  // The panel re-renders in place (still expanded) showing the sense as known.
  await expect(page.locator('.ambiguity-candidate', { hasText: 'six' })).toContainText(/You know this meaning/);
});

test('none-of-these opens the flag-only correction modal prefilled for the surface', async ({ page }) => {
  await mockUser(page);
  await mockKuusiParse(page);

  await parseKuusi(page);
  await page.locator('.ambiguity-chip').first().click();

  await page.getByRole('button', { name: 'None of these looks right' }).click();

  const modal = page.locator('#correction-modal');
  await expect(modal).not.toHaveClass(/hidden/);
  // It is parser feedback: flag-only path is selected and proposed fields hidden.
  await expect(page.locator('#correction-mode-flag')).toBeChecked();
  await expect(page.locator('#correction-proposed-fields')).toBeHidden();
  // The propose path is disabled - this is never a proposed correction.
  await expect(page.locator('#correction-mode-propose')).toBeDisabled();

  // Submitting sends a flag-only report for the kuusi surface.
  let captured: any = null;
  await page.route('**/api/parse/feedback', async (route) => {
    captured = route.request().postDataJSON();
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ feedback_id: 5, status: 'submitted' }) });
  });
  await page.locator('#correction-submit').click();
  await expect.poll(() => captured).not.toBeNull();
  expect(captured).toMatchObject({ flag_only: true, surface: 'kuusi' });
});
