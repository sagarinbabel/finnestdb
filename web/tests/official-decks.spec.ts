import { expect, test, type Page } from '@playwright/test';

// These tests guard the "publishing is admin-only" boundary on both
// frontend surfaces that expose it:
//   1. The save-as-deck form on /inspect (admin sees a "Publish as an
//      official deck" checkbox before saving).
//   2. The /decks "My Decks" tab (admin sees a Publish / Unpublish button
//      per owned deck; non-admins see only Rename / Delete).
//
// API calls are mocked so the tests don't depend on the live Go server.

const finnishText = 'Kissa juoksee.';

type Role = 'user' | 'admin';

interface DeckRow {
  id:         number;
  title:      string;
  lang:       string;
  known:      number;
  unique:     number;
  due:        number;
  is_public:  boolean;
  is_owner?:  boolean;
  subscribed?: boolean;
}

async function mockMeWithDecks(page: Page, role: Role, decks: DeckRow[] = []): Promise<void> {
  await page.route('**/api/me', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        authenticated: true,
        user: {
          id: role === 'admin' ? 1 : 2,
          email: role === 'admin' ? 'admin@example.com' : 'alice@example.com',
          is_admin: role === 'admin',
        },
        dashboard: {
          known_count:        0,
          due_count:          0,
          new_capacity_today: 0,
          decks,
        },
      }),
    });
  });
}

async function mockParse(page: Page): Promise<void> {
  await page.route('**/api/parse', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        lang:              'FI',
        parse_id:          1,
        total_tokens:      2,
        parse_duration_ms: 5,
        words: [
          { lemma: 'kissa', pos: 'NOUN', forms: ['Kissa'], count: 1 },
          { lemma: 'juosta', pos: 'VERB', forms: ['juoksee'], count: 1 },
        ],
      }),
    });
  });
}

// ── Save-as-deck form: publish checkbox visibility ─────────────────────────

test('non-admin save-as-deck form has no "Publish as official" checkbox', async ({ page }) => {
  await mockMeWithDecks(page, 'user');
  await mockParse(page);

  await page.goto('/#/inspect');
  await page.locator('#inspect-text').fill(finnishText);
  await page.getByRole('button', { name: 'Parse text' }).click();

  await expect(page.locator('#results-page')).toHaveClass(/active/);
  await page.getByRole('button', { name: 'Save as deck' }).click();

  // The checkbox markup exists but is guarded by data-role-show="admin".
  // Non-admin users must NEVER see it.
  await expect(page.locator('.results-save-public')).toBeHidden();
  await expect(page.locator('#results-deck-public')).toBeHidden();
});

test('admin save-as-deck form exposes the "Publish as official" checkbox', async ({ page }) => {
  await mockMeWithDecks(page, 'admin');
  await mockParse(page);

  await page.goto('/#/inspect');
  await page.locator('#inspect-text').fill(finnishText);
  await page.getByRole('button', { name: 'Parse text' }).click();

  await expect(page.locator('#results-page')).toHaveClass(/active/);
  await page.getByRole('button', { name: 'Save as deck' }).click();

  await expect(page.locator('.results-save-public')).toBeVisible();
  await expect(page.locator('#results-deck-public')).toBeVisible();
  await expect(page.locator('.results-save-public')).toContainText(/Publish as an official deck/i);
});

test('admin checked checkbox sends is_public:true on create', async ({ page }) => {
  await mockMeWithDecks(page, 'admin');
  await mockParse(page);

  const createPayloads: Array<Record<string, unknown>> = [];
  await page.route('**/api/decks', async (route) => {
    if (route.request().method() !== 'POST') {
      await route.continue();
      return;
    }
    createPayloads.push(route.request().postDataJSON());
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ deck_id: 9 }),
    });
  });

  await page.goto('/#/inspect');
  await page.locator('#inspect-text').fill(finnishText);
  await page.getByRole('button', { name: 'Parse text' }).click();

  await page.getByRole('button', { name: 'Save as deck' }).click();
  await page.locator('#results-deck-title').fill('Beginner Finnish');
  await page.locator('#results-deck-public').check();
  await page.locator('#results-save-submit').click();

  await expect.poll(() => createPayloads.length).toBeGreaterThan(0);
  expect(createPayloads[0]).toMatchObject({
    title:     'Beginner Finnish',
    lang:      'FI',
    is_public: true,
  });
});

// ── My Decks list: Publish / Unpublish button visibility ───────────────────

const ownedDeck: DeckRow = {
  id:        5,
  title:     'My deck',
  lang:      'FI',
  known:     1,
  unique:    2,
  due:       0,
  is_public: false,
};

const ownedPublicDeck: DeckRow = {
  ...ownedDeck,
  is_public: true,
};

test('non-admin My Decks row does not expose a Publish button', async ({ page }) => {
  await mockMeWithDecks(page, 'user', [ownedDeck]);
  await page.goto('/#/decks');

  await expect(page.locator('#decks-page')).toHaveClass(/active/);
  const row = page.locator('.deck-list-item').first();
  await expect(row).toContainText('My deck');
  // Rename / Delete are still there; Publish / Unpublish must not be.
  await expect(row.getByRole('button', { name: 'Rename' })).toBeVisible();
  await expect(row.getByRole('button', { name: 'Delete' })).toBeVisible();
  await expect(row.locator('[data-toggle-public]')).toHaveCount(0);
});

test('admin My Decks row exposes a Publish button on a private deck', async ({ page }) => {
  await mockMeWithDecks(page, 'admin', [ownedDeck]);
  await page.goto('/#/decks');

  const row = page.locator('.deck-list-item').first();
  const publishBtn = row.locator('[data-toggle-public]');
  await expect(publishBtn).toBeVisible();
  await expect(publishBtn).toHaveText('Publish');
  await expect(publishBtn).toHaveAttribute('data-current-public', '0');
});

test('admin My Decks row exposes Unpublish on an already-public deck', async ({ page }) => {
  await mockMeWithDecks(page, 'admin', [ownedPublicDeck]);
  await page.goto('/#/decks');

  const row = page.locator('.deck-list-item').first();
  const toggleBtn = row.locator('[data-toggle-public]');
  await expect(toggleBtn).toBeVisible();
  await expect(toggleBtn).toHaveText('Unpublish');
  await expect(toggleBtn).toHaveAttribute('data-current-public', '1');
});

test('admin Publish click sends PATCH with is_public:true', async ({ page }) => {
  await mockMeWithDecks(page, 'admin', [ownedDeck]);

  const patchPayloads: Array<Record<string, unknown>> = [];
  await page.route('**/api/decks/5', async (route) => {
    if (route.request().method() !== 'PATCH') {
      await route.continue();
      return;
    }
    patchPayloads.push(route.request().postDataJSON());
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ status: 'ok' }),
    });
  });

  await page.goto('/#/decks');

  const row = page.locator('.deck-list-item').first();
  await row.locator('[data-toggle-public]').click();
  // Site's standard confirm modal — admin must affirm before the call fires.
  await page.locator('#dialog-modal-confirm').click();

  await expect.poll(() => patchPayloads.length).toBeGreaterThan(0);
  expect(patchPayloads[0]).toEqual({ is_public: true });
});
