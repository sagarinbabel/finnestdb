import { expect, test, type Page } from '@playwright/test';

// Post-parse coverage reveal (aha moment #1). These specs assert the NUMBERS
// the reveal settles on — derived from the mocked parse response with the same
// token-mass formula the server uses for saved-deck comprehension — and the
// reduced-motion collapse. They assert final text, never animation frames.

const finnishText = 'Menin pankkiin eilen ja ostin hyvää leipää. Kissa nukkui täällä.';

// ── Shared /api/me mocks ────────────────────────────────────────────────────

async function mockUserMe(page: Page): Promise<void> {
  await page.route('**/api/me', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        authenticated: true,
        user: { id: 1, email: 'alice@example.com', is_admin: false },
        dashboard: { known_count: 10, due_count: 0, new_capacity_today: 5, decks: [] },
        languages: { learning: ['FI', 'ET'], active: 'FI', stats: { FI: { decks: 0, known_words: 10 } } },
      }),
    });
  });
}

async function mockAnonMe(page: Page): Promise<void> {
  await page.route('**/api/me', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ authenticated: false, user: null, anon_max_chars: 300000 }),
    });
  });
}

// Signed-in parse: covered mass (known 6 + ignored 2) = 8 of 16 total = 50%.
// Unknown words c(4), d(3), e(1) carry the remaining 8; there are only 3, so
// the reveal offers all 3 as the top-N, projecting 8/16 → 16/16 = 100%.
async function mockUserParse(page: Page): Promise<void> {
  await page.route('**/api/parse', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        lang: 'FI',
        parse_id: null,
        total_tokens: 16,
        parse_duration_ms: 10,
        words: [
          { lemma: 'a', pos: 'NOUN', forms: ['a'], count: 6, gloss: 'a', learning_state: 'known' },
          { lemma: 'b', pos: 'PROPN', forms: ['b'], count: 2, gloss: 'b', learning_state: 'ignored' },
          { lemma: 'c', pos: 'VERB', forms: ['c'], count: 4, gloss: 'c' },
          { lemma: 'd', pos: 'NOUN', forms: ['d'], count: 3, gloss: 'd' },
          { lemma: 'e', pos: 'ADV', forms: ['e'], count: 1, gloss: 'e' },
        ],
      }),
    });
  });
}

// Anonymous parse: no learning_state exists, so coverage starts at zero and the
// reveal projects from frequency. 15 unknown words totalling 50 tokens; the top
// 10 by count carry 45 (a five-tail of count-1 words falls outside the top 10),
// so the reveal states the 10 most frequent carry exactly 90% — no hedge.
async function mockAnonParse(page: Page): Promise<void> {
  const counts = [10, 8, 6, 5, 4, 3, 3, 2, 2, 2, 1, 1, 1, 1, 1];
  const words = counts.map((count, i) => ({
    lemma: `w${String.fromCharCode(97 + i)}`, pos: 'NOUN',
    forms: [`w${i}`], count, gloss: 'x',
  }));
  await page.route('**/api/parse', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        lang: 'FI',
        parse_id: null,
        total_tokens: 50,
        parse_duration_ms: 8,
        words,
      }),
    });
  });
}

// ── Signed-in reveal ────────────────────────────────────────────────────────

test('signed-in reveal counts up to the API-derived projected coverage', async ({ page }) => {
  await mockUserMe(page);
  await mockUserParse(page);
  await page.goto('/#/inspect');

  await page.locator('#inspect-text').fill(finnishText);
  await page.getByRole('button', { name: 'Parse text' }).click();
  await expect(page.locator('#results-page')).toHaveClass(/active/);

  const reveal = page.locator('#coverage-reveal');
  await expect(reveal).toBeVisible();
  // Headline names the already-known share (50%).
  await expect(reveal.locator('.coverage-reveal-headline')).toContainText('You already know');
  await expect(reveal.locator('.coverage-reveal-headline')).toContainText('50%');
  // Projection offers the 3 unknown words and the lift to 100%.
  await expect(reveal.locator('.coverage-reveal-projection')).toContainText('Learn the top');
  await expect(reveal.locator('.coverage-reveal-projection')).toContainText('3');
  await expect(reveal.locator('.coverage-reveal-projection')).toContainText('100%');

  // The headline count-up settles on the "already known" figure (50%), the
  // number the user is meant to feel. Poll on final text, not frames.
  await expect
    .poll(async () => (await reveal.locator('#coverage-reveal-figure').textContent())?.trim())
    .toBe('50%');

  // The bar previews the lift: known floor 50% + gain to the projected 100%.
  await expect
    .poll(() => reveal.locator('#coverage-reveal-known').evaluate(el => (el as HTMLElement).style.width))
    .toBe('50%');
  await expect
    .poll(() => reveal.locator('#coverage-reveal-gain').evaluate(el => (el as HTMLElement).style.width))
    .toBe('50%');
});

test('signed-in reveal has no exclamation marks and hedges nothing when exact', async ({ page }) => {
  await mockUserMe(page);
  await mockUserParse(page);
  await page.goto('/#/inspect');
  await page.locator('#inspect-text').fill(finnishText);
  await page.getByRole('button', { name: 'Parse text' }).click();
  await expect(page.locator('#results-page')).toHaveClass(/active/);

  const text = (await page.locator('#coverage-reveal').textContent()) ?? '';
  expect(text).not.toContain('!');
  // 50% and 100% are exact ratios of 16, so no ≈ hedge should appear.
  expect(text).not.toContain('≈');
});

// ── Anonymous reveal (projection from zero) ─────────────────────────────────

test('anonymous reveal frames coverage from frequency, not known state', async ({ page }) => {
  await mockAnonMe(page);
  await mockAnonParse(page);
  await page.goto('/');

  await page.locator('#landing-text').fill(finnishText);
  await page.getByRole('button', { name: 'Parse text' }).click();
  await expect(page.locator('#results-page')).toHaveClass(/active/);

  const reveal = page.locator('#coverage-reveal');
  await expect(reveal).toBeVisible();
  // Frequency framing: the 10 most frequent words carry 90%.
  await expect(reveal.locator('.coverage-reveal-headline')).toContainText('10 most frequent words');
  await expect(reveal.locator('.coverage-reveal-headline')).toContainText('90%');
  // No "you already know" claim — there is no known state for an anon visitor.
  await expect(reveal.locator('.coverage-reveal-headline')).not.toContainText('already know');

  // Count-up settles on 90%.
  await expect
    .poll(async () => (await reveal.locator('#coverage-reveal-figure').textContent())?.trim())
    .toBe('90%');

  // The existing signup ribbon still follows the reveal for anonymous visitors.
  await expect(page.locator('#anon-signup-ribbon')).toBeVisible();
});

// ── Reduced motion ──────────────────────────────────────────────────────────

test('reduced-motion collapses the reveal to its final value instantly', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await mockUserMe(page);
  await mockUserParse(page);
  await page.goto('/#/inspect');

  await page.locator('#inspect-text').fill(finnishText);
  await page.getByRole('button', { name: 'Parse text' }).click();
  await expect(page.locator('#results-page')).toHaveClass(/active/);

  const reveal = page.locator('#coverage-reveal');
  await expect(reveal).toBeVisible();
  // With reduced motion the figure is final on first paint — assert directly,
  // no polling window needed. The headline figure is the known share (50%).
  await expect(reveal.locator('#coverage-reveal-figure')).toHaveText('50%');
  // Bar segments are at their final widths (known 50% + gain 50% → 100%).
  await expect
    .poll(() => reveal.locator('#coverage-reveal-known').evaluate(el => (el as HTMLElement).style.width))
    .toBe('50%');
  await expect
    .poll(() => reveal.locator('#coverage-reveal-gain').evaluate(el => (el as HTMLElement).style.width))
    .toBe('50%');
  // Projection still shows the lift to 100%.
  await expect(reveal.locator('.coverage-reveal-projection')).toContainText('100%');
});
