// First-experience release-candidate pack: Playwright half.
//
// This spec is the browser-driven counterpart to cmd/firstexperiencerc. Both
// consume the single canonical manifest at
// testdata/first-experience-rc/manifest.json (see
// docs/GO_LIVE_CHECKLIST.md "First-experience quality check") so the launch
// gate cannot drift across separate case lists.
//
// One test() is generated per manifest case with automation:"playwright".
// Non-playwright cases (automation:"parser", exercised by the Go runner) get
// a test.skip() stub carrying the manifest's own notes, so the full
// journey x language matrix is visible in `npx playwright test --list`.
// automation:"pending" no longer exists in the manifest — the Go runner
// fails the run on any pending case, so an unautomated journey can never
// hide behind a green `make first-experience-rc`.
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { expect, test, type Page } from '@playwright/test';
import manifest from '../../testdata/first-experience-rc/manifest.json' with { type: 'json' };

interface RCCase {
  id: string;
  language: 'fi' | 'et';
  journey: string;
  automation: 'parser' | 'playwright' | 'manual' | 'pending';
  fixture?: string;
  expect?: { notes?: string; lemma_pos?: Array<{ lemma: string; pos: string }> };
  notes?: string;
}

const cases = manifest.cases as RCCase[];

function pendingReason(c: RCCase): string {
  return c.expect?.notes || c.notes || 'not yet automated';
}

// Fixture text comes from the same testdata/first-experience-rc/*.txt files
// the Go runner reads, loaded at spec load time so the fixtures stay
// single-source (manifest "fixture" field -> file on disk, no inline copies).
const fixtureDir = join(__dirname, '..', '..', 'testdata', 'first-experience-rc');

function fixtureText(c: RCCase): string {
  if (!c.fixture) throw new Error(`first-experience-rc case ${c.id} has no fixture path`);
  return readFileSync(join(fixtureDir, c.fixture), 'utf8').trim();
}

// ── Shared mocks (mirrors web/tests/parse-results.spec.ts conventions) ─────

async function mockSignedInUser(page: Page, lang: 'FI' | 'ET'): Promise<void> {
  await page.route('**/api/me', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        authenticated: true,
        user: { id: 1, email: 'alice@example.com', is_admin: false },
        dashboard: { known_count: 0, due_count: 0, new_capacity_today: 0, decks: [] },
        languages: { learning: ['FI', 'ET'], active: lang, stats: {} },
      }),
    });
  });
}

// ── anonymous-demo: landing paste -> parse -> explore-only results ─────────
//
// Mirrors the journey coverage in web/tests/anonymous-parser-demo.spec.ts
// (anonymous /api/me + /api/parse mocks, explore-only assertions) but drives
// it with the manifest's embedded-text fixture so the RC pack exercises the
// same text the Go runner parses. The ET case additionally flips the landing
// FI/ET selector, since detectLang would otherwise block an Estonian paste
// on the FI-selected form.

interface AnonDemoWords {
  words: Array<{ lemma: string; pos: string; forms: string[]; count: number; gloss: string }>;
}

const anonDemoMockWords: Record<'fi' | 'et', AnonDemoWords> = {
  fi: {
    words: [
      { lemma: 'kissa', pos: 'NOUN', forms: ['Kissa'], count: 2, gloss: 'cat' },
      { lemma: 'aurinko', pos: 'NOUN', forms: ['Aurinko'], count: 1, gloss: 'sun' },
    ],
  },
  et: {
    words: [
      { lemma: 'kass', pos: 'NOUN', forms: ['Kass'], count: 2, gloss: 'cat' },
      { lemma: 'päike', pos: 'NOUN', forms: ['Päike'], count: 1, gloss: 'sun' },
    ],
  },
};

async function runAnonymousDemo(page: Page, lang: 'fi' | 'et', text: string): Promise<void> {
  const upper = lang.toUpperCase() as 'FI' | 'ET';
  const mock = anonDemoMockWords[lang];

  await page.route('**/api/me', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ authenticated: false, user: null, anon_max_chars: 300000 }),
    });
  });

  let parsePayload: any = null;
  await page.route('**/api/parse', async (route) => {
    parsePayload = route.request().postDataJSON();
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        lang: upper,
        parse_id: null, // ephemeral: anonymous parses are never persisted
        total_tokens: 35,
        parse_duration_ms: 9,
        words: mock.words,
      }),
    });
  });

  await page.goto('/');
  await expect(page.locator('#landing-page')).toHaveClass(/active/);
  if (upper === 'ET') {
    await page.locator('#landing-lang button[data-value="ET"]').click();
  }
  await page.locator('#landing-text').fill(text);
  await page.locator('#landing-submit').click();

  // The parse request carries the fixture text and the selected language.
  await expect.poll(() => parsePayload).not.toBeNull();
  expect(parsePayload).toMatchObject({ lang: upper, text });

  await expect(page.locator('#results-page')).toHaveClass(/active/);
  // Non-admin anonymous parser pill is the soft "Your text" label.
  await expect(page.locator('#results-parser')).toHaveText('Your text');
  // Signed-in-only save CTA must NOT be visible to an anonymous visitor.
  await expect(page.locator('.results-deck-cta')).toBeHidden();

  // Explore: the lemma table lives behind the Words tab (Read is default).
  await page.locator('#results-tab-words').click();
  await expect(page.locator('#word-table-body tr')).toHaveCount(mock.words.length);
  for (const w of mock.words) {
    await expect(page.locator('#word-table-body')).toContainText(w.lemma);
  }
  await expect(page.locator('.pos-filter-chip', { hasText: 'Nouns' })).toBeVisible();

  // Signed-in-only controls must NOT be visible to an anonymous visitor.
  await expect(page.locator('.correction-btn')).toHaveCount(0);
  await expect(page.locator('.word-pill-known')).toHaveCount(0);

  // Privacy footer present for anonymous results.
  await expect(page.locator('#anon-privacy-footer')).toBeVisible();
  await expect(page.locator('#anon-privacy-footer')).toContainText(/wasn't saved/i);
}

for (const lang of ['fi', 'et'] as const) {
  const anonCase = cases.find(c => c.journey === 'anonymous-demo' && c.language === lang);
  if (!anonCase) {
    throw new Error(`first-experience-rc manifest is missing a ${lang} anonymous-demo case`);
  }
  if (anonCase.automation === 'playwright') {
    test(`${lang} journey: anonymous demo paste -> parse -> explore`, async ({ page }) => {
      await runAnonymousDemo(page, lang, fixtureText(anonCase));
    });
  } else {
    test.skip(`${lang} journey: anonymous demo paste -> parse -> explore`, () => {
      // eslint-disable-next-line no-console
      console.log(pendingReason(anonCase));
    });
  }
}

// ── deck-save / first-review: own-text Inspect -> Save as deck -> Review ───
//
// Journeys are separate manifest entries because CONTEXT.md defines them as
// separate product steps, but today's UI delivers both in one uninterrupted
// flow (parse -> save -> land on Decks -> Review -> answer first card), so
// one test covers both the *-deck-save and *-first-review manifest cases per
// language. See the manifest notes on fi-first-review / et-first-review.

interface DeckSaveFixture {
  text: string;
  lemma: string;
  gloss: string;
  deckTitle: string;
}

const deckSaveFixtures: Record<'fi' | 'et', DeckSaveFixture> = {
  fi: { text: 'Kissa juoksee.', lemma: 'kissa', gloss: 'cat', deckTitle: 'FI RC deck' },
  et: { text: 'Kass jookseb.', lemma: 'kass', gloss: 'cat', deckTitle: 'ET RC deck' },
};

async function runDeckSaveAndFirstReview(page: Page, lang: 'FI' | 'ET'): Promise<void> {
  const fixture = deckSaveFixtures[lang.toLowerCase() as 'fi' | 'et'];
  let meDecks: Array<{ id: number; title: string; lang: string; known: number; unique: number; due: number }> = [];
  let nextReviewCard: null | Record<string, unknown> = {
    card_id: '42',
    mode: 'sentence',
    deck_counts: [[fixture.deckTitle, '1']],
    front: { type: 'sentence', text: fixture.text },
    back: {
      lemma: fixture.lemma,
      meaning: fixture.gloss,
      grammar: '',
      examples: [{ text: fixture.text, source_deck: fixture.deckTitle }],
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
        languages: { learning: ['FI', 'ET'], active: lang, stats: {} },
      }),
    });
  });

  await page.route('**/api/decks', async (route) => {
    if (route.request().method() !== 'POST') {
      await route.continue();
      return;
    }
    meDecks = [{ id: 7, title: fixture.deckTitle, lang, known: 0, unique: 1, due: 1 }];
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
  await page.locator('#inspect-text').fill(fixture.text);
  await page.getByRole('button', { name: 'Parse text' }).click();

  // deck-save half of the journey
  await expect(page.locator('#results-page')).toHaveClass(/active/);
  await page.getByRole('button', { name: 'Save as deck' }).click();
  await page.locator('#results-deck-title').fill(fixture.deckTitle);
  await page.locator('#results-save-submit').click();

  await expect(page.locator('#decks-page')).toHaveClass(/active/);
  await expect(page.locator('#decks-list')).toContainText(fixture.deckTitle);

  // first-review half of the journey
  await page.getByRole('button', { name: 'Review' }).click();
  await expect(page.locator('#review-page')).toHaveClass(/active/);
  await expect(page.locator('#review-card')).not.toHaveClass(/hidden/);
  // Surface form is the card's primary identity and always shows. For these
  // fixtures the encountered surface normalizes to the lemma, so it appears in
  // the surface line (the lemma line is hidden when surface == lemma).
  await expect(page.locator('#review-card-surface')).toContainText(fixture.lemma);

  await page.getByRole('button', { name: 'Good' }).click();
  await expect(page.locator('#review-empty')).not.toHaveClass(/hidden/);
}

for (const lang of ['fi', 'et'] as const) {
  const deckSaveCase = cases.find(c => c.journey === 'deck-save' && c.language === lang);
  const firstReviewCase = cases.find(c => c.journey === 'first-review' && c.language === lang);
  if (!deckSaveCase || !firstReviewCase) {
    throw new Error(`first-experience-rc manifest is missing a ${lang} deck-save/first-review case`);
  }

  if (deckSaveCase.automation === 'playwright' && firstReviewCase.automation === 'playwright') {
    test(`${lang} journey: own-text inspect -> save deck -> first review`, async ({ page }) => {
      await runDeckSaveAndFirstReview(page, lang.toUpperCase() as 'FI' | 'ET');
    });
  } else {
    test.skip(`${lang} journey: own-text inspect -> save deck -> first review`, () => {
      // eslint-disable-next-line no-console
      console.log(pendingReason(deckSaveCase));
    });
  }
}

// ── known-word-import: paste a word list on /#/vocab -> import ─────────────
//
// Mirrors the mocked-route conventions of parse-results.spec.ts "user can
// import and remove known words", driven by the manifest's known-word-import
// fixture (the raw comma-separated paste) and its expect.lemma_pos block
// (the (lemma, POS) each pasted word must resolve to). The POST is asserted
// to carry exactly the fixture's words, and the refreshed list must render
// every expected lemma with its POS label.

const POS_LABELS: Record<string, string> = { NOUN: 'Noun', VERB: 'Verb' };

async function runKnownWordImport(page: Page, lang: 'fi' | 'et', c: RCCase): Promise<void> {
  const upper = lang.toUpperCase() as 'FI' | 'ET';
  const raw = fixtureText(c);
  const pastedWords = raw.split(',').map(w => w.trim()).filter(Boolean);
  const expected = c.expect?.lemma_pos ?? [];
  if (expected.length === 0) {
    throw new Error(`first-experience-rc case ${c.id} has no expect.lemma_pos to assert against`);
  }

  await mockSignedInUser(page, upper);

  let knownWords: Array<{ lemma: string; pos: string; lang: string }> = [];
  let importPayload: any = null;
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
    importPayload = request.postDataJSON();
    knownWords = expected.map(e => ({ lemma: e.lemma, pos: e.pos, lang: upper }));
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ imported: knownWords, unresolved: [] }),
    });
  });

  await page.goto('/#/vocab');
  await expect(page.locator('#known-words-input')).toBeVisible();

  await page.locator('#known-words-input').fill(raw);
  await page.getByRole('button', { name: 'Import words' }).click();

  // The import request carries the active language and exactly the fixture's
  // pasted words (comma-split, in order).
  await expect.poll(() => importPayload).not.toBeNull();
  expect(importPayload).toMatchObject({ lang: upper, words: pastedWords });

  // Every pasted word resolved to its expected (lemma, POS) pair.
  for (const e of expected) {
    const chip = page.locator('.known-word-chip', { hasText: e.lemma });
    await expect(chip).toBeVisible();
    await expect(chip.locator('.known-word-pos')).toHaveText(POS_LABELS[e.pos] ?? e.pos);
  }
  await expect(page.locator('#known-words-unresolved')).toBeHidden();
}

for (const lang of ['fi', 'et'] as const) {
  const importCase = cases.find(c => c.journey === 'known-word-import' && c.language === lang);
  if (!importCase) {
    throw new Error(`first-experience-rc manifest is missing a ${lang} known-word-import case`);
  }
  if (importCase.automation === 'playwright') {
    test(`${lang} journey: known-word import on /#/vocab`, async ({ page }) => {
      await runKnownWordImport(page, lang, importCase);
    });
  } else {
    test.skip(`${lang} journey: known-word import on /#/vocab`, () => {
      // eslint-disable-next-line no-console
      console.log(pendingReason(importCase));
    });
  }
}

// ── parser-feedback: Inspect -> parse -> Suggest fix -> submit ─────────────
//
// Reuses the /api/parse/feedback contract already covered in
// parse-results.spec.ts ("correction submit posts the PR-53
// /api/parse/feedback contract"), driven against this manifest's
// parser-feedback fixture text per language so the RC pack and the feature
// spec assert the same contract instead of duplicating fixtures.

interface ParserFeedbackFixture {
  lemma: string;
  surface: string;
  grammarLabel: string;
}

const parserFeedbackFixtures: Record<'fi' | 'et', ParserFeedbackFixture> = {
  fi: { lemma: 'laulaa', surface: 'Lauloi', grammarLabel: 'past 3sg' },
  et: { lemma: 'laulma', surface: 'Laulis', grammarLabel: 'past 3sg' },
};

async function runParserFeedbackSubmit(page: Page, lang: 'fi' | 'et', text: string): Promise<void> {
  const upper = lang.toUpperCase() as 'FI' | 'ET';
  const fixture = parserFeedbackFixtures[lang];
  await mockSignedInUser(page, upper);

  await page.route('**/api/parse', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        lang: upper,
        parse_id: 9001,
        total_tokens: 10,
        parse_duration_ms: 8,
        words: [
          { lemma: fixture.lemma, pos: 'VERB', forms: [fixture.surface], count: 1, grammar_label: fixture.grammarLabel },
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
      body: JSON.stringify({ feedback_id: 99, status: 'submitted' }),
    });
  });

  await page.goto('/#/inspect');
  await page.locator('#inspect-text').fill(text);
  await page.getByRole('button', { name: 'Parse text' }).click();
  await expect(page.locator('#results-page')).toHaveClass(/active/);
  // The correction entry point lives in the lemma table (Words tab); the
  // results page now defaults to the Read tab.
  const wordsTab = page.locator('#results-tab-words');
  if (await wordsTab.isVisible()) await wordsTab.click();

  await page.locator('.correction-btn').first().click();
  await expect(page.locator('#correction-modal')).not.toHaveClass(/hidden/);
  await page.locator('#correction-mode-propose').check();
  await page.locator('#correction-proposed-lemma').fill(fixture.lemma);
  await page.locator('#correction-proposed-pos').selectOption('VERB');
  await page.locator('#correction-submit').click();

  await expect.poll(() => captured).not.toBeNull();
  expect(captured).toMatchObject({
    parse_id: 9001,
    lang: upper,
    surface: fixture.surface,
    original_lemma: fixture.lemma,
    original_pos: 'VERB',
    proposed_lemma: fixture.lemma,
    proposed_pos: 'VERB',
  });
  await expect(page.locator('.toast.success')).toContainText(/sent/i);
  await expect(page.locator('#correction-modal')).toHaveClass(/hidden/);
}

for (const lang of ['fi', 'et'] as const) {
  const feedbackCase = cases.find(c => c.journey === 'parser-feedback' && c.language === lang);
  if (!feedbackCase) {
    throw new Error(`first-experience-rc manifest is missing a ${lang} parser-feedback case`);
  }
  if (feedbackCase.automation === 'playwright') {
    test(`${lang} journey: parser feedback correction submit`, async ({ page }) => {
      await runParserFeedbackSubmit(page, lang, fixtureText(feedbackCase));
    });
  } else {
    test.skip(`${lang} journey: parser feedback correction submit`, () => {
      // eslint-disable-next-line no-console
      console.log(pendingReason(feedbackCase));
    });
  }
}

// ── Every other manifest case: explicit pending/manual stub ────────────────
//
// Keeps `npx playwright test --list` showing the full journey x language
// matrix (per docs/GO_LIVE_CHECKLIST.md) instead of only the cases that
// happen to be implemented today.

const coveredIds = new Set([
  'fi-anonymous-demo', 'et-anonymous-demo',
  'fi-deck-save', 'et-deck-save',
  'fi-first-review', 'et-first-review',
  'fi-known-word-import', 'et-known-word-import',
  'fi-parser-feedback', 'et-parser-feedback',
]);

for (const c of cases) {
  if (coveredIds.has(c.id)) continue;
  if (c.automation === 'playwright') {
    throw new Error(`first-experience-rc.spec.ts has no test wired for playwright case ${c.id}; add one or change its automation in the manifest`);
  }
  test.skip(`${c.language} journey: ${c.journey} [${c.id}]`, () => {
    // eslint-disable-next-line no-console
    console.log(pendingReason(c));
  });
}
