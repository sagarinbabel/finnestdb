"use strict";
// FinEstDB — frontend with three role-aware surfaces:
//   anonymous landing/about/signin, authenticated user product, admin workbench.
const MAX_CHARS = 1500000;
const POS_LABELS = {
    NOUN: 'Noun',
    VERB: 'Verb',
    ADJ: 'Adjective',
    ADV: 'Adverb',
    PRON: 'Pronoun',
    DET: 'Determiner',
    ADP: 'Adposition',
    NUM: 'Numeral',
    PUNCT: 'Punctuation',
    SYM: 'Symbol',
    INTJ: 'Interjection',
    CCONJ: 'Conjunction',
    SCONJ: 'Conjunction',
    PART: 'Particle',
    AUX: 'Auxiliary',
    PROPN: 'Proper noun',
    X: 'Other',
};
const POS_ABBREV = {
    NOUN: 'n',
    VERB: 'v',
    ADJ: 'adj',
    ADV: 'adv',
    PRON: 'pron',
    DET: 'det',
    ADP: 'adp',
    NUM: 'num',
    PUNCT: 'punct',
    SYM: 'sym',
    INTJ: 'intj',
    CCONJ: 'conj',
    SCONJ: 'conj',
    PART: 'part',
    AUX: 'aux',
    PROPN: 'pn',
    X: '?',
};
// Closed set of language codes the app supports today. Source of truth for
// the Languages page checkboxes and any code that needs to enumerate options
// without hitting the server. Keep in sync with store.SupportedLanguages.
const SUPPORTED_LANGUAGES = ['FI', 'ET'];
const LANGUAGE_NAMES = { FI: 'Finnish', ET: 'Estonian' };
function languageName(lang) {
    return LANGUAGE_NAMES[lang] || lang;
}
// Inline SVG flags — flat, cartoony, system-font-independent. Returned as a
// string and injected via innerHTML next to language labels. Each flag uses
// `class="lang-flag"`; CSS sizes them via the parent's font-size (height: 1em)
// and adds the rounded-corner / soft-shadow treatment that gives them a
// sticker-like look. Pass `tooltip` to attach data-tooltip directly to the
// SVG element so the portal tooltip fires only over actual flag pixels —
// not the surrounding flexbox gap of any wrapper. Keep in sync with
// SUPPORTED_LANGUAGES.
function languageFlag(lang, tooltip) {
    const tipAttr = tooltip ? ` data-tooltip="${escapeAttr(tooltip)}"` : '';
    if (lang === 'FI') {
        // Finland: 18:11 white field with a blue Nordic cross.
        // Cross arms: 4:3:4 vertical, 5:3:10 horizontal (spec proportions).
        return `<svg class="lang-flag" viewBox="0 0 18 11" preserveAspectRatio="xMidYMid meet" aria-hidden="true" focusable="false"${tipAttr}>
            <rect width="18" height="11" fill="#ffffff"/>
            <rect y="4" width="18" height="3" fill="#003580"/>
            <rect x="5" width="3" height="11" fill="#003580"/>
        </svg>`;
    }
    if (lang === 'ET') {
        // Estonia: 11:7 horizontal triband, blue / black / white.
        return `<svg class="lang-flag" viewBox="0 0 33 21" preserveAspectRatio="xMidYMid meet" aria-hidden="true" focusable="false"${tipAttr}>
            <rect width="33" height="21" fill="#ffffff"/>
            <rect width="33" height="7" fill="#0072CE"/>
            <rect y="7" width="33" height="7" fill="#000000"/>
        </svg>`;
    }
    return '';
}
const state = {
    user: null,
    dashboard: null,
    decks: [],
    officialDecks: [],
    officialDecksLoaded: false,
    decksTab: 'mine',
    role: 'anon',
    currentResults: null,
    currentContext: 'inspect',
    currentParserMode: 'basic',
    currentTextPreview: '',
    currentSourceText: '',
    currentDeckID: null,
    currentDeckCreatedAt: '',
    currentRow: null,
    currentSort: { key: 'row', dir: 'asc' },
    currentPOSFilter: 'all',
    currentLemmaStates: new Map(),
    pendingLemmaStates: new Set(),
    currentReviewCard: null,
    reviewDeckFilter: '',
    adminFeedback: [],
    adminFeedbackStatus: 'submitted',
    // Active language drives the deck list filter, Inspect/Known-Words defaults,
    // and Review queue. learningLanguages is the user's opt-in set; the nav
    // dropdown only shows entries from this list. Both are hydrated from
    // /api/me; defaults are FI active, both languages learning.
    learningLanguages: ['FI', 'ET'],
    activeLanguage: 'FI',
    languageStats: {},
    knownWords: [],
    // resultsEpub: the EPUB whose parse is currently displayed on the
    // results page (drives the chapter sidebar + cache). Each form's
    // "loaded but not yet parsed" EPUB lives on its own ParseFormElements.
    // The global slot is set when a form's Parse press completes against
    // a loaded EPUB; cleared on the next text-only parse.
    resultsEpub: null,
    // Per-chapter ParseResponse cache for the results-context EPUB. Key
    // -1 = whole book, 0..N-1 = chapter index.
    epubChapterCache: new Map(),
    // Sidebar selection: -1 = whole book, 0..N-1 = chapter, null = no EPUB
    // context (sidebar hidden).
    activeChapterIdx: null,
};
const NOUN_POS = ['NOUN', 'PROPN'];
const VERB_POS = ['VERB', 'AUX'];
const ADJ_POS = ['ADJ'];
const ADV_POS = ['ADV'];
const OTHER_POS = ['PRON', 'DET', 'ADP', 'NUM', 'CCONJ', 'SCONJ', 'PART', 'INTJ', 'X', 'SYM', 'PUNCT'];
// ── Theme ──────────────────────────────────────────────────────────────────
function initTheme() {
    const saved = localStorage.getItem('theme') || 'dark';
    applyTheme(saved);
}
function applyTheme(theme) {
    document.documentElement.setAttribute('data-theme', theme);
    document.querySelectorAll('.theme-icon').forEach(el => {
        el.textContent = theme === 'light' ? '🌙' : '☀️';
    });
}
function toggleTheme() {
    const current = document.documentElement.getAttribute('data-theme') || 'light';
    const next = current === 'light' ? 'dark' : 'light';
    localStorage.setItem('theme', next);
    applyTheme(next);
}
// ── Toast notifications ────────────────────────────────────────────────────
function showToast(message, type = 'info', duration = 3000) {
    const container = document.getElementById('toast-container');
    if (!container)
        return;
    const toast = document.createElement('div');
    toast.className = `toast ${type}`;
    toast.textContent = message;
    container.appendChild(toast);
    setTimeout(() => {
        toast.classList.add('hiding');
        setTimeout(() => toast.remove(), 300);
    }, duration);
}
// Tracks the currently-open dialog so a superseding open() can resolve the
// prior one to null (cancel-equivalent) before stealing the modal markup.
// Without this, the first Promise would hang forever — listeners get torn
// down, but the resolve() function was never called.
let activeDialog = null;
function openDialog(opts) {
    return new Promise(resolve => {
        const modal = document.getElementById('dialog-modal');
        const titleEl = document.getElementById('dialog-modal-title');
        const messageEl = document.getElementById('dialog-modal-message');
        const inputWrap = document.getElementById('dialog-modal-input-wrap');
        const inputLabel = document.getElementById('dialog-modal-input-label');
        const input = document.getElementById('dialog-modal-input');
        const backdrop = document.getElementById('dialog-modal-backdrop');
        const confirmBtn = document.getElementById('dialog-modal-confirm');
        const cancelBtn = document.getElementById('dialog-modal-cancel');
        if (!modal || !titleEl || !messageEl || !inputWrap || !input || !confirmBtn || !cancelBtn || !backdrop) {
            // Markup missing — fall back to native dialogs so the user never
            // gets a silently dropped confirmation.
            if (opts.prompt)
                resolve(window.prompt(opts.message, opts.initialValue) ?? null);
            else
                resolve(window.confirm(opts.message) ? '' : null);
            return;
        }
        // If a previous dialog is still open, cancel it first. cancel()
        // both removes its listeners AND resolves its promise so awaiters
        // unblock instead of hanging.
        if (activeDialog)
            activeDialog.cancel();
        titleEl.textContent = opts.title ?? (opts.prompt ? 'Edit' : 'Confirm');
        messageEl.textContent = opts.message;
        confirmBtn.textContent = opts.confirmLabel ?? (opts.prompt ? 'Save' : 'OK');
        cancelBtn.textContent = opts.cancelLabel ?? 'Cancel';
        confirmBtn.classList.toggle('btn-danger', !!opts.danger);
        if (opts.prompt) {
            inputWrap.classList.remove('hidden');
            if (inputLabel)
                inputLabel.textContent = opts.label ?? 'Value';
            input.value = opts.initialValue ?? '';
            input.placeholder = opts.placeholder ?? '';
        }
        else {
            inputWrap.classList.add('hidden');
        }
        let settled = false;
        const detachListeners = () => {
            confirmBtn.removeEventListener('click', onConfirm);
            cancelBtn.removeEventListener('click', onCancel);
            backdrop.removeEventListener('click', onCancel);
            document.removeEventListener('keydown', onKey);
        };
        const finish = (value) => {
            if (settled)
                return;
            settled = true;
            modal.classList.add('hidden');
            detachListeners();
            if (activeDialog && activeDialog.cancel === onCancel) {
                activeDialog = null;
            }
            resolve(value);
        };
        const onConfirm = () => finish(opts.prompt ? input.value.trim() : '');
        const onCancel = () => finish(null);
        const onKey = (e) => {
            if (e.key === 'Escape') {
                e.preventDefault();
                onCancel();
            }
            else if (e.key === 'Enter' && opts.prompt && document.activeElement === input) {
                e.preventDefault();
                onConfirm();
            }
        };
        confirmBtn.addEventListener('click', onConfirm);
        cancelBtn.addEventListener('click', onCancel);
        backdrop.addEventListener('click', onCancel);
        document.addEventListener('keydown', onKey);
        activeDialog = { cancel: onCancel };
        modal.classList.remove('hidden');
        if (opts.prompt)
            input.focus();
        else
            confirmBtn.focus();
    });
}
async function showConfirm(opts) {
    const result = await openDialog({ ...opts, prompt: false });
    return result !== null;
}
async function showPrompt(opts) {
    return openDialog({ ...opts, prompt: true });
}
// ── Helpers ────────────────────────────────────────────────────────────────
function escapeHtml(str) {
    const d = document.createElement('div');
    d.textContent = str;
    return d.innerHTML;
}
function escapeAttr(str) {
    return str
        .replace(/&/g, '&amp;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#39;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;');
}
function escapeRegExp(str) {
    return str.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}
function posLabel(pos) {
    return POS_LABELS[pos] || pos;
}
function posAbbrev(pos) {
    return POS_ABBREV[pos] || pos.toLowerCase();
}
function lemmaStateKey(lang, lemma, pos) {
    return `${lang}\u0000${lemma}\u0000${pos}`;
}
function currentLemmaState(lemma, pos) {
    const lang = state.currentResults?.lang || '';
    if (!lang)
        return undefined;
    return state.currentLemmaStates.get(lemmaStateKey(lang, lemma, pos));
}
function compareStrings(a, b) {
    return a.localeCompare(b, undefined, { sensitivity: 'base' });
}
function setParseButtonsDisabled(disabled) {
    document.querySelectorAll('.btn-parse').forEach(btn => {
        btn.disabled = disabled;
    });
}
function formatParserMode(parserMode) {
    return parserMode === 'custom' ? 'Custom parser' : 'Basic parser';
}
function formatParseDuration(parseDurationMs) {
    if (parseDurationMs < 1000)
        return `${parseDurationMs} ms`;
    return `${(parseDurationMs / 1000).toFixed(parseDurationMs >= 10000 ? 1 : 2)} s`;
}
// ── Auth / role ────────────────────────────────────────────────────────────
function computeRole(user) {
    if (!user)
        return 'anon';
    return user.is_admin ? 'admin' : 'user';
}
function applyRoleVisibility() {
    const role = state.role;
    document.body.dataset.authState = role;
    document.body.dataset.role = role;
    document.querySelectorAll('[data-role-show]').forEach(el => {
        const allowed = (el.dataset.roleShow || '').split(/\s+/).filter(Boolean);
        const visible = allowed.includes(role);
        el.classList.toggle('role-hidden', !visible);
    });
}
async function fetchMe() {
    try {
        const resp = await fetch('/api/me', { credentials: 'same-origin' });
        if (!resp.ok)
            throw new Error(`/api/me ${resp.status}`);
        const data = await resp.json();
        applyMeResponse(data);
    }
    catch (err) {
        // Treat any failure as anonymous; we don't want to lock people out.
        applyMeResponse({ authenticated: false, user: null });
    }
}
function applyMeResponse(data) {
    state.user = data.authenticated ? (data.user || null) : null;
    state.dashboard = data.dashboard || null;
    state.decks = data.dashboard?.decks || [];
    state.role = computeRole(state.user);
    if (data.languages) {
        applyLanguagesResponse(data.languages);
    }
    const emailEl = document.getElementById('nav-user-email');
    if (emailEl)
        emailEl.textContent = state.user?.email || '';
    applyRoleVisibility();
    renderNavLanguageSelector();
    updateInspectLede();
    renderDashboard();
}
// ── Language settings ──────────────────────────────────────────────────────
function applyLanguagesResponse(langs) {
    const cleaned = (langs.learning || []).filter(l => SUPPORTED_LANGUAGES.includes(l));
    state.learningLanguages = cleaned.length > 0 ? cleaned : ['FI'];
    state.activeLanguage = state.learningLanguages.includes(langs.active) ? langs.active : state.learningLanguages[0];
    state.languageStats = langs.stats || {};
}
// Build the top-bar dropdown so it lists exactly the user's learning
// languages plus a trailing "Manage languages…" entry that routes to the
// dedicated page. The toggle button shows the current active language;
// clicking opens a custom listbox menu (we style it ourselves so it
// matches the rest of the app — native <select> popups can't be themed).
// Toggling *which* languages are studied happens on /languages.
function renderNavLanguageSelector() {
    const toggle = document.getElementById('nav-language-toggle');
    const current = document.getElementById('nav-language-current');
    const menu = document.getElementById('nav-language-menu');
    if (!toggle || !current || !menu)
        return;
    if (state.role === 'anon') {
        current.innerHTML = '';
        menu.innerHTML = '';
        closeNavLanguageMenu();
        return;
    }
    // Flag-only on the toggle. No tooltip here: the dropdown opens directly
    // below the trigger and would obscure (or push offscreen) any tooltip.
    const flag = languageFlag(state.activeLanguage);
    current.innerHTML = flag || escapeHtml(languageName(state.activeLanguage));
    const items = [...state.learningLanguages].sort((a, b) => languageName(a).localeCompare(languageName(b))).map(l => {
        const isActive = l === state.activeLanguage;
        const name = languageName(l);
        // Flag-only options. data-tooltip lives on the whole button so the
        // portal tooltip fires anywhere on the row, not just the flag.
        // aria-label keeps the option discoverable for screen readers.
        return `<li role="presentation">
            <button type="button" class="nav-lang-option nav-lang-option-flagonly" role="option" data-lang="${l}" aria-selected="${isActive ? 'true' : 'false'}" data-tooltip="${escapeAttr(name)}" aria-label="${escapeAttr(name)}">
                <span class="nav-lang-option-flag" aria-hidden="true">${languageFlag(l)}</span>
                ${isActive ? '<span class="nav-lang-option-check" aria-hidden="true">✓</span>' : ''}
            </button>
        </li>`;
    });
    items.push(`<li class="nav-lang-menu-divider" role="presentation" aria-hidden="true"></li>`);
    items.push(`<li role="presentation">
        <button type="button" class="nav-lang-option nav-lang-menu-manage" role="option" data-manage="1">More…</button>
    </li>`);
    menu.innerHTML = items.join('');
}
function isNavLanguageMenuOpen() {
    const menu = document.getElementById('nav-language-menu');
    return !!menu && !menu.classList.contains('hidden');
}
function openNavLanguageMenu() {
    const toggle = document.getElementById('nav-language-toggle');
    const menu = document.getElementById('nav-language-menu');
    if (!toggle || !menu)
        return;
    menu.classList.remove('hidden');
    toggle.setAttribute('aria-expanded', 'true');
    // Don't auto-focus an option here: focusin fires the portal tooltip, and
    // showing the active language's tooltip immediately on every menu open is
    // noisy. Mouse users hover to see it; keyboard users press ArrowDown
    // from the toggle (wired below) to step into the menu.
}
function closeNavLanguageMenu() {
    const toggle = document.getElementById('nav-language-toggle');
    const menu = document.getElementById('nav-language-menu');
    if (!toggle || !menu)
        return;
    menu.classList.add('hidden');
    toggle.setAttribute('aria-expanded', 'false');
}
// PATCH user settings. Caller passes whichever fields it wants to change;
// success applies the canonical server response (which dedups, reorders, and
// re-anchors the active language). Failure surfaces a toast and leaves state
// untouched. Returns true on success so callers can branch (e.g. don't navigate
// after a failed toggle).
async function patchUserLanguages(patch) {
    try {
        const resp = await fetch('/api/me/languages', {
            method: 'PATCH',
            credentials: 'same-origin',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(patch),
        });
        if (!resp.ok) {
            const msg = await resp.text();
            throw new Error(msg || 'Failed to update languages');
        }
        const data = await resp.json();
        applyLanguagesResponse(data);
        return true;
    }
    catch (err) {
        showToast(err?.message || 'Failed to update languages.', 'error');
        return false;
    }
}
// Switch the site's active language. Persists to the server, then refreshes
// derived views: nav selector, decks list, known words (per-language fetch),
// review queue, and Inspect's language-warning gate. Safe to call when the
// requested language is already active (no-op fast path).
async function setActiveLanguage(lang) {
    if (!state.learningLanguages.includes(lang) || state.activeLanguage === lang) {
        renderNavLanguageSelector();
        return;
    }
    const prev = state.activeLanguage;
    state.activeLanguage = lang;
    renderNavLanguageSelector();
    const ok = await patchUserLanguages({ active: lang });
    if (!ok) {
        state.activeLanguage = prev;
        renderNavLanguageSelector();
        return;
    }
    onActiveLanguageChanged();
}
// Re-render anything that depends on the active language. Called after a
// successful PATCH; consolidated so the inspect/dropdown/languages-page paths
// don't drift.
function onActiveLanguageChanged() {
    renderNavLanguageSelector();
    renderDashboard();
    if (currentRoute() === '/decks' || currentRoute() === '/decks/official') {
        renderDecksPage();
    }
    if (currentRoute() === '/vocab') {
        // Vocab page reads from state.activeLanguage for stats, hint, and the
        // POST/DELETE bodies — re-render and re-fetch the per-language list.
        renderVocabPage();
        void loadKnownWords();
    }
    if (currentRoute() === '/review') {
        // Old card is for whatever language was active before — re-fetch.
        state.currentReviewCard = null;
        renderReviewPage();
        void loadNextReviewCard(false);
    }
    if (currentRoute() === '/languages') {
        renderLanguagesPage();
    }
    refreshInspectFormForActiveLanguage();
}
// Languages page: split rows into "Studying" (active set) and "Other"
// (inactive). Within each group, sort alphabetically by display name. The
// active section comes first so the language the user is focused on is
// always at the top; the divider makes it visually clear which set a
// language belongs to. The user can't drop their last language — its
// checkbox is disabled to prevent ending up in a zero-language state.
function renderLanguagesPage() {
    const list = document.getElementById('languages-list');
    if (!list)
        return;
    const learning = new Set(state.learningLanguages);
    const onlyOne = learning.size <= 1;
    const byName = (a, b) => languageName(a).localeCompare(languageName(b));
    const activeLangs = SUPPORTED_LANGUAGES.filter(l => learning.has(l)).sort(byName);
    const inactiveLangs = SUPPORTED_LANGUAGES.filter(l => !learning.has(l)).sort(byName);
    const renderRow = (lang) => {
        const isLearning = learning.has(lang);
        const isActive = state.activeLanguage === lang;
        const stats = state.languageStats[lang] || { decks: 0, known_words: 0 };
        const statsLine = `${stats.decks} deck${stats.decks === 1 ? '' : 's'} · ${stats.known_words.toLocaleString()} known word${stats.known_words === 1 ? '' : 's'}`;
        const activeBadge = isActive
            ? '<span class="language-row-active-badge">Active</span>'
            : '';
        const setActiveBtn = isLearning && !isActive
            ? `<button type="button" class="btn btn-link btn-sm" data-set-active="${lang}">Set active</button>`
            : '';
        // Button-driven add/remove instead of a native checkbox so the action
        // matches the rest of the site's button styling and so we can disable
        // (with a tooltip) the row that would leave the user with zero
        // languages.
        let toggleBtn;
        if (isLearning) {
            const disabledAttr = onlyOne ? ' disabled data-tooltip="At least one language is required"' : '';
            toggleBtn = `<button type="button" class="btn btn-outline btn-sm" data-toggle-learning="${lang}" data-studying="1"${disabledAttr}>Remove</button>`;
        }
        else {
            toggleBtn = `<button type="button" class="btn btn-primary btn-sm" data-toggle-learning="${lang}" data-studying="0">Add</button>`;
        }
        return `<div class="language-row${isActive ? ' is-active' : ''}">
            <div class="language-row-info">
                <span class="language-row-flag" aria-hidden="true">${languageFlag(lang)}</span>
                <div class="language-row-text">
                    <span class="language-row-name">${escapeHtml(languageName(lang))} ${activeBadge}</span>
                    <span class="language-row-sub">${statsLine}</span>
                </div>
            </div>
            <div class="language-row-actions">
                ${setActiveBtn}
                ${toggleBtn}
            </div>
        </div>`;
    };
    const sections = [];
    sections.push(`<h2 class="languages-section-heading">Studying</h2>`);
    sections.push(activeLangs.length === 0
        ? `<p class="empty-state">You're not studying any languages yet.</p>`
        : activeLangs.map(renderRow).join(''));
    if (inactiveLangs.length > 0) {
        sections.push(`<h2 class="languages-section-heading">Other languages</h2>`);
        sections.push(inactiveLangs.map(renderRow).join(''));
    }
    list.innerHTML = sections.join('');
}
async function toggleLearningLanguage(lang, studying) {
    const set = new Set(state.learningLanguages);
    if (studying)
        set.add(lang);
    else
        set.delete(lang);
    if (set.size === 0) {
        showToast('You must study at least one language.', 'error');
        renderLanguagesPage();
        return;
    }
    const next = SUPPORTED_LANGUAGES.filter(l => set.has(l));
    const nextActive = next.includes(state.activeLanguage) ? state.activeLanguage : next[0];
    const prev = { learning: state.learningLanguages.slice(), active: state.activeLanguage };
    state.learningLanguages = next;
    state.activeLanguage = nextActive;
    const ok = await patchUserLanguages({ learning: next, active: nextActive });
    if (!ok) {
        state.learningLanguages = prev.learning;
        state.activeLanguage = prev.active;
        renderLanguagesPage();
        renderNavLanguageSelector();
        return;
    }
    onActiveLanguageChanged();
    showToast(studying ? `Added ${languageName(lang)} to your languages.` : `Removed ${languageName(lang)} from your languages.`, 'success');
}
function initLanguagesPage() {
    const list = document.getElementById('languages-list');
    list?.addEventListener('click', (e) => {
        const target = e.target?.closest('button');
        if (!target)
            return;
        const setActive = target.getAttribute('data-set-active');
        if (setActive) {
            void setActiveLanguage(setActive);
            return;
        }
        const toggle = target.getAttribute('data-toggle-learning');
        if (toggle) {
            // data-studying reflects the *current* state, so the new value
            // is the negation. Disabled buttons (the "last language" case)
            // don't fire clicks at all.
            const studyingNow = target.getAttribute('data-studying') === '1';
            void toggleLearningLanguage(toggle, !studyingNow);
        }
    });
}
function initNavLanguageSelector() {
    const toggle = document.getElementById('nav-language-toggle');
    const menu = document.getElementById('nav-language-menu');
    if (!toggle || !menu)
        return;
    toggle.addEventListener('click', (e) => {
        e.stopPropagation();
        if (isNavLanguageMenuOpen())
            closeNavLanguageMenu();
        else
            openNavLanguageMenu();
    });
    menu.addEventListener('click', (e) => {
        const btn = e.target?.closest('.nav-lang-option');
        if (!btn)
            return;
        e.stopPropagation();
        if (btn.dataset.manage === '1') {
            closeNavLanguageMenu();
            navigate('/languages');
            return;
        }
        const lang = btn.dataset.lang;
        if (lang) {
            closeNavLanguageMenu();
            void setActiveLanguage(lang);
        }
    });
    // Arrow-key navigation between options inside the open menu. Wired to
    // the menu container (not each button) so re-renders don't need to
    // re-attach listeners.
    menu.addEventListener('keydown', (e) => {
        if (!isNavLanguageMenuOpen())
            return;
        const options = Array.from(menu.querySelectorAll('.nav-lang-option'));
        const idx = options.indexOf(document.activeElement);
        if (e.key === 'ArrowDown') {
            e.preventDefault();
            options[(idx + 1 + options.length) % options.length]?.focus();
        }
        else if (e.key === 'ArrowUp') {
            e.preventDefault();
            options[(idx - 1 + options.length) % options.length]?.focus();
        }
        else if (e.key === 'Escape') {
            e.preventDefault();
            closeNavLanguageMenu();
            toggle.focus();
        }
    });
    // Click-outside / Escape from the toggle close the menu.
    document.addEventListener('click', (e) => {
        if (!isNavLanguageMenuOpen())
            return;
        const target = e.target;
        const dropdown = toggle.closest('.nav-lang-dropdown');
        if (target && dropdown && !dropdown.contains(target)) {
            closeNavLanguageMenu();
        }
    });
    toggle.addEventListener('keydown', (e) => {
        if (e.key === 'Escape' && isNavLanguageMenuOpen()) {
            e.preventDefault();
            closeNavLanguageMenu();
            return;
        }
        if (e.key === 'ArrowDown' || e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            if (!isNavLanguageMenuOpen())
                openNavLanguageMenu();
            if (e.key === 'ArrowDown') {
                // Step from the toggle into the menu's first option. Active
                // option (if present) is preferred so keyboard users land on
                // a useful default.
                const active = menu.querySelector('.nav-lang-option[aria-selected="true"]')
                    || menu.querySelector('.nav-lang-option');
                active?.focus();
            }
        }
    });
}
async function handleSignout() {
    try {
        await fetch('/api/auth/logout', { method: 'POST', credentials: 'same-origin' });
    }
    catch {
        // Best-effort — even if the endpoint is missing, clear local state.
    }
    state.user = null;
    state.dashboard = null;
    state.decks = [];
    state.officialDecks = [];
    state.officialDecksLoaded = false;
    state.decksTab = 'mine';
    state.role = 'anon';
    state.learningLanguages = ['FI', 'ET'];
    state.activeLanguage = 'FI';
    state.languageStats = {};
    state.currentResults = null;
    state.currentTextPreview = '';
    state.currentSourceText = '';
    state.currentParserMode = 'basic';
    state.currentContext = 'inspect';
    state.currentRow = null;
    state.currentLemmaStates.clear();
    state.currentReviewCard = null;
    state.reviewDeckFilter = '';
    try {
        sessionStorage.removeItem(LAST_PARSE_KEY);
    }
    catch { }
    clearResultsDom();
    applyRoleVisibility();
    showToast('Signed out', 'info');
    navigate('/');
}
function clearResultsDom() {
    const tbody = document.getElementById('word-table-body');
    if (tbody)
        tbody.innerHTML = '';
    const setText = (id, text) => {
        const el = document.getElementById(id);
        if (el)
            el.textContent = text;
    };
    setText('results-title', '');
    setText('results-duration', '');
    setText('results-stats', '');
    setText('results-parser', '');
    const inspectInput = document.getElementById('inspect-text');
    const workbenchText = document.getElementById('parse-text');
    if (inspectInput)
        inspectInput.value = '';
    if (workbenchText)
        workbenchText.value = '';
}
// ── Hash router ────────────────────────────────────────────────────────────
const ROUTE_TO_PAGE = {
    '/': 'landing-page',
    '/about': 'about-page',
    '/signin': 'signin-page',
    '/dashboard': 'dashboard-page',
    '/inspect': 'inspect-page',
    '/decks': 'decks-page',
    '/decks/official': 'decks-page',
    '/languages': 'languages-page',
    '/vocab': 'vocab-page',
    '/review': 'review-page',
    '/admin/workbench': 'admin-workbench-page',
    '/admin/feedback': 'admin-feedback-page',
    '/admin/users': 'admin-users-page',
    '/results': 'results-page',
};
// /decks → My Decks (default), /decks/official → Official Decks.
// Mapping is centralised so the hash router and the tab buttons agree.
function tabForDecksRoute(route) {
    return route === '/decks/official' ? 'official' : 'mine';
}
function routeForDecksTab(tab) {
    return tab === 'official' ? '/decks/official' : '/decks';
}
function currentRoute() {
    const hash = window.location.hash || '#/';
    return hash.startsWith('#') ? hash.slice(1) : hash;
}
function navigate(route) {
    const target = route.startsWith('/') ? route : `/${route}`;
    if (currentRoute() === target) {
        renderRoute();
        return;
    }
    window.location.hash = `#${target}`;
}
function showPage(id) {
    document.querySelectorAll('.page').forEach(p => p.classList.remove('active'));
    const el = document.getElementById(id);
    if (el)
        el.classList.add('active');
    window.scrollTo({ top: 0, behavior: 'instant' });
}
function setActiveNavLink(route) {
    document.querySelectorAll('[data-route]').forEach(el => {
        el.classList.toggle('active', el.dataset.route === route);
    });
}
const DECK_DETAIL_RE = /^\/decks\/(\d+)$/;
function isRouteAllowed(route) {
    const role = state.role;
    // Admin-only routes
    if (route.startsWith('/admin/')) {
        if (role === 'admin')
            return { allowed: true };
        if (role === 'anon')
            return { allowed: false, redirect: '/signin' };
        return { allowed: false, redirect: '/dashboard' };
    }
    // Authenticated-only routes
    const userOnly = ['/dashboard', '/inspect', '/decks', '/decks/official', '/languages', '/vocab', '/review', '/results'];
    const isDeckDetail = DECK_DETAIL_RE.test(route);
    if (userOnly.includes(route) || isDeckDetail) {
        if (role === 'anon')
            return { allowed: false, redirect: '/signin' };
        return { allowed: true };
    }
    // Anonymous-only: signed-in users hitting / or /signin land on dashboard
    if ((route === '/' || route === '/signin') && (role === 'user' || role === 'admin')) {
        return { allowed: false, redirect: '/dashboard' };
    }
    return { allowed: true };
}
function renderRoute() {
    let route = currentRoute();
    const deckMatch = route.match(DECK_DETAIL_RE);
    if (!deckMatch && !ROUTE_TO_PAGE[route])
        route = '/';
    const guard = isRouteAllowed(route);
    if (!guard.allowed && guard.redirect) {
        window.location.hash = `#${guard.redirect}`;
        return;
    }
    if (deckMatch) {
        // Deck detail reuses the results page.
        showPage('results-page');
        setActiveNavLink('/decks');
        closeMobileNav();
        void loadDeckDetail(Number(deckMatch[1]));
        return;
    }
    showPage(ROUTE_TO_PAGE[route]);
    setActiveNavLink(route);
    closeMobileNav();
    // Per-page hooks
    if (route === '/dashboard')
        renderDashboard();
    if (route === '/decks' || route === '/decks/official') {
        // Tab state is derived from the route so refreshing the page or
        // sharing a link keeps the same tab open. Both routes share the
        // same page; only the active tab differs.
        state.decksTab = tabForDecksRoute(route);
        // Highlight the /decks nav link for the Official subroute too.
        setActiveNavLink('/decks');
        renderDecksPage();
        if (state.decksTab === 'official' && !state.officialDecksLoaded) {
            void (async () => {
                await loadOfficialDecks();
                if (state.decksTab === 'official')
                    renderDecksPage();
            })();
        }
    }
    if (route === '/languages') {
        renderLanguagesPage();
    }
    if (route === '/vocab') {
        renderVocabPage();
        void loadKnownWords();
    }
    if (route === '/review') {
        renderReviewPage();
        void loadNextReviewCard(false);
    }
    if (route === '/admin/feedback') {
        renderAdminFeedbackPage();
        void loadAdminFeedback();
    }
    if (route === '/admin/users') {
        void loadAdminUsers();
    }
    if (route === '/results' && !state.currentResults) {
        // Hard refresh / direct navigation: re-hydrate from sessionStorage so
        // the user doesn't land on an empty page. If nothing is cached, send
        // them back to /inspect rather than leaving the shell empty.
        void (async () => {
            if (!await restoreLastParse()) {
                window.location.hash = '#/inspect';
            }
        })();
    }
}
// ── Mobile nav ─────────────────────────────────────────────────────────────
function initMobileNav() {
    const hamburger = document.getElementById('nav-hamburger');
    const overlay = document.getElementById('nav-mobile-overlay');
    if (!hamburger || !overlay)
        return;
    hamburger.addEventListener('click', () => {
        const isOpen = !overlay.classList.contains('hidden');
        overlay.classList.toggle('hidden', isOpen);
        hamburger.classList.toggle('open', !isOpen);
        document.body.classList.toggle('nav-open', !isOpen);
    });
    overlay.addEventListener('click', e => {
        if (e.target === overlay)
            closeMobileNav();
    });
    overlay.querySelectorAll('.nav-mobile-link').forEach(link => {
        link.addEventListener('click', closeMobileNav);
    });
}
function closeMobileNav() {
    const hamburger = document.getElementById('nav-hamburger');
    const overlay = document.getElementById('nav-mobile-overlay');
    overlay?.classList.add('hidden');
    hamburger?.classList.remove('open');
    document.body.classList.remove('nav-open');
}
let signinMode = 'login';
function setSigninMode(mode) {
    signinMode = mode;
    const heading = document.getElementById('signin-heading');
    const lede = document.getElementById('signin-lede');
    const submit = document.getElementById('signin-submit');
    const password = document.getElementById('signin-password');
    const errorEl = document.getElementById('signin-error');
    document.querySelectorAll('.signin-tab').forEach(tab => {
        const isActive = tab.dataset.mode === mode;
        tab.classList.toggle('active', isActive);
        tab.setAttribute('aria-selected', isActive ? 'true' : 'false');
    });
    if (mode === 'register') {
        if (heading)
            heading.textContent = 'Create account';
        if (lede)
            lede.textContent = 'Pick an email and a password (8+ characters).';
        if (submit)
            submit.textContent = 'Create account';
        if (password)
            password.autocomplete = 'new-password';
    }
    else {
        if (heading)
            heading.textContent = 'Sign in';
        if (lede)
            lede.textContent = 'Welcome back. Enter your email and password.';
        if (submit)
            submit.textContent = 'Sign in';
        if (password)
            password.autocomplete = 'current-password';
    }
    if (errorEl)
        errorEl.classList.add('hidden');
}
function initSigninForm() {
    const form = document.getElementById('signin-form');
    if (!form)
        return;
    document.querySelectorAll('.signin-tab').forEach(tab => {
        tab.addEventListener('click', () => {
            const mode = tab.dataset.mode;
            if (mode)
                setSigninMode(mode);
        });
    });
    setSigninMode('login');
    form.addEventListener('submit', async (e) => {
        e.preventDefault();
        const emailEl = document.getElementById('signin-email');
        const passwordEl = document.getElementById('signin-password');
        const errorEl = document.getElementById('signin-error');
        const submitBtn = document.getElementById('signin-submit');
        const email = emailEl.value.trim();
        const password = passwordEl.value;
        if (!email) {
            errorEl.textContent = 'Please enter your email.';
            errorEl.classList.remove('hidden');
            return;
        }
        if (password.length < 8) {
            errorEl.textContent = 'Password must be at least 8 characters.';
            errorEl.classList.remove('hidden');
            return;
        }
        errorEl.classList.add('hidden');
        submitBtn.disabled = true;
        const origLabel = submitBtn.textContent || '';
        submitBtn.textContent = signinMode === 'register' ? 'Creating account…' : 'Signing in…';
        try {
            const endpoint = signinMode === 'register' ? '/api/auth/register' : '/api/auth/login';
            const resp = await fetch(endpoint, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                credentials: 'same-origin',
                body: JSON.stringify({ email, password }),
            });
            if (!resp.ok) {
                const msg = (await resp.text()).trim();
                throw new Error(msg || (signinMode === 'register' ? 'Sign-up failed' : 'Sign-in failed'));
            }
            const data = await resp.json();
            if (!data.authenticated || !data.user) {
                throw new Error('Sign-in failed');
            }
            // Refresh /api/me so we get the dashboard payload too.
            await fetchMe();
            showToast(signinMode === 'register' ? `Welcome, ${data.user.email}` : `Welcome back, ${data.user.email}`, 'success');
            navigate('/dashboard');
        }
        catch (err) {
            errorEl.textContent = err.message || 'Sign-in failed';
            errorEl.classList.remove('hidden');
        }
        finally {
            submitBtn.disabled = false;
            submitBtn.textContent = origLabel;
        }
    });
}
// ── Dashboard ──────────────────────────────────────────────────────────────
function renderDashboard() {
    const greeting = document.getElementById('dashboard-greeting-suffix');
    if (greeting) {
        greeting.textContent = state.user ? `, ${state.user.email.split('@')[0]}` : '';
    }
    const setStat = (id, value) => {
        const el = document.getElementById(id);
        if (!el)
            return;
        el.textContent = value === undefined ? '—' : value.toLocaleString();
    };
    setStat('stat-known', state.dashboard?.known_count);
    setStat('stat-due', state.dashboard?.due_count);
    setStat('stat-new-capacity', state.dashboard?.new_capacity_today);
    const decksList = document.getElementById('dashboard-decks-list');
    if (!decksList)
        return;
    const allDecks = state.dashboard?.decks || [];
    const decks = allDecks.filter(d => d.lang === state.activeLanguage);
    if (decks.length === 0) {
        const langLabel = languageName(state.activeLanguage);
        const hint = allDecks.length === 0
            ? `No decks yet — paste some text under <a href="#/inspect">Parse</a> to get started.`
            : `No ${escapeHtml(langLabel)} decks yet. Switch the language in the top bar to see your other decks, or <a href="#/inspect">parse a ${escapeHtml(langLabel)} text</a>.`;
        decksList.innerHTML = `<p class="empty-state">${hint}</p>`;
        return;
    }
    decksList.innerHTML = decks.map(d => {
        const langName = d.lang === 'FI' ? 'Finnish' : d.lang === 'ET' ? 'Estonian' : escapeHtml(d.lang);
        const knownPct = d.unique > 0 ? Math.round((d.known / d.unique) * 100) : 0;
        return `<a href="#/decks" class="deck-card">
            <h4>${escapeHtml(d.title)}</h4>
            <p class="deck-meta">${langName} · ${d.known}/${d.unique} known (${knownPct}%) · ${d.due} due</p>
        </a>`;
    }).join('');
}
async function refreshDashboardData(options = {}) {
    await fetchMe();
    if (options.rerenderRoute !== false) {
        renderRoute();
    }
}
async function markResultLemma(status, lemma, pos, trigger) {
    const lang = state.currentResults?.lang;
    if (!lang)
        return;
    const stateKey = lemmaStateKey(lang, lemma, pos);
    if (state.pendingLemmaStates.has(stateKey))
        return;
    state.pendingLemmaStates.add(stateKey);
    trigger.disabled = true;
    if (state.currentResults) {
        renderResultsTable(state.currentResults);
    }
    try {
        const body = status === 'neutral'
            ? { lang, lemma, pos, status: '' }
            : { lang, lemma, pos, status };
        const resp = await fetch('/api/lemma-state', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'same-origin',
            body: JSON.stringify(body),
        });
        if (!resp.ok) {
            throw new Error(await resp.text() || 'Failed to update word');
        }
        if (status === 'neutral') {
            state.currentLemmaStates.delete(stateKey);
        }
        else {
            state.currentLemmaStates.set(stateKey, status);
        }
        await refreshDashboardData({ rerenderRoute: false });
        const toastMsg = status === 'known'
            ? 'Word marked known.'
            : status === 'ignored'
                ? 'Word ignored.'
                : 'Word reset to unknown.';
        showToast(toastMsg, 'success');
    }
    catch (err) {
        showToast(err.message || 'Failed to update word.', 'error');
        trigger.disabled = false;
    }
    finally {
        state.pendingLemmaStates.delete(stateKey);
        if (state.currentResults) {
            renderResultsTable(state.currentResults);
        }
    }
}
function renderDecksPage() {
    const tabMine = document.getElementById('decks-tab-mine');
    const tabOfficial = document.getElementById('decks-tab-official');
    tabMine?.classList.toggle('active', state.decksTab === 'mine');
    tabOfficial?.classList.toggle('active', state.decksTab === 'official');
    tabMine?.setAttribute('aria-selected', state.decksTab === 'mine' ? 'true' : 'false');
    tabOfficial?.setAttribute('aria-selected', state.decksTab === 'official' ? 'true' : 'false');
    if (state.decksTab === 'mine') {
        renderMyDecksTab();
    }
    else {
        renderOfficialDecksTab();
    }
    renderKnownWordsPanel();
}
function renderMyDecksTab() {
    const empty = document.getElementById('decks-empty');
    const list = document.getElementById('decks-list');
    const officialEmpty = document.getElementById('official-decks-empty');
    const officialList = document.getElementById('official-decks-list');
    if (!empty || !list || !officialEmpty || !officialList)
        return;
    officialEmpty.classList.add('hidden');
    officialList.classList.add('hidden');
    const decks = (state.decks || []).filter(d => d.lang === state.activeLanguage);
    empty.classList.toggle('hidden', decks.length > 0);
    list.classList.toggle('hidden', decks.length === 0);
    if (decks.length === 0) {
        list.innerHTML = '';
        return;
    }
    const isAdmin = state.role === 'admin';
    list.innerHTML = decks.map(deck => {
        const langName = deck.lang === 'FI' ? 'Finnish' : 'Estonian';
        const knownPct = deck.unique > 0 ? Math.round((deck.known / deck.unique) * 100) : 0;
        const isOwner = !deck.subscribed;
        const badges = [];
        if (deck.is_public && isOwner)
            badges.push('<span class="deck-badge deck-badge-official">Official</span>');
        if (deck.is_public && !isOwner)
            badges.push('<span class="deck-badge deck-badge-official">Official · added</span>');
        const actions = [
            `<button type="button" class="btn btn-link btn-sm" data-open-review="${deck.id}">Review</button>`,
        ];
        if (isOwner) {
            actions.push(`<button type="button" class="btn btn-link btn-sm" data-rename-deck="${deck.id}">Rename</button>`);
            if (isAdmin) {
                const label = deck.is_public ? 'Unpublish' : 'Publish';
                actions.push(`<button type="button" class="btn btn-link btn-sm" data-toggle-public="${deck.id}" data-current-public="${deck.is_public ? '1' : '0'}">${label}</button>`);
            }
            actions.push(`<button type="button" class="btn btn-link btn-sm" data-delete-deck="${deck.id}">Delete</button>`);
        }
        else {
            actions.push(`<button type="button" class="btn btn-link btn-sm" data-unsubscribe-deck="${deck.id}">Remove</button>`);
        }
        return `<article class="deck-list-item">
            <div>
                <h2><a href="#/decks/${deck.id}" class="deck-list-title">${escapeHtml(deck.title)}</a> ${badges.join(' ')}</h2>
                <p class="deck-list-meta">${langName} · ${deck.known}/${deck.unique} known (${knownPct}%) · ${deck.due} due</p>
            </div>
            <div class="deck-list-actions">
                ${actions.join('')}
            </div>
        </article>`;
    }).join('');
}
function renderOfficialDecksTab() {
    const empty = document.getElementById('decks-empty');
    const list = document.getElementById('decks-list');
    const officialEmpty = document.getElementById('official-decks-empty');
    const officialList = document.getElementById('official-decks-list');
    if (!empty || !list || !officialEmpty || !officialList)
        return;
    empty.classList.add('hidden');
    list.classList.add('hidden');
    const decks = (state.officialDecks || []).filter(d => d.lang === state.activeLanguage);
    if (!state.officialDecksLoaded) {
        officialEmpty.classList.add('hidden');
        officialList.classList.remove('hidden');
        officialList.innerHTML = '<p class="empty-state">Loading official decks…</p>';
        return;
    }
    officialEmpty.classList.toggle('hidden', decks.length > 0);
    officialList.classList.toggle('hidden', decks.length === 0);
    if (decks.length === 0) {
        officialList.innerHTML = '';
        return;
    }
    officialList.innerHTML = decks.map(deck => {
        const langName = deck.lang === 'FI' ? 'Finnish' : 'Estonian';
        const subscribed = !!deck.subscribed;
        const isOwner = !!deck.is_owner;
        // Owner of the deck doesn't subscribe to their own publication — they
        // can manage it from "My decks". Other users get the studying-list
        // toggle.
        let actionBtn;
        let badge;
        if (isOwner) {
            actionBtn = '';
            badge = '<span class="deck-badge deck-badge-subscribed">You published this</span>';
        }
        else if (subscribed) {
            actionBtn = `<button type="button" class="btn btn-link btn-sm" data-unsubscribe-deck="${deck.id}">Remove from studying</button>`;
            badge = '<span class="deck-badge deck-badge-subscribed">Added</span>';
        }
        else {
            actionBtn = `<button type="button" class="btn btn-primary btn-sm" data-subscribe-deck="${deck.id}">Add to studying list</button>`;
            badge = '';
        }
        return `<article class="deck-list-item">
            <div>
                <h2><a href="#/decks/${deck.id}" class="deck-list-title">${escapeHtml(deck.title)}</a> ${badge}</h2>
                <p class="deck-list-meta">${langName} · ${deck.unique} unique words</p>
            </div>
            <div class="deck-list-actions">
                ${actionBtn}
            </div>
        </article>`;
    }).join('');
}
async function loadOfficialDecks() {
    try {
        const resp = await fetch('/api/decks/public', { credentials: 'same-origin' });
        if (!resp.ok)
            throw new Error(await resp.text() || 'Failed to load official decks');
        const data = await resp.json();
        state.officialDecks = data.decks || [];
        state.officialDecksLoaded = true;
    }
    catch (err) {
        // Surface the failure via toast but DON'T wipe state.officialDecks or
        // mark the cache as loaded — wiping would render the misleading
        // "No official decks have been published yet" empty state on a
        // network blip, and flipping officialDecksLoaded would suppress the
        // automatic retry on next visit. Keep what we had; let the user try
        // again.
        showToast(err.message || 'Failed to load official decks.', 'error');
    }
}
function getDeckByID(deckID) {
    return state.decks.find(deck => deck.id === deckID)
        ?? state.officialDecks.find(deck => deck.id === deckID);
}
function renderVocabPage() {
    const total = document.getElementById('vocab-stat-total');
    if (total) {
        const v = state.dashboard?.known_count;
        total.textContent = v === undefined ? '—' : v.toLocaleString();
    }
    renderVocabLangStat();
}
function renderVocabLangStat() {
    const langNameEl = document.getElementById('vocab-stat-lang-name');
    if (langNameEl) {
        langNameEl.textContent = languageName(state.activeLanguage);
    }
    const langCount = document.getElementById('vocab-stat-lang');
    if (langCount) {
        // Prefer the server-side stat (it's the source of truth and stays
        // accurate even before the in-memory list is loaded). Fall back to
        // the loaded list length once it lands.
        const fromStats = state.languageStats?.[state.activeLanguage]?.known_words;
        const fallback = state.knownWords.length;
        const value = fromStats !== undefined ? fromStats : fallback;
        langCount.textContent = value.toLocaleString();
    }
}
function renderKnownWordsPanel() {
    const list = document.getElementById('known-words-list');
    const empty = document.getElementById('known-words-empty');
    const summary = document.getElementById('known-words-summary');
    const hint = document.getElementById('known-words-lang-hint');
    if (hint) {
        hint.textContent = `Importing in ${languageName(state.activeLanguage)} — switch the dropdown at the top to import in another language.`;
    }
    if (!list || !empty)
        return;
    const words = state.knownWords;
    empty.classList.toggle('hidden', words.length > 0);
    list.classList.toggle('hidden', words.length === 0);
    if (summary && !summary.textContent) {
        summary.textContent = words.length === 0 ? '' : `${words.length.toLocaleString()} known`;
    }
    if (words.length === 0) {
        list.innerHTML = '';
        return;
    }
    list.innerHTML = words.map(word => `<div class="known-word-chip">
        <span>${escapeHtml(word.lemma)}</span>
        <span class="known-word-pos">${escapeHtml(posLabel(word.pos))}</span>
        <button type="button" class="known-word-delete" aria-label="Remove ${escapeAttr(word.lemma)}" data-known-lemma="${escapeAttr(word.lemma)}" data-known-pos="${escapeAttr(word.pos)}">×</button>
    </div>`).join('');
    renderVocabLangStat();
}
function parseKnownWordsInput(raw) {
    const seen = new Set();
    const words = [];
    for (const part of raw.split(/[\n,;]+/)) {
        const word = part.trim();
        if (!word)
            continue;
        const key = word.toLocaleLowerCase();
        if (seen.has(key))
            continue;
        seen.add(key);
        words.push(word);
    }
    return words;
}
function renderKnownWordsUnresolved(unresolved) {
    const el = document.getElementById('known-words-unresolved');
    if (!el)
        return;
    el.classList.toggle('hidden', unresolved.length === 0);
    el.textContent = unresolved.length === 0
        ? ''
        : `Could not resolve: ${unresolved.join(', ')}`;
}
async function loadKnownWords() {
    if (state.role === 'anon')
        return;
    try {
        const resp = await fetch(`/api/known-words?lang=${encodeURIComponent(state.activeLanguage)}`, { credentials: 'same-origin' });
        if (!resp.ok)
            throw new Error(await resp.text() || 'Failed to load known words');
        const data = await resp.json();
        state.knownWords = data.known_words || [];
        const summary = document.getElementById('known-words-summary');
        if (summary)
            summary.textContent = state.knownWords.length === 0 ? '' : `${state.knownWords.length.toLocaleString()} known`;
        renderKnownWordsPanel();
    }
    catch (err) {
        state.knownWords = [];
        renderKnownWordsPanel();
        showToast(err.message || 'Failed to load known words.', 'error');
    }
}
async function postKnownWords(words) {
    const resp = await fetch('/api/known-words', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'same-origin',
        body: JSON.stringify({ lang: state.activeLanguage, words }),
    });
    if (!resp.ok)
        throw new Error(await resp.text() || 'Failed to import known words');
    return await resp.json();
}
function describeImportResult(data) {
    const importedCount = data.imported?.length || 0;
    const unresolvedCount = data.unresolved?.length || 0;
    return `${importedCount} imported${unresolvedCount ? `, ${unresolvedCount} unresolved` : ''}`;
}
async function importKnownWords() {
    const input = document.getElementById('known-words-input');
    const submitBtn = document.getElementById('known-words-submit');
    const summary = document.getElementById('known-words-summary');
    if (!input || !submitBtn)
        return;
    const words = parseKnownWordsInput(input.value);
    if (words.length === 0) {
        showToast('Paste at least one word to import.', 'error');
        return;
    }
    submitBtn.disabled = true;
    const label = submitBtn.textContent || '';
    submitBtn.textContent = 'Importing...';
    try {
        const data = await postKnownWords(words);
        input.value = '';
        renderKnownWordsUnresolved(data.unresolved || []);
        if (summary)
            summary.textContent = describeImportResult(data);
        await refreshDashboardData();
        await loadKnownWords();
        showToast('Known words imported.', 'success');
    }
    catch (err) {
        showToast(err.message || 'Failed to import known words.', 'error');
    }
    finally {
        submitBtn.disabled = false;
        submitBtn.textContent = label;
    }
}
// CSV/TSV first-column extraction. Also handles bare one-per-line word lists.
function parseFileWords(raw) {
    const seen = new Set();
    const words = [];
    const lines = raw.replace(/^﻿/, '').split(/\r?\n/);
    for (const line of lines) {
        if (!line)
            continue;
        const firstCol = line.split(/[\t,;]/)[0] || '';
        const trimmed = firstCol.trim().replace(/^"(.*)"$/, '$1').trim();
        if (!trimmed)
            continue;
        const key = trimmed.toLocaleLowerCase();
        if (seen.has(key))
            continue;
        seen.add(key);
        words.push(trimmed);
    }
    return words;
}
async function importKnownWordsFromFile(file) {
    const status = document.getElementById('vocab-file-status');
    if (status)
        status.textContent = `Reading ${file.name}…`;
    try {
        const text = await file.text();
        const words = parseFileWords(text);
        if (words.length === 0) {
            if (status)
                status.textContent = '';
            showToast('No words found in file.', 'error');
            return;
        }
        if (status)
            status.textContent = `Importing ${words.length.toLocaleString()} words from ${file.name}…`;
        const data = await postKnownWords(words);
        renderKnownWordsUnresolved(data.unresolved || []);
        if (status)
            status.textContent = `${file.name}: ${describeImportResult(data)}`;
        await refreshDashboardData();
        await loadKnownWords();
        showToast('Known words imported.', 'success');
    }
    catch (err) {
        if (status)
            status.textContent = '';
        showToast(err.message || 'Failed to import file.', 'error');
    }
}
// ── AnkiConnect ────────────────────────────────────────────────────────────
//
// AnkiConnect exposes Anki via http://127.0.0.1:8765 with a JSON-RPC-ish API.
// We pull deck names, then notes from the chosen deck, then the chosen field's
// value from each note, and pipe the result through the standard import path.
const ANKI_CONNECT_URL = 'http://127.0.0.1:8765';
async function ankiInvoke(action, params = {}) {
    let resp;
    try {
        resp = await fetch(ANKI_CONNECT_URL, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ action, version: 6, params }),
        });
    }
    catch {
        throw new Error('Could not reach Anki on localhost:8765. Is Anki open with the AnkiConnect add-on installed?');
    }
    if (!resp.ok)
        throw new Error(`AnkiConnect HTTP ${resp.status}`);
    const data = await resp.json();
    if (data.error)
        throw new Error(`AnkiConnect: ${data.error}`);
    return data.result;
}
const ankiState = {
    decks: [],
    fields: [],
};
async function connectToAnki() {
    const button = document.getElementById('vocab-anki-connect');
    const status = document.getElementById('vocab-anki-status');
    const stepConnect = document.getElementById('vocab-anki-step-connect');
    const stepPick = document.getElementById('vocab-anki-step-pick');
    if (!button)
        return;
    button.disabled = true;
    if (status)
        status.textContent = 'Connecting…';
    try {
        await ankiInvoke('version');
        const decks = await ankiInvoke('deckNames');
        ankiState.decks = decks.slice().sort();
        const deckSelect = document.getElementById('vocab-anki-deck');
        if (deckSelect) {
            deckSelect.innerHTML = ankiState.decks
                .map(d => `<option value="${escapeAttr(d)}">${escapeHtml(d)}</option>`)
                .join('');
        }
        await refreshAnkiFieldsForSelectedDeck();
        stepConnect?.classList.add('hidden');
        stepPick?.classList.remove('hidden');
        if (status)
            status.textContent = '';
    }
    catch (err) {
        if (status)
            status.textContent = err.message || 'Failed to connect.';
        showToast(err.message || 'Failed to connect to Anki.', 'error');
    }
    finally {
        button.disabled = false;
    }
}
async function refreshAnkiFieldsForSelectedDeck() {
    const deckSelect = document.getElementById('vocab-anki-deck');
    const fieldSelect = document.getElementById('vocab-anki-field');
    if (!deckSelect || !fieldSelect)
        return;
    const deck = deckSelect.value;
    if (!deck) {
        fieldSelect.innerHTML = '';
        return;
    }
    const noteIDs = await ankiInvoke('findNotes', { query: `deck:"${deck}"` });
    if (noteIDs.length === 0) {
        ankiState.fields = [];
        fieldSelect.innerHTML = '<option value="">(deck is empty)</option>';
        return;
    }
    const sample = await ankiInvoke('notesInfo', { notes: [noteIDs[0]] });
    const fields = sample[0]?.fields || {};
    ankiState.fields = Object.entries(fields)
        .sort((a, b) => (a[1].order || 0) - (b[1].order || 0))
        .map(([name]) => name);
    fieldSelect.innerHTML = ankiState.fields
        .map(name => `<option value="${escapeAttr(name)}">${escapeHtml(name)}</option>`)
        .join('');
}
function stripHtml(s) {
    const doc = new DOMParser().parseFromString(s, 'text/html');
    return (doc.body.textContent || '').trim();
}
async function importKnownWordsFromAnki() {
    const deckSelect = document.getElementById('vocab-anki-deck');
    const fieldSelect = document.getElementById('vocab-anki-field');
    const button = document.getElementById('vocab-anki-import');
    const status = document.getElementById('vocab-anki-status');
    if (!deckSelect || !fieldSelect || !button)
        return;
    const deck = deckSelect.value;
    const field = fieldSelect.value;
    if (!deck || !field) {
        showToast('Pick a deck and a field first.', 'error');
        return;
    }
    button.disabled = true;
    const origLabel = button.textContent || '';
    button.textContent = 'Importing…';
    if (status)
        status.textContent = `Fetching notes from "${deck}"…`;
    try {
        const noteIDs = await ankiInvoke('findNotes', { query: `deck:"${deck}"` });
        if (noteIDs.length === 0) {
            if (status)
                status.textContent = 'Deck is empty.';
            return;
        }
        const notes = await ankiInvoke('notesInfo', { notes: noteIDs });
        const seen = new Set();
        const words = [];
        for (const note of notes) {
            const raw = note.fields?.[field]?.value || '';
            const text = stripHtml(raw);
            if (!text)
                continue;
            // A field may itself contain multiple comma/newline-separated words.
            for (const w of parseKnownWordsInput(text)) {
                const key = w.toLocaleLowerCase();
                if (seen.has(key))
                    continue;
                seen.add(key);
                words.push(w);
            }
        }
        if (words.length === 0) {
            if (status)
                status.textContent = 'No words extracted from that field.';
            return;
        }
        if (status)
            status.textContent = `Importing ${words.length.toLocaleString()} words…`;
        const data = await postKnownWords(words);
        renderKnownWordsUnresolved(data.unresolved || []);
        if (status)
            status.textContent = `${deck} → ${field}: ${describeImportResult(data)}`;
        await refreshDashboardData();
        await loadKnownWords();
        showToast('Known words imported from Anki.', 'success');
    }
    catch (err) {
        if (status)
            status.textContent = err.message || 'Failed to import.';
        showToast(err.message || 'Failed to import from Anki.', 'error');
    }
    finally {
        button.disabled = false;
        button.textContent = origLabel;
    }
}
function disconnectFromAnki() {
    const stepConnect = document.getElementById('vocab-anki-step-connect');
    const stepPick = document.getElementById('vocab-anki-step-pick');
    const status = document.getElementById('vocab-anki-status');
    ankiState.decks = [];
    ankiState.fields = [];
    stepConnect?.classList.remove('hidden');
    stepPick?.classList.add('hidden');
    if (status)
        status.textContent = '';
}
async function deleteKnownWord(lemma, pos) {
    try {
        const params = new URLSearchParams({ lang: state.activeLanguage, lemma, pos });
        const resp = await fetch(`/api/known-words?${params.toString()}`, {
            method: 'DELETE',
            credentials: 'same-origin',
        });
        if (!resp.ok)
            throw new Error(await resp.text() || 'Failed to remove known word');
        state.knownWords = state.knownWords.filter(word => !(word.lemma === lemma && word.pos === pos && word.lang === state.activeLanguage));
        renderKnownWordsPanel();
        await refreshDashboardData();
        showToast('Known word removed.', 'success');
    }
    catch (err) {
        showToast(err.message || 'Failed to remove known word.', 'error');
    }
}
// ── Language detection (shared between inspect + workbench) ────────────────
const LANG_DETECT_MIN_CHARS = 20;
const LANG_DETECT_SAMPLE_CHARS = 4096;
function detectLang(text) {
    const lower = text.slice(0, LANG_DETECT_SAMPLE_CHARS).toLowerCase();
    const letters = lower.match(/[a-zäöüõ]/g) || [];
    if (letters.length === 0)
        return 'unknown';
    if (/õ/.test(lower))
        return 'ET';
    const nordicCount = (lower.match(/[äö]/g) || []).length;
    if (nordicCount / letters.length > 0.015)
        return 'FI';
    return 'unknown';
}
function getLangWarningState(text, selectedLang) {
    const detected = detectLang(text.trim());
    if (detected === 'unknown') {
        const langName = selectedLang === 'FI' ? 'Finnish' : 'Estonian';
        return {
            detected,
            message: `Warning: this text doesn't contain Finnish or Estonian characters (ä, ö, õ). Is it really ${langName}?`,
            canSwitch: false,
            blocksParse: false,
        };
    }
    if (detected === 'FI' && selectedLang === 'ET') {
        return {
            detected,
            message: 'Warning: you selected Estonian, but this text looks like Finnish. Would you like to switch to Finnish instead?',
            canSwitch: true,
            blocksParse: true,
        };
    }
    if (detected === 'ET' && selectedLang === 'FI') {
        return {
            detected,
            message: 'Warning: you selected Finnish, but this text looks like Estonian. Would you like to switch to Estonian instead?',
            canSwitch: true,
            blocksParse: true,
        };
    }
    return { detected, message: null, canSwitch: false, blocksParse: false };
}
function bindBtnRadio(rootId) {
    const root = document.getElementById(rootId);
    if (!root)
        return null;
    const buttons = Array.from(root.querySelectorAll('button[data-value]'));
    if (buttons.length === 0)
        return null;
    const apply = (v) => {
        for (const b of buttons) {
            const active = b.dataset.value === v;
            b.setAttribute('aria-checked', active ? 'true' : 'false');
            b.classList.toggle('is-active', active);
        }
        root.dataset.value = v;
    };
    apply(root.dataset.value || buttons[0].dataset.value);
    if (root.dataset.btnRadioBound !== '1') {
        root.dataset.btnRadioBound = '1';
        for (const b of buttons) {
            b.addEventListener('click', () => {
                if (root.dataset.value === b.dataset.value)
                    return;
                apply(b.dataset.value);
                root.dispatchEvent(new Event('change'));
            });
        }
    }
    return {
        get value() { return root.dataset.value || ''; },
        set value(v) { apply(v); },
        addEventListener(type, listener) {
            root.addEventListener(type, listener);
        },
    };
}
// Inspect's "language" is the site-wide active language — there's no
// per-form radio anymore. We expose it as a read-only BtnRadioLike so the
// shared parse runner / warning code can stay generic across inspect and the
// admin workbench (which still has a radio for parser testing).
const inspectLangBinding = {
    get value() { return state.activeLanguage; },
    set value(_v) { },
    addEventListener(_type, _listener) { },
};
function getInspectEls() {
    const text = document.getElementById('inspect-text');
    const file = document.getElementById('inspect-file');
    const cc = document.getElementById('inspect-char-count');
    const warn = document.getElementById('inspect-lang-warning');
    const swBtn = document.getElementById('inspect-lang-switch');
    const dz = document.getElementById('inspect-dropzone');
    const pill = document.getElementById('inspect-loaded');
    const chap = document.getElementById('inspect-chapters');
    if (!text || !file || !cc || !warn || !swBtn || !dz || !pill || !chap)
        return null;
    return { lang: inspectLangBinding, text, file, charCount: cc, warning: warn, switchBtn: swBtn, dropzone: dz, loadedPill: pill, chapterList: chap, loadedEpub: null };
}
// Re-run the language-warning gate when the site active language changes.
// Used by setActiveLanguage so an already-pasted text immediately re-evaluates
// against the new language (e.g. the user accepts a switch, warning clears).
function refreshInspectFormForActiveLanguage() {
    updateInspectLede();
    const els = getInspectEls();
    if (!els)
        return;
    updateLangWarning(els, true);
}
// The Inspect page lede references the active language by name (e.g.
// "Paste Finnish text…"). Re-rendered whenever the active language changes
// so the copy keeps pace with the dropdown.
function updateInspectLede() {
    const lede = document.getElementById('inspect-lede');
    if (!lede)
        return;
    lede.textContent = `Paste ${languageName(state.activeLanguage)} text. We'll show unique lemmas, forms, definitions, examples, token counts, and row-level known/ignore actions.`;
}
function getWorkbenchEls() {
    const lang = bindBtnRadio('parse-lang');
    const text = document.getElementById('parse-text');
    const file = document.getElementById('parse-file');
    const cc = document.getElementById('char-count');
    const warn = document.getElementById('lang-warning');
    const swBtn = document.getElementById('lang-switch-btn');
    const dz = document.getElementById('parse-dropzone');
    const pill = document.getElementById('parse-loaded');
    const chap = document.getElementById('parse-chapters');
    if (!lang || !text || !file || !cc || !warn || !swBtn || !dz || !pill || !chap)
        return null;
    return { lang, text, file, charCount: cc, warning: warn, switchBtn: swBtn, dropzone: dz, loadedPill: pill, chapterList: chap, loadedEpub: null };
}
function updateCharCount(els) {
    // When an EPUB is held, the textarea is empty — count from the loaded
    // book's totalChars so the user sees the real size, not "0 / 1,000,000".
    const count = els.loadedEpub
        ? els.loadedEpub.totalChars
        : els.text.value.length;
    els.charCount.textContent = `${count.toLocaleString()} / ${MAX_CHARS.toLocaleString()}`;
    els.charCount.classList.toggle('char-count-warn', count > MAX_CHARS * 0.9);
    els.charCount.classList.toggle('char-count-over', count >= MAX_CHARS);
}
// Text the parser will actually see — the held EPUB when one is loaded, else
// whatever's in the textarea. Used for lang detection and submit gating.
function effectiveSourceText(els) {
    if (els.loadedEpub)
        return els.loadedEpub.fullText;
    return els.text.value;
}
function updateLangWarning(els, gateInspectButton) {
    const source = effectiveSourceText(els);
    if (source.trim().length < LANG_DETECT_MIN_CHARS) {
        els.warning.classList.add('hidden');
        els.switchBtn.classList.add('hidden');
        if (gateInspectButton)
            gateSubmit('inspect-submit', false);
        else
            setParseButtonsDisabled(false);
        return;
    }
    const ws = getLangWarningState(source, els.lang.value);
    if (ws.message) {
        els.warning.textContent = ws.message;
        els.warning.classList.remove('hidden');
        if (ws.canSwitch) {
            els.switchBtn.textContent = `Switch to ${ws.detected === 'FI' ? 'Finnish' : 'Estonian'}`;
            els.switchBtn.classList.remove('hidden');
        }
        else {
            els.switchBtn.classList.add('hidden');
        }
    }
    else {
        els.warning.classList.add('hidden');
        els.switchBtn.classList.add('hidden');
    }
    if (gateInspectButton)
        gateSubmit('inspect-submit', ws.blocksParse);
    else
        setParseButtonsDisabled(ws.blocksParse);
}
function gateSubmit(id, disabled) {
    const btn = document.getElementById(id);
    if (btn)
        btn.disabled = disabled;
}
// Upload an EPUB to /api/import/extract and return its parsed structure, or
// null on failure (caller surfaces toast).
async function uploadEpubForExtraction(file) {
    const form = new FormData();
    form.append('file', file);
    try {
        const resp = await fetch('/api/import/extract', {
            method: 'POST',
            credentials: 'same-origin',
            body: form,
        });
        if (!resp.ok) {
            const msg = await resp.text();
            showToast(msg || 'Failed to extract EPUB.', 'error');
            return null;
        }
        return await resp.json();
    }
    catch (err) {
        showToast(err?.message || 'EPUB upload failed.', 'error');
        return null;
    }
}
// Render the "EPUB loaded" pill above the textarea. Caller is responsible for
// the loadedEpub being non-null on the form.
function renderLoadedPill(els, gateInspectButton) {
    const epub = els.loadedEpub;
    if (!epub) {
        els.loadedPill.classList.add('hidden');
        els.loadedPill.innerHTML = '';
        return;
    }
    const chapWord = epub.chapters.length === 1 ? 'chapter' : 'chapters';
    const subParts = [];
    if (epub.bookAuthor)
        subParts.push(escapeHtml(epub.bookAuthor));
    subParts.push(`${epub.chapters.length} ${chapWord}`);
    subParts.push(`${epub.totalChars.toLocaleString()} characters`);
    subParts.push('ready to parse');
    els.loadedPill.innerHTML = `
        <span class="loaded-icon" aria-hidden="true">📖</span>
        <div class="loaded-meta">
            <span class="loaded-filename" title="${escapeHtml(epub.filename)}">${escapeHtml(epub.bookTitle)}</span>
            <span class="loaded-sub">${subParts.join(' · ')}</span>
        </div>
        <button type="button" class="loaded-clear" aria-label="Clear loaded EPUB">Clear</button>
    `;
    els.loadedPill.classList.remove('hidden');
    const clearBtn = els.loadedPill.querySelector('.loaded-clear');
    clearBtn?.addEventListener('click', () => clearLoadedEpub(els, gateInspectButton));
}
// Rough word count: whitespace-separated runs. Good enough for the chapter
// sidebar's at-a-glance count; the parser's tokenizer is authoritative for
// anything that actually depends on token boundaries.
function countWords(text) {
    const trimmed = text.trim();
    if (!trimmed)
        return 0;
    return trimmed.split(/\s+/).length;
}
// Build a single-line "first 75 chars [...] last 75 chars" preview for a
// chapter. Whitespace is collapsed so line breaks in the source don't waste
// space. Short chapters (≤150 chars after collapse) are shown in full.
function chapterPreviewSnippet(text) {
    const collapsed = text.replace(/\s+/g, ' ').trim();
    if (collapsed.length <= 150)
        return collapsed;
    const head = collapsed.slice(0, 75).trim();
    const tail = collapsed.slice(-75).trim();
    return `${head} [...] ${tail}`;
}
// Render the loaded EPUB's chapter list in place of the textarea. The chapter
// list is scrollable so a 100-chapter book doesn't blow the layout.
function renderChapterList(els) {
    const epub = els.loadedEpub;
    if (!epub) {
        els.chapterList.classList.add('hidden');
        els.chapterList.innerHTML = '';
        return;
    }
    els.chapterList.innerHTML = epub.chapters.map(ch => {
        const snippet = chapterPreviewSnippet(ch.text);
        return `
            <li data-tooltip-snippet="${escapeHtml(snippet)}">
                <span class="chapter-title">${escapeHtml(ch.title)}</span>
                <span class="chapter-size">${ch.char_count.toLocaleString()} chars</span>
            </li>
        `;
    }).join('');
    els.chapterList.classList.remove('hidden');
}
function hideChapterList(els) {
    els.chapterList.classList.add('hidden');
    els.chapterList.innerHTML = '';
}
// Render the results-page chapter-filter sidebar. Hidden when no EPUB is held
// or no results are in view. Wires click handlers to selectChapter on each
// row.
function renderChapterNav() {
    const nav = document.getElementById('results-chapter-nav');
    if (!nav)
        return;
    const epub = state.resultsEpub;
    if (!epub || !state.currentResults || state.activeChapterIdx === null) {
        nav.classList.add('hidden');
        nav.innerHTML = '';
        return;
    }
    const active = state.activeChapterIdx;
    // "X words" available immediately, "Y lemmas" only once the chapter has
    // been parsed (i.e. is in the cache). Em-dash placeholder until then.
    const subFor = (idx, words) => {
        const wordsLabel = `${words.toLocaleString()} word${words === 1 ? '' : 's'}`;
        const cached = state.epubChapterCache.get(idx);
        const lemmasLabel = cached
            ? `${cached.words.length.toLocaleString()} lemma${cached.words.length === 1 ? '' : 's'}`
            : '— lemmas';
        return `${wordsLabel} · ${lemmasLabel}`;
    };
    const rowFor = (idx, label, sub, extraCls = '') => {
        const cls = ['chapter-nav-item'];
        if (idx === active)
            cls.push('active');
        return `<li class="${extraCls}"><button type="button" class="${cls.join(' ')}" data-chapter-idx="${idx}">
            <span class="chapter-nav-label">${label}</span>
            <span class="chapter-nav-sub">${sub}</span>
        </button></li>`;
    };
    const chapterRows = [];
    for (let i = 0; i < epub.chapters.length; i++) {
        const ch = epub.chapters[i];
        chapterRows.push(rowFor(i, escapeHtml(ch.title), subFor(i, ch.word_count)));
    }
    nav.innerHTML = `
        <ol class="chapter-nav-list">
            ${rowFor(-1, '📖 Whole book', subFor(-1, epub.totalWords), 'chapter-nav-whole-book')}
            ${chapterRows.join('')}
        </ol>
    `;
    nav.classList.remove('hidden');
    nav.querySelectorAll('.chapter-nav-item').forEach(btn => {
        btn.addEventListener('click', () => {
            const idx = parseInt(btn.dataset.chapterIdx || '-1', 10);
            void selectChapter(idx);
        });
    });
}
// Switch the displayed results to a different EPUB chapter (or the whole
// book at idx = -1). Cached parses render instantly; uncached parses fetch
// /api/parse with the chapter text. Lang and parser mode are inherited from
// the existing whole-book parse.
async function selectChapter(idx) {
    const epub = state.resultsEpub;
    if (!epub)
        return;
    if (state.activeChapterIdx === idx && state.epubChapterCache.has(idx))
        return;
    if (idx !== -1 && (idx < 0 || idx >= epub.chapters.length))
        return;
    const text = idx === -1 ? epub.fullText : epub.chapters[idx].text;
    if (!text.trim()) {
        showToast('That chapter has no parseable text.', 'info');
        return;
    }
    // Apply the active highlight IMMEDIATELY and yield so the browser paints
    // it before we do anything expensive. Otherwise the heavy table render
    // inside showResults blocks the next paint and the click feels laggy.
    state.activeChapterIdx = idx;
    renderChapterNav();
    await afterNextPaint();
    const cached = state.epubChapterCache.get(idx);
    if (cached) {
        const preview = idx === -1 ? epub.bookTitle : epub.chapters[idx].title;
        showResults(cached, preview, state.currentParserMode, state.currentContext);
        return;
    }
    const lang = state.currentResults?.lang || 'FI';
    try {
        const resp = await fetch('/api/parse', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'same-origin',
            // Chapter parses are auxiliary data — no parse_sessions row.
            // The whole-book parse (-1) remains persistent; we never reach
            // this branch for -1 because the whole-book cache is seeded by
            // the user-initiated runParse before any chapter click.
            body: JSON.stringify({ lang, text, parser: state.currentParserMode }),
        });
        if (!resp.ok)
            throw new Error(await resp.text() || resp.statusText);
        const data = await resp.json();
        state.epubChapterCache.set(idx, data);
        const preview = idx === -1 ? epub.bookTitle : epub.chapters[idx].title;
        showResults(data, preview, state.currentParserMode, state.currentContext);
    }
    catch (err) {
        showToast(err?.message || 'Chapter parse failed.', 'error');
        const fallback = state.epubChapterCache.has(-1) ? -1 : null;
        state.activeChapterIdx = fallback;
        renderChapterNav();
    }
}
// Resolves after the next browser paint. Double rAF: the first callback fires
// before the upcoming paint, the second fires before the paint AFTER that —
// guaranteeing at least one paint has happened in between.
function afterNextPaint() {
    return new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(() => resolve())));
}
// synthesizeChapterResponse wraps one ChapterResponseEntry into a
// ParseResponse-shaped value so the chapter cache stays uniform with the
// whole-book entry. lang and parser_duration_ms come from the parent response
// since the chapter's words were produced inside the same parse call.
function synthesizeChapterResponse(parent, chapter) {
    return {
        lang: parent.lang,
        // No parse_id: chapters are derived data inside a non-persisted parse.
        total_tokens: chapter.token_count,
        parse_duration_ms: parent.parse_duration_ms,
        words: chapter.words,
    };
}
// Drop the held EPUB from THIS form and re-enable normal textarea editing.
// Results-page state (state.resultsEpub + chapter cache) is left alone since
// the user might still be reading the prior parse's results; the next Parse
// press is what invalidates them.
function clearLoadedEpub(els, gateInspectButton) {
    els.loadedEpub = null;
    renderLoadedPill(els, gateInspectButton);
    hideChapterList(els);
    els.text.classList.remove('hidden');
    els.text.disabled = false;
    els.text.placeholder = els.text.dataset.originalPlaceholder || '';
    els.text.value = '';
    updateCharCount(els);
    updateLangWarning(els, gateInspectButton);
}
// Load a user-uploaded file. .epub is uploaded to the server, held in state,
// and surfaced as a pill — the textarea is left empty and disabled until the
// user clears it. .txt/.md is read client-side and pasted into the textarea
// (existing behavior).
async function loadFileIntoForm(els, file, gateInspectButton) {
    const lower = file.name.toLowerCase();
    if (lower.endsWith('.epub')) {
        const data = await uploadEpubForExtraction(file);
        if (!data)
            return;
        const chaptersRaw = data.chapters ?? [];
        const chapters = chaptersRaw.map(ch => ({
            ...ch,
            word_count: countWords(ch.text),
        }));
        const filename = data.filename || file.name;
        const bookTitle = (data.book_title || '').trim() || filename.replace(/\.epub$/i, '');
        els.loadedEpub = {
            filename,
            fullText: data.text,
            chapters,
            totalChars: data.char_count || data.text.length,
            totalWords: countWords(data.text),
            bookTitle,
            bookAuthor: (data.book_author || '').trim(),
        };
        if (data.truncated) {
            showToast(`EPUB is large — kept the first ${MAX_CHARS.toLocaleString()} characters for analysis.`, 'info');
        }
        // Hide the textarea entirely while a book is held and show the
        // chapter list in its place. The pill's Clear button restores both.
        if (els.text.dataset.originalPlaceholder === undefined) {
            els.text.dataset.originalPlaceholder = els.text.placeholder;
        }
        els.text.value = '';
        els.text.disabled = true;
        els.text.classList.add('hidden');
        renderChapterList(els);
        renderLoadedPill(els, gateInspectButton);
        updateCharCount(els);
        // Auto-detect language from the first chapter body.
        const sniff = chapters[0]?.text || data.text.slice(0, 4000);
        runLangDetectOnText(els, sniff, gateInspectButton, 'file');
        return;
    }
    // .txt / .md and unknown extensions — read client-side and populate the
    // textarea, dropping any previously held EPUB on this form.
    if (els.loadedEpub)
        clearLoadedEpub(els, gateInspectButton);
    try {
        const raw = await file.text();
        els.text.value = raw.slice(0, MAX_CHARS);
        updateCharCount(els);
        maybeAutoSwitchFromIngest(els, gateInspectButton, 'file');
    }
    catch (err) {
        showToast(err?.message || 'Could not read file.', 'error');
    }
}
// Wire drag/drop on the dropzone wrapper. preventDefault on dragover is
// unconditional — gating it on a types.includes('Files') check is unreliable
// because some browsers (notably Firefox) hide types during dragover for
// security and the drop event then never fires. The only thing we actually
// need to gate is whether to process the dropped payload, which we do by
// looking at e.dataTransfer.files at drop time.
function wireDragDrop(els, gateInspectButton) {
    const zone = els.dropzone;
    const setDragging = (on) => zone.classList.toggle('drag-over', on);
    zone.addEventListener('dragenter', (e) => {
        e.preventDefault();
        setDragging(true);
    });
    zone.addEventListener('dragover', (e) => {
        e.preventDefault();
        if (e.dataTransfer)
            e.dataTransfer.dropEffect = 'copy';
        setDragging(true);
    });
    zone.addEventListener('dragleave', (e) => {
        // Only clear when the cursor actually leaves the dropzone — not when
        // it crosses an internal child boundary (textarea → CTA, etc.).
        // relatedTarget is where the cursor is going next; if it's still
        // inside the zone, ignore.
        const related = e.relatedTarget;
        if (related && zone.contains(related))
            return;
        setDragging(false);
    });
    zone.addEventListener('drop', async (e) => {
        e.preventDefault();
        setDragging(false);
        const file = e.dataTransfer?.files?.[0];
        if (!file)
            return;
        await loadFileIntoForm(els, file, gateInspectButton);
    });
}
// Page-level safety net: when a user drops a file anywhere on the page, the
// browser's default is to navigate to (or download) the file, losing the
// session. Run in CAPTURE phase so preventDefault fires before anything along
// the dispatch path can act on the default. Each dropzone's own bubble-phase
// drop handler still runs and processes the dropped file.
function preventStrayFileDrops() {
    const stop = (e) => {
        if (e.type === 'drop' && !e.dataTransfer?.files?.length)
            return;
        e.preventDefault();
    };
    document.addEventListener('dragenter', stop, true);
    document.addEventListener('dragover', stop, true);
    document.addEventListener('drop', stop, true);
}
function maybeAutoSwitchFromIngest(els, gateInspectButton, source) {
    runLangDetectOnText(els, els.text.value, gateInspectButton, source);
}
// Inspect: never auto-switch the site's active language behind the user's
// back. updateLangWarning surfaces a "Switch to X" button when detection
// disagrees with the active language, and the button (wired in
// initInspectForm) routes through setActiveLanguage so the change persists
// site-wide.
//
// Workbench (admin): the form's lang radio is local-only, so on a paste/file
// that looks like the other language we still auto-switch the radio — it
// doesn't touch site state and saves an extra click for the parser-testing
// flow.
function runLangDetectOnText(els, sourceText, gateInspectButton, source) {
    if (sourceText.trim().length < LANG_DETECT_MIN_CHARS) {
        updateLangWarning(els, gateInspectButton);
        return;
    }
    if (!gateInspectButton) {
        const detected = detectLang(sourceText);
        if (detected !== 'unknown' && detected !== els.lang.value) {
            els.lang.value = detected;
            const sourceLabel = source === 'paste' ? 'pasted text' : 'file content';
            showToast(`Switched to ${detected === 'FI' ? 'Finnish' : 'Estonian'} — detected from ${sourceLabel}`, 'info');
        }
    }
    updateLangWarning(els, gateInspectButton);
}
function initInspectForm() {
    const els = getInspectEls();
    if (!els)
        return;
    els.text.addEventListener('input', () => {
        updateCharCount(els);
        updateLangWarning(els, true);
    });
    els.text.addEventListener('paste', () => {
        setTimeout(() => {
            maybeAutoSwitchFromIngest(els, true, 'paste');
        }, 0);
    });
    els.switchBtn.addEventListener('click', async () => {
        const ws = getLangWarningState(effectiveSourceText(els), els.lang.value);
        if (ws.canSwitch && (ws.detected === 'FI' || ws.detected === 'ET')) {
            // The site-wide setter handles persistence + dropdown re-render +
            // re-running the warning. We just need to make sure the language
            // is in the user's learning set first; otherwise add it for them.
            if (!state.learningLanguages.includes(ws.detected)) {
                const next = SUPPORTED_LANGUAGES.filter(l => state.learningLanguages.includes(l) || l === ws.detected);
                const ok = await patchUserLanguages({ learning: next, active: ws.detected });
                if (ok)
                    onActiveLanguageChanged();
                return;
            }
            await setActiveLanguage(ws.detected);
        }
    });
    els.file.addEventListener('change', async (e) => {
        const input = e.target;
        const file = input.files?.[0];
        if (!file)
            return;
        await loadFileIntoForm(els, file, true);
    });
    wireDragDrop(els, true);
    const form = document.getElementById('inspect-form');
    form?.addEventListener('submit', async (e) => {
        e.preventDefault();
        await runParse(els, 'custom', 'inspect', 'inspect-submit');
    });
}
// ── Workbench form (admin surface — keeps prior behavior) ──────────────────
function initWorkbenchForm() {
    const els = getWorkbenchEls();
    if (!els)
        return;
    els.text.addEventListener('input', () => {
        updateCharCount(els);
        updateLangWarning(els, false);
    });
    els.text.addEventListener('paste', () => {
        setTimeout(() => {
            maybeAutoSwitchFromIngest(els, false, 'paste');
        }, 0);
    });
    els.lang.addEventListener('change', () => updateLangWarning(els, false));
    els.switchBtn.addEventListener('click', () => {
        const ws = getLangWarningState(effectiveSourceText(els), els.lang.value);
        if (ws.canSwitch) {
            els.lang.value = ws.detected;
            updateLangWarning(els, false);
        }
    });
    els.file.addEventListener('change', async (e) => {
        const input = e.target;
        const file = input.files?.[0];
        if (!file)
            return;
        await loadFileIntoForm(els, file, false);
    });
    wireDragDrop(els, false);
    const handle = (mode, btnId) => async (e) => {
        e.preventDefault();
        await runParse(els, mode, 'workbench', btnId);
    };
    document.getElementById('parse-btn-basic')?.addEventListener('click', handle('basic', 'parse-btn-basic'));
    document.getElementById('parse-btn-custom')?.addEventListener('click', handle('custom', 'parse-btn-custom'));
    document.getElementById('parse-form')?.addEventListener('submit', (e) => {
        e.preventDefault();
        // Submitting via Enter behaves like the basic button.
        runParse(els, 'basic', 'workbench', 'parse-btn-basic');
    });
}
// ── Shared parse runner ────────────────────────────────────────────────────
async function runParse(els, parserMode, context, activeBtnId) {
    // EPUB scope is per-form: only THIS form's loaded book contributes to
    // the parse input. A book held in inspect won't be parsed when the user
    // hits Parse from the workbench, and vice versa.
    const epub = els.loadedEpub;
    const text = (epub ? epub.fullText : els.text.value).trim();
    const lang = els.lang.value;
    const ws = getLangWarningState(text, lang);
    if (!text)
        return;
    if (text.length > MAX_CHARS) {
        showToast(`Text must be ${MAX_CHARS.toLocaleString()} characters or fewer.`, 'error');
        return;
    }
    if (ws.blocksParse)
        return;
    const activeBtn = document.getElementById(activeBtnId);
    const origLabel = activeBtn?.textContent || '';
    // Disable all parse buttons in the current form
    if (context === 'workbench')
        setParseButtonsDisabled(true);
    if (activeBtn) {
        activeBtn.disabled = true;
        activeBtn.textContent = 'Parsing…';
    }
    // Fresh top-level parse — drop any per-chapter cache from a previous
    // EPUB. Whole-book result is re-cached below once the request returns,
    // and per-chapter entries are seeded from response.chapters when the
    // EPUB path is used.
    state.epubChapterCache.clear();
    state.activeChapterIdx = null;
    // When an EPUB is loaded, send the chapter array so the server can
    // produce per-chapter Words in a single response. Plain-text parses still
    // send `text`. Server rejects both being populated, so they're mutually
    // exclusive on this side too.
    const body = epub
        ? { lang, parser: parserMode, chapters: epub.chapters.map(ch => ({ title: ch.title, text: ch.text })) }
        : { lang, parser: parserMode, text };
    try {
        const resp = await fetch('/api/parse', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'same-origin',
            body: JSON.stringify(body),
        });
        if (!resp.ok) {
            const msg = await resp.text();
            throw new Error(msg || resp.statusText);
        }
        const data = await resp.json();
        state.currentSourceText = text;
        const preview = epub ? epub.bookTitle : text.slice(0, 60);
        if (epub) {
            state.resultsEpub = epub;
            state.epubChapterCache.set(-1, data);
            state.activeChapterIdx = -1;
            // Seed per-chapter cache from the single response so chapter
            // clicks render instantly with no extra /api/parse round-trip.
            // Each chapter's words match what a stand-alone /api/parse for
            // that chapter would have returned, modulo learning_state which
            // is already applied server-side for both whole-book and per-
            // chapter Words.
            if (data.chapters) {
                data.chapters.forEach((ch, idx) => {
                    state.epubChapterCache.set(idx, synthesizeChapterResponse(data, ch));
                });
            }
        }
        else {
            state.resultsEpub = null;
        }
        showResults(data, preview, parserMode, context);
    }
    catch (err) {
        showToast(`Parse failed: ${err.message}`, 'error');
    }
    finally {
        if (activeBtn) {
            activeBtn.disabled = false;
            activeBtn.textContent = origLabel;
        }
        if (context === 'workbench') {
            updateLangWarning(els, false);
        }
        else {
            updateLangWarning(els, true);
        }
    }
}
// ── Results rendering (shared) ─────────────────────────────────────────────
function computeCoverageScore(data) {
    const definedRows = data.words.filter(word => Boolean(word.gloss)).length;
    const definedTokens = data.words.reduce((sum, word) => sum + (word.gloss ? word.count : 0), 0);
    const expandedTokens = data.words.reduce((sum, word) => sum + word.count, 0);
    const tokenDenominator = Math.max(data.total_tokens, expandedTokens);
    const rowCoverage = data.words.length === 0 ? 0 : definedRows / data.words.length;
    const tokenCoverage = tokenDenominator === 0 ? 0 : definedTokens / tokenDenominator;
    const score = Math.min(100, Math.round(((tokenCoverage * 0.7) + (rowCoverage * 0.3)) * 100));
    return { score, definedRows, definedTokens, rowCoverage, tokenCoverage };
}
function matchesPOSFilter(pos, filter) {
    if (filter === 'all')
        return true;
    if (filter === 'NOUN')
        return NOUN_POS.includes(pos);
    if (filter === 'VERB')
        return VERB_POS.includes(pos);
    if (filter === 'ADJ')
        return ADJ_POS.includes(pos);
    if (filter === 'ADV')
        return ADV_POS.includes(pos);
    if (filter === 'other')
        return OTHER_POS.includes(pos);
    return true;
}
function filterWords(words, filter) {
    return filter === 'all' ? words : words.filter(w => matchesPOSFilter(w.pos, filter));
}
function sortWords(words, sort) {
    const sorted = [...words];
    const direction = sort.dir === 'asc' ? 1 : -1;
    sorted.sort((a, b) => {
        let cmp = 0;
        switch (sort.key) {
            case 'row':
                cmp = a.originalIndex - b.originalIndex;
                break;
            case 'lemma':
                cmp = compareStrings(a.lemma, b.lemma);
                break;
            case 'pos':
                cmp = compareStrings(posLabel(a.pos), posLabel(b.pos));
                break;
            case 'forms':
                cmp = compareStrings(a.forms.join(', '), b.forms.join(', '));
                break;
            case 'definition': {
                const aMissing = a.gloss ? 0 : 1;
                const bMissing = b.gloss ? 0 : 1;
                cmp = aMissing - bMissing;
                if (cmp === 0)
                    cmp = compareStrings(a.gloss || '', b.gloss || '');
                break;
            }
            case 'tokens':
                cmp = a.count - b.count;
                break;
        }
        if (cmp !== 0)
            return cmp * direction;
        return a.originalIndex - b.originalIndex;
    });
    return sorted;
}
function highlightFormsInSentence(sentence, forms) {
    let result = escapeHtml(sentence);
    for (const form of forms) {
        const escaped = escapeRegExp(escapeHtml(form));
        const regex = new RegExp(`\\b(${escaped})\\b`, 'gi');
        result = result.replace(regex, '<span class="highlight-form">$1</span>');
    }
    return result;
}
function updateSortButtons() {
    document.querySelectorAll('.sort-btn').forEach(btn => {
        const key = btn.dataset.sort;
        if (!key)
            return;
        const active = state.currentSort.key === key;
        btn.classList.toggle('active', active);
        btn.setAttribute('aria-sort', active ? state.currentSort.dir : 'none');
        const arrow = btn.querySelector('.sort-arrow');
        if (arrow)
            arrow.textContent = active ? (state.currentSort.dir === 'asc' ? ' ↑' : ' ↓') : '';
    });
}
function updatePOSFilterButtons() {
    document.querySelectorAll('.pos-filter-chip').forEach(btn => {
        const filter = btn.dataset.filter;
        const active = filter === state.currentPOSFilter;
        btn.classList.toggle('active', active);
        btn.setAttribute('aria-pressed', active ? 'true' : 'false');
    });
}
function renderResultsTable(data) {
    const tbody = document.getElementById('word-table-body');
    const help = document.getElementById('results-help');
    if (!tbody || !help)
        return;
    const baseWords = data.words.map((word, index) => ({ ...word, originalIndex: index }));
    const filteredWords = filterWords(baseWords, state.currentPOSFilter);
    const sortedWords = sortWords(filteredWords, state.currentSort);
    const hasGrammar = data.words.some(word => Boolean(word.grammar_label));
    const showActions = state.role === 'user' || state.role === 'admin';
    help.textContent = hasGrammar
        ? `Coverage = dictionary-backed tokens. Grammar labels shown as badges when case/morphology was inferred.`
        : `Coverage = dictionary-backed tokens. Grammar labels appear when case/morphology inference is available.`;
    // If a tooltip is currently visible, its trigger is about to be wiped by
    // the innerHTML reassignment below. The browser won't fire mouseout for
    // a removed-then-replaced element, so we hide explicitly to avoid a
    // stuck tooltip when the user hasn't moved the mouse.
    hidePortalTooltip();
    tbody.innerHTML = sortedWords.map((w, index) => {
        const grammarBadge = w.grammar_label
            ? `<span class="grammar-badge">${escapeHtml(w.grammar_label)}</span>`
            : '';
        const rowKey = `${index}`;
        // Example sentence is NOT rendered into the DOM until the user clicks
        // the toggle. Storing the highlighted HTML in a data-* attribute keeps
        // the text out of the searchable document, so browser find (Ctrl+F)
        // can't match against collapsed examples.
        const exampleToggle = w.example_sentence
            ? `<button type="button" class="example-toggle" data-example-toggle="${rowKey}" aria-expanded="false" data-example-html="${escapeAttr(highlightFormsInSentence(w.example_sentence, w.forms))}">▸ example</button>`
            : '';
        const exampleBlock = w.example_sentence
            ? `<div class="example-text hidden" data-example-text="${rowKey}" hidden></div>`
            : '';
        const glossHtml = w.gloss
            ? escapeHtml(w.gloss)
            : '<span class="no-gloss">Missing</span>';
        const posPill = `<span class="pos-pill" data-pos="${escapeHtml(w.pos)}" data-tooltip="${escapeHtml(posLabel(w.pos))}">${escapeHtml(posAbbrev(w.pos))}</span>`;
        const surfaceForm = w.forms[0] || w.lemma;
        const rowStatus = currentLemmaState(w.lemma, w.pos);
        const rowPending = state.pendingLemmaStates.has(lemmaStateKey(data.lang, w.lemma, w.pos));
        const isKnown = rowStatus === 'known';
        const isIgnored = rowStatus === 'ignored';
        const knownNextStatus = isKnown ? 'neutral' : 'known';
        const ignoreNextStatus = isIgnored ? 'neutral' : 'ignored';
        const knownTooltip = isKnown ? 'Mark as unknown' : 'Mark as known';
        const ignoreTooltip = isIgnored ? 'Stop ignoring' : 'Ignore';
        const knownIcon = isKnown
            ? '<svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true"><path d="M3.5 8.5l3 3 6-6.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>'
            : '<svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true"><circle cx="8" cy="8" r="6" fill="none" stroke="currentColor" stroke-width="1.5"/></svg>';
        const trashIcon = '<svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true"><path d="M3 4h10M6.5 4V2.5h3V4M5 4l.7 9.5a1 1 0 001 .9h2.6a1 1 0 001-.9L11 4M7 7v5M9 7v5" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"/></svg>';
        const pencilIcon = '<svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true"><path d="M11.5 2.5l2 2-7.5 7.5-2.5.5.5-2.5 7.5-7.5z" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"/></svg>';
        const actionCell = showActions
            ? `<td class="col-actions"><div class="word-actions">
                <button type="button"
                    class="word-pill word-pill-known${isKnown ? ' is-active' : ''}"
                    data-lemma-status="${knownNextStatus}"
                    data-lemma="${escapeHtml(w.lemma)}"
                    data-pos="${escapeHtml(w.pos)}"
                    data-tooltip="${knownTooltip}"
                    ${rowPending ? 'disabled' : ''}>
                    ${knownIcon}<span>${isKnown ? 'Known' : 'Unknown'}</span>
                </button>
                <button type="button"
                    class="word-icon-btn word-icon-ignore${isIgnored ? ' is-active' : ''}"
                    data-lemma-status="${ignoreNextStatus}"
                    data-lemma="${escapeHtml(w.lemma)}"
                    data-pos="${escapeHtml(w.pos)}"
                    data-tooltip="${ignoreTooltip}"
                    aria-label="${ignoreTooltip}"
                    ${rowPending ? 'disabled' : ''}>${trashIcon}</button>
                <button type="button" class="word-icon-btn correction-btn"
                    data-lemma="${escapeHtml(w.lemma)}"
                    data-pos="${escapeHtml(w.pos)}"
                    data-surface="${escapeHtml(surfaceForm)}"
                    data-grammar="${escapeHtml(w.grammar_label || '')}"
                    data-tooltip="Suggest fix"
                    aria-label="Suggest fix">${pencilIcon}</button>
               </div></td>`
            : '';
        return `<tr class="word-row">
            <td class="col-row">${index + 1}</td>
            <td class="col-lemma">
                <div class="lemma-pos-grid">
                    <span class="lemma-side">${escapeHtml(w.lemma)}${grammarBadge}</span>
                    <span class="pos-side">${posPill}</span>
                </div>
                ${exampleToggle}
                ${exampleBlock}
            </td>
            <td class="col-def">${glossHtml}</td>
            <td class="col-count">${w.count}</td>
            ${actionCell}
        </tr>`;
    }).join('');
    updateSortButtons();
    updatePOSFilterButtons();
    tbody.querySelectorAll('.word-pill, .word-icon-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            const status = btn.dataset.lemmaStatus;
            const lemma = btn.dataset.lemma || '';
            const pos = btn.dataset.pos || '';
            if (status && lemma && pos) {
                void markResultLemma(status, lemma, pos, btn);
            }
        });
    });
    tbody.querySelectorAll('.example-toggle').forEach(btn => {
        btn.addEventListener('click', () => {
            const key = btn.dataset.exampleToggle || '';
            const example = tbody.querySelector(`.example-text[data-example-text="${key}"]`);
            if (!example)
                return;
            const willOpen = example.classList.contains('hidden');
            if (willOpen) {
                if (!example.innerHTML) {
                    example.innerHTML = btn.dataset.exampleHtml || '';
                }
                example.classList.remove('hidden');
                example.removeAttribute('hidden');
            }
            else {
                example.classList.add('hidden');
                example.setAttribute('hidden', '');
                // Wipe the rendered HTML so Ctrl+F can't match it while
                // collapsed (defense in depth alongside display:none).
                example.innerHTML = '';
            }
            btn.setAttribute('aria-expanded', willOpen ? 'true' : 'false');
            btn.textContent = willOpen ? '▾ example' : '▸ example';
        });
    });
    // Wire up newly-rendered correction buttons
    tbody.querySelectorAll('.correction-btn').forEach(btn => {
        btn.addEventListener('click', () => openCorrectionModal({
            lemma: btn.dataset.lemma || '',
            pos: btn.dataset.pos || '',
            surface: btn.dataset.surface || btn.dataset.lemma || '',
            occurrence: 0,
            original_grammar_label: btn.dataset.grammar || '',
        }));
    });
}
function showResults(data, textPreview, parserMode, context) {
    const langName = data.lang === 'FI' ? 'Finnish' : 'Estonian';
    const preview = textPreview.replace(/\s+/g, ' ').trim();
    const ellipsis = preview.length >= 60 ? '…' : '';
    const uniqueLemmas = data.words.length;
    const coverage = computeCoverageScore(data);
    document.body.dataset.resultsContext = context;
    document.getElementById('results-title').textContent = context === 'deck'
        ? `${preview} (${langName})`
        : `"${preview}${ellipsis}" (${langName})`;
    // Parser pill: deck shows "Saved deck", inspect shows a softer label,
    // workbench/admin shows the parser mode.
    const parserPill = document.getElementById('results-parser');
    if (context === 'deck') {
        parserPill.textContent = 'Saved deck';
    }
    else if (context === 'inspect' && state.role !== 'admin') {
        parserPill.textContent = 'Your text';
    }
    else {
        parserPill.textContent = formatParserMode(parserMode);
    }
    document.getElementById('results-duration').textContent =
        `Parse time ${formatParseDuration(data.parse_duration_ms)}`;
    const createdPill = document.getElementById('results-created');
    if (createdPill) {
        if (context === 'deck' && state.currentDeckCreatedAt) {
            createdPill.textContent = `Saved ${state.currentDeckCreatedAt}`;
            createdPill.classList.remove('hidden');
        }
        else {
            createdPill.textContent = '';
            createdPill.classList.add('hidden');
        }
    }
    const coverageFill = document.getElementById('coverage-fill');
    const coverageValue = document.getElementById('coverage-value');
    coverageFill.style.width = `${coverage.score}%`;
    coverageFill.classList.remove('low', 'medium', 'high');
    coverageFill.classList.add(coverage.score >= 80 ? 'high' : coverage.score >= 50 ? 'medium' : 'low');
    coverageValue.textContent = `${coverage.score}%`;
    document.getElementById('results-stats').textContent =
        `${uniqueLemmas} unique lemmas · ${data.total_tokens} tokens · `
            + `${coverage.definedRows}/${uniqueLemmas} with definitions`;
    state.currentResults = data;
    state.currentContext = context;
    state.currentParserMode = parserMode;
    state.currentTextPreview = preview;
    state.currentSort = { key: 'tokens', dir: 'desc' };
    state.currentPOSFilter = 'all';
    state.currentLemmaStates.clear();
    state.pendingLemmaStates.clear();
    for (const word of data.words) {
        if (word.learning_state) {
            state.currentLemmaStates.set(lemmaStateKey(data.lang, word.lemma, word.pos), word.learning_state);
        }
    }
    renderResultsTable(data);
    renderResultsSaveState();
    renderChapterNav();
    // Re-apply role visibility so admin-only pills/cells show correctly.
    applyRoleVisibility();
    if (context !== 'deck') {
        // Persist the parse so a hard refresh on /results doesn't drop the user
        // onto an empty page. Deck detail handles its own refresh by re-fetching
        // from /api/decks/:id, so we skip that context here.
        persistLastParse(data, preview, parserMode, context);
        navigate('/results');
    }
}
const LAST_PARSE_KEY = 'finnestdb:lastParse:v1';
function persistLastParse(data, textPreview, parserMode, context) {
    try {
        const payload = {
            data,
            textPreview,
            parserMode,
            context,
            sourceText: state.currentSourceText,
        };
        sessionStorage.setItem(LAST_PARSE_KEY, JSON.stringify(payload));
    }
    catch {
        // Quota exceeded or sessionStorage unavailable — silently skip; the
        // page will just be empty after refresh, same as before.
    }
}
async function hydrateLearningStates(data) {
    const seen = new Set();
    const lemmas = [];
    for (const word of data.words) {
        const lemma = word.lemma.trim();
        const pos = word.pos.trim();
        if (!lemma || !pos)
            continue;
        const key = `${lemma}\u0000${pos}`;
        if (seen.has(key))
            continue;
        seen.add(key);
        lemmas.push({ lemma, pos });
    }
    if (lemmas.length === 0)
        return data;
    try {
        const resp = await fetch('/api/lemma-states', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'same-origin',
            body: JSON.stringify({ lang: data.lang, lemmas }),
        });
        if (!resp.ok)
            return data;
        const payload = await resp.json();
        const states = new Map();
        for (const item of payload.states || []) {
            if (item.status) {
                states.set(lemmaStateKey(data.lang, item.lemma, item.pos), item.status);
            }
        }
        return {
            ...data,
            words: data.words.map(word => {
                const refreshed = { ...word };
                delete refreshed.learning_state;
                const status = states.get(lemmaStateKey(data.lang, word.lemma, word.pos));
                if (status)
                    refreshed.learning_state = status;
                return refreshed;
            }),
        };
    }
    catch {
        return data;
    }
}
async function restoreLastParse() {
    let payload;
    try {
        const raw = sessionStorage.getItem(LAST_PARSE_KEY);
        if (!raw)
            return false;
        const parsed = JSON.parse(raw);
        if (!parsed?.data || !parsed.context)
            return false;
        payload = parsed;
    }
    catch {
        return false;
    }
    state.currentSourceText = payload.sourceText || '';
    // Refresh only learning_state. Re-posting to /api/parse would create a new
    // parse_sessions row for authenticated users, making refresh a write.
    const fresh = await hydrateLearningStates(payload.data);
    showResults(fresh, payload.textPreview, payload.parserMode, payload.context);
    return true;
}
async function loadDeckDetail(deckID) {
    document.body.dataset.resultsContext = 'deck';
    const titleEl = document.getElementById('results-title');
    if (titleEl)
        titleEl.textContent = 'Loading deck…';
    const tbody = document.getElementById('word-table-body');
    if (tbody)
        tbody.innerHTML = '';
    try {
        const resp = await fetch(`/api/decks/${deckID}`, { credentials: 'same-origin' });
        if (resp.status === 404) {
            showToast('Deck not found.', 'error');
            navigate('/decks');
            return;
        }
        if (!resp.ok)
            throw new Error(await resp.text() || 'Failed to load deck');
        const data = await resp.json();
        state.currentDeckID = data.id;
        state.currentDeckCreatedAt = formatDeckCreatedAt(data.created_at);
        state.currentSourceText = '';
        const parseResponse = {
            lang: data.lang,
            parse_id: data.parse_id,
            total_tokens: data.total_tokens,
            parse_duration_ms: 0,
            words: data.words,
        };
        showResults(parseResponse, data.title, data.parser || 'custom', 'deck');
    }
    catch (err) {
        showToast(err.message || 'Failed to load deck.', 'error');
        navigate('/decks');
    }
}
function formatDeckCreatedAt(iso) {
    const d = new Date(iso);
    if (Number.isNaN(d.getTime()))
        return '';
    return d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
}
function renderResultsSaveState() {
    const cta = document.querySelector('.results-deck-cta');
    const form = document.getElementById('results-save-form');
    const input = document.getElementById('results-deck-title');
    if (cta)
        cta.classList.toggle('hidden', state.currentContext === 'deck');
    if (!form || !input)
        return;
    form.classList.add('hidden');
    // For an EPUB import, the book title is a much better deck-name default
    // than "Finnish: <first chars of body>". Use the results-context EPUB so
    // a still-loaded book in another form doesn't leak into this default.
    const epub = state.resultsEpub;
    if (epub) {
        input.value = epub.bookTitle;
        return;
    }
    const langName = state.currentResults?.lang === 'ET' ? 'Estonian' : 'Finnish';
    input.value = state.currentTextPreview
        ? `${langName}: ${state.currentTextPreview.slice(0, 48)}`
        : '';
}
async function saveCurrentResultsAsDeck() {
    const titleInput = document.getElementById('results-deck-title');
    const submitBtn = document.getElementById('results-save-submit');
    const publicCheckbox = document.getElementById('results-deck-public');
    if (!titleInput || !submitBtn || !state.currentResults || !state.currentSourceText.trim())
        return;
    const title = titleInput.value.trim();
    if (!title) {
        showToast('Please enter a deck title.', 'error');
        return;
    }
    const isPublic = state.role === 'admin' && !!publicCheckbox?.checked;
    submitBtn.disabled = true;
    const originalLabel = submitBtn.textContent || '';
    submitBtn.textContent = 'Saving…';
    try {
        const resp = await fetch('/api/decks', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'same-origin',
            body: JSON.stringify({
                title,
                lang: state.currentResults.lang,
                text: state.currentSourceText,
                is_public: isPublic,
            }),
        });
        if (!resp.ok) {
            throw new Error(await resp.text() || 'Failed to create deck');
        }
        const created = await resp.json();
        // Invalidate the official-deck cache so the new entry shows up
        // when the admin switches to the "Official decks" tab.
        if (isPublic)
            state.officialDecksLoaded = false;
        await refreshDashboardData();
        showToast(isPublic ? `Official deck saved (#${created.deck_id}).` : `Deck saved (#${created.deck_id}).`, 'success');
        navigate('/decks');
    }
    catch (err) {
        showToast(err.message || 'Failed to save deck.', 'error');
    }
    finally {
        submitBtn.disabled = false;
        submitBtn.textContent = originalLabel;
    }
}
function renderReviewPage() {
    const filter = document.getElementById('review-deck-filter');
    if (filter) {
        const current = state.reviewDeckFilter;
        const decks = state.decks.filter(d => d.lang === state.activeLanguage);
        filter.innerHTML = `<option value="">All decks</option>` + decks.map(deck => `<option value="${deck.id}">${escapeHtml(deck.title)}</option>`).join('');
        // If the previous filter pointed at a deck in the other language, reset
        // it so the dropdown's visible value matches state.
        const stillValid = current === '' || decks.some(d => String(d.id) === current);
        if (!stillValid) {
            state.reviewDeckFilter = '';
            filter.value = '';
        }
        else {
            filter.value = current;
        }
    }
    renderCurrentReviewCard();
}
function renderCurrentReviewCard() {
    const cardEl = document.getElementById('review-card');
    const emptyEl = document.getElementById('review-empty');
    const deckCountsEl = document.getElementById('review-card-decks');
    const exampleEl = document.getElementById('review-card-example');
    const frontTextEl = document.getElementById('review-card-front-text');
    const lemmaEl = document.getElementById('review-card-lemma');
    const meaningEl = document.getElementById('review-card-meaning');
    const modeEl = document.getElementById('review-card-mode');
    if (!cardEl || !emptyEl || !deckCountsEl || !exampleEl || !frontTextEl || !lemmaEl || !meaningEl || !modeEl)
        return;
    const card = state.currentReviewCard;
    const hasCard = Boolean(card);
    cardEl.classList.toggle('hidden', !hasCard);
    emptyEl.classList.toggle('hidden', hasCard);
    if (!card)
        return;
    modeEl.textContent = card.mode === 'sentence' ? 'Sentence card' : 'Word card';
    frontTextEl.textContent = card.front.text || card.back.lemma;
    lemmaEl.textContent = card.back.lemma;
    meaningEl.textContent = card.back.meaning || 'No gloss yet';
    deckCountsEl.innerHTML = card.deck_counts.map(pair => `<span class="review-deck-pill">${escapeHtml(pair[0])} · ${escapeHtml(pair[1])}</span>`).join('');
    const example = card.back.examples?.[0];
    if (example) {
        exampleEl.classList.remove('hidden');
        exampleEl.innerHTML = `<strong>${escapeHtml(example.source_deck || 'Example')}</strong><br>${escapeHtml(example.text)}`;
    }
    else {
        exampleEl.classList.add('hidden');
        exampleEl.innerHTML = '';
    }
}
async function loadNextReviewCard(showEmptyToast) {
    if (state.role === 'anon')
        return;
    const params = new URLSearchParams({ lang: state.activeLanguage });
    if (state.reviewDeckFilter)
        params.set('deck_id', state.reviewDeckFilter);
    try {
        const resp = await fetch(`/api/review/next?${params.toString()}`, { credentials: 'same-origin' });
        if (resp.status === 204) {
            state.currentReviewCard = null;
            renderCurrentReviewCard();
            if (showEmptyToast)
                showToast('Nothing due right now.', 'info');
            return;
        }
        if (!resp.ok) {
            throw new Error(await resp.text() || 'Failed to load review card');
        }
        state.currentReviewCard = await resp.json();
        renderCurrentReviewCard();
    }
    catch (err) {
        state.currentReviewCard = null;
        renderCurrentReviewCard();
        showToast(err.message || 'Failed to load review card.', 'error');
    }
}
async function submitReviewAnswer(rating) {
    const cardID = Number(state.currentReviewCard?.card_id || 0);
    if (!cardID)
        return;
    try {
        const resp = await fetch('/api/review/answer', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'same-origin',
            body: JSON.stringify({ card_id: cardID, rating }),
        });
        if (!resp.ok) {
            throw new Error(await resp.text() || 'Failed to record review answer');
        }
        await refreshDashboardData();
        await loadNextReviewCard(false);
    }
    catch (err) {
        showToast(err.message || 'Failed to record review answer.', 'error');
    }
}
async function mutateReviewCard(endpoint, successMessage) {
    const cardID = Number(state.currentReviewCard?.card_id || 0);
    if (!cardID)
        return;
    try {
        const resp = await fetch(endpoint, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'same-origin',
            body: JSON.stringify({ card_id: cardID }),
        });
        if (!resp.ok) {
            throw new Error(await resp.text() || 'Failed to update card');
        }
        await refreshDashboardData();
        showToast(successMessage, 'success');
        await loadNextReviewCard(false);
    }
    catch (err) {
        showToast(err.message || 'Failed to update card.', 'error');
    }
}
async function loadAdminUsers() {
    const tbody = document.getElementById('admin-users-tbody');
    const empty = document.getElementById('admin-users-empty');
    if (!tbody)
        return;
    tbody.innerHTML = '<tr><td colspan="3">Loading users…</td></tr>';
    if (empty)
        empty.classList.add('hidden');
    try {
        const resp = await fetch('/api/admin/users', { credentials: 'same-origin' });
        if (!resp.ok) {
            tbody.innerHTML = `<tr><td colspan="3">Failed to load users (${resp.status}).</td></tr>`;
            return;
        }
        const data = await resp.json();
        renderAdminUsers(data.users || []);
    }
    catch (err) {
        tbody.innerHTML = '<tr><td colspan="3">Failed to load users.</td></tr>';
    }
}
function renderAdminUsers(users) {
    const tbody = document.getElementById('admin-users-tbody');
    const empty = document.getElementById('admin-users-empty');
    if (!tbody)
        return;
    if (users.length === 0) {
        tbody.innerHTML = '';
        if (empty)
            empty.classList.remove('hidden');
        return;
    }
    if (empty)
        empty.classList.add('hidden');
    const currentUserId = state.user?.id;
    tbody.innerHTML = users.map(u => {
        const isSelf = u.id === currentUserId;
        const checked = u.is_admin ? 'checked' : '';
        const disabled = isSelf && u.is_admin ? 'disabled' : '';
        const title = isSelf && u.is_admin ? 'You cannot remove your own admin access' : '';
        return `<tr data-user-id="${u.id}">
            <td>${u.id}</td>
            <td>${escapeHtml(u.email)}</td>
            <td>
                <label class="admin-users-toggle">
                    <input type="checkbox" class="admin-user-admin-toggle" data-user-id="${u.id}" ${checked} ${disabled} ${title ? `title="${title}"` : ''}>
                    <span>Admin</span>
                </label>
            </td>
        </tr>`;
    }).join('');
    tbody.querySelectorAll('.admin-user-admin-toggle').forEach(input => {
        input.addEventListener('change', async () => {
            const userIdStr = input.dataset.userId;
            if (!userIdStr)
                return;
            const userId = parseInt(userIdStr, 10);
            const want = input.checked;
            input.disabled = true;
            try {
                const resp = await fetch(`/api/admin/users?id=${userId}`, {
                    method: 'PATCH',
                    headers: { 'Content-Type': 'application/json' },
                    credentials: 'same-origin',
                    body: JSON.stringify({ is_admin: want }),
                });
                if (!resp.ok) {
                    const msg = (await resp.text()).trim() || 'Update failed';
                    showToast(msg, 'error');
                    input.checked = !want;
                    return;
                }
                showToast(want ? 'Granted admin' : 'Removed admin', 'success');
            }
            catch {
                input.checked = !want;
                showToast('Update failed', 'error');
            }
            finally {
                input.disabled = false;
            }
        });
    });
}
// ── Admin feedback queue ───────────────────────────────────────────────────
function feedbackStatusLabel(status) {
    switch (status) {
        case 'submitted': return 'Submitted';
        case 'accepted': return 'Accepted';
        case 'rejected': return 'Rejected';
        case 'needs_follow_up': return 'Needs follow-up';
        default: return status || 'All';
    }
}
function formatFeedbackDate(value) {
    const date = new Date(value);
    if (Number.isNaN(date.getTime()))
        return value;
    return date.toLocaleString(undefined, {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
    });
}
function renderAdminFeedbackPage() {
    const filter = document.getElementById('admin-feedback-status');
    if (filter)
        filter.value = state.adminFeedbackStatus;
    renderAdminFeedbackList();
}
function renderAdminFeedbackList() {
    const list = document.getElementById('admin-feedback-list');
    const empty = document.getElementById('admin-feedback-empty');
    if (!list || !empty)
        return;
    const feedback = state.adminFeedback;
    empty.classList.toggle('hidden', feedback.length > 0);
    list.classList.toggle('hidden', feedback.length === 0);
    if (feedback.length === 0) {
        list.innerHTML = '';
        return;
    }
    list.innerHTML = feedback.map(item => {
        const original = [item.original_lemma, item.original_pos].filter(Boolean).join(' / ') || 'Not captured';
        const proposed = [item.proposed_lemma, item.proposed_pos].filter(Boolean).join(' / ');
        const originalGrammar = item.original_grammar_label ? ` · ${escapeHtml(item.original_grammar_label)}` : '';
        const proposedGrammar = item.proposed_grammar_label ? ` · ${escapeHtml(item.proposed_grammar_label)}` : '';
        const note = item.note ? `<p class="admin-feedback-note">${escapeHtml(item.note)}</p>` : '';
        const reviewNote = item.review_note ? `<p class="admin-feedback-review-note">${escapeHtml(item.review_note)}</p>` : '';
        return `<article class="admin-feedback-item" data-feedback-id="${item.id}">
            <header class="admin-feedback-item-header">
                <div>
                    <h2>${escapeHtml(item.surface)}</h2>
                    <p class="admin-feedback-meta">${escapeHtml(item.lang)} · ${escapeHtml(item.parser)} · session #${item.parse_session_id} · user #${item.user_id} · ${formatFeedbackDate(item.created_at)}</p>
                </div>
                <span class="admin-feedback-status">${feedbackStatusLabel(item.status)}</span>
            </header>
            <div class="admin-feedback-comparison">
                <div>
                    <span class="admin-feedback-label">Original</span>
                    <p>${escapeHtml(original)}${originalGrammar}</p>
                </div>
                <div>
                    <span class="admin-feedback-label">Proposed</span>
                    <p>${escapeHtml(proposed)}${proposedGrammar}</p>
                </div>
            </div>
            ${note}
            ${reviewNote}
            <div class="admin-feedback-actions">
                <input type="text" class="admin-feedback-review-input" data-review-note="${item.id}" placeholder="Review note (optional)" value="${escapeAttr(item.review_note || '')}">
                <button type="button" class="btn btn-primary btn-sm" data-feedback-action="accepted" data-feedback-id="${item.id}">Accept</button>
                <button type="button" class="btn btn-outline btn-sm" data-feedback-action="rejected" data-feedback-id="${item.id}">Reject</button>
                <button type="button" class="btn btn-outline btn-sm" data-feedback-action="needs_follow_up" data-feedback-id="${item.id}">Follow up</button>
            </div>
        </article>`;
    }).join('');
}
async function loadAdminFeedback() {
    if (state.role !== 'admin')
        return;
    const statusParam = state.adminFeedbackStatus ? `?status=${encodeURIComponent(state.adminFeedbackStatus)}` : '';
    try {
        const resp = await fetch(`/api/admin/parse-feedback${statusParam}`, { credentials: 'same-origin' });
        if (!resp.ok) {
            throw new Error(await resp.text() || 'Failed to load parser feedback');
        }
        const data = await resp.json();
        state.adminFeedback = data.feedback || [];
        renderAdminFeedbackList();
    }
    catch (err) {
        state.adminFeedback = [];
        renderAdminFeedbackList();
        showToast(err.message || 'Failed to load parser feedback.', 'error');
    }
}
async function reviewAdminFeedback(feedbackID, status) {
    const noteInput = document.querySelector(`[data-review-note="${feedbackID}"]`);
    const reviewNote = noteInput?.value.trim() || '';
    try {
        const resp = await fetch(`/api/admin/parse-feedback?id=${encodeURIComponent(String(feedbackID))}`, {
            method: 'PATCH',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'same-origin',
            body: JSON.stringify({ status, review_note: reviewNote }),
        });
        if (!resp.ok) {
            throw new Error(await resp.text() || 'Failed to review parser feedback');
        }
        showToast(`Feedback marked ${feedbackStatusLabel(status).toLowerCase()}.`, 'success');
        await loadAdminFeedback();
    }
    catch (err) {
        showToast(err.message || 'Failed to review parser feedback.', 'error');
    }
}
// ── Correction modal ───────────────────────────────────────────────────────
function openCorrectionModal(row) {
    const modal = document.getElementById('correction-modal');
    const lemmaEl = document.getElementById('correction-lemma');
    const proposedLemmaEl = document.getElementById('correction-proposed-lemma');
    const proposedPosEl = document.getElementById('correction-proposed-pos');
    const proposedGramEl = document.getElementById('correction-proposed-grammar');
    const noteEl = document.getElementById('correction-note');
    const submitBtn = document.getElementById('correction-submit');
    const authHint = document.getElementById('correction-auth-hint');
    if (!modal || !lemmaEl || !proposedLemmaEl || !proposedPosEl || !proposedGramEl || !noteEl || !submitBtn)
        return;
    state.currentRow = row;
    const grammarSuffix = row.original_grammar_label ? ` · ${row.original_grammar_label}` : '';
    lemmaEl.textContent = `${row.lemma} (${posLabel(row.pos)})${grammarSuffix}`;
    proposedLemmaEl.value = row.lemma;
    proposedPosEl.value = row.pos in POS_LABELS ? row.pos : 'X';
    proposedGramEl.value = row.original_grammar_label;
    noteEl.value = '';
    // Backend requires authentication. Anonymous parses can't submit
    // corrections — the feedback endpoint creates the parse_session
    // lazily but it still has to belong to a user. Surface that
    // explicitly via the auth hint.
    const canSubmit = state.role === 'user' || state.role === 'admin';
    submitBtn.disabled = !canSubmit;
    if (authHint)
        authHint.classList.toggle('hidden', canSubmit);
    modal.classList.remove('hidden');
    proposedLemmaEl.focus();
}
function closeCorrectionModal() {
    document.getElementById('correction-modal')?.classList.add('hidden');
}
function initCorrectionModal() {
    document.getElementById('correction-modal-close')?.addEventListener('click', closeCorrectionModal);
    document.getElementById('correction-modal-backdrop')?.addEventListener('click', closeCorrectionModal);
    document.getElementById('correction-cancel')?.addEventListener('click', closeCorrectionModal);
    const form = document.getElementById('correction-form');
    form?.addEventListener('submit', async (e) => {
        e.preventDefault();
        const submitBtn = document.getElementById('correction-submit');
        submitBtn.disabled = true;
        const orig = submitBtn.textContent || '';
        submitBtn.textContent = 'Sending…';
        try {
            const row = state.currentRow;
            const results = state.currentResults;
            if (!row || !results) {
                showToast("Can't send correction — no parse loaded.", 'error');
                return;
            }
            const proposedLemma = document.getElementById('correction-proposed-lemma').value.trim();
            const proposedPos = document.getElementById('correction-proposed-pos').value;
            const proposedGram = document.getElementById('correction-proposed-grammar').value.trim();
            const note = document.getElementById('correction-note').value.trim();
            if (!proposedLemma || !proposedPos) {
                showToast('Please fill in both base form and part of speech.', 'error');
                return;
            }
            // Two attribution paths: deck-detail feedback has a persisted
            // parse_session (results.parse_id); Inspect-view feedback ships
            // the source text inline and the server creates a session lazily.
            const body = {
                lang: results.lang,
                parser: state.currentParserMode,
                surface: row.surface,
                occurrence: row.occurrence,
                original_lemma: row.lemma,
                original_pos: row.pos,
                original_grammar_label: row.original_grammar_label,
                proposed_lemma: proposedLemma,
                proposed_pos: proposedPos,
                proposed_grammar_label: proposedGram,
                note,
            };
            if (typeof results.parse_id === 'number') {
                body.parse_id = results.parse_id;
            }
            else {
                body.source_text = state.currentSourceText;
                body.total_tokens = results.total_tokens;
                body.unique_lemma_count = results.words.length;
            }
            const resp = await fetch('/api/parse/feedback', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                credentials: 'same-origin',
                body: JSON.stringify(body),
            });
            if (resp.ok) {
                showToast('Thanks — correction sent.', 'success');
                closeCorrectionModal();
            }
            else {
                showToast("Couldn't send correction — please try again later.", 'error');
            }
        }
        catch {
            showToast("Couldn't send correction — check your connection and try again.", 'error');
        }
        finally {
            submitBtn.disabled = false;
            submitBtn.textContent = orig;
        }
    });
}
function initDecksPage() {
    document.querySelectorAll('[data-decks-tab]').forEach(btn => {
        btn.addEventListener('click', () => {
            const tab = btn.getAttribute('data-decks-tab');
            if (!tab)
                return;
            // Navigate via the hash router so the URL reflects the active
            // tab and a refresh restores it. The route handler in
            // renderRoute() loads the catalog as needed.
            navigate(routeForDecksTab(tab));
        });
    });
    const handleDeckClick = (e) => { void handleDeckListClick(e); };
    document.getElementById('decks-list')?.addEventListener('click', handleDeckClick);
    document.getElementById('official-decks-list')?.addEventListener('click', handleDeckClick);
}
async function handleDeckListClick(e) {
    const target = e.target;
    if (!target)
        return;
    const reviewDeckID = target.getAttribute('data-open-review');
    if (reviewDeckID) {
        state.reviewDeckFilter = reviewDeckID;
        navigate('/review');
        return;
    }
    const renameDeckID = target.getAttribute('data-rename-deck');
    if (renameDeckID) {
        const deckID = Number(renameDeckID);
        const deck = getDeckByID(deckID);
        if (!deck)
            return;
        const title = (await showPrompt({
            title: 'Rename deck',
            message: `Pick a new title for "${deck.title}".`,
            label: 'New title',
            initialValue: deck.title,
            confirmLabel: 'Rename',
        }))?.trim();
        if (!title || title === deck.title)
            return;
        try {
            const resp = await fetch(`/api/decks/${deckID}`, {
                method: 'PATCH',
                headers: { 'Content-Type': 'application/json' },
                credentials: 'same-origin',
                body: JSON.stringify({ title }),
            });
            if (!resp.ok)
                throw new Error(await resp.text() || 'Rename failed');
            await refreshDashboardData();
            showToast('Deck renamed.', 'success');
        }
        catch (err) {
            showToast(err.message || 'Rename failed.', 'error');
        }
        return;
    }
    const togglePublicDeckID = target.getAttribute('data-toggle-public');
    if (togglePublicDeckID) {
        const deckID = Number(togglePublicDeckID);
        const deck = getDeckByID(deckID);
        if (!deck)
            return;
        const nextPublic = target.getAttribute('data-current-public') !== '1';
        const action = nextPublic ? 'Publish' : 'Unpublish';
        const confirmed = await showConfirm({
            title: nextPublic ? 'Publish as official deck?' : 'Unpublish official deck?',
            message: nextPublic
                ? `"${deck.title}" will be visible to every user under Official decks.`
                : `"${deck.title}" will be removed from Official decks. Users who already added it keep their progress.`,
            confirmLabel: action,
            danger: !nextPublic,
        });
        if (!confirmed)
            return;
        const button = target;
        button.disabled = true;
        try {
            const resp = await fetch(`/api/decks/${deckID}`, {
                method: 'PATCH',
                headers: { 'Content-Type': 'application/json' },
                credentials: 'same-origin',
                body: JSON.stringify({ is_public: nextPublic }),
            });
            if (!resp.ok)
                throw new Error(await resp.text() || `${action} failed`);
            // Catalog membership changes — bust the cache so the Official
            // tab reflects the new state on next view.
            state.officialDecksLoaded = false;
            await refreshDashboardData();
            showToast(nextPublic ? 'Deck published as official.' : 'Deck unpublished.', 'success');
        }
        catch (err) {
            showToast(err.message || `${action} failed.`, 'error');
            button.disabled = false;
        }
        return;
    }
    const deleteDeckID = target.getAttribute('data-delete-deck');
    if (deleteDeckID) {
        const deckID = Number(deleteDeckID);
        const deck = getDeckByID(deckID);
        if (!deck)
            return;
        const confirmed = await showConfirm({
            title: 'Delete deck?',
            message: `Delete "${deck.title}"? This removes the deck text but keeps global learning state.`,
            confirmLabel: 'Delete',
            danger: true,
        });
        if (!confirmed)
            return;
        try {
            const resp = await fetch(`/api/decks/${deckID}`, {
                method: 'DELETE',
                credentials: 'same-origin',
            });
            if (!resp.ok)
                throw new Error(await resp.text() || 'Delete failed');
            await refreshDashboardData();
            showToast('Deck deleted.', 'success');
        }
        catch (err) {
            showToast(err.message || 'Delete failed.', 'error');
        }
        return;
    }
    const subscribeDeckID = target.getAttribute('data-subscribe-deck');
    if (subscribeDeckID) {
        const deckID = Number(subscribeDeckID);
        const button = target;
        button.disabled = true;
        try {
            const resp = await fetch(`/api/decks/${deckID}/subscribe`, {
                method: 'POST',
                credentials: 'same-origin',
            });
            if (!resp.ok)
                throw new Error(await resp.text() || 'Failed to add deck');
            await Promise.all([loadOfficialDecks(), refreshDashboardData({ rerenderRoute: false })]);
            renderDecksPage();
            showToast('Added to your studying list.', 'success');
        }
        catch (err) {
            showToast(err.message || 'Failed to add deck.', 'error');
            button.disabled = false;
        }
        return;
    }
    const unsubscribeDeckID = target.getAttribute('data-unsubscribe-deck');
    if (unsubscribeDeckID) {
        const deckID = Number(unsubscribeDeckID);
        const deck = getDeckByID(deckID);
        if (!deck)
            return;
        const confirmed = await showConfirm({
            title: 'Remove from studying list?',
            message: `Remove "${deck.title}" from your studying list? Cards you've already reviewed stay in your history.`,
            confirmLabel: 'Remove',
            danger: true,
        });
        if (!confirmed)
            return;
        const button = target;
        button.disabled = true;
        try {
            const resp = await fetch(`/api/decks/${deckID}/subscribe`, {
                method: 'DELETE',
                credentials: 'same-origin',
            });
            if (!resp.ok)
                throw new Error(await resp.text() || 'Failed to remove deck');
            await Promise.all([loadOfficialDecks(), refreshDashboardData({ rerenderRoute: false })]);
            renderDecksPage();
            showToast('Removed from your studying list.', 'success');
        }
        catch (err) {
            showToast(err.message || 'Failed to remove deck.', 'error');
            button.disabled = false;
        }
        return;
    }
}
function initKnownWordsPanel() {
    const form = document.getElementById('known-words-form');
    form?.addEventListener('submit', async (e) => {
        e.preventDefault();
        await importKnownWords();
    });
    document.getElementById('known-words-list')?.addEventListener('click', (e) => {
        const target = e.target;
        if (!target)
            return;
        const lemma = target.getAttribute('data-known-lemma');
        const pos = target.getAttribute('data-known-pos');
        if (lemma && pos)
            void deleteKnownWord(lemma, pos);
    });
}
function initVocabFileImport() {
    const input = document.getElementById('vocab-file-input');
    const cta = document.getElementById('vocab-file-cta');
    if (!input || !cta)
        return;
    input.addEventListener('change', async () => {
        const file = input.files?.[0];
        if (!file)
            return;
        await importKnownWordsFromFile(file);
        // Allow re-selecting the same file later.
        input.value = '';
    });
    // Drag-and-drop directly on the CTA tile.
    cta.addEventListener('dragover', (e) => {
        e.preventDefault();
        cta.classList.add('is-drag');
    });
    cta.addEventListener('dragleave', () => cta.classList.remove('is-drag'));
    cta.addEventListener('drop', async (e) => {
        e.preventDefault();
        cta.classList.remove('is-drag');
        const file = e.dataTransfer?.files?.[0];
        if (file)
            await importKnownWordsFromFile(file);
    });
}
function initVocabAnkiImport() {
    document.getElementById('vocab-anki-connect')?.addEventListener('click', () => {
        void connectToAnki();
    });
    document.getElementById('vocab-anki-deck')?.addEventListener('change', () => {
        void refreshAnkiFieldsForSelectedDeck();
    });
    document.getElementById('vocab-anki-import')?.addEventListener('click', () => {
        void importKnownWordsFromAnki();
    });
    document.getElementById('vocab-anki-reset')?.addEventListener('click', disconnectFromAnki);
}
function initReviewPage() {
    const filter = document.getElementById('review-deck-filter');
    filter?.addEventListener('change', async () => {
        state.reviewDeckFilter = filter.value;
        await loadNextReviewCard(false);
    });
    document.querySelectorAll('[data-review-answer]').forEach(btn => {
        btn.addEventListener('click', () => {
            const rating = btn.getAttribute('data-review-answer');
            if (rating)
                void submitReviewAnswer(rating);
        });
    });
    document.getElementById('review-mark-known')?.addEventListener('click', () => {
        void mutateReviewCard('/api/card/known', 'Word marked known.');
    });
    document.getElementById('review-mark-ignored')?.addEventListener('click', () => {
        void mutateReviewCard('/api/card/ignore', 'Word ignored.');
    });
    document.getElementById('review-skip')?.addEventListener('click', () => {
        void loadNextReviewCard(true);
    });
}
function initResultsSaveForm() {
    const toggleBtn = document.getElementById('results-save-toggle');
    const form = document.getElementById('results-save-form');
    const cancelBtn = document.getElementById('results-save-cancel');
    const htmlForm = form;
    toggleBtn?.addEventListener('click', () => {
        form?.classList.toggle('hidden');
        const input = document.getElementById('results-deck-title');
        const publicCheckbox = document.getElementById('results-deck-public');
        if (form && !form.classList.contains('hidden')) {
            input?.focus();
            if (publicCheckbox)
                publicCheckbox.checked = false;
        }
    });
    cancelBtn?.addEventListener('click', () => {
        form?.classList.add('hidden');
    });
    htmlForm?.addEventListener('submit', async (e) => {
        e.preventDefault();
        await saveCurrentResultsAsDeck();
    });
}
function initAdminFeedbackPage() {
    const filter = document.getElementById('admin-feedback-status');
    filter?.addEventListener('change', async () => {
        state.adminFeedbackStatus = filter.value;
        await loadAdminFeedback();
    });
    document.getElementById('admin-feedback-list')?.addEventListener('click', (e) => {
        const target = e.target;
        if (!target)
            return;
        const action = target.getAttribute('data-feedback-action');
        const id = Number(target.getAttribute('data-feedback-id') || 0);
        if (action && id > 0)
            void reviewAdminFeedback(id, action);
    });
}
// Portal-style tooltip — pseudo-element ::after tooltips get clipped by
// ancestor overflow (e.g. .word-table { overflow: hidden }), so we render a
// single body-level element that's positioned via getBoundingClientRect and
// can escape any ancestor stacking/clipping context.
let portalTip = null;
function ensurePortalTip() {
    if (portalTip)
        return portalTip;
    portalTip = document.createElement('div');
    portalTip.className = 'portal-tooltip';
    portalTip.setAttribute('role', 'tooltip');
    document.body.appendChild(portalTip);
    return portalTip;
}
// Single-line plain `data-tooltip` and multi-line `data-tooltip-snippet` both
// route through this. Snippet wins when both are present.
const TOOLTIP_SELECTOR = '[data-tooltip],[data-tooltip-snippet]';
function showPortalTooltip(target) {
    const snippet = target.getAttribute('data-tooltip-snippet');
    const el = ensurePortalTip();
    const rect = target.getBoundingClientRect();
    if (snippet) {
        el.classList.add('rich');
        // textContent + CSS white-space: pre-line preserves the \n separator
        // without us hand-rolling HTML sanitization.
        el.textContent = snippet;
        // Left-aligned: tooltip's left edge sits at the trigger's left edge.
        el.style.left = `${rect.left}px`;
    }
    else {
        const text = target.getAttribute('data-tooltip');
        if (!text)
            return;
        el.classList.remove('rich');
        el.textContent = text;
        // Plain variant: horizontally centered over the trigger.
        el.style.left = `${rect.left + rect.width / 2}px`;
    }
    el.style.top = `${rect.top - 6}px`;
    el.classList.add('visible');
}
function hidePortalTooltip() {
    if (portalTip)
        portalTip.classList.remove('visible');
}
function initPortalTooltips() {
    // mouseover: show on trigger, hide on anything else. The "hide on
    // non-trigger" branch matters when the table re-renders (sort change or
    // lemma-state click): the trigger element is removed from the DOM
    // without firing mouseout, so without this hide() the tooltip stays
    // stuck. Combined with the explicit hidePortalTooltip() call from
    // renderResultsTable for the no-mouse-move case.
    document.addEventListener('mouseover', (e) => {
        const t = e.target?.closest(TOOLTIP_SELECTOR);
        if (t)
            showPortalTooltip(t);
        else
            hidePortalTooltip();
    });
    document.addEventListener('mouseout', (e) => {
        const t = e.target?.closest(TOOLTIP_SELECTOR);
        if (t)
            hidePortalTooltip();
    });
    document.addEventListener('focusin', (e) => {
        const t = e.target?.closest(TOOLTIP_SELECTOR);
        if (t)
            showPortalTooltip(t);
    });
    document.addEventListener('focusout', (e) => {
        const t = e.target?.closest(TOOLTIP_SELECTOR);
        if (t)
            hidePortalTooltip();
    });
    window.addEventListener('scroll', hidePortalTooltip, true);
}
// ── Init ───────────────────────────────────────────────────────────────────
document.addEventListener('DOMContentLoaded', async () => {
    initTheme();
    initMobileNav();
    initSigninForm();
    initInspectForm();
    initWorkbenchForm();
    preventStrayFileDrops();
    initCorrectionModal();
    initDecksPage();
    initKnownWordsPanel();
    initLanguagesPage();
    initNavLanguageSelector();
    initVocabFileImport();
    initVocabAnkiImport();
    initReviewPage();
    initResultsSaveForm();
    initAdminFeedbackPage();
    initPortalTooltips();
    document.getElementById('theme-toggle')?.addEventListener('click', toggleTheme);
    document.getElementById('nav-signout')?.addEventListener('click', handleSignout);
    document.getElementById('nav-mobile-signout')?.addEventListener('click', handleSignout);
    document.getElementById('results-back')?.addEventListener('click', () => {
        if (state.currentContext === 'deck') {
            navigate('/decks');
            return;
        }
        navigate(state.currentContext === 'workbench' ? '/admin/workbench' : '/inspect');
    });
    document.querySelectorAll('.sort-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            if (!state.currentResults)
                return;
            const key = btn.dataset.sort;
            if (!key)
                return;
            state.currentSort = state.currentSort.key === key
                ? { key, dir: state.currentSort.dir === 'asc' ? 'desc' : 'asc' }
                : { key, dir: key === 'tokens' ? 'desc' : 'asc' };
            renderResultsTable(state.currentResults);
        });
    });
    document.querySelectorAll('.pos-filter-chip').forEach(btn => {
        btn.addEventListener('click', () => {
            if (!state.currentResults)
                return;
            const filter = btn.dataset.filter;
            if (!filter)
                return;
            state.currentPOSFilter = filter;
            renderResultsTable(state.currentResults);
        });
    });
    updateSortButtons();
    updatePOSFilterButtons();
    // Resolve auth state, then route.
    await fetchMe();
    renderRoute();
    window.addEventListener('hashchange', renderRoute);
});
