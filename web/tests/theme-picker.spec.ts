import { expect, test, type Page } from '@playwright/test';

// The theme picker replaces the old single 🌓 toggle. Theming is two
// dimensional: skin (`data-skin`: ink | aalto) × mode (`data-theme`:
// light | dark). The Aalto skin ("Paimio" light / "Sanatorium" dark) is
// opt-in; the default stays the ink skin. These tests encode WHY the picker
// matters: choices must persist across reloads, both dimensions must apply to
// the document root, and the brand must always read "FinnEst" regardless of
// skin (owner correction, grill Q53).

// Anonymous /api/me so the landing nav (which hosts the picker) renders.
async function mockAnonMe(page: Page): Promise<void> {
  await page.route('**/api/me', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ authenticated: false, user: null, anon_max_chars: 300000 }),
    });
  });
}

async function openPicker(page: Page): Promise<void> {
  await page.locator('#theme-toggle').click();
  await expect(page.locator('#theme-menu')).toBeVisible();
}

test('default is the Aalto skin · Paimio light for a first-time visitor (no saved choice)', async ({ page }) => {
  await mockAnonMe(page);
  await page.goto('/');
  const root = page.locator('html');
  // Owner decision: the Claude Design prototype is the product's default face.
  // A visitor with no saved skin lands on Aalto · Paimio light. The Ink skin
  // stays fully selectable in the picker (covered by the tests below).
  await expect(root).toHaveAttribute('data-skin', 'aalto');
  await expect(root).toHaveAttribute('data-theme', 'light');
});

test('a saved Ink · dark choice is still honored (only the fallback default changed)', async ({ page }) => {
  await mockAnonMe(page);
  await page.goto('/');
  // Simulate a returning user who had explicitly chosen Ink · dark.
  await page.evaluate(() => {
    localStorage.setItem('skin', 'ink');
    localStorage.setItem('theme', 'dark');
  });
  await page.reload();
  const root = page.locator('html');
  await expect(root).toHaveAttribute('data-skin', 'ink');
  await expect(root).toHaveAttribute('data-theme', 'dark');
});

test('picker switches to Aalto · Paimio (skin+mode both apply to the root)', async ({ page }) => {
  await mockAnonMe(page);
  await page.goto('/');
  await openPicker(page);

  await page.locator('.theme-option[data-skin="aalto"][data-mode="light"]').click();

  const root = page.locator('html');
  await expect(root).toHaveAttribute('data-skin', 'aalto');
  await expect(root).toHaveAttribute('data-theme', 'light');
  // Selecting closes the menu.
  await expect(page.locator('#theme-menu')).toBeHidden();
});

test('picker switches to Aalto · Sanatorium (dark)', async ({ page }) => {
  await mockAnonMe(page);
  await page.goto('/');
  await openPicker(page);

  await page.locator('.theme-option[data-skin="aalto"][data-mode="dark"]').click();

  const root = page.locator('html');
  await expect(root).toHaveAttribute('data-skin', 'aalto');
  await expect(root).toHaveAttribute('data-theme', 'dark');
});

test('theme choice persists across a reload', async ({ page }) => {
  await mockAnonMe(page);
  await page.goto('/');
  await openPicker(page);
  await page.locator('.theme-option[data-skin="aalto"][data-mode="light"]').click();

  await page.reload();

  const root = page.locator('html');
  await expect(root).toHaveAttribute('data-skin', 'aalto');
  await expect(root).toHaveAttribute('data-theme', 'light');
  // Persistence is via localStorage: separate keys for skin and mode.
  expect(await page.evaluate(() => localStorage.getItem('skin'))).toBe('aalto');
  expect(await page.evaluate(() => localStorage.getItem('theme'))).toBe('light');
});

test('switching back to Ink · Light applies and persists', async ({ page }) => {
  await mockAnonMe(page);
  await page.goto('/');
  // Start on aalto, then switch back — proves the picker is not one-way.
  await openPicker(page);
  await page.locator('.theme-option[data-skin="aalto"][data-mode="dark"]').click();
  await openPicker(page);
  await page.locator('.theme-option[data-skin="ink"][data-mode="light"]').click();

  const root = page.locator('html');
  await expect(root).toHaveAttribute('data-skin', 'ink');
  await expect(root).toHaveAttribute('data-theme', 'light');
  await page.reload();
  await expect(root).toHaveAttribute('data-skin', 'ink');
  await expect(root).toHaveAttribute('data-theme', 'light');
});

test('the active option is marked selected in the menu', async ({ page }) => {
  await mockAnonMe(page);
  await page.goto('/');
  await openPicker(page);
  await page.locator('.theme-option[data-skin="aalto"][data-mode="light"]').click();

  await openPicker(page);
  const active = page.locator('.theme-option[data-skin="aalto"][data-mode="light"]');
  await expect(active).toHaveAttribute('aria-selected', 'true');
  await expect(active).toHaveClass(/selected/);
  // Exactly one option selected at a time.
  await expect(page.locator('.theme-option[aria-selected="true"]')).toHaveCount(1);
});

test('the brand reads "FinnEst" in both skins', async ({ page }) => {
  await mockAnonMe(page);
  await page.goto('/');

  // Ink skin: logo text comes from ::before content. Assert the rendered text.
  const inkBrand = await page.evaluate(() => {
    const el = document.getElementById('nav-logo');
    if (!el) return '';
    return getComputedStyle(el, '::before').content;
  });
  expect(inkBrand).toContain('FinnEst');

  // Switch to Aalto and re-check the pseudo-element text.
  await openPicker(page);
  await page.locator('.theme-option[data-skin="aalto"][data-mode="light"]').click();
  const aaltoBrand = await page.evaluate(() => {
    const el = document.getElementById('nav-logo');
    if (!el) return '';
    return getComputedStyle(el, '::before').content;
  });
  expect(aaltoBrand).toContain('FinnEst');
});

test('the menu dismisses on outside click and Escape', async ({ page }) => {
  await mockAnonMe(page);
  await page.goto('/');
  await openPicker(page);
  await page.locator('body').click({ position: { x: 5, y: 300 } });
  await expect(page.locator('#theme-menu')).toBeHidden();

  await openPicker(page);
  await page.keyboard.press('Escape');
  await expect(page.locator('#theme-menu')).toBeHidden();
});
