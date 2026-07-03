// First-experience release-candidate pack: Playwright half.
//
// This spec is the browser-driven counterpart to cmd/firstexperiencerc. Both
// consume the single canonical manifest at
// testdata/first-experience-rc/manifest.json (see
// docs/GO_LIVE_CHECKLIST.md "First-experience quality check") so the launch
// gate cannot drift across separate case lists.
//
// One test() is generated per manifest case with automation:"playwright".
// Cases with automation:"pending" (or any other non-playwright value) get a
// test.skip() stub carrying the manifest's own pending reason, so the full
// journey x language matrix is visible in `npx playwright test --list`
// even before every flow is implemented.
import { expect, test, type Page } from '@playwright/test';
import manifest from '../../testdata/first-experience-rc/manifest.json' with { type: 'json' };

interface RCCase {
  id: string;
  language: 'fi' | 'et';
  journey: string;
  automation: 'parser' | 'playwright' | 'manual' | 'pending';
  fixture?: string;
  expect?: { notes?: string };
  notes?: string;
}

const cases = manifest.cases as RCCase[];

function pendingReason(c: RCCase): string {
  return c.expect?.notes || c.notes || 'not yet automated';
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
  await expect(page.locator('#review-card-lemma')).toContainText(fixture.lemma);

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

// ── parser-feedback: Inspect -> parse -> Suggest fix -> submit ─────────────
//
// Reuses the /api/parse/feedback contract already covered in
// parse-results.spec.ts ("correction submit posts the PR-53
// /api/parse/feedback contract"), driven against this manifest's FI
// parser-feedback fixture text so the RC pack and the feature spec assert
// the same contract instead of duplicating fixtures.

const fiParserFeedbackCase = cases.find(c => c.journey === 'parser-feedback' && c.language === 'fi');
if (!fiParserFeedbackCase) {
  throw new Error('first-experience-rc manifest is missing an fi parser-feedback case');
}

if (fiParserFeedbackCase.automation === 'playwright') {
  test('fi journey: parser feedback correction submit', async ({ page }) => {
    await mockSignedInUser(page, 'FI');
    const feedbackText = 'Lauloi iloisesti pihalla, kun aurinko paistoi ja linnut visersivät puissa.';

    await page.route('**/api/parse', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          lang: 'FI',
          parse_id: 9001,
          total_tokens: 10,
          parse_duration_ms: 8,
          words: [
            { lemma: 'laulaa', pos: 'VERB', forms: ['Lauloi'], count: 1, grammar_label: 'past 3sg' },
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
    await page.locator('#inspect-text').fill(feedbackText);
    await page.getByRole('button', { name: 'Parse text' }).click();
    await expect(page.locator('#results-page')).toHaveClass(/active/);

    await page.locator('.correction-btn').first().click();
    await expect(page.locator('#correction-modal')).not.toHaveClass(/hidden/);
    await page.locator('#correction-proposed-lemma').fill('laulaa');
    await page.locator('#correction-proposed-pos').selectOption('VERB');
    await page.locator('#correction-submit').click();

    await expect.poll(() => captured).not.toBeNull();
    expect(captured).toMatchObject({
      parse_id: 9001,
      lang: 'FI',
      surface: 'Lauloi',
      original_lemma: 'laulaa',
      original_pos: 'VERB',
      proposed_lemma: 'laulaa',
      proposed_pos: 'VERB',
    });
    await expect(page.locator('.toast.success')).toContainText(/sent/i);
    await expect(page.locator('#correction-modal')).toHaveClass(/hidden/);
  });
} else {
  test.skip('fi journey: parser feedback correction submit', () => {
    // eslint-disable-next-line no-console
    console.log(pendingReason(fiParserFeedbackCase));
  });
}

// ── Every other manifest case: explicit pending/manual stub ────────────────
//
// Keeps `npx playwright test --list` showing the full journey x language
// matrix (per docs/GO_LIVE_CHECKLIST.md) instead of only the cases that
// happen to be implemented today.

const coveredIds = new Set(['fi-deck-save', 'et-deck-save', 'fi-first-review', 'et-first-review', 'fi-parser-feedback']);

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
