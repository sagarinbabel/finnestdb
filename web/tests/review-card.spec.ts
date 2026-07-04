import { expect, test, type Page } from '@playwright/test';

// Review card: centered layout + richer example-sentence rendering.
//
// Two things landed together here:
//  - the review column is now a centered, comfortably-narrow card instead of
//    a full-width panel (CSS-only, web/styles.css `.review-main`);
//  - a card whose deck occurrence carries a real corpus sentence now shows
//    that sentence with the target inflected form visually emphasized
//    (the same `.highlight-form` token used in parse-results examples),
//    plus a tightened lemma/POS/gloss/deck-tag hierarchy below it.
//
// The no-sentence "compact word card" path (PR #272) is pinned separately in
// parse-results.spec.ts ("review card with no source sentence renders as a
// plain word card") and is not touched here.

async function mockMe(page: Page): Promise<void> {
  await page.route('**/api/me', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        authenticated: true,
        user: { id: 1, email: 'alice@example.com', is_admin: false },
        dashboard: { known_count: 0, due_count: 0, new_capacity_today: 1, decks: [] },
      }),
    });
  });
}

const sentenceCard = {
  card_id: '42',
  mode: 'sentence',
  deck_counts: [['Top 1000 Finnish words', '1']],
  front: { type: 'sentence', text: 'Kissa oli pöydän alla.', highlight: 'oli' },
  back: {
    surface: 'oli',
    lemma: 'olla',
    pos: 'VERB',
    lang: 'FI',
    meaning: 'to be',
    grammar: '',
    examples: [{ text: 'Kissa oli pöydän alla.', source_deck: 'Top 1000 Finnish words' }],
  },
};

test('review card highlights the occurrence form inside the example sentence', async ({ page }) => {
  await mockMe(page);
  await page.route('**/api/review/next**', async (route) => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(sentenceCard) });
  });

  await page.goto('/#/review');
  await expect(page.locator('#review-page')).toHaveClass(/active/);
  await expect(page.locator('#review-card')).not.toHaveClass(/hidden/);

  const example = page.locator('#review-card-example');
  await expect(example).not.toHaveClass(/hidden/);
  await expect(example).toContainText('Kissa oli pöydän alla.');

  // The occurrence form ("oli") is wrapped in the shared highlight token, not
  // just present as plain text alongside the rest of the sentence.
  const highlighted = example.locator('.highlight-form');
  await expect(highlighted).toHaveText('oli');

  // Supporting metadata renders below the sentence: lemma (differs from
  // surface "oli"), POS, gloss, deck tag.
  await expect(page.locator('#review-card-lemma')).toContainText('olla');
  await expect(page.locator('#review-card-pos')).toContainText('Verb');
  await expect(page.locator('#review-card-meaning')).toContainText('to be');
  await expect(page.locator('#review-card-decks')).toContainText('Top 1000 Finnish words');
});

test('review card hides the POS badge when the card has none', async ({ page }) => {
  await mockMe(page);
  const noPos = { ...sentenceCard, back: { ...sentenceCard.back, pos: undefined } };
  await page.route('**/api/review/next**', async (route) => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(noPos) });
  });

  await page.goto('/#/review');
  await expect(page.locator('#review-card')).not.toHaveClass(/hidden/);
  await expect(page.locator('#review-card-pos')).toHaveClass(/hidden/);
});

test('review column is centered with a comfortable max-width on desktop', async ({ page }) => {
  await page.setViewportSize({ width: 1400, height: 900 });
  await mockMe(page);
  await page.route('**/api/review/next**', async (route) => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(sentenceCard) });
  });

  await page.goto('/#/review');
  await expect(page.locator('#review-card')).not.toHaveClass(/hidden/);

  const box = await page.locator('.review-main').boundingBox();
  expect(box).not.toBeNull();
  if (!box) throw new Error('unreachable');

  // Centered: roughly equal space on both sides of a wide 1400px viewport.
  const leftGap = box.x;
  const rightGap = 1400 - (box.x + box.width);
  expect(Math.abs(leftGap - rightGap)).toBeLessThan(2);

  // Comfortably narrow, not a full-width panel.
  expect(box.width).toBeLessThanOrEqual(720);
  expect(box.width).toBeGreaterThan(400);
});

test('review column fills the viewport unchanged at mobile widths', async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 812 });
  await mockMe(page);
  await page.route('**/api/review/next**', async (route) => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(sentenceCard) });
  });

  await page.goto('/#/review');
  await expect(page.locator('#review-card')).not.toHaveClass(/hidden/);

  const box = await page.locator('.review-main').boundingBox();
  expect(box).not.toBeNull();
  if (!box) throw new Error('unreachable');

  // At 375px the centered max-width (700px) is a no-op: the column still
  // fills the viewport, same as before this change.
  expect(box.width).toBeGreaterThan(340);
});
