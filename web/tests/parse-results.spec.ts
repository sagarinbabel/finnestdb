import { expect, test, type Page } from '@playwright/test';

const mixedFinnishText = 'Viisutubettaja lauloi nopeasti. Menin pankkiin eilen.';

// The results page defaults to the Read tab (the living text); the lemma table
// now lives behind the Words tab. Tests that assert on table rows / correction
// buttons / chapter sidebar must switch to Words first. When there's no source
// text (deck context) the tab bar is hidden and the table shows regardless, so
// this no-ops there.
async function openWordsTab(page: Page): Promise<void> {
  const tab = page.locator('#results-tab-words');
  if (await tab.isVisible()) await tab.click();
}

type Role = 'anon' | 'user' | 'admin';

interface MockMeOptions {
  /** Override the active language. Defaults to 'FI'. */
  activeLanguage?: string;
  /** Override the user's learning languages. Defaults to ['FI','ET']. */
  learningLanguages?: string[];
  /** Per-language vocab stats; absent languages default to zeros on render. */
  stats?: Record<string, { decks: number; known_words: number }>;
  /** Override the dashboard decks list. Defaults to []. */
  decks?: Array<{
    id: number; title: string; lang: string;
    known: number; unique: number; due: number;
    is_public: boolean; is_owner?: boolean; subscribed?: boolean;
  }>;
}

async function mockMe(page: Page, role: Role, opts: MockMeOptions = {}): Promise<void> {
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
    const learning = opts.learningLanguages ?? ['FI', 'ET'];
    const active = opts.activeLanguage ?? 'FI';
    const stats = opts.stats ?? Object.fromEntries(learning.map(l => [l, { decks: 0, known_words: 0 }]));
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
          decks: opts.decks ?? [],
        },
        languages: { learning, active, stats },
      }),
    });
  });
}

// Capture-and-respond mock for PATCH /api/me/languages. The handler echoes
// the patched fields back (merged with defaults) so the frontend's
// applyLanguagesResponse path is exercised, and pushes each request's body
// into `received` for assertions.
async function mockLanguagesPatch(
  page: Page,
  received: Array<{ learning?: string[]; active?: string }>,
  baseline: { learning: string[]; active: string },
): Promise<void> {
  await page.route('**/api/me/languages', async (route) => {
    const body = route.request().postDataJSON() as { learning?: string[]; active?: string };
    received.push(body);
    const learning = body.learning ?? baseline.learning;
    const active = body.active ?? baseline.active;
    const stats = Object.fromEntries(learning.map(l => [l, { decks: 0, known_words: 0 }]));
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ learning, active, stats }),
    });
  });
}

// ── Anonymous surface ──────────────────────────────────────────────────────

test('anonymous user sees landing page, not the workbench', async ({ page }) => {
  await mockMe(page, 'anon');
  await page.goto('/');

  await expect(page.locator('#landing-page')).toHaveClass(/active/);
  await expect(page.locator('.hero-title')).toHaveText('Learn in Context');
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

test('nav logo renders the FinnEst brand name with correct capitalization', async ({ page }) => {
  await mockMe(page, 'anon');
  await page.goto('/');

  // The logo text comes from a ::before pseudo-element in styles.css, so
  // assert on the rendered accessible text rather than the DOM text content.
  const logoText = await page.locator('#nav-logo').evaluate((el) => {
    const before = getComputedStyle(el, '::before').content;
    return before.replace(/^"|"$/g, '');
  });
  expect(logoText).toBe('FinnEst');
});

test('about page explains the product', async ({ page }) => {
  await mockMe(page, 'anon');
  await page.goto('/#/about');

  await expect(page.locator('#about-page')).toHaveClass(/active/);
  await expect(page.locator('.about-hero h1')).toContainText('How FinnEst works');
  await expect(page.locator('.about-steps')).toContainText('Paste a text');
  await expect(page.locator('.about-steps')).toContainText('Parse the words');
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

test('anonymous user trying parse history is redirected to sign-in', async ({ page }) => {
  await mockMe(page, 'anon');
  await page.goto('/#/history');

  await expect(page.locator('#signin-page')).toHaveClass(/active/);
  await expect(page.locator('#history-page')).not.toHaveClass(/active/);
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
  await page.getByLabel('Password').fill('password123');
  await page.getByRole('button', { name: 'Sign in' }).click();

  await expect(page.locator('#dashboard-page')).toHaveClass(/active/);
  await expect(page.locator('#stat-known')).toHaveText('1,234');
  await expect(page.locator('#stat-due')).toHaveText('87');
});

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
            learning_state: 'known',
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

// ── User (logged in, not admin) ────────────────────────────────────────────

test('user dashboard shows stats and product nav, no admin links', async ({ page }) => {
  await mockMe(page, 'user');
  await page.goto('/#/dashboard');

  await expect(page.locator('#dashboard-page')).toHaveClass(/active/);
  await expect(page.locator('#stat-known')).toHaveText('1,234');

  // User nav visible
  await expect(page.getByRole('link', { name: 'Parse' }).first()).toBeVisible();
  await expect(page.getByRole('link', { name: 'History' }).first()).toBeVisible();
  await expect(page.getByRole('link', { name: 'Decks' }).first()).toBeVisible();
  await expect(page.getByRole('link', { name: 'Review' }).first()).toBeVisible();

  // Admin nav must NOT be visible to a regular user
  await expect(page.locator('.nav-admin').first()).toBeHidden();
});

test('user can review and delete retained parse history', async ({ page }) => {
  await mockMe(page, 'user');

  let sessions = [
    {
      id: 11,
      lang: 'FI',
      parser: 'custom',
      title: 'Kissa juoksee nopeasti.',
      source_preview: 'Kissa juoksee nopeasti.',
      total_tokens: 3,
      unique_words: 3,
      deck_count: 1,
      feedback_count: 1,
      created_at: '2026-06-03T12:00:00Z',
    },
    {
      id: 12,
      lang: 'ET',
      parser: 'custom',
      title: 'Koer jookseb kiiresti.',
      source_preview: 'Koer jookseb kiiresti.',
      total_tokens: 3,
      unique_words: 3,
      deck_count: 0,
      feedback_count: 1,
      created_at: '2026-06-03T13:00:00Z',
    },
  ];
  const deleteURLs: string[] = [];

  await page.route('**/api/parse/sessions**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    if (request.method() === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ sessions }),
      });
      return;
    }
    if (request.method() === 'DELETE') {
      deleteURLs.push(url.pathname);
      const match = url.pathname.match(/\/api\/parse\/sessions\/(\d+)$/);
      if (match) {
        const deletedID = Number(match[1]);
        sessions = sessions.filter(session => session.id !== deletedID);
      } else {
        sessions = [];
      }
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ status: 'ok', deleted: 1 }),
      });
      return;
    }
    await route.fallback();
  });

  await page.goto('/#/history');

  await expect(page.locator('#history-page')).toHaveClass(/active/);
  await expect(page.locator('.history-lede')).toContainText('Raw source text is kept for 30 days');
  await expect(page.locator('#history-list')).toContainText('Kissa juoksee nopeasti.');
  await expect(page.locator('#history-list')).toContainText('Koer jookseb kiiresti.');
  await expect(page.locator('#history-list')).toContainText('1 deck');
  await expect(page.locator('#history-list')).toContainText('1 feedback item');

  await page.locator('[data-delete-parse-session="11"]').click();
  await expect(page.locator('#dialog-modal')).not.toHaveClass(/hidden/);
  await expect(page.locator('#dialog-modal-message')).toContainText('Linked decks stay');
  await page.locator('#dialog-modal-cancel').click();
  expect(deleteURLs).toEqual([]);

  await page.locator('[data-delete-parse-session="11"]').click();
  await page.locator('#dialog-modal-confirm').click();
  await expect.poll(() => deleteURLs).toEqual(['/api/parse/sessions/11']);
  await expect(page.locator('#history-list')).not.toContainText('Kissa juoksee nopeasti.');
  await expect(page.locator('#history-list')).toContainText('Koer jookseb kiiresti.');

  await page.locator('#history-delete-all').click();
  await expect(page.locator('#dialog-modal-message')).toContainText('Delete 1 retained parse session');
  await page.locator('#dialog-modal-confirm').click();
  await expect.poll(() => deleteURLs).toEqual(['/api/parse/sessions/11', '/api/parse/sessions']);
  await expect(page.locator('#history-empty')).not.toHaveClass(/hidden/);
  await expect(page.locator('#history-empty')).toContainText('No retained parse sessions.');
});

test('user inspect flow parses text and shows results with correction entry point', async ({ page }) => {
  await mockMe(page, 'user');
  await page.goto('/#/inspect');

  await expect(page.locator('#inspect-page')).toHaveClass(/active/);
  await page.locator('#inspect-text').fill(mixedFinnishText);
  await page.getByRole('button', { name: 'Parse text' }).click();

  await expect(page.locator('#results-page')).toHaveClass(/active/);
  // Internal parser-mode terminology must be hidden from non-admin users
  await expect(page.locator('#results-parser')).toHaveText('Your text');

  await openWordsTab(page);
  await expect(page.locator('#coverage-value')).toContainText('%');

  // User has the correction entry point
  await expect(page.locator('.correction-btn').first()).toBeVisible();

  await page.locator('.correction-btn').first().click();
  await expect(page.locator('#correction-modal')).not.toHaveClass(/hidden/);
  await page.getByRole('button', { name: 'Cancel' }).click();
  await expect(page.locator('#correction-modal')).toHaveClass(/hidden/);
});

test('user can mark result rows known or ignored from inspect results', async ({ page }) => {
  await mockMe(page, 'user');
  await mockParseWithId(page, 2323);

  const captured: Array<Record<string, unknown>> = [];
  await page.route('**/api/lemma-state', async (route) => {
    captured.push(route.request().postDataJSON());
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ status: route.request().postDataJSON().status }),
    });
  });

  await page.goto('/#/inspect');
  await page.locator('#inspect-text').fill(mixedFinnishText);
  await page.getByRole('button', { name: 'Parse text' }).click();
  await expect(page.locator('#results-page')).toHaveClass(/active/);
  await openWordsTab(page);

  const firstRow = page.locator('#word-table-body tr').first();
  await expect(firstRow.locator('.word-pill-known.is-active')).toBeVisible();
  await expect(firstRow.getByRole('button', { name: 'Known' })).toBeVisible();
  await expect(firstRow.getByRole('button', { name: 'Ignore' })).toBeVisible();
  await expect(firstRow.getByRole('button', { name: 'Suggest fix' })).toBeVisible();

  await firstRow.getByRole('button', { name: 'Ignore' }).click();
  await expect(firstRow.locator('.word-icon-ignore.is-active')).toBeVisible();
  await expect(firstRow.locator('.word-pill-known.is-active')).toHaveCount(0);
  await expect(page.locator('.toast.success')).toContainText(/ignored/i);

  const secondRow = page.locator('#word-table-body tr').nth(1);
  await secondRow.getByRole('button', { name: 'Known' }).click();
  await expect(secondRow.locator('.word-pill-known.is-active')).toBeVisible();

  expect(captured).toEqual([
    { lang: 'FI', lemma: 'laulaa', pos: 'VERB', status: 'ignored' },
    { lang: 'FI', lemma: 'pankki', pos: 'NOUN', status: 'known' },
  ]);
});

test('user can save inspected results as a deck and review the first due card', async ({ page }) => {
  let meDecks: Array<{ id: number; title: string; lang: string; known: number; unique: number; due: number }> = [];
  let nextReviewCard: null | Record<string, unknown> = {
    card_id: '42',
    mode: 'sentence',
    deck_counts: [['My test deck', '1']],
    front: { type: 'sentence', text: 'Kissa juoksee.', highlight: 'kissa' },
    back: {
      surface: 'kissa',
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
  await page.getByRole('button', { name: 'Parse text' }).click();

  await expect(page.locator('#results-page')).toHaveClass(/active/);
  await page.getByRole('button', { name: 'Save as deck' }).click();
  await page.locator('#results-deck-title').fill('My test deck');
  await page.locator('#results-save-submit').click();

  await expect(page.locator('#decks-page')).toHaveClass(/active/);
  await expect(page.locator('#decks-list')).toContainText('My test deck');

  await page.getByRole('button', { name: 'Review' }).click();
  await expect(page.locator('#review-page')).toHaveClass(/active/);
  await expect(page.locator('#review-card')).not.toHaveClass(/hidden/);
  // Surface form is the card's primary identity; it always shows.
  await expect(page.locator('#review-card-surface')).toContainText('kissa');

  await page.getByRole('button', { name: 'Good' }).click();
  await expect(page.locator('#review-empty')).not.toHaveClass(/hidden/);
});

// Smart paste titles (owner ask: "What title do we do if people paste in a
// random paragraph? We should display it nicely."). The save modal must
// prefill a derived title from the pasted text's first clause/sentence
// (store.DeriveTitle / the client-side deriveTitle mirror), not a raw
// "Finnish: <first 48 chars>" dump - and the learner must still be able to
// see and accept that suggestion before it becomes the deck's real title.
test('paste a paragraph, save modal prefills a derived title, deck list shows it', async ({ page }) => {
  let meDecks: Array<{ id: number; title: string; lang: string; known: number; unique: number; due: number }> = [];

  await page.route('**/api/me', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        authenticated: true,
        user: { id: 1, email: 'paste-title@example.com', is_admin: false },
        dashboard: { known_count: 0, due_count: 0, new_capacity_today: 0, decks: meDecks },
      }),
    });
  });

  let createdTitle = '';
  await page.route('**/api/decks', async (route) => {
    if (route.request().method() !== 'POST') {
      await route.continue();
      return;
    }
    const body = route.request().postDataJSON() as { title: string; lang: string };
    createdTitle = body.title;
    meDecks = [{ id: 9, title: createdTitle, lang: 'FI', known: 0, unique: 1, due: 1 }];
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ deck_id: 9 }),
    });
  });

  await page.goto('/#/inspect');
  // A raw multi-sentence paste, the exact scenario the owner asked about:
  // no title, just a pasted paragraph. The first sentence is short enough
  // that the derived title should be the full first sentence, unchanged.
  await page.locator('#inspect-text').fill('Kissa juoksee. Se on nopea ja iloinen eläin.');
  await page.getByRole('button', { name: 'Parse text' }).click();
  await expect(page.locator('#results-page')).toHaveClass(/active/);

  await page.getByRole('button', { name: 'Save as deck' }).click();
  // The suggestion is prefilled BEFORE saving, and is still an editable
  // input the learner could change - this assertion is the core contract.
  await expect(page.locator('#results-deck-title')).toHaveValue('Kissa juoksee.');

  await page.locator('#results-save-submit').click();
  await expect(page.locator('#decks-page')).toHaveClass(/active/);
  expect(createdTitle).toBe('Kissa juoksee.');
  await expect(page.locator('#decks-list')).toContainText('Kissa juoksee.');
});

// Starter decks (e.g. Top-1000 frequency lists) attach no source sentence to
// their cards. The API reports mode "word" with empty front text for these;
// the review page must not show the "Sentence card" chip or an empty
// front-text line above a surface heading that just repeats it.
test('review card with no source sentence renders as a plain word card', async ({ page }) => {
  const wordOnlyCard = {
    card_id: '99',
    mode: 'word',
    deck_counts: [['Top 1000 Finnish words', '1']],
    front: { type: 'word', text: '', highlight: 'ase' },
    back: {
      surface: 'ase',
      lemma: 'ase',
      meaning: 'weapon (also figuratively)',
      grammar: '',
      examples: [],
    },
  };

  await page.route('**/api/me', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        authenticated: true,
        user: { id: 1, email: 'alice@example.com', is_admin: false },
        dashboard: { known_count: 0, due_count: 0, new_capacity_today: 1, decks: [] },
      }),
    });
  });
  await page.route('**/api/review/next**', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(wordOnlyCard),
    });
  });

  await page.goto('/#/review');
  await expect(page.locator('#review-page')).toHaveClass(/active/);
  await expect(page.locator('#review-card')).not.toHaveClass(/hidden/);

  // No "Sentence card" chip and no front-text line for a card with no example.
  await expect(page.locator('#review-card-front')).toHaveClass(/hidden/);
  await expect(page.locator('#review-card-mode')).toHaveText('');
  await expect(page.locator('#review-card-front-text')).toHaveText('');

  // The card still reads cleanly: surface, lemma/gloss, deck tag.
  await expect(page.locator('#review-card-surface')).toContainText('ase');
  await expect(page.locator('#review-card-meaning')).toContainText('weapon');
  await expect(page.locator('#review-card-decks')).toContainText('Top 1000 Finnish words');
  await expect(page.locator('#review-card-example')).toHaveClass(/hidden/);
});

test('user can import and remove known words', async ({ page }) => {
  let knownWords = [{ lemma: `kissa "oma"`, pos: 'NOUN', lang: 'FI' }];
  let importPayload: any = null;
  let deletedUrl = '';

  await mockMe(page, 'user');
  await page.route('**/api/known-words**', async (route) => {
    const request = route.request();
    if (request.method() === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ known_words: knownWords }),
      });
      return;
    }
    if (request.method() === 'POST') {
      importPayload = request.postDataJSON();
      knownWords = [
        { lemma: `kissa "oma"`, pos: 'NOUN', lang: 'FI' },
        { lemma: 'juosta', pos: 'VERB', lang: 'FI' },
      ];
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          imported: [{ lemma: 'juosta', pos: 'VERB', lang: 'FI' }],
          unresolved: ['mysteeri'],
        }),
      });
      return;
    }
    deletedUrl = request.url();
    knownWords = knownWords.filter(word => word.lemma !== 'juosta');
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ status: 'ok' }),
    });
  });

  // The known-words panel lives on /vocab now (moved from /decks during the
  // Vocab-page extraction). The flow is otherwise identical.
  await page.goto('/#/vocab');

  await expect(page.locator('#known-words-list')).toContainText('kissa "oma"');
  await expect(page.getByRole('button', { name: 'Remove kissa "oma"' })).toBeVisible();
  await page.locator('#known-words-input').fill('juoksen, mysteeri');
  await page.getByRole('button', { name: 'Import words' }).click();

  await expect.poll(() => importPayload).not.toBeNull();
  expect(importPayload).toMatchObject({ lang: 'FI', words: ['juoksen', 'mysteeri'] });
  await expect(page.locator('#known-words-list')).toContainText('juosta');
  await expect(page.locator('#known-words-unresolved')).toContainText('mysteeri');

  await page.getByRole('button', { name: 'Remove juosta' }).click();
  await expect.poll(() => deletedUrl).toContain('lemma=juosta');
  await expect(page.locator('#known-words-list')).not.toContainText('juosta');
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

test('admin can review parser feedback queue', async ({ page }) => {
  await mockMe(page, 'admin');
  let statusFilter = '';
  let reviewed: any = null;

  await page.route('**/api/admin/parse-feedback**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    if (request.method() === 'GET') {
      statusFilter = url.searchParams.get('status') || '';
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          feedback: [
            {
              id: 7,
              parse_session_id: 42,
              user_id: 3,
              lang: 'FI',
              parser: 'custom',
              surface: 'lauloi',
              occurrence: 0,
              original_lemma: 'laulaa',
              original_pos: 'VERB',
              original_grammar_label: 'past 3sg',
              proposed_lemma: 'laulaa',
              proposed_pos: 'VERB',
              proposed_grammar_label: 'past 3sg',
              note: 'Looks right',
              status: statusFilter || 'submitted',
              review_note: `needs "quote" check`,
              created_at: '2026-05-01T12:00:00Z',
            },
          ],
        }),
      });
      return;
    }

    reviewed = {
      id: url.searchParams.get('id'),
      body: request.postDataJSON(),
    };
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ status: reviewed.body.status }),
    });
  });

  await page.goto('/#/admin/feedback');

  await expect(page.locator('#admin-feedback-page')).toHaveClass(/active/);
  await expect(page.locator('#admin-feedback-list')).toContainText('lauloi');
  await expect(page.locator('#admin-feedback-list')).toContainText('laulaa / VERB');
  await expect(page.locator('[data-review-note="7"]')).toHaveValue('needs "quote" check');
  expect(statusFilter).toBe('submitted');

  await page.locator('[data-review-note="7"]').fill('accepted into gold queue');
  await page.getByRole('button', { name: 'Accept' }).click();

  await expect.poll(() => reviewed).not.toBeNull();
  expect(reviewed).toMatchObject({
    id: '7',
    body: { status: 'accepted', review_note: 'accepted into gold queue' },
  });
});

// ── Language mismatch (regression coverage for the warn-and-switch flow) ───

test('inspect lang mismatch warning blocks parse until switching site language', async ({ page }) => {
  // Active language is Estonian. Pasting Finnish text should surface the
  // warning, gate the Parse button, and offer a Switch-to-Finnish button
  // that flips the site's active language via PATCH /api/me/languages.
  await mockMe(page, 'user', { activeLanguage: 'ET' });
  const patchCalls: Array<{ learning?: string[]; active?: string }> = [];
  await mockLanguagesPatch(page, patchCalls, { learning: ['FI', 'ET'], active: 'ET' });
  await page.goto('/#/inspect');

  await page.locator('#inspect-text').fill('Menin pankkiin tänään ja söin hyvää leipää.');

  await expect(page.locator('#inspect-lang-warning')).toContainText('looks like Finnish');
  await expect(page.locator('#inspect-lang-switch')).toBeVisible();
  await expect(page.locator('#inspect-submit')).toBeDisabled();

  await page.getByRole('button', { name: 'Switch to Finnish' }).click();

  // PATCH was hit with active=FI, warning cleared, submit re-enabled.
  await expect.poll(() => patchCalls).toContainEqual(expect.objectContaining({ active: 'FI' }));
  await expect(page.locator('#inspect-lang-warning')).toBeHidden();
  await expect(page.locator('#inspect-submit')).toBeEnabled();
});

test('inspect does not auto-switch the site language on paste - it warns instead', async ({ page }) => {
  // Inspect must never silently flip the user's site-wide language behind
  // their back. Pasting the other language surfaces the warning + switch
  // button and disables Parse until the user explicitly confirms.
  await mockMe(page, 'user', { activeLanguage: 'ET' });
  const patchCalls: Array<unknown> = [];
  await page.route('**/api/me/languages', async (route) => {
    patchCalls.push(route.request().postDataJSON());
    await route.fulfill({ status: 200, contentType: 'application/json', body: '{}' });
  });
  await page.goto('/#/inspect');

  const finnish = 'Menin pankkiin tänään ja söin hyvää leipää.';
  await page.locator('#inspect-text').evaluate((el, value) => {
    const input = el as HTMLTextAreaElement;
    input.value = value as string;
    input.dispatchEvent(new Event('paste', { bubbles: true }));
  }, finnish);

  await expect(page.locator('#inspect-lang-warning')).toContainText('looks like Finnish');
  await expect(page.locator('#inspect-lang-switch')).toBeVisible();
  await expect(page.locator('#inspect-submit')).toBeDisabled();
  // No silent PATCH issued.
  expect(patchCalls).toEqual([]);
});

test('inspect does not auto-switch the site language on file load - it warns instead', async ({ page }) => {
  await mockMe(page, 'user', { activeLanguage: 'ET' });
  const patchCalls: Array<unknown> = [];
  await page.route('**/api/me/languages', async (route) => {
    patchCalls.push(route.request().postDataJSON());
    await route.fulfill({ status: 200, contentType: 'application/json', body: '{}' });
  });
  await page.goto('/#/inspect');

  await page.locator('#inspect-file').setInputFiles({
    name: 'finnish.txt',
    mimeType: 'text/plain',
    buffer: Buffer.from('Menin pankkiin tänään ja söin hyvää leipää.'),
  });

  await expect(page.locator('#inspect-lang-warning')).toContainText('looks like Finnish');
  await expect(page.locator('#inspect-lang-switch')).toBeVisible();
  await expect(page.locator('#inspect-submit')).toBeDisabled();
  expect(patchCalls).toEqual([]);
});

// ── Anonymous results demo + signout clears prior parse state ──────────────

// The anonymous parser demo makes /results reachable without sign-in (it renders
// the demo parse), but an anonymous visitor hitting /#/results with no cached
// parse has nothing to show - the guard sends them to the landing parse form,
// NOT to sign-in, since anonymous parsing is a first-class public surface.
test('anonymous user hitting /#/results with no parse lands on the landing form', async ({ page }) => {
  await mockMe(page, 'anon');
  await page.goto('/#/results');

  await expect(page.locator('#landing-page')).toHaveClass(/active/);
  await expect(page.locator('#landing-form')).toBeVisible();
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
  await page.getByRole('button', { name: 'Parse text' }).click();
  await expect(page.locator('#results-page')).toHaveClass(/active/);
  await openWordsTab(page);
  await expect(page.locator('.correction-btn').first()).toBeVisible();

  // Sign out via the desktop nav. Signout clears the cached parse (sessionStorage
  // + in-memory state) so the prior signed-in results can't leak to the now-anon
  // session.
  await page.locator('#nav-signout').click();
  await expect(page.locator('#landing-page')).toHaveClass(/active/);

  // Navigating back to the cached results route: with the parse cleared, the
  // anonymous visitor has nothing to restore, so the guard sends them to the
  // landing form - and the prior signed-in table (with correction buttons) is
  // gone.
  await page.goto('/#/results');
  await expect(page.locator('#landing-page')).toHaveClass(/active/);
  await expect(page.locator('#results-page')).not.toHaveClass(/active/);
  await expect(page.locator('.correction-btn')).toHaveCount(0);
});

// ── Correction submit: honest UX + PR-53 /api/parse/feedback contract ──────

test('correction submit shows error toast on backend failure', async ({ page }) => {
  await mockMe(page, 'user');
  await mockParseWithId(page, 4242);
  await page.route('**/api/parse/feedback', async (route) => {
    await route.fulfill({ status: 404, contentType: 'application/json', body: '{}' });
  });

  await page.goto('/#/inspect');
  await page.locator('#inspect-text').fill(mixedFinnishText);
  await page.getByRole('button', { name: 'Parse text' }).click();
  await expect(page.locator('#results-page')).toHaveClass(/active/);
  await openWordsTab(page);

  await page.locator('.correction-btn').first().click();
  await expect(page.locator('#correction-modal')).not.toHaveClass(/hidden/);
  await page.locator('#correction-submit').click();

  // Modal stays open and an error toast surfaces - no fake success.
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
  await page.getByRole('button', { name: 'Parse text' }).click();
  await expect(page.locator('#results-page')).toHaveClass(/active/);
  await openWordsTab(page);

  await page.locator('.correction-btn').first().click();
  // Switch to the "propose a fix" path so the proposed fields are sent.
  await page.locator('#correction-mode-propose').check();
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
    flag_only:              false,
    proposed_lemma:         'laulaa',
    proposed_pos:           'VERB',
    proposed_grammar_label: 'past 3sg',
    note:                   'looks right',
  });

  // On success the modal closes and a success toast surfaces.
  await expect(page.locator('.toast.success')).toContainText(/sent/i);
  await expect(page.locator('#correction-modal')).toHaveClass(/hidden/);
});

test('flag-only path submits without a proposed correction', async ({ page }) => {
  await mockMe(page, 'user');
  await mockParseWithId(page, 4242);
  let captured: any = null;
  await page.route('**/api/parse/feedback', async (route) => {
    captured = route.request().postDataJSON();
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ feedback_id: 9, status: 'submitted' }),
    });
  });

  await page.goto('/#/inspect');
  await page.locator('#inspect-text').fill(mixedFinnishText);
  await page.getByRole('button', { name: 'Parse text' }).click();
  await expect(page.locator('#results-page')).toHaveClass(/active/);
  await openWordsTab(page);

  await page.locator('.correction-btn').first().click();
  await expect(page.locator('#correction-modal')).not.toHaveClass(/hidden/);

  // Flag-only is the default path: the proposed-analysis fields are hidden and
  // the flag-only radio is selected.
  await expect(page.locator('#correction-mode-flag')).toBeChecked();
  await expect(page.locator('#correction-proposed-fields')).toBeHidden();

  await page.locator('#correction-submit').click();

  await expect.poll(() => captured).not.toBeNull();
  expect(captured).toMatchObject({
    flag_only:      true,
    surface:        'lauloi',
    proposed_lemma: '',
    proposed_pos:   '',
  });
  await expect(page.locator('.toast.success')).toContainText(/flagged/i);
  await expect(page.locator('#correction-modal')).toHaveClass(/hidden/);

  // Switching to the propose path reveals the proposed-analysis fields.
  await page.locator('.correction-btn').first().click();
  await page.locator('#correction-mode-propose').check();
  await expect(page.locator('#correction-proposed-fields')).toBeVisible();
});

test('correction submit without parse_id sends source text for lazy attribution', async ({ page }) => {
  await mockMe(page, 'user');
  // No parse_id: current /api/parse results are ephemeral. Signed-in users
  // can still submit feedback by sending enough source context for the
  // backend to create a parse_session lazily at feedback-submit time.
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
  let captured: any = null;
  await page.route('**/api/parse/feedback', async (route) => {
    captured = route.request().postDataJSON();
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ feedback_id: 8, status: 'submitted' }),
    });
  });

  await page.goto('/#/inspect');
  await page.locator('#inspect-text').fill(mixedFinnishText);
  await page.getByRole('button', { name: 'Parse text' }).click();
  await expect(page.locator('#results-page')).toHaveClass(/active/);
  await openWordsTab(page);

  await page.locator('.correction-btn').first().click();
  await expect(page.locator('#correction-auth-hint')).toBeHidden();
  await expect(page.locator('#correction-submit')).toBeEnabled();
  await page.locator('#correction-mode-propose').check();
  await page.locator('#correction-proposed-lemma').fill('laulaa');
  await page.locator('#correction-proposed-pos').selectOption('VERB');
  await page.locator('#correction-submit').click();

  await expect.poll(() => captured).not.toBeNull();
  expect(captured).not.toHaveProperty('parse_id');
  expect(captured).toMatchObject({
    lang:               'FI',
    parser:             'custom',
    source_text:        mixedFinnishText,
    total_tokens:       2,
    unique_lemma_count: 1,
    surface:            'lauloi',
    original_lemma:     'laulaa',
    original_pos:       'VERB',
    flag_only:          false,
    proposed_lemma:     'laulaa',
    proposed_pos:       'VERB',
  });
  await expect(page.locator('.toast.success')).toContainText(/sent/i);
  await expect(page.locator('#correction-modal')).toHaveClass(/hidden/);
});

// ── Mobile (375 px) ────────────────────────────────────────────────────────

test('mobile keeps the correction entry point visible at 375 px', async ({ page }) => {
  await mockMe(page, 'user');
  await page.setViewportSize({ width: 375, height: 800 });
  await page.goto('/#/inspect');
  await page.locator('#inspect-text').fill(mixedFinnishText);
  await page.getByRole('button', { name: 'Parse text' }).click();
  await expect(page.locator('#results-page')).toHaveClass(/active/);
  await openWordsTab(page);
  await expect(page.locator('.correction-btn').first()).toBeVisible();
});

// ── EPUB chapters payload: one /api/parse per upload ─────────────────────

test('epub upload sends chapters payload and chapter clicks reuse the cache', async ({ page }) => {
  await mockMe(page, 'user');

  // /api/import/extract: hand back a tiny 2-chapter book with all metadata
  // shaped the way the real backend produces it.
  await page.route('**/api/import/extract', async (route) => {
    await route.fulfill({
      status:      200,
      contentType: 'application/json',
      body: JSON.stringify({
        text:        'Kissa juoksee.\n\nKoira haukkuu.',
        filename:    'tiny.epub',
        char_count:  31,
        truncated:   false,
        book_title:  'Tiny',
        book_author: 'Test',
        chapters: [
          { title: 'Cat',  text: 'Kissa juoksee.', char_count: 14 },
          { title: 'Dog',  text: 'Koira haukkuu.', char_count: 14 },
        ],
      }),
    });
  });

  // Capture every body that POSTs to /api/parse so we can assert call count
  // and the chapters-shape of the body.
  const parseBodies: any[] = [];
  await page.route('**/api/parse', async (route) => {
    parseBodies.push(JSON.parse(route.request().postData() || '{}'));
    await route.fulfill({
      status:      200,
      contentType: 'application/json',
      body: JSON.stringify({
        lang:              'ET',
        total_tokens:      4,
        parse_duration_ms: 7,
        words: [
          { lemma: 'kissa', pos: 'NOUN', forms: ['Kissa'], count: 1 },
          { lemma: 'juosta', pos: 'VERB', forms: ['juoksee'], count: 1 },
          { lemma: 'koira', pos: 'NOUN', forms: ['Koira'], count: 1 },
          { lemma: 'haukkua', pos: 'VERB', forms: ['haukkuu'], count: 1 },
        ],
        chapters: [
          {
            title:             'Cat',
            char_count:        14,
            sentence_count:    1,
            token_count:       2,
            resolved_tokens:   2,
            unresolved_tokens: 0,
            lemma_count:       2,
            words: [
              { lemma: 'kissa',  pos: 'NOUN', forms: ['Kissa'],   count: 1 },
              { lemma: 'juosta', pos: 'VERB', forms: ['juoksee'], count: 1 },
            ],
          },
          {
            title:             'Dog',
            char_count:        14,
            sentence_count:    1,
            token_count:       2,
            resolved_tokens:   2,
            unresolved_tokens: 0,
            lemma_count:       2,
            words: [
              { lemma: 'koira',   pos: 'NOUN', forms: ['Koira'],   count: 1 },
              { lemma: 'haukkua', pos: 'VERB', forms: ['haukkuu'], count: 1 },
            ],
          },
        ],
      }),
    });
  });

  await page.goto('/#/inspect');

  // Drive the hidden file input directly - playwright's file_chooser is the
  // supported way; the dropzone is just visual UX over this input.
  await page.locator('#inspect-file').setInputFiles({
    name: 'tiny.epub',
    mimeType: 'application/epub+zip',
    buffer: Buffer.from('placeholder'), // server route stub doesn't read it
  });

  // Pill should render with the book metadata from the extract response.
  await expect(page.locator('.loaded-filename').first()).toHaveText('Tiny');
  await page.getByRole('button', { name: 'Parse text' }).click();
  await expect(page.locator('#results-page')).toHaveClass(/active/);
  await openWordsTab(page);

  // EXACTLY one /api/parse call so far, with chapters body (not text).
  expect(parseBodies).toHaveLength(1);
  expect(parseBodies[0]).toHaveProperty('chapters');
  expect(parseBodies[0].chapters).toHaveLength(2);
  expect(parseBodies[0]).not.toHaveProperty('text');

  // Click chapter 1 (Dog) in the sidebar. Cache was pre-seeded from
  // response.chapters[], so this must NOT fire another /api/parse.
  await page.locator('[data-chapter-idx="1"]').click();
  await page.waitForTimeout(150); // let any errant fetch settle
  expect(parseBodies).toHaveLength(1);
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

// ── Top-bar language dropdown ──────────────────────────────────────────────

test('nav language dropdown opens and lists the user\'s learning languages', async ({ page }) => {
  await mockMe(page, 'user', { activeLanguage: 'FI', learningLanguages: ['FI', 'ET'] });
  await page.goto('/#/dashboard');

  // Closed state: just the trigger, menu hidden.
  await expect(page.locator('#nav-language-menu')).toHaveClass(/hidden/);
  await expect(page.locator('#nav-language-toggle')).toHaveAttribute('aria-expanded', 'false');

  await page.locator('#nav-language-toggle').click();

  // Open state: menu visible, one option per learning language plus "More…".
  await expect(page.locator('#nav-language-menu')).not.toHaveClass(/hidden/);
  await expect(page.locator('#nav-language-toggle')).toHaveAttribute('aria-expanded', 'true');
  await expect(page.locator('#nav-language-menu .nav-lang-option[data-lang]')).toHaveCount(2);
  await expect(page.locator('#nav-language-menu [data-lang="FI"]')).toHaveAttribute('aria-selected', 'true');
  await expect(page.locator('#nav-language-menu [data-lang="ET"]')).toHaveAttribute('aria-selected', 'false');
  await expect(page.locator('#nav-language-menu [data-manage="1"]')).toContainText('More');
});

test('nav language dropdown is anon-hidden and not anchored to a removed select', async ({ page }) => {
  // Regression guard: the dropdown shouldn't render for anonymous users
  // (they have no `learning_languages`). Belt-and-suspenders: also assert
  // the legacy `<select id="nav-language">` is gone.
  await mockMe(page, 'anon');
  await page.goto('/');
  await expect(page.locator('select#nav-language')).toHaveCount(0);
  await expect(page.locator('.nav-lang-dropdown')).toHaveClass(/role-hidden/);
});

test('selecting a language from the dropdown PATCHes the API and re-renders', async ({ page }) => {
  await mockMe(page, 'user', { activeLanguage: 'FI', learningLanguages: ['FI', 'ET'] });
  const patchCalls: Array<{ learning?: string[]; active?: string }> = [];
  await mockLanguagesPatch(page, patchCalls, { learning: ['FI', 'ET'], active: 'FI' });
  await page.goto('/#/dashboard');

  await page.locator('#nav-language-toggle').click();
  await page.locator('#nav-language-menu [data-lang="ET"]').click();

  // Server got the PATCH with active=ET; menu closes; the option marked
  // selected updates after the response merges into state.
  await expect.poll(() => patchCalls).toContainEqual({ active: 'ET' });
  await expect(page.locator('#nav-language-menu')).toHaveClass(/hidden/);

  // Re-open and confirm ET is now the selected option.
  await page.locator('#nav-language-toggle').click();
  await expect(page.locator('#nav-language-menu [data-lang="ET"]')).toHaveAttribute('aria-selected', 'true');
});

test('"More" in the dropdown routes to the Languages page', async ({ page }) => {
  await mockMe(page, 'user');
  await page.goto('/#/dashboard');

  await page.locator('#nav-language-toggle').click();
  await page.locator('#nav-language-menu [data-manage="1"]').click();

  await expect(page).toHaveURL(/#\/languages$/);
  await expect(page.locator('#languages-page')).toHaveClass(/active/);
});

// ── Languages page ─────────────────────────────────────────────────────────

test('languages page splits Studying and Other, sorted alphabetically with stats', async ({ page }) => {
  // Only FI is in learning; ET is not. Provide stats so the row copy is
  // checkable. Section headings are present in this exact order: Studying
  // first, then Other languages.
  await mockMe(page, 'user', {
    activeLanguage: 'FI',
    learningLanguages: ['FI'],
    stats: { FI: { decks: 3, known_words: 142 }, ET: { decks: 0, known_words: 0 } },
  });
  await page.goto('/#/languages');

  await expect(page.locator('#languages-page')).toHaveClass(/active/);
  // Scoped to the languages list: the static Account section below the list
  // reuses the same heading style but is not part of the Studying/Other split.
  const headings = page.locator('#languages-list .languages-section-heading');
  await expect(headings).toHaveCount(2);
  await expect(headings.nth(0)).toHaveText('Studying');
  await expect(headings.nth(1)).toHaveText('Other languages');

  // Studying section: Finnish row only, with stats line, Active badge,
  // and a disabled Remove (only one language).
  const studying = page.locator('.language-row').filter({ hasText: 'Finnish' });
  await expect(studying).toHaveCount(1);
  await expect(studying).toContainText('3 decks · 142 known words');
  await expect(studying.locator('.language-row-active-badge')).toHaveText('Active');
  await expect(studying.getByRole('button', { name: 'Remove' })).toBeDisabled();

  // Other section: Estonian row with an Add button.
  const other = page.locator('.language-row').filter({ hasText: 'Estonian' });
  await expect(other).toHaveCount(1);
  await expect(other.getByRole('button', { name: 'Add' })).toBeEnabled();
});

test('languages page Add adds the language to the studying list', async ({ page }) => {
  await mockMe(page, 'user', { activeLanguage: 'FI', learningLanguages: ['FI'] });
  const patchCalls: Array<{ learning?: string[]; active?: string }> = [];
  await mockLanguagesPatch(page, patchCalls, { learning: ['FI'], active: 'FI' });
  await page.goto('/#/languages');

  const estonianRow = page.locator('.language-row').filter({ hasText: 'Estonian' });
  await estonianRow.getByRole('button', { name: 'Add' }).click();

  await expect.poll(() => patchCalls).toContainEqual(expect.objectContaining({
    learning: ['FI', 'ET'],
    active: 'FI',
  }));
  // After Add, Estonian should now appear in the Studying section (above
  // the Other heading) and offer Remove + Set active instead of Add.
  const studyingNow = page.locator('.language-row').filter({ hasText: 'Estonian' });
  await expect(studyingNow.getByRole('button', { name: 'Remove' })).toBeEnabled();
  await expect(studyingNow.getByRole('button', { name: 'Set active' })).toBeVisible();
});

test('languages page Remove disabled when only one language remains', async ({ page }) => {
  // Mirrors "at least one language required" - the Remove button on the
  // user's only language is disabled, with the explanation in a tooltip.
  await mockMe(page, 'user', { activeLanguage: 'FI', learningLanguages: ['FI'] });
  await page.goto('/#/languages');

  const removeBtn = page.locator('.language-row').filter({ hasText: 'Finnish' }).getByRole('button', { name: 'Remove' });
  await expect(removeBtn).toBeDisabled();
  await expect(removeBtn).toHaveAttribute('data-tooltip', /at least one/i);
});

test('languages page Set active flips the active language', async ({ page }) => {
  await mockMe(page, 'user', { activeLanguage: 'FI', learningLanguages: ['FI', 'ET'] });
  const patchCalls: Array<{ learning?: string[]; active?: string }> = [];
  await mockLanguagesPatch(page, patchCalls, { learning: ['FI', 'ET'], active: 'FI' });
  await page.goto('/#/languages');

  const estonianRow = page.locator('.language-row').filter({ hasText: 'Estonian' });
  await estonianRow.getByRole('button', { name: 'Set active' }).click();

  await expect.poll(() => patchCalls).toContainEqual(expect.objectContaining({ active: 'ET' }));
  // Active badge moves to Estonian; Set active disappears from the active row.
  await expect(estonianRow.locator('.language-row-active-badge')).toHaveText('Active');
  await expect(estonianRow.getByRole('button', { name: 'Set active' })).toHaveCount(0);
});

// ── Active-language drives surfaces (decks + inspect lede) ─────────────────

test('decks list filters to the active language', async ({ page }) => {
  const decks = [
    { id: 1, title: 'FI deck',  lang: 'FI', known: 0, unique: 10, due: 0, is_public: false },
    { id: 2, title: 'ET deck',  lang: 'ET', known: 0, unique: 20, due: 0, is_public: false },
    { id: 3, title: 'FI deck2', lang: 'FI', known: 0, unique: 30, due: 0, is_public: false },
  ];
  await mockMe(page, 'user', { activeLanguage: 'FI', learningLanguages: ['FI', 'ET'], decks });
  await page.route('**/api/known-words**', async (route) => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: '{"known_words":[]}' });
  });
  await page.goto('/#/decks');

  // Only the two FI decks render; the ET deck is filtered out.
  await expect(page.locator('#decks-list .deck-list-item')).toHaveCount(2);
  await expect(page.locator('#decks-list')).toContainText('FI deck');
  await expect(page.locator('#decks-list')).toContainText('FI deck2');
  await expect(page.locator('#decks-list')).not.toContainText('ET deck');
});

test('inspect lede references the active language by name', async ({ page }) => {
  await mockMe(page, 'user', { activeLanguage: 'ET', learningLanguages: ['FI', 'ET'] });
  await page.goto('/#/inspect');

  await expect(page.locator('#inspect-lede')).toContainText(/Paste Estonian text/);
  // And: the legacy per-page radio is gone.
  await expect(page.locator('#inspect-lang')).toHaveCount(0);
});
