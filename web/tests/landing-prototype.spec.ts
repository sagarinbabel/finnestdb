import { expect, test, type Page } from '@playwright/test';

// Ported Claude Design "Aalto edition" landing (design/aalto-landing.jsx).
// These specs encode WHY the port matters: the owner's verdict was that
// "nothing has been carried through from the design prototype" - so they pin
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

test('landing copy explains contextual learning and reveals native language names on hover', async ({ page }) => {
  await mockAnonMe(page);
  await page.goto('/');

  const hero = page.locator('.hero-title.hero-h');
  await expect(hero).toHaveText('Learn in Context');

  const sub = page.locator('.hero-subtitle.hero-sub');
  await expect(sub).toContainText(
    'Drop in any Finnish or Estonian text - news, a chapter, or a conversation. FinnEst will split the text into words. It will tell you what words to learn to improve your understanding of the text. Learn them on FinnEst or flashcard app of choice. Enjoy reading & watch your vocabulary & comprehension grow!',
  );
  await expect(sub.locator('.hero-sub-quiet')).toHaveText(
    '(sign in to export, create, sync and review your flashcards)',
  );

  // The English words remain readable by default, then reveal their native
  // names in the landing display face when a pointer passes over them.
  for (const [english, native] of [['Finnish', 'Suomi'], ['Estonian', 'Eesti']]) {
    const word = page.locator(`.language-reveal[data-native="${native}"]`);
    await expect(word).toHaveText(english);
    await word.hover();
    await expect.poll(() => word.evaluate((el) => getComputedStyle(el, '::after').opacity)).toBe('1');
    await expect.poll(() => word.evaluate((el) => getComputedStyle(el, '::after').content)).toBe(`"${native}"`);
    const nativeStyle = await word.evaluate((el) => {
      const pseudo = getComputedStyle(el, '::after');
      const headline = document.querySelector('.landing-wrap .hero-title.hero-h');
      const activeLanguage = document.querySelector('.landing-wrap .btn-radio-option.is-active');
      if (!headline || !activeLanguage) throw new Error('landing typography controls are missing');
      return {
        accent: getComputedStyle(activeLanguage).backgroundColor,
        color: pseudo.color,
        headlineFamily: getComputedStyle(headline).fontFamily,
        fontFamily: pseudo.fontFamily,
        fontStyle: pseudo.fontStyle,
      };
    });
    expect(nativeStyle.fontFamily).toBe(nativeStyle.headlineFamily);
    expect(nativeStyle.fontStyle).toBe('italic');
    expect(nativeStyle.color).toBe(nativeStyle.accent);
  }

  // The sign-in note is now a second paragraph line in the same typography,
  // rather than a smaller, muted sans-serif aside.
  const typography = await sub.evaluate((el) => {
    const quiet = el.querySelector('.hero-sub-quiet')!;
    const parent = getComputedStyle(el);
    const note = getComputedStyle(quiet);
    return [parent.fontFamily, parent.fontSize, parent.fontWeight, parent.lineHeight]
      .map((value, index) => [value, [note.fontFamily, note.fontSize, note.fontWeight, note.lineHeight][index]]);
  });
  expect(typography.every(([parent, note]) => parent === note)).toBeTruthy();
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
  // Cell ii: verbatim prototype "Copy or download" - made true by the anon
  // export controls exercised in the results test below.
  await expect(cells.nth(1)).toContainText('Copy or download');
  await expect(cells.nth(1)).toContainText('Word list as plain text or CSV');
  // Cell iii: "Free Google sign-in" → "Free sign-up." (OAuth not shipped).
  await expect(cells.nth(2)).toContainText('Free sign-up.');
  await expect(cells.nth(2)).not.toContainText(/Google/i);
});

test('desktop hero copy and parser use the full landing content width', async ({ page }) => {
  await mockAnonMe(page);
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto('/');

  // The owner expects the subtitle and parse box to span the same landing
  // column as the blue three-step band, rather than looking like a narrow
  // left-aligned slice of it.
  const [subtitle, form, strip] = await Promise.all([
    page.locator('.hero-subtitle.hero-sub').boundingBox(),
    page.locator('#landing-form').boundingBox(),
    page.locator('.freemium-strip').boundingBox(),
  ]);
  expect(subtitle).not.toBeNull();
  expect(form).not.toBeNull();
  expect(strip).not.toBeNull();
  expect(subtitle!.width).toBeCloseTo(strip!.width, 0);
  expect(form!.width).toBeCloseTo(strip!.width, 0);
});

test('About uses the same hero typography and omits design-only labels', async ({ page }) => {
  await mockAnonMe(page);
  for (const skin of ['aalto', 'ink']) {
    await page.goto('/');
    await page.evaluate((selectedSkin) => {
      localStorage.setItem('skin', selectedSkin);
      localStorage.setItem('theme', 'light');
    }, skin);
    await page.reload();

    await expect(page.locator('.aalto-mark')).toHaveCount(0);
    await expect(page.locator('.colophon')).toHaveCount(0);

    const homeType = await page.locator('.hero-title.hero-h').evaluate((el) => {
      const style = getComputedStyle(el);
      return [style.fontFamily, style.fontSize, style.fontWeight, style.letterSpacing, style.lineHeight];
    });
    const homeLedeType = await page.locator('.hero-subtitle.hero-sub').evaluate((el) => {
      const style = getComputedStyle(el);
      return [style.fontFamily, style.fontSize, style.fontWeight, style.lineHeight];
    });

    await page.goto('/#/about');
    const aboutType = await page.locator('.about-hero h1').evaluate((el) => {
      const style = getComputedStyle(el);
      return [style.fontFamily, style.fontSize, style.fontWeight, style.letterSpacing, style.lineHeight];
    });
    const aboutLedeType = await page.locator('.about-lede').evaluate((el) => {
      const style = getComputedStyle(el);
      return [style.fontFamily, style.fontSize, style.fontWeight, style.lineHeight];
    });

    expect(aboutType).toEqual(homeType);
    expect(aboutLedeType).toEqual(homeLedeType);
  }
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

// Mobile: the landing must be clean at 375 px - decorations hide, no overflow.
test('landing is clean at 375 px with decorations hidden and no horizontal overflow', async ({ page }) => {
  await mockAnonMe(page);
  await page.setViewportSize({ width: 375, height: 812 });
  await page.goto('/');

  await expect(page.locator('.hero-title.hero-h')).toBeVisible();
  await expect(page.locator('.freemium-strip')).toBeVisible();
  // The vase silhouette is a desktop-only decoration.
  await expect(page.locator('.vase-svg')).toBeHidden();

  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
  );
  expect(overflow, 'horizontal overflow px @375').toBeLessThanOrEqual(1);
});
