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
  await expect(page.locator('.about-steps')).toContainText('Inspect the words');
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

test('user can save inspected results as a deck and review the first due card', async ({ page }) => {
  let meDecks: Array<{ id: number; title: string; lang: string; known: number; unique: number; due: number }> = [];
  let nextReviewCard: null | Record<string, unknown> = {
    card_id: '42',
    mode: 'sentence',
    deck_counts: [['My test deck', '1']],
    front: { type: 'sentence', text: 'Kissa juoksee.' },
    back: {
      lemma: 'kissa',
      meaning: 'cat',
      grammar: '',
      examples: [{ text: 'Kissa juoksee.', source_deck: 'My test deck' }],
    },
  };

  await page.route('**/api/me', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        authenticated: true,
        user: { id: 1, email: 'alice@example.com', is_admin: false },
        dashboard: {
          known_count: 0,
          due_count: nextReviewCard ? 1 : 0,
          new_capacity_today: nextReviewCard ? 1 : 0,
          decks: meDecks,
        },
      }),
    });
  });

  await page.route('**/api/decks', async (route) => {
    if (route.request().method() !== 'POST') {
      await route.continue();
      return;
    }
    meDecks = [{ id: 7, title: 'My test deck', lang: 'FI', known: 0, unique: 1, due: 1 }];
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ deck_id: 7 }),
    });
  });

  await page.route('**/api/review/next**', async (route) => {
    if (!nextReviewCard) {
      await route.fulfill({ status: 204, body: '' });
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(nextReviewCard),
    });
  });

  await page.route('**/api/review/answer', async (route) => {
    nextReviewCard = null;
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ status: 'ok' }),
    });
  });

  await page.goto('/#/inspect');
  await page.locator('#inspect-text').fill('Kissa juoksee.');
  await page.getByRole('button', { name: 'Inspect text' }).click();

  await expect(page.locator('#results-page')).toHaveClass(/active/);
  await page.getByRole('button', { name: 'Save as deck' }).click();
  await page.locator('#results-deck-title').fill('My test deck');
  await page.locator('#results-save-submit').click();

  await expect(page.locator('#decks-page')).toHaveClass(/active/);
  await expect(page.locator('#decks-list')).toContainText('My test deck');

  await page.getByRole('button', { name: 'Review' }).click();
  await expect(page.locator('#review-page')).toHaveClass(/active/);
  await expect(page.locator('#review-card')).not.toHaveClass(/hidden/);
  await expect(page.locator('#review-card-lemma')).toContainText('kissa');

  await page.getByRole('button', { name: 'Good' }).click();
  await expect(page.locator('#review-empty')).not.toHaveClass(/hidden/);
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

// ── Results route is auth-only and signout clears prior parse state ────────

test('anonymous user trying /#/results is redirected to sign-in', async ({ page }) => {
  await mockMe(page, 'anon');
  await page.goto('/#/results');

  await expect(page.locator('#signin-page')).toHaveClass(/active/);
  await expect(page.locator('#results-page')).not.toHaveClass(/active/);
});

test('signing out clears prior parse results from memory and route', async ({ page }) => {
  // Start signed in, parse, then flip /api/me to anon and sign out.
  let authed = true;
  await page.route('**/api/me', async (route) => {
    if (authed) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          authenticated: true,
          user: { id: 1, email: 'alice@example.com', is_admin: false },
          dashboard: { known_count: 1, due_count: 0, new_capacity_today: 0, decks: [] },
        }),
      });
    } else {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ authenticated: false, user: null }),
      });
    }
  });
  await page.route('**/api/auth/logout', async (route) => {
    authed = false;
    await route.fulfill({ status: 200, contentType: 'application/json', body: '{}' });
  });

  await page.goto('/#/inspect');
  await page.locator('#inspect-text').fill(mixedFinnishText);
  await page.getByRole('button', { name: 'Inspect text' }).click();
  await expect(page.locator('#results-page')).toHaveClass(/active/);
  await expect(page.locator('.correction-btn').first()).toBeVisible();

  // Sign out via the desktop nav (anonymous user must not be able to reach #/results).
  await page.locator('#nav-signout').click();
  await expect(page.locator('#landing-page')).toHaveClass(/active/);

  // Try to navigate back to the cached results route — guard must redirect to sign-in
  // and the prior table must not be on screen.
  await page.goto('/#/results');
  await expect(page.locator('#signin-page')).toHaveClass(/active/);
  await expect(page.locator('#results-page')).not.toHaveClass(/active/);
  await expect(page.locator('.correction-btn')).toHaveCount(0);
});

// ── Correction submit: honest UX + PR-53 /api/parse/feedback contract ──────

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
          {
            lemma: 'laulaa',
            pos: 'VERB',
            forms: ['lauloi'],
            count: 1,
            grammar_label: 'past 3sg',
          },
          {
            lemma: 'pankki',
            pos: 'NOUN',
            forms: ['pankkiin'],
            count: 1,
            grammar_label: 'illative sg',
          },
        ],
      }),
    });
  });
}

test('correction submit shows error toast on backend failure', async ({ page }) => {
  await mockMe(page, 'user');
  await mockParseWithId(page, 4242);
  await page.route('**/api/parse/feedback', async (route) => {
    await route.fulfill({ status: 404, contentType: 'application/json', body: '{}' });
  });

  await page.goto('/#/inspect');
  await page.locator('#inspect-text').fill(mixedFinnishText);
  await page.getByRole('button', { name: 'Inspect text' }).click();
  await expect(page.locator('#results-page')).toHaveClass(/active/);

  await page.locator('.correction-btn').first().click();
  await expect(page.locator('#correction-modal')).not.toHaveClass(/hidden/);
  await page.locator('#correction-submit').click();

  // Modal stays open and an error toast surfaces — no fake success.
  await expect(page.locator('.toast.error')).toContainText(/try again/i);
  await expect(page.locator('#correction-modal')).not.toHaveClass(/hidden/);
});

test('correction submit posts the PR-53 /api/parse/feedback contract', async ({ page }) => {
  await mockMe(page, 'user');
  await mockParseWithId(page, 4242);
  let captured: any = null;
  await page.route('**/api/parse/feedback', async (route) => {
    captured = route.request().postDataJSON();
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ feedback_id: 7, status: 'submitted' }),
    });
  });

  await page.goto('/#/inspect');
  await page.locator('#inspect-text').fill(mixedFinnishText);
  await page.getByRole('button', { name: 'Inspect text' }).click();
  await expect(page.locator('#results-page')).toHaveClass(/active/);

  await page.locator('.correction-btn').first().click();
  // Modal pre-fills the proposed_lemma to the original lemma; user can adjust.
  await page.locator('#correction-proposed-lemma').fill('laulaa');
  await page.locator('#correction-proposed-pos').selectOption('VERB');
  await page.locator('#correction-proposed-grammar').fill('past 3sg');
  await page.locator('#correction-note').fill('looks right');
  await page.locator('#correction-submit').click();

  await expect.poll(() => captured).not.toBeNull();
  expect(captured).toMatchObject({
    parse_id:               4242,
    lang:                   'FI',
    parser:                 'custom',
    surface:                'lauloi',
    occurrence:             0,
    original_lemma:         'laulaa',
    original_pos:           'VERB',
    original_grammar_label: 'past 3sg',
    proposed_lemma:         'laulaa',
    proposed_pos:           'VERB',
    proposed_grammar_label: 'past 3sg',
    note:                   'looks right',
  });

  // On success the modal closes and a success toast surfaces.
  await expect(page.locator('.toast.success')).toContainText(/sent/i);
  await expect(page.locator('#correction-modal')).toHaveClass(/hidden/);
});

test('correction modal disables submit when no parse_id is attached', async ({ page }) => {
  await mockMe(page, 'user');
  // No parse_id (simulates anonymous-style parses or pre-PR-53 backend).
  await page.route('**/api/parse', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        lang: 'FI',
        total_tokens: 2,
        parse_duration_ms: 5,
        words: [
          { lemma: 'laulaa', pos: 'VERB', forms: ['lauloi'], count: 1 },
        ],
      }),
    });
  });

  await page.goto('/#/inspect');
  await page.locator('#inspect-text').fill(mixedFinnishText);
  await page.getByRole('button', { name: 'Inspect text' }).click();
  await expect(page.locator('#results-page')).toHaveClass(/active/);

  await page.locator('.correction-btn').first().click();
  await expect(page.locator('#correction-auth-hint')).toBeVisible();
  await expect(page.locator('#correction-submit')).toBeDisabled();
});

// ── Mobile (375 px) ────────────────────────────────────────────────────────

test('mobile keeps the correction entry point visible at 375 px', async ({ page }) => {
  await mockMe(page, 'user');
  await page.setViewportSize({ width: 375, height: 800 });
  await page.goto('/#/inspect');
  await page.locator('#inspect-text').fill(mixedFinnishText);
  await page.getByRole('button', { name: 'Inspect text' }).click();
  await expect(page.locator('#results-page')).toHaveClass(/active/);
  await expect(page.locator('.correction-btn').first()).toBeVisible();
});

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
