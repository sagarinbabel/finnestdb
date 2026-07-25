"use strict";
// FinnEst - frontend with three role-aware surfaces:
//   anonymous landing/about/signin, authenticated user product, admin workbench.
const MAX_CHARS = 1500000;
// Client-side fallback for the anonymous /api/parse text-size cap. The server
// is authoritative (FINNESTDB_ANON_MAX_CHARS, default 300,000) and surfaces the
// live value on /api/me → state.anonMaxChars; this default only applies if that
// response is missing the field (e.g. a stale server).
const DEFAULT_ANON_MAX_CHARS = 300000;
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
// Inline SVG flags - flat, cartoony, system-font-independent. Returned as a
// string and injected via innerHTML next to language labels. Each flag uses
// `class="lang-flag"`; CSS sizes them via the parent's font-size (height: 1em)
// and adds the rounded-corner / soft-shadow treatment that gives them a
// sticker-like look. Pass `tooltip` to attach data-tooltip directly to the
// SVG element so the portal tooltip fires only over actual flag pixels -
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
    // Multiple-possible-meanings state for the current parse results.
    // ambiguityBySurface: candidate metadata keyed by surface form.
    ambiguityBySurface: new Map(),
    // ambiguityExpanded: surfaces whose meaning-check panel is open.
    ambiguityExpanded: new Set(),
    // selectedSenses: senses the learner explicitly chose to "Study" (or left
    // as "Not sure"). Keyed by `${surface}\x00${lemma}\x00${pos}`. These are
    // threaded into the deck save so an FST-only sense still creates its card.
    selectedSenses: new Set(),
    // ambiguityKnownPending: per-candidate in-flight guard for "I know this
    // meaning" so a double-click can't double-record.
    ambiguityKnownPending: new Set(),
    // ── Reading surface (Read / Words tabs) ──
    // resultsTab: which results view is showing. Read = the living text,
    // Words = the existing lemma table. Persisted in localStorage; defaults to
    // 'read'. In deck context there's no source text, so the Words table shows
    // regardless of this and the tab bar is hidden.
    resultsTab: 'read',
    // formIndex: surface form (exact, case-preserved) → the WordEntry rows it
    // resolves to, for the current parse. A homograph surface maps to >1 row.
    // Rebuilt in showResults; drives Read-view token coloring + popover content.
    formIndex: new Map(),
    // readPopoverSurface: the surface currently shown in the reading popover
    // (empty when closed). One popover at a time.
    readPopoverSurface: '',
    currentReviewCard: null,
    reviewDeckFilter: '',
    parseSessions: [],
    parseSessionsLoaded: false,
    adminFeedback: [],
    adminFeedbackStatus: 'submitted',
    adminFeedbackFlagOnly: '',
    adminIssues: [],
    adminIssuesStatus: '',
    // Active language drives the deck list filter, Inspect/Known-Words defaults,
    // and Review queue. learningLanguages is the user's opt-in set; the nav
    // dropdown only shows entries from this list. Both are hydrated from
    // /api/me; defaults are FI active, both languages learning.
    learningLanguages: ['FI', 'ET'],
    activeLanguage: 'FI',
    languageStats: {},
    // Anonymous /api/parse text-size cap, hydrated from /api/me. Drives the
    // landing char counter and client-side pre-submit guard.
    anonMaxChars: DEFAULT_ANON_MAX_CHARS,
    // Sign-up ribbon on anonymous results is dismissible per session, but
    // reappears on the next parse (USER_FLOWS §2). Tracked in memory (reset on
    // reload) rather than sessionStorage so "next parse" always re-shows it.
    anonRibbonDismissed: false,
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
    // Curated Embedded Text catalog, hydrated lazily from /api/catalog the
    // first time a cold-start empty state renders. null = not yet loaded.
    catalog: null,
};
const NOUN_POS = ['NOUN', 'PROPN'];
const VERB_POS = ['VERB', 'AUX'];
const ADJ_POS = ['ADJ'];
const ADV_POS = ['ADV'];
const OTHER_POS = ['PRON', 'DET', 'ADP', 'NUM', 'CCONJ', 'SCONJ', 'PART', 'INTJ', 'X', 'SYM', 'PUNCT'];
const THEME_MODE_KEY = 'theme';
const THEME_SKIN_KEY = 'skin';
function readThemeSkin() {
    // Default flips to 'aalto' when nothing is saved; an explicit 'ink' still wins.
    return localStorage.getItem(THEME_SKIN_KEY) === 'ink' ? 'ink' : 'aalto';
}
function readThemeMode() {
    const saved = localStorage.getItem(THEME_MODE_KEY);
    if (saved === 'light')
        return 'light';
    if (saved === 'dark')
        return 'dark';
    // No saved mode: the Aalto default lands on Paimio light; anyone who saved
    // a skin but somehow no mode keeps the prior 'dark' fallback.
    return localStorage.getItem(THEME_SKIN_KEY) ? 'dark' : 'light';
}
function initTheme() {
    applyTheme(readThemeSkin(), readThemeMode());
}
function applyTheme(skin, mode) {
    const root = document.documentElement;
    root.setAttribute('data-skin', skin);
    root.setAttribute('data-theme', mode);
    // Trigger icon mirrors the active mode (a sun in dark mode invites switching
    // to light, and vice versa - matching the prior single-toggle affordance).
    document.querySelectorAll('.theme-icon').forEach(el => {
        el.textContent = mode === 'light' ? '🌙' : '☀️';
    });
    updateThemeMenuSelection(skin, mode);
}
function setTheme(skin, mode) {
    localStorage.setItem(THEME_SKIN_KEY, skin);
    localStorage.setItem(THEME_MODE_KEY, mode);
    applyTheme(skin, mode);
}
function updateThemeMenuSelection(skin, mode) {
    document.querySelectorAll('.theme-option').forEach(btn => {
        const isActive = btn.dataset.skin === skin && btn.dataset.mode === mode;
        btn.setAttribute('aria-selected', isActive ? 'true' : 'false');
        btn.classList.toggle('selected', isActive);
    });
}
function isThemeMenuOpen() {
    const menu = document.getElementById('theme-menu');
    return !!menu && !menu.classList.contains('hidden');
}
function openThemeMenu() {
    const toggle = document.getElementById('theme-toggle');
    const menu = document.getElementById('theme-menu');
    if (!toggle || !menu)
        return;
    menu.classList.remove('hidden');
    toggle.setAttribute('aria-expanded', 'true');
}
function closeThemeMenu() {
    const toggle = document.getElementById('theme-toggle');
    const menu = document.getElementById('theme-menu');
    if (!toggle || !menu)
        return;
    menu.classList.add('hidden');
    toggle.setAttribute('aria-expanded', 'false');
}
function toggleThemeMenu() {
    if (isThemeMenuOpen()) {
        closeThemeMenu();
    }
    else {
        openThemeMenu();
    }
}
function initThemePicker() {
    const toggle = document.getElementById('theme-toggle');
    const menu = document.getElementById('theme-menu');
    if (!toggle || !menu)
        return;
    toggle.addEventListener('click', (e) => {
        e.stopPropagation();
        toggleThemeMenu();
    });
    menu.querySelectorAll('.theme-option').forEach(btn => {
        btn.addEventListener('click', () => {
            const skin = btn.dataset.skin === 'aalto' ? 'aalto' : 'ink';
            const mode = btn.dataset.mode === 'light' ? 'light' : 'dark';
            setTheme(skin, mode);
            closeThemeMenu();
        });
    });
    // Dismiss on outside click or Escape, matching the language dropdown.
    document.addEventListener('click', (e) => {
        if (!isThemeMenuOpen())
            return;
        const target = e.target;
        if (!toggle.contains(target) && !menu.contains(target)) {
            closeThemeMenu();
        }
    });
    document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape' && isThemeMenuOpen()) {
            closeThemeMenu();
            toggle.focus();
        }
    });
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
// Without this, the first Promise would hang forever - listeners get torn
// down, but the resolve() function was never called.
let activeDialog = null;
// Last dialog's "Don't show this again" checkbox state - captured by
// openDialog when a rememberLabel is provided so showConfirmWithRemember can
// surface it alongside the confirm/cancel result.
let lastDialogRemember = false;
function openDialog(opts) {
    return new Promise(resolve => {
        const modal = document.getElementById('dialog-modal');
        const titleEl = document.getElementById('dialog-modal-title');
        const messageEl = document.getElementById('dialog-modal-message');
        const inputWrap = document.getElementById('dialog-modal-input-wrap');
        const inputLabel = document.getElementById('dialog-modal-input-label');
        const input = document.getElementById('dialog-modal-input');
        const rememberWrap = document.getElementById('dialog-modal-remember-wrap');
        const rememberInput = document.getElementById('dialog-modal-remember');
        const rememberLabel = document.getElementById('dialog-modal-remember-label');
        const backdrop = document.getElementById('dialog-modal-backdrop');
        const confirmBtn = document.getElementById('dialog-modal-confirm');
        const cancelBtn = document.getElementById('dialog-modal-cancel');
        if (!modal || !titleEl || !messageEl || !inputWrap || !input || !confirmBtn || !cancelBtn || !backdrop) {
            // Markup missing - fall back to native dialogs so the user never
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
        // Optional "Don't show this again"-style checkbox. Hidden by default
        // - only callers passing rememberLabel see it, and only that caller
        // reads lastDialogRemember afterwards.
        lastDialogRemember = false;
        if (rememberWrap && rememberInput) {
            if (opts.rememberLabel) {
                rememberWrap.classList.remove('hidden');
                rememberInput.checked = false;
                if (rememberLabel)
                    rememberLabel.textContent = opts.rememberLabel;
            }
            else {
                rememberWrap.classList.add('hidden');
            }
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
            // Snapshot the remember checkbox before tearing down. Only
            // meaningful when the dialog was opened with rememberLabel; other
            // callers ignore lastDialogRemember.
            if (opts.rememberLabel && rememberInput)
                lastDialogRemember = rememberInput.checked;
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
// Same as showConfirm but the result includes whether the user checked the
// "Don't show this again" box. Pass `rememberLabel` to customise the wording.
async function showConfirmWithRemember(opts) {
    const result = await openDialog({ ...opts, prompt: false });
    return { confirmed: result !== null, remember: lastDialogRemember };
}
// Confirm dialog whose Confirm button doubles as the loading indicator. The
// caller supplies a `classify` function that maps the resolved value to a
// success/error state. The button is disabled the moment the dialog opens
// and stays disabled until classify returns success - at which point it
// flips to its normal confirmLabel and re-enables. On failure the button
// label switches to the error text and stays disabled, so the user can only
// cancel out.
//
// Used by the Anki sync flow: the dialog opens immediately, the Confirm
// button reads "Checking Anki…" with an inline spinner, and only flips to
// "Sync and replace" once discovery comes back clean.
async function showConfirmWithStatus(opts, statusPromise, classify) {
    const confirmBtn = document.getElementById('dialog-modal-confirm');
    const finalLabel = opts.confirmLabel ?? 'OK';
    // Renders <spinner> + text inside the button. Used for both the loading
    // and error states (error skips the spinner). On success the button
    // resets to the caller's confirmLabel.
    const setButton = (state, text) => {
        if (!confirmBtn)
            return;
        if (state === 'success') {
            confirmBtn.textContent = text;
            confirmBtn.disabled = false;
        }
        else if (state === 'loading') {
            confirmBtn.innerHTML = '<span class="dialog-btn-spinner" aria-hidden="true"></span>';
            confirmBtn.append(text);
            confirmBtn.disabled = true;
        }
        else {
            confirmBtn.textContent = text;
            confirmBtn.disabled = true;
        }
    };
    // Important: kick off openDialog FIRST (its Promise body runs sync up to
    // `modal.classList.remove('hidden')`, which sets confirmBtn.textContent
    // and shows the dialog). Then override the button with our spinner. If
    // we called setButton before, openDialog's textContent assignment would
    // wipe the spinner and the dialog would appear with a plain disabled
    // button - looking unresponsive to the user.
    const dialogPromise = openDialog({ ...opts, prompt: false });
    setButton('loading', opts.loadingText);
    let resolvedValue;
    let succeeded = false;
    statusPromise.then((val) => {
        resolvedValue = val;
        const decision = classify(val);
        if (decision.state === 'success') {
            succeeded = true;
            setButton('success', finalLabel);
        }
        else {
            setButton('error', decision.text);
        }
    }, (err) => {
        const msg = err && err.message ? err.message : 'Failed.';
        setButton('error', msg);
    });
    const result = await dialogPromise;
    // Reset the button so the next openDialog caller doesn't inherit our
    // spinner / disabled state / temporary label.
    if (confirmBtn) {
        confirmBtn.innerHTML = '';
        confirmBtn.textContent = finalLabel;
        confirmBtn.disabled = false;
    }
    return {
        confirmed: result !== null,
        remember: lastDialogRemember,
        status: resolvedValue,
        succeeded,
    };
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
    if (typeof data.anon_max_chars === 'number' && data.anon_max_chars > 0) {
        state.anonMaxChars = data.anon_max_chars;
    }
    // Keep the landing char counter accurate once the real cap arrives.
    const landingEls = getLandingEls();
    if (landingEls)
        updateCharCount(landingEls);
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
// matches the rest of the app - native <select> popups can't be themed).
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
        // POST/DELETE bodies - re-render and re-fetch the per-language list.
        renderVocabPage();
        void loadKnownWords();
    }
    if (currentRoute() === '/review') {
        // Old card is for whatever language was active before - re-fetch.
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
// language belongs to. The user can't drop their last language - its
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
        // Best-effort - even if the endpoint is missing, clear local state.
    }
    resetClientSessionState();
    showToast('Signed out', 'info');
    navigate('/');
}
// Clears all signed-in client state and flips the UI back to the anonymous
// role. Shared by sign-out and account deletion - the server has already
// invalidated (or deleted) the session by the time this runs.
function resetClientSessionState() {
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
    state.parseSessions = [];
    state.parseSessionsLoaded = false;
    try {
        sessionStorage.removeItem(LAST_PARSE_KEY);
    }
    catch { }
    clearResultsDom();
    applyRoleVisibility();
}
// Account deletion (Languages page → Account section). DELETE /api/me
// cascades all user data server-side and clears the session cookie; the
// confirmation dialog mirrors the vocab delete-all pattern (the strongest
// destructive-confirm precedent in the app).
async function handleDeleteAccount() {
    const email = state.user?.email;
    const confirmed = await showConfirm({
        title: 'Delete your account?',
        message: `This permanently deletes ${email ? `the account ${email}` : 'your account'} and everything in it - all decks, review history, parse history, parser feedback, and known and ignored words. This cannot be undone.`,
        confirmLabel: 'Delete my account',
        danger: true,
    });
    if (!confirmed)
        return;
    const button = document.getElementById('account-delete');
    if (button)
        button.disabled = true;
    try {
        const resp = await fetch('/api/me', {
            method: 'DELETE',
            credentials: 'same-origin',
        });
        if (!resp.ok)
            throw new Error(await resp.text() || 'Failed to delete account');
        // Server has deleted the user and invalidated the session cookie -
        // drop all client state and land on the signed-out landing page.
        resetClientSessionState();
        showToast('Your account and data have been deleted.', 'info');
        navigate('/');
    }
    catch (err) {
        showToast(err.message || 'Failed to delete account.', 'error');
    }
    finally {
        if (button)
            button.disabled = false;
    }
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
    '/history': 'history-page',
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
    // Authenticated-only routes. /results is intentionally NOT here: anonymous
    // visitors reach it via the landing demo parse. Its signed-in-only chrome
    // (save-as-deck, known/ignore, feedback) is gated by data-role-show, and
    // deck detail (/decks/:id) below still requires sign-in.
    const userOnly = ['/dashboard', '/inspect', '/history', '/decks', '/decks/official', '/languages', '/vocab', '/review'];
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
    if (route === '/inspect')
        renderInspectColdStart();
    if (route === '/history') {
        renderHistoryPage();
        void loadParseSessions();
    }
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
        renderAdminIssuesPage();
        void loadAdminIssues();
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
                // Anonymous visitors can't reach /inspect (sign-in gated); send
                // them back to the landing parse form instead.
                window.location.hash = state.role === 'anon' ? '#/' : '#/inspect';
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
            lede.textContent = 'An account lets you save decks, upload whole EPUBs and books, track the words you already know, and review with spaced repetition. Pick an email and a password (8+ characters).';
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
            // Carry-forward (USER_FLOWS "Carry-forward of anonymous parses"):
            // if the visitor had a parse open before signing in, return them to
            // it with learner enrichments now applied - not the dashboard.
            await landAfterAuth();
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
        el.textContent = value === undefined ? '-' : value.toLocaleString();
    };
    setStat('stat-known', state.dashboard?.known_count);
    setStat('stat-due', state.dashboard?.due_count);
    setStat('stat-new-capacity', state.dashboard?.new_capacity_today);
    setStat('stat-cards-in-review', state.dashboard?.cards_in_review);
    setStat('stat-reviews-today', state.dashboard?.reviews_today);
    renderReviewActivityChart(state.dashboard?.review_activity || []);
    // Cold start: learners with no decks at all get the embedded-text catalog
    // (USER_FLOWS §4). Returning learners with decks don't need it on the
    // dashboard - Inspect still offers it.
    const noDecks = (state.dashboard?.decks || []).length === 0;
    renderCatalogSection('dashboard-cold-start', 'dashboard-catalog', noDecks);
    const decksList = document.getElementById('dashboard-decks-list');
    if (!decksList)
        return;
    const allDecks = state.dashboard?.decks || [];
    const decks = allDecks.filter(d => d.lang === state.activeLanguage);
    if (decks.length === 0) {
        const langLabel = languageName(state.activeLanguage);
        const hint = allDecks.length === 0
            ? `No decks yet - paste some text under <a href="#/inspect">Parse</a>, or add a <a href="#/decks/official">Top 1000 starter deck</a>.`
            : `No ${escapeHtml(langLabel)} decks yet. Switch the language in the top bar to see your other decks, or <a href="#/inspect">parse a ${escapeHtml(langLabel)} text</a>.`;
        decksList.innerHTML = `<p class="empty-state">${hint}</p>`;
        return;
    }
    decksList.innerHTML = decks.map(d => {
        const langName = d.lang === 'FI' ? 'Finnish' : d.lang === 'ET' ? 'Estonian' : escapeHtml(d.lang);
        const knownPct = d.unique > 0 ? Math.round((d.known / d.unique) * 100) : 0;
        const comprehensionPart = typeof d.comprehension_pct === 'number'
            ? ` · ${d.comprehension_pct}% comprehension`
            : '';
        return `<a href="#/decks" class="deck-card">
            <h4>${escapeHtml(d.title)}</h4>
            <p class="deck-meta">${langName} · ${d.known}/${d.unique} known (${knownPct}%) · ${d.due} due${comprehensionPart}</p>
        </a>`;
    }).join('');
}
// ── Embedded Text catalog (cold start) ─────────────────────────────────────
const GENRE_LABEL = {
    story: 'Story', article: 'Article', poem: 'Poem',
};
const DIFFICULTY_LABEL = {
    'easy': 'Easy', 'easy-medium': 'Easy\u2013Medium', 'medium': 'Medium',
    'medium-hard': 'Medium\u2013Hard', 'hard': 'Hard',
};
// renderCatalogSection toggles a cold-start section on/off and paints the
// catalog grid inside it. It lazy-loads /api/catalog on first show. Both the
// dashboard and Inspect empty states call this with their own container ids.
function renderCatalogSection(sectionId, gridId, show) {
    const section = document.getElementById(sectionId);
    if (!section)
        return;
    section.classList.toggle('hidden', !show);
    if (!show)
        return;
    const grid = document.getElementById(gridId);
    if (grid && !state.catalog) {
        grid.innerHTML = '<p class="empty-state">Loading texts…</p>';
    }
    void loadCatalog().then(() => paintCatalogGrid(gridId));
}
// loadCatalog fetches the metadata + per-learner coverage once and caches it.
// Concurrent callers share one in-flight fetch (tracked in catalogPromise) so
// a second render doesn't resolve early and paint before the data arrives.
let catalogPromise = null;
async function loadCatalog() {
    if (state.catalog)
        return;
    if (catalogPromise)
        return catalogPromise;
    catalogPromise = (async () => {
        try {
            const resp = await fetch('/api/catalog', { credentials: 'same-origin' });
            if (!resp.ok)
                return;
            state.catalog = await resp.json();
        }
        catch {
            // Leave state.catalog null; paintCatalogGrid renders an error line.
        }
        finally {
            // Allow a retry on the next render if the fetch failed.
            if (!state.catalog)
                catalogPromise = null;
        }
    })();
    return catalogPromise;
}
function paintCatalogGrid(gridId) {
    const grid = document.getElementById(gridId);
    if (!grid)
        return;
    if (!state.catalog) {
        grid.innerHTML = '<p class="empty-state">Couldn\'t load the text catalog. <a href="#/inspect">Paste your own text</a> instead.</p>';
        return;
    }
    // Show texts for the active language first; if none, fall back to all.
    const active = state.activeLanguage.toLowerCase();
    let entries = state.catalog.entries.filter(e => e.language === active);
    if (entries.length === 0)
        entries = state.catalog.entries;
    if (entries.length === 0) {
        grid.innerHTML = '<p class="empty-state">No curated texts yet.</p>';
        return;
    }
    const hasKnown = !!state.catalog.has_known_words[state.activeLanguage];
    grid.innerHTML = entries.map(e => renderCatalogCard(e, hasKnown)).join('');
    grid.querySelectorAll('[data-catalog-id]').forEach(btn => {
        btn.addEventListener('click', () => {
            void pickCatalogText(btn.dataset.catalogId || '');
        });
    });
}
function renderCatalogCard(e, hasKnownForLang) {
    const genre = GENRE_LABEL[e.genre] || escapeHtml(e.genre);
    const difficulty = DIFFICULTY_LABEL[e.difficulty] || escapeHtml(e.difficulty);
    const author = e.author ? ` · ${escapeHtml(e.author)}` : '';
    // Personalized Text Fit overlay when we have coverage; otherwise prompt
    // import (only meaningful if the learner truly has no known words).
    let fit;
    if (e.coverage) {
        fit = `<span class="catalog-fit">≈${e.coverage.known_pct}% words you know</span>`;
    }
    else if (!hasKnownForLang) {
        fit = `<a class="catalog-fit catalog-fit-prompt" href="#/vocab">Import known words for a fit estimate</a>`;
    }
    else {
        fit = '';
    }
    return `<div class="catalog-card">
        <div class="catalog-card-head">
            <span class="catalog-tag catalog-tag-genre">${genre}</span>
            <span class="catalog-tag catalog-tag-difficulty catalog-diff-${escapeHtml(e.difficulty)}">${difficulty}</span>
        </div>
        <h4 class="catalog-title">${escapeHtml(e.title)}</h4>
        <p class="catalog-meta">${escapeHtml(String(e.word_count))} words${author}</p>
        ${fit}
        <button type="button" class="btn btn-primary btn-sm catalog-load" data-catalog-id="${escapeHtml(e.id)}">Read this text</button>
        ${e.attribution ? `<p class="catalog-attribution">${escapeHtml(e.attribution)}</p>` : ''}
    </div>`;
}
// pickCatalogText lazy-loads the full text and opens it "like a book": it drops
// the text into the Inspect textarea (so re-parse/edit still works via the
// Words/Inspect path), then parses immediately and lands on the Read tab - zero
// intermediate clicks. The owner hit a dead end here before: the text stopped at
// the textarea and there was no obvious next step (USER_FLOWS §"catalog").
async function pickCatalogText(id) {
    if (!id)
        return;
    const meta = state.catalog?.entries.find(e => e.id === id);
    try {
        const resp = await fetch(`/api/catalog/${encodeURIComponent(id)}/text`, { credentials: 'same-origin' });
        if (!resp.ok) {
            showToast('Could not load that text. Please try another.', 'error');
            return;
        }
        const data = await resp.json();
        const lang = (data.language || meta?.language || 'fi').toUpperCase();
        if (lang !== state.activeLanguage && state.learningLanguages.includes(lang)) {
            await setActiveLanguage(lang);
        }
        navigate('/inspect');
        // Populate after navigation so the inspect page exists in the DOM. The
        // text stays in the textarea state so the learner can edit and re-parse
        // from Inspect afterwards.
        const els = getInspectEls();
        const ta = els?.text ?? null;
        if (els && ta) {
            clearLoadedEpub(els, true);
            ta.value = data.text;
            ta.dispatchEvent(new Event('input', { bubbles: true }));
            // Open the text like a book: parse now and land on the Read tab
            // (forced below), instead of leaving the learner at the textarea.
            await runParse(els, 'custom', 'inspect', 'inspect-submit');
            if (state.currentResults)
                switchResultsTab('read');
        }
    }
    catch {
        showToast('Could not load that text. Please try another.', 'error');
    }
}
// renderInspectColdStart shows the catalog picker on the Inspect page while
// the textarea is empty (an empty-state cold-start affordance). Once the
// learner has pasted or loaded text, it hides so it doesn't crowd the parse
// action. Called on entering /inspect; the textarea's input handler hides it
// as soon as content appears.
function renderInspectColdStart() {
    const ta = document.getElementById('inspect-text');
    const empty = !ta || ta.value.trim() === '';
    renderCatalogSection('inspect-catalog-section', 'inspect-catalog', empty);
}
// Renders the trailing-14-day review activity as plain CSS bars. Hidden until
// the user has answered at least one review in the window - an all-zero chart
// on a fresh account reads as "something is broken", not "get started".
function renderReviewActivityChart(days) {
    const section = document.getElementById('dashboard-activity');
    const chart = document.getElementById('dashboard-activity-chart');
    if (!section || !chart)
        return;
    const max = days.reduce((m, d) => Math.max(m, d.count), 0);
    if (max === 0) {
        section.classList.add('hidden');
        chart.innerHTML = '';
        return;
    }
    chart.innerHTML = days.map(d => {
        const heightPct = Math.max(4, Math.round((d.count / max) * 100));
        const date = new Date(`${d.day}T00:00:00Z`);
        const label = Number.isNaN(date.getTime())
            ? d.day
            : date.toLocaleDateString(undefined, { month: 'short', day: 'numeric', timeZone: 'UTC' });
        return `<div class="activity-bar-slot" data-tooltip="${escapeHtml(label)}: ${d.count}">
            <div class="activity-bar" style="height: ${heightPct}%"></div>
            <span class="activity-count">${d.count > 0 ? d.count : ''}</span>
        </div>`;
    }).join('');
    section.classList.remove('hidden');
}
async function refreshDashboardData(options = {}) {
    await fetchMe();
    if (options.rerenderRoute !== false) {
        renderRoute();
    }
}
function formatHistoryDate(value) {
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
function renderHistoryPage() {
    const list = document.getElementById('history-list');
    const empty = document.getElementById('history-empty');
    const deleteAll = document.getElementById('history-delete-all');
    if (!list || !empty)
        return;
    if (!state.parseSessionsLoaded) {
        empty.classList.add('hidden');
        list.innerHTML = '<p class="empty-state">Loading parse history...</p>';
        if (deleteAll)
            deleteAll.disabled = true;
        return;
    }
    const sessions = state.parseSessions;
    if (deleteAll)
        deleteAll.disabled = sessions.length === 0;
    empty.classList.toggle('hidden', sessions.length > 0);
    if (sessions.length === 0) {
        list.innerHTML = '';
        return;
    }
    list.innerHTML = sessions.map(item => {
        const lang = escapeHtml(languageName(item.lang));
        const parser = escapeHtml(item.parser || 'custom');
        // item.title is the derived display title (store.DeriveTitle, same
        // rule set as the deck-save modal's prefill) - it replaces the raw
        // truncated source_preview as the row's headline so raw pastes read
        // as cleanly here as a deliberately-named deck.
        const title = escapeHtml(item.title || '(empty text)');
        const deckPart = `${item.deck_count.toLocaleString()} deck${item.deck_count === 1 ? '' : 's'}`;
        const feedbackPart = `${item.feedback_count.toLocaleString()} feedback item${item.feedback_count === 1 ? '' : 's'}`;
        return `<article class="history-row">
            <div class="history-row-main">
                <p class="history-row-meta">${lang} · ${parser} · ${formatHistoryDate(item.created_at)}</p>
                <p class="history-row-preview">${title}</p>
                <p class="history-row-counts">${item.total_tokens.toLocaleString()} tokens · ${item.unique_words.toLocaleString()} unique words · ${deckPart} · ${feedbackPart}</p>
            </div>
            <button type="button" class="btn btn-outline btn-sm" data-delete-parse-session="${item.id}">Delete</button>
        </article>`;
    }).join('');
}
async function loadParseSessions() {
    state.parseSessionsLoaded = false;
    renderHistoryPage();
    try {
        const resp = await fetch('/api/parse/sessions', { credentials: 'same-origin' });
        if (!resp.ok)
            throw new Error(await resp.text() || 'Failed to load parse history');
        const data = await resp.json();
        state.parseSessions = data.sessions || [];
        state.parseSessionsLoaded = true;
        renderHistoryPage();
    }
    catch (err) {
        state.parseSessionsLoaded = true;
        state.parseSessions = [];
        renderHistoryPage();
        showToast(err.message || 'Failed to load parse history.', 'error');
    }
}
async function deleteParseSession(id) {
    const item = state.parseSessions.find(s => s.id === id);
    const confirmed = await showConfirm({
        title: 'Delete parse session?',
        message: item
            ? `Delete this retained ${languageName(item.lang)} parse? Linked decks stay, but feedback tied to this parse is removed.`
            : 'Delete this retained parse? Linked decks stay, but feedback tied to this parse is removed.',
        confirmLabel: 'Delete',
        danger: true,
    });
    if (!confirmed)
        return;
    try {
        const resp = await fetch(`/api/parse/sessions/${id}`, {
            method: 'DELETE',
            credentials: 'same-origin',
        });
        if (!resp.ok)
            throw new Error(await resp.text() || 'Delete failed');
        showToast('Parse session deleted.', 'success');
        await loadParseSessions();
    }
    catch (err) {
        showToast(err.message || 'Failed to delete parse session.', 'error');
    }
}
async function deleteAllParseSessions() {
    const count = state.parseSessions.length;
    if (count === 0) {
        showToast('No retained parse sessions to delete.', 'info');
        return;
    }
    const confirmed = await showConfirm({
        title: 'Delete all parse history?',
        message: `Delete ${count.toLocaleString()} retained parse session${count === 1 ? '' : 's'}? Linked decks stay, but feedback tied to these parses is removed.`,
        confirmLabel: 'Delete all',
        danger: true,
    });
    if (!confirmed)
        return;
    try {
        const resp = await fetch('/api/parse/sessions', {
            method: 'DELETE',
            credentials: 'same-origin',
        });
        if (!resp.ok)
            throw new Error(await resp.text() || 'Delete failed');
        showToast('Parse history deleted.', 'success');
        await loadParseSessions();
    }
    catch (err) {
        showToast(err.message || 'Failed to delete parse history.', 'error');
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
        // Keep the table, Read view, and coverage reveal in step with the new
        // known-state, settling instantly rather than replaying the entrance
        // animation.
        refreshResultsViews();
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
        const comprehensionPart = typeof deck.comprehension_pct === 'number'
            ? ` · ${deck.comprehension_pct}% comprehension`
            : '';
        return `<article class="deck-list-item">
            <div>
                <h2><a href="#/decks/${deck.id}" class="deck-list-title">${escapeHtml(deck.title)}</a> ${badges.join(' ')}</h2>
                <p class="deck-list-meta">${langName} · ${deck.known}/${deck.unique} known (${knownPct}%) · ${deck.due} due${comprehensionPart}</p>
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
        // Owner of the deck doesn't subscribe to their own publication - they
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
        // mark the cache as loaded - wiping would render the misleading
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
        total.textContent = v === undefined ? '-' : v.toLocaleString();
    }
    renderVocabLangStat();
    renderVocabAnkiSyncButton();
}
// Visible only after a successful import for the active language. Clicking
// it opens the Anki modal in sync mode (skips deck + field pickers). The
// "Skip confirmation on next sync" toggle lives inside the Anki import
// settings popup now.
function renderVocabAnkiSyncButton() {
    const btn = document.getElementById('vocab-anki-sync');
    if (!btn)
        return;
    const prefs = loadAnkiPrefs(state.activeLanguage);
    const visible = prefs.lastSyncAt > 0 && (prefs.decks?.length ?? 0) > 0;
    btn.classList.toggle('hidden', !visible);
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
        hint.textContent = `Importing in ${languageName(state.activeLanguage)} - switch the dropdown at the top to import in another language.`;
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
async function postKnownWords(words, source = 'manual', lang = state.activeLanguage, signal) {
    const resp = await fetch('/api/known-words', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'same-origin',
        body: JSON.stringify({ lang, words, source }),
        signal,
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
async function putKnownWords(words, scope = 'anki', lang = state.activeLanguage, signal) {
    const resp = await fetch('/api/known-words', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'same-origin',
        body: JSON.stringify({ lang, words, scope }),
        signal,
    });
    if (!resp.ok)
        throw new Error(await resp.text() || 'Failed to sync known words');
    return await resp.json();
}
function describeReplaceResult(data) {
    const added = data.added?.length || 0;
    const removed = data.removed?.length || 0;
    const unresolved = data.unresolved?.length || 0;
    const parts = [];
    parts.push(`${added} added`);
    parts.push(`${removed} removed`);
    if (unresolved > 0)
        parts.push(`${unresolved} unresolved`);
    return parts.join(', ');
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
// Strip Anki's HTML formatting (cards often contain <b>, <img>, …) so we send
// a clean text value to the parser.
function stripHtml(s) {
    const doc = new DOMParser().parseFromString(s, 'text/html');
    return (doc.body.textContent || '').trim();
}
// Anki decks built from textbooks routinely include pedagogical notation that
// our parser can't lemmatise: `anna/n` for "stem + 1sg ending", `diréktor`
// with an acute-accent stress mark, `iial(gi)` for an optional suffix, etc.
// Each rewrite here is grounded in a real textbook convention and confirmed
// to recover entries that were otherwise dropped as unresolved.
//
// Returns "" to drop a surface form entirely (used for phrase-pattern slots
// like `… all` that can never resolve).
function cleanAnkiSurfaceForm(word) {
    let s = word.trim();
    if (!s)
        return '';
    // Phrase-pattern slots: anything containing an ellipsis isn't a word.
    if (s.includes('…') || s.includes('...'))
        return '';
    // Drop combining acute accents over vowels (`é`, `á`, `ó`, …). Neither
    // Estonian nor Finnish orthography uses acutes for stress; textbooks do.
    // NFD decomposes precomposed letters; the regex strips just the acute.
    s = s.normalize('NFD').replace(/́/g, '').normalize('NFC');
    // 1sg verb-stem notation: `anna/n` means "stem 'anna' + 1sg suffix '-n'"
    // = the actual surface form `annan`. Concatenate by rewriting `/n` (at a
    // word boundary) into a bare `n`.
    s = s.replace(/([\p{L}])\/n(\b|$)/gu, '$1n');
    // Parenthetical alternates: `iial(gi)` ≈ "iial, optionally with -gi";
    // `tool (n)` ≈ "tool, noun". Strip the parenthetical and any leading
    // whitespace that was holding it apart.
    s = s.replace(/\s*\([^)]*\)/g, '');
    // Trailing sentence punctuation: `mine!` → `mine`. Keeps the bare imperative
    // resolvable. Doesn't affect mid-word punctuation (none expected here).
    s = s.replace(/[!?.,;:]+$/, '');
    return s.trim();
}
function ankiPrefsKey(lang) {
    return `finest:anki-import:${lang}`;
}
// Default filter is a case-insensitive substring/regex match that picks up the
// language's English and native names. Users override by typing in the filter.
function defaultAnkiFilter(lang) {
    if (lang === 'ET')
        return 'estonian|eesti';
    if (lang === 'FI')
        return 'finnish|suomi';
    return '';
}
function loadAnkiPrefs(lang) {
    const empty = {
        filter: defaultAnkiFilter(lang),
        decks: [],
        fieldByModel: {},
        includeNew: false,
        includeSuspended: false,
        replaceMode: false,
        lastSyncAt: 0,
        replaceConfirmSkip: false,
        preserveManualOnReplace: true,
    };
    try {
        const raw = localStorage.getItem(ankiPrefsKey(lang));
        if (!raw)
            return empty;
        const parsed = JSON.parse(raw);
        return {
            filter: typeof parsed.filter === 'string' ? parsed.filter : empty.filter,
            decks: Array.isArray(parsed.decks) ? parsed.decks : empty.decks,
            fieldByModel: parsed.fieldByModel && typeof parsed.fieldByModel === 'object' ? parsed.fieldByModel : empty.fieldByModel,
            includeNew: typeof parsed.includeNew === 'boolean' ? parsed.includeNew : empty.includeNew,
            includeSuspended: typeof parsed.includeSuspended === 'boolean' ? parsed.includeSuspended : empty.includeSuspended,
            replaceMode: typeof parsed.replaceMode === 'boolean' ? parsed.replaceMode : empty.replaceMode,
            lastSyncAt: typeof parsed.lastSyncAt === 'number' ? parsed.lastSyncAt : empty.lastSyncAt,
            replaceConfirmSkip: typeof parsed.replaceConfirmSkip === 'boolean' ? parsed.replaceConfirmSkip : empty.replaceConfirmSkip,
            preserveManualOnReplace: typeof parsed.preserveManualOnReplace === 'boolean' ? parsed.preserveManualOnReplace : empty.preserveManualOnReplace,
        };
    }
    catch {
        return empty;
    }
}
function saveAnkiPrefs(lang, prefs) {
    try {
        localStorage.setItem(ankiPrefsKey(lang), JSON.stringify(prefs));
    }
    catch {
        // localStorage may be unavailable (private browsing). Silently skip -
        // the user just loses the preset for this session.
    }
}
function buildAnkiDeckTree(deckNames) {
    const root = { name: '', fullName: '', children: [] };
    const sorted = deckNames.slice().sort((a, b) => a.localeCompare(b));
    for (const name of sorted) {
        const parts = name.split('::');
        let node = root;
        let path = '';
        for (const part of parts) {
            path = path ? `${path}::${part}` : part;
            let child = node.children.find(c => c.name === part);
            if (!child) {
                child = { name: part, fullName: path, children: [] };
                node.children.push(child);
            }
            node = child;
        }
    }
    return root.children;
}
const ankiImport = {
    open: false,
    sessionID: 0,
    cancelRequested: false,
    abortController: null,
    lang: 'FI',
    deckNames: [],
    tree: [],
    selected: new Set(),
    expanded: new Set(),
    filter: '',
    models: [],
    fieldsByModel: {},
    fieldByModel: {},
    examplesByModel: {},
    allNotes: [],
    includeNew: false,
    includeSuspended: false,
    replaceMode: false,
    preserveManualOnReplace: true,
    syncMode: false,
    replaceConfirmedThisRun: false,
};
// Field names that hint "this is the bare word/lemma", per active language.
// Universal English terms first so any English-labeled deck works; then the
// native-language equivalents so users whose deck templates use Estonian or
// Finnish names get a sensible guess too. Matched by case-insensitive
// substring so variants like "Word (FI)" or "Sõna (lihtne)" still hit.
const PREFERRED_FIELD_NAMES = {
    FI: ['word', 'expression', 'term', 'lemma', 'headword', 'vocab', 'sana', 'ilmaisu', 'käsite', 'termi'],
    ET: ['word', 'expression', 'term', 'lemma', 'headword', 'vocab', 'sõna', 'väljend', 'mõiste', 'termin'],
};
// Pick the field most likely to hold a single bare word for this model.
// Strategy: rank candidates by what fraction of sampled non-empty values are
// a single token (no internal whitespace). Among the ones that pass the
// single-word bar, prefer field names matching the language's word/expression
// vocabulary. Fall back to the field with the highest single-word ratio, or
// to the model's first field if we have no examples at all.
function pickBestField(fields, examples, lang) {
    if (fields.length === 0)
        return '';
    const singleWordRatio = {};
    for (const f of fields) {
        const nonEmpty = (examples[f] || []).filter(v => v.trim() !== '');
        if (nonEmpty.length === 0) {
            singleWordRatio[f] = 0;
            continue;
        }
        const singleCount = nonEmpty.filter(v => !/\s/.test(v.trim())).length;
        singleWordRatio[f] = singleCount / nonEmpty.length;
    }
    const candidates = fields.filter(f => singleWordRatio[f] >= 0.5);
    if (candidates.length === 1)
        return candidates[0];
    const preferred = PREFERRED_FIELD_NAMES[lang] || PREFERRED_FIELD_NAMES.FI;
    const pool = candidates.length > 0 ? candidates : fields;
    for (const hint of preferred) {
        const match = pool.find(f => f.toLowerCase().includes(hint.toLowerCase()));
        if (match)
            return match;
    }
    if (candidates.length > 0)
        return candidates[0];
    // No single-word fields and no name hints - pick whichever field at least
    // had any non-empty content, falling back to the first.
    const withContent = fields.find(f => (examples[f] || []).some(v => v.trim() !== ''));
    return withContent || fields[0];
}
const ANKI_STAGE_IDS = {
    loading: 'anki-import-stage-loading',
    decks: 'anki-import-stage-decks',
    fields: 'anki-import-stage-fields',
    running: 'anki-import-stage-running',
};
function showAnkiStage(stage) {
    for (const s of Object.keys(ANKI_STAGE_IDS)) {
        document.getElementById(ANKI_STAGE_IDS[s])?.classList.toggle('hidden', s !== stage);
    }
}
function openAnkiImportModal() {
    openAnkiModal(false);
}
// Quick-action "Sync from Anki" entry point. Same setup as the manual flow,
// but flags `syncMode = true` so once discovery completes we skip past the
// deck picker and field-picker stages and go straight to import using the
// saved prefs. Fails over to the manual flow if discovery turns up no
// matching decks.
// Quick-action sync flow. Doesn't show the "Connect to Anki" modal up
// front - instead surfaces the replace-mode confirmation dialog (if
// applicable) with a status indicator that flips from a spinner to a check
// mark once discovery completes. The full modal only appears at the running
// stage, or when discovery surfaces a state change that needs review.
async function openAnkiSyncModal() {
    // Disable the trigger button for the duration of the sync so a frantic
    // double-click can't kick off two parallel imports. Re-enabled in the
    // finally below - guarantees the button isn't left stuck on any error
    // or early-return path.
    const syncBtn = document.getElementById('vocab-anki-sync');
    if (syncBtn)
        syncBtn.disabled = true;
    try {
        await runAnkiSyncFlow();
    }
    finally {
        if (syncBtn)
            syncBtn.disabled = false;
    }
}
// The replace-mode confirmation must describe what the current
// preserve-manual setting will actually do: in the default preserve-manual
// mode, textbox/file words survive the sync, and warning that they'll be
// removed trains users to distrust (and disable) the safe setting.
function ankiReplaceConfirmMessage(langName, preserveManual) {
    if (preserveManual) {
        return `This will sync your ${langName} Anki-imported known-words to exactly what's in the selected Anki decks. Words you added through the textbox or a file will be kept; Anki-imported words not in this selection will be removed.`;
    }
    return `This will sync your ${langName} known-words to exactly what's in the selected Anki decks. Lemmas not in this selection - including ones you added through the textbox or a file - will be removed.`;
}
async function runAnkiSyncFlow() {
    const prefs = loadAnkiPrefs(state.activeLanguage);
    // Sync needs a prior successful import and at least one saved deck.
    // Anything else routes to the manual flow.
    if (!prefs.lastSyncAt || prefs.decks.length === 0) {
        openAnkiModal(false);
        return;
    }
    initializeAnkiState(prefs, /* sync */ true);
    const sessionID = ankiImport.sessionID;
    // Kick off discovery immediately - runs concurrently with whatever dialog
    // is shown so the user never waits for sequential steps. The promise
    // resolves to a structured result rather than throwing so the dialog can
    // surface "Anki ready" vs an actionable error without separate paths.
    const discovery = runSyncDiscovery(sessionID);
    const needsConfirm = ankiImport.replaceMode && !prefs.replaceConfirmSkip;
    let validation;
    let dialogConfirmed = true;
    if (needsConfirm) {
        const langName = languageName(ankiImport.lang);
        // No rememberLabel here - the "Skip confirmation on next sync" toggle
        // lives below the sync button on the vocab page now, so the dialog
        // stays focused on the confirm action.
        const dialog = await showConfirmWithStatus({
            title: `Replace ${langName} vocabulary?`,
            message: ankiReplaceConfirmMessage(langName, ankiImport.preserveManualOnReplace),
            confirmLabel: 'Sync and replace',
            danger: true,
            loadingText: 'Checking Anki…',
        }, discovery, (val) => val.ok
            ? { state: 'success', text: 'Anki ready.' }
            : { state: 'error', text: val.detail });
        dialogConfirmed = dialog.confirmed;
        // status is the SyncDiscoveryResultOrFailure from runSyncDiscovery -
        // undefined only if the promise itself failed (shouldn't happen
        // since runSyncDiscovery catches everything).
        validation = dialog.status || { ok: false, reason: 'connect-failed', detail: 'Discovery did not complete.' };
        // If validation failed, fall through to the failure-handling
        // branch below regardless of whether the user cancelled - the
        // Confirm button was disabled so they could only cancel anyway.
        if (validation.ok && !dialogConfirmed)
            return; // user cancelled a healthy validation
    }
    else {
        // No confirm dialog → the import modal IS the immediate feedback.
        // Open it at the loading stage with the sync-flavoured "Syncing
        // from Anki…" label so the user sees something the moment they
        // click. Once discovery completes we transition to the running
        // stage (or to a manual stage on a state-change error).
        openAnkiModalAtStage('loading');
        const loadingMsg = document.getElementById('anki-import-loading-msg');
        if (loadingMsg)
            loadingMsg.textContent = 'Syncing from Anki…';
        validation = await discovery;
        if (!validation.ok && validation.reason === 'cancelled')
            return;
    }
    if (isAnkiImportCancelled(sessionID))
        return;
    if (!validation.ok) {
        if (validation.reason === 'cancelled')
            return;
        // Discovery turned up something the user should see - open the modal
        // in the appropriate manual-flow stage with the toast we'd normally
        // show in the auto-advance path.
        if (validation.reason === 'connect-failed') {
            showToast(validation.detail || 'Could not reach Anki.', 'error');
            openAnkiSetupModal();
            return;
        }
        // Deck-related issues (missing decks, fully empty selection) route to
        // the deck-picker stage so the user can re-select. A model-set change
        // routes to the field-picker stage so they can review the new card
        // types.
        const stage = validation.reason === 'model-changed' ? 'fields' : 'decks';
        openAnkiModalAtStage(stage);
        showToast(validation.detail || 'Anki state has changed. Review your selection.', 'info', 6000);
        return;
    }
    // All clear - show the modal at the running stage and execute the
    // import. The note snapshots are already in memory from discovery.
    // Flag the run as already-confirmed so runAnkiImport doesn't pop a
    // second replace dialog on top of the one the user just dismissed.
    if (needsConfirm)
        ankiImport.replaceConfirmedThisRun = true;
    openAnkiModalAtStage('running');
    // Await the import so the sync-button disabled-window in
    // openAnkiSyncModal's finally extends through the entire run.
    await runAnkiImport();
}
function openAnkiModal(sync) {
    const modal = document.getElementById('anki-import-modal');
    if (!modal)
        return;
    initializeAnkiState(loadAnkiPrefs(state.activeLanguage), sync);
    modal.classList.remove('hidden');
    showAnkiStage('loading');
    const loadingMsg = document.getElementById('anki-import-loading-msg');
    if (loadingMsg)
        loadingMsg.textContent = sync ? 'Syncing from Anki…' : 'Connecting to AnkiConnect…';
    void connectAndLoadDecks();
}
// Pre-populate `ankiImport` state from saved prefs without touching the DOM.
// Shared between the manual and the silent (quick-sync) entry points.
function initializeAnkiState(prefs, sync) {
    ankiImport.abortController?.abort();
    ankiImport.sessionID += 1;
    ankiImport.open = true;
    ankiImport.cancelRequested = false;
    ankiImport.abortController = null;
    ankiImport.lang = state.activeLanguage;
    ankiImport.filter = prefs.filter;
    ankiImport.selected = new Set(prefs.decks);
    ankiImport.fieldByModel = { ...prefs.fieldByModel };
    ankiImport.includeNew = prefs.includeNew;
    ankiImport.includeSuspended = prefs.includeSuspended;
    ankiImport.replaceMode = prefs.replaceMode;
    ankiImport.preserveManualOnReplace = prefs.preserveManualOnReplace;
    ankiImport.syncMode = sync;
    ankiImport.allNotes = [];
    ankiImport.expanded = new Set();
}
function isAnkiImportCancelled(sessionID) {
    return ankiImport.cancelRequested || ankiImport.sessionID !== sessionID;
}
// Open the modal at a specific stage. Used by the sync flow once discovery
// has populated `ankiImport.allNotes` etc - we skip the "loading" stage
// because there's nothing left to load.
function openAnkiModalAtStage(stage) {
    const modal = document.getElementById('anki-import-modal');
    if (!modal)
        return;
    modal.classList.remove('hidden');
    // The sync flow has already populated state but didn't render any
    // section - fill in everything the target stage needs to look right.
    if (stage === 'decks') {
        // Build the deck tree from the deckNames we cached during discovery.
        if (ankiImport.tree.length === 0) {
            ankiImport.tree = buildAnkiDeckTree(ankiImport.deckNames);
        }
        // Auto-expand ancestors of every preselected deck so they're
        // visible without the user having to drill in.
        for (const deck of ankiImport.selected) {
            for (const ancestor of ancestorPaths(deck))
                ankiImport.expanded.add(ancestor);
        }
        renderAnkiFilter();
        renderAnkiDeckTree();
        renderAnkiDeckSummary();
    }
    else if (stage === 'fields') {
        renderAnkiFieldPickers();
        renderAnkiImportEstimate();
    }
    else if (stage === 'running') {
        const msg = document.getElementById('anki-import-running-msg');
        const bar = document.getElementById('anki-import-progress-bar');
        if (msg)
            msg.textContent = 'Starting…';
        if (bar)
            bar.style.width = '0%';
    }
    showAnkiStage(stage);
}
// Runs the full discovery pipeline silently (no modal side effects beyond
// populating `ankiImport.allNotes` / `ankiImport.fieldByModel` / etc). Used
// by the quick-sync flow. Mirrors connectAndLoadDecks +
// loadAnkiModelsForSelection but condensed and returns a structured result
// instead of branching into stages.
async function runSyncDiscovery(sessionID) {
    const cancelled = () => ({ ok: false, reason: 'cancelled', detail: '' });
    try {
        await ankiInvoke('version');
        if (isAnkiImportCancelled(sessionID))
            return cancelled();
        const deckNames = await ankiInvoke('deckNames');
        if (isAnkiImportCancelled(sessionID))
            return cancelled();
        ankiImport.deckNames = deckNames;
        ankiImport.tree = buildAnkiDeckTree(deckNames);
        const valid = new Set(deckNames);
        const savedDecks = Array.from(ankiImport.selected);
        const missingDecks = savedDecks.filter(d => !valid.has(d));
        for (const d of missingDecks)
            ankiImport.selected.delete(d);
        if (ankiImport.selected.size === 0) {
            return { ok: false, reason: 'empty-selection', detail: missingDecks.length > 0
                    ? `${missingDecks.length} previously-imported deck${missingDecks.length === 1 ? '' : 's'} no longer exist in Anki. Pick a new selection.`
                    : 'No previously-imported decks exist in Anki any more.' };
        }
        if (missingDecks.length > 0) {
            return { ok: false, reason: 'deck-missing', detail: `${missingDecks.length} previously-imported deck${missingDecks.length === 1 ? '' : 's'} no longer exist${missingDecks.length === 1 ? 's' : ''} in Anki. Review your selection.` };
        }
        // Fetch notes (all + studied + non-suspended sets per deck).
        const decks = Array.from(ankiImport.selected);
        const perDeck = await Promise.all(decks.map(async (d) => {
            const [all, studied, notSuspended] = await Promise.all([
                ankiInvoke('findNotes', { query: `deck:"${d}"` }),
                ankiInvoke('findNotes', { query: `deck:"${d}" -is:new` }),
                ankiInvoke('findNotes', { query: `deck:"${d}" -is:suspended` }),
            ]);
            return { all, studied, notSuspended };
        }));
        if (isAnkiImportCancelled(sessionID))
            return cancelled();
        const studiedSet = new Set();
        const notSuspendedSet = new Set();
        const seenIDs = new Set();
        const allIDs = [];
        for (const { all, studied, notSuspended } of perDeck) {
            for (const id of all) {
                if (seenIDs.has(id))
                    continue;
                seenIDs.add(id);
                allIDs.push(id);
            }
            for (const id of studied)
                studiedSet.add(id);
            for (const id of notSuspended)
                notSuspendedSet.add(id);
        }
        const snapshots = [];
        const CHUNK = 500;
        for (let i = 0; i < allIDs.length; i += CHUNK) {
            if (isAnkiImportCancelled(sessionID))
                return cancelled();
            const chunk = allIDs.slice(i, i + CHUNK);
            const notes = await ankiInvoke('notesInfo', { notes: chunk });
            if (isAnkiImportCancelled(sessionID))
                return cancelled();
            for (const note of notes) {
                if (!note?.modelName)
                    continue;
                const stripped = {};
                for (const [name, info] of Object.entries(note.fields || {})) {
                    stripped[name] = stripHtml(info.value || '');
                }
                snapshots.push({
                    noteId: note.noteId,
                    modelName: note.modelName,
                    fields: stripped,
                    studied: studiedSet.has(note.noteId),
                    suspended: !notSuspendedSet.has(note.noteId),
                });
            }
        }
        ankiImport.allNotes = snapshots;
        const savedModelKeys = new Set(Object.keys(ankiImport.fieldByModel));
        const modelSet = new Set();
        for (const n of snapshots)
            modelSet.add(n.modelName);
        const models = Array.from(modelSet).sort((a, b) => a.localeCompare(b));
        ankiImport.models = models;
        // Field lists per model.
        const fieldsByModel = {};
        await Promise.all(models.map(async (model) => {
            try {
                fieldsByModel[model] = await ankiInvoke('modelFieldNames', { modelName: model });
            }
            catch {
                fieldsByModel[model] = [];
            }
        }));
        if (isAnkiImportCancelled(sessionID))
            return cancelled();
        ankiImport.fieldsByModel = fieldsByModel;
        // Per-(model, field) examples - same as loadAnkiModelsForSelection,
        // also auto-picks a field for models the user hasn't seen yet.
        const examplesByModel = {};
        for (const model of models) {
            const fieldList = fieldsByModel[model] || [];
            const examples = {};
            for (const f of fieldList)
                examples[f] = [];
            for (const note of snapshots) {
                if (note.modelName !== model)
                    continue;
                for (const f of fieldList) {
                    const v = (note.fields[f] || '').trim();
                    if (!v)
                        continue;
                    const bucket = examples[f];
                    if (bucket.length < 2 && !bucket.includes(v))
                        bucket.push(v);
                }
            }
            examplesByModel[model] = examples;
            if (!(model in ankiImport.fieldByModel)) {
                ankiImport.fieldByModel[model] = pickBestField(fieldList, examples, ankiImport.lang);
            }
        }
        ankiImport.examplesByModel = examplesByModel;
        if (isAnkiImportCancelled(sessionID))
            return cancelled();
        persistAnkiPrefs();
        // Detect model-set drift (new card type / removed one). Same contract
        // as the modal-driven sync flow.
        const newModels = models.filter(m => !savedModelKeys.has(m));
        const goneModels = Array.from(savedModelKeys).filter(m => !modelSet.has(m));
        if (newModels.length > 0 || goneModels.length > 0) {
            const parts = [];
            if (newModels.length > 0)
                parts.push(`${newModels.length} new card type${newModels.length === 1 ? '' : 's'}`);
            if (goneModels.length > 0)
                parts.push(`${goneModels.length} card type${goneModels.length === 1 ? '' : 's'} removed`);
            return { ok: false, reason: 'model-changed', detail: `Anki state has changed (${parts.join(', ')}). Review the field selection before syncing.` };
        }
        if (selectedAnkiNotes().length === 0) {
            return { ok: false, reason: 'empty-selection', detail: 'No notes match the current selection.' };
        }
        return { ok: true };
    }
    catch (err) {
        const msg = err && err.message
            ? err.message
            : 'Failed to reach Anki.';
        return { ok: false, reason: 'connect-failed', detail: msg };
    }
}
function closeAnkiImportModal() {
    ankiImport.cancelRequested = true;
    ankiImport.abortController?.abort();
    ankiImport.abortController = null;
    ankiImport.open = false;
    document.getElementById('anki-import-modal')?.classList.add('hidden');
}
async function connectAndLoadDecks() {
    const loadingMsg = document.getElementById('anki-import-loading-msg');
    try {
        await ankiInvoke('version');
        if (loadingMsg)
            loadingMsg.textContent = 'Fetching decks…';
        const deckNames = await ankiInvoke('deckNames');
        ankiImport.deckNames = deckNames;
        ankiImport.tree = buildAnkiDeckTree(deckNames);
        // Forget previously-selected decks that no longer exist.
        const valid = new Set(deckNames);
        // Snapshot which saved decks no longer exist BEFORE dropping them.
        // The sync flow uses that to bail to the manual picker rather than
        // silently importing a smaller set than the user expects.
        const savedDecks = Array.from(ankiImport.selected);
        const missingDecks = savedDecks.filter(d => !valid.has(d));
        for (const d of missingDecks)
            ankiImport.selected.delete(d);
        // Sync mode: route to the manual picker on ANY state mismatch
        // (missing deck, or zero remaining decks). This is the "no surprises"
        // contract - the user explicitly hit Sync, they should review changes
        // before we apply a destructive replace. The deck-picker stage opens
        // pre-populated with whatever still exists.
        if (ankiImport.syncMode) {
            if (missingDecks.length > 0 || ankiImport.selected.size === 0) {
                ankiImport.syncMode = false;
                const msg = missingDecks.length > 0
                    ? `${missingDecks.length} previously-imported deck${missingDecks.length === 1 ? '' : 's'} no longer exist${missingDecks.length === 1 ? 's' : ''} in Anki. Review your selection.`
                    : 'No previously-imported decks exist in Anki any more. Pick a new selection.';
                showToast(msg, 'info', 5000);
                // Fall through to the manual deck picker below.
            }
            else {
                void loadAnkiModelsForSelection();
                return;
            }
        }
        ankiImport.syncMode = false; // sync prereqs not met → manual flow
        // Expand ancestors of every preselected deck so the user sees them
        // immediately rather than having to drill in.
        for (const deck of ankiImport.selected) {
            for (const ancestor of ancestorPaths(deck))
                ankiImport.expanded.add(ancestor);
        }
        showAnkiStage('decks');
        renderAnkiFilter();
        renderAnkiDeckTree();
        renderAnkiDeckSummary();
    }
    catch (err) {
        closeAnkiImportModal();
        showToast(err.message || 'Failed to connect to Anki.', 'error');
        openAnkiSetupModal();
    }
}
function ancestorPaths(deckName) {
    const parts = deckName.split('::');
    const out = [];
    for (let i = 1; i < parts.length; i++)
        out.push(parts.slice(0, i).join('::'));
    return out;
}
function renderAnkiFilter() {
    const input = document.getElementById('anki-import-filter');
    if (input && input.value !== ankiImport.filter)
        input.value = ankiImport.filter;
}
// Split filter on `|` and treat each chunk as a case-insensitive substring.
// Matches the way the user expressed the default ("estonian|eesti") without
// forcing them to know regex.
function deckMatchesFilter(name, filter) {
    const f = filter.trim().toLowerCase();
    if (!f)
        return true;
    const haystack = name.toLowerCase();
    return f.split('|').some(part => part.trim() !== '' && haystack.includes(part.trim()));
}
// A node is visible if it (or any descendant) matches the filter. We compute
// this once per render so descendants of a matching ancestor still show even
// if their leaf name doesn't itself match.
function collectVisible(tree, filter) {
    const visible = new Set();
    const walk = (node) => {
        let anyDescendantMatches = false;
        for (const child of node.children) {
            if (walk(child))
                anyDescendantMatches = true;
        }
        const matches = deckMatchesFilter(node.fullName, filter) || anyDescendantMatches;
        if (matches)
            visible.add(node.fullName);
        return matches;
    };
    for (const node of tree)
        walk(node);
    return visible;
}
function renderAnkiDeckTree() {
    const container = document.getElementById('anki-import-tree');
    const empty = document.getElementById('anki-import-tree-empty');
    if (!container || !empty)
        return;
    const visible = collectVisible(ankiImport.tree, ankiImport.filter);
    if (visible.size === 0) {
        container.innerHTML = '';
        empty.classList.remove('hidden');
        return;
    }
    empty.classList.add('hidden');
    const renderNode = (node, depth) => {
        if (!visible.has(node.fullName))
            return '';
        const hasChildren = node.children.length > 0;
        // Auto-expand a folder when filter is active so matches deeper in the
        // tree aren't hidden by collapsed parents.
        const filterActive = ankiImport.filter.trim() !== '';
        const expanded = ankiImport.expanded.has(node.fullName) || filterActive;
        const checked = ankiImport.selected.has(node.fullName);
        const id = `anki-deck-${node.fullName.replace(/[^A-Za-z0-9_-]/g, '_')}`;
        const children = hasChildren && expanded
            ? `<div class="anki-deck-children">${node.children.map(c => renderNode(c, depth + 1)).join('')}</div>`
            : '';
        return `<div class="anki-deck-row" style="--anki-depth: ${depth};" role="treeitem">
            <button type="button" class="anki-deck-toggle${hasChildren ? '' : ' is-leaf'}" data-deck-toggle="${escapeAttr(node.fullName)}" aria-label="${expanded ? 'Collapse' : 'Expand'} ${escapeAttr(node.name)}">
                ${hasChildren ? (expanded ? '▾' : '▸') : ''}
            </button>
            <label class="anki-deck-label">
                <input type="checkbox" class="anki-deck-check" data-deck-check="${escapeAttr(node.fullName)}" id="${id}" ${checked ? 'checked' : ''} />
                <span class="anki-deck-name">${escapeHtml(node.name)}</span>
            </label>
        </div>${children}`;
    };
    container.innerHTML = ankiImport.tree.map(n => renderNode(n, 0)).join('');
}
function renderAnkiDeckSummary() {
    const el = document.getElementById('anki-import-deck-summary');
    if (!el)
        return;
    const n = ankiImport.selected.size;
    el.textContent = n === 0
        ? 'No decks selected.'
        : `${n.toLocaleString()} deck${n === 1 ? '' : 's'} selected.`;
}
function persistAnkiPrefs() {
    // Preserve lastSyncAt + replaceConfirmSkip across writes - those are
    // managed by separate code paths (successful import / dismiss-dialog).
    const existing = loadAnkiPrefs(ankiImport.lang);
    saveAnkiPrefs(ankiImport.lang, {
        filter: ankiImport.filter,
        decks: Array.from(ankiImport.selected),
        fieldByModel: ankiImport.fieldByModel,
        includeNew: ankiImport.includeNew,
        includeSuspended: ankiImport.includeSuspended,
        replaceMode: ankiImport.replaceMode,
        preserveManualOnReplace: ankiImport.preserveManualOnReplace,
        lastSyncAt: existing.lastSyncAt,
        replaceConfirmSkip: existing.replaceConfirmSkip,
    });
}
function recordReplaceConfirmSkip() {
    const prefs = loadAnkiPrefs(ankiImport.lang);
    prefs.replaceConfirmSkip = true;
    saveAnkiPrefs(ankiImport.lang, prefs);
}
function recordAnkiSyncTime() {
    const prefs = loadAnkiPrefs(ankiImport.lang);
    prefs.lastSyncAt = Date.now();
    saveAnkiPrefs(ankiImport.lang, prefs);
}
function onAnkiFilterInput(value) {
    ankiImport.filter = value;
    renderAnkiDeckTree();
    persistAnkiPrefs();
}
function onAnkiDeckToggle(fullName) {
    if (ankiImport.expanded.has(fullName))
        ankiImport.expanded.delete(fullName);
    else
        ankiImport.expanded.add(fullName);
    renderAnkiDeckTree();
}
function findAnkiNode(roots, fullName) {
    for (const node of roots) {
        if (node.fullName === fullName)
            return node;
        const hit = findAnkiNode(node.children, fullName);
        if (hit)
            return hit;
    }
    return null;
}
function collectAnkiSubtree(node) {
    const out = [node.fullName];
    for (const child of node.children)
        out.push(...collectAnkiSubtree(child));
    return out;
}
// Checking a parent deck cascades to every descendant, and unchecking
// cascades the same way. This matches the user's mental model: "select the
// Estonian folder" means "import everything under Estonian", not just the
// parent placeholder deck (which often has zero notes of its own).
function onAnkiDeckCheck(fullName, checked) {
    const node = findAnkiNode(ankiImport.tree, fullName);
    const names = node ? collectAnkiSubtree(node) : [fullName];
    if (checked) {
        for (const name of names)
            ankiImport.selected.add(name);
    }
    else {
        for (const name of names)
            ankiImport.selected.delete(name);
    }
    // Re-render so cascaded descendant checkboxes reflect the new state.
    renderAnkiDeckTree();
    renderAnkiDeckSummary();
    persistAnkiPrefs();
}
// Step 2: discover the note models (card types) used in the user's selected
// decks, then ask which field of each model holds the word. We sample a
// handful of notes per deck rather than every note - fast enough that even a
// 50-deck selection feels instant, and we'd never offer the user a field that
// no real note has, since `modelFieldNames` returns the canonical schema.
async function loadAnkiModelsForSelection() {
    const summary = document.getElementById('anki-import-field-summary');
    if (summary)
        summary.textContent = 'Inspecting card types…';
    showAnkiStage('fields');
    const fieldsContainer = document.getElementById('anki-import-fields');
    if (fieldsContainer)
        fieldsContainer.innerHTML = '';
    try {
        const decks = Array.from(ankiImport.selected);
        // For each selected deck, fetch the full note set plus two subsets:
        //   - "studied"      = notes with at least one non-new card (-is:new)
        //   - "notSuspended" = notes with at least one non-suspended card
        // We invert the suspended set so the snapshot's `suspended` flag is
        // true only when every card on the note is paused. Three queries per
        // deck, fired in parallel - even for a few dozen decks the discovery
        // step stays comfortably under a second.
        const perDeck = await Promise.all(decks.map(async (d) => {
            const [all, studied, notSuspended] = await Promise.all([
                ankiInvoke('findNotes', { query: `deck:"${d}"` }),
                ankiInvoke('findNotes', { query: `deck:"${d}" -is:new` }),
                ankiInvoke('findNotes', { query: `deck:"${d}" -is:suspended` }),
            ]);
            return { all, studied, notSuspended };
        }));
        const studiedSet = new Set();
        const notSuspendedSet = new Set();
        const allIDsSeen = new Set();
        const allIDs = [];
        for (const { all, studied, notSuspended } of perDeck) {
            for (const id of all) {
                if (allIDsSeen.has(id))
                    continue;
                allIDsSeen.add(id);
                allIDs.push(id);
            }
            for (const id of studied)
                studiedSet.add(id);
            for (const id of notSuspended)
                notSuspendedSet.add(id);
        }
        // Fetch every note in the selected decks (chunked to keep payloads
        // reasonable). We need the full data - not just a sample - so that
        // (a) rare models that only appear later don't go missing and (b) the
        // estimate at the bottom of step 2 reflects the real set, not a
        // projection.
        const snapshots = [];
        const CHUNK = 500;
        for (let i = 0; i < allIDs.length; i += CHUNK) {
            const chunk = allIDs.slice(i, i + CHUNK);
            const notes = await ankiInvoke('notesInfo', { notes: chunk });
            for (const note of notes) {
                if (!note?.modelName)
                    continue;
                const stripped = {};
                for (const [name, info] of Object.entries(note.fields || {})) {
                    stripped[name] = stripHtml(info.value || '');
                }
                snapshots.push({
                    noteId: note.noteId,
                    modelName: note.modelName,
                    fields: stripped,
                    studied: studiedSet.has(note.noteId),
                    suspended: !notSuspendedSet.has(note.noteId),
                });
            }
            if (summary) {
                const scanned = Math.min(i + CHUNK, allIDs.length);
                summary.textContent = `Inspecting card types… (${scanned.toLocaleString()} / ${allIDs.length.toLocaleString()} notes scanned)`;
            }
        }
        ankiImport.allNotes = snapshots;
        // Enumerate models from the full set (not just a sample) and grab
        // each model's canonical field list. modelFieldNames is the source
        // of truth for which fields exist and in what order - sampled notes
        // may omit empty fields entirely depending on the model definition.
        const modelSet = new Set();
        for (const n of snapshots)
            modelSet.add(n.modelName);
        const models = Array.from(modelSet).sort((a, b) => a.localeCompare(b));
        ankiImport.models = models;
        const fieldsByModel = {};
        await Promise.all(models.map(async (model) => {
            try {
                const fields = await ankiInvoke('modelFieldNames', { modelName: model });
                fieldsByModel[model] = fields;
            }
            catch {
                fieldsByModel[model] = [];
            }
        }));
        ankiImport.fieldsByModel = fieldsByModel;
        // Snapshot the saved field-by-model keys BEFORE auto-pick mutates
        // them. Sync-mode change detection compares this against the
        // discovered model set so newly-added card types force a manual
        // review.
        const savedModelKeys = new Set(Object.keys(ankiImport.fieldByModel));
        // Build the per-(model, field) example list from the in-memory
        // snapshots, then auto-pick a field for any model the user hasn't
        // manually picked in a previous session. Saved picks always win.
        const examplesByModel = {};
        for (const model of models) {
            const fieldList = fieldsByModel[model] || [];
            const examples = {};
            for (const f of fieldList)
                examples[f] = [];
            for (const note of snapshots) {
                if (note.modelName !== model)
                    continue;
                for (const f of fieldList) {
                    const v = (note.fields[f] || '').trim();
                    if (!v)
                        continue;
                    const bucket = examples[f];
                    if (bucket.length < 2 && !bucket.includes(v))
                        bucket.push(v);
                }
            }
            examplesByModel[model] = examples;
            if (!(model in ankiImport.fieldByModel)) {
                ankiImport.fieldByModel[model] = pickBestField(fieldList, examples, ankiImport.lang);
            }
        }
        ankiImport.examplesByModel = examplesByModel;
        persistAnkiPrefs();
        renderAnkiFieldPickers();
        renderAnkiImportEstimate();
        // Sync mode: skip the manual confirmation and run the import using
        // the saved prefs - UNLESS Anki state has drifted in a way the user
        // should review. Specifically:
        //   - A new card type has appeared in the discovered set that
        //     wasn't in the saved fieldByModel
        //   - A previously-saved card type is no longer present
        // Either case routes back to the field-picker stage with a toast so
        // the user can decide whether to import the new model and what
        // field to use.
        if (ankiImport.syncMode) {
            // savedModelKeys may include models from earlier-different deck
            // selections, but if the deck set is unchanged it's exactly the
            // models the user previously saw.
            const discoveredSet = new Set(models);
            const newModels = models.filter(m => !savedModelKeys.has(m));
            const goneModels = Array.from(savedModelKeys).filter(m => !discoveredSet.has(m));
            if (newModels.length > 0 || goneModels.length > 0) {
                ankiImport.syncMode = false;
                const parts = [];
                if (newModels.length > 0)
                    parts.push(`${newModels.length} new card type${newModels.length === 1 ? '' : 's'}`);
                if (goneModels.length > 0)
                    parts.push(`${goneModels.length} card type${goneModels.length === 1 ? '' : 's'} removed`);
                showToast(`Anki state has changed (${parts.join(', ')}). Review the field selection before syncing.`, 'info', 6000);
                // Stay on the fields stage - the picker is already rendered.
                return;
            }
            if (selectedAnkiNotes().length > 0) {
                ankiImport.syncMode = false;
                void runAnkiImport();
            }
        }
    }
    catch (err) {
        if (summary)
            summary.textContent = err.message || 'Failed to read card types.';
        showToast(err.message || 'Failed to read card types from Anki.', 'error');
    }
}
// Label for the picker trigger button. Empty string = "Skip this card type",
// otherwise the field name itself.
function fieldDisplayLabel(field) {
    return field === '' ? 'Skip this card type' : field;
}
function renderAnkiFieldPickers() {
    const container = document.getElementById('anki-import-fields');
    const summary = document.getElementById('anki-import-field-summary');
    if (!container || !summary)
        return;
    if (ankiImport.models.length === 0) {
        container.innerHTML = '<p class="anki-import-empty">No card types found in the selected decks.</p>';
        summary.textContent = '';
        return;
    }
    summary.textContent = `${ankiImport.models.length} card type${ankiImport.models.length === 1 ? '' : 's'} found.`;
    container.innerHTML = ankiImport.models.map(model => {
        const fields = ankiImport.fieldsByModel[model] || [];
        const examples = ankiImport.examplesByModel[model] || {};
        const current = ankiImport.fieldByModel[model] ?? '';
        // Skip option always sits at the top so an "ignore this card type"
        // choice is one click away. We attach data-examples even on real
        // fields so the hover-tooltip handler doesn't need a separate lookup.
        const options = [];
        options.push(renderFieldOption(model, '', current === '', []));
        for (const f of fields) {
            options.push(renderFieldOption(model, f, current === f, examples[f] || []));
        }
        return `<div class="anki-import-field-row" data-model-row="${escapeAttr(model)}">
            <div class="anki-import-field-label">${escapeHtml(model)}</div>
            <div class="field-picker" data-field-picker="${escapeAttr(model)}">
                <button type="button" class="field-picker-toggle" data-field-toggle="${escapeAttr(model)}" aria-haspopup="listbox" aria-expanded="false">
                    <span class="field-picker-current">${escapeHtml(fieldDisplayLabel(current))}</span>
                    <span class="field-picker-caret" aria-hidden="true"></span>
                </button>
                <ul class="field-picker-menu hidden" role="listbox" aria-label="Field for ${escapeAttr(model)}">
                    ${options.join('')}
                </ul>
            </div>
        </div>`;
    }).join('');
}
function renderFieldOption(model, field, isSelected, examples) {
    // Encode examples in a data attribute so the hover handler can read them
    // without a separate state lookup. Newline-joined and escaped - the
    // showFieldExamplesTip handler splits on \n when rendering.
    const exAttr = examples.length > 0 ? escapeAttr(examples.join('\n')) : '';
    const label = fieldDisplayLabel(field);
    return `<li role="presentation">
        <button type="button" class="field-picker-option${isSelected ? ' is-active' : ''}" role="option" aria-selected="${isSelected ? 'true' : 'false'}" data-field-option="${escapeAttr(model)}" data-field-value="${escapeAttr(field)}" data-field-examples="${exAttr}">
            <span class="field-picker-option-label">${escapeHtml(label)}</span>
            ${isSelected ? '<span class="field-picker-option-check" aria-hidden="true">✓</span>' : ''}
        </button>
    </li>`;
}
function onAnkiFieldPick(model, field) {
    ankiImport.fieldByModel[model] = field;
    persistAnkiPrefs();
    renderAnkiFieldPickers();
    renderAnkiImportEstimate();
}
// ── Anki import settings popup ─────────────────────────────────────────────
//
// All sync-affecting prefs live in a single popup so the user can tweak them
// from the vocab page (before clicking Sync) OR from step 2 of the import
// modal (during the manual flow). The popup reads the active language's
// prefs each time it opens; change handlers write back to prefs AND to
// ankiImport state so an open import modal sees the new values immediately.
function openAnkiSettingsModal() {
    const modal = document.getElementById('anki-settings-modal');
    if (!modal)
        return;
    renderAnkiSettings();
    modal.classList.remove('hidden');
}
function closeAnkiSettingsModal() {
    document.getElementById('anki-settings-modal')?.classList.add('hidden');
    // If the import modal is open at the fields stage, refresh the estimate
    // - toggle changes might have shifted what gets imported.
    if (ankiImport.open)
        renderAnkiImportEstimate();
}
function renderAnkiSettings() {
    const prefs = loadAnkiPrefs(state.activeLanguage);
    const setCb = (id, checked) => {
        const cb = document.getElementById(id);
        if (cb)
            cb.checked = checked;
    };
    setCb('anki-settings-include-new', prefs.includeNew);
    setCb('anki-settings-include-suspended', prefs.includeSuspended);
    setCb('anki-settings-replace-mode', prefs.replaceMode);
    setCb('anki-settings-preserve-manual', prefs.preserveManualOnReplace);
    setCb('anki-settings-skip-confirm', prefs.replaceConfirmSkip);
    const preserveWrap = document.getElementById('anki-settings-preserve-manual-wrap');
    if (preserveWrap)
        preserveWrap.classList.toggle('expanded', prefs.replaceMode);
}
// Generic helper: update a single boolean pref + mirror it onto ankiImport
// state so any open import modal stays in sync.
function updateAnkiPref(key, value) {
    const prefs = loadAnkiPrefs(state.activeLanguage);
    prefs[key] = value;
    saveAnkiPrefs(state.activeLanguage, prefs);
    // Mirror onto ankiImport so an open import modal's filter / estimate
    // reflect the change without waiting for a re-open.
    if (key === 'includeNew')
        ankiImport.includeNew = value;
    if (key === 'includeSuspended')
        ankiImport.includeSuspended = value;
    if (key === 'replaceMode')
        ankiImport.replaceMode = value;
    if (key === 'preserveManualOnReplace')
        ankiImport.preserveManualOnReplace = value;
    // replaceConfirmSkip lives in prefs only - runAnkiImport reads it
    // directly at confirm time.
}
function onSettingsIncludeNewToggle(checked) {
    updateAnkiPref('includeNew', checked);
}
function onSettingsIncludeSuspendedToggle(checked) {
    updateAnkiPref('includeSuspended', checked);
}
function onSettingsReplaceModeToggle(checked) {
    updateAnkiPref('replaceMode', checked);
    // The "Preserve manually-imported words" sub-option is only relevant
    // when Replace is on - animate it in/out via the .expanded class.
    const wrap = document.getElementById('anki-settings-preserve-manual-wrap');
    if (wrap)
        wrap.classList.toggle('expanded', checked);
}
function onSettingsPreserveManualToggle(checked) {
    updateAnkiPref('preserveManualOnReplace', checked);
}
function onSettingsSkipConfirmToggle(checked) {
    updateAnkiPref('replaceConfirmSkip', checked);
}
// Restore the five behavioural prefs to their out-of-the-box values for the
// active language. Filter / decks / fieldByModel / lastSyncAt are left
// untouched - "Reset defaults" is about the import behaviour, not which
// decks you've picked or whether you've synced before.
function onSettingsResetDefaults() {
    updateAnkiPref('includeNew', false);
    updateAnkiPref('includeSuspended', false);
    updateAnkiPref('replaceMode', false);
    updateAnkiPref('preserveManualOnReplace', true);
    updateAnkiPref('replaceConfirmSkip', false);
    renderAnkiSettings();
    if (ankiImport.open)
        renderAnkiImportEstimate();
}
// Returns the set of notes that would actually be imported given the current
// toggle + field choices. Used both for the estimate (count + unique word
// preview) and the import step itself, so the two can't drift.
function selectedAnkiNotes() {
    return ankiImport.allNotes.filter(note => {
        if (!ankiImport.includeNew && !note.studied)
            return false;
        if (!ankiImport.includeSuspended && note.suspended)
            return false;
        const field = ankiImport.fieldByModel[note.modelName];
        if (!field)
            return false;
        return (note.fields[field] || '').trim() !== '';
    });
}
// Run the same surface-form extraction + dedupe the actual import does. With
// every snapshot already in memory this is a few milliseconds even for many
// thousand notes.
function estimateImportWords(notes) {
    const seen = new Set();
    for (const note of notes) {
        const field = ankiImport.fieldByModel[note.modelName];
        if (!field)
            continue;
        const text = note.fields[field] || '';
        if (!text)
            continue;
        for (const raw of parseKnownWordsInput(text)) {
            const w = cleanAnkiSurfaceForm(raw);
            if (!w)
                continue;
            seen.add(w.toLocaleLowerCase());
        }
    }
    return seen.size;
}
function renderAnkiImportEstimate() {
    const el = document.getElementById('anki-import-estimate');
    if (!el)
        return;
    if (ankiImport.allNotes.length === 0) {
        el.textContent = '';
        return;
    }
    // Three filter steps, each attributed separately so no note ever shows up
    // under more than one reason:
    //   step 1: new-card toggle      → `afterNew`
    //   step 2: suspended toggle     → `afterSuspended`
    //   step 3: field-empty / Skip   → `active`
    const afterNew = ankiImport.allNotes.filter(n => ankiImport.includeNew || n.studied);
    const afterSuspended = afterNew.filter(n => ankiImport.includeSuspended || !n.suspended);
    const active = afterSuspended.filter(n => {
        const field = ankiImport.fieldByModel[n.modelName];
        if (!field)
            return false;
        return (n.fields[field] || '').trim() !== '';
    });
    const newExcluded = ankiImport.allNotes.length - afterNew.length;
    const suspendedExcluded = afterNew.length - afterSuspended.length;
    const fieldSkipped = afterSuspended.length - active.length;
    const words = estimateImportWords(active);
    const detailParts = [];
    if (newExcluded > 0) {
        detailParts.push(`${newExcluded.toLocaleString()} new card${newExcluded === 1 ? '' : 's'} excluded`);
    }
    if (suspendedExcluded > 0) {
        detailParts.push(`${suspendedExcluded.toLocaleString()} suspended card${suspendedExcluded === 1 ? '' : 's'} excluded`);
    }
    if (fieldSkipped > 0) {
        detailParts.push(`${fieldSkipped.toLocaleString()} note${fieldSkipped === 1 ? '' : 's'} with empty or skipped field`);
    }
    const detail = detailParts.length ? ' · ' + detailParts.join(' · ') : '';
    el.textContent = `${active.length.toLocaleString()} note${active.length === 1 ? '' : 's'} → ≈ ${words.toLocaleString()} word${words === 1 ? '' : 's'} to import${detail}`;
}
// ── Field-picker open/close + hover examples ───────────────────────────────
// Look up a field picker by model name without going through a CSS attribute
// selector. Anki ships with model names that contain spaces and parentheses
// (e.g. "Basic (and reversed card)"); CSS.escape on those would emit
// backslash-escapes that the browser then treats literally inside a quoted
// attribute selector, so the lookup silently returns nothing.
function findFieldPicker(model) {
    const els = document.querySelectorAll('[data-field-picker]');
    for (const el of Array.from(els)) {
        if (el.getAttribute('data-field-picker') === model)
            return el;
    }
    return null;
}
function toggleFieldPicker(model) {
    closeAllFieldPickers(model);
    const picker = findFieldPicker(model);
    if (!picker)
        return;
    const menu = picker.querySelector('.field-picker-menu');
    const toggle = picker.querySelector('.field-picker-toggle');
    if (!menu || !toggle)
        return;
    const willOpen = menu.classList.contains('hidden');
    if (willOpen) {
        positionFieldPickerMenu(toggle, menu);
        menu.classList.remove('hidden');
        toggle.setAttribute('aria-expanded', 'true');
    }
    else {
        menu.classList.add('hidden');
        toggle.setAttribute('aria-expanded', 'false');
        hideFieldExamplesTip();
    }
}
// Anchor the fixed-position menu just below its toggle, matched to the
// toggle's width. If the menu would run off the bottom of the viewport,
// flip it above instead.
function positionFieldPickerMenu(toggle, menu) {
    const rect = toggle.getBoundingClientRect();
    menu.style.left = `${rect.left}px`;
    menu.style.width = `${rect.width}px`;
    menu.style.top = `${rect.bottom + 4}px`;
    // Measure with visibility:hidden so the layout cost is paid without a
    // visible flash; then restore. The `hidden` class is left in place - the
    // caller flips it after we return.
    const prevVisibility = menu.style.visibility;
    menu.style.visibility = 'hidden';
    menu.classList.remove('hidden');
    const menuRect = menu.getBoundingClientRect();
    menu.classList.add('hidden');
    menu.style.visibility = prevVisibility;
    if (rect.bottom + 4 + menuRect.height > window.innerHeight - 8) {
        menu.style.top = `${Math.max(8, rect.top - menuRect.height - 4)}px`;
    }
}
function closeAllFieldPickers(except) {
    document.querySelectorAll('[data-field-picker]').forEach(picker => {
        if (except !== undefined && picker.getAttribute('data-field-picker') === except)
            return;
        picker.querySelector('.field-picker-menu')?.classList.add('hidden');
        picker.querySelector('.field-picker-toggle')?.setAttribute('aria-expanded', 'false');
    });
    hideFieldExamplesTip();
}
let fieldExamplesTip = null;
function ensureFieldExamplesTip() {
    if (fieldExamplesTip)
        return fieldExamplesTip;
    fieldExamplesTip = document.createElement('div');
    fieldExamplesTip.className = 'field-examples-tip';
    fieldExamplesTip.setAttribute('role', 'tooltip');
    document.body.appendChild(fieldExamplesTip);
    return fieldExamplesTip;
}
// Position the tooltip to the right of the hovered option so it doesn't
// overlap the dropdown itself. If the menu is too close to the right edge,
// flip the tooltip to the left of the menu.
function showFieldExamplesTip(option) {
    const data = option.getAttribute('data-field-examples') || '';
    if (!data) {
        hideFieldExamplesTip();
        return;
    }
    const examples = data.split('\n').filter(Boolean);
    if (examples.length === 0) {
        hideFieldExamplesTip();
        return;
    }
    const tip = ensureFieldExamplesTip();
    tip.innerHTML = `<div class="field-examples-tip-title">Examples</div>` +
        examples.map(e => `<div class="field-examples-tip-row">${escapeHtml(e)}</div>`).join('');
    // Position relative to the parent picker so the tooltip stays anchored
    // even as the menu scrolls internally.
    const picker = option.closest('[data-field-picker]') || option;
    const rect = picker.getBoundingClientRect();
    tip.style.visibility = 'hidden';
    tip.classList.add('visible');
    const tipRect = tip.getBoundingClientRect();
    const margin = 12;
    const flipLeft = rect.right + tipRect.width + margin > window.innerWidth;
    if (flipLeft) {
        tip.style.left = `${rect.left - tipRect.width - margin}px`;
    }
    else {
        tip.style.left = `${rect.right + margin}px`;
    }
    // Vertically align with the hovered option, but clamp to viewport.
    const optionRect = option.getBoundingClientRect();
    let top = optionRect.top;
    const maxTop = window.innerHeight - tipRect.height - 8;
    if (top > maxTop)
        top = Math.max(8, maxTop);
    tip.style.top = `${top}px`;
    tip.style.visibility = '';
}
function hideFieldExamplesTip() {
    fieldExamplesTip?.classList.remove('visible');
}
// Step 3: actually run the import. For each selected deck, fetch all note IDs,
// then notesInfo in chunks (AnkiConnect handles big batches but chunking lets
// us update the progress bar). For each note, look up the field its model
// maps to (or skip if "Skip"). Extract the field text, run through the same
// parser as the textbox import so multi-word fields split sensibly.
async function runAnkiImport() {
    const sessionID = ankiImport.sessionID;
    const runBtn = document.getElementById('anki-import-run');
    const backBtn = document.getElementById('anki-import-back');
    const detail = document.getElementById('anki-import-running-detail');
    const msg = document.getElementById('anki-import-running-msg');
    const bar = document.getElementById('anki-import-progress-bar');
    const doneActions = document.getElementById('anki-import-done-actions');
    // Skip-only sanity check.
    const usedModels = ankiImport.models.filter(m => (ankiImport.fieldByModel[m] || '').trim() !== '');
    if (usedModels.length === 0) {
        showToast('Pick a field for at least one card type, or set all to Skip and cancel.', 'error');
        return;
    }
    // Replace-mode confirmation: a destructive operation that deletes lemmas
    // not in the new selection (textbox/file-added lemmas survive when
    // preserve-manual mode is on, so the copy is keyed to that setting).
    // Skipped on a per-language basis once the user has explicitly checked
    // "Don't show this again" on the dialog. Also skipped when the
    // quick-sync flow has already shown its own status-bearing version of
    // the same dialog (replaceConfirmedThisRun).
    if (ankiImport.replaceMode && !ankiImport.replaceConfirmedThisRun) {
        const langName = languageName(ankiImport.lang);
        const prefs = loadAnkiPrefs(ankiImport.lang);
        if (!prefs.replaceConfirmSkip) {
            const result = await showConfirmWithRemember({
                title: `Replace ${langName} vocabulary?`,
                message: ankiReplaceConfirmMessage(langName, ankiImport.preserveManualOnReplace),
                confirmLabel: 'Sync and replace',
                danger: true,
                rememberLabel: "Don't show this again",
            });
            if (!result.confirmed)
                return;
            if (result.remember)
                recordReplaceConfirmSkip();
        }
    }
    if (isAnkiImportCancelled(sessionID))
        return;
    // Reset for the next run regardless of which path we took.
    ankiImport.replaceConfirmedThisRun = false;
    const abortController = new AbortController();
    ankiImport.abortController?.abort();
    ankiImport.abortController = abortController;
    if (runBtn)
        runBtn.disabled = true;
    if (backBtn)
        backBtn.disabled = true;
    showAnkiStage('running');
    if (msg)
        msg.textContent = 'Fetching notes…';
    if (bar)
        bar.style.width = '0%';
    if (doneActions)
        doneActions.classList.add('hidden');
    persistAnkiPrefs();
    try {
        // The note snapshots - including the studied/new flag - were fetched
        // during discovery, so we don't hit Anki again here. The filter
        // applies both the toggle and the per-model field choice.
        const notes = selectedAnkiNotes();
        if (notes.length === 0) {
            if (msg)
                msg.textContent = 'No notes match the current selection.';
            if (detail) {
                // Diagnose which filter is responsible so the user knows
                // which toggle (or field choice) to revisit.
                const hasNew = ankiImport.allNotes.some(n => !n.studied);
                const hasSuspended = ankiImport.allNotes.some(n => n.suspended);
                if (ankiImport.allNotes.length === 0) {
                    detail.textContent = 'Selected decks are empty.';
                }
                else if (!ankiImport.includeNew && hasNew && !ankiImport.allNotes.some(n => n.studied)) {
                    detail.textContent = 'Every note in the selected decks is still "new" - turn on “Mark new cards as known” to import them.';
                }
                else if (!ankiImport.includeSuspended && hasSuspended && !ankiImport.allNotes.some(n => !n.suspended)) {
                    detail.textContent = 'Every note in the selected decks is suspended - turn on “Include suspended cards” to import them.';
                }
                else {
                    detail.textContent = 'No notes had a non-empty value for the chosen fields.';
                }
            }
            if (doneActions)
                doneActions.classList.remove('hidden');
            return;
        }
        // Process snapshots in chunks so the progress bar still moves on
        // very large imports. The work is in-memory and fast - the chunk
        // boundary mostly exists to keep the UI thread responsive.
        const CHUNK = 500;
        const seen = new Set();
        const words = [];
        for (let i = 0; i < notes.length; i += CHUNK) {
            if (isAnkiImportCancelled(sessionID))
                return;
            const chunk = notes.slice(i, i + CHUNK);
            for (const note of chunk) {
                const field = ankiImport.fieldByModel[note.modelName];
                if (!field)
                    continue;
                const text = note.fields[field] || '';
                if (!text)
                    continue;
                for (const raw of parseKnownWordsInput(text)) {
                    const w = cleanAnkiSurfaceForm(raw);
                    if (!w)
                        continue;
                    const key = w.toLocaleLowerCase();
                    if (seen.has(key))
                        continue;
                    seen.add(key);
                    words.push(w);
                }
            }
            const processed = Math.min(i + CHUNK, notes.length);
            const pct = Math.round((processed / notes.length) * 100);
            if (bar)
                bar.style.width = `${pct}%`;
            if (detail)
                detail.textContent = `${processed.toLocaleString()} / ${notes.length.toLocaleString()} notes processed`;
            // Yield to the event loop so the progress bar repaints.
            await new Promise(r => setTimeout(r, 0));
            if (isAnkiImportCancelled(sessionID))
                return;
        }
        if (words.length === 0) {
            if (msg)
                msg.textContent = 'No words extracted.';
            if (detail)
                detail.textContent = 'The chosen fields were empty for every note in the selected decks.';
            if (doneActions)
                doneActions.classList.remove('hidden');
            return;
        }
        if (msg)
            msg.textContent = ankiImport.replaceMode
                ? `Syncing ${words.length.toLocaleString()} words…`
                : `Importing ${words.length.toLocaleString()} words…`;
        if (isAnkiImportCancelled(sessionID))
            return;
        if (ankiImport.replaceMode) {
            // scope='anki' (default) preserves words the user added through
            // the textbox / file / inspect / review flows; scope='all' wipes
            // every row not in the new Anki state.
            const scope = ankiImport.preserveManualOnReplace ? 'anki' : 'all';
            const data = await putKnownWords(words, scope, ankiImport.lang, abortController.signal);
            if (isAnkiImportCancelled(sessionID))
                return;
            renderKnownWordsUnresolved(data.unresolved || []);
            if (msg)
                msg.textContent = 'Done.';
            if (detail)
                detail.textContent = describeReplaceResult(data);
            await refreshDashboardData();
            await loadKnownWords();
            showToast('Vocabulary synced from Anki.', 'success');
        }
        else {
            // Additive Anki import - tag new rows so a later sync can diff
            // them. Manual rows (textbox/file/inspect/review) keep their
            // own source.
            const data = await postKnownWords(words, 'anki', ankiImport.lang, abortController.signal);
            if (isAnkiImportCancelled(sessionID))
                return;
            renderKnownWordsUnresolved(data.unresolved || []);
            if (msg)
                msg.textContent = 'Done.';
            if (detail)
                detail.textContent = describeImportResult(data);
            await refreshDashboardData();
            await loadKnownWords();
            showToast('Known words imported from Anki.', 'success');
        }
        // Record the successful run so the vocab page can offer the
        // one-click "Sync from Anki" shortcut next time.
        recordAnkiSyncTime();
        renderVocabAnkiSyncButton();
        if (doneActions)
            doneActions.classList.remove('hidden');
    }
    catch (err) {
        if (isAnkiImportCancelled(sessionID) || err?.name === 'AbortError')
            return;
        if (msg)
            msg.textContent = err.message || 'Import failed.';
        showToast(err.message || 'Failed to import from Anki.', 'error');
        if (doneActions)
            doneActions.classList.remove('hidden');
    }
    finally {
        if (ankiImport.abortController === abortController)
            ankiImport.abortController = null;
        if (!isAnkiImportCancelled(sessionID)) {
            if (runBtn)
                runBtn.disabled = false;
            if (backBtn)
                backBtn.disabled = false;
        }
    }
}
function openAnkiSetupModal() {
    const modal = document.getElementById('anki-setup-modal');
    if (!modal)
        return;
    // OS-aware Add-ons shortcut. Anki uses Cmd on macOS, Ctrl elsewhere.
    const shortcut = document.getElementById('anki-setup-shortcut');
    if (shortcut) {
        const isMac = /Mac|iPhone|iPad/.test(navigator.platform);
        shortcut.textContent = isMac ? '⌘+Shift+A' : 'Ctrl+Shift+A';
    }
    modal.classList.remove('hidden');
    document.getElementById('anki-setup-modal-done')?.focus();
}
function closeAnkiSetupModal() {
    document.getElementById('anki-setup-modal')?.classList.add('hidden');
}
async function copyAnkiSetupConfig() {
    const source = document.getElementById('anki-setup-copy-source');
    const button = document.getElementById('anki-setup-copy-btn');
    if (!source || !button)
        return;
    const text = source.textContent || '';
    try {
        await navigator.clipboard.writeText(text);
        const original = button.textContent || 'Copy';
        button.textContent = 'Copied!';
        button.disabled = true;
        setTimeout(() => {
            button.textContent = original;
            button.disabled = false;
        }, 1500);
    }
    catch {
        showToast('Could not access the clipboard - copy manually.', 'error');
    }
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
async function deleteAllKnownWords() {
    const lang = state.activeLanguage;
    const langName = languageName(lang);
    const count = state.knownWords.length;
    if (count === 0) {
        showToast(`No ${langName} vocabulary to delete.`, 'info');
        return;
    }
    const confirmed = await showConfirm({
        title: `Delete all ${langName} vocabulary?`,
        message: `This removes all ${count.toLocaleString()} known ${langName} words from your account. Deck coverage and review selection will reset to "nothing known". This cannot be undone.`,
        confirmLabel: 'Delete all',
        danger: true,
    });
    if (!confirmed)
        return;
    const button = document.getElementById('vocab-delete-all');
    if (button)
        button.disabled = true;
    try {
        const params = new URLSearchParams({ lang, all: '1' });
        const resp = await fetch(`/api/known-words?${params.toString()}`, {
            method: 'DELETE',
            credentials: 'same-origin',
        });
        if (!resp.ok)
            throw new Error(await resp.text() || 'Failed to delete vocabulary');
        state.knownWords = [];
        renderKnownWordsPanel();
        renderKnownWordsUnresolved([]);
        await refreshDashboardData();
        showToast(`${langName} vocabulary cleared.`, 'success');
    }
    catch (err) {
        showToast(err.message || 'Failed to delete vocabulary.', 'error');
    }
    finally {
        if (button)
            button.disabled = false;
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
// The active text-size cap for a form: the anonymous demo cap for the landing
// form, the full signed-in ceiling everywhere else.
function formMaxChars(els) {
    return els.anonCapped ? state.anonMaxChars : MAX_CHARS;
}
// Inspect's "language" is the site-wide active language - there's no
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
// The anonymous landing demo is paste-only (no file upload, no EPUB) - file
// upload is a signed-in Inspect capability (USER_FLOWS §1). The shared
// ParseFormElements interface still wants dropzone/file/pill/chapter nodes, so
// we hand it detached stubs the landing path never wires or shows.
let landingStubNodes = null;
function landingStubs() {
    if (!landingStubNodes) {
        landingStubNodes = {
            file: document.createElement('input'),
            dropzone: document.createElement('div'),
            pill: document.createElement('div'),
            chap: document.createElement('div'),
        };
    }
    return landingStubNodes;
}
function getLandingEls() {
    const lang = bindBtnRadio('landing-lang');
    const text = document.getElementById('landing-text');
    const cc = document.getElementById('landing-char-count');
    const warn = document.getElementById('landing-lang-warning');
    const swBtn = document.getElementById('landing-lang-switch');
    if (!lang || !text || !cc || !warn || !swBtn)
        return null;
    const stubs = landingStubs();
    return {
        lang, text, file: stubs.file, charCount: cc, warning: warn, switchBtn: swBtn,
        dropzone: stubs.dropzone, loadedPill: stubs.pill, chapterList: stubs.chap,
        loadedEpub: null, anonCapped: true, submitBtnId: 'landing-submit',
    };
}
function initLandingForm() {
    const els = getLandingEls();
    if (!els)
        return;
    updateCharCount(els);
    els.text.addEventListener('input', () => {
        updateCharCount(els);
        updateLangWarning(els, true);
        autoGrowTextarea(els.text);
    });
    autoGrowTextarea(els.text);
    window.addEventListener('resize', () => autoGrowTextarea(els.text));
    els.lang.addEventListener('change', () => updateLangWarning(els, true));
    els.switchBtn.addEventListener('click', () => {
        const ws = getLangWarningState(effectiveSourceText(els), els.lang.value);
        if (ws.canSwitch) {
            els.lang.value = ws.detected;
            updateLangWarning(els, true);
        }
    });
    const form = document.getElementById('landing-form');
    form?.addEventListener('submit', async (e) => {
        e.preventDefault();
        await runParse(els, 'custom', 'landing', 'landing-submit');
    });
    initLandingDemoChips(els);
}
// Landing "or try →" demo chips. Each chip pulls one curated embedded text from
// the anonymous /api/demo/text/{id} allowlist, drops it into the paste box, sets
// the FI/ET selector to the text's language, and scrolls the box into view. It
// deliberately does NOT auto-parse - the visitor sees the text land in the box
// (and the char meter tick up) before pressing Parse, matching the prototype.
function initLandingDemoChips(els) {
    const chips = document.querySelectorAll('.demo-chip[data-demo-id]');
    chips.forEach(chip => {
        chip.addEventListener('click', async () => {
            const id = chip.dataset.demoId;
            if (!id)
                return;
            const origLabel = chip.textContent || '';
            chip.disabled = true;
            chip.textContent = 'Loading…';
            try {
                const resp = await fetch(`/api/demo/text/${encodeURIComponent(id)}`, { credentials: 'same-origin' });
                if (!resp.ok) {
                    showToast('Could not load that sample. Please try another.', 'error');
                    return;
                }
                const data = await resp.json();
                const lang = (data.language || chip.dataset.demoLang || 'FI').toUpperCase();
                els.lang.value = lang === 'ET' ? 'ET' : 'FI';
                els.text.value = data.text || '';
                // Drive the same input path a paste would, so the char meter,
                // language-warning gate, and Parse-button enable/disable all
                // update exactly as if the visitor had pasted the text.
                els.text.dispatchEvent(new Event('input', { bubbles: true }));
                const box = document.getElementById('landing-paste-box');
                (box || els.text).scrollIntoView({ behavior: 'smooth', block: 'center' });
                els.text.focus();
            }
            catch {
                showToast('Could not load that sample. Please try another.', 'error');
            }
            finally {
                chip.disabled = false;
                chip.textContent = origLabel;
            }
        });
    });
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
    // When an EPUB is held, the textarea is empty - count from the loaded
    // book's totalChars so the user sees the real size, not "0 / 1,000,000".
    const count = els.loadedEpub
        ? els.loadedEpub.totalChars
        : els.text.value.length;
    const cap = formMaxChars(els);
    els.charCount.textContent = `${count.toLocaleString()} / ${cap.toLocaleString()}`;
    els.charCount.classList.toggle('char-count-warn', count > cap * 0.9);
    els.charCount.classList.toggle('char-count-over', count > cap);
}
// autoGrowTextarea grows a textarea to fit its content instead of scrolling
// inside a fixed small box while the page below sits empty (the owner's "why am
// I scrolling if there's space on the page" complaint). It grows up to ~70vh,
// then lets the textarea scroll internally for very long pastes. Driven on input
// and on load; a manual vertical resize (resize: vertical stays on) persists
// until the next keystroke, so we don't fight the user's explicit drag.
function autoGrowTextarea(ta) {
    // Reset first so shrinking works when text is deleted, then measure the
    // content's natural height from scrollHeight.
    ta.style.height = 'auto';
    const maxPx = Math.round(window.innerHeight * 0.7);
    const next = Math.min(ta.scrollHeight, maxPx);
    ta.style.height = `${next}px`;
    // Only scroll internally once we've hit the cap; below it there's no
    // overflow so the box shows all content with room to spare.
    ta.style.overflowY = ta.scrollHeight > maxPx ? 'auto' : 'hidden';
}
// Text the parser will actually see - the held EPUB when one is loaded, else
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
            gateSubmit(els.submitBtnId || 'inspect-submit', false);
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
        gateSubmit(els.submitBtnId || 'inspect-submit', ws.blocksParse);
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
            : '- lemmas';
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
            // Chapter parses are auxiliary data - no parse_sessions row.
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
// before the upcoming paint, the second fires before the paint AFTER that -
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
// and surfaced as a pill - the textarea is left empty and disabled until the
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
            showToast(`EPUB is large - kept the first ${MAX_CHARS.toLocaleString()} characters for analysis.`, 'info');
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
    // .txt / .md and unknown extensions - read client-side and populate the
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
// unconditional - gating it on a types.includes('Files') check is unreliable
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
        // Only clear when the cursor actually leaves the dropzone - not when
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
// that looks like the other language we still auto-switch the radio - it
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
            showToast(`Switched to ${detected === 'FI' ? 'Finnish' : 'Estonian'} - detected from ${sourceLabel}`, 'info');
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
        autoGrowTextarea(els.text);
        // Hide the cold-start catalog once the learner has text to parse.
        const section = document.getElementById('inspect-catalog-section');
        if (section)
            section.classList.toggle('hidden', els.text.value.trim() !== '');
    });
    autoGrowTextarea(els.text);
    window.addEventListener('resize', () => autoGrowTextarea(els.text));
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
// ── Workbench form (admin surface - keeps prior behavior) ──────────────────
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
    const cap = formMaxChars(els);
    if (text.length > cap) {
        if (els.anonCapped) {
            showToast(`Anonymous parsing is limited to ${cap.toLocaleString()} characters. Sign up to parse longer texts.`, 'error');
        }
        else {
            showToast(`Text must be ${cap.toLocaleString()} characters or fewer.`, 'error');
        }
        return;
    }
    if (ws.blocksParse)
        return;
    // A fresh anonymous parse re-shows the sign-up ribbon even if the visitor
    // dismissed it on the previous results view (USER_FLOWS §2).
    if (context === 'landing')
        state.anonRibbonDismissed = false;
    const activeBtn = document.getElementById(activeBtnId);
    const origLabel = activeBtn?.textContent || '';
    // Disable all parse buttons in the current form
    if (context === 'workbench')
        setParseButtonsDisabled(true);
    if (activeBtn) {
        activeBtn.disabled = true;
        activeBtn.textContent = 'Parsing…';
    }
    // Fresh top-level parse - drop any per-chapter cache from a previous
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
            // The anonymous over-cap path returns a JSON body with an `error`
            // field; older/plain errors return text. Prefer the JSON message.
            const raw = await resp.text();
            let msg = raw;
            try {
                const parsed = JSON.parse(raw);
                if (parsed && typeof parsed.error === 'string')
                    msg = parsed.error;
            }
            catch { /* not JSON - use raw text */ }
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
// rankRevealUnlocks orders the unknown words by token mass exactly as
// DeckComprehension's SQL does (COUNT(*) DESC, lemma ASC, pos ASC), so the
// projection lines up with what the saved-deck comprehension endpoint would
// later report for the same words.
function rankRevealUnlocks(unknown) {
    return [...unknown].sort((a, b) => {
        if (a.count !== b.count)
            return b.count - a.count;
        const byLemma = compareStrings(a.lemma, b.lemma);
        if (byLemma !== 0)
            return byLemma;
        return compareStrings(a.pos, b.pos);
    });
}
function computeCoverageReveal(data) {
    const anonymous = state.role === 'anon';
    let totalMass = 0;
    let coveredMass = 0;
    const unknown = [];
    for (const word of data.words) {
        const count = word.count;
        if (count <= 0)
            continue;
        totalMass += count;
        // Signed-in: covered = known OR ignored, read through the live map so
        // in-session changes are reflected. Anonymous has no known state, so
        // every word is an unlock candidate and coverage starts at zero.
        const stateNow = anonymous ? undefined : currentLemmaState(word.lemma, word.pos);
        if (stateNow === 'known' || stateNow === 'ignored') {
            coveredMass += count;
        }
        else {
            unknown.push({ lemma: word.lemma, pos: word.pos, count });
        }
    }
    const knownPct = totalMass === 0 ? 0 : Math.round((coveredMass / totalMass) * 100);
    const ranked = rankRevealUnlocks(unknown);
    const massForTopN = (n) => {
        let sum = 0;
        for (let i = 0; i < n && i < ranked.length; i++)
            sum += ranked[i].count;
        return sum;
    };
    // Choose the step size. When there are 10 or fewer unknowns, offer exactly
    // those - a clean "learn these and you're done" ask. When there are more,
    // the step is 10 or 20 (never an in-between count): prefer the smaller ask
    // of 10, and only escalate to 20 when the full set of 20 exists AND the
    // extra 10 words buy a materially larger coverage jump (≥5 more points of
    // total token mass). This keeps the projection honest and the ask minimal.
    let unlockCount;
    let unlockMass;
    if (ranked.length <= 10) {
        unlockCount = ranked.length;
        unlockMass = massForTopN(ranked.length);
    }
    else {
        const mass10 = massForTopN(10);
        unlockCount = 10;
        unlockMass = mass10;
        if (ranked.length >= 20) {
            const mass20 = massForTopN(20);
            const extraPoints = totalMass === 0 ? 0 : ((mass20 - mass10) / totalMass) * 100;
            if (extraPoints >= 5) {
                unlockCount = 20;
                unlockMass = mass20;
            }
        }
    }
    const projectedMass = coveredMass + unlockMass;
    const projectedPct = totalMass === 0 ? 0 : Math.round((projectedMass / totalMass) * 100);
    // "estimated" flags the copy to hedge (≈/roughly): true whenever the shown
    // whole-percent hides a fraction, i.e. the ratio isn't exact.
    const exact = totalMass !== 0
        && (coveredMass * 100) % totalMass === 0
        && (projectedMass * 100) % totalMass === 0;
    return {
        anonymous,
        totalMass,
        coveredMass,
        knownPct,
        unlockCount,
        unlockMass,
        projectedPct,
        estimated: !exact,
    };
}
// renderCoverageReveal fills the #coverage-reveal panel and runs the count-up +
// bar-fill animation. It is idempotent per parse: showResults clears any prior
// run before calling it, and the animation cancels its own frame on re-entry.
let coverageRevealRaf = 0;
function renderCoverageReveal(data, animate = true) {
    const host = document.getElementById('coverage-reveal');
    if (!host)
        return;
    if (coverageRevealRaf) {
        cancelAnimationFrame(coverageRevealRaf);
        coverageRevealRaf = 0;
    }
    // Deck-detail view has its own comprehension panel; the reveal is for the
    // fresh-parse moment (inspect / landing). No mass means nothing honest to
    // show. Hide in both cases.
    const model = computeCoverageReveal(data);
    if (state.currentContext === 'deck' || model.totalMass === 0) {
        host.classList.add('hidden');
        host.innerHTML = '';
        return;
    }
    const approx = model.estimated ? '≈' : '';
    // knownPct is the bar floor and, for signed-in, the count-up target - the
    // "you already know X%" number. The gain segment previews the projected
    // lift to Y%. For anonymous there is no known floor (0), so the count-up
    // target is the frequency figure Z% itself.
    const knownPct = model.anonymous ? 0 : model.knownPct;
    const projectedPct = model.projectedPct;
    const gainWidth = Math.max(0, projectedPct - knownPct);
    const figureTarget = model.anonymous ? projectedPct : knownPct;
    let headline;
    let projection;
    if (model.anonymous) {
        // Projection-from-zero: no known state exists, so the honest reveal is
        // frequency-based. Doubles as the signup hook (ribbon follows).
        headline = `The ${model.unlockCount} most frequent words in this text carry `
            + `<strong class="coverage-reveal-figure" id="coverage-reveal-figure">${approx}${figureTarget}%</strong> of it`;
        projection = `Learn those first to read most of what is here. `
            + `An account tracks the words you already know and shows how much of any text you will understand.`;
    }
    else {
        headline = `You already know <strong class="coverage-reveal-figure" id="coverage-reveal-figure">`
            + `${approx}${figureTarget}%</strong> of this text`;
        projection = model.unlockCount > 0 && projectedPct > knownPct
            ? `Learn the top <strong>${model.unlockCount}</strong> words `
                + `<span aria-hidden="true">→</span> ${approx}${projectedPct}%`
            : `You know every word here that this parse can attach.`;
    }
    host.innerHTML = `
        <div class="coverage-reveal-inner">
            <p class="coverage-reveal-headline">${headline}</p>
            <div class="coverage-reveal-bar" role="img"
                 aria-label="${model.anonymous
        ? `Roughly ${projectedPct} percent of tokens are carried by the ${model.unlockCount} most frequent words`
        : `You know roughly ${knownPct} percent of this text; learning ${model.unlockCount} more words reaches roughly ${projectedPct} percent`}">
                <div class="coverage-reveal-known" id="coverage-reveal-known"></div>
                <div class="coverage-reveal-gain" id="coverage-reveal-gain"></div>
            </div>
            <p class="coverage-reveal-projection">${projection}</p>
        </div>`;
    host.classList.remove('hidden');
    const figureEl = document.getElementById('coverage-reveal-figure');
    const knownBar = document.getElementById('coverage-reveal-known');
    const gainBar = document.getElementById('coverage-reveal-gain');
    if (!figureEl || !knownBar || !gainBar)
        return;
    const setBars = (knownW, gainW) => {
        knownBar.style.width = `${knownW}%`;
        gainBar.style.width = `${gainW}%`;
    };
    const setFigure = (pct) => {
        figureEl.textContent = `${approx}${pct}%`;
    };
    const reduce = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches;
    if (reduce || !animate) {
        // Collapse to the final state instantly: reduced-motion users, and
        // in-session refreshes (a known/ignore toggle updates the numbers
        // without re-playing the entrance animation).
        setBars(knownPct, gainWidth);
        setFigure(figureTarget);
        return;
    }
    // Count-up the headline figure to figureTarget while the bar fills from 0 to
    // its known floor + projected gain, ~1.2s ease-out. The full bar animates
    // from the known level up to the projected level - the preview of the lift.
    const durationMs = 1200;
    setBars(0, 0);
    setFigure(0);
    const t0 = performance.now();
    const easeOut = (t) => 1 - Math.pow(1 - t, 3);
    const tick = (now) => {
        const raw = Math.min(1, (now - t0) / durationMs);
        const e = easeOut(raw);
        const knownNow = knownPct * e;
        const gainNow = gainWidth * e;
        setBars(knownNow, gainNow);
        // The headline figure counts to figureTarget - the number the user is
        // meant to feel - settling exactly on the API-derived value.
        setFigure(Math.round(figureTarget * e));
        if (raw < 1) {
            coverageRevealRaf = requestAnimationFrame(tick);
        }
        else {
            setBars(knownPct, gainWidth);
            setFigure(figureTarget);
            coverageRevealRaf = 0;
        }
    };
    coverageRevealRaf = requestAnimationFrame(tick);
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
// senseKey identifies a selected study sense within the current parse.
const SENSE_KEY_SEP = '\x00';
function senseKey(surface, lemma, pos) {
    return [surface, lemma, pos].join(SENSE_KEY_SEP);
}
// ambiguousSurfacesForRow returns the ambiguous surfaces (with candidates) that
// this results row's forms touch. A row aggregates by (lemma, pos) but the chip
// is per surface, so a row can surface more than one ambiguous form.
function ambiguousSurfacesForRow(w) {
    if (state.ambiguityBySurface.size === 0)
        return [];
    const out = [];
    const seen = new Set();
    for (const form of w.forms) {
        if (seen.has(form))
            continue;
        const amb = state.ambiguityBySurface.get(form);
        if (amb) {
            seen.add(form);
            out.push(amb);
        }
    }
    return out;
}
// renderAmbiguityPanel builds the expanded "Multiple possible meanings" content
// for one ambiguous surface: the sentence context, the candidate meanings, and
// per-candidate actions. Copy is verbatim from USER_FLOWS §5 / srs-deck-spec.
function renderAmbiguityPanel(lang, amb) {
    const example = amb.example
        ? `<p class="ambiguity-example">${highlightFormsInSentence(amb.example, [amb.surface])}</p>`
        : '';
    const rows = amb.candidates.map(c => {
        const key = senseKey(amb.surface, c.lemma, c.pos);
        const isKnown = currentLemmaState(c.lemma, c.pos) === 'known';
        const isStudy = state.selectedSenses.has(key);
        const pending = state.ambiguityKnownPending.has(key);
        const glossHtml = c.gloss
            ? escapeHtml(c.gloss)
            : '<span class="no-gloss">meaning unavailable</span>';
        const posPill = `<span class="pos-pill" data-pos="${escapeHtml(c.pos)}" data-tooltip="${escapeHtml(posLabel(c.pos))}">${escapeHtml(posAbbrev(c.pos))}</span>`;
        // "Study this meaning" helper copy is fixed by the grill in parse
        // results: "Creates a review card when you save."
        return `<div class="ambiguity-candidate${isKnown ? ' is-known' : ''}"
                data-surface="${escapeAttr(amb.surface)}"
                data-lemma="${escapeAttr(c.lemma)}"
                data-pos="${escapeAttr(c.pos)}">
            <div class="ambiguity-candidate-head">
                <span class="ambiguity-candidate-lemma">${escapeHtml(c.lemma)}</span>
                ${posPill}
                <span class="ambiguity-candidate-gloss">${glossHtml}</span>
            </div>
            <div class="ambiguity-candidate-actions">
                <button type="button" class="ambiguity-know-btn${isKnown ? ' is-active' : ''}"
                    data-ambiguity-action="know"
                    ${pending || isKnown ? 'disabled' : ''}>
                    ${isKnown ? 'You know this meaning' : 'I know this meaning'}
                </button>
                <button type="button" class="ambiguity-study-btn${isStudy ? ' is-active' : ''}"
                    data-ambiguity-action="study"
                    ${isKnown ? 'disabled' : ''}
                    data-tooltip="Creates a review card when you save.">
                    ${isStudy ? 'Selected to study' : 'Study this meaning'}
                </button>
                <button type="button" class="ambiguity-notsure-btn${isStudy ? ' is-active' : ''}"
                    data-ambiguity-action="notsure"
                    ${isKnown ? 'disabled' : ''}>
                    Not sure
                </button>
            </div>
        </div>`;
    }).join('');
    return `<div class="ambiguity-panel" data-ambiguity-panel="${escapeAttr(amb.surface)}">
        <p class="ambiguity-panel-title">Multiple possible meanings for “${escapeHtml(amb.surface)}”</p>
        ${example}
        <div class="ambiguity-candidates">${rows}</div>
        <div class="ambiguity-panel-foot">
            <span class="ambiguity-study-hint">Creates a review card when you save.</span>
            <button type="button" class="ambiguity-flag-btn"
                data-ambiguity-action="flag"
                data-surface="${escapeAttr(amb.surface)}">None of these looks right</button>
        </div>
    </div>`;
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
        // Multiple-possible-meanings chip (signed-in only). One chip per
        // ambiguous surface this row touches; expanding shows the meaning check.
        const ambRows = showActions ? ambiguousSurfacesForRow(w) : [];
        const ambChips = ambRows.map(amb => {
            const expanded = state.ambiguityExpanded.has(amb.surface);
            return `<button type="button" class="ambiguity-chip${expanded ? ' is-open' : ''}"
                data-ambiguity-chip="${escapeAttr(amb.surface)}"
                aria-expanded="${expanded ? 'true' : 'false'}">
                ${expanded ? '▾' : '▸'} Multiple possible meanings</button>`;
        }).join('');
        const ambPanels = ambRows
            .filter(amb => state.ambiguityExpanded.has(amb.surface))
            .map(amb => `<tr class="ambiguity-row"><td class="col-row"></td><td colspan="4">${renderAmbiguityPanel(data.lang, amb)}</td></tr>`)
            .join('');
        return `<tr class="word-row">
            <td class="col-row">${index + 1}</td>
            <td class="col-lemma">
                <div class="lemma-pos-grid">
                    <span class="lemma-side">${escapeHtml(w.lemma)}${grammarBadge}</span>
                    <span class="pos-side">${posPill}</span>
                </div>
                ${exampleToggle}
                ${ambChips}
                ${exampleBlock}
            </td>
            <td class="col-def">${glossHtml}</td>
            <td class="col-count">${w.count}</td>
            ${actionCell}
        </tr>${ambPanels}`;
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
    wireAmbiguityControls(tbody, data);
}
// wireAmbiguityControls attaches the chip-toggle and per-candidate action
// handlers for the Multiple-possible-meanings flow. Re-run on every table
// render because innerHTML replacement drops listeners.
function wireAmbiguityControls(tbody, data) {
    tbody.querySelectorAll('.ambiguity-chip').forEach(btn => {
        btn.addEventListener('click', () => {
            const surface = btn.dataset.ambiguityChip || '';
            if (!surface)
                return;
            if (state.ambiguityExpanded.has(surface)) {
                state.ambiguityExpanded.delete(surface);
            }
            else {
                state.ambiguityExpanded.add(surface);
            }
            renderResultsTable(data);
        });
    });
    tbody.querySelectorAll('[data-ambiguity-action]').forEach(btn => {
        btn.addEventListener('click', () => {
            const action = btn.dataset.ambiguityAction;
            if (action === 'flag') {
                const surface = btn.dataset.surface || '';
                // "None of these looks right" is parser feedback, never a
                // study/known action. Open the flag-only correction path with
                // the surface + first-occurrence context prefilled.
                const amb = state.ambiguityBySurface.get(surface);
                openCorrectionModal({
                    lemma: amb?.candidates[0]?.lemma || surface,
                    pos: amb?.candidates[0]?.pos || 'X',
                    surface,
                    occurrence: 0,
                    original_grammar_label: '',
                }, { forceFlagOnly: true });
                return;
            }
            const holder = btn.closest('.ambiguity-candidate');
            if (!holder)
                return;
            const surface = holder.dataset.surface || '';
            const lemma = holder.dataset.lemma || '';
            const pos = holder.dataset.pos || '';
            if (action === 'know') {
                void ambiguityMarkKnown(data.lang, surface, lemma, pos);
            }
            else if (action === 'study' || action === 'notsure') {
                // "Not sure" behaves conservatively like study: keep selected.
                state.selectedSenses.add(senseKey(surface, lemma, pos));
                refreshResultsViews();
                // Keep an open reading popover (which reuses this panel) in step.
                if (state.readPopoverSurface === surface) {
                    const body = document.getElementById('read-popover-body');
                    if (body) {
                        body.innerHTML = renderReadPopoverBody(surface, data);
                        wireReadPopoverControls(body, data);
                    }
                }
            }
        });
    });
}
// ambiguityMarkKnown records a resolved (lemma, pos) as known via the existing
// lemma-state endpoint (current known model is (lemma, pos)-backed), updates
// coverage in place, and excludes that sense from the pending deck selection.
async function ambiguityMarkKnown(lang, surface, lemma, pos) {
    const key = senseKey(surface, lemma, pos);
    if (state.ambiguityKnownPending.has(key))
        return;
    state.ambiguityKnownPending.add(key);
    if (state.currentResults)
        renderResultsTable(state.currentResults);
    try {
        const resp = await fetch('/api/lemma-state', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'same-origin',
            body: JSON.stringify({ lang, lemma, pos, status: 'known' }),
        });
        if (!resp.ok)
            throw new Error(await resp.text() || 'Failed to update word');
        state.currentLemmaStates.set(lemmaStateKey(lang, lemma, pos), 'known');
        // A known sense is no longer a study candidate.
        state.selectedSenses.delete(key);
        await refreshDashboardData({ rerenderRoute: false });
        showToast('Meaning marked known.', 'success');
    }
    catch (err) {
        showToast(err.message || 'Failed to update word.', 'error');
    }
    finally {
        state.ambiguityKnownPending.delete(key);
        refreshResultsViews();
        // If the reading popover is open on this surface, re-render its body so
        // the just-marked sense reflects known state.
        if (state.readPopoverSurface === surface && state.currentResults) {
            const body = document.getElementById('read-popover-body');
            if (body) {
                body.innerHTML = renderReadPopoverBody(surface, state.currentResults);
                wireReadPopoverControls(body, state.currentResults);
            }
        }
    }
}
// ── Reading surface (Read / Words tabs, the living text) ───────────────────
//
// The Read view is the text-first inversion of results: it renders the source
// text (state.currentSourceText) with paragraph structure preserved, and colors
// every parsed token by its live learner state. Words is the existing lemma
// table, untouched. The two tabs share one parse; the Read view derives its
// per-token state from the same currentLemmaStates / selectedSenses model the
// table uses, so a known/ignore/study action from either tab (or the popover)
// updates both through refreshResultsViews().
//
// No new server data: WordEntry.forms carries the exact (case-preserved) surface
// strings each (lemma, pos) resolved from, so we tokenize the source text on the
// client and match each surface back to its WordEntry(ies). A surface that maps
// to more than one row is a homograph - routed to the ambiguity popover, which
// reuses the Multiple-possible-meanings rendering.
const RESULTS_TAB_KEY = 'finnestdb:resultsTab:v1';
function loadResultsTab() {
    try {
        return localStorage.getItem(RESULTS_TAB_KEY) === 'words' ? 'words' : 'read';
    }
    catch {
        return 'read';
    }
}
function saveResultsTab(tab) {
    try {
        localStorage.setItem(RESULTS_TAB_KEY, tab);
    }
    catch { /* storage unavailable */ }
}
// buildFormIndex maps each surface form to the WordEntry rows that list it. A
// homograph surface (e.g. "kuusi" → NUM + NOUN) maps to more than one entry.
function buildFormIndex(data) {
    const index = new Map();
    for (const word of data.words) {
        for (const form of word.forms) {
            const rows = index.get(form);
            if (rows)
                rows.push(word);
            else
                index.set(form, [word]);
        }
    }
    return index;
}
function readTokenStatus(rows) {
    // Aggregate over every row a surface resolves to. Priority for the single
    // shown color, most-settled first: known > ignored(neutral) > learning >
    // new. "learning" = the sense is a pending study selection (selectedSenses)
    // OR already has a study card (currentLemmaState has no 'learning' value in
    // the (lemma,pos) model, so study intent is carried only by selectedSenses).
    let sawLearning = false;
    let sawNew = false;
    for (const w of rows) {
        const st = currentLemmaState(w.lemma, w.pos);
        if (st === 'known')
            return 'known';
        if (st === 'ignored')
            return 'neutral';
        // Study selection: any selected sense whose (lemma,pos) matches this row.
        const studied = [...state.selectedSenses].some(k => {
            const [, lemma, pos] = k.split(SENSE_KEY_SEP);
            return lemma === w.lemma && pos === w.pos;
        });
        if (studied)
            sawLearning = true;
        else
            sawNew = true;
    }
    if (sawLearning)
        return 'learning';
    if (sawNew)
        return 'new';
    return 'neutral';
}
// TEXT_TOKEN_RE splits source text into word tokens vs. non-word runs while
// keeping every character (so the rendered text is byte-faithful to the source
// minus HTML escaping). \p{L}\p{N} word chars plus intra-word marks the parser
// treats as part of a token (apostrophe, hyphen). Everything else - whitespace,
// punctuation - is passed through as plain text.
const TEXT_TOKEN_RE = /[\p{L}\p{N}][\p{L}\p{N}'’\-]*/gu;
// renderReadView renders state.currentSourceText into #read-text with paragraph
// structure preserved and each parsed surface as a tappable, state-colored span.
// Anonymous parses get neutral coloring (no learner state exists).
function renderReadView(data) {
    const host = document.getElementById('read-text');
    if (!host)
        return;
    const source = state.currentSourceText;
    if (!source.trim()) {
        host.innerHTML = '<p class="read-empty">The source text isn\'t available for this view.</p>';
        return;
    }
    const anonymous = state.role === 'anon';
    // Split on blank lines into paragraphs; preserve single newlines as breaks.
    const paragraphs = source.replace(/\r\n/g, '\n').split(/\n{2,}/);
    const html = paragraphs.map(para => {
        if (!para.trim())
            return '';
        // Render each line within a paragraph, joining with <br> so poem/line
        // structure survives.
        const lines = para.split('\n').map(line => renderReadLine(line, anonymous)).join('<br>');
        return `<p class="read-para">${lines}</p>`;
    }).join('');
    host.innerHTML = html || '<p class="read-empty">Nothing to show.</p>';
    wireReadTokens(host, data);
}
// renderReadLine tokenizes one line, wrapping each recognized surface in a
// tappable span and passing through everything else as escaped plain text.
function renderReadLine(line, anonymous) {
    let out = '';
    let last = 0;
    TEXT_TOKEN_RE.lastIndex = 0;
    let m;
    while ((m = TEXT_TOKEN_RE.exec(line)) !== null) {
        const surface = m[0];
        const start = m.index;
        if (start > last)
            out += escapeHtml(line.slice(last, start));
        last = start + surface.length;
        const rows = state.formIndex.get(surface);
        if (!rows || rows.length === 0) {
            // Unresolved token (number, unknown word the parser didn't attach):
            // plain text, not tappable.
            out += escapeHtml(surface);
            continue;
        }
        const status = anonymous ? 'neutral' : readTokenStatus(rows);
        const ambiguous = state.ambiguityBySurface.has(surface);
        const cls = ['read-token', `is-${status}`];
        if (ambiguous)
            cls.push('is-ambiguous');
        out += `<span class="${cls.join(' ')}" role="button" tabindex="0"`
            + ` data-read-surface="${escapeAttr(surface)}">${escapeHtml(surface)}</span>`;
    }
    if (last < line.length)
        out += escapeHtml(line.slice(last));
    return out;
}
// wireReadTokens attaches tap/keyboard handlers to open the word popover. Rerun
// on every render because innerHTML replacement drops listeners.
function wireReadTokens(host, data) {
    host.querySelectorAll('.read-token').forEach(span => {
        const open = () => {
            const surface = span.dataset.readSurface || '';
            if (surface)
                openReadPopover(surface, span, data);
        };
        span.addEventListener('click', open);
        span.addEventListener('keydown', (e) => {
            if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                open();
            }
        });
    });
}
// ── Reading popover ────────────────────────────────────────────────────────
let readPopoverAnchor = null;
// renderReadPopoverBody builds the popover content for a surface. For an
// ambiguous surface it reuses renderAmbiguityPanel (the Multiple-possible-
// meanings rendering); otherwise a compact gloss card with Known / Study /
// Ignore actions. Anonymous parses get gloss only plus a sign-in nudge.
function renderReadPopoverBody(surface, data) {
    const anonymous = state.role === 'anon';
    const amb = state.ambiguityBySurface.get(surface);
    if (amb && !anonymous) {
        // Reuse the exact ambiguity panel (candidates + per-candidate actions +
        // "None of these looks right"). Its controls are wired by the shared
        // wireAmbiguityControls below.
        return renderAmbiguityPanel(data.lang, amb);
    }
    const rows = state.formIndex.get(surface) || [];
    if (rows.length === 0) {
        return `<p class="read-pop-lemma">${escapeHtml(surface)}</p>`
            + `<p class="read-pop-empty">No dictionary entry attached.</p>`;
    }
    // Non-ambiguous (or anonymous, where ambiguity actions aren't offered): show
    // each resolved sense. Almost always one row.
    const cards = rows.map(w => {
        const glossHtml = w.gloss ? escapeHtml(w.gloss) : '<span class="no-gloss">Missing</span>';
        const posPill = `<span class="pos-pill" data-pos="${escapeHtml(w.pos)}" data-tooltip="${escapeHtml(posLabel(w.pos))}">${escapeHtml(posAbbrev(w.pos))}</span>`;
        const grammar = w.grammar_label ? `<span class="grammar-badge">${escapeHtml(w.grammar_label)}</span>` : '';
        const head = `<div class="read-pop-head">
                <span class="read-pop-surface">${escapeHtml(surface)}</span>
                <span class="read-pop-lemma-line">${escapeHtml(w.lemma)}${grammar} ${posPill}</span>
            </div>
            <p class="read-pop-gloss">${glossHtml}</p>`;
        if (anonymous)
            return `<div class="read-pop-sense">${head}</div>`;
        const st = currentLemmaState(w.lemma, w.pos);
        const key = senseKey(surface, w.lemma, w.pos);
        const isKnown = st === 'known';
        const isIgnored = st === 'ignored';
        const isStudy = state.selectedSenses.has(key);
        const pending = state.pendingLemmaStates.has(lemmaStateKey(data.lang, w.lemma, w.pos));
        // Actions mirror the table's known/ignore + the chip's study semantics.
        const actions = `<div class="read-pop-actions"
                data-surface="${escapeAttr(surface)}"
                data-lemma="${escapeAttr(w.lemma)}"
                data-pos="${escapeAttr(w.pos)}">
            <button type="button" class="read-pop-btn read-pop-known${isKnown ? ' is-active' : ''}"
                data-read-action="known" ${pending ? 'disabled' : ''}>
                ${isKnown ? 'Known' : 'Mark known'}
            </button>
            <button type="button" class="read-pop-btn read-pop-study${isStudy ? ' is-active' : ''}"
                data-read-action="study" ${isKnown ? 'disabled' : ''}
                data-tooltip="Creates a review card when you save.">
                ${isStudy ? 'Studying' : 'Study'}
            </button>
            <button type="button" class="read-pop-btn read-pop-ignore${isIgnored ? ' is-active' : ''}"
                data-read-action="ignore" ${pending ? 'disabled' : ''}>
                ${isIgnored ? 'Ignored' : 'Ignore'}
            </button>
        </div>
        <p class="read-pop-hint">Creates a review card when you save.</p>`;
        return `<div class="read-pop-sense">${head}${actions}</div>`;
    }).join('');
    const nudge = anonymous
        ? `<p class="read-pop-nudge"><a href="#/signin">Create an account</a> to mark words known or add them to study.</p>`
        : '';
    return cards + nudge;
}
// openReadPopover positions the popover under the tapped token (anchored on
// desktop; CSS turns it into a bottom sheet on narrow screens) and wires its
// controls. One popover at a time.
function openReadPopover(surface, anchor, data) {
    const pop = document.getElementById('read-popover');
    const backdrop = document.getElementById('read-popover-backdrop');
    const body = document.getElementById('read-popover-body');
    if (!pop || !backdrop || !body)
        return;
    state.readPopoverSurface = surface;
    readPopoverAnchor = anchor;
    body.innerHTML = renderReadPopoverBody(surface, data);
    wireReadPopoverControls(body, data);
    backdrop.classList.remove('hidden');
    pop.classList.remove('hidden');
    positionReadPopover(pop, anchor);
    // Focus the popover for keyboard users; ESC closes.
    pop.focus();
}
function positionReadPopover(pop, anchor) {
    // Bottom-sheet mode is driven by CSS (max-width media query); skip absolute
    // positioning there so the sheet pins to the viewport bottom.
    const isSheet = window.matchMedia?.('(max-width: 440px)').matches;
    if (isSheet) {
        pop.style.top = '';
        pop.style.left = '';
        return;
    }
    const rect = anchor.getBoundingClientRect();
    const popRect = pop.getBoundingClientRect();
    const margin = 8;
    let top = rect.bottom + window.scrollY + 6;
    let left = rect.left + window.scrollX;
    // Keep within the viewport horizontally.
    const maxLeft = window.scrollX + document.documentElement.clientWidth - popRect.width - margin;
    if (left > maxLeft)
        left = Math.max(window.scrollX + margin, maxLeft);
    // Flip above the token if it would overflow the bottom.
    const overflowBottom = rect.bottom + popRect.height + 12 > document.documentElement.clientHeight;
    if (overflowBottom && rect.top - popRect.height - 6 > 0) {
        top = rect.top + window.scrollY - popRect.height - 6;
    }
    pop.style.top = `${Math.round(top)}px`;
    pop.style.left = `${Math.round(left)}px`;
}
function closeReadPopover() {
    const pop = document.getElementById('read-popover');
    const backdrop = document.getElementById('read-popover-backdrop');
    if (pop)
        pop.classList.add('hidden');
    if (backdrop)
        backdrop.classList.add('hidden');
    state.readPopoverSurface = '';
    const anchor = readPopoverAnchor;
    readPopoverAnchor = null;
    // Return focus to the token the popover came from (keyboard flow).
    if (anchor && document.contains(anchor))
        anchor.focus();
}
// wireReadPopoverControls attaches the popover's action handlers. Ambiguous
// surfaces route through the shared ambiguity controls (same code as the table
// chip); non-ambiguous senses use the compact known/study/ignore buttons.
function wireReadPopoverControls(body, data) {
    if (body.querySelector('.ambiguity-panel')) {
        wireAmbiguityControls(body, data);
        return;
    }
    body.querySelectorAll('[data-read-action]').forEach(btn => {
        btn.addEventListener('click', () => {
            const holder = btn.closest('.read-pop-actions');
            if (!holder)
                return;
            const action = btn.dataset.readAction;
            const surface = holder.dataset.surface || '';
            const lemma = holder.dataset.lemma || '';
            const pos = holder.dataset.pos || '';
            if (action === 'study') {
                // Same pending-deck semantics as the chip's "Study this meaning":
                // select the sense; it becomes a review card on save.
                const key = senseKey(surface, lemma, pos);
                if (state.selectedSenses.has(key))
                    state.selectedSenses.delete(key);
                else
                    state.selectedSenses.add(key);
                refreshResultsViews();
                // Re-render the popover body in place to reflect the toggle.
                body.innerHTML = renderReadPopoverBody(surface, data);
                wireReadPopoverControls(body, data);
            }
            else if (action === 'known') {
                const next = currentLemmaState(lemma, pos) === 'known' ? 'neutral' : 'known';
                void markReadLemma(next, surface, lemma, pos, data);
            }
            else if (action === 'ignore') {
                const next = currentLemmaState(lemma, pos) === 'ignored' ? 'neutral' : 'ignored';
                void markReadLemma(next, surface, lemma, pos, data);
            }
        });
    });
    // data-tooltip triggers are handled by the global delegated tooltip
    // listener (initPortalTooltips), so no per-body wiring is needed here.
}
// markReadLemma is the popover's known/ignore action. It reuses markResultLemma
// (the table's endpoint call + toast + view refresh), then re-renders the open
// popover body so its buttons reflect the new state.
async function markReadLemma(status, surface, lemma, pos, data) {
    // A dummy trigger button satisfies markResultLemma's disable/enable contract
    // without coupling to a specific DOM node (the popover re-renders anyway).
    const trigger = document.createElement('button');
    await markResultLemma(status, lemma, pos, trigger);
    // If the popover is still open on this surface, re-render its body.
    if (state.readPopoverSurface === surface) {
        const body = document.getElementById('read-popover-body');
        if (body) {
            body.innerHTML = renderReadPopoverBody(surface, data);
            wireReadPopoverControls(body, data);
        }
    }
}
// ── Tab switching + shared refresh ─────────────────────────────────────────
// refreshResultsViews re-renders every results surface that depends on live
// learner state (table, Read view, coverage reveal) without replaying the
// coverage entrance animation. Both tabs stay in sync from any action.
function refreshResultsViews() {
    const data = state.currentResults;
    if (!data)
        return;
    renderResultsTable(data);
    if (state.currentContext !== 'deck') {
        renderReadView(data);
        renderCoverageReveal(data, false);
    }
}
// renderResultsTabs shows/hides the tab bar and the two views. The Read view is
// available only when there's source text to read (never in deck context); when
// it isn't, the Words table shows alone and the tab bar is hidden.
function renderResultsTabs() {
    const tabs = document.getElementById('results-tabs');
    const readView = document.getElementById('read-view');
    const wordsView = document.getElementById('words-view');
    const tabRead = document.getElementById('results-tab-read');
    const tabWords = document.getElementById('results-tab-words');
    if (!tabs || !readView || !wordsView || !tabRead || !tabWords)
        return;
    const canRead = state.currentContext !== 'deck' && state.currentSourceText.trim().length > 0;
    if (!canRead) {
        // No text to read: Words only, tab bar hidden. Coverage reveal is already
        // hidden for decks by renderCoverageReveal.
        tabs.classList.add('hidden');
        readView.classList.add('hidden');
        wordsView.classList.remove('hidden');
        return;
    }
    tabs.classList.remove('hidden');
    const active = state.resultsTab;
    tabRead.setAttribute('aria-selected', active === 'read' ? 'true' : 'false');
    tabWords.setAttribute('aria-selected', active === 'words' ? 'true' : 'false');
    tabRead.classList.toggle('active', active === 'read');
    tabWords.classList.toggle('active', active === 'words');
    readView.classList.toggle('hidden', active !== 'read');
    wordsView.classList.toggle('hidden', active !== 'words');
}
function switchResultsTab(tab) {
    state.resultsTab = tab;
    saveResultsTab(tab);
    renderResultsTabs();
    // Reposition/close popover if it hung around while hidden.
    if (tab !== 'read')
        closeReadPopover();
}
function initResultsTabs() {
    document.querySelectorAll('.results-tab').forEach(btn => {
        btn.addEventListener('click', () => {
            const tab = btn.dataset.resultsTab;
            if (tab)
                switchResultsTab(tab);
        });
    });
    // Popover dismissal: backdrop tap, close button, ESC.
    document.getElementById('read-popover-backdrop')?.addEventListener('click', closeReadPopover);
    document.getElementById('read-popover-close')?.addEventListener('click', closeReadPopover);
    document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape' && state.readPopoverSurface)
            closeReadPopover();
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
    else if ((context === 'inspect' || context === 'landing') && state.role !== 'admin') {
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
    // Deck comprehension is fetched separately by loadDeckDetail; hide the
    // stale panel whenever new results render (parse results, other decks).
    const comprehensionPanel = document.getElementById('deck-comprehension');
    if (comprehensionPanel) {
        comprehensionPanel.classList.add('hidden');
        comprehensionPanel.innerHTML = '';
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
    // Reset and hydrate Multiple-possible-meanings state for this parse.
    state.ambiguityBySurface.clear();
    state.ambiguityExpanded.clear();
    state.selectedSenses.clear();
    state.ambiguityKnownPending.clear();
    for (const amb of data.ambiguous_surfaces || []) {
        state.ambiguityBySurface.set(amb.surface, amb);
    }
    // Surface → WordEntry index for the Read view's token coloring + popover.
    state.formIndex = buildFormIndex(data);
    // Any open reading popover belongs to the previous parse.
    closeReadPopover();
    state.resultsTab = loadResultsTab();
    renderResultsTable(data);
    // Coverage reveal (aha #1): runs after currentLemmaStates + currentContext
    // are hydrated above so the numbers match the table's live known-state.
    renderCoverageReveal(data);
    // Read view (the living text) + tab bar. Read is the default landing view;
    // the tab bar hides entirely in deck context (no source text to read).
    renderReadView(data);
    renderResultsTabs();
    renderResultsSaveState();
    renderChapterNav();
    renderAnonResultsChrome();
    renderResultsExport(data, context);
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
// Show/hide the anonymous-only sign-up ribbon and privacy footer on the results
// page. The ribbon is dismissible per session but reappears on the next parse
// (USER_FLOWS §2) - startLandingParse resets state.anonRibbonDismissed. The
// footer always shows for anonymous visitors. Both are additionally gated by
// data-role-show="anon", so a signed-in user never sees them.
function renderAnonResultsChrome() {
    const isAnon = state.role === 'anon';
    const ribbon = document.getElementById('anon-signup-ribbon');
    const footer = document.getElementById('anon-privacy-footer');
    if (ribbon)
        ribbon.classList.toggle('hidden', !isAnon || state.anonRibbonDismissed);
    if (footer)
        footer.classList.toggle('hidden', !isAnon);
}
function initAnonResultsChrome() {
    document.getElementById('anon-ribbon-dismiss')?.addEventListener('click', () => {
        state.anonRibbonDismissed = true;
        document.getElementById('anon-signup-ribbon')?.classList.add('hidden');
    });
}
// ── Word-list export (copy + CSV) ──────────────────────────────────────────
//
// Available to every role, anonymous included (landing freemium cell ii: "Copy
// or download - word list as plain text or CSV"). Both formats are generated
// entirely client-side from the parse response already in memory: no server
// call, nothing stored, so it works within the anonymous ephemeral guarantee.
// The export controls hide in deck context (a saved deck has no ephemeral
// word-list to export) and when there are no words.
// csvCell RFC-4180-quotes one field: wrap in double quotes and double any
// embedded quotes, so commas, quotes, and newlines survive a spreadsheet import.
function csvCell(value) {
    return `"${String(value).replace(/"/g, '""')}"`;
}
function exportRows(data) {
    // Occurrence-desc, mirroring the table's default sort, so the exported list
    // reads in the same order the learner sees.
    return [...data.words].sort((a, b) => b.count - a.count);
}
function wordListAsText(data) {
    // Tab-separated lemma / POS / definition - the shape that drops straight
    // into Anki or a spreadsheet paste.
    return exportRows(data)
        .map(w => [w.lemma, posLabel(w.pos), w.gloss || ''].join('\t'))
        .join('\n');
}
function wordListAsCSV(data) {
    const rows = ['lemma,pos,forms,definition,grammar,count'];
    for (const w of exportRows(data)) {
        rows.push([
            csvCell(w.lemma),
            csvCell(posLabel(w.pos)),
            csvCell((w.forms || []).join('|')),
            csvCell(w.gloss || ''),
            csvCell(w.grammar_label || ''),
            csvCell(w.count),
        ].join(','));
    }
    return rows.join('\r\n');
}
// renderResultsExport shows the export controls whenever there's an ephemeral
// word list (any non-deck parse with words) and hides them otherwise.
function renderResultsExport(data, context) {
    const wrap = document.getElementById('results-export');
    if (!wrap)
        return;
    const show = !!data && context !== 'deck' && data.words.length > 0;
    wrap.classList.toggle('hidden', !show);
}
async function copyWordList() {
    const data = state.currentResults;
    if (!data || data.words.length === 0)
        return;
    const text = wordListAsText(data);
    try {
        await navigator.clipboard.writeText(text);
        showToast(`Copied ${data.words.length} words to the clipboard.`, 'success');
    }
    catch {
        showToast('Could not copy to the clipboard.', 'error');
    }
}
function downloadWordListCSV() {
    const data = state.currentResults;
    if (!data || data.words.length === 0)
        return;
    const csv = wordListAsCSV(data);
    const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `finnest-${(data.lang || 'xx').toLowerCase()}-wordlist.csv`;
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
    showToast(`Downloaded ${data.words.length} words as CSV.`, 'success');
}
function initResultsExport() {
    document.getElementById('results-copy-list')?.addEventListener('click', () => void copyWordList());
    document.getElementById('results-download-csv')?.addEventListener('click', downloadWordListCSV);
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
        // Quota exceeded or sessionStorage unavailable - silently skip; the
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
// hasCarriedParse reports whether a carried parse exists in sessionStorage
// (an anonymous parse the visitor had open before signing in). Deck contexts
// aren't persisted here, so any hit is a real results/read view to restore.
function hasCarriedParse() {
    try {
        const raw = sessionStorage.getItem(LAST_PARSE_KEY);
        if (!raw)
            return false;
        const parsed = JSON.parse(raw);
        return !!parsed?.data && !!parsed.context;
    }
    catch {
        return false;
    }
}
// landAfterAuth decides where a freshly-authenticated user lands. Carry-forward
// (USER_FLOWS "Carry-forward of anonymous parses"): if a parse was open before
// sign-in / account creation / session re-auth, restore it - re-rendered against
// the now-authenticated state so the reveal's known % becomes real and learner
// controls appear - instead of dropping the user on the dashboard and losing
// their place. With no carried context, land on the dashboard as before.
async function landAfterAuth() {
    if (hasCarriedParse() && await restoreLastParse())
        return;
    navigate('/dashboard');
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
        void loadDeckComprehension(deckID);
    }
    catch (err) {
        showToast(err.message || 'Failed to load deck.', 'error');
        navigate('/decks');
    }
}
// Fetches and renders the token-weighted comprehension projection on the deck
// detail view: headline percentage plus a "learn these next" list showing the
// before → after coverage if the user learns the top unlock candidates.
// Non-fatal on any failure - the deck page works without the projection.
async function loadDeckComprehension(deckID) {
    const panel = document.getElementById('deck-comprehension');
    if (!panel)
        return;
    try {
        const resp = await fetch(`/api/decks/${deckID}/comprehension`, { credentials: 'same-origin' });
        if (!resp.ok)
            return;
        const data = await resp.json();
        if (state.currentDeckID !== deckID || !data.total_tokens)
            return;
        const unlocks = data.top_unlocks || [];
        const projected = Math.min(100, Math.round((data.coverage_pct + unlocks.reduce((sum, u) => sum + u.gain_pct, 0)) * 10) / 10);
        const unlockItems = unlocks.map(u => `
            <li class="deck-unlock-item">
                <span class="deck-unlock-lemma">${escapeHtml(u.lemma)}</span>
                <span class="deck-unlock-pos">${escapeHtml(u.pos)}</span>
                <span class="deck-unlock-gain">+${u.gain_pct}%</span>
            </li>`).join('');
        const projection = unlocks.length > 0
            ? `<details class="deck-unlocks">
                <summary>Learn these ${unlocks.length} words to reach ~${projected}% comprehension</summary>
                <ul class="deck-unlock-list">${unlockItems}</ul>
            </details>`
            : '';
        panel.innerHTML = `
            <p class="deck-comprehension-headline">
                Predicted comprehension <strong>${data.coverage_pct}%</strong>
                <span class="deck-comprehension-detail">${data.known_tokens.toLocaleString()} of ${data.total_tokens.toLocaleString()} words in running text</span>
            </p>
            ${projection}`;
        panel.classList.remove('hidden');
    }
    catch {
        // Non-fatal: leave the panel hidden.
    }
}
function formatDeckCreatedAt(iso) {
    const d = new Date(iso);
    if (Number.isNaN(d.getTime()))
        return '';
    return d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
}
const MAX_DERIVED_TITLE_LEN = 60;
// deriveTitle mirrors internal/store/titles.go's DeriveTitle: the first
// clause/sentence of pasted text, cleaned and cut at a sentence/clause
// boundary under MAX_DERIVED_TITLE_LEN chars. It exists so the save modal can
// prefill a good title BEFORE the round trip to the server (better UX: the
// learner sees and can edit the suggestion immediately). The server re-derives
// the same way as the blank-title fallback (CreateDeckRequest.Title), so a
// title is never required client-side and the API contract stays honest for
// non-browser callers. Keep this in sync with DeriveTitle if the rules change.
function deriveTitle(sourceText, lang) {
    const cleaned = cleanTitleSource(sourceText);
    if (!cleaned) {
        return lang === 'ET' ? 'Untitled Estonian text' : 'Untitled Finnish text';
    }
    if (isDegenerateTitleSource(cleaned)) {
        return truncateToWordsForTitle(cleaned, 4);
    }
    return truncateAtClause(cleaned);
}
function cleanTitleSource(text) {
    let s = text.trim();
    if (!s)
        return '';
    const newlineIx = s.search(/[\n\r]/);
    if (newlineIx >= 0)
        s = s.slice(0, newlineIx);
    s = s.trim().replace(/^(#{1,6}\s+|[-*+]\s+|>\s+)/, '');
    s = s.replace(/\s+/g, ' ').trim();
    s = s.replace(/^["'“”«»*_]+|["'“”«»*_]+$/g, '');
    return s.trim();
}
function isDegenerateTitleSource(text) {
    if (/^https?:\/\/\S+$/.test(text))
        return true;
    if (isDigitsOnlyTitleSource(text))
        return true;
    return text.split(/\s+/).filter(Boolean).length <= 1;
}
function isDigitsOnlyTitleSource(text) {
    let seenDigit = false;
    for (const ch of text) {
        if (/\d/.test(ch)) {
            seenDigit = true;
            continue;
        }
        if (/[\s.,:-]/.test(ch))
            continue;
        return false;
    }
    return seenDigit;
}
function truncateToWordsForTitle(text, n) {
    const fields = text.split(/\s+/).filter(Boolean).slice(0, n);
    return hardTruncateTitle(fields.join(' '));
}
// Matches a real sentence end (. ! ?), optionally followed by a closing
// quote, followed by whitespace or end-of-string - mirrors reSentenceEnd in
// titles.go so "example.com" isn't mistaken for a sentence boundary.
const SENTENCE_END_RE = /[.!?]["'”’]?(\s|$)/;
function truncateAtClause(text) {
    if ([...text].length <= MAX_DERIVED_TITLE_LEN) {
        const m = SENTENCE_END_RE.exec(text);
        if (m)
            return text.slice(0, m.index + 1).trim();
        return text;
    }
    const chars = [...text];
    const window = chars.slice(0, MAX_DERIVED_TITLE_LEN).join('');
    const m = SENTENCE_END_RE.exec(window);
    if (m)
        return window.slice(0, m.index + 1).trim();
    const ellipsisWindow = chars.slice(0, MAX_DERIVED_TITLE_LEN - 1).join('');
    const clauseIx = lastIndexOfAny(ellipsisWindow, ',;:');
    if (clauseIx >= 0)
        return ellipsisWindow.slice(0, clauseIx).trim() + '…';
    const spaceIx = ellipsisWindow.lastIndexOf(' ');
    if (spaceIx > 0)
        return ellipsisWindow.slice(0, spaceIx).trim() + '…';
    return ellipsisWindow.trim() + '…';
}
function lastIndexOfAny(text, chars) {
    let best = -1;
    for (const ch of chars) {
        const ix = text.lastIndexOf(ch);
        if (ix > best)
            best = ix;
    }
    return best;
}
function hardTruncateTitle(text) {
    const chars = [...text];
    if (chars.length <= MAX_DERIVED_TITLE_LEN)
        return text;
    return chars.slice(0, MAX_DERIVED_TITLE_LEN - 1).join('').trim() + '…';
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
    // than a derived first-clause title. Use the results-context EPUB so
    // a still-loaded book in another form doesn't leak into this default.
    const epub = state.resultsEpub;
    if (epub) {
        input.value = epub.bookTitle;
        return;
    }
    const lang = state.currentResults?.lang === 'ET' ? 'ET' : 'FI';
    input.value = state.currentSourceText
        ? deriveTitle(state.currentSourceText, lang)
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
        // Explicit "Study this meaning" / "Not sure" selections from Multiple
        // possible meanings. The server injects the FST-only senses that the
        // dict-only deck expansion would otherwise drop, so an explicitly chosen
        // homograph sense still gets its review card.
        const selectedSenses = [...state.selectedSenses].map(k => {
            const [surface, lemma, pos] = k.split(SENSE_KEY_SEP);
            return { surface, lemma, pos };
        });
        const resp = await fetch('/api/decks', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'same-origin',
            body: JSON.stringify({
                title,
                lang: state.currentResults.lang,
                text: state.currentSourceText,
                is_public: isPublic,
                selected_senses: selectedSenses,
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
    const frontEl = document.getElementById('review-card-front');
    const frontTextEl = document.getElementById('review-card-front-text');
    const surfaceEl = document.getElementById('review-card-surface');
    const lemmaEl = document.getElementById('review-card-lemma');
    const posEl = document.getElementById('review-card-pos');
    const homographEl = document.getElementById('review-card-homograph');
    const meaningEl = document.getElementById('review-card-meaning');
    const modeEl = document.getElementById('review-card-mode');
    if (!cardEl || !emptyEl || !deckCountsEl || !exampleEl || !frontEl || !frontTextEl || !surfaceEl || !lemmaEl || !posEl || !homographEl || !meaningEl || !modeEl)
        return;
    const card = state.currentReviewCard;
    const hasCard = Boolean(card);
    cardEl.classList.toggle('hidden', !hasCard);
    emptyEl.classList.toggle('hidden', hasCard);
    if (!card)
        return;
    // Starter decks (e.g. Top-1000) have no source sentence, so the API sends
    // mode "word" with no front text. Hide the "Sentence card" chip and the
    // front-text line entirely rather than showing a chip over text that just
    // repeats the surface heading below with nothing in between.
    const isSentenceCard = card.mode === 'sentence' && Boolean(card.front.text);
    frontEl.classList.toggle('hidden', !isSentenceCard);
    if (isSentenceCard) {
        modeEl.textContent = 'Sentence card';
        frontTextEl.textContent = card.front.text || card.back.surface || card.back.lemma;
    }
    else {
        modeEl.textContent = '';
        frontTextEl.textContent = '';
    }
    // Surface form is the card's primary identity; lemma/POS are supporting
    // metadata shown beneath it.
    const surface = card.back.surface || card.back.lemma;
    surfaceEl.textContent = surface;
    if (card.back.lemma && card.back.lemma.toLowerCase() !== surface.toLowerCase()) {
        lemmaEl.textContent = `lemma: ${card.back.lemma}`;
        lemmaEl.classList.remove('hidden');
    }
    else {
        lemmaEl.textContent = '';
        lemmaEl.classList.add('hidden');
    }
    if (card.back.homograph_note) {
        homographEl.textContent = card.back.homograph_note;
        homographEl.classList.remove('hidden');
    }
    else {
        homographEl.textContent = '';
        homographEl.classList.add('hidden');
    }
    meaningEl.textContent = card.back.meaning || 'No gloss yet';
    if (card.back.pos) {
        posEl.textContent = posLabel(card.back.pos);
        posEl.classList.remove('hidden');
    }
    else {
        posEl.textContent = '';
        posEl.classList.add('hidden');
    }
    deckCountsEl.innerHTML = card.deck_counts.map(pair => `<span class="review-deck-pill">${escapeHtml(pair[0])} · ${escapeHtml(pair[1])}</span>`).join('');
    const example = card.back.examples?.[0];
    if (example) {
        exampleEl.classList.remove('hidden');
        const highlightForm = surface || card.front.highlight || '';
        const sentenceHtml = highlightForm
            ? highlightFormsInSentence(example.text, [highlightForm])
            : escapeHtml(example.text);
        exampleEl.innerHTML = sentenceHtml +
            (example.source_deck ? `<span class="review-example-source">${escapeHtml(example.source_deck)}</span>` : '');
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
    const flagFilter = document.getElementById('admin-feedback-flag-only');
    if (flagFilter)
        flagFilter.value = state.adminFeedbackFlagOnly;
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
        const flagBadge = item.flag_only
            ? '<span class="admin-feedback-flag-badge">Flag-only</span>'
            : '';
        // For a flag-only report the learner didn't propose an analysis. Give
        // the admin lemma/POS inputs so they can convert it into a concrete
        // parser-identity correction (path b) before accepting. Leaving them
        // blank accepts the report without any lexical writeback.
        const proposedBlock = item.flag_only
            ? `<div>
                    <span class="admin-feedback-label">Supply correction (optional)</span>
                    <div class="admin-feedback-correction-inputs">
                        <input type="text" class="admin-feedback-correction-lemma" data-correction-lemma="${item.id}" placeholder="Base form" autocomplete="off">
                        <select class="admin-feedback-correction-pos" data-correction-pos="${item.id}">
                            <option value="">POS…</option>
                            ${adminPosOptions()}
                        </select>
                    </div>
                    <p class="admin-feedback-correction-hint">Leave blank to accept the flag without changing the parser.</p>
                </div>`
            : `<div>
                    <span class="admin-feedback-label">Proposed</span>
                    <p>${escapeHtml(proposed)}${proposedGrammar}</p>
                </div>`;
        return `<article class="admin-feedback-item" data-feedback-id="${item.id}">
            <header class="admin-feedback-item-header">
                <div>
                    <h2>${escapeHtml(item.surface)}${flagBadge}</h2>
                    <p class="admin-feedback-meta">${escapeHtml(item.lang)} · ${escapeHtml(item.parser)} · session #${item.parse_session_id} · user #${item.user_id} · ${formatFeedbackDate(item.created_at)}</p>
                </div>
                <span class="admin-feedback-status">${feedbackStatusLabel(item.status)}</span>
            </header>
            <div class="admin-feedback-comparison">
                <div>
                    <span class="admin-feedback-label">Original</span>
                    <p>${escapeHtml(original)}${originalGrammar}</p>
                </div>
                ${proposedBlock}
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
// adminPosOptions returns the POS <option> list shared by the admin
// flag-only correction inputs. Mirrors the correction modal's POS choices.
function adminPosOptions() {
    const opts = [
        ['NOUN', 'Noun'], ['VERB', 'Verb'], ['ADJ', 'Adjective'], ['ADV', 'Adverb'],
        ['PROPN', 'Proper noun'], ['PRON', 'Pronoun'], ['DET', 'Determiner'],
        ['ADP', 'Adposition'], ['NUM', 'Number'], ['CCONJ', 'Conjunction (coordinating)'],
        ['SCONJ', 'Conjunction (subordinating)'], ['PART', 'Particle'],
        ['INTJ', 'Interjection'], ['AUX', 'Auxiliary'], ['X', 'Other'],
    ];
    return opts.map(([v, l]) => `<option value="${v}">${l}</option>`).join('');
}
async function loadAdminFeedback() {
    if (state.role !== 'admin')
        return;
    const params = new URLSearchParams();
    if (state.adminFeedbackStatus)
        params.set('status', state.adminFeedbackStatus);
    if (state.adminFeedbackFlagOnly)
        params.set('flag_only', state.adminFeedbackFlagOnly);
    const query = params.toString() ? `?${params.toString()}` : '';
    try {
        const resp = await fetch(`/api/admin/parse-feedback${query}`, { credentials: 'same-origin' });
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
    // On a flag-only row the admin may supply a concrete correction (path b),
    // converting it to a normal parser-identity fix. Only send it when both
    // lemma and POS are filled and the row is being accepted.
    const lemmaInput = document.querySelector(`[data-correction-lemma="${feedbackID}"]`);
    const posInput = document.querySelector(`[data-correction-pos="${feedbackID}"]`);
    const body = { status, review_note: reviewNote };
    if (status === 'accepted' && lemmaInput && posInput) {
        const lemma = lemmaInput.value.trim();
        const pos = posInput.value.trim();
        if (lemma || pos) {
            if (!lemma || !pos) {
                showToast('Supply both base form and part of speech, or leave both blank.', 'error');
                return;
            }
            body.proposed_lemma = lemma;
            body.proposed_pos = pos;
        }
    }
    try {
        const resp = await fetch(`/api/admin/parse-feedback?id=${encodeURIComponent(String(feedbackID))}`, {
            method: 'PATCH',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'same-origin',
            body: JSON.stringify(body),
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
// ── Admin correction issues ────────────────────────────────────────────────
const ISSUE_ALPHA_CLASSES = [
    ['parser_issue', 'Parser issue'],
    ['bad_card_content', 'Bad card content'],
    ['source_extraction_issue', 'Source/extraction issue'],
    ['not_sure', 'Not sure'],
];
function issueStatusLabel(status) {
    switch (status) {
        case 'open': return 'Open';
        case 'quarantined': return 'Quarantined';
        case 'fixed': return 'Fixed';
        case 'reopened': return 'Reopened';
        default: return status || 'All';
    }
}
function issueScopeLabel(issue) {
    // Card scope shows lemma/pos; surface-only scope shows the normalized
    // surface. This mirrors how suppression matches (see the Go predicates).
    if (issue.lemma && issue.pos) {
        return `${escapeHtml(issue.lemma)} / ${escapeHtml(issue.pos)}`;
    }
    return `surface “${escapeHtml(issue.norm_surface)}”`;
}
function renderAdminIssuesPage() {
    const filter = document.getElementById('admin-issues-status');
    if (filter)
        filter.value = state.adminIssuesStatus;
    renderAdminIssuesList();
}
function renderAdminIssuesList() {
    const list = document.getElementById('admin-issues-list');
    const empty = document.getElementById('admin-issues-empty');
    if (!list || !empty)
        return;
    const issues = state.adminIssues;
    empty.classList.toggle('hidden', issues.length > 0);
    list.classList.toggle('hidden', issues.length === 0);
    if (issues.length === 0) {
        list.innerHTML = '';
        return;
    }
    list.innerHTML = issues.map(issue => {
        const thresholdBadge = issue.threshold_candidate
            ? '<span class="admin-feedback-flag-badge">Threshold candidate</span>'
            : '';
        const reopenedBadge = issue.reopened_count > 0
            ? `<span class="admin-feedback-flag-badge">Reopened ×${issue.reopened_count}</span>`
            : '';
        const classSelect = `<select class="admin-issue-class" data-issue-class="${issue.id}">
                <option value="">Classify…</option>
                ${ISSUE_ALPHA_CLASSES.map(([v, l]) => `<option value="${v}"${issue.alpha_class === v ? ' selected' : ''}>${l}</option>`).join('')}
            </select>`;
        // Quarantine and restore are mutually exclusive by status. Triage is
        // always allowed; quarantine requires a class (enforced client- and
        // server-side) and a reason.
        const isQuarantined = issue.status === 'quarantined';
        const actions = isQuarantined
            ? `<button type="button" class="btn btn-primary btn-sm" data-issue-action="restore" data-issue-id="${issue.id}">Restore (mark fixed)</button>`
            : `<input type="text" class="admin-issue-reason" data-issue-reason="${issue.id}" placeholder="Quarantine reason (required)">
               <button type="button" class="btn btn-outline btn-sm" data-issue-action="triage" data-issue-id="${issue.id}">Save class</button>
               <button type="button" class="btn btn-primary btn-sm" data-issue-action="quarantine" data-issue-id="${issue.id}">Quarantine now</button>`;
        const reason = issue.quarantine_reason
            ? `<p class="admin-feedback-review-note">Quarantine reason: ${escapeHtml(issue.quarantine_reason)}</p>` : '';
        const fixNote = issue.fix_note
            ? `<p class="admin-feedback-review-note">Fix note: ${escapeHtml(issue.fix_note)}</p>` : '';
        return `<article class="admin-feedback-item" data-issue-id="${issue.id}">
            <header class="admin-feedback-item-header">
                <div>
                    <h2>${issueScopeLabel(issue)}${thresholdBadge}${reopenedBadge}</h2>
                    <p class="admin-feedback-meta">${escapeHtml(issue.lang)} · ${escapeHtml(issue.parser)} · ${issue.report_count} report${issue.report_count === 1 ? '' : 's'} · ${issue.distinct_reporter_count} distinct reporter${issue.distinct_reporter_count === 1 ? '' : 's'}${issue.last_reported_at ? ' · ' + formatFeedbackDate(issue.last_reported_at) : ''}</p>
                </div>
                <span class="admin-feedback-status">${issueStatusLabel(issue.status)}</span>
            </header>
            ${reason}
            ${fixNote}
            <div class="admin-feedback-actions">
                ${classSelect}
                ${actions}
            </div>
        </article>`;
    }).join('');
}
async function loadAdminIssues() {
    if (state.role !== 'admin')
        return;
    const params = new URLSearchParams();
    if (state.adminIssuesStatus)
        params.set('status', state.adminIssuesStatus);
    const query = params.toString() ? `?${params.toString()}` : '';
    try {
        const resp = await fetch(`/api/admin/correction-issues${query}`, { credentials: 'same-origin' });
        if (!resp.ok) {
            throw new Error(await resp.text() || 'Failed to load correction issues');
        }
        const data = await resp.json();
        state.adminIssues = data.issues || [];
        renderAdminIssuesList();
    }
    catch (err) {
        state.adminIssues = [];
        renderAdminIssuesList();
        showToast(err.message || 'Failed to load correction issues.', 'error');
    }
}
async function actOnAdminIssue(issueID, action) {
    const classSelect = document.querySelector(`[data-issue-class="${issueID}"]`);
    const reasonInput = document.querySelector(`[data-issue-reason="${issueID}"]`);
    const alphaClass = classSelect?.value.trim() || '';
    const body = { action };
    if (action === 'triage') {
        if (!alphaClass) {
            showToast('Pick a classification first.', 'error');
            return;
        }
        body.alpha_class = alphaClass;
    }
    if (action === 'quarantine') {
        if (!alphaClass) {
            showToast('Classify the issue before quarantining.', 'error');
            return;
        }
        const reason = reasonInput?.value.trim() || '';
        if (!reason) {
            showToast('A quarantine reason is required.', 'error');
            return;
        }
        // Send class + reason so the server triages and quarantines atomically.
        body.alpha_class = alphaClass;
        body.reason = reason;
    }
    try {
        const resp = await fetch(`/api/admin/correction-issues?id=${encodeURIComponent(String(issueID))}`, {
            method: 'PATCH',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'same-origin',
            body: JSON.stringify(body),
        });
        if (!resp.ok) {
            throw new Error(await resp.text() || 'Failed to update correction issue');
        }
        const labels = {
            triage: 'Classification saved.',
            quarantine: 'Issue quarantined. Matching content is now hidden from study.',
            restore: 'Issue restored. Matching content is back in circulation.',
        };
        showToast(labels[action], 'success');
        await loadAdminIssues();
    }
    catch (err) {
        showToast(err.message || 'Failed to update correction issue.', 'error');
    }
}
// ── Correction modal ───────────────────────────────────────────────────────
function openCorrectionModal(row, opts) {
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
    // Two-path modal (USER_FLOWS §10): default to flag-only so a learner who
    // just knows something's wrong can report it in one click. The "None of
    // these looks right" entry point from Multiple possible meanings forces the
    // flag-only path - it is parser feedback that the candidate list looks
    // wrong, never a proposed correction.
    const flagRadio = document.getElementById('correction-mode-flag');
    const proposeRadio = document.getElementById('correction-mode-propose');
    if (flagRadio)
        flagRadio.checked = true;
    // When forced (Multiple possible meanings escape hatch), also disable the
    // propose path so this stays parser feedback only.
    if (proposeRadio)
        proposeRadio.disabled = Boolean(opts?.forceFlagOnly);
    syncCorrectionMode();
    // Backend requires authentication. Anonymous parses can't submit
    // corrections - the feedback endpoint creates the parse_session
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
// isFlagOnlyMode reports whether the "I don't know the right answer" radio is
// selected. Flag-only reports omit the proposed lemma/POS.
function isFlagOnlyMode() {
    const propose = document.getElementById('correction-mode-propose');
    return !(propose && propose.checked);
}
// syncCorrectionMode shows or hides the proposed-analysis fields to match the
// selected path and updates the submit button label.
function syncCorrectionMode() {
    const flagOnly = isFlagOnlyMode();
    const fields = document.getElementById('correction-proposed-fields');
    const submitBtn = document.getElementById('correction-submit');
    fields?.classList.toggle('hidden', flagOnly);
    if (submitBtn && !submitBtn.disabled) {
        submitBtn.textContent = flagOnly ? 'Flag as wrong' : 'Send correction';
    }
}
function initCorrectionModal() {
    document.getElementById('correction-modal-close')?.addEventListener('click', closeCorrectionModal);
    document.getElementById('correction-modal-backdrop')?.addEventListener('click', closeCorrectionModal);
    document.getElementById('correction-cancel')?.addEventListener('click', closeCorrectionModal);
    document.querySelectorAll('input[name="correction_mode"]').forEach(radio => {
        radio.addEventListener('change', syncCorrectionMode);
    });
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
                showToast("Can't send correction - no parse loaded.", 'error');
                return;
            }
            const flagOnly = isFlagOnlyMode();
            const proposedLemma = document.getElementById('correction-proposed-lemma').value.trim();
            const proposedPos = document.getElementById('correction-proposed-pos').value;
            const proposedGram = document.getElementById('correction-proposed-grammar').value.trim();
            const note = document.getElementById('correction-note').value.trim();
            if (!flagOnly && (!proposedLemma || !proposedPos)) {
                showToast('Please fill in both base form and part of speech.', 'error');
                return;
            }
            // Two attribution paths: deck-detail feedback has a persisted
            // parse_session (results.parse_id); Inspect-view feedback ships
            // the source text inline and the server creates a session lazily.
            // Flag-only reports omit the proposed analysis entirely.
            const body = {
                lang: results.lang,
                parser: state.currentParserMode,
                surface: row.surface,
                occurrence: row.occurrence,
                original_lemma: row.lemma,
                original_pos: row.pos,
                original_grammar_label: row.original_grammar_label,
                flag_only: flagOnly,
                proposed_lemma: flagOnly ? '' : proposedLemma,
                proposed_pos: flagOnly ? '' : proposedPos,
                proposed_grammar_label: flagOnly ? '' : proposedGram,
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
                showToast(flagOnly ? 'Thanks - flagged for review.' : 'Thanks - correction sent.', 'success');
                closeCorrectionModal();
            }
            else {
                showToast("Couldn't send correction - please try again later.", 'error');
            }
        }
        catch {
            showToast("Couldn't send correction - check your connection and try again.", 'error');
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
            // Catalog membership changes - bust the cache so the Official
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
    document.getElementById('vocab-delete-all')?.addEventListener('click', () => {
        void deleteAllKnownWords();
    });
}
function initHistoryPage() {
    document.getElementById('history-list')?.addEventListener('click', (e) => {
        const target = e.target;
        if (!target)
            return;
        const id = Number(target.getAttribute('data-delete-parse-session') || 0);
        if (id > 0)
            void deleteParseSession(id);
    });
    document.getElementById('history-delete-all')?.addEventListener('click', () => {
        void deleteAllParseSessions();
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
    // Vocab page launcher buttons.
    document.getElementById('vocab-anki-connect')?.addEventListener('click', openAnkiImportModal);
    document.getElementById('vocab-anki-sync')?.addEventListener('click', () => { void openAnkiSyncModal(); });
    document.getElementById('vocab-anki-settings')?.addEventListener('click', (e) => {
        e.preventDefault();
        openAnkiSettingsModal();
    });
    document.getElementById('anki-import-open-settings')?.addEventListener('click', (e) => {
        e.preventDefault();
        openAnkiSettingsModal();
    });
    document.getElementById('vocab-anki-help')?.addEventListener('click', (e) => {
        e.preventDefault();
        openAnkiSetupModal();
    });
    // Settings popup close + toggle handlers.
    document.getElementById('anki-settings-modal-close')?.addEventListener('click', closeAnkiSettingsModal);
    document.getElementById('anki-settings-modal-done')?.addEventListener('click', closeAnkiSettingsModal);
    document.getElementById('anki-settings-modal-backdrop')?.addEventListener('click', closeAnkiSettingsModal);
    document.getElementById('anki-settings-reset')?.addEventListener('click', onSettingsResetDefaults);
    document.getElementById('anki-settings-include-new')?.addEventListener('change', (e) => {
        const t = e.target;
        if (t)
            onSettingsIncludeNewToggle(t.checked);
    });
    document.getElementById('anki-settings-include-suspended')?.addEventListener('change', (e) => {
        const t = e.target;
        if (t)
            onSettingsIncludeSuspendedToggle(t.checked);
    });
    document.getElementById('anki-settings-replace-mode')?.addEventListener('change', (e) => {
        const t = e.target;
        if (t)
            onSettingsReplaceModeToggle(t.checked);
    });
    document.getElementById('anki-settings-preserve-manual')?.addEventListener('change', (e) => {
        const t = e.target;
        if (t)
            onSettingsPreserveManualToggle(t.checked);
    });
    document.getElementById('anki-settings-skip-confirm')?.addEventListener('change', (e) => {
        const t = e.target;
        if (t)
            onSettingsSkipConfirmToggle(t.checked);
    });
    // Setup-instructions modal close + copy.
    document.getElementById('anki-setup-modal-close')?.addEventListener('click', closeAnkiSetupModal);
    document.getElementById('anki-setup-modal-done')?.addEventListener('click', closeAnkiSetupModal);
    document.getElementById('anki-setup-modal-backdrop')?.addEventListener('click', closeAnkiSetupModal);
    document.getElementById('anki-setup-copy-btn')?.addEventListener('click', () => {
        void copyAnkiSetupConfig();
    });
    // Import modal - close handlers.
    document.getElementById('anki-import-modal-close')?.addEventListener('click', closeAnkiImportModal);
    document.getElementById('anki-import-modal-backdrop')?.addEventListener('click', closeAnkiImportModal);
    document.getElementById('anki-import-cancel')?.addEventListener('click', closeAnkiImportModal);
    document.getElementById('anki-import-done')?.addEventListener('click', closeAnkiImportModal);
    // Import modal - filter input + clear.
    const filterInput = document.getElementById('anki-import-filter');
    filterInput?.addEventListener('input', () => onAnkiFilterInput(filterInput.value));
    document.getElementById('anki-import-clear-filter')?.addEventListener('click', () => {
        if (filterInput)
            filterInput.value = '';
        onAnkiFilterInput('');
    });
    // Import modal - deck tree click delegation.
    document.getElementById('anki-import-tree')?.addEventListener('click', (e) => {
        const target = e.target;
        if (!target)
            return;
        const toggle = target.closest('[data-deck-toggle]');
        if (toggle) {
            const name = toggle.getAttribute('data-deck-toggle');
            if (name)
                onAnkiDeckToggle(name);
        }
    });
    document.getElementById('anki-import-tree')?.addEventListener('change', (e) => {
        const target = e.target;
        if (!target || target.type !== 'checkbox')
            return;
        const name = target.getAttribute('data-deck-check');
        if (name)
            onAnkiDeckCheck(name, target.checked);
    });
    // Import modal - step transitions.
    document.getElementById('anki-import-next')?.addEventListener('click', () => {
        if (ankiImport.selected.size === 0) {
            showToast('Pick at least one deck to continue.', 'error');
            return;
        }
        void loadAnkiModelsForSelection();
    });
    document.getElementById('anki-import-back')?.addEventListener('click', () => {
        showAnkiStage('decks');
    });
    document.getElementById('anki-import-run')?.addEventListener('click', () => {
        void runAnkiImport();
    });
    // (The four behavioural toggles - include-new, include-suspended,
    // replace-mode, preserve-manual - now live inside the Anki settings
    // popup. Their handlers are wired above with the settings popup.)
    // Import modal - custom field picker (click to open/close + select).
    const fieldsContainer = document.getElementById('anki-import-fields');
    fieldsContainer?.addEventListener('click', (e) => {
        const target = e.target;
        if (!target)
            return;
        const toggle = target.closest('[data-field-toggle]');
        if (toggle) {
            const model = toggle.getAttribute('data-field-toggle');
            if (model)
                toggleFieldPicker(model);
            return;
        }
        const option = target.closest('[data-field-option]');
        if (option) {
            const model = option.getAttribute('data-field-option');
            const field = option.getAttribute('data-field-value') || '';
            if (model !== null) {
                onAnkiFieldPick(model, field);
                closeAllFieldPickers();
            }
        }
    });
    // Hover on an option → show its examples in a tooltip to the right.
    fieldsContainer?.addEventListener('mouseover', (e) => {
        const target = e.target;
        const option = target?.closest('[data-field-option]');
        if (option)
            showFieldExamplesTip(option);
    });
    fieldsContainer?.addEventListener('mouseout', (e) => {
        const target = e.target;
        const option = target?.closest('[data-field-option]');
        const related = e.relatedTarget;
        if (option && !related?.closest('[data-field-option]'))
            hideFieldExamplesTip();
    });
    // Click outside any open picker closes them all.
    document.addEventListener('click', (e) => {
        const target = e.target;
        if (target?.closest('.field-picker'))
            return;
        closeAllFieldPickers();
    });
    // The menu uses position:fixed (so it escapes the modal/fields-container
    // overflow boxes); when the underlying layout moves, close the menu so it
    // doesn't visually detach from its anchor.
    window.addEventListener('resize', () => closeAllFieldPickers());
    document.getElementById('anki-import-fields')?.addEventListener('scroll', () => closeAllFieldPickers());
    document.querySelector('#anki-import-modal .modal-card')?.addEventListener('scroll', () => closeAllFieldPickers());
    // Escape closes whichever Anki modal is open. Order matters: the
    // settings popup can stack on TOP of the import modal (z-index 2100),
    // so close it first if it's open.
    document.addEventListener('keydown', (e) => {
        if (e.key !== 'Escape')
            return;
        const settings = document.getElementById('anki-settings-modal');
        if (settings && !settings.classList.contains('hidden')) {
            closeAnkiSettingsModal();
            return;
        }
        const setup = document.getElementById('anki-setup-modal');
        if (setup && !setup.classList.contains('hidden')) {
            closeAnkiSetupModal();
            return;
        }
        const importModal = document.getElementById('anki-import-modal');
        if (importModal && !importModal.classList.contains('hidden'))
            closeAnkiImportModal();
    });
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
    document.getElementById('review-flag')?.addEventListener('click', () => {
        void flagReviewCard();
    });
    document.getElementById('review-skip')?.addEventListener('click', () => {
        void loadNextReviewCard(true);
    });
}
// flagReviewCard submits flag-only parser feedback from the review card back -
// the "None of these looks right" escape (USER_FLOWS §9.4). It is parser
// feedback ("the analysis looks wrong"), never a study/known action. The
// feedback endpoint creates a lazy parse_session from the inline source_text
// (the card's example or surface) so admin triage keeps context.
async function flagReviewCard() {
    const card = state.currentReviewCard;
    if (!card)
        return;
    const surface = card.back.surface || card.back.lemma;
    const lang = card.back.lang || state.activeLanguage;
    const context = card.back.examples?.[0]?.text || card.front.text || surface;
    try {
        const resp = await fetch('/api/parse/feedback', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'same-origin',
            body: JSON.stringify({
                lang,
                parser: 'custom',
                surface,
                occurrence: 0,
                original_lemma: card.back.lemma,
                original_pos: card.back.pos || '',
                flag_only: true,
                source_text: context,
                total_tokens: 0,
                unique_lemma_count: 0,
            }),
        });
        if (resp.ok) {
            showToast('Thanks - flagged for review.', 'success');
        }
        else {
            showToast("Couldn't flag this card - please try again later.", 'error');
        }
    }
    catch {
        showToast("Couldn't flag this card - check your connection and try again.", 'error');
    }
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
    const flagFilter = document.getElementById('admin-feedback-flag-only');
    flagFilter?.addEventListener('change', async () => {
        state.adminFeedbackFlagOnly = flagFilter.value;
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
    const issuesFilter = document.getElementById('admin-issues-status');
    issuesFilter?.addEventListener('change', async () => {
        state.adminIssuesStatus = issuesFilter.value;
        await loadAdminIssues();
    });
    document.getElementById('admin-issues-list')?.addEventListener('click', (e) => {
        const target = e.target;
        if (!target)
            return;
        const action = target.getAttribute('data-issue-action');
        const id = Number(target.getAttribute('data-issue-id') || 0);
        if (action && id > 0)
            void actOnAdminIssue(id, action);
    });
}
// Portal-style tooltip - pseudo-element ::after tooltips get clipped by
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
// ── Test surface ───────────────────────────────────────────────────────────
//
// Pure helpers exposed on `window.__finestTest` so the Playwright spec can
// exercise them via `page.evaluate()` without standing up a full e2e flow.
// Adds a tiny constant to the bundle; not used by the app itself.
window.__finestTest = {
    buildAnkiDeckTree,
    deckMatchesFilter,
    pickBestField,
    defaultAnkiFilter,
    parseFileWords,
    loadAnkiPrefs,
    saveAnkiPrefs,
    ankiPrefsKey,
    cleanAnkiSurfaceForm,
};
// ── Init ───────────────────────────────────────────────────────────────────
document.addEventListener('DOMContentLoaded', async () => {
    initTheme();
    initMobileNav();
    initSigninForm();
    initLandingForm();
    initInspectForm();
    initWorkbenchForm();
    initAnonResultsChrome();
    initResultsExport();
    preventStrayFileDrops();
    initCorrectionModal();
    initDecksPage();
    initKnownWordsPanel();
    initHistoryPage();
    initLanguagesPage();
    initNavLanguageSelector();
    initVocabFileImport();
    initVocabAnkiImport();
    initReviewPage();
    initResultsSaveForm();
    initResultsTabs();
    initAdminFeedbackPage();
    initPortalTooltips();
    initThemePicker();
    document.getElementById('nav-signout')?.addEventListener('click', handleSignout);
    document.getElementById('nav-mobile-signout')?.addEventListener('click', handleSignout);
    document.getElementById('account-delete')?.addEventListener('click', handleDeleteAccount);
    document.getElementById('results-back')?.addEventListener('click', () => {
        if (state.currentContext === 'deck') {
            navigate('/decks');
            return;
        }
        if (state.currentContext === 'landing') {
            navigate('/');
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
