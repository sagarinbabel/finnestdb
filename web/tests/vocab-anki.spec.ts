import { expect, test, type Page, type Route } from '@playwright/test';

// Tests for the Vocab page and the AnkiConnect import flow.
//
// Two layers:
//
// 1. Pure-logic tests exercise the small helpers exposed on
//    `window.__finestTest` via page.evaluate(). They cover the parts that have
//    no DOM coupling: deck-tree building, filter matching, the field auto-pick
//    algorithm, and the localStorage prefs round-trip.
//
// 2. E2E tests drive the UI with mocked /api/* + mocked AnkiConnect calls.
//    AnkiConnect speaks plain HTTP at 127.0.0.1:8765, so we intercept that
//    URL the same way we intercept the backend.

type Role = 'anon' | 'user' | 'admin';

interface MockMeOptions {
  activeLanguage?: string;
  learningLanguages?: string[];
  knownCount?: number;
  stats?: Record<string, { decks: number; known_words: number }>;
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
    const learning = opts.learningLanguages ?? ['FI', 'ET'];
    const active = opts.activeLanguage ?? 'FI';
    const stats = opts.stats ?? Object.fromEntries(learning.map(l => [l, { decks: 0, known_words: 0 }]));
    const knownCount = opts.knownCount ?? Object.values(stats).reduce((sum, s) => sum + s.known_words, 0);
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        authenticated: true,
        user: { id: 1, email: 'alice@example.com', is_admin: role === 'admin' },
        dashboard: {
          known_count: knownCount,
          due_count: 0,
          new_capacity_today: 0,
          decks: [],
        },
        languages: { learning, active, stats },
      }),
    });
  });
}

async function mockKnownWordsEmpty(page: Page): Promise<void> {
  await page.route('**/api/known-words?*', async (route) => {
    if (route.request().method() === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ known_words: [] }),
      });
    } else {
      await route.fallback();
    }
  });
}

// Builds an AnkiConnect mock that dispatches on the `action` field in the JSON
// body. Caller provides per-action handlers; anything not listed responds with
// `{ error: 'unhandled', result: null }`.
type AnkiHandler = (params: Record<string, unknown>) => unknown;
type AnkiMocks = Record<string, AnkiHandler>;

interface AnkiCall {
  action: string;
  params: Record<string, unknown>;
}

async function mockAnkiConnect(
  page: Page,
  handlers: AnkiMocks,
  log?: AnkiCall[],
): Promise<void> {
  await page.route('http://127.0.0.1:8765/**', async (route: Route) => {
    if (route.request().method() === 'OPTIONS') {
      await route.fulfill({ status: 200, body: '' });
      return;
    }
    const body = route.request().postDataJSON() as { action: string; params?: Record<string, unknown> };
    const action = body?.action || '';
    const params = body?.params || {};
    log?.push({ action, params });
    const handler = handlers[action];
    if (!handler) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ result: null, error: `unhandled action: ${action}` }),
      });
      return;
    }
    const result = handler(params);
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ result, error: null }),
    });
  });
}

// Clears the page's localStorage before each evaluate-driven test so the prefs
// round-trip starts from a known baseline.
async function clearStorage(page: Page): Promise<void> {
  await page.evaluate(() => {
    localStorage.clear();
  });
}

// ── 1. Pure-logic helpers via window.__finestTest ──────────────────────────

test.describe('pure helpers', () => {
  test('buildAnkiDeckTree splits names on :: into a tree', async ({ page }) => {
    await mockMe(page, 'user');
    await page.goto('/');
    const tree = await page.evaluate(() => {
      const w = window as unknown as { __finestTest: { buildAnkiDeckTree: (n: string[]) => unknown } };
      return w.__finestTest.buildAnkiDeckTree([
        'Estonian::A1::Verbs',
        'Estonian::A1::Nouns',
        'Estonian::A2',
        'Finnish',
        'Mixed',
      ]);
    });
    expect(tree).toEqual([
      {
        name: 'Estonian',
        fullName: 'Estonian',
        children: [
          {
            name: 'A1',
            fullName: 'Estonian::A1',
            children: [
              { name: 'Nouns', fullName: 'Estonian::A1::Nouns', children: [] },
              { name: 'Verbs', fullName: 'Estonian::A1::Verbs', children: [] },
            ],
          },
          { name: 'A2', fullName: 'Estonian::A2', children: [] },
        ],
      },
      { name: 'Finnish', fullName: 'Finnish', children: [] },
      { name: 'Mixed', fullName: 'Mixed', children: [] },
    ]);
  });

  test('deckMatchesFilter is pipe-separated, case-insensitive substring', async ({ page }) => {
    await mockMe(page, 'user');
    await page.goto('/');
    const results = await page.evaluate(() => {
      const w = window as unknown as { __finestTest: { deckMatchesFilter: (n: string, f: string) => boolean } };
      const f = w.__finestTest.deckMatchesFilter;
      return {
        emptyFilterMatchesAnything: f('Anything', ''),
        whitespaceFilterMatchesAnything: f('Anything', '   '),
        substring: f('Estonian::A1', 'estonian'),
        caseInsensitive: f('Estonian::A1', 'ESTONIAN'),
        pipeAnyMatches: f('Eesti keel', 'estonian|eesti'),
        pipeNeitherMatches: f('Other deck', 'estonian|eesti'),
        partialMatch: f('Suomi sanat', 'finnish|suomi'),
      };
    });
    expect(results).toEqual({
      emptyFilterMatchesAnything: true,
      whitespaceFilterMatchesAnything: true,
      substring: true,
      caseInsensitive: true,
      pipeAnyMatches: true,
      pipeNeitherMatches: false,
      partialMatch: true,
    });
  });

  test('defaultAnkiFilter returns language-specific defaults', async ({ page }) => {
    await mockMe(page, 'user');
    await page.goto('/');
    const result = await page.evaluate(() => {
      const w = window as unknown as { __finestTest: { defaultAnkiFilter: (l: string) => string } };
      const f = w.__finestTest.defaultAnkiFilter;
      return { fi: f('FI'), et: f('ET'), other: f('XX') };
    });
    expect(result).toEqual({ fi: 'finnish|suomi', et: 'estonian|eesti', other: '' });
  });

  test('pickBestField prefers single-word fields with name hints', async ({ page }) => {
    await mockMe(page, 'user');
    await page.goto('/');
    const results = await page.evaluate(() => {
      const w = window as unknown as { __finestTest: { pickBestField: (f: string[], e: Record<string, string[]>, l: string) => string } };
      const f = w.__finestTest.pickBestField;
      return {
        // Two single-word candidates; "Sõna" wins because of the ET name list.
        nameHintEstonian: f(
          ['Front', 'Back', 'Sõna'],
          { Front: ['kissa'], Back: ['the cat'], 'Sõna': ['kassi'] },
          'ET',
        ),
        // Same data but FI active: "sana" name hint matches "Sana", "Sõna" doesn't.
        nameHintFinnish: f(
          ['Sana', 'Translation'],
          { Sana: ['kissa', 'koira'], Translation: ['cat', 'dog'] },
          'FI',
        ),
        // No name hint match but one field is consistently single-word.
        pickSingleWordField: f(
          ['Front', 'Notes'],
          { Front: ['kissa', 'koira', 'auto'], Notes: ['a cat that meows', 'the dog runs'] },
          'FI',
        ),
        // No samples for any field — fall back to the first field.
        emptySamplesFallsBackToFirst: f(['Front', 'Back'], { Front: [], Back: [] }, 'FI'),
        // No fields at all.
        emptyFieldsReturnsEmpty: f([], {}, 'FI'),
        // English-only deck with "Word" — universal vocab term wins.
        universalWordHint: f(
          ['Word', 'Definition'],
          { Word: ['cat', 'dog'], Definition: ['a small carnivorous mammal'] },
          'FI',
        ),
      };
    });
    expect(results).toEqual({
      nameHintEstonian: 'Sõna',
      nameHintFinnish: 'Sana',
      pickSingleWordField: 'Front',
      emptySamplesFallsBackToFirst: 'Front',
      emptyFieldsReturnsEmpty: '',
      universalWordHint: 'Word',
    });
  });

  test('parseFileWords handles TXT, CSV, TSV, BOM, and dedupes', async ({ page }) => {
    await mockMe(page, 'user');
    await page.goto('/');
    const results = await page.evaluate(() => {
      const w = window as unknown as { __finestTest: { parseFileWords: (raw: string) => string[] } };
      const f = w.__finestTest.parseFileWords;
      return {
        oneWordPerLine: f('kissa\nkoira\nauto'),
        csvFirstColumn: f('kissa,cat\nkoira,dog\nauto,car'),
        tsvFirstColumn: f('kissa\tcat\nkoira\tdog'),
        bomStripped: f('﻿kissa\nkoira'),
        quotedCells: f('"kissa",cat\n"koira",dog'),
        dedupesCaseInsensitive: f('Kissa\nkissa\nKISSA'),
        skipsBlankLines: f('kissa\n\n\nkoira\n   \nauto'),
      };
    });
    expect(results).toEqual({
      oneWordPerLine: ['kissa', 'koira', 'auto'],
      csvFirstColumn: ['kissa', 'koira', 'auto'],
      tsvFirstColumn: ['kissa', 'koira'],
      bomStripped: ['kissa', 'koira'],
      quotedCells: ['kissa', 'koira'],
      dedupesCaseInsensitive: ['Kissa'],
      skipsBlankLines: ['kissa', 'koira', 'auto'],
    });
  });

  test('cleanAnkiSurfaceForm rewrites textbook notation to real Estonian forms', async ({ page }) => {
    await mockMe(page, 'user');
    await page.goto('/');
    const results = await page.evaluate(() => {
      const w = window as unknown as { __finestTest: { cleanAnkiSurfaceForm: (s: string) => string } };
      const f = w.__finestTest.cleanAnkiSurfaceForm;
      return {
        // /n at word boundary → bare n (textbook 1sg notation).
        slashN_single:        f('anna/n'),
        slashN_inPhrase:      f('jää/n alla'),
        slashN_atEnd:         f('saa/n'),
        // Stress acute on vowels (NFC and NFD inputs both supported).
        acuteNFC:             f('diréktor'),
        acuteCombined:        f('diréktor'.normalize('NFD')),
        acuteMultiple:        f('telegraféerima'),
        // Parenthetical alternates anywhere.
        parensInline:         f('iial(gi)'),
        parensSpaced:         f('tool (n)'),
        parensMulti:          f('appi (kellegi)'),
        // Trailing sentence punctuation.
        trailingExcl:         f('mine!'),
        trailingQ:            f('mis aastal?'),
        // Phrase-pattern slots → dropped entirely.
        ellipsisStart:        f('… all'),
        ellipsisEnd:          f('üle …'),
        ellipsisMid:          f('kui...ka'),
        // Already-clean words pass through unchanged.
        plainWord:            f('kissa'),
        plainPhrase:          f('tänan väga'),
        // Combinations: acute + parens + trailing punct.
        combo:                f('diréktor(?)!'),
        // Empty and whitespace-only.
        empty:                f(''),
        whitespace:           f('   '),
      };
    });
    expect(results).toEqual({
      slashN_single:    'annan',
      slashN_inPhrase:  'jään alla',
      slashN_atEnd:     'saan',
      acuteNFC:         'direktor',
      acuteCombined:    'direktor',
      acuteMultiple:    'telegrafeerima',
      parensInline:     'iial',
      parensSpaced:     'tool',
      parensMulti:      'appi',
      trailingExcl:     'mine',
      trailingQ:        'mis aastal',
      ellipsisStart:    '',
      ellipsisEnd:      '',
      ellipsisMid:      '',
      plainWord:        'kissa',
      plainPhrase:      'tänan väga',
      combo:            'direktor',
      empty:            '',
      whitespace:       '',
    });
  });

  test('loadAnkiPrefs / saveAnkiPrefs round-trip per language', async ({ page }) => {
    await mockMe(page, 'user');
    await page.goto('/');
    await clearStorage(page);
    const results = await page.evaluate(() => {
      const w = window as unknown as { __finestTest: {
        loadAnkiPrefs: (l: string) => { filter: string; decks: string[]; fieldByModel: Record<string, string> };
        saveAnkiPrefs: (l: string, p: { filter: string; decks: string[]; fieldByModel: Record<string, string> }) => void;
        defaultAnkiFilter: (l: string) => string;
      } };
      const initialFI = w.__finestTest.loadAnkiPrefs('FI');
      const initialET = w.__finestTest.loadAnkiPrefs('ET');
      w.__finestTest.saveAnkiPrefs('FI', {
        filter: 'custom-fi',
        decks: ['Finnish::A1'],
        fieldByModel: { Basic: 'Word' },
        includeNew: false,
        includeSuspended: false,
        replaceMode: false,
        lastSyncAt: 0,
      });
      w.__finestTest.saveAnkiPrefs('ET', {
        filter: 'custom-et',
        decks: ['Estonian::A1', 'Estonian::A2'],
        fieldByModel: { Basic: 'Sõna' },
        includeNew: true,
        includeSuspended: true,
        replaceMode: true,
        lastSyncAt: 1700000000000,
      });
      return {
        initialFI,
        initialET,
        savedFI: w.__finestTest.loadAnkiPrefs('FI'),
        savedET: w.__finestTest.loadAnkiPrefs('ET'),
      };
    });
    expect(results.initialFI).toEqual({ filter: 'finnish|suomi', decks: [], fieldByModel: {}, includeNew: false, includeSuspended: false, replaceMode: false, lastSyncAt: 0 });
    expect(results.initialET).toEqual({ filter: 'estonian|eesti', decks: [], fieldByModel: {}, includeNew: false, includeSuspended: false, replaceMode: false, lastSyncAt: 0 });
    expect(results.savedFI).toEqual({
      filter: 'custom-fi',
      decks: ['Finnish::A1'],
      fieldByModel: { Basic: 'Word' },
      includeNew: false,
      includeSuspended: false,
      replaceMode: false,
      lastSyncAt: 0,
    });
    expect(results.savedET).toEqual({
      filter: 'custom-et',
      decks: ['Estonian::A1', 'Estonian::A2'],
      fieldByModel: { Basic: 'Sõna' },
      includeNew: true,
      includeSuspended: true,
      replaceMode: true,
      lastSyncAt: 1700000000000,
    });
  });
});

// ── 2. Vocab page baseline ─────────────────────────────────────────────────

test.describe('vocab page', () => {
  test('Vocab nav link is hidden from anonymous users', async ({ page }) => {
    await mockMe(page, 'anon');
    await page.goto('/');
    await expect(page.getByRole('link', { name: 'Vocab' }).first()).toBeHidden();
  });

  test('Vocab nav link is visible to logged-in users', async ({ page }) => {
    await mockMe(page, 'user');
    await mockKnownWordsEmpty(page);
    await page.goto('/#/dashboard');
    await expect(page.getByRole('link', { name: 'Vocab' }).first()).toBeVisible();
  });

  test('Vocab page shows total + per-language known stats from /api/me', async ({ page }) => {
    await mockMe(page, 'user', {
      activeLanguage: 'ET',
      stats: { FI: { decks: 0, known_words: 80 }, ET: { decks: 0, known_words: 120 } },
    });
    await mockKnownWordsEmpty(page);
    await page.goto('/#/vocab');

    await expect(page.locator('#vocab-page')).toHaveClass(/active/);
    await expect(page.locator('#vocab-stat-total')).toHaveText('200');
    await expect(page.locator('#vocab-stat-lang-name')).toHaveText('Estonian');
    await expect(page.locator('#vocab-stat-lang')).toHaveText('120');
  });

  test('Delete all vocabulary requires confirm and sends DELETE …?lang=…&all=1', async ({ page }) => {
    await mockMe(page, 'user', {
      activeLanguage: 'FI',
      stats: { FI: { decks: 0, known_words: 3 }, ET: { decks: 0, known_words: 0 } },
    });
    // The GET mock returns 3 words pre-delete and 0 post-delete. The
    // delete-all flow refreshes the dashboard + reloads known words, so the
    // mock has to actually mirror the server-side state for the empty-state
    // assertion to be meaningful.
    let serverHasWords = true;
    await page.route('**/api/known-words?lang=FI*', async (route) => {
      if (route.request().method() === 'GET') {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            known_words: serverHasWords ? [
              { lemma: 'kissa', pos: 'NOUN', lang: 'FI' },
              { lemma: 'koira', pos: 'NOUN', lang: 'FI' },
              { lemma: 'juosta', pos: 'VERB', lang: 'FI' },
            ] : [],
          }),
        });
      } else {
        await route.fallback();
      }
    });

    let deleteUrl = '';
    await page.route('**/api/known-words?**', async (route) => {
      if (route.request().method() !== 'DELETE') {
        await route.fallback();
        return;
      }
      deleteUrl = route.request().url();
      serverHasWords = false;
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ status: 'ok', deleted: 3 }),
      });
    });

    await page.goto('/#/vocab');
    const deleteBtn = page.locator('#vocab-delete-all');
    await expect(deleteBtn).toBeVisible();

    // Tooltip text lives on the button as data-tooltip; portal tooltip system
    // surfaces it on hover, but the data attribute itself is what we need to
    // guarantee — the wording is the contract.
    await expect(deleteBtn).toHaveAttribute('data-tooltip', /confirm deletion/i);

    // Click → confirmation dialog must appear, with a danger-styled confirm.
    await deleteBtn.click();
    const dialog = page.locator('#dialog-modal');
    await expect(dialog).not.toHaveClass(/hidden/);
    await expect(dialog).toContainText(/Delete all Finnish vocabulary/i);
    await expect(dialog).toContainText(/3/);
    await expect(dialog).toContainText(/cannot be undone/i);

    // Cancel path: dialog dismisses, no DELETE call.
    await page.getByRole('button', { name: 'Cancel' }).click();
    await expect(dialog).toHaveClass(/hidden/);
    expect(deleteUrl).toBe('');

    // Confirm path: DELETE fires with lang + all=1.
    await deleteBtn.click();
    await expect(dialog).not.toHaveClass(/hidden/);
    // Target the dialog's confirm button by ID (the page button "Delete all
    // vocabulary" overlaps the dialog's "Delete all" name accessibility-wise).
    await page.locator('#dialog-modal-confirm').click();
    await expect.poll(() => deleteUrl).not.toBe('');
    expect(deleteUrl).toContain('lang=FI');
    expect(deleteUrl).toContain('all=1');
    // The list re-renders to empty after deletion.
    await expect(page.locator('#known-words-empty')).not.toHaveClass(/hidden/);
  });

  test('Delete all does nothing when there are no known words to delete', async ({ page }) => {
    await mockMe(page, 'user', { activeLanguage: 'FI' });
    await mockKnownWordsEmpty(page);
    let deleteCalled = false;
    await page.route('**/api/known-words?**', async (route) => {
      if (route.request().method() === 'DELETE') deleteCalled = true;
      await route.fallback();
    });
    await page.goto('/#/vocab');
    await page.locator('#vocab-delete-all').click();
    // No confirm dialog should pop, no DELETE should fire — there's nothing
    // to delete.
    await expect(page.locator('#dialog-modal')).toHaveClass(/hidden/);
    expect(deleteCalled).toBe(false);
  });

  test('textbox import POSTs words to /api/known-words with the active language', async ({ page }) => {
    await mockMe(page, 'user', { activeLanguage: 'FI' });
    await mockKnownWordsEmpty(page);
    let importBody: { lang?: string; words?: string[] } | null = null;
    await page.route('**/api/known-words', async (route) => {
      if (route.request().method() === 'POST') {
        importBody = route.request().postDataJSON();
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ imported: [], unresolved: [] }),
        });
      } else {
        await route.fallback();
      }
    });
    await page.goto('/#/vocab');
    await page.locator('#known-words-input').fill('kissa\nkoira, auto');
    await page.getByRole('button', { name: 'Import words' }).click();
    await expect.poll(() => importBody).not.toBeNull();
    expect(importBody!.lang).toBe('FI');
    expect(importBody!.words).toEqual(['kissa', 'koira', 'auto']);
  });
});

// ── 3. Setup-instructions popup ────────────────────────────────────────────

test.describe('Anki setup popup', () => {
  test('opens via "Setup instructions" link with the JSON config block', async ({ page }) => {
    await mockMe(page, 'user');
    await mockKnownWordsEmpty(page);
    await page.goto('/#/vocab');

    await expect(page.locator('#anki-setup-modal')).toHaveClass(/hidden/);
    await page.getByRole('link', { name: 'Setup instructions' }).click();
    await expect(page.locator('#anki-setup-modal')).not.toHaveClass(/hidden/);
    await expect(page.locator('#anki-setup-modal')).toContainText('AnkiConnect');
    await expect(page.locator('#anki-setup-copy-source')).toContainText('webCorsOriginList');
    await expect(page.locator('#anki-setup-copy-source')).toContainText('"https://finne.st"');

    // Close via the × button
    await page.locator('#anki-setup-modal-close').click();
    await expect(page.locator('#anki-setup-modal')).toHaveClass(/hidden/);
  });

  test('opens automatically when AnkiConnect is unreachable', async ({ page }) => {
    await mockMe(page, 'user');
    await mockKnownWordsEmpty(page);
    // Abort every request to AnkiConnect → frontend treats it as "not reachable"
    await page.route('http://127.0.0.1:8765/**', route => route.abort('failed'));
    await page.goto('/#/vocab');
    await page.getByRole('button', { name: 'Connect to Anki' }).click();
    await expect(page.locator('#anki-setup-modal')).not.toHaveClass(/hidden/, { timeout: 5000 });
  });
});

// ── 4. Anki import popup (E2E) ─────────────────────────────────────────────

const SAMPLE_DECKS = [
  'Default',
  'Estonian',
  'Estonian::A1',
  'Estonian::A1::Verbs',
  'Estonian::A1::Nouns',
  'Estonian::A2',
  'Finnish',
  'Finnish::A1',
  'Mixed',
];

// Two note types in the sampled notes:
//   - "ETBasic" with fields ["Sõna", "Tähendus", "Lause"] — sõna single-word
//   - "Basic"   with fields ["Front", "Back"]            — Front single-word
const NOTES_INFO: Record<number, {
  noteId: number;
  modelName: string;
  fields: Record<string, { value: string; order: number }>;
}> = {
  100: { noteId: 100, modelName: 'ETBasic', fields: { 'Sõna': { value: 'kassi', order: 0 }, 'Tähendus': { value: 'cat (animal)', order: 1 }, 'Lause': { value: 'See on kassi.', order: 2 } } },
  101: { noteId: 101, modelName: 'ETBasic', fields: { 'Sõna': { value: 'koer', order: 0 }, 'Tähendus': { value: 'dog (animal)', order: 1 }, 'Lause': { value: 'See on koer.', order: 2 } } },
  102: { noteId: 102, modelName: 'ETBasic', fields: { 'Sõna': { value: 'auto', order: 0 }, 'Tähendus': { value: 'car', order: 1 }, 'Lause': { value: 'See on minu auto.', order: 2 } } },
  200: { noteId: 200, modelName: 'Basic', fields: { Front: { value: 'maja', order: 0 }, Back: { value: 'house', order: 1 } } },
  201: { noteId: 201, modelName: 'Basic', fields: { Front: { value: 'raamat', order: 0 }, Back: { value: 'book', order: 1 } } },
};

function notesInDeck(deck: string): number[] {
  if (deck === 'Estonian::A1::Verbs') return [100, 101];
  if (deck === 'Estonian::A1::Nouns') return [102];
  if (deck === 'Estonian::A2') return [200, 201];
  if (deck === 'Estonian::A1') return [100, 101, 102];
  if (deck === 'Estonian') return [100, 101, 102, 200, 201];
  return [];
}

// Parse an AnkiConnect findNotes query and return the matching note IDs.
// Supports `deck:"X"` and optional `-is:new` / `-is:suspended` predicates that
// filter out IDs in the corresponding fixture sets. Mirrors Anki's own
// semantics: a deck restriction plus negative card-state filters.
function findNotesQuery(
  query: string,
  newNoteIds: Set<number>,
  suspendedNoteIds: Set<number>,
  resolveDeck: (deck: string) => number[],
): number[] {
  const m = query.match(/deck:"([^"]+)"/);
  if (!m) return [];
  let ids = resolveDeck(m[1]);
  if (/-is:new\b/.test(query)) ids = ids.filter(id => !newNoteIds.has(id));
  if (/-is:suspended\b/.test(query)) ids = ids.filter(id => !suspendedNoteIds.has(id));
  return ids;
}

function baselineAnkiMocks(
  newNoteIds: Set<number> = new Set(),
  suspendedNoteIds: Set<number> = new Set(),
): AnkiMocks {
  return {
    version: () => 6,
    deckNames: () => SAMPLE_DECKS,
    findNotes: (params) => findNotesQuery(String(params.query || ''), newNoteIds, suspendedNoteIds, notesInDeck),
    notesInfo: (params) => {
      const ids = (params.notes as number[]) || [];
      return ids.map(id => NOTES_INFO[id]).filter(Boolean);
    },
    modelFieldNames: (params) => {
      const m = String(params.modelName || '');
      if (m === 'ETBasic') return ['Sõna', 'Tähendus', 'Lause'];
      if (m === 'Basic') return ['Front', 'Back'];
      return [];
    },
  };
}

test.describe('Anki import popup', () => {
  test('opens, builds deck tree hierarchically, default filter matches language', async ({ page }) => {
    await mockMe(page, 'user', { activeLanguage: 'ET' });
    await mockKnownWordsEmpty(page);
    await mockAnkiConnect(page, baselineAnkiMocks());
    await page.goto('/#/vocab');
    await clearStorage(page);

    await page.getByRole('button', { name: 'Connect to Anki' }).click();
    await expect(page.locator('#anki-import-modal')).not.toHaveClass(/hidden/);
    await expect(page.locator('#anki-import-stage-decks')).not.toHaveClass(/hidden/);

    // Default filter for active=ET
    await expect(page.locator('#anki-import-filter')).toHaveValue('estonian|eesti');

    // With the default filter active, only Estonian branch is visible.
    const tree = page.locator('#anki-import-tree');
    await expect(tree).toContainText('Estonian');
    await expect(tree).toContainText('A1');
    await expect(tree).toContainText('Verbs');
    await expect(tree).not.toContainText('Finnish');
    await expect(tree).not.toContainText('Default');
  });

  test('clearing the filter reveals the full deck list', async ({ page }) => {
    await mockMe(page, 'user', { activeLanguage: 'ET' });
    await mockKnownWordsEmpty(page);
    await mockAnkiConnect(page, baselineAnkiMocks());
    await page.goto('/#/vocab');
    await clearStorage(page);
    await page.getByRole('button', { name: 'Connect to Anki' }).click();
    await expect(page.locator('#anki-import-stage-decks')).not.toHaveClass(/hidden/);

    await page.locator('#anki-import-clear-filter').click();
    await expect(page.locator('#anki-import-filter')).toHaveValue('');
    const tree = page.locator('#anki-import-tree');
    await expect(tree).toContainText('Finnish');
    await expect(tree).toContainText('Default');
  });

  test('selecting a parent deck cascades to every descendant', async ({ page }) => {
    await mockMe(page, 'user', { activeLanguage: 'ET' });
    await mockKnownWordsEmpty(page);
    await mockAnkiConnect(page, baselineAnkiMocks());
    await page.goto('/#/vocab');
    await clearStorage(page);
    await page.getByRole('button', { name: 'Connect to Anki' }).click();
    await expect(page.locator('#anki-import-stage-decks')).not.toHaveClass(/hidden/);

    // Expand Estonian, then its A1 child, so descendants are visible.
    await page.locator('[data-deck-toggle="Estonian"]').click();
    await page.locator('[data-deck-toggle="Estonian::A1"]').click();

    // Check the Estonian parent.
    await page.locator('[data-deck-check="Estonian"]').check();

    for (const deck of ['Estonian', 'Estonian::A1', 'Estonian::A1::Verbs', 'Estonian::A1::Nouns', 'Estonian::A2']) {
      await expect(page.locator(`[data-deck-check="${deck}"]`)).toBeChecked();
    }

    // Unchecking the parent uncascades.
    await page.locator('[data-deck-check="Estonian"]').uncheck();
    for (const deck of ['Estonian', 'Estonian::A1', 'Estonian::A1::Verbs', 'Estonian::A1::Nouns']) {
      await expect(page.locator(`[data-deck-check="${deck}"]`)).not.toBeChecked();
    }
  });

  test('selected decks + filter persist across modal opens (localStorage)', async ({ page }) => {
    await mockMe(page, 'user', { activeLanguage: 'ET' });
    await mockKnownWordsEmpty(page);
    await mockAnkiConnect(page, baselineAnkiMocks());
    await page.goto('/#/vocab');
    await clearStorage(page);

    // Open, set a custom filter and a single deck, close.
    await page.getByRole('button', { name: 'Connect to Anki' }).click();
    await expect(page.locator('#anki-import-stage-decks')).not.toHaveClass(/hidden/);
    await page.locator('#anki-import-filter').fill('a1');
    await page.locator('[data-deck-toggle="Estonian"]').click();
    await page.locator('[data-deck-toggle="Estonian::A1"]').click();
    await page.locator('[data-deck-check="Estonian::A1::Verbs"]').check();
    await page.getByRole('button', { name: 'Cancel' }).click();
    await expect(page.locator('#anki-import-modal')).toHaveClass(/hidden/);

    // Open again — the filter and the selected deck should be restored.
    await page.getByRole('button', { name: 'Connect to Anki' }).click();
    await expect(page.locator('#anki-import-stage-decks')).not.toHaveClass(/hidden/);
    await expect(page.locator('#anki-import-filter')).toHaveValue('a1');
    await expect(page.locator('[data-deck-check="Estonian::A1::Verbs"]')).toBeChecked();
  });

  test('Continue → field picker auto-picks single-word fields with name hints (ET)', async ({ page }) => {
    await mockMe(page, 'user', { activeLanguage: 'ET' });
    await mockKnownWordsEmpty(page);
    await mockAnkiConnect(page, baselineAnkiMocks());
    await page.goto('/#/vocab');
    await clearStorage(page);
    await page.getByRole('button', { name: 'Connect to Anki' }).click();
    await expect(page.locator('#anki-import-stage-decks')).not.toHaveClass(/hidden/);

    // Pick the ET A1 parent — covers ETBasic notes. Then A2 — covers Basic.
    await page.locator('[data-deck-toggle="Estonian"]').click();
    await page.locator('[data-deck-check="Estonian::A1"]').check();
    await page.locator('[data-deck-check="Estonian::A2"]').check();

    await page.getByRole('button', { name: 'Continue' }).click();
    await expect(page.locator('#anki-import-stage-fields')).not.toHaveClass(/hidden/);

    // ETBasic: "Sõna" wins (ET name hint + single-word).
    const etBasicToggle = page.locator('[data-field-toggle="ETBasic"]');
    await expect(etBasicToggle.locator('.field-picker-current')).toHaveText('Sõna');

    // Basic: no ET name hint matches, but "Front" is consistently single-word.
    const basicToggle = page.locator('[data-field-toggle="Basic"]');
    await expect(basicToggle.locator('.field-picker-current')).toHaveText('Front');
  });

  test('hovering a field option shows example values in a tooltip', async ({ page }) => {
    await mockMe(page, 'user', { activeLanguage: 'ET' });
    await mockKnownWordsEmpty(page);
    await mockAnkiConnect(page, baselineAnkiMocks());
    await page.goto('/#/vocab');
    await clearStorage(page);
    await page.getByRole('button', { name: 'Connect to Anki' }).click();
    await expect(page.locator('#anki-import-stage-decks')).not.toHaveClass(/hidden/);
    await page.locator('[data-deck-toggle="Estonian"]').click();
    await page.locator('[data-deck-check="Estonian::A1"]').check();
    await page.getByRole('button', { name: 'Continue' }).click();
    await expect(page.locator('#anki-import-stage-fields')).not.toHaveClass(/hidden/);

    // Open the ETBasic picker.
    await page.locator('[data-field-toggle="ETBasic"]').click();
    const menu = page.locator('[data-field-picker="ETBasic"] .field-picker-menu');
    await expect(menu).not.toHaveClass(/hidden/);

    // Hover Tähendus → tooltip shows the two cat/dog meanings.
    await page.locator('[data-field-option="ETBasic"][data-field-value="Tähendus"]').hover();
    const tip = page.locator('.field-examples-tip');
    await expect(tip).toHaveClass(/visible/);
    await expect(tip).toBeVisible();
    await expect(tip).toContainText('cat (animal)');
    await expect(tip).toContainText('dog (animal)');

    // Regression guard: the tip is appended to <body> and the modal has
    // z-index 1000, so the tip's effective z-index must beat 1000 to render
    // above the modal card.
    const stackedAboveModal = await tip.evaluate((el) => {
      const tipZ = Number(window.getComputedStyle(el).zIndex || '0');
      const modal = document.getElementById('anki-import-modal');
      const modalZ = modal ? Number(window.getComputedStyle(modal).zIndex || '0') : 0;
      return tipZ > modalZ;
    });
    expect(stackedAboveModal).toBe(true);
  });

  test('field-picker menu uses position:fixed and escapes the modal/fields-container overflow', async ({ page }) => {
    // Build a fake set of many models so the .anki-import-fields container
    // overflows and the modal-card has both a max-height clip and a scrollbar.
    // The last model's picker, when opened, must render fully on screen.
    // 20 models is under the 25-sample-per-deck cap and tall enough to
    // overflow .anki-import-fields (max-height: 360px ≈ 7 rows).
    const modelCount = 20;
    const models = Array.from({ length: modelCount }, (_, i) => `Model ${i + 1}`);
    const noteIds = models.map((_, i) => 1000 + i);
    const notesInfo: Record<number, { noteId: number; modelName: string; fields: Record<string, { value: string; order: number }> }> = {};
    for (let i = 0; i < models.length; i++) {
      notesInfo[noteIds[i]] = {
        noteId: noteIds[i],
        modelName: models[i],
        fields: { Front: { value: `word${i}`, order: 0 }, Back: { value: `back${i}`, order: 1 } },
      };
    }
    await mockMe(page, 'user', { activeLanguage: 'ET' });
    await mockKnownWordsEmpty(page);
    await mockAnkiConnect(page, {
      version: () => 6,
      deckNames: () => ['Estonian::A1'],
      findNotes: () => noteIds,
      notesInfo: (params) => ((params.notes as number[]) || []).map(id => notesInfo[id]).filter(Boolean),
      modelFieldNames: () => ['Front', 'Back'],
    });
    await page.goto('/#/vocab');
    await clearStorage(page);
    await page.getByRole('button', { name: 'Connect to Anki' }).click();
    await expect(page.locator('#anki-import-stage-decks')).not.toHaveClass(/hidden/);
    await page.locator('[data-deck-check="Estonian::A1"]').check();
    await page.getByRole('button', { name: 'Continue' }).click();
    await expect(page.locator('#anki-import-stage-fields')).not.toHaveClass(/hidden/);

    const lastModel = models[models.length - 1];
    const lastToggle = page.locator(`[data-field-toggle="${lastModel}"]`);
    const lastMenu = page.locator(`[data-field-picker="${lastModel}"] .field-picker-menu`);

    // Scroll the fields container so the last toggle is in view.
    await lastToggle.scrollIntoViewIfNeeded();
    await lastToggle.click();
    await expect(lastMenu).not.toHaveClass(/hidden/);

    // 1. Menu is position:fixed so it can render outside any overflow ancestor.
    const positionType = await lastMenu.evaluate(el => window.getComputedStyle(el).position);
    expect(positionType).toBe('fixed');

    // 2. Menu's z-index is above the modal so it lands on top of the popup.
    const menuZ = await lastMenu.evaluate(el => Number(window.getComputedStyle(el).zIndex || '0'));
    const modalZ = await page.locator('#anki-import-modal').evaluate(el => Number(window.getComputedStyle(el).zIndex || '0'));
    expect(menuZ).toBeGreaterThan(modalZ);

    // 3. Menu is actually visible — `toBeVisible` checks that its bounding box
    // has nonzero size and isn't clipped to nothing.
    await expect(lastMenu).toBeVisible();

    // 4. Menu's rendered top is below the toggle (or flipped above, but
    // within the viewport in either case).
    const layout = await page.evaluate(([modelName]) => {
      const picker = Array.from(document.querySelectorAll<HTMLElement>('[data-field-picker]'))
        .find(el => el.getAttribute('data-field-picker') === modelName);
      if (!picker) return null;
      const toggle = picker.querySelector<HTMLElement>('.field-picker-toggle');
      const menu = picker.querySelector<HTMLElement>('.field-picker-menu');
      if (!toggle || !menu) return null;
      const t = toggle.getBoundingClientRect();
      const m = menu.getBoundingClientRect();
      return {
        toggleBottom: t.bottom,
        menuTop: m.top,
        menuBottom: m.bottom,
        viewportHeight: window.innerHeight,
      };
    }, [lastModel]);
    expect(layout).not.toBeNull();
    expect(layout!.menuBottom).toBeLessThanOrEqual(layout!.viewportHeight);
    expect(layout!.menuTop).toBeGreaterThanOrEqual(0);
  });

  test('discovery surfaces models that only appear past the per-deck sample cap', async ({ page }) => {
    // Mimic real-world deck shape: 30 notes all of model A, then 1 note of
    // model B at the very end. Before the fix, discovery sampled the first
    // 25 IDs per deck and silently missed B. Continue → field picker must
    // show BOTH models.
    const NOTES = 31;
    const noteIds: number[] = Array.from({ length: NOTES }, (_, i) => 1000 + i);
    const notesInfo: Record<number, { noteId: number; modelName: string; fields: Record<string, { value: string; order: number }> }> = {};
    for (let i = 0; i < NOTES - 1; i++) {
      notesInfo[noteIds[i]] = {
        noteId: noteIds[i],
        modelName: 'BulkModel',
        fields: { Word: { value: `word${i}`, order: 0 }, Note: { value: `meaning ${i}`, order: 1 } },
      };
    }
    // The rare model lives at index 30 — well past the 25-note cap.
    notesInfo[noteIds[NOTES - 1]] = {
      noteId: noteIds[NOTES - 1],
      modelName: 'RareModel',
      fields: { Front: { value: 'rara', order: 0 }, Back: { value: 'rare', order: 1 } },
    };

    await mockMe(page, 'user', { activeLanguage: 'ET' });
    await mockKnownWordsEmpty(page);
    await mockAnkiConnect(page, {
      version: () => 6,
      deckNames: () => ['DeckWithRareModel'],
      // The query is `deck:"<name>"` — order matters for the regression: the
      // rare model must come last so a head-sample misses it.
      findNotes: () => noteIds,
      notesInfo: (params) => ((params.notes as number[]) || []).map(id => notesInfo[id]).filter(Boolean),
      modelFieldNames: (params) => {
        if (params.modelName === 'BulkModel') return ['Word', 'Note'];
        if (params.modelName === 'RareModel') return ['Front', 'Back'];
        return [];
      },
    });
    await page.goto('/#/vocab');
    await clearStorage(page);
    await page.getByRole('button', { name: 'Connect to Anki' }).click();
    await expect(page.locator('#anki-import-stage-decks')).not.toHaveClass(/hidden/);
    await page.locator('#anki-import-clear-filter').click();
    await page.locator('[data-deck-check="DeckWithRareModel"]').check();
    await page.getByRole('button', { name: 'Continue' }).click();
    await expect(page.locator('#anki-import-stage-fields')).not.toHaveClass(/hidden/);

    // Both models render — including the one buried at the end of the deck.
    await expect(page.locator('[data-field-picker="BulkModel"]')).toBeVisible();
    await expect(page.locator('[data-field-picker="RareModel"]')).toBeVisible();
    await expect(page.locator('#anki-import-field-summary')).toContainText('2 card types found');
  });

  test('include-new toggle is off by default and the estimate excludes new cards', async ({ page }) => {
    // Notes 100/101 are studied, 102 is new (within Estonian::A1 = [100,101,102]).
    const newIds = new Set<number>([102]);
    await mockMe(page, 'user', { activeLanguage: 'ET' });
    await mockKnownWordsEmpty(page);
    await mockAnkiConnect(page, baselineAnkiMocks(newIds));
    await page.goto('/#/vocab');
    await clearStorage(page);
    await page.getByRole('button', { name: 'Connect to Anki' }).click();
    await expect(page.locator('#anki-import-stage-decks')).not.toHaveClass(/hidden/);
    await page.locator('[data-deck-toggle="Estonian"]').click();
    await page.locator('[data-deck-check="Estonian::A1"]').check();
    await page.getByRole('button', { name: 'Continue' }).click();
    await expect(page.locator('#anki-import-stage-fields')).not.toHaveClass(/hidden/);

    // Toggle is off by default.
    const toggle = page.locator('#anki-import-include-new');
    await expect(toggle).not.toBeChecked();

    // The picker defaulted to "Sõna" for ETBasic. With toggle OFF, the new
    // note (102 = "auto") is excluded — estimate counts 2 notes / 2 words.
    const estimate = page.locator('#anki-import-estimate');
    await expect(estimate).toContainText('2 notes');
    await expect(estimate).toContainText('2 words');
    await expect(estimate).toContainText('1 new card excluded');

    // Flip the toggle: now all 3 notes count.
    await toggle.check();
    await expect(estimate).toContainText('3 notes');
    await expect(estimate).toContainText('3 words');
    await expect(estimate).not.toContainText('new card excluded');

    // Flip off again: estimate snaps back to 2.
    await toggle.uncheck();
    await expect(estimate).toContainText('2 notes');
  });

  test('estimate does not double-count new cards as also "skipped"', async ({ page }) => {
    // Note 102 ("auto") is the only new note. With toggle OFF it must show up
    // exactly once — under "1 new card excluded" — not also as "(1 skipped)".
    const newIds = new Set<number>([102]);
    await mockMe(page, 'user', { activeLanguage: 'ET' });
    await mockKnownWordsEmpty(page);
    await mockAnkiConnect(page, baselineAnkiMocks(newIds));
    await page.goto('/#/vocab');
    await clearStorage(page);
    await page.getByRole('button', { name: 'Connect to Anki' }).click();
    await expect(page.locator('#anki-import-stage-decks')).not.toHaveClass(/hidden/);
    await page.locator('[data-deck-toggle="Estonian"]').click();
    await page.locator('[data-deck-check="Estonian::A1"]').check();
    await page.getByRole('button', { name: 'Continue' }).click();
    await expect(page.locator('#anki-import-stage-fields')).not.toHaveClass(/hidden/);

    const estimate = page.locator('#anki-import-estimate');
    // 3 total notes, 1 new excluded → 2 active.
    await expect(estimate).toContainText('2 notes');
    await expect(estimate).toContainText('1 new card excluded');
    // The old wording included a parenthetical "(N skipped)" that conflated
    // toggle exclusions with empty-field skips. The new copy says
    // "with empty or skipped field" only when that's actually the reason —
    // here there are no field-empty notes, so this phrase must NOT appear.
    await expect(estimate).not.toContainText(/skipped/);
    await expect(estimate).not.toContainText(/empty or skipped field/);
  });

  test('estimate reports field-empty skips separately from new-card exclusions', async ({ page }) => {
    // Mix: one new note (102 "auto"), one studied note with an empty Word
    // field. Toggle OFF must report both, with separate counts and labels —
    // 1 new card excluded AND 1 note with empty/skipped field.
    const newIds = new Set<number>([102]);
    const customNotes: Record<number, { noteId: number; modelName: string; fields: Record<string, { value: string; order: number }> }> = {
      // Studied note with empty Sõna field.
      150: { noteId: 150, modelName: 'ETBasic', fields: { 'Sõna': { value: '', order: 0 }, 'Tähendus': { value: '(no word)', order: 1 }, 'Lause': { value: '', order: 2 } } },
      // Studied note with a real value.
      151: { noteId: 151, modelName: 'ETBasic', fields: { 'Sõna': { value: 'kala', order: 0 }, 'Tähendus': { value: 'fish', order: 1 }, 'Lause': { value: '', order: 2 } } },
      // New note (excluded by toggle).
      102: { noteId: 102, modelName: 'ETBasic', fields: { 'Sõna': { value: 'auto', order: 0 }, 'Tähendus': { value: 'car', order: 1 }, 'Lause': { value: '', order: 2 } } },
    };
    await mockMe(page, 'user', { activeLanguage: 'ET' });
    await mockKnownWordsEmpty(page);
    await mockAnkiConnect(page, {
      version: () => 6,
      deckNames: () => ['SmallDeck'],
      findNotes: (params) => {
        const q = String(params.query || '');
        const all = [102, 150, 151];
        if (/-is:new\b/.test(q)) return all.filter(id => !newIds.has(id));
        return all;
      },
      notesInfo: (params) => ((params.notes as number[]) || []).map(id => customNotes[id]).filter(Boolean),
      modelFieldNames: () => ['Sõna', 'Tähendus', 'Lause'],
    });
    await page.goto('/#/vocab');
    await clearStorage(page);
    await page.getByRole('button', { name: 'Connect to Anki' }).click();
    await expect(page.locator('#anki-import-stage-decks')).not.toHaveClass(/hidden/);
    await page.locator('#anki-import-clear-filter').click();
    await page.locator('[data-deck-check="SmallDeck"]').check();
    await page.getByRole('button', { name: 'Continue' }).click();
    await expect(page.locator('#anki-import-stage-fields')).not.toHaveClass(/hidden/);

    const estimate = page.locator('#anki-import-estimate');
    // 3 total, 1 new excluded, 1 with empty Sõna → 1 active.
    await expect(estimate).toContainText('1 note ');
    await expect(estimate).toContainText('1 new card excluded');
    await expect(estimate).toContainText('1 note with empty or skipped field');
  });

  test('include-new state persists across modal reopen', async ({ page }) => {
    const newIds = new Set<number>([102]);
    await mockMe(page, 'user', { activeLanguage: 'ET' });
    await mockKnownWordsEmpty(page);
    await mockAnkiConnect(page, baselineAnkiMocks(newIds));
    await page.goto('/#/vocab');
    await clearStorage(page);
    await page.getByRole('button', { name: 'Connect to Anki' }).click();
    await expect(page.locator('#anki-import-stage-decks')).not.toHaveClass(/hidden/);
    await page.locator('[data-deck-toggle="Estonian"]').click();
    await page.locator('[data-deck-check="Estonian::A1"]').check();
    await page.getByRole('button', { name: 'Continue' }).click();
    await page.locator('#anki-import-include-new').check();
    // Close + reopen.
    await page.locator('#anki-import-modal-close').click();
    await expect(page.locator('#anki-import-modal')).toHaveClass(/hidden/);
    await page.getByRole('button', { name: 'Connect to Anki' }).click();
    await expect(page.locator('#anki-import-stage-decks')).not.toHaveClass(/hidden/);
    await page.getByRole('button', { name: 'Continue' }).click();
    await expect(page.locator('#anki-import-stage-fields')).not.toHaveClass(/hidden/);
    await expect(page.locator('#anki-import-include-new')).toBeChecked();
  });

  test('Import respects the include-new toggle and only sends studied-note words by default', async ({ page }) => {
    // 100 (kassi), 101 (koer) studied; 102 (auto) is new.
    const newIds = new Set<number>([102]);
    await mockMe(page, 'user', { activeLanguage: 'ET' });
    await mockKnownWordsEmpty(page);
    await mockAnkiConnect(page, baselineAnkiMocks(newIds));

    let importBody: { lang?: string; words?: string[] } | null = null;
    await page.route('**/api/known-words', async (route) => {
      if (route.request().method() === 'POST') {
        importBody = route.request().postDataJSON();
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ imported: [], unresolved: [] }),
        });
      } else {
        await route.fallback();
      }
    });

    await page.goto('/#/vocab');
    await clearStorage(page);
    await page.getByRole('button', { name: 'Connect to Anki' }).click();
    await expect(page.locator('#anki-import-stage-decks')).not.toHaveClass(/hidden/);
    await page.locator('[data-deck-toggle="Estonian"]').click();
    await page.locator('[data-deck-check="Estonian::A1"]').check();
    await page.getByRole('button', { name: 'Continue' }).click();
    await expect(page.locator('#anki-import-stage-fields')).not.toHaveClass(/hidden/);
    // Default toggle OFF — "auto" is new and should be excluded.
    await page.getByRole('button', { name: 'Import', exact: true }).click();
    await expect.poll(() => importBody).not.toBeNull();
    expect(new Set(importBody!.words)).toEqual(new Set(['kassi', 'koer']));
  });

  test('Import with include-new on imports everything', async ({ page }) => {
    const newIds = new Set<number>([102]);
    await mockMe(page, 'user', { activeLanguage: 'ET' });
    await mockKnownWordsEmpty(page);
    await mockAnkiConnect(page, baselineAnkiMocks(newIds));

    let importBody: { lang?: string; words?: string[] } | null = null;
    await page.route('**/api/known-words', async (route) => {
      if (route.request().method() === 'POST') {
        importBody = route.request().postDataJSON();
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ imported: [], unresolved: [] }),
        });
      } else {
        await route.fallback();
      }
    });

    await page.goto('/#/vocab');
    await clearStorage(page);
    await page.getByRole('button', { name: 'Connect to Anki' }).click();
    await expect(page.locator('#anki-import-stage-decks')).not.toHaveClass(/hidden/);
    await page.locator('[data-deck-toggle="Estonian"]').click();
    await page.locator('[data-deck-check="Estonian::A1"]').check();
    await page.getByRole('button', { name: 'Continue' }).click();
    await expect(page.locator('#anki-import-stage-fields')).not.toHaveClass(/hidden/);
    await page.locator('#anki-import-include-new').check();
    await page.getByRole('button', { name: 'Import', exact: true }).click();
    await expect.poll(() => importBody).not.toBeNull();
    expect(new Set(importBody!.words)).toEqual(new Set(['kassi', 'koer', 'auto']));
  });

  test('include-suspended toggle is off by default and the estimate excludes suspended cards', async ({ page }) => {
    // Notes 100/101 active; 102 suspended (within Estonian::A1 = [100,101,102]).
    const suspendedIds = new Set<number>([102]);
    await mockMe(page, 'user', { activeLanguage: 'ET' });
    await mockKnownWordsEmpty(page);
    await mockAnkiConnect(page, baselineAnkiMocks(new Set(), suspendedIds));
    await page.goto('/#/vocab');
    await clearStorage(page);
    await page.getByRole('button', { name: 'Connect to Anki' }).click();
    await expect(page.locator('#anki-import-stage-decks')).not.toHaveClass(/hidden/);
    await page.locator('[data-deck-toggle="Estonian"]').click();
    await page.locator('[data-deck-check="Estonian::A1"]').check();
    await page.getByRole('button', { name: 'Continue' }).click();
    await expect(page.locator('#anki-import-stage-fields')).not.toHaveClass(/hidden/);

    const toggle = page.locator('#anki-import-include-suspended');
    await expect(toggle).not.toBeChecked();

    const estimate = page.locator('#anki-import-estimate');
    await expect(estimate).toContainText('2 notes');
    await expect(estimate).toContainText('1 suspended card excluded');
    // Toggle on → suspended note counts.
    await toggle.check();
    await expect(estimate).toContainText('3 notes');
    await expect(estimate).not.toContainText('suspended card excluded');
  });

  test('include-suspended state persists across modal reopen', async ({ page }) => {
    const suspendedIds = new Set<number>([102]);
    await mockMe(page, 'user', { activeLanguage: 'ET' });
    await mockKnownWordsEmpty(page);
    await mockAnkiConnect(page, baselineAnkiMocks(new Set(), suspendedIds));
    await page.goto('/#/vocab');
    await clearStorage(page);
    await page.getByRole('button', { name: 'Connect to Anki' }).click();
    await expect(page.locator('#anki-import-stage-decks')).not.toHaveClass(/hidden/);
    await page.locator('[data-deck-toggle="Estonian"]').click();
    await page.locator('[data-deck-check="Estonian::A1"]').check();
    await page.getByRole('button', { name: 'Continue' }).click();
    await page.locator('#anki-import-include-suspended').check();
    await page.locator('#anki-import-modal-close').click();
    await page.getByRole('button', { name: 'Connect to Anki' }).click();
    await page.getByRole('button', { name: 'Continue' }).click();
    await expect(page.locator('#anki-import-stage-fields')).not.toHaveClass(/hidden/);
    await expect(page.locator('#anki-import-include-suspended')).toBeChecked();
  });

  test('Import respects the include-suspended toggle by default (suspended notes excluded)', async ({ page }) => {
    // 100 (kassi), 101 (koer) active; 102 (auto) is suspended.
    const suspendedIds = new Set<number>([102]);
    await mockMe(page, 'user', { activeLanguage: 'ET' });
    await mockKnownWordsEmpty(page);
    await mockAnkiConnect(page, baselineAnkiMocks(new Set(), suspendedIds));

    let importBody: { lang?: string; words?: string[] } | null = null;
    await page.route('**/api/known-words', async (route) => {
      if (route.request().method() === 'POST') {
        importBody = route.request().postDataJSON();
        await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ imported: [], unresolved: [] }) });
      } else {
        await route.fallback();
      }
    });

    await page.goto('/#/vocab');
    await clearStorage(page);
    await page.getByRole('button', { name: 'Connect to Anki' }).click();
    await expect(page.locator('#anki-import-stage-decks')).not.toHaveClass(/hidden/);
    await page.locator('[data-deck-toggle="Estonian"]').click();
    await page.locator('[data-deck-check="Estonian::A1"]').check();
    await page.getByRole('button', { name: 'Continue' }).click();
    await page.getByRole('button', { name: 'Import', exact: true }).click();
    await expect.poll(() => importBody).not.toBeNull();
    expect(new Set(importBody!.words)).toEqual(new Set(['kassi', 'koer']));
  });

  test('Estimate breaks new vs suspended exclusions into separate counts', async ({ page }) => {
    // 100 is new+suspended (counted once), 101 is suspended only,
    // 102 is new only, plus 200/201 (Estonian::A2) which are active.
    const newIds = new Set<number>([100, 102]);
    const suspendedIds = new Set<number>([100, 101]);
    await mockMe(page, 'user', { activeLanguage: 'ET' });
    await mockKnownWordsEmpty(page);
    await mockAnkiConnect(page, baselineAnkiMocks(newIds, suspendedIds));
    await page.goto('/#/vocab');
    await clearStorage(page);
    await page.getByRole('button', { name: 'Connect to Anki' }).click();
    await expect(page.locator('#anki-import-stage-decks')).not.toHaveClass(/hidden/);
    await page.locator('[data-deck-toggle="Estonian"]').click();
    await page.locator('[data-deck-check="Estonian::A1"]').check();
    await page.locator('[data-deck-check="Estonian::A2"]').check();
    await page.getByRole('button', { name: 'Continue' }).click();
    await expect(page.locator('#anki-import-stage-fields')).not.toHaveClass(/hidden/);
    const estimate = page.locator('#anki-import-estimate');
    // Filter order: new first (100, 102 dropped), then suspended (101 dropped
    // since 100 was already gone). Active = 200, 201 → 2 notes.
    await expect(estimate).toContainText('2 notes');
    await expect(estimate).toContainText('2 new cards excluded');
    await expect(estimate).toContainText('1 suspended card excluded');
  });

  test('clicking the field-picker toggle opens its menu (incl. model names with spaces/parens)', async ({ page }) => {
    // Anki ships built-in models like "Basic (and reversed card)" whose names
    // contain spaces and parentheses. Earlier the toggle handler looked the
    // picker up via a CSS attribute selector with CSS.escape — that emits
    // backslash-escapes that the browser then matches literally, so the
    // selector silently returned nothing and the menu never opened.
    const fancyModel = 'Basic (and reversed card)';
    const noteId = 999;
    const notesInfo: Record<number, { noteId: number; modelName: string; fields: Record<string, { value: string; order: number }> }> = {
      [noteId]: {
        noteId,
        modelName: fancyModel,
        fields: { Front: { value: 'maja', order: 0 }, Back: { value: 'house', order: 1 } },
      },
    };
    await mockMe(page, 'user', { activeLanguage: 'ET' });
    await mockKnownWordsEmpty(page);
    await mockAnkiConnect(page, {
      version: () => 6,
      deckNames: () => ['Estonian::A1'],
      findNotes: () => [noteId],
      notesInfo: (params) => ((params.notes as number[]) || []).map(id => notesInfo[id]).filter(Boolean),
      modelFieldNames: () => ['Front', 'Back'],
    });
    await page.goto('/#/vocab');
    await clearStorage(page);
    await page.getByRole('button', { name: 'Connect to Anki' }).click();
    await expect(page.locator('#anki-import-stage-decks')).not.toHaveClass(/hidden/);
    await page.locator('[data-deck-check="Estonian::A1"]').check();
    await page.getByRole('button', { name: 'Continue' }).click();
    await expect(page.locator('#anki-import-stage-fields')).not.toHaveClass(/hidden/);

    // The toggle exists with the literal model name (rendered via escapeAttr).
    const toggle = page.locator(`[data-field-toggle="${fancyModel}"]`);
    const menu = page.locator(`[data-field-picker="${fancyModel}"] .field-picker-menu`);
    await expect(toggle).toBeVisible();
    await expect(menu).toHaveClass(/hidden/);

    // First click → opens.
    await toggle.click();
    await expect(menu).not.toHaveClass(/hidden/);
    await expect(menu).toBeVisible();

    // Click again → closes (toggle behaviour).
    await toggle.click();
    await expect(menu).toHaveClass(/hidden/);

    // Open again and pick an option — menu closes, toggle label updates.
    await toggle.click();
    await expect(menu).not.toHaveClass(/hidden/);
    await page.locator(`[data-field-option="${fancyModel}"][data-field-value="Back"]`).click();
    await expect(menu).toHaveClass(/hidden/);
    await expect(toggle.locator('.field-picker-current')).toHaveText('Back');
  });

  test('Anki import cleans textbook notation before posting (acute accents, /n stems, parens, punctuation, ellipses)', async ({ page }) => {
    // Each note's `Word` field exercises one cleanup rule. The expected
    // post-cleanup form is what we should see in the body of /api/known-words.
    const fixtures: Array<[number, string]> = [
      [300, 'anna/n'],         // → "annan"
      [301, 'diréktor'],       // → "direktor"
      [302, 'iial(gi)'],       // → "iial"
      [303, 'tool (n)'],       // → "tool"
      [304, 'mine!'],          // → "mine"
      [305, '… all'],          // → "" (dropped)
      [306, 'kissa'],          // → "kissa" (unchanged passthrough)
    ];
    const notesInfo: Record<number, { noteId: number; modelName: string; fields: Record<string, { value: string; order: number }> }> = {};
    for (const [id, val] of fixtures) {
      notesInfo[id] = { noteId: id, modelName: 'Textbook', fields: { Word: { value: val, order: 0 } } };
    }
    const noteIds = fixtures.map(([id]) => id);

    await mockMe(page, 'user', { activeLanguage: 'ET' });
    await mockKnownWordsEmpty(page);
    await mockAnkiConnect(page, {
      version: () => 6,
      deckNames: () => ['TextbookDeck'],
      // Everything is "studied" in this fixture so we don't need the toggle.
      findNotes: () => noteIds,
      notesInfo: (params) => ((params.notes as number[]) || []).map(id => notesInfo[id]).filter(Boolean),
      modelFieldNames: () => ['Word'],
    });

    let importBody: { lang?: string; words?: string[] } | null = null;
    await page.route('**/api/known-words', async (route) => {
      if (route.request().method() === 'POST') {
        importBody = route.request().postDataJSON();
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ imported: [], unresolved: [] }),
        });
      } else {
        await route.fallback();
      }
    });

    await page.goto('/#/vocab');
    await clearStorage(page);
    await page.getByRole('button', { name: 'Connect to Anki' }).click();
    await expect(page.locator('#anki-import-stage-decks')).not.toHaveClass(/hidden/);
    await page.locator('#anki-import-clear-filter').click();
    await page.locator('[data-deck-check="TextbookDeck"]').check();
    await page.getByRole('button', { name: 'Continue' }).click();
    await expect(page.locator('#anki-import-stage-fields')).not.toHaveClass(/hidden/);

    // The ellipsis entry is dropped during cleanup → 6 distinct surface forms
    // out of 7 notes. The "notes" count reports notes that contributed a
    // non-empty field value pre-cleanup (all 7 here, since "… all" is a
    // non-empty string), so notes=7 / words=6 — the cleanup happens between.
    await expect(page.locator('#anki-import-estimate')).toContainText('7 notes');
    await expect(page.locator('#anki-import-estimate')).toContainText('6 words');

    await page.getByRole('button', { name: 'Import', exact: true }).click();
    await expect.poll(() => importBody).not.toBeNull();
    expect(importBody!.lang).toBe('ET');
    expect(new Set(importBody!.words)).toEqual(new Set([
      'annan', 'direktor', 'iial', 'tool', 'mine', 'kissa',
    ]));
    // The "… all" ellipsis entry must be dropped, not sent through as garbage.
    expect(importBody!.words!.some(w => w.includes('…'))).toBe(false);
  });

  test('Replace mode confirms before running, then PUTs to /api/known-words', async ({ page }) => {
    await mockMe(page, 'user', { activeLanguage: 'ET' });
    await mockKnownWordsEmpty(page);
    await mockAnkiConnect(page, baselineAnkiMocks());

    let putBody: { lang?: string; words?: string[] } | null = null;
    let postCalled = false;
    await page.route('**/api/known-words', async (route) => {
      const m = route.request().method();
      if (m === 'PUT') {
        putBody = route.request().postDataJSON();
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            added:   [{ lemma: 'kassi', pos: 'NOUN', lang: 'ET' }],
            removed: [{ lemma: 'gone',  pos: 'NOUN', lang: 'ET' }],
            unresolved: [],
          }),
        });
        return;
      }
      if (m === 'POST') {
        postCalled = true;
      }
      await route.fallback();
    });

    await page.goto('/#/vocab');
    await clearStorage(page);
    await page.getByRole('button', { name: 'Connect to Anki' }).click();
    await expect(page.locator('#anki-import-stage-decks')).not.toHaveClass(/hidden/);
    await page.locator('[data-deck-toggle="Estonian"]').click();
    await page.locator('[data-deck-check="Estonian::A1"]').check();
    await page.getByRole('button', { name: 'Continue' }).click();
    await expect(page.locator('#anki-import-stage-fields')).not.toHaveClass(/hidden/);

    // Default off.
    const replaceCb = page.locator('#anki-import-replace-mode');
    await expect(replaceCb).not.toBeChecked();

    // Enable + import → confirmation dialog must appear.
    await replaceCb.check();
    await page.getByRole('button', { name: 'Import', exact: true }).click();
    const dialog = page.locator('#dialog-modal');
    await expect(dialog).not.toHaveClass(/hidden/);
    await expect(dialog).toContainText(/Replace Estonian vocabulary/i);
    await expect(dialog).toContainText(/removed/i);

    // Cancel: no network call.
    await page.getByRole('button', { name: 'Cancel' }).click();
    await expect(dialog).toHaveClass(/hidden/);
    expect(putBody).toBeNull();
    expect(postCalled).toBe(false);

    // Re-run + confirm: PUT fires (not POST).
    await page.getByRole('button', { name: 'Import', exact: true }).click();
    await expect(dialog).not.toHaveClass(/hidden/);
    await page.locator('#dialog-modal-confirm').click();
    await expect.poll(() => putBody).not.toBeNull();
    expect(putBody!.lang).toBe('ET');
    // All three Estonian::A1 notes contribute (none are flagged new in this
    // fixture; the suspended/new toggles are unrelated to replace mode).
    expect(new Set(putBody!.words)).toEqual(new Set(['kassi', 'koer', 'auto']));
    expect(postCalled).toBe(false);
  });

  test('Replace toggle persists across modal reopen', async ({ page }) => {
    await mockMe(page, 'user', { activeLanguage: 'ET' });
    await mockKnownWordsEmpty(page);
    await mockAnkiConnect(page, baselineAnkiMocks());
    await page.goto('/#/vocab');
    await clearStorage(page);
    await page.getByRole('button', { name: 'Connect to Anki' }).click();
    await expect(page.locator('#anki-import-stage-decks')).not.toHaveClass(/hidden/);
    await page.locator('[data-deck-toggle="Estonian"]').click();
    await page.locator('[data-deck-check="Estonian::A1"]').check();
    await page.getByRole('button', { name: 'Continue' }).click();
    await page.locator('#anki-import-replace-mode').check();
    await page.locator('#anki-import-modal-close').click();
    await page.getByRole('button', { name: 'Connect to Anki' }).click();
    await page.getByRole('button', { name: 'Continue' }).click();
    await expect(page.locator('#anki-import-stage-fields')).not.toHaveClass(/hidden/);
    await expect(page.locator('#anki-import-replace-mode')).toBeChecked();
  });

  test('"Sync from Anki" button stays hidden until a successful import lands', async ({ page }) => {
    await mockMe(page, 'user', { activeLanguage: 'ET' });
    await mockKnownWordsEmpty(page);
    await mockAnkiConnect(page, baselineAnkiMocks());
    await page.route('**/api/known-words', async (route) => {
      if (route.request().method() === 'POST') {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ imported: [], unresolved: [] }),
        });
      } else {
        await route.fallback();
      }
    });

    await page.goto('/#/vocab');
    await clearStorage(page);
    const syncBtn = page.locator('#vocab-anki-sync');
    await expect(syncBtn).toHaveClass(/hidden/);

    // Run a full import once.
    await page.getByRole('button', { name: 'Connect to Anki' }).click();
    await expect(page.locator('#anki-import-stage-decks')).not.toHaveClass(/hidden/);
    await page.locator('[data-deck-toggle="Estonian"]').click();
    await page.locator('[data-deck-check="Estonian::A1"]').check();
    await page.getByRole('button', { name: 'Continue' }).click();
    await page.getByRole('button', { name: 'Import', exact: true }).click();
    // Wait for the Done button to surface, signalling the import wrote the
    // success timestamp via recordAnkiSyncTime.
    await expect(page.locator('#anki-import-done-actions')).not.toHaveClass(/hidden/);
    await page.locator('#anki-import-done').click();
    await expect(page.locator('#anki-import-modal')).toHaveClass(/hidden/);

    // Sync button now visible on the vocab card.
    await expect(syncBtn).not.toHaveClass(/hidden/);
    await expect(syncBtn).toBeVisible();
  });

  test('Sync button skips the deck + field pickers and runs import directly', async ({ page }) => {
    // Pre-seed localStorage with prior prefs so the sync flow has something
    // to run with. This mirrors the "user already did one successful
    // import" state without having to walk through the popup again.
    await mockMe(page, 'user', { activeLanguage: 'ET' });
    await mockKnownWordsEmpty(page);
    await mockAnkiConnect(page, baselineAnkiMocks());
    let postBody: { lang?: string; words?: string[] } | null = null;
    await page.route('**/api/known-words', async (route) => {
      if (route.request().method() === 'POST') {
        postBody = route.request().postDataJSON();
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ imported: [], unresolved: [] }),
        });
      } else {
        await route.fallback();
      }
    });
    await page.goto('/#/vocab');
    await page.evaluate(() => {
      localStorage.setItem('finest:anki-import:ET', JSON.stringify({
        filter:           'estonian|eesti',
        decks:            ['Estonian::A1'],
        fieldByModel:     { ETBasic: 'Sõna' },
        includeNew:       false,
        includeSuspended: false,
        replaceMode:      false,
        lastSyncAt:       Date.now(),
      }));
    });
    // Reload so renderVocabPage runs against the seeded prefs.
    await page.reload();
    const syncBtn = page.locator('#vocab-anki-sync');
    await expect(syncBtn).toBeVisible();

    await syncBtn.click();
    // Modal opens, skips deck + field stages, runs import.
    await expect(page.locator('#anki-import-modal')).not.toHaveClass(/hidden/);
    await expect(page.locator('#anki-import-stage-running')).not.toHaveClass(/hidden/, { timeout: 10000 });
    await expect.poll(() => postBody).not.toBeNull();
    expect(postBody!.lang).toBe('ET');
    // Only Estonian::A1 was saved, so only its notes contribute.
    expect(new Set(postBody!.words)).toEqual(new Set(['kassi', 'koer', 'auto']));
  });

  test('Sync falls through to manual picker if saved decks no longer exist', async ({ page }) => {
    await mockMe(page, 'user', { activeLanguage: 'ET' });
    await mockKnownWordsEmpty(page);
    await mockAnkiConnect(page, baselineAnkiMocks());
    await page.goto('/#/vocab');
    await page.evaluate(() => {
      // Reference decks that the mocked Anki no longer knows about.
      localStorage.setItem('finest:anki-import:ET', JSON.stringify({
        filter:           '',
        decks:            ['Gone::Deck1', 'Gone::Deck2'],
        fieldByModel:     {},
        includeNew:       false,
        includeSuspended: false,
        replaceMode:      false,
        lastSyncAt:       Date.now(),
      }));
    });
    await page.reload();
    await page.locator('#vocab-anki-sync').click();
    // Stale deck selection cleared → fall through to deck picker, not auto-run.
    await expect(page.locator('#anki-import-stage-decks')).not.toHaveClass(/hidden/);
    await expect(page.locator('#anki-import-stage-running')).toHaveClass(/hidden/);
  });

  test('Import posts deduped words to /api/known-words from the active field per model', async ({ page }) => {
    await mockMe(page, 'user', { activeLanguage: 'ET' });
    await mockKnownWordsEmpty(page);
    const ankiLog: AnkiCall[] = [];
    await mockAnkiConnect(page, baselineAnkiMocks(), ankiLog);

    let importBody: { lang?: string; words?: string[] } | null = null;
    await page.route('**/api/known-words', async (route) => {
      if (route.request().method() === 'POST') {
        importBody = route.request().postDataJSON();
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ imported: [], unresolved: [] }),
        });
      } else {
        await route.fallback();
      }
    });

    await page.goto('/#/vocab');
    await clearStorage(page);
    await page.getByRole('button', { name: 'Connect to Anki' }).click();
    await expect(page.locator('#anki-import-stage-decks')).not.toHaveClass(/hidden/);
    await page.locator('[data-deck-toggle="Estonian"]').click();
    await page.locator('[data-deck-check="Estonian::A1"]').check();
    await page.locator('[data-deck-check="Estonian::A2"]').check();
    await page.getByRole('button', { name: 'Continue' }).click();
    await expect(page.locator('#anki-import-stage-fields')).not.toHaveClass(/hidden/);

    await page.getByRole('button', { name: 'Import', exact: true }).click();
    await expect(page.locator('#anki-import-stage-running')).not.toHaveClass(/hidden/);
    await expect.poll(() => importBody).not.toBeNull();

    expect(importBody!.lang).toBe('ET');
    // ETBasic→Sõna gives [kassi, koer, auto]; Basic→Front gives [maja, raamat].
    // Order isn't asserted because the deck iteration order is implementation
    // detail — we just need the set to be right and deduped.
    expect(new Set(importBody!.words)).toEqual(new Set(['kassi', 'koer', 'auto', 'maja', 'raamat']));
  });
});
