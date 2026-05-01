// FinEstDB — frontend with three role-aware surfaces:
//   anonymous landing/about/signin, authenticated user product, admin workbench.

const MAX_CHARS = 300_000;

const POS_LABELS: Record<string, string> = {
    NOUN:  'Noun',
    VERB:  'Verb',
    ADJ:   'Adjective',
    ADV:   'Adverb',
    PRON:  'Pronoun',
    DET:   'Determiner',
    ADP:   'Adposition',
    NUM:   'Numeral',
    PUNCT: 'Punctuation',
    SYM:   'Symbol',
    INTJ:  'Interjection',
    CCONJ: 'Conjunction',
    SCONJ: 'Conjunction',
    PART:  'Particle',
    AUX:   'Auxiliary',
    PROPN: 'Proper noun',
    X:     'Other',
};

interface WordEntry {
    lemma:             string;
    pos:               string;
    forms:             string[];
    count:             number;
    gloss?:            string;
    grammar_label?:    string;
    example_sentence?: string;
    learning_state?:   LemmaStateStatus;
}

interface ParseResponse {
    lang:              string;
    parse_id?:         number;
    total_tokens:      number;
    parse_duration_ms: number;
    words:             WordEntry[];
}

interface CorrectionRowContext {
    lemma:                  string;
    pos:                    string;
    surface:                string;
    occurrence:             number;
    original_grammar_label: string;
}

interface SessionUser {
    id:       number;
    email:    string;
    is_admin: boolean;
}

interface DashboardData {
    known_count:        number;
    due_count:          number;
    new_capacity_today: number;
    decks:              DeckSummary[];
}

interface DeckSummary {
    id:     number;
    title:  string;
    lang:   string;
    known:  number;
    unique: number;
    due:    number;
}

interface MeResponse {
    authenticated: boolean;
    user:          SessionUser | null;
    dashboard?:    DashboardData;
}

interface LoginResponse {
    authenticated: boolean;
    user:          SessionUser | null;
}

interface CreateDeckResponse {
    deck_id: number;
}

interface DeckListResponse {
    decks: DeckSummary[];
}

interface ReviewCardResponse {
    card_id:     string;
    mode:        string;
    deck_counts: string[][];
    front: {
        type:      string;
        text:      string;
        highlight?: string;
    };
    back: {
        lemma:   string;
        meaning: string;
        grammar: string;
        examples: Array<{
            text: string;
            source_deck: string;
        }>;
    };
}

interface ParseFeedbackItem {
    id:                       number;
    parse_session_id:         number;
    user_id:                  number;
    lang:                     string;
    parser:                   string;
    surface:                  string;
    occurrence:               number;
    original_lemma:           string;
    original_pos:             string;
    original_grammar_label?:  string;
    proposed_lemma:           string;
    proposed_pos:             string;
    proposed_grammar_label?:  string;
    note?:                    string;
    status:                   string;
    review_note?:             string;
    reviewed_by_user_id?:     number;
    reviewed_at?:             string;
    created_at:               string;
}

interface ParseFeedbackListResponse {
    feedback: ParseFeedbackItem[];
}

type Role = 'anon' | 'user' | 'admin';
type SortKey = 'row' | 'lemma' | 'pos' | 'forms' | 'definition' | 'tokens';
type POSFilter = 'all' | 'NOUN' | 'VERB' | 'ADJ' | 'ADV' | 'other';
type ParserMode = 'basic' | 'custom';
type ResultsContext = 'inspect' | 'workbench';
type LemmaStateStatus = 'known' | 'ignored';

interface SortState { key: SortKey; dir: 'asc' | 'desc'; }
interface DisplayWordEntry extends WordEntry { originalIndex: number; }

// ── App-wide state ─────────────────────────────────────────────────────────

const state = {
    user:        null as SessionUser | null,
    dashboard:   null as DashboardData | null,
    decks:       [] as DeckSummary[],
    role:        'anon' as Role,
    currentResults:     null as ParseResponse | null,
    currentContext:     'inspect' as ResultsContext,
    currentParserMode:  'basic' as ParserMode,
    currentTextPreview: '',
    currentSourceText:  '',
    currentRow:         null as CorrectionRowContext | null,
    currentSort:        { key: 'row', dir: 'asc' } as SortState,
    currentPOSFilter:   'all' as POSFilter,
    currentLemmaStates: new Map<string, LemmaStateStatus>(),
    pendingLemmaStates: new Set<string>(),
    currentReviewCard:  null as ReviewCardResponse | null,
    reviewDeckFilter:   '',
    adminFeedback:      [] as ParseFeedbackItem[],
    adminFeedbackStatus: 'submitted',
};

const NOUN_POS = ['NOUN', 'PROPN'];
const VERB_POS = ['VERB', 'AUX'];
const ADJ_POS = ['ADJ'];
const ADV_POS = ['ADV'];
const OTHER_POS = ['PRON', 'DET', 'ADP', 'NUM', 'CCONJ', 'SCONJ', 'PART', 'INTJ', 'X', 'SYM', 'PUNCT'];

// ── Theme ──────────────────────────────────────────────────────────────────

function initTheme(): void {
    const saved = localStorage.getItem('theme') || 'light';
    applyTheme(saved);
}

function applyTheme(theme: string): void {
    document.documentElement.setAttribute('data-theme', theme);
    document.querySelectorAll('.theme-icon').forEach(el => {
        el.textContent = theme === 'light' ? '🌙' : '☀️';
    });
}

function toggleTheme(): void {
    const current = document.documentElement.getAttribute('data-theme') || 'light';
    const next = current === 'light' ? 'dark' : 'light';
    localStorage.setItem('theme', next);
    applyTheme(next);
}

// ── Toast notifications ────────────────────────────────────────────────────

function showToast(message: string, type: 'info' | 'success' | 'error' = 'info', duration = 3000): void {
    const container = document.getElementById('toast-container');
    if (!container) return;

    const toast = document.createElement('div');
    toast.className = `toast ${type}`;
    toast.textContent = message;
    container.appendChild(toast);

    setTimeout(() => {
        toast.classList.add('hiding');
        setTimeout(() => toast.remove(), 300);
    }, duration);
}

// ── Helpers ────────────────────────────────────────────────────────────────

function escapeHtml(str: string): string {
    const d = document.createElement('div');
    d.textContent = str;
    return d.innerHTML;
}

function escapeAttr(str: string): string {
    return str
        .replace(/&/g, '&amp;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#39;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;');
}

function posLabel(pos: string): string {
    return POS_LABELS[pos] || pos;
}

function lemmaStateKey(lang: string, lemma: string, pos: string): string {
    return `${lang}\u0000${lemma}\u0000${pos}`;
}

function currentLemmaState(lemma: string, pos: string): LemmaStateStatus | undefined {
    const lang = state.currentResults?.lang || '';
    if (!lang) return undefined;
    return state.currentLemmaStates.get(lemmaStateKey(lang, lemma, pos));
}

function compareStrings(a: string, b: string): number {
    return a.localeCompare(b, undefined, { sensitivity: 'base' });
}

function setParseButtonsDisabled(disabled: boolean): void {
    document.querySelectorAll<HTMLButtonElement>('.btn-parse').forEach(btn => {
        btn.disabled = disabled;
    });
}

function formatParserMode(parserMode: ParserMode): string {
    return parserMode === 'custom' ? 'Custom parser' : 'Basic parser';
}

function formatParseDuration(parseDurationMs: number): string {
    if (parseDurationMs < 1000) return `${parseDurationMs} ms`;
    return `${(parseDurationMs / 1000).toFixed(parseDurationMs >= 10_000 ? 1 : 2)} s`;
}

// ── Auth / role ────────────────────────────────────────────────────────────

function computeRole(user: SessionUser | null): Role {
    if (!user) return 'anon';
    return user.is_admin ? 'admin' : 'user';
}

function applyRoleVisibility(): void {
    const role = state.role;
    document.body.dataset.authState = role;
    document.body.dataset.role = role;

    document.querySelectorAll<HTMLElement>('[data-role-show]').forEach(el => {
        const allowed = (el.dataset.roleShow || '').split(/\s+/).filter(Boolean);
        const visible = allowed.includes(role);
        el.classList.toggle('role-hidden', !visible);
    });
}

async function fetchMe(): Promise<void> {
    try {
        const resp = await fetch('/api/me', { credentials: 'same-origin' });
        if (!resp.ok) throw new Error(`/api/me ${resp.status}`);
        const data: MeResponse = await resp.json();
        applyMeResponse(data);
    } catch (err) {
        // Treat any failure as anonymous; we don't want to lock people out.
        applyMeResponse({ authenticated: false, user: null });
    }
}

function applyMeResponse(data: MeResponse): void {
    state.user = data.authenticated ? (data.user || null) : null;
    state.dashboard = data.dashboard || null;
    state.decks = data.dashboard?.decks || [];
    state.role = computeRole(state.user);

    const emailEl = document.getElementById('nav-user-email');
    if (emailEl) emailEl.textContent = state.user?.email || '';

    applyRoleVisibility();
    renderDashboard();
}

async function handleSignout(): Promise<void> {
    try {
        await fetch('/api/auth/logout', { method: 'POST', credentials: 'same-origin' });
    } catch {
        // Best-effort — even if the endpoint is missing, clear local state.
    }
    state.user = null;
    state.dashboard = null;
    state.decks = [];
    state.role = 'anon';
    state.currentResults = null;
    state.currentTextPreview = '';
    state.currentSourceText = '';
    state.currentParserMode = 'basic';
    state.currentContext = 'inspect';
    state.currentRow = null;
    state.currentLemmaStates.clear();
    state.currentReviewCard = null;
    state.reviewDeckFilter = '';
    clearResultsDom();
    applyRoleVisibility();
    showToast('Signed out', 'info');
    navigate('/');
}

function clearResultsDom(): void {
    const tbody = document.getElementById('word-table-body');
    if (tbody) tbody.innerHTML = '';
    const setText = (id: string, text: string) => {
        const el = document.getElementById(id);
        if (el) el.textContent = text;
    };
    setText('results-title', '');
    setText('results-duration', '');
    setText('results-stats', '');
    setText('results-parser', '');
    const inspectInput  = document.getElementById('inspect-text') as HTMLTextAreaElement | null;
    const workbenchText = document.getElementById('parse-text')   as HTMLTextAreaElement | null;
    if (inspectInput)  inspectInput.value = '';
    if (workbenchText) workbenchText.value = '';
}

// ── Hash router ────────────────────────────────────────────────────────────

const ROUTE_TO_PAGE: Record<string, string> = {
    '/':                 'landing-page',
    '/about':            'about-page',
    '/signin':           'signin-page',
    '/dashboard':        'dashboard-page',
    '/inspect':          'inspect-page',
    '/decks':            'decks-page',
    '/review':           'review-page',
    '/admin/workbench':  'admin-workbench-page',
    '/admin/feedback':   'admin-feedback-page',
    '/results':          'results-page',
};

function currentRoute(): string {
    const hash = window.location.hash || '#/';
    return hash.startsWith('#') ? hash.slice(1) : hash;
}

function navigate(route: string): void {
    const target = route.startsWith('/') ? route : `/${route}`;
    if (currentRoute() === target) {
        renderRoute();
        return;
    }
    window.location.hash = `#${target}`;
}

function showPage(id: string): void {
    document.querySelectorAll('.page').forEach(p => p.classList.remove('active'));
    const el = document.getElementById(id);
    if (el) el.classList.add('active');
    window.scrollTo({ top: 0, behavior: 'instant' as ScrollBehavior });
}

function setActiveNavLink(route: string): void {
    document.querySelectorAll<HTMLElement>('[data-route]').forEach(el => {
        el.classList.toggle('active', el.dataset.route === route);
    });
}

function isRouteAllowed(route: string): { allowed: boolean; redirect?: string } {
    const role = state.role;

    // Admin-only routes
    if (route.startsWith('/admin/')) {
        if (role === 'admin') return { allowed: true };
        if (role === 'anon')  return { allowed: false, redirect: '/signin' };
        return { allowed: false, redirect: '/dashboard' };
    }

    // Authenticated-only routes
    const userOnly = ['/dashboard', '/inspect', '/decks', '/review', '/results'];
    if (userOnly.includes(route)) {
        if (role === 'anon') return { allowed: false, redirect: '/signin' };
        return { allowed: true };
    }

    // Anonymous-only: signed-in users hitting / or /signin land on dashboard
    if ((route === '/' || route === '/signin') && (role === 'user' || role === 'admin')) {
        return { allowed: false, redirect: '/dashboard' };
    }

    return { allowed: true };
}

function renderRoute(): void {
    let route = currentRoute();
    if (!ROUTE_TO_PAGE[route]) route = '/';

    const guard = isRouteAllowed(route);
    if (!guard.allowed && guard.redirect) {
        window.location.hash = `#${guard.redirect}`;
        return;
    }

    showPage(ROUTE_TO_PAGE[route]);
    setActiveNavLink(route);
    closeMobileNav();

    // Per-page hooks
    if (route === '/dashboard') renderDashboard();
    if (route === '/decks') renderDecksPage();
    if (route === '/review') {
        renderReviewPage();
        void loadNextReviewCard(false);
    }
    if (route === '/admin/feedback') {
        renderAdminFeedbackPage();
        void loadAdminFeedback();
    }
}

// ── Mobile nav ─────────────────────────────────────────────────────────────

function initMobileNav(): void {
    const hamburger = document.getElementById('nav-hamburger');
    const overlay   = document.getElementById('nav-mobile-overlay');
    if (!hamburger || !overlay) return;

    hamburger.addEventListener('click', () => {
        const isOpen = !overlay.classList.contains('hidden');
        overlay.classList.toggle('hidden', isOpen);
        hamburger.classList.toggle('open', !isOpen);
        document.body.classList.toggle('nav-open', !isOpen);
    });

    overlay.addEventListener('click', e => {
        if (e.target === overlay) closeMobileNav();
    });

    overlay.querySelectorAll('.nav-mobile-link').forEach(link => {
        link.addEventListener('click', closeMobileNav);
    });
}

function closeMobileNav(): void {
    const hamburger = document.getElementById('nav-hamburger');
    const overlay   = document.getElementById('nav-mobile-overlay');
    overlay?.classList.add('hidden');
    hamburger?.classList.remove('open');
    document.body.classList.remove('nav-open');
}

// ── Sign-in flow ───────────────────────────────────────────────────────────

function initSigninForm(): void {
    const form = document.getElementById('signin-form') as HTMLFormElement | null;
    if (!form) return;

    form.addEventListener('submit', async (e) => {
        e.preventDefault();
        const emailEl    = document.getElementById('signin-email')    as HTMLInputElement;
        const passwordEl = document.getElementById('signin-password') as HTMLInputElement;
        const errorEl    = document.getElementById('signin-error')!;
        const submitBtn  = document.getElementById('signin-submit')   as HTMLButtonElement;

        const email = emailEl.value.trim();
        if (!email) {
            errorEl.textContent = 'Please enter your email.';
            errorEl.classList.remove('hidden');
            return;
        }

        errorEl.classList.add('hidden');
        submitBtn.disabled = true;
        const origLabel = submitBtn.textContent || '';
        submitBtn.textContent = 'Signing in…';

        try {
            const resp = await fetch('/api/auth/login', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                credentials: 'same-origin',
                body: JSON.stringify({ email, password: passwordEl.value }),
            });

            if (!resp.ok) {
                const msg = await resp.text();
                throw new Error(msg || 'Sign-in failed');
            }

            const data: LoginResponse = await resp.json();
            if (!data.authenticated || !data.user) {
                throw new Error('Sign-in failed');
            }

            // Refresh /api/me so we get the dashboard payload too.
            await fetchMe();
            showToast(`Welcome, ${data.user.email}`, 'success');
            navigate('/dashboard');
        } catch (err: any) {
            errorEl.textContent = err.message || 'Sign-in failed';
            errorEl.classList.remove('hidden');
        } finally {
            submitBtn.disabled = false;
            submitBtn.textContent = origLabel;
        }
    });
}

// ── Dashboard ──────────────────────────────────────────────────────────────

function renderDashboard(): void {
    const greeting = document.getElementById('dashboard-greeting-suffix');
    if (greeting) {
        greeting.textContent = state.user ? `, ${state.user.email.split('@')[0]}` : '';
    }

    const setStat = (id: string, value: number | undefined): void => {
        const el = document.getElementById(id);
        if (!el) return;
        el.textContent = value === undefined ? '—' : value.toLocaleString();
    };

    setStat('stat-known',        state.dashboard?.known_count);
    setStat('stat-due',          state.dashboard?.due_count);
    setStat('stat-new-capacity', state.dashboard?.new_capacity_today);

    const decksList = document.getElementById('dashboard-decks-list');
    if (!decksList) return;

    const decks = state.dashboard?.decks || [];
    if (decks.length === 0) {
        decksList.innerHTML = `<p class="empty-state">No decks yet — paste some text under <a href="#/inspect">Inspect</a> to get started.</p>`;
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

async function refreshDashboardData(options: { rerenderRoute?: boolean } = {}): Promise<void> {
    await fetchMe();
    if (options.rerenderRoute !== false) {
        renderRoute();
    }
}

async function markResultLemma(status: LemmaStateStatus, lemma: string, pos: string, trigger: HTMLButtonElement): Promise<void> {
    const lang = state.currentResults?.lang;
    if (!lang) return;

    const stateKey = lemmaStateKey(lang, lemma, pos);
    if (state.pendingLemmaStates.has(stateKey)) return;
    state.pendingLemmaStates.add(stateKey);
    const originalLabel = trigger.textContent || '';
    trigger.disabled = true;
    trigger.textContent = status === 'known' ? 'Marking...' : 'Ignoring...';
    if (state.currentResults) {
        renderResultsTable(state.currentResults);
    }
    try {
        const resp = await fetch('/api/lemma-state', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'same-origin',
            body: JSON.stringify({ lang, lemma, pos, status }),
        });
        if (!resp.ok) {
            throw new Error(await resp.text() || 'Failed to update word');
        }

        state.currentLemmaStates.set(stateKey, status);
        await refreshDashboardData({ rerenderRoute: false });
        showToast(status === 'known' ? 'Word marked known.' : 'Word ignored.', 'success');
    } catch (err: any) {
        showToast(err.message || 'Failed to update word.', 'error');
        trigger.disabled = false;
        trigger.textContent = originalLabel;
    } finally {
        state.pendingLemmaStates.delete(stateKey);
        if (state.currentResults) {
            renderResultsTable(state.currentResults);
        }
    }
}

function renderDecksPage(): void {
    const empty = document.getElementById('decks-empty');
    const list = document.getElementById('decks-list');
    if (!empty || !list) return;

    const decks = state.decks || [];
    empty.classList.toggle('hidden', decks.length > 0);
    list.classList.toggle('hidden', decks.length === 0);

    if (decks.length === 0) {
        list.innerHTML = '';
        return;
    }

    list.innerHTML = decks.map(deck => {
        const langName = deck.lang === 'FI' ? 'Finnish' : 'Estonian';
        const knownPct = deck.unique > 0 ? Math.round((deck.known / deck.unique) * 100) : 0;
        return `<article class="deck-list-item">
            <div>
                <h2>${escapeHtml(deck.title)}</h2>
                <p class="deck-list-meta">${langName} · ${deck.known}/${deck.unique} known (${knownPct}%) · ${deck.due} due</p>
            </div>
            <div class="deck-list-actions">
                <button type="button" class="btn btn-link btn-sm" data-open-review="${deck.id}">Review</button>
                <button type="button" class="btn btn-link btn-sm" data-rename-deck="${deck.id}">Rename</button>
                <button type="button" class="btn btn-link btn-sm" data-delete-deck="${deck.id}">Delete</button>
            </div>
        </article>`;
    }).join('');
}

function getDeckByID(deckID: number): DeckSummary | undefined {
    return state.decks.find(deck => deck.id === deckID);
}

// ── Language detection (shared between inspect + workbench) ────────────────

function detectLang(text: string): 'FI' | 'ET' | 'unknown' {
    const lower = text.toLowerCase();
    const letters = lower.match(/[a-zäöüõ]/g) || [];
    if (letters.length === 0) return 'unknown';
    if (/õ/.test(lower)) return 'ET';
    const nordicCount = (lower.match(/[äö]/g) || []).length;
    if (nordicCount / letters.length > 0.015) return 'FI';
    return 'unknown';
}

interface LangWarningState {
    detected: 'FI' | 'ET' | 'unknown';
    message: string | null;
    canSwitch: boolean;
    blocksParse: boolean;
}

function getLangWarningState(text: string, selectedLang: string): LangWarningState {
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

// ── Inspect form (user surface) ────────────────────────────────────────────

interface ParseFormElements {
    lang:        HTMLSelectElement;
    text:        HTMLTextAreaElement;
    file:        HTMLInputElement;
    charCount:   HTMLElement;
    warning:     HTMLElement;
    switchBtn:   HTMLButtonElement;
}

function getInspectEls(): ParseFormElements | null {
    const lang     = document.getElementById('inspect-lang')         as HTMLSelectElement | null;
    const text     = document.getElementById('inspect-text')         as HTMLTextAreaElement | null;
    const file     = document.getElementById('inspect-file')         as HTMLInputElement | null;
    const cc       = document.getElementById('inspect-char-count');
    const warn     = document.getElementById('inspect-lang-warning');
    const swBtn    = document.getElementById('inspect-lang-switch') as HTMLButtonElement | null;
    if (!lang || !text || !file || !cc || !warn || !swBtn) return null;
    return { lang, text, file, charCount: cc, warning: warn, switchBtn: swBtn };
}

function getWorkbenchEls(): ParseFormElements | null {
    const lang  = document.getElementById('parse-lang')    as HTMLSelectElement | null;
    const text  = document.getElementById('parse-text')    as HTMLTextAreaElement | null;
    const file  = document.getElementById('parse-file')    as HTMLInputElement | null;
    const cc    = document.getElementById('char-count');
    const warn  = document.getElementById('lang-warning');
    const swBtn = document.getElementById('lang-switch-btn') as HTMLButtonElement | null;
    if (!lang || !text || !file || !cc || !warn || !swBtn) return null;
    return { lang, text, file, charCount: cc, warning: warn, switchBtn: swBtn };
}

function updateCharCount(els: ParseFormElements): void {
    const count = els.text.value.length;
    els.charCount.textContent = `${count.toLocaleString()} / ${MAX_CHARS.toLocaleString()}`;
    els.charCount.classList.toggle('char-count-warn', count > MAX_CHARS * 0.9);
    els.charCount.classList.toggle('char-count-over', count >= MAX_CHARS);
}

function updateLangWarning(els: ParseFormElements, gateInspectButton: boolean): void {
    if (els.text.value.trim().length < 20) {
        els.warning.classList.add('hidden');
        els.switchBtn.classList.add('hidden');
        if (gateInspectButton) gateSubmit('inspect-submit', false);
        else setParseButtonsDisabled(false);
        return;
    }

    const ws = getLangWarningState(els.text.value, els.lang.value);
    if (ws.message) {
        els.warning.textContent = ws.message;
        els.warning.classList.remove('hidden');
        if (ws.canSwitch) {
            els.switchBtn.textContent = `Switch to ${ws.detected === 'FI' ? 'Finnish' : 'Estonian'}`;
            els.switchBtn.classList.remove('hidden');
        } else {
            els.switchBtn.classList.add('hidden');
        }
    } else {
        els.warning.classList.add('hidden');
        els.switchBtn.classList.add('hidden');
    }

    if (gateInspectButton) gateSubmit('inspect-submit', ws.blocksParse);
    else setParseButtonsDisabled(ws.blocksParse);
}

function gateSubmit(id: string, disabled: boolean): void {
    const btn = document.getElementById(id) as HTMLButtonElement | null;
    if (btn) btn.disabled = disabled;
}

function initInspectForm(): void {
    const els = getInspectEls();
    if (!els) return;

    els.text.addEventListener('input', () => {
        updateCharCount(els);
        updateLangWarning(els, true);
    });

    els.text.addEventListener('paste', () => {
        setTimeout(() => {
            const text = els.text.value;
            if (text.trim().length < 20) return;
            const detected = detectLang(text);
            if (detected !== 'unknown' && detected !== els.lang.value) {
                els.lang.value = detected;
                showToast(`Switched to ${detected === 'FI' ? 'Finnish' : 'Estonian'} — detected from text`, 'info');
                updateLangWarning(els, true);
            }
        }, 0);
    });

    els.lang.addEventListener('change', () => updateLangWarning(els, true));

    els.switchBtn.addEventListener('click', () => {
        const ws = getLangWarningState(els.text.value, els.lang.value);
        if (ws.canSwitch) {
            els.lang.value = ws.detected;
            updateLangWarning(els, true);
        }
    });

    els.file.addEventListener('change', async (e) => {
        const input = e.target as HTMLInputElement;
        const file = input.files?.[0];
        if (!file) return;
        const text = await file.text();
        els.text.value = text.slice(0, MAX_CHARS);
        updateCharCount(els);
        updateLangWarning(els, true);
    });

    const form = document.getElementById('inspect-form') as HTMLFormElement | null;
    form?.addEventListener('submit', async (e) => {
        e.preventDefault();
        await runParse(els, 'custom', 'inspect', 'inspect-submit');
    });
}

// ── Workbench form (admin surface — keeps prior behavior) ──────────────────

function initWorkbenchForm(): void {
    const els = getWorkbenchEls();
    if (!els) return;

    els.text.addEventListener('input', () => {
        updateCharCount(els);
        updateLangWarning(els, false);
    });

    els.text.addEventListener('paste', () => {
        setTimeout(() => {
            const text = els.text.value;
            if (text.trim().length < 20) return;
            const detected = detectLang(text);
            if (detected !== 'unknown' && detected !== els.lang.value) {
                els.lang.value = detected;
                showToast(`Switched to ${detected === 'FI' ? 'Finnish' : 'Estonian'} — detected from text`, 'info');
                updateLangWarning(els, false);
            }
        }, 0);
    });

    els.lang.addEventListener('change', () => updateLangWarning(els, false));

    els.switchBtn.addEventListener('click', () => {
        const ws = getLangWarningState(els.text.value, els.lang.value);
        if (ws.canSwitch) {
            els.lang.value = ws.detected;
            updateLangWarning(els, false);
        }
    });

    els.file.addEventListener('change', async (e) => {
        const input = e.target as HTMLInputElement;
        const file = input.files?.[0];
        if (!file) return;
        const text = await file.text();
        els.text.value = text.slice(0, MAX_CHARS);
        updateCharCount(els);
        updateLangWarning(els, false);
    });

    const handle = (mode: ParserMode, btnId: string) => async (e: Event) => {
        e.preventDefault();
        await runParse(els, mode, 'workbench', btnId);
    };

    document.getElementById('parse-btn-basic') ?.addEventListener('click', handle('basic',  'parse-btn-basic'));
    document.getElementById('parse-btn-custom')?.addEventListener('click', handle('custom', 'parse-btn-custom'));

    document.getElementById('parse-form')?.addEventListener('submit', (e) => {
        e.preventDefault();
        // Submitting via Enter behaves like the basic button.
        runParse(els, 'basic', 'workbench', 'parse-btn-basic');
    });
}

// ── Shared parse runner ────────────────────────────────────────────────────

async function runParse(
    els: ParseFormElements,
    parserMode: ParserMode,
    context: ResultsContext,
    activeBtnId: string,
): Promise<void> {
    const text = els.text.value.trim();
    const lang = els.lang.value;
    const ws = getLangWarningState(text, lang);

    if (!text) return;
    if (text.length > MAX_CHARS) {
        alert(`Text must be ${MAX_CHARS.toLocaleString()} characters or fewer.`);
        return;
    }
    if (ws.blocksParse) return;

    const activeBtn = document.getElementById(activeBtnId) as HTMLButtonElement | null;
    const origLabel = activeBtn?.textContent || '';

    // Disable all parse buttons in the current form
    if (context === 'workbench') setParseButtonsDisabled(true);
    if (activeBtn) {
        activeBtn.disabled = true;
        activeBtn.textContent = context === 'inspect' ? 'Inspecting…' : 'Parsing…';
    }

    try {
        const resp = await fetch('/api/parse', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'same-origin',
            body: JSON.stringify({ lang, text, parser: parserMode }),
        });
        if (!resp.ok) {
            const msg = await resp.text();
            throw new Error(msg || resp.statusText);
        }
        const data: ParseResponse = await resp.json();
        state.currentSourceText = text;
        showResults(data, text.slice(0, 60), parserMode, context);
    } catch (err: any) {
        alert(`Parse failed: ${err.message}`);
    } finally {
        if (activeBtn) {
            activeBtn.disabled = false;
            activeBtn.textContent = origLabel;
        }
        if (context === 'workbench') {
            updateLangWarning(els, false);
        } else {
            updateLangWarning(els, true);
        }
    }
}

// ── Results rendering (shared) ─────────────────────────────────────────────

function computeCoverageScore(data: ParseResponse): {
    score: number;
    definedRows: number;
    definedTokens: number;
    rowCoverage: number;
    tokenCoverage: number;
} {
    const definedRows = data.words.filter(word => Boolean(word.gloss)).length;
    const definedTokens = data.words.reduce((sum, word) => sum + (word.gloss ? word.count : 0), 0);
    const rowCoverage = data.words.length === 0 ? 0 : definedRows / data.words.length;
    const tokenCoverage = data.total_tokens === 0 ? 0 : definedTokens / data.total_tokens;
    const score = Math.round(((tokenCoverage * 0.7) + (rowCoverage * 0.3)) * 100);
    return { score, definedRows, definedTokens, rowCoverage, tokenCoverage };
}

function matchesPOSFilter(pos: string, filter: POSFilter): boolean {
    if (filter === 'all') return true;
    if (filter === 'NOUN') return NOUN_POS.includes(pos);
    if (filter === 'VERB') return VERB_POS.includes(pos);
    if (filter === 'ADJ') return ADJ_POS.includes(pos);
    if (filter === 'ADV') return ADV_POS.includes(pos);
    if (filter === 'other') return OTHER_POS.includes(pos);
    return true;
}

function filterWords(words: DisplayWordEntry[], filter: POSFilter): DisplayWordEntry[] {
    return filter === 'all' ? words : words.filter(w => matchesPOSFilter(w.pos, filter));
}

function sortWords(words: DisplayWordEntry[], sort: SortState): DisplayWordEntry[] {
    const sorted = [...words];
    const direction = sort.dir === 'asc' ? 1 : -1;
    sorted.sort((a, b) => {
        let cmp = 0;
        switch (sort.key) {
            case 'row':   cmp = a.originalIndex - b.originalIndex; break;
            case 'lemma': cmp = compareStrings(a.lemma, b.lemma); break;
            case 'pos':   cmp = compareStrings(posLabel(a.pos), posLabel(b.pos)); break;
            case 'forms': cmp = compareStrings(a.forms.join(', '), b.forms.join(', ')); break;
            case 'definition': {
                const aMissing = a.gloss ? 0 : 1;
                const bMissing = b.gloss ? 0 : 1;
                cmp = aMissing - bMissing;
                if (cmp === 0) cmp = compareStrings(a.gloss || '', b.gloss || '');
                break;
            }
            case 'tokens': cmp = a.count - b.count; break;
        }
        if (cmp === 0) cmp = a.originalIndex - b.originalIndex;
        return cmp * direction;
    });
    return sorted;
}

function highlightFormsInSentence(sentence: string, forms: string[]): string {
    let result = escapeHtml(sentence);
    for (const form of forms) {
        const escaped = escapeHtml(form);
        const regex = new RegExp(`\\b(${escaped})\\b`, 'gi');
        result = result.replace(regex, '<span class="highlight-form">$1</span>');
    }
    return result;
}

function updateSortButtons(): void {
    document.querySelectorAll<HTMLButtonElement>('.sort-btn').forEach(btn => {
        const key = btn.dataset.sort as SortKey | undefined;
        if (!key) return;
        const active = state.currentSort.key === key;
        btn.classList.toggle('active', active);
        btn.setAttribute('aria-sort', active ? state.currentSort.dir : 'none');
        const arrow = btn.querySelector<HTMLElement>('.sort-arrow');
        if (arrow) arrow.textContent = active ? (state.currentSort.dir === 'asc' ? ' ↑' : ' ↓') : '';
    });
}

function updatePOSFilterButtons(): void {
    document.querySelectorAll<HTMLButtonElement>('.pos-filter-chip').forEach(btn => {
        const filter = btn.dataset.filter as POSFilter | undefined;
        const active = filter === state.currentPOSFilter;
        btn.classList.toggle('active', active);
        btn.setAttribute('aria-pressed', active ? 'true' : 'false');
    });
}

function renderResultsTable(data: ParseResponse): void {
    const tbody = document.getElementById('word-table-body');
    const help  = document.getElementById('results-help');
    if (!tbody || !help) return;

    const baseWords = data.words.map((word, index) => ({ ...word, originalIndex: index }));
    const filteredWords = filterWords(baseWords, state.currentPOSFilter);
    const sortedWords = sortWords(filteredWords, state.currentSort);
    const hasGrammar = data.words.some(word => Boolean(word.grammar_label));
    const showActions = state.role === 'user' || state.role === 'admin';

    help.textContent = hasGrammar
        ? `Coverage = dictionary-backed tokens. Grammar labels shown as badges when case/morphology was inferred.`
        : `Coverage = dictionary-backed tokens. Grammar labels appear when case/morphology inference is available.`;

    tbody.innerHTML = sortedWords.map((w, index) => {
        const forms = w.forms.slice(0, 3).map(escapeHtml).join(', ')
            + (w.forms.length > 3 ? ` +${w.forms.length - 3}` : '');

        const grammarBadge = w.grammar_label
            ? `<span class="grammar-badge">${escapeHtml(w.grammar_label)}</span>`
            : '';

        const exampleHtml = w.example_sentence
            ? `<details class="example-details">
                <summary class="example-toggle">▸ example</summary>
                <span class="example-text">${highlightFormsInSentence(w.example_sentence, w.forms)}</span>
               </details>`
            : '';

        const glossHtml = w.gloss
            ? escapeHtml(w.gloss)
            : '<span class="no-gloss">Missing</span>';

        const posPill = `<span class="pos-pill" data-pos="${escapeHtml(w.pos)}">${escapeHtml(posLabel(w.pos))}</span>`;

        const surfaceForm = w.forms[0] || w.lemma;
        const rowStatus = currentLemmaState(w.lemma, w.pos);
        const rowPending = state.pendingLemmaStates.has(lemmaStateKey(data.lang, w.lemma, w.pos));
        const rowStatusHtml = rowStatus
            ? `<span class="word-state-badge ${rowStatus}">${rowStatus === 'known' ? 'Known' : 'Ignored'}</span>`
            : '';
        const actionCell = showActions
            ? `<td class="col-actions"><div class="word-actions">
                ${rowStatusHtml}
                <button type="button" class="btn btn-link btn-sm word-state-btn"
                    data-lemma-status="known"
                    data-lemma="${escapeHtml(w.lemma)}"
                    data-pos="${escapeHtml(w.pos)}"
                    ${rowPending || rowStatus === 'known' ? 'disabled' : ''}>Known</button>
                <button type="button" class="btn btn-link btn-sm word-state-btn"
                    data-lemma-status="ignored"
                    data-lemma="${escapeHtml(w.lemma)}"
                    data-pos="${escapeHtml(w.pos)}"
                    ${rowPending || rowStatus === 'ignored' ? 'disabled' : ''}>Ignore</button>
                <button type="button" class="btn btn-link btn-sm correction-btn"
                    data-lemma="${escapeHtml(w.lemma)}"
                    data-pos="${escapeHtml(w.pos)}"
                    data-surface="${escapeHtml(surfaceForm)}"
                    data-grammar="${escapeHtml(w.grammar_label || '')}">Suggest fix</button>
               </div></td>`
            : '';

        return `<tr>
            <td class="col-row">${index + 1}</td>
            <td class="col-lemma">${escapeHtml(w.lemma)}${grammarBadge}${exampleHtml}</td>
            <td class="col-pos">${posPill}</td>
            <td class="col-forms">${forms}</td>
            <td class="col-def">${glossHtml}</td>
            <td class="col-count">${w.count}</td>
            ${actionCell}
        </tr>`;
    }).join('');

    updateSortButtons();
    updatePOSFilterButtons();

    tbody.querySelectorAll<HTMLButtonElement>('.word-state-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            const status = btn.dataset.lemmaStatus as LemmaStateStatus | undefined;
            const lemma = btn.dataset.lemma || '';
            const pos = btn.dataset.pos || '';
            if (status && lemma && pos) {
                void markResultLemma(status, lemma, pos, btn);
            }
        });
    });

    // Wire up newly-rendered correction buttons
    tbody.querySelectorAll<HTMLButtonElement>('.correction-btn').forEach(btn => {
        btn.addEventListener('click', () => openCorrectionModal({
            lemma:                  btn.dataset.lemma   || '',
            pos:                    btn.dataset.pos     || '',
            surface:                btn.dataset.surface || btn.dataset.lemma || '',
            occurrence:             0,
            original_grammar_label: btn.dataset.grammar || '',
        }));
    });
}

function showResults(data: ParseResponse, textPreview: string, parserMode: ParserMode, context: ResultsContext): void {
    const langName = data.lang === 'FI' ? 'Finnish' : 'Estonian';
    const preview  = textPreview.replace(/\s+/g, ' ').trim();
    const ellipsis = preview.length >= 60 ? '…' : '';
    const uniqueLemmas = data.words.length;
    const coverage = computeCoverageScore(data);

    document.getElementById('results-title')!.textContent =
        `"${preview}${ellipsis}" (${langName})`;

    // For users, hide internal parser-mode pill; show a softer label.
    const parserPill = document.getElementById('results-parser')!;
    if (context === 'inspect' && state.role !== 'admin') {
        parserPill.textContent = 'Your text';
    } else {
        parserPill.textContent = formatParserMode(parserMode);
    }

    document.getElementById('results-duration')!.textContent =
        `Parse time ${formatParseDuration(data.parse_duration_ms)}`;

    const coverageFill  = document.getElementById('coverage-fill')!;
    const coverageValue = document.getElementById('coverage-value')!;
    coverageFill.style.width = `${coverage.score}%`;
    coverageFill.classList.remove('low', 'medium', 'high');
    coverageFill.classList.add(coverage.score >= 80 ? 'high' : coverage.score >= 50 ? 'medium' : 'low');
    coverageValue.textContent = `${coverage.score}%`;

    document.getElementById('results-stats')!.textContent =
        `${uniqueLemmas} unique lemmas · ${data.total_tokens} tokens · `
        + `${coverage.definedRows}/${uniqueLemmas} with definitions`;

    state.currentResults = data;
    state.currentContext = context;
    state.currentParserMode = parserMode;
    state.currentTextPreview = preview;
    state.currentSort = { key: 'row', dir: 'asc' };
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

    // Re-apply role visibility so admin-only pills/cells show correctly.
    applyRoleVisibility();

    navigate('/results');
}

function renderResultsSaveState(): void {
    const form = document.getElementById('results-save-form');
    const input = document.getElementById('results-deck-title') as HTMLInputElement | null;
    if (!form || !input) return;
    form.classList.add('hidden');
    const langName = state.currentResults?.lang === 'ET' ? 'Estonian' : 'Finnish';
    input.value = state.currentTextPreview
        ? `${langName}: ${state.currentTextPreview.slice(0, 48)}`
        : '';
}

async function saveCurrentResultsAsDeck(): Promise<void> {
    const titleInput = document.getElementById('results-deck-title') as HTMLInputElement | null;
    const submitBtn = document.getElementById('results-save-submit') as HTMLButtonElement | null;
    if (!titleInput || !submitBtn || !state.currentResults || !state.currentSourceText.trim()) return;

    const title = titleInput.value.trim();
    if (!title) {
        showToast('Please enter a deck title.', 'error');
        return;
    }

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
            }),
        });
        if (!resp.ok) {
            throw new Error(await resp.text() || 'Failed to create deck');
        }
        const created: CreateDeckResponse = await resp.json();
        await refreshDashboardData();
        showToast(`Deck saved (#${created.deck_id}).`, 'success');
        navigate('/decks');
    } catch (err: any) {
        showToast(err.message || 'Failed to save deck.', 'error');
    } finally {
        submitBtn.disabled = false;
        submitBtn.textContent = originalLabel;
    }
}

function renderReviewPage(): void {
    const filter = document.getElementById('review-deck-filter') as HTMLSelectElement | null;
    if (filter) {
        const current = state.reviewDeckFilter;
        filter.innerHTML = `<option value="">All decks</option>` + state.decks.map(deck =>
            `<option value="${deck.id}">${escapeHtml(deck.title)}</option>`
        ).join('');
        filter.value = current;
    }
    renderCurrentReviewCard();
}

function renderCurrentReviewCard(): void {
    const cardEl = document.getElementById('review-card');
    const emptyEl = document.getElementById('review-empty');
    const deckCountsEl = document.getElementById('review-card-decks');
    const exampleEl = document.getElementById('review-card-example');
    const frontTextEl = document.getElementById('review-card-front-text');
    const lemmaEl = document.getElementById('review-card-lemma');
    const meaningEl = document.getElementById('review-card-meaning');
    const modeEl = document.getElementById('review-card-mode');
    if (!cardEl || !emptyEl || !deckCountsEl || !exampleEl || !frontTextEl || !lemmaEl || !meaningEl || !modeEl) return;

    const card = state.currentReviewCard;
    const hasCard = Boolean(card);
    cardEl.classList.toggle('hidden', !hasCard);
    emptyEl.classList.toggle('hidden', hasCard);
    if (!card) return;

    modeEl.textContent = card.mode === 'sentence' ? 'Sentence card' : 'Word card';
    frontTextEl.textContent = card.front.text || card.back.lemma;
    lemmaEl.textContent = card.back.lemma;
    meaningEl.textContent = card.back.meaning || 'No gloss yet';
    deckCountsEl.innerHTML = card.deck_counts.map(pair =>
        `<span class="review-deck-pill">${escapeHtml(pair[0])} · ${escapeHtml(pair[1])}</span>`
    ).join('');

    const example = card.back.examples?.[0];
    if (example) {
        exampleEl.classList.remove('hidden');
        exampleEl.innerHTML = `<strong>${escapeHtml(example.source_deck || 'Example')}</strong><br>${escapeHtml(example.text)}`;
    } else {
        exampleEl.classList.add('hidden');
        exampleEl.innerHTML = '';
    }
}

async function loadNextReviewCard(showEmptyToast: boolean): Promise<void> {
    if (state.role === 'anon') return;

    const deckParam = state.reviewDeckFilter ? `?deck_id=${encodeURIComponent(state.reviewDeckFilter)}` : '';
    try {
        const resp = await fetch(`/api/review/next${deckParam}`, { credentials: 'same-origin' });
        if (resp.status === 204) {
            state.currentReviewCard = null;
            renderCurrentReviewCard();
            if (showEmptyToast) showToast('Nothing due right now.', 'info');
            return;
        }
        if (!resp.ok) {
            throw new Error(await resp.text() || 'Failed to load review card');
        }
        state.currentReviewCard = await resp.json() as ReviewCardResponse;
        renderCurrentReviewCard();
    } catch (err: any) {
        state.currentReviewCard = null;
        renderCurrentReviewCard();
        showToast(err.message || 'Failed to load review card.', 'error');
    }
}

async function submitReviewAnswer(rating: 'again' | 'hard' | 'good' | 'easy'): Promise<void> {
    const cardID = Number(state.currentReviewCard?.card_id || 0);
    if (!cardID) return;
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
    } catch (err: any) {
        showToast(err.message || 'Failed to record review answer.', 'error');
    }
}

async function mutateReviewCard(endpoint: '/api/card/known' | '/api/card/ignore', successMessage: string): Promise<void> {
    const cardID = Number(state.currentReviewCard?.card_id || 0);
    if (!cardID) return;
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
    } catch (err: any) {
        showToast(err.message || 'Failed to update card.', 'error');
    }
}

// ── Admin feedback queue ───────────────────────────────────────────────────

function feedbackStatusLabel(status: string): string {
    switch (status) {
        case 'submitted': return 'Submitted';
        case 'accepted': return 'Accepted';
        case 'rejected': return 'Rejected';
        case 'needs_follow_up': return 'Needs follow-up';
        default: return status || 'All';
    }
}

function formatFeedbackDate(value: string): string {
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return value;
    return date.toLocaleString(undefined, {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
    });
}

function renderAdminFeedbackPage(): void {
    const filter = document.getElementById('admin-feedback-status') as HTMLSelectElement | null;
    if (filter) filter.value = state.adminFeedbackStatus;
    renderAdminFeedbackList();
}

function renderAdminFeedbackList(): void {
    const list = document.getElementById('admin-feedback-list');
    const empty = document.getElementById('admin-feedback-empty');
    if (!list || !empty) return;

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

async function loadAdminFeedback(): Promise<void> {
    if (state.role !== 'admin') return;
    const statusParam = state.adminFeedbackStatus ? `?status=${encodeURIComponent(state.adminFeedbackStatus)}` : '';
    try {
        const resp = await fetch(`/api/admin/parse-feedback${statusParam}`, { credentials: 'same-origin' });
        if (!resp.ok) {
            throw new Error(await resp.text() || 'Failed to load parser feedback');
        }
        const data = await resp.json() as ParseFeedbackListResponse;
        state.adminFeedback = data.feedback || [];
        renderAdminFeedbackList();
    } catch (err: any) {
        state.adminFeedback = [];
        renderAdminFeedbackList();
        showToast(err.message || 'Failed to load parser feedback.', 'error');
    }
}

async function reviewAdminFeedback(feedbackID: number, status: 'accepted' | 'rejected' | 'needs_follow_up'): Promise<void> {
    const noteInput = document.querySelector<HTMLInputElement>(`[data-review-note="${feedbackID}"]`);
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
    } catch (err: any) {
        showToast(err.message || 'Failed to review parser feedback.', 'error');
    }
}

// ── Correction modal ───────────────────────────────────────────────────────

function openCorrectionModal(row: CorrectionRowContext): void {
    const modal           = document.getElementById('correction-modal');
    const lemmaEl         = document.getElementById('correction-lemma');
    const proposedLemmaEl = document.getElementById('correction-proposed-lemma')   as HTMLInputElement    | null;
    const proposedPosEl   = document.getElementById('correction-proposed-pos')     as HTMLSelectElement   | null;
    const proposedGramEl  = document.getElementById('correction-proposed-grammar') as HTMLInputElement    | null;
    const noteEl          = document.getElementById('correction-note')             as HTMLTextAreaElement | null;
    const submitBtn       = document.getElementById('correction-submit')           as HTMLButtonElement   | null;
    const authHint        = document.getElementById('correction-auth-hint');
    if (!modal || !lemmaEl || !proposedLemmaEl || !proposedPosEl || !proposedGramEl || !noteEl || !submitBtn) return;

    state.currentRow = row;

    const grammarSuffix = row.original_grammar_label ? ` · ${row.original_grammar_label}` : '';
    lemmaEl.textContent = `${row.lemma} (${posLabel(row.pos)})${grammarSuffix}`;
    proposedLemmaEl.value = row.lemma;
    proposedPosEl.value   = row.pos in POS_LABELS ? row.pos : 'X';
    proposedGramEl.value  = row.original_grammar_label;
    noteEl.value = '';

    // Backend requires a parse_id — only present for authenticated parses. If it's
    // missing, disable submit and surface why.
    const canSubmit = typeof state.currentResults?.parse_id === 'number';
    submitBtn.disabled = !canSubmit;
    if (authHint) authHint.classList.toggle('hidden', canSubmit);

    modal.classList.remove('hidden');
    proposedLemmaEl.focus();
}

function closeCorrectionModal(): void {
    document.getElementById('correction-modal')?.classList.add('hidden');
}

function initCorrectionModal(): void {
    document.getElementById('correction-modal-close')   ?.addEventListener('click', closeCorrectionModal);
    document.getElementById('correction-modal-backdrop')?.addEventListener('click', closeCorrectionModal);
    document.getElementById('correction-cancel')        ?.addEventListener('click', closeCorrectionModal);

    const form = document.getElementById('correction-form') as HTMLFormElement | null;
    form?.addEventListener('submit', async (e) => {
        e.preventDefault();
        const submitBtn = document.getElementById('correction-submit') as HTMLButtonElement;
        submitBtn.disabled = true;
        const orig = submitBtn.textContent || '';
        submitBtn.textContent = 'Sending…';
        try {
            const row     = state.currentRow;
            const results = state.currentResults;
            if (!row || !results || typeof results.parse_id !== 'number') {
                showToast("Can't send correction — no parse session attached.", 'error');
                return;
            }

            const proposedLemma = (document.getElementById('correction-proposed-lemma')   as HTMLInputElement).value.trim();
            const proposedPos   = (document.getElementById('correction-proposed-pos')     as HTMLSelectElement).value;
            const proposedGram  = (document.getElementById('correction-proposed-grammar') as HTMLInputElement).value.trim();
            const note          = (document.getElementById('correction-note')             as HTMLTextAreaElement).value.trim();
            if (!proposedLemma || !proposedPos) {
                showToast('Please fill in both base form and part of speech.', 'error');
                return;
            }

            const resp = await fetch('/api/parse/feedback', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                credentials: 'same-origin',
                body: JSON.stringify({
                    parse_id:               results.parse_id,
                    lang:                   results.lang,
                    parser:                 state.currentParserMode,
                    surface:                row.surface,
                    occurrence:             row.occurrence,
                    original_lemma:         row.lemma,
                    original_pos:           row.pos,
                    original_grammar_label: row.original_grammar_label,
                    proposed_lemma:         proposedLemma,
                    proposed_pos:           proposedPos,
                    proposed_grammar_label: proposedGram,
                    note,
                }),
            });
            if (resp.ok) {
                showToast('Thanks — correction sent.', 'success');
                closeCorrectionModal();
            } else {
                showToast("Couldn't send correction — please try again later.", 'error');
            }
        } catch {
            showToast("Couldn't send correction — check your connection and try again.", 'error');
        } finally {
            submitBtn.disabled = false;
            submitBtn.textContent = orig;
        }
    });
}

function initDecksPage(): void {
    document.getElementById('decks-list')?.addEventListener('click', async (e) => {
        const target = e.target as HTMLElement | null;
        if (!target) return;

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
            if (!deck) return;
            const title = window.prompt('Rename deck', deck.title)?.trim();
            if (!title || title === deck.title) return;
            try {
                const resp = await fetch(`/api/decks/${deckID}`, {
                    method: 'PATCH',
                    headers: { 'Content-Type': 'application/json' },
                    credentials: 'same-origin',
                    body: JSON.stringify({ title }),
                });
                if (!resp.ok) throw new Error(await resp.text() || 'Rename failed');
                await refreshDashboardData();
                showToast('Deck renamed.', 'success');
            } catch (err: any) {
                showToast(err.message || 'Rename failed.', 'error');
            }
            return;
        }

        const deleteDeckID = target.getAttribute('data-delete-deck');
        if (deleteDeckID) {
            const deckID = Number(deleteDeckID);
            const deck = getDeckByID(deckID);
            if (!deck) return;
            if (!window.confirm(`Delete "${deck.title}"? This removes the deck text but keeps global learning state.`)) return;
            try {
                const resp = await fetch(`/api/decks/${deckID}`, {
                    method: 'DELETE',
                    credentials: 'same-origin',
                });
                if (!resp.ok) throw new Error(await resp.text() || 'Delete failed');
                await refreshDashboardData();
                showToast('Deck deleted.', 'success');
            } catch (err: any) {
                showToast(err.message || 'Delete failed.', 'error');
            }
        }
    });
}

function initReviewPage(): void {
    const filter = document.getElementById('review-deck-filter') as HTMLSelectElement | null;
    filter?.addEventListener('change', async () => {
        state.reviewDeckFilter = filter.value;
        await loadNextReviewCard(false);
    });

    document.querySelectorAll<HTMLElement>('[data-review-answer]').forEach(btn => {
        btn.addEventListener('click', () => {
            const rating = btn.getAttribute('data-review-answer') as 'again' | 'hard' | 'good' | 'easy' | null;
            if (rating) void submitReviewAnswer(rating);
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

function initResultsSaveForm(): void {
    const toggleBtn = document.getElementById('results-save-toggle');
    const form = document.getElementById('results-save-form');
    const cancelBtn = document.getElementById('results-save-cancel');
    const htmlForm = form as HTMLFormElement | null;
    toggleBtn?.addEventListener('click', () => {
        form?.classList.toggle('hidden');
        const input = document.getElementById('results-deck-title') as HTMLInputElement | null;
        if (form && !form.classList.contains('hidden')) input?.focus();
    });
    cancelBtn?.addEventListener('click', () => {
        form?.classList.add('hidden');
    });
    htmlForm?.addEventListener('submit', async (e) => {
        e.preventDefault();
        await saveCurrentResultsAsDeck();
    });
}

function initAdminFeedbackPage(): void {
    const filter = document.getElementById('admin-feedback-status') as HTMLSelectElement | null;
    filter?.addEventListener('change', async () => {
        state.adminFeedbackStatus = filter.value;
        await loadAdminFeedback();
    });

    document.getElementById('admin-feedback-list')?.addEventListener('click', (e) => {
        const target = e.target as HTMLElement | null;
        if (!target) return;
        const action = target.getAttribute('data-feedback-action') as 'accepted' | 'rejected' | 'needs_follow_up' | null;
        const id = Number(target.getAttribute('data-feedback-id') || 0);
        if (action && id > 0) void reviewAdminFeedback(id, action);
    });
}

// ── Init ───────────────────────────────────────────────────────────────────

document.addEventListener('DOMContentLoaded', async () => {
    initTheme();
    initMobileNav();
    initSigninForm();
    initInspectForm();
    initWorkbenchForm();
    initCorrectionModal();
    initDecksPage();
    initReviewPage();
    initResultsSaveForm();
    initAdminFeedbackPage();

    document.getElementById('theme-toggle')?.addEventListener('click', toggleTheme);

    document.getElementById('nav-signout')       ?.addEventListener('click', handleSignout);
    document.getElementById('nav-mobile-signout')?.addEventListener('click', handleSignout);

    document.getElementById('results-back')?.addEventListener('click', () => {
        navigate(state.currentContext === 'workbench' ? '/admin/workbench' : '/inspect');
    });

    document.querySelectorAll<HTMLButtonElement>('.sort-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            if (!state.currentResults) return;
            const key = btn.dataset.sort as SortKey | undefined;
            if (!key) return;
            state.currentSort = state.currentSort.key === key
                ? { key, dir: state.currentSort.dir === 'asc' ? 'desc' : 'asc' }
                : { key, dir: key === 'tokens' ? 'desc' : 'asc' };
            renderResultsTable(state.currentResults);
        });
    });

    document.querySelectorAll<HTMLButtonElement>('.pos-filter-chip').forEach(btn => {
        btn.addEventListener('click', () => {
            if (!state.currentResults) return;
            const filter = btn.dataset.filter as POSFilter | undefined;
            if (!filter) return;
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
