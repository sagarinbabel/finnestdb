// Flow-continuity regressions (docs/USER_FLOWS.md "Carry-forward of anonymous
// parses"; the owner's second manual test).
//
// Covers:
//   1. Signing in no longer loses your place: an anonymous parse is carried
//      forward in sessionStorage and, after sign-in, the visitor returns to the
//      results view (Read tab) re-rendered against the authenticated state — the
//      reveal's known % becomes real — instead of being dropped on the dashboard.
//   2. The landing/Inspect textarea auto-grows with content instead of scrolling
//      inside a fixed small box while the page below sits empty.
//
// Backend calls are mocked so the test is deterministic and independent of the
// dictionary DB, mirroring web/tests/anonymous-parser-demo.spec.ts conventions.
import { expect, test, type Page } from '@playwright/test';

// Enough ä/ö density that detectLang keeps the FI-selected landing form from
// blocking the parse on a language mismatch.
const finnishText = 'Menin pankkiin eilen ja ostin hyvää leipää. Kissa nukkui pöydällä.';

async function mockAnonMe(page: Page): Promise<void> {
  await page.route('**/api/me', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ authenticated: false, user: null, anon_max_chars: 300000 }),
    });
  });
}

async function mockAnonParse(page: Page): Promise<void> {
  await page.route('**/api/parse', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        lang: 'FI',
        parse_id: null,
        total_tokens: 6,
        parse_duration_ms: 8,
        words: [
          { lemma: 'pankki', pos: 'NOUN', forms: ['pankkiin'], count: 3, gloss: 'bank' },
          { lemma: 'kissa', pos: 'NOUN', forms: ['Kissa'], count: 3, gloss: 'cat' },
        ],
      }),
    });
  });
}

test('anonymous parse survives sign-in: returns to results on the Read tab with real known state', async ({ page }) => {
  // /api/me is anon until login, then authenticated so the app re-roles.
  let meCall = 0;
  await page.route('**/api/me', async (route) => {
    meCall += 1;
    const authed = meCall > 1;
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(authed
        ? {
            authenticated: true,
            user: { id: 1, email: 'alice@example.com', is_admin: false },
            dashboard: { known_count: 1, due_count: 0, new_capacity_today: 0, decks: [] },
            languages: { learning: ['FI'], active: 'FI', stats: {} },
          }
        : { authenticated: false, user: null, anon_max_chars: 300000 }),
    });
  });
  await mockAnonParse(page);
  await page.route('**/api/auth/login', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ authenticated: true, user: { id: 1, email: 'alice@example.com', is_admin: false } }),
    });
  });
  // After sign-in, carry-forward re-hydrates learning state. Mark "kissa" known
  // so the reveal has a real (non-zero) known floor once authenticated.
  await page.route('**/api/lemma-states', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ states: [{ lemma: 'kissa', pos: 'NOUN', status: 'known' }] }),
    });
  });

  // Anonymous parse: lands on results, and the reveal is projection-from-zero.
  await page.goto('/');
  await page.locator('#landing-text').fill(finnishText);
  await page.getByRole('button', { name: 'Parse text' }).click();
  await expect(page.locator('#results-page')).toHaveClass(/active/);
  await expect(page.locator('#coverage-reveal')).toContainText(/most frequent words in this text carry/i);

  // Go sign in. The carried parse is held in sessionStorage.
  await page.goto('/#/signin');
  await page.getByLabel('Email').fill('alice@example.com');
  await page.getByLabel('Password').fill('password123');
  await page.getByRole('button', { name: 'Sign in' }).click();

  // Carry-forward: back on results, NOT the dashboard.
  await expect(page.locator('#results-page')).toHaveClass(/active/);
  await expect(page.locator('#dashboard-page')).not.toHaveClass(/active/);
  // Read tab is the restored view (the reading surface, "open like a book").
  await expect(page.locator('#results-tab-read')).toHaveAttribute('aria-selected', 'true');
  await expect(page.locator('#read-view')).not.toHaveClass(/hidden/);
  // The reveal re-rendered against the authenticated state: it now speaks in
  // real known-state terms ("You already know …%"), not projection-from-zero.
  await expect(page.locator('#coverage-reveal')).toContainText(/You already know/i);
  // Signed-in-only chrome is now present (role visibility re-applied on restore).
  await expect(page.locator('#anon-privacy-footer')).toBeHidden();
});

test('landing textarea auto-grows with content instead of scrolling inside a fixed box', async ({ page }) => {
  await mockAnonMe(page);
  await page.goto('/');

  const ta = page.locator('#landing-text');
  const emptyHeight = await ta.evaluate((el) => (el as HTMLTextAreaElement).clientHeight);
  const cap = await page.evaluate(() => Math.round(window.innerHeight * 0.7));

  // A modest multi-line paste (comfortably under the ~70vh cap) should grow the
  // box to fit its content with NO inner scrollbar — the whole point: don't
  // scroll inside a small box while the page below is empty.
  const modest = Array.from({ length: 14 }, (_, i) => `Rivi numero ${i} tähän.`).join('\n');
  await ta.fill(modest);
  const modestHeight = await ta.evaluate((el) => (el as HTMLTextAreaElement).clientHeight);
  expect(modestHeight).toBeGreaterThan(emptyHeight);
  expect(modestHeight).toBeLessThanOrEqual(cap);
  const modestOverflow = await ta.evaluate((el) => {
    const t = el as HTMLTextAreaElement;
    return t.scrollHeight - t.clientHeight;
  });
  expect(modestOverflow).toBeLessThanOrEqual(4); // sub-pixel/rounding slack only.

  // A very long paste stops growing at the ~70vh cap and then scrolls
  // internally, so the box never eats the whole page.
  const huge = Array.from({ length: 120 }, (_, i) => `Pitkä rivi numero ${i} tähän tekstiin.`).join('\n');
  await ta.fill(huge);
  const hugeHeight = await ta.evaluate((el) => (el as HTMLTextAreaElement).clientHeight);
  expect(hugeHeight).toBeLessThanOrEqual(cap + 2); // capped (small rounding slack)
  const hugeOverflow = await ta.evaluate((el) => {
    const t = el as HTMLTextAreaElement;
    return t.scrollHeight - t.clientHeight;
  });
  expect(hugeOverflow).toBeGreaterThan(0); // now it scrolls internally
});
