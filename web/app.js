"use strict";
// FinEstDB — frontend with three role-aware surfaces:
//   anonymous landing/about/signin, authenticated user product, admin workbench.
const MAX_CHARS = 300000;
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
// ── App-wide state ─────────────────────────────────────────────────────────
const state = {
    user: null,
    dashboard: null,
    role: 'anon',
    currentResults: null,
    currentContext: 'inspect',
    currentParserMode: 'basic',
    currentTextPreview: '',
    currentRow: null,
    currentSort: { key: 'row', dir: 'asc' },
    currentPOSFilter: 'all',
};
const NOUN_POS = ['NOUN', 'PROPN'];
const VERB_POS = ['VERB', 'AUX'];
const ADJ_POS = ['ADJ'];
const ADV_POS = ['ADV'];
const OTHER_POS = ['PRON', 'DET', 'ADP', 'NUM', 'CCONJ', 'SCONJ', 'PART', 'INTJ', 'X', 'SYM', 'PUNCT'];
// ── Theme ──────────────────────────────────────────────────────────────────
function initTheme() {
    const saved = localStorage.getItem('theme') || 'light';
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
// ── Helpers ────────────────────────────────────────────────────────────────
function escapeHtml(str) {
    const d = document.createElement('div');
    d.textContent = str;
    return d.innerHTML;
}
function posLabel(pos) {
    return POS_LABELS[pos] || pos;
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
    state.role = computeRole(state.user);
    const emailEl = document.getElementById('nav-user-email');
    if (emailEl)
        emailEl.textContent = state.user?.email || '';
    applyRoleVisibility();
    renderDashboard();
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
    state.role = 'anon';
    state.currentResults = null;
    state.currentTextPreview = '';
    state.currentParserMode = 'basic';
    state.currentContext = 'inspect';
    state.currentRow = null;
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
    '/review': 'review-page',
    '/admin/workbench': 'admin-workbench-page',
    '/admin/feedback': 'admin-feedback-page',
    '/results': 'results-page',
};
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
    const userOnly = ['/dashboard', '/inspect', '/decks', '/review', '/results'];
    if (userOnly.includes(route)) {
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
    if (!ROUTE_TO_PAGE[route])
        route = '/';
    const guard = isRouteAllowed(route);
    if (!guard.allowed && guard.redirect) {
        window.location.hash = `#${guard.redirect}`;
        return;
    }
    showPage(ROUTE_TO_PAGE[route]);
    setActiveNavLink(route);
    closeMobileNav();
    // Per-page hooks
    if (route === '/dashboard')
        renderDashboard();
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
// ── Sign-in flow ───────────────────────────────────────────────────────────
function initSigninForm() {
    const form = document.getElementById('signin-form');
    if (!form)
        return;
    form.addEventListener('submit', async (e) => {
        e.preventDefault();
        const emailEl = document.getElementById('signin-email');
        const passwordEl = document.getElementById('signin-password');
        const errorEl = document.getElementById('signin-error');
        const submitBtn = document.getElementById('signin-submit');
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
            const data = await resp.json();
            if (!data.authenticated || !data.user) {
                throw new Error('Sign-in failed');
            }
            // Refresh /api/me so we get the dashboard payload too.
            await fetchMe();
            showToast(`Welcome, ${data.user.email}`, 'success');
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
// ── Language detection (shared between inspect + workbench) ────────────────
function detectLang(text) {
    const lower = text.toLowerCase();
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
function getInspectEls() {
    const lang = document.getElementById('inspect-lang');
    const text = document.getElementById('inspect-text');
    const file = document.getElementById('inspect-file');
    const cc = document.getElementById('inspect-char-count');
    const warn = document.getElementById('inspect-lang-warning');
    const swBtn = document.getElementById('inspect-lang-switch');
    if (!lang || !text || !file || !cc || !warn || !swBtn)
        return null;
    return { lang, text, file, charCount: cc, warning: warn, switchBtn: swBtn };
}
function getWorkbenchEls() {
    const lang = document.getElementById('parse-lang');
    const text = document.getElementById('parse-text');
    const file = document.getElementById('parse-file');
    const cc = document.getElementById('char-count');
    const warn = document.getElementById('lang-warning');
    const swBtn = document.getElementById('lang-switch-btn');
    if (!lang || !text || !file || !cc || !warn || !swBtn)
        return null;
    return { lang, text, file, charCount: cc, warning: warn, switchBtn: swBtn };
}
function updateCharCount(els) {
    const count = els.text.value.length;
    els.charCount.textContent = `${count.toLocaleString()} / ${MAX_CHARS.toLocaleString()}`;
    els.charCount.classList.toggle('char-count-warn', count > MAX_CHARS * 0.9);
    els.charCount.classList.toggle('char-count-over', count >= MAX_CHARS);
}
function updateLangWarning(els, gateInspectButton) {
    if (els.text.value.trim().length < 20) {
        els.warning.classList.add('hidden');
        els.switchBtn.classList.add('hidden');
        if (gateInspectButton)
            gateSubmit('inspect-submit', false);
        else
            setParseButtonsDisabled(false);
        return;
    }
    const ws = getLangWarningState(els.text.value, els.lang.value);
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
            const text = els.text.value;
            if (text.trim().length < 20)
                return;
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
        const input = e.target;
        const file = input.files?.[0];
        if (!file)
            return;
        const text = await file.text();
        els.text.value = text.slice(0, MAX_CHARS);
        updateCharCount(els);
        updateLangWarning(els, true);
    });
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
            const text = els.text.value;
            if (text.trim().length < 20)
                return;
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
        const input = e.target;
        const file = input.files?.[0];
        if (!file)
            return;
        const text = await file.text();
        els.text.value = text.slice(0, MAX_CHARS);
        updateCharCount(els);
        updateLangWarning(els, false);
    });
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
    const text = els.text.value.trim();
    const lang = els.lang.value;
    const ws = getLangWarningState(text, lang);
    if (!text)
        return;
    if (text.length > MAX_CHARS) {
        alert(`Text must be ${MAX_CHARS.toLocaleString()} characters or fewer.`);
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
        const data = await resp.json();
        showResults(data, text.slice(0, 60), parserMode, context);
    }
    catch (err) {
        alert(`Parse failed: ${err.message}`);
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
    const rowCoverage = data.words.length === 0 ? 0 : definedRows / data.words.length;
    const tokenCoverage = data.total_tokens === 0 ? 0 : definedTokens / data.total_tokens;
    const score = Math.round(((tokenCoverage * 0.7) + (rowCoverage * 0.3)) * 100);
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
        if (cmp === 0)
            cmp = a.originalIndex - b.originalIndex;
        return cmp * direction;
    });
    return sorted;
}
function highlightFormsInSentence(sentence, forms) {
    let result = escapeHtml(sentence);
    for (const form of forms) {
        const escaped = escapeHtml(form);
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
        const actionCell = showActions
            ? `<td class="col-actions"><button type="button" class="btn btn-link btn-sm correction-btn"
                data-lemma="${escapeHtml(w.lemma)}"
                data-pos="${escapeHtml(w.pos)}"
                data-surface="${escapeHtml(surfaceForm)}"
                data-grammar="${escapeHtml(w.grammar_label || '')}">Suggest fix</button></td>`
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
    document.getElementById('results-title').textContent =
        `"${preview}${ellipsis}" (${langName})`;
    // For users, hide internal parser-mode pill; show a softer label.
    const parserPill = document.getElementById('results-parser');
    if (context === 'inspect' && state.role !== 'admin') {
        parserPill.textContent = 'Your text';
    }
    else {
        parserPill.textContent = formatParserMode(parserMode);
    }
    document.getElementById('results-duration').textContent =
        `Parse time ${formatParseDuration(data.parse_duration_ms)}`;
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
    state.currentSort = { key: 'row', dir: 'asc' };
    state.currentPOSFilter = 'all';
    renderResultsTable(data);
    // Re-apply role visibility so admin-only pills/cells show correctly.
    applyRoleVisibility();
    navigate('/results');
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
    // Backend requires a parse_id — only present for authenticated parses. If it's
    // missing, disable submit and surface why.
    const canSubmit = typeof state.currentResults?.parse_id === 'number';
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
            if (!row || !results || typeof results.parse_id !== 'number') {
                showToast("Can't send correction — no parse session attached.", 'error');
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
            const resp = await fetch('/api/parse/feedback', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                credentials: 'same-origin',
                body: JSON.stringify({
                    parse_id: results.parse_id,
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
                }),
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
// ── Init ───────────────────────────────────────────────────────────────────
document.addEventListener('DOMContentLoaded', async () => {
    initTheme();
    initMobileNav();
    initSigninForm();
    initInspectForm();
    initWorkbenchForm();
    initCorrectionModal();
    document.getElementById('theme-toggle')?.addEventListener('click', toggleTheme);
    document.getElementById('nav-signout')?.addEventListener('click', handleSignout);
    document.getElementById('nav-mobile-signout')?.addEventListener('click', handleSignout);
    document.getElementById('results-back')?.addEventListener('click', () => {
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
