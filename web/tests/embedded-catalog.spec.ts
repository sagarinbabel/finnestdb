// Embedded Text catalog cold-start flow (signed-in).
//
// Verifies the curated-catalog empty-state path from docs/USER_FLOWS.md §4:
// a signed-in learner with no decks sees the catalog on the dashboard, picks a
// text, lands on Inspect with the full text loaded into the textarea, and can
// parse it. All backend calls are mocked so the test is deterministic and does
// not depend on the dictionary DB, mirroring web/tests/parse-results.spec.ts
// conventions.
import { expect, test, type Page } from '@playwright/test';

const CATALOG_TEXT = 'Kissa istui ikkunalla ja katseli lintuja pihalla. Aurinko paistoi kirkkaasti.';

async function mockSignedInNoDecks(page: Page): Promise<void> {
  await page.route('**/api/me', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        authenticated: true,
        user: { id: 1, email: 'alice@example.com', is_admin: false },
        dashboard: { known_count: 0, due_count: 0, new_capacity_today: 0, decks: [] },
        languages: { learning: ['FI', 'ET'], active: 'FI', stats: {} },
      }),
    });
  });
}

async function mockCatalog(page: Page): Promise<void> {
  await page.route('**/api/catalog', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        has_known_words: { FI: false, ET: false },
        entries: [
          {
            id: 'fi-sample-story',
            language: 'fi',
            title: 'Kissa ikkunalla',
            author: 'FinnEst',
            genre: 'story',
            difficulty: 'easy',
            difficulty_review: 'pending',
            corpus_source: 'finnest-original',
            license: 'CC0 1.0',
            text_length: CATALOG_TEXT.length,
            word_count: 11,
          },
        ],
      }),
    });
  });

  await page.route('**/api/catalog/fi-sample-story/text', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        id: 'fi-sample-story',
        language: 'fi',
        title: 'Kissa ikkunalla',
        text: CATALOG_TEXT,
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
        lang: 'FI',
        parser: 'custom',
        total_tokens: 11,
        stats: { unique_forms: 11, total_sentences: 2, resolved_tokens: 11, unresolved_tokens: 0, punct_tokens: 2, timings: {} },
        words: [
          { lemma: 'kissa', pos: 'NOUN', forms: ['Kissa'], count: 1, example_sentence: CATALOG_TEXT },
          { lemma: 'aurinko', pos: 'NOUN', forms: ['Aurinko'], count: 1, example_sentence: CATALOG_TEXT },
        ],
        sentences: [],
      }),
    });
  });
}

test('signed-in cold start: dashboard catalog -> pick text -> textarea populated -> parse', async ({ page }) => {
  await mockSignedInNoDecks(page);
  await mockCatalog(page);
  await mockParse(page);

  await page.goto('/#/dashboard');

  // The cold-start catalog section appears for a learner with no decks.
  const coldStart = page.locator('#dashboard-cold-start');
  await expect(coldStart).not.toHaveClass(/hidden/);
  await expect(page.locator('#dashboard-catalog')).toContainText('Kissa ikkunalla');
  // With no known words, the card prompts import rather than showing coverage.
  await expect(page.locator('#dashboard-catalog')).toContainText('Import known words');

  // Pick the text: lands on Inspect with the full text loaded.
  await page.locator('#dashboard-catalog [data-catalog-id="fi-sample-story"]').click();

  await expect(page.locator('#inspect-page')).toHaveClass(/active/);
  await expect(page.locator('#inspect-text')).toHaveValue(CATALOG_TEXT);

  // The cold-start catalog on Inspect hides once text is present.
  await expect(page.locator('#inspect-catalog-section')).toHaveClass(/hidden/);

  // The normal parse flow takes over.
  await page.getByRole('button', { name: 'Parse text' }).click();
  await expect(page.locator('#results-page')).toHaveClass(/active/);
  await expect(page.locator('#results-page')).toContainText('kissa');
});

test('inspect empty state shows the catalog and hides it after typing', async ({ page }) => {
  await mockSignedInNoDecks(page);
  await mockCatalog(page);

  await page.goto('/#/inspect');

  const section = page.locator('#inspect-catalog-section');
  await expect(section).not.toHaveClass(/hidden/);
  await expect(page.locator('#inspect-catalog')).toContainText('Kissa ikkunalla');

  await page.locator('#inspect-text').fill('Oma teksti tähän.');
  await expect(section).toHaveClass(/hidden/);
});
