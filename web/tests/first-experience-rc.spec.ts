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
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import manifest from '../../testdata/first-experience-rc/manifest.json' with { type: 'json' };

// __dirname resolves relative to this spec file regardless of the process's
// cwd, matching how Playwright's config (webServer cwd: '..') and CI may
// invoke the test runner from different working directories.
const fixtureDir = join(__dirname, '../../testdata/first-experience-rc');

interface RCCase {
  id: string;
  language: 'fi' | 'et';
  journey: string;
  automation: 'parser' | 'playwright' | 'manual' | 'pending';
  fixture?: string;
  expect?: {
    notes?: string;
    known_lemma_pos?: Array<{ lemma: string; pos: string }>;
  };
  notes?: string;
}

// Fixture files live next to the manifest; RC cases that drive a real
// browser flow read their text/word-list straight from the fixture so the
// spec and the manifest can never drift on which sentence/words a case uses.
function readFixture(fixture: string): string {
  return readFileSync(join(fixtureDir, fixture), 'utf-8').trim();
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

// ── anonymous-demo: unsigned paste -> parse -> word list -> explore ────────
//
// Per CONTEXT.md "Anonymous Parser Demo": unsigned paste/parse/explore is
// stateless and ephemeral. This journey pass reuses the mock conventions from
// web/tests/anonymous-parser-demo.spec.ts (which already covers the demo in
// detail — cap copy, sign-up ribbon, privacy footer) but drives the manifest's
// own embedded-text fixture per language, and checks the explore surface
// (Words tab table + POS filter) plus that signed-in-only controls are absent.

async function mockAnonMe(page: Page, anonMaxChars = 300000): Promise<void> {
  await page.route('**/api/me', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ authenticated: false, user: null, anon_max_chars: anonMaxChars }),
    });
  });
}

async function runAnonymousDemo(page: Page, lang: 'FI' | 'ET', fixtureText: string, lemma: string): Promise<void> {
  await mockAnonMe(page);
  await page.route('**/api/parse', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        lang,
        parse_id: null,
        total_tokens: 30,
        parse_duration_ms: 9,
        words: [
          { lemma, pos: 'NOUN', forms: [lemma], count: 3, gloss: lang === 'FI' ? 'cat' : 'cat' },
          { lemma: lang === 'FI' ? 'aurinko' : 'ja', pos: lang === 'FI' ? 'NOUN' : 'CCONJ', forms: ['x'], count: 2 },
        ],
      }),
    });
  });

  await page.goto('/');
  await expect(page.locator('#landing-page')).toHaveClass(/active/);
  if (lang === 'ET') {
    await page.locator('#landing-lang .btn-radio-option[data-value="ET"]').click();
  }

  await page.locator('#landing-text').fill(fixtureText);
  await page.locator('#landing-submit').click();

  await expect(page.locator('#results-page')).toHaveClass(/active/);
  // Anonymous visitor: soft "Your text" pill, no save-as-deck CTA.
  await expect(page.locator('#results-parser')).toHaveText('Your text');
  await expect(page.locator('.results-deck-cta')).toBeHidden();

  // Explore: the lemma table lives behind the Words tab (Read is default).
  await page.locator('#results-tab-words').click();
  await expect(page.locator('#word-table-body tr')).toHaveCount(2);
  await expect(page.locator('#word-table-body')).toContainText(lemma);
  await expect(page.locator('.pos-filter-chip', { hasText: 'Nouns' })).toBeVisible();

  // Signed-in-only controls stay absent for the anonymous demo.
  await expect(page.locator('.correction-btn')).toHaveCount(0);
  await expect(page.locator('.word-pill-known')).toHaveCount(0);

  // Stateless/ephemeral: the privacy footer says so.
  await expect(page.locator('#anon-privacy-footer')).toBeVisible();
  await expect(page.locator('#anon-privacy-footer')).toContainText(/wasn't saved/i);
}

for (const lang of ['fi', 'et'] as const) {
  const anonCase = cases.find(c => c.journey === 'anonymous-demo' && c.language === lang);
  if (!anonCase) throw new Error(`first-experience-rc manifest is missing a ${lang} anonymous-demo case`);

  if (anonCase.automation === 'playwright') {
    test(`${lang} journey: anonymous paste -> parse -> explore`, async ({ page }) => {
      if (!anonCase.fixture) throw new Error(`${anonCase.id} is automation:playwright but has no fixture`);
      const text = readFixture(anonCase.fixture);
      const lemma = lang === 'fi' ? 'kissa' : 'kass';
      await runAnonymousDemo(page, lang.toUpperCase() as 'FI' | 'ET', text, lemma);
    });
  } else {
    test.skip(`${lang} journey: anonymous paste -> parse -> explore`, () => {
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

// ── parser-feedback: Inspect -> parse -> Suggest fix -> submit ─────────────
//
// Reuses the /api/parse/feedback contract already covered in
// parse-results.spec.ts ("correction submit posts the PR-53
// /api/parse/feedback contract"), driven against this manifest's FI and ET
// parser-feedback fixture text so the RC pack and the feature spec assert
// the same contract instead of duplicating fixtures. The flag-only +
// propose-fix modal works identically for both languages, so FI and ET share
// one parametric flow.

interface ParserFeedbackFixture {
  surface: string;
  lemma: string;
  pos: string;
  grammarLabel: string;
}

const parserFeedbackFixtures: Record<'fi' | 'et', ParserFeedbackFixture> = {
  fi: { surface: 'Lauloi', lemma: 'laulaa', pos: 'VERB', grammarLabel: 'past 3sg' },
  et: { surface: 'Laulis', lemma: 'laulma', pos: 'VERB', grammarLabel: 'past 3sg' },
};

async function runParserFeedback(page: Page, lang: 'FI' | 'ET', feedbackText: string): Promise<void> {
  const fixture = parserFeedbackFixtures[lang.toLowerCase() as 'fi' | 'et'];
  await mockSignedInUser(page, lang);

  await page.route('**/api/parse', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        lang,
        parse_id: 9001,
        total_tokens: 10,
        parse_duration_ms: 8,
        words: [
          { lemma: fixture.lemma, pos: fixture.pos, forms: [fixture.surface], count: 1, grammar_label: fixture.grammarLabel },
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
  // The correction entry point lives in the lemma table (Words tab); the
  // results page now defaults to the Read tab.
  const wordsTab = page.locator('#results-tab-words');
  if (await wordsTab.isVisible()) await wordsTab.click();

  await page.locator('.correction-btn').first().click();
  await expect(page.locator('#correction-modal')).not.toHaveClass(/hidden/);
  await page.locator('#correction-mode-propose').check();
  await page.locator('#correction-proposed-lemma').fill(fixture.lemma);
  await page.locator('#correction-proposed-pos').selectOption(fixture.pos);
  await page.locator('#correction-submit').click();

  await expect.poll(() => captured).not.toBeNull();
  expect(captured).toMatchObject({
    parse_id: 9001,
    lang,
    surface: fixture.surface,
    original_lemma: fixture.lemma,
    original_pos: fixture.pos,
    proposed_lemma: fixture.lemma,
    proposed_pos: fixture.pos,
  });
  await expect(page.locator('.toast.success')).toContainText(/sent/i);
  await expect(page.locator('#correction-modal')).toHaveClass(/hidden/);
}

for (const lang of ['fi', 'et'] as const) {
  const feedbackCase = cases.find(c => c.journey === 'parser-feedback' && c.language === lang);
  if (!feedbackCase) throw new Error(`first-experience-rc manifest is missing a ${lang} parser-feedback case`);

  if (feedbackCase.automation === 'playwright') {
    test(`${lang} journey: parser feedback correction submit`, async ({ page }) => {
      if (!feedbackCase.fixture) throw new Error(`${feedbackCase.id} is automation:playwright but has no fixture`);
      const text = readFixture(feedbackCase.fixture);
      await runParserFeedback(page, lang.toUpperCase() as 'FI' | 'ET', text);
    });
  } else {
    test.skip(`${lang} journey: parser feedback correction submit`, () => {
      // eslint-disable-next-line no-console
      console.log(pendingReason(feedbackCase));
    });
  }
}

// ── known-word-import: paste a word list on /#/vocab -> resolve lemma/POS ──
//
// Reuses the /api/known-words import contract already covered in
// parse-results.spec.ts ("user can import and remove known words"), driven
// against this manifest's fixture word lists so every fixture word resolves
// to the manifest's expected (lemma, POS).

interface KnownWordImportFixture {
  activeLanguage: 'FI' | 'ET';
}

const knownWordImportFixtures: Record<'fi' | 'et', KnownWordImportFixture> = {
  fi: { activeLanguage: 'FI' },
  et: { activeLanguage: 'ET' },
};

async function runKnownWordImport(page: Page, lang: 'fi' | 'et', words: string[], expected: Array<{ lemma: string; pos: string }>): Promise<void> {
  const fixture = knownWordImportFixtures[lang];
  let knownWords: Array<{ lemma: string; pos: string; lang: string }> = [];
  let importPayload: any = null;

  await page.route('**/api/me', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        authenticated: true,
        user: { id: 1, email: 'alice@example.com', is_admin: false },
        dashboard: { known_count: 0, due_count: 0, new_capacity_today: 0, decks: [] },
        languages: {
          learning: ['FI', 'ET'],
          active: fixture.activeLanguage,
          stats: { FI: { decks: 0, known_words: 0 }, ET: { decks: 0, known_words: 0 } },
        },
      }),
    });
  });

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
      knownWords = expected.map(w => ({ lemma: w.lemma, pos: w.pos, lang: fixture.activeLanguage }));
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          imported: expected.map(w => ({ lemma: w.lemma, pos: w.pos, lang: fixture.activeLanguage })),
          unresolved: [],
        }),
      });
      return;
    }
    await route.continue();
  });

  await page.goto('/#/vocab');
  await page.locator('#known-words-input').fill(words.join(', '));
  await page.getByRole('button', { name: 'Import words' }).click();

  await expect.poll(() => importPayload).not.toBeNull();
  expect(importPayload).toMatchObject({ lang: fixture.activeLanguage, words });

  for (const w of expected) {
    await expect(page.locator('#known-words-list')).toContainText(w.lemma);
  }
  await expect(page.locator('#known-words-unresolved')).toBeHidden();
}

for (const lang of ['fi', 'et'] as const) {
  const importCase = cases.find(c => c.journey === 'known-word-import' && c.language === lang);
  if (!importCase) throw new Error(`first-experience-rc manifest is missing a ${lang} known-word-import case`);

  if (importCase.automation === 'playwright') {
    test(`${lang} journey: known-word import`, async ({ page }) => {
      if (!importCase.fixture) throw new Error(`${importCase.id} is automation:playwright but has no fixture`);
      const expected = importCase.expect?.known_lemma_pos;
      if (!expected || expected.length === 0) {
        throw new Error(`${importCase.id} is automation:playwright but has no expect.known_lemma_pos`);
      }
      const words = readFixture(importCase.fixture).split(',').map(w => w.trim());
      await runKnownWordImport(page, lang, words, expected);
    });
  } else {
    test.skip(`${lang} journey: known-word import`, () => {
      // eslint-disable-next-line no-console
      console.log(pendingReason(importCase));
    });
  }
}

// ── ambiguity-homograph: parse -> open meanings panel -> flag-only escape ──
//
// Journey coverage only: parses the manifest's ambiguity fixture sentence
// (mocked, since the RC pack's job is journey coverage, not re-testing the
// real FST-merge candidate quality — that lives in the "parser" automation
// cases above and in the eval slice per docs/PARSER_EVAL_METHODOLOGY.md),
// opens the ambiguous word's "Multiple possible meanings" panel, and checks
// there are >= 2 candidates and the flag-only "None of these looks right"
// escape exists. Does not grade which candidate is "correct" — see
// web/tests/ambiguity-meanings.spec.ts for the full candidate-selection
// feature coverage this journey pass does not duplicate.

interface AmbiguityFixture {
  sentence: string;
  surface: string;
  candidates: Array<{ lemma: string; pos: string; gloss: string; source: 'dict' | 'fst' }>;
}

const ambiguityFixtures: Record<'fi' | 'et', AmbiguityFixture> = {
  fi: {
    sentence: 'Pihalla kasvoi korkea kuusi, jonka oksat olivat lumen peitossa.',
    surface: 'kuusi',
    candidates: [
      { lemma: 'kuusi', pos: 'NUM', gloss: 'six', source: 'dict' },
      { lemma: 'kuusi', pos: 'NOUN', gloss: 'spruce', source: 'fst' },
    ],
  },
  et: {
    sentence: 'Lõkkest tõusis soe tuli, mis valgustas pimedat õue.',
    surface: 'tuli',
    candidates: [
      { lemma: 'tulema', pos: 'VERB', gloss: 'came', source: 'dict' },
      { lemma: 'tuli', pos: 'NOUN', gloss: 'fire', source: 'fst' },
    ],
  },
};

async function runAmbiguityMeaningsPanel(page: Page, lang: 'FI' | 'ET'): Promise<void> {
  const fixture = ambiguityFixtures[lang.toLowerCase() as 'fi' | 'et'];
  await mockSignedInUser(page, lang);

  await page.route('**/api/parse', async (route) => {
    // Exactly one word-list row for the ambiguous surface (its primary/dict
    // sense), matching the /api/parse shape in web/tests/ambiguity-meanings.spec.ts:
    // the FST-only alternate senses live only in ambiguous_surfaces, not as
    // their own word-list rows. A row per candidate would render one
    // ambiguity chip (and panel) per row instead of one shared chip.
    const primary = fixture.candidates[0];
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        lang,
        total_tokens: 10,
        parse_duration_ms: 8,
        words: [
          { lemma: primary.lemma, pos: primary.pos, forms: [fixture.surface], count: 1, gloss: primary.gloss, example_sentence: fixture.sentence },
        ],
        ambiguous_surfaces: [
          { surface: fixture.surface, example: fixture.sentence, candidates: fixture.candidates },
        ],
      }),
    });
  });

  await page.goto('/#/inspect');
  await page.locator('#inspect-text').fill(fixture.sentence);
  await page.getByRole('button', { name: 'Parse text' }).click();
  await expect(page.locator('#results-page')).toHaveClass(/active/);
  const wordsTab = page.locator('#results-tab-words');
  if (await wordsTab.isVisible()) await wordsTab.click();

  const chip = page.locator('.ambiguity-chip').first();
  await expect(chip).toContainText('Multiple possible meanings');
  await chip.click();

  const panel = page.locator('.ambiguity-panel');
  await expect(panel).toBeVisible();
  await expect(panel.locator('.ambiguity-candidate')).toHaveCount(fixture.candidates.length);
  expect(fixture.candidates.length).toBeGreaterThanOrEqual(2);

  // The flag-only escape exists regardless of which candidate (if any) is right.
  await expect(page.getByRole('button', { name: 'None of these looks right' })).toBeVisible();
}

for (const lang of ['fi', 'et'] as const) {
  const ambiguityCase = cases.find(c => c.journey === 'ambiguity-homograph' && c.language === lang);
  if (!ambiguityCase) throw new Error(`first-experience-rc manifest is missing a ${lang} ambiguity-homograph case`);

  test(`${lang} journey: ambiguity homograph meanings panel`, async ({ page }) => {
    await runAmbiguityMeaningsPanel(page, lang.toUpperCase() as 'FI' | 'ET');
  });
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
  'fi-ambiguity-homograph', 'fi-ambiguity-tuli', 'fi-ambiguity-voi', 'et-ambiguity-homograph',
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
