import { expect, test, type Page } from '@playwright/test';

type Role = 'anon' | 'user' | 'admin';

async function mockMe(page: Page, role: Role): Promise<void> {
  await page.route('**/api/me', async (route) => {
    if (role === 'anon') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ authenticated: false, user: null }),
      });
      return;
    }
    const user = {
      id: 1,
      email: role === 'admin' ? 'admin@example.com' : 'alice@example.com',
      is_admin: role === 'admin',
    };
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        authenticated: true,
        user,
        dashboard: { known_count: 0, due_count: 0, new_capacity_today: 0, decks: [] },
      }),
    });
  });
}

async function mockParseWithId(page: Page, parseId: number): Promise<void> {
  await page.route('**/api/parse', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        lang: 'FI',
        parse_id: parseId,
        total_tokens: 6,
        parse_duration_ms: 12,
        words: [
          { lemma: 'laulaa', pos: 'VERB', forms: ['lauloi'], count: 1, grammar_label: 'past 3sg' },
          { lemma: 'pankki', pos: 'NOUN', forms: ['pankkiin'], count: 1, grammar_label: 'illative sg' },
        ],
      }),
    });
  });
}

test('PR88: reopened deck detail hides Suggest fix when no parse_id', async ({ page }) => {
  await mockMe(page, 'user');

  await page.route('**/api/decks/1', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        id: 1,
        title: 'My deck',
        lang: 'FI',
        created_at: '2026-05-06T00:00:00Z',
        total_tokens: 2,
        words: [{ lemma: 'pankki', pos: 'NOUN', forms: ['pankkiin'], count: 1, grammar_label: 'illative sg' }],
      }),
    });
  });

  await page.goto('/#/decks/1');
  await expect(page.locator('#results-page')).toHaveClass(/active/);

  // Deck detail reuses results UI but should NOT expose the correction flow
  // because reopened decks don't have a parse session attached.
  await expect(page.locator('.correction-btn')).toHaveCount(0);
  await expect(page.locator('#correction-modal')).toHaveClass(/hidden/);
});

test('PR88: deck load clears stale results so nothing is clickable', async ({ page }) => {
  await mockMe(page, 'user');
  await mockParseWithId(page, 2323);

  // Capture lemma-state mutations to prove stale row clicks can't fire while the deck loads.
  const captured: Array<any> = [];
  await page.route('**/api/lemma-state', async (route) => {
    captured.push(route.request().postDataJSON());
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ status: route.request().postDataJSON().status }),
    });
  });

  // Start on a results page with known rows.
  await page.goto('/#/inspect');
  await page.locator('#inspect-text').fill('Viisutubettaja lauloi nopeasti. Menin pankkiin eilen.');
  await page.getByRole('button', { name: 'Inspect text' }).click();
  await expect(page.locator('#results-page')).toHaveClass(/active/);
  await expect(page.locator('#word-table-body')).toContainText('laulaa');

  // Now navigate to a deck detail route, but delay the deck response.
  await page.route('**/api/decks/1', async (route) => {
    await new Promise((r) => setTimeout(r, 1200));
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        id: 1,
        title: 'Delayed deck',
        lang: 'FI',
        created_at: '2026-05-06T00:00:00Z',
        total_tokens: 1,
        words: [{ lemma: 'omena', pos: 'NOUN', forms: ['omena'], count: 1 }],
      }),
    });
  });

  await page.goto('/#/decks/1');

  // During the fetch, the title swaps and prior table content is cleared.
  await expect(page.locator('#results-title')).toHaveText('Loading deck…');
  await expect(page.locator('#word-table-body')).not.toContainText('laulaa');
  await expect(page.getByRole('button', { name: 'Ignore' })).toHaveCount(0);
  expect(captured.length).toBe(0);

  // Once the deck arrives, the table swaps to the deck data.
  await expect(page.locator('#word-table-body')).toContainText('omena');
});

