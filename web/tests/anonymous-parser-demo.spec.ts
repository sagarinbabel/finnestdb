import { expect, test, type Page } from '@playwright/test';

// A short Finnish snippet with enough ä/ö density to satisfy detectLang so the
// FI-selected landing form doesn't block parsing on a language mismatch.
const finnishText = 'Menin pankkiin eilen ja ostin hyvää leipää. Kissa nukkui.';

// Anonymous /api/me mock. anonMax lets a test dial the surfaced cap so the
// char counter and cap-error copy can be asserted deterministically.
async function mockAnonMe(page: Page, anonMax = 300000): Promise<void> {
  await page.route('**/api/me', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ authenticated: false, user: null, anon_max_chars: anonMax }),
    });
  });
}

// Successful anonymous parse response (no parse_id - ephemeral). Two words,
// one with a definition so coverage is non-zero.
async function mockAnonParse(page: Page): Promise<void> {
  await page.route('**/api/parse', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        lang: 'FI',
        parse_id: null,
        total_tokens: 5,
        parse_duration_ms: 9,
        words: [
          { lemma: 'pankki', pos: 'NOUN', forms: ['pankkiin'], count: 1, gloss: 'bank' },
          { lemma: 'kissa', pos: 'NOUN', forms: ['Kissa'], count: 1, gloss: 'cat' },
        ],
      }),
    });
  });
}

test('anonymous landing shows a parse form with the demo cap in the counter', async ({ page }) => {
  await mockAnonMe(page, 300000);
  await page.goto('/');

  await expect(page.locator('#landing-page')).toHaveClass(/active/);
  await expect(page.locator('#landing-form')).toBeVisible();
  await expect(page.locator('#landing-text')).toBeVisible();
  // Counter reflects the server-surfaced anonymous cap, not the 1.5M signed-in one.
  await expect(page.locator('#landing-char-count')).toContainText('/ 300,000');
  // Ported prototype hero + freemium band render around the form.
  await expect(page.locator('.hero-title')).toContainText(/Lift the words out/i);
  await expect(page.locator('.freemium-strip')).toBeVisible();
});

test('anonymous paste → parse → explore-only results', async ({ page }) => {
  await mockAnonMe(page);
  await mockAnonParse(page);
  await page.goto('/');

  await page.locator('#landing-text').fill(finnishText);
  await page.locator('#landing-submit').click();

  await expect(page.locator('#results-page')).toHaveClass(/active/);
  // Non-admin anonymous parser pill is the soft "Your text" label.
  await expect(page.locator('#results-parser')).toHaveText('Your text');
  // Signed-in-only save CTA must NOT be visible to an anonymous visitor
  // (shared chrome above the tabs, gated by data-role-show).
  await expect(page.locator('.results-deck-cta')).toBeHidden();

  // The lemma table lives behind the Words tab (Read is default). Switch to it
  // for the table / filter / sort / column assertions.
  await page.locator('#results-tab-words').click();
  await expect(page.locator('#word-table-body tr')).toHaveCount(2);
  // Explore controls present: POS filter chips + sortable columns.
  await expect(page.locator('.pos-filter-chip', { hasText: 'Nouns' })).toBeVisible();
  await expect(page.locator('.sort-btn', { hasText: 'Occurrences' })).toBeVisible();

  // Signed-in-only controls must NOT be visible to an anonymous visitor.
  await expect(page.locator('.correction-btn')).toHaveCount(0);
  await expect(page.locator('.word-pill-known')).toHaveCount(0);
  await expect(page.locator('.word-icon-ignore')).toHaveCount(0);
  await expect(page.locator('.col-actions').first()).toBeHidden();

  // Privacy footer present for anonymous results.
  await expect(page.locator('#anon-privacy-footer')).toBeVisible();
  await expect(page.locator('#anon-privacy-footer')).toContainText(/wasn't saved/i);
});

test('sign-up ribbon dismisses per session and reappears on the next parse', async ({ page }) => {
  await mockAnonMe(page);
  await mockAnonParse(page);
  await page.goto('/');

  await page.locator('#landing-text').fill(finnishText);
  await page.locator('#landing-submit').click();
  await expect(page.locator('#results-page')).toHaveClass(/active/);

  const ribbon = page.locator('#anon-signup-ribbon');
  await expect(ribbon).toBeVisible();
  await expect(ribbon).toContainText(/Save these words/i);
  // The ribbon must sell a concrete gain behind signup, not a flat save-your-work line.
  await expect(ribbon).toContainText(/study deck/i);

  // Dismiss for the session.
  await page.locator('#anon-ribbon-dismiss').click();
  await expect(ribbon).toBeHidden();

  // Back to the landing form, parse again - the ribbon must reappear.
  await page.locator('#results-back').click();
  await expect(page.locator('#landing-page')).toHaveClass(/active/);
  await page.locator('#landing-text').fill(finnishText + ' Uusi lause tähän.');
  await page.locator('#landing-submit').click();
  await expect(page.locator('#results-page')).toHaveClass(/active/);
  await expect(ribbon).toBeVisible();
});

test('anonymous over-cap parse surfaces the limit message and sign-up CTA', async ({ page }) => {
  await mockAnonMe(page, 30);
  // Server rejects an over-cap anonymous parse with a JSON error naming the limit.
  await page.route('**/api/parse', async (route) => {
    await route.fulfill({
      status: 400,
      contentType: 'application/json',
      body: JSON.stringify({
        error: 'Anonymous parsing is limited to 30 characters. Sign up to parse longer texts.',
        anon_max_chars: 30,
        chars: 200,
      }),
    });
  });
  await page.goto('/');

  // With a 30-char cap, the client counter reflects it.
  await expect(page.locator('#landing-char-count')).toContainText('/ 30');

  // A long paste is blocked client-side before the request even leaves.
  await page.locator('#landing-text').fill(finnishText.repeat(6));
  await page.locator('#landing-submit').click();
  await expect(page.locator('.toast.error')).toContainText(/limited to 30 characters/i);
  await expect(page.locator('.toast.error')).toContainText(/[Ss]ign up/);
  // Still on the landing page - no results rendered.
  await expect(page.locator('#landing-page')).toHaveClass(/active/);
});
