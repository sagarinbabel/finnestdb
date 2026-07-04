import { expect, test, type Page } from '@playwright/test';

// Ported Claude Design "Aalto edition" landing (design/aalto-landing.jsx).
// These specs encode WHY the port matters: the owner's verdict was that
// "nothing has been carried through from the design prototype" — so they pin
// the exact hero wording, the truthful eyebrow, the freemium band, and the
// real demo-chip wiring against the anonymous /api/demo/text allowlist, so a
// regression that drops the prototype look/behaviour fails a test.

async function mockAnonMe(page: Page, anonMax = 300000): Promise<void> {
  await page.route('**/api/me', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ authenticated: false, user: null, anon_max_chars: anonMax }),
    });
  });
}

test('hero reads the exact ported wording with italic-blue Suomi / Eesti', async ({ page }) => {
  await mockAnonMe(page);
  await page.goto('/');

  const hero = page.locator('.hero-title.hero-h');
  await expect(hero).toContainText('Paste your');
  await expect(hero).toContainText('Suomi');
  await expect(hero).toContainText('Eesti');
  await expect(hero).toContainText('Lift the words out');
  // Suomi/Eesti are the italic-accent <em>s from the prototype.
  await expect(hero.locator('em')).toHaveText(['Suomi', 'Eesti']);

  // Owner-override subtitle verbatim, plus the quieter sign-in parenthetical.
  const sub = page.locator('.hero-subtitle.hero-sub');
  await expect(sub).toContainText(
    'Drop in any Finnish or Estonian text — news, a chapter, a conversation. Learn it in context. Enjoy reading the article smoothly.',
  );
  await expect(sub.locator('.hero-sub-quiet')).toHaveText(
    '(sign in to export, create, sync and review your flashcards)',
  );
});

test('eyebrow states the truthful anonymous demo promise with a pulse dot', async ({ page }) => {
  await mockAnonMe(page);
  await page.goto('/');

  const eyebrow = page.locator('.landing-wrap .hero-eyebrow');
  await expect(eyebrow).toContainText(/FREE · NO ACCOUNT · NO HISTORY SAVED/i);
  await expect(eyebrow.locator('.pulse')).toBeVisible();
  // The dishonest "Ephemeral OFF" toggle from the prototype must NOT be ported:
  // anonymous parses are always ephemeral, so a toggle would be a lie.
  await expect(page.locator('#landing-page')).not.toContainText(/Ephemeral/i);
});

test('freemium band renders i / ii / iii with the truth-adjusted wording', async ({ page }) => {
  await mockAnonMe(page);
  await page.goto('/');

  const cells = page.locator('.freemium-strip .freemium-cell');
  await expect(cells).toHaveCount(3);
  // Cell i: the 300k / no-login claim is now TRUE (anon cap is 300,000).
  await expect(cells.nth(0)).toContainText('Up to 300k characters per paste. No login.');
  // Cell ii: verbatim prototype "Copy or download" — made true by the anon
  // export controls exercised in the results test below.
  await expect(cells.nth(1)).toContainText('Copy or download');
  await expect(cells.nth(1)).toContainText('Word list as plain text or CSV');
  // Cell iii: "Free Google sign-in" → "Free sign-up." (OAuth not shipped).
  await expect(cells.nth(2)).toContainText('Free sign-up.');
  await expect(cells.nth(2)).not.toContainText(/Google/i);
});

test('demo chips load real embedded texts, fill the box, and set the language', async ({ page }) => {
  await mockAnonMe(page);
  await page.goto('/');

  // The FI article chip pulls the real sauna fixture from the anonymous
  // allowlist endpoint (no auth). Text lands in the box; Parse becomes reachable.
  await page.locator('.demo-chip[data-demo-id="fi-sauna-article"]').click();
  await expect.poll(async () => (await page.locator('#landing-text').inputValue()).length).toBeGreaterThan(100);
  await expect(page.locator('#landing-lang')).toHaveAttribute('data-value', 'FI');

  // The ET story chip switches the selector to Estonian and swaps the text.
  await page.locator('.demo-chip[data-demo-id="et-linnu-keel-story"]').click();
  await expect(page.locator('#landing-lang')).toHaveAttribute('data-value', 'ET');
  await expect.poll(async () => (await page.locator('#landing-text').inputValue()).length).toBeGreaterThan(100);
});

test('anonymous results expose copy + download-CSV export (freemium cell ii is true)', async ({ page }) => {
  await mockAnonMe(page);
  await page.route('**/api/parse', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        lang: 'FI',
        parse_id: null,
        total_tokens: 3,
        parse_duration_ms: 5,
        words: [
          { lemma: 'sauna', pos: 'NOUN', forms: ['sauna'], count: 2, gloss: 'sauna' },
          { lemma: 'lämmin', pos: 'ADJ', forms: ['lämmin'], count: 1, gloss: 'warm' },
        ],
      }),
    });
  });
  await page.goto('/');
  await page.locator('#landing-text').fill('Sauna on lämmin. Sauna on hyvä.');
  await page.locator('#landing-submit').click();
  await expect(page.locator('#results-page')).toHaveClass(/active/);

  // Export controls are visible to the anonymous visitor (not sign-in gated).
  const exportWrap = page.locator('#results-export');
  await expect(exportWrap).toBeVisible();
  await expect(exportWrap.getByRole('button', { name: 'Copy list' })).toBeVisible();

  // Download produces a CSV whose header + rows come from the parse response.
  const [download] = await Promise.all([
    page.waitForEvent('download'),
    exportWrap.getByRole('button', { name: 'Download CSV' }).click(),
  ]);
  expect(download.suggestedFilename()).toBe('finnest-fi-wordlist.csv');
  const stream = await download.createReadStream();
  const chunks: Buffer[] = [];
  for await (const c of stream) chunks.push(c as Buffer);
  const csv = Buffer.concat(chunks).toString('utf-8');
  expect(csv.split(/\r?\n/)[0]).toBe('lemma,pos,forms,definition,grammar,count');
  expect(csv).toContain('"sauna"');
  expect(csv).toContain('"warm"');
});

// Mobile: the landing must be clean at 375 px — decorations hide, no overflow.
test('landing is clean at 375 px with decorations hidden and no horizontal overflow', async ({ page }) => {
  await mockAnonMe(page);
  await page.setViewportSize({ width: 375, height: 812 });
  await page.goto('/');

  await expect(page.locator('.hero-title.hero-h')).toBeVisible();
  await expect(page.locator('.freemium-strip')).toBeVisible();
  // The birch wordmark + vase silhouette are desktop-only decorations.
  await expect(page.locator('.aalto-mark')).toBeHidden();
  await expect(page.locator('.vase-svg')).toBeHidden();

  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
  );
  expect(overflow, 'horizontal overflow px @375').toBeLessThanOrEqual(1);
});
