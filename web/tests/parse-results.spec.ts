import { expect, test, type Page } from '@playwright/test';

const mixedFinnishText = 'Viisutubettaja lauloi nopeasti. Menin pankkiin eilen.';

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
        dashboard: {
          known_count: 1234,
          due_count: 87,
          new_capacity_today: 12,
          decks: [],
        },
      }),
    });
  });
}

// ── Anonymous surface ──────────────────────────────────────────────────────

test('anonymous user sees landing page, not the workbench', async ({ page }) => {
  await mockMe(page, 'anon');
  await page.goto('/');

  await expect(page.locator('#landing-page')).toHaveClass(/active/);
  await expect(page.locator('.hero-title')).toContainText(/Learn Finnish/i);
  await expect(page.locator('#admin-workbench-page')).not.toHaveClass(/active/);

  // No admin terminology in the public surface
  await expect(page.locator('#landing-page')).not.toContainText(/Workbench/i);
  await expect(page.locator('#landing-page')).not.toContainText(/parser/i);
});

test('anonymous nav exposes Sign in but not user/admin surfaces', async ({ page }) => {
  await mockMe(page, 'anon');
  await page.goto('/');

  await expect(page.locator('.nav-signin').first()).toBeVisible();
  await expect(page.locator('.nav-user').first()).toBeHidden();
  await expect(page.locator('.nav-admin').first()).toBeHidden();
});

test('about page explains the product', async ({ page }) => {
  await mockMe(page, 'anon');
  await page.goto('/#/about');

  await expect(page.locator('#about-page')).toHaveClass(/active/);
  await expect(page.locator('.about-hero h1')).toContainText('How FinEstDB works');
  await expect(page.locator('.about-steps')).toContainText('Paste a text');
  await expect(page.locator('.about-steps')).toContainText('See your gap');
});

test('anonymous user trying admin workbench is redirected', async ({ page }) => {
  await mockMe(page, 'anon');
  await page.goto('/#/admin/workbench');

  await expect(page.locator('#signin-page')).toHaveClass(/active/);
  await expect(page.locator('#admin-workbench-page')).not.toHaveClass(/active/);
});

test('anonymous user trying inspect is redirected to sign-in', async ({ page }) => {
  await mockMe(page, 'anon');
  await page.goto('/#/inspect');

  await expect(page.locator('#signin-page')).toHaveClass(/active/);
});

// ── Sign-in flow ───────────────────────────────────────────────────────────

test('successful sign-in lands on dashboard', async ({ page }) => {
  // First /api/me is anon so we render the signin form, second is user after login.
  let meCallCount = 0;
  await page.route('**/api/me', async (route) => {
    meCallCount += 1;
    if (meCallCount === 1) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ authenticated: false, user: null }),
      });
    } else {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          authenticated: true,
          user: { id: 1, email: 'alice@example.com', is_admin: false },
          dashboard: { known_count: 1234, due_count: 87, new_capacity_today: 12, decks: [] },
        }),
      });
    }
  });
  await page.route('**/api/auth/login', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        authenticated: true,
        user: { id: 1, email: 'alice@example.com', is_admin: false },
      }),
    });
  });

  await page.goto('/#/signin');
  await page.getByLabel('Email').fill('alice@example.com');
  await page.getByRole('button', { name: 'Sign in' }).click();

  await expect(page.locator('#dashboard-page')).toHaveClass(/active/);
  await expect(page.locator('#stat-known')).toHaveText('1,234');
  await expect(page.locator('#stat-due')).toHaveText('87');
});

// ── User (logged in, not admin) ────────────────────────────────────────────

test('user dashboard shows stats and product nav, no admin links', async ({ page }) => {
  await mockMe(page, 'user');
  await page.goto('/#/dashboard');

  await expect(page.locator('#dashboard-page')).toHaveClass(/active/);
  await expect(page.locator('#stat-known')).toHaveText('1,234');

  // User nav visible
  await expect(page.getByRole('link', { name: 'Inspect' }).first()).toBeVisible();
  await expect(page.getByRole('link', { name: 'Decks' }).first()).toBeVisible();
  await expect(page.getByRole('link', { name: 'Review' }).first()).toBeVisible();

  // Admin nav must NOT be visible to a regular user
  await expect(page.locator('.nav-admin').first()).toBeHidden();
});

test('user inspect flow parses text and shows results with correction entry point', async ({ page }) => {
  await mockMe(page, 'user');
  await page.goto('/#/inspect');

  await expect(page.locator('#inspect-page')).toHaveClass(/active/);
  await page.locator('#inspect-text').fill(mixedFinnishText);
  await page.getByRole('button', { name: 'Inspect text' }).click();

  await expect(page.locator('#results-page')).toHaveClass(/active/);
  // Internal parser-mode terminology must be hidden from non-admin users
  await expect(page.locator('#results-parser')).toHaveText('Your text');
  await expect(page.locator('#coverage-value')).toContainText('%');

  // User has the correction entry point
  await expect(page.locator('.correction-btn').first()).toBeVisible();

  await page.locator('.correction-btn').first().click();
  await expect(page.locator('#correction-modal')).not.toHaveClass(/hidden/);
  await page.getByRole('button', { name: 'Cancel' }).click();
  await expect(page.locator('#correction-modal')).toHaveClass(/hidden/);
});

test('user trying admin workbench is sent to dashboard', async ({ page }) => {
  await mockMe(page, 'user');
  await page.goto('/#/admin/workbench');

  await expect(page.locator('#dashboard-page')).toHaveClass(/active/);
  await expect(page.locator('#admin-workbench-page')).not.toHaveClass(/active/);
});

// ── Admin ──────────────────────────────────────────────────────────────────

test('admin sees workbench cluster in nav', async ({ page }) => {
  await mockMe(page, 'admin');
  await page.goto('/#/dashboard');

  await expect(page.locator('.nav-admin').first()).toBeVisible();
  await expect(page.getByRole('link', { name: 'Workbench' }).first()).toBeVisible();
});

test('admin can open the parser workbench with both parser buttons', async ({ page }) => {
  await mockMe(page, 'admin');
  await page.goto('/#/admin/workbench');

  await expect(page.locator('#admin-workbench-page')).toHaveClass(/active/);
  await expect(page.locator('.admin-banner').first()).toBeVisible();
  await expect(page.getByRole('button', { name: /Custom Parser/ })).toBeVisible();
  await expect(page.getByRole('button', { name: /Basic Parser/ })).toBeVisible();
});

test('admin parse keeps internal parser-mode label', async ({ page }) => {
  await mockMe(page, 'admin');
  await page.goto('/#/admin/workbench');

  await page.locator('#parse-text').fill(mixedFinnishText);
  await page.getByRole('button', { name: /Custom Parser/ }).click();

  await expect(page.locator('#results-page')).toHaveClass(/active/);
  await expect(page.locator('#results-parser')).toHaveText('Custom parser');
});

// ── Language mismatch (still important regression coverage) ────────────────

test('inspect lang mismatch warning blocks parse until switching languages', async ({ page }) => {
  await mockMe(page, 'user');
  await page.goto('/#/inspect');

  await page.locator('#inspect-lang').selectOption('ET');
  await page.locator('#inspect-text').fill('Menin pankkiin tänään ja söin hyvää leipää.');

  await expect(page.locator('#inspect-lang-warning')).toContainText('looks like Finnish');
  await expect(page.locator('#inspect-lang-switch')).toBeVisible();
  await expect(page.locator('#inspect-submit')).toBeDisabled();

  await page.getByRole('button', { name: 'Switch to Finnish' }).click();
  await expect(page.locator('#inspect-lang')).toHaveValue('FI');
  await expect(page.locator('#inspect-lang-warning')).toBeHidden();
  await expect(page.locator('#inspect-submit')).toBeEnabled();
});

// ── Mobile (375 px) ────────────────────────────────────────────────────────

test('mobile landing layout fits at 375 px', async ({ page }) => {
  await mockMe(page, 'anon');
  await page.setViewportSize({ width: 375, height: 800 });
  await page.goto('/');

  await expect(page.locator('.hero-title')).toBeVisible();

  // Hamburger menu opens and shows the right links
  await page.locator('#nav-hamburger').click();
  await expect(page.locator('#nav-mobile-overlay')).toBeVisible();
  await expect(page.getByRole('link', { name: 'Sign in' }).first()).toBeVisible();

  // No admin terminology visible in the mobile menu for anonymous users
  await expect(page.getByRole('link', { name: 'Workbench' })).toBeHidden();
  await expect(page.getByRole('link', { name: 'Feedback' })).toBeHidden();
});
