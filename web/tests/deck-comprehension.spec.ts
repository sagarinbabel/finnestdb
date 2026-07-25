import { expect, test, type Page } from '@playwright/test';

// Guards the deck comprehension surfaces:
//   1. The deck list meta line shows the headline "N% comprehension" when the
//      backend supplies comprehension_pct (and omits it for empty decks).
//   2. The deck detail view renders the projection panel from
//      GET /api/decks/:id/comprehension - headline percentage plus the
//      "learn these next" before → after expansion.
//
// API calls are mocked so the tests don't depend on the live Go server.

async function mockMeWithDeck(page: Page, comprehensionPct: number | null): Promise<void> {
  await page.route('**/api/me', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        authenticated: true,
        user: { id: 2, email: 'alice@example.com', is_admin: false },
        dashboard: {
          known_count:        1,
          due_count:          0,
          new_capacity_today: 0,
          decks: [{
            id: 5, title: 'Witcher 1', lang: 'FI',
            known: 1, unique: 2, due: 0, is_public: false,
            ...(comprehensionPct === null ? {} : { comprehension_pct: comprehensionPct }),
          }],
        },
      }),
    });
  });
}

test('deck list meta shows the comprehension headline when present', async ({ page }) => {
  await mockMeWithDeck(page, 62.5);
  await page.goto('/#/decks');
  const meta = page.locator('.deck-list-meta');
  await expect(meta).toContainText('62.5% comprehension');
});

test('deck list meta omits comprehension for decks without token data', async ({ page }) => {
  await mockMeWithDeck(page, null);
  await page.goto('/#/decks');
  const meta = page.locator('.deck-list-meta');
  await expect(meta).toContainText('1/2 known');
  await expect(meta).not.toContainText('comprehension');
});

test('deck detail renders the comprehension projection panel', async ({ page }) => {
  await mockMeWithDeck(page, 62.5);
  await page.route('**/api/decks/5/comprehension', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        coverage_pct: 62.5,
        total_tokens: 8,
        known_tokens: 5,
        top_unlocks: [
          { lemma: 'nähdä', pos: 'VERB', token_count: 2, gain_pct: 25 },
          { lemma: 'sinä', pos: 'PRON', token_count: 1, gain_pct: 12.5 },
        ],
      }),
    });
  });
  await page.route('**/api/decks/5', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        id: 5, title: 'Witcher 1', lang: 'FI', parser: 'custom',
        parse_id: null, total_tokens: 8, created_at: '2026-07-01T00:00:00Z',
        words: [
          { lemma: 'kissa', pos: 'NOUN', forms: ['Kissa'], count: 1 },
          { lemma: 'nähdä', pos: 'VERB', forms: ['näkee'], count: 2 },
        ],
      }),
    });
  });

  await page.goto('/#/decks/5');
  const panel = page.locator('#deck-comprehension');
  await expect(panel).toBeVisible();
  await expect(panel.locator('.deck-comprehension-headline')).toContainText('62.5%');
  await expect(panel.locator('.deck-comprehension-headline')).toContainText('5 of 8 words');

  // Projection: 62.5 + 25 + 12.5 = 100% after learning both unlocks.
  const unlocks = panel.locator('.deck-unlocks summary');
  await expect(unlocks).toContainText('Learn these 2 words to reach ~100% comprehension');
  await unlocks.click();
  await expect(panel.locator('.deck-unlock-item')).toHaveCount(2);
  await expect(panel.locator('.deck-unlock-item').first()).toContainText('nähdä');
  await expect(panel.locator('.deck-unlock-item').first()).toContainText('+25%');
});
