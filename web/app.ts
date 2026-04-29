// FinEstDB — parse word list view

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
}

interface ParseResponse {
    lang:         string;
    total_tokens: number;
    parse_duration_ms: number;
    words:        WordEntry[];
}

type SortKey = 'row' | 'lemma' | 'pos' | 'forms' | 'definition' | 'tokens';
type POSFilter = 'all' | 'NOUN' | 'VERB' | 'ADJ' | 'ADV' | 'other';

interface SortState {
    key: SortKey;
    dir: 'asc' | 'desc';
}

interface DisplayWordEntry extends WordEntry {
    originalIndex: number;
}

let currentResults: ParseResponse | null = null;
let currentSort: SortState = { key: 'row', dir: 'asc' };
let currentPOSFilter: POSFilter = 'all';

// POS categories for filtering
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

// ── Page navigation ────────────────────────────────────────────────────────

function showPage(id: string): void {
    document.querySelectorAll('.page').forEach(p => p.classList.remove('active'));
    document.getElementById(id)!.classList.add('active');
}

// ── Toast notifications ────────────────────────────────────────────────────

function showToast(message: string, type: 'info' | 'success' = 'info', duration = 3000): void {
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

// ── Language detection ─────────────────────────────────────────────────────
//
//  Finnish:  ä, ö are common vowels; y is a vowel; no õ
//  Estonian: õ is exclusive to Estonian (never appears in Finnish)
//  Both:     Use nordic characters (ä, ö); Estonian also has ü
//
//  Rules:
//    - Text contains õ  → Estonian
//    - ä/ö ratio > 1.5% of total letters → Finnish
//    - Otherwise        → unrecognised (warn user)

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
    return {
        detected,
        message: null,
        canSwitch: false,
        blocksParse: false,
    };
}

// ── Helpers ────────────────────────────────────────────────────────────────

function escapeHtml(str: string): string {
    const d = document.createElement('div');
    d.textContent = str;
    return d.innerHTML;
}

function posLabel(pos: string): string {
    return POS_LABELS[pos] || pos;
}

function setParseButtonsDisabled(disabled: boolean): void {
    document.querySelectorAll<HTMLButtonElement>('.btn-parse').forEach(btn => {
        btn.disabled = disabled;
    });
}

function compareStrings(a: string, b: string): number {
    return a.localeCompare(b, undefined, { sensitivity: 'base' });
}

function formatParserMode(parserMode: 'basic' | 'custom'): string {
    return parserMode === 'custom' ? 'Custom parser' : 'Basic parser';
}

function formatParseDuration(parseDurationMs: number): string {
    if (parseDurationMs < 1000) {
        return `${parseDurationMs} ms`;
    }

    return `${(parseDurationMs / 1000).toFixed(parseDurationMs >= 10_000 ? 1 : 2)} s`;
}

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
    // Weighted proxy score for comparing parsers: tokens are weighted higher (0.7)
    // because they reflect actual text coverage (a high-frequency word with a definition
    // contributes more than a rare one). Row coverage (0.3) captures breadth across
    // unique lemmas. Together they give a single number to compare basic vs custom parser.
    const score = Math.round(((tokenCoverage * 0.7) + (rowCoverage * 0.3)) * 100);

    return {
        score,
        definedRows,
        definedTokens,
        rowCoverage,
        tokenCoverage,
    };
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
    if (filter === 'all') return words;
    return words.filter(w => matchesPOSFilter(w.pos, filter));
}

function sortWords(words: DisplayWordEntry[], sort: SortState): DisplayWordEntry[] {
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
                if (cmp === 0) {
                    cmp = compareStrings(a.gloss || '', b.gloss || '');
                }
                break;
            }
            case 'tokens':
                cmp = a.count - b.count;
                break;
        }

        if (cmp === 0) {
            cmp = a.originalIndex - b.originalIndex;
        }

        return cmp * direction;
    });

    return sorted;
}

function highlightFormsInSentence(sentence: string, forms: string[]): string {
    let result = escapeHtml(sentence);
    for (const form of forms) {
        const escaped = escapeHtml(form);
        // Case-insensitive replacement, preserving original case
        const regex = new RegExp(`\\b(${escaped})\\b`, 'gi');
        result = result.replace(regex, '<span class="highlight-form">$1</span>');
    }
    return result;
}

function updateSortButtons(): void {
    document.querySelectorAll<HTMLButtonElement>('.sort-btn').forEach(btn => {
        const key = btn.dataset.sort as SortKey | undefined;
        if (!key) return;

        const active = currentSort.key === key;
        btn.classList.toggle('active', active);
        btn.setAttribute('aria-sort', active ? currentSort.dir : 'none');

        // Write the indicator into the dedicated <span> so the label text is never touched.
        const arrow = btn.querySelector<HTMLElement>('.sort-arrow');
        if (arrow) {
            arrow.textContent = active ? (currentSort.dir === 'asc' ? ' ↑' : ' ↓') : '';
        }
    });
}

function updatePOSFilterButtons(): void {
    document.querySelectorAll<HTMLButtonElement>('.pos-filter-chip').forEach(btn => {
        const filter = btn.dataset.filter as POSFilter | undefined;
        const active = filter === currentPOSFilter;
        btn.classList.toggle('active', active);
        btn.setAttribute('aria-pressed', active ? 'true' : 'false');
    });
}

function renderResultsTable(data: ParseResponse): void {
    const tbody = document.getElementById('word-table-body')!;
    const help = document.getElementById('results-help')!;
    const baseWords = data.words.map((word, index) => ({ ...word, originalIndex: index }));
    const filteredWords = filterWords(baseWords, currentPOSFilter);
    const sortedWords = sortWords(filteredWords, currentSort);
    const hasGrammar = data.words.some(word => Boolean(word.grammar_label));

    help.textContent = hasGrammar
        ? `Coverage = dictionary-backed tokens. Grammar labels shown as badges when case/morphology was inferred.`
        : `Coverage = dictionary-backed tokens. Grammar labels appear when case/morphology inference is available.`;

    tbody.innerHTML = sortedWords.map((w, index) => {
        const forms = w.forms.slice(0, 3).map(escapeHtml).join(', ')
            + (w.forms.length > 3 ? ` +${w.forms.length - 3}` : '');

        // Grammar badge (inline with lemma)
        const grammarBadge = w.grammar_label
            ? `<span class="grammar-badge">${escapeHtml(w.grammar_label)}</span>`
            : '';

        // Example sentence with highlighted forms
        const exampleHtml = w.example_sentence
            ? `<details class="example-details">
                <summary class="example-toggle">▸ example</summary>
                <span class="example-text">${highlightFormsInSentence(w.example_sentence, w.forms)}</span>
               </details>`
            : '';

        const glossHtml = w.gloss ? escapeHtml(w.gloss) : '<span class="no-gloss">Missing</span>';

        // POS pill with data attribute for styling
        const posPill = `<span class="pos-pill" data-pos="${escapeHtml(w.pos)}">${escapeHtml(posLabel(w.pos))}</span>`;

        return `<tr>
            <td class="col-row">${index + 1}</td>
            <td class="col-lemma">${escapeHtml(w.lemma)}${grammarBadge}${exampleHtml}</td>
            <td class="col-pos">${posPill}</td>
            <td class="col-forms">${forms}</td>
            <td class="col-def">${glossHtml}</td>
            <td class="col-count">${w.count}</td>
        </tr>`;
    }).join('');

    updateSortButtons();
    updatePOSFilterButtons();
}

// ── Parse form ─────────────────────────────────────────────────────────────

function updateCharCount(): void {
    const text  = (document.getElementById('parse-text') as HTMLTextAreaElement).value;
    const count = text.length;
    const el    = document.getElementById('char-count')!;
    el.textContent = `${count.toLocaleString()} / ${MAX_CHARS.toLocaleString()}`;
    el.classList.toggle('char-count-warn', count > MAX_CHARS * 0.9);
    el.classList.toggle('char-count-over', count >= MAX_CHARS);
}

function updateLangWarning(): void {
    const text      = (document.getElementById('parse-text') as HTMLTextAreaElement).value;
    const lang      = (document.getElementById('parse-lang') as HTMLSelectElement).value;
    const warningEl = document.getElementById('lang-warning')!;
    const switchBtn = document.getElementById('lang-switch-btn') as HTMLButtonElement;

    if (text.trim().length < 20) {
        warningEl.classList.add('hidden');
        switchBtn.classList.add('hidden');
        setParseButtonsDisabled(false);
        return;
    }

    const warningState = getLangWarningState(text, lang);
    if (warningState.message) {
        warningEl.textContent = warningState.message;
        warningEl.classList.remove('hidden');
        if (warningState.canSwitch) {
            switchBtn.textContent = `Switch to ${warningState.detected === 'FI' ? 'Finnish' : 'Estonian'}`;
            switchBtn.classList.remove('hidden');
        } else {
            switchBtn.classList.add('hidden');
        }
    } else {
        warningEl.classList.add('hidden');
        switchBtn.classList.add('hidden');
    }

    setParseButtonsDisabled(warningState.blocksParse);
}

async function handleParseSubmit(e: Event, parserMode: 'basic' | 'custom'): Promise<void> {
    e.preventDefault();

    const text = (document.getElementById('parse-text') as HTMLTextAreaElement).value.trim();
    const lang = (document.getElementById('parse-lang') as HTMLSelectElement).value;
    const warningState = getLangWarningState(text, lang);
    const btnBasic = document.getElementById('parse-btn-basic') as HTMLButtonElement;
    const btnCustom = document.getElementById('parse-btn-custom') as HTMLButtonElement;

    if (!text) return;
    if (text.length > MAX_CHARS) {
        alert(`Text must be ${MAX_CHARS.toLocaleString()} characters or fewer.`);
        return;
    }
    if (warningState.blocksParse) return;

    btnBasic.disabled = true;
    btnCustom.disabled = true;
    const activeBtn = parserMode === 'custom' ? btnCustom : btnBasic;
    const origLabel = activeBtn.textContent || '';
    activeBtn.textContent = 'Parsing…';

    try {
        const resp = await fetch('/api/parse', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ lang, text, parser: parserMode }),
        });

        if (!resp.ok) {
            const msg = await resp.text();
            throw new Error(msg || resp.statusText);
        }

        const data: ParseResponse = await resp.json();
        showResults(data, text.slice(0, 60), parserMode);
    } catch (err: any) {
        alert(`Parse failed: ${err.message}`);
    } finally {
        updateLangWarning();
        activeBtn.textContent = origLabel;
    }
}

// ── Results rendering ──────────────────────────────────────────────────────

function showResults(data: ParseResponse, textPreview: string, parserMode: 'basic' | 'custom'): void {
    const langName = data.lang === 'FI' ? 'Finnish' : 'Estonian';
    const preview  = textPreview.replace(/\s+/g, ' ').trim();
    const ellipsis = preview.length >= 60 ? '…' : '';
    const uniqueLemmas = data.words.length;
    const coverage = computeCoverageScore(data);

    document.getElementById('results-title')!.textContent =
        `"${preview}${ellipsis}" (${langName})`;
    document.getElementById('results-parser')!.textContent = formatParserMode(parserMode);
    document.getElementById('results-duration')!.textContent =
        `Parse time ${formatParseDuration(data.parse_duration_ms)}`;

    // Update coverage gauge
    const coverageFill = document.getElementById('coverage-fill')!;
    const coverageValue = document.getElementById('coverage-value')!;
    coverageFill.style.width = `${coverage.score}%`;
    coverageFill.classList.remove('low', 'medium', 'high');
    coverageFill.classList.add(
        coverage.score >= 80 ? 'high' : coverage.score >= 50 ? 'medium' : 'low'
    );
    coverageValue.textContent = `${coverage.score}%`;

    document.getElementById('results-stats')!.textContent =
        `${uniqueLemmas} unique lemmas · ${data.total_tokens} tokens · `
        + `${coverage.definedRows}/${uniqueLemmas} with definitions`;

    currentResults = data;
    currentSort = { key: 'row', dir: 'asc' };
    currentPOSFilter = 'all';
    renderResultsTable(data);

    showPage('results-page');
}

// ── Mobile nav ──────────────────────────────────────────────────────────────

function initMobileNav(): void {
    const hamburger = document.getElementById('nav-hamburger');
    const overlay = document.getElementById('nav-mobile-overlay');

    if (!hamburger || !overlay) return;

    hamburger.addEventListener('click', () => {
        const isOpen = !overlay.classList.contains('hidden');
        overlay.classList.toggle('hidden', isOpen);
        hamburger.classList.toggle('open', !isOpen);
        document.body.classList.toggle('nav-open', !isOpen);
    });

    // Close on overlay background click
    overlay.addEventListener('click', (e) => {
        if (e.target === overlay) {
            overlay.classList.add('hidden');
            hamburger.classList.remove('open');
            document.body.classList.remove('nav-open');
        }
    });

    // Close on mobile link click
    overlay.querySelectorAll('.nav-mobile-link').forEach(link => {
        link.addEventListener('click', () => {
            overlay.classList.add('hidden');
            hamburger.classList.remove('open');
            document.body.classList.remove('nav-open');
        });
    });
}

// ── Init ───────────────────────────────────────────────────────────────────

document.addEventListener('DOMContentLoaded', () => {
    initTheme();
    initMobileNav();

    // Theme toggles
    document.getElementById('theme-toggle')
        ?.addEventListener('click', toggleTheme);
    document.getElementById('results-theme-toggle')
        ?.addEventListener('click', toggleTheme);

    // Nav links (logo + Import both go to parse page)
    document.getElementById('nav-logo')
        ?.addEventListener('click', (e) => {
            e.preventDefault();
            showPage('parse-page');
        });
    document.getElementById('nav-import')
        ?.addEventListener('click', (e) => {
            e.preventDefault();
            showPage('parse-page');
        });
    document.getElementById('nav-mobile-import')
        ?.addEventListener('click', (e) => {
            e.preventDefault();
            showPage('parse-page');
        });

    // Back button
    document.getElementById('results-back')
        ?.addEventListener('click', () => showPage('parse-page'));

    // Char counter + lang warning on textarea input
    const textarea = document.getElementById('parse-text') as HTMLTextAreaElement;
    textarea?.addEventListener('input', () => {
        updateCharCount();
        updateLangWarning();
    });

    // Auto-detect language on paste with high confidence
    textarea?.addEventListener('paste', () => {
        // Use setTimeout to get the pasted content after it's inserted
        setTimeout(() => {
            const text = textarea.value;
            if (text.trim().length < 20) return;

            const langSelect = document.getElementById('parse-lang') as HTMLSelectElement;
            const detected = detectLang(text);

            if (detected !== 'unknown' && detected !== langSelect.value) {
                // High confidence detection — auto-switch
                langSelect.value = detected;
                const langName = detected === 'FI' ? 'Finnish' : 'Estonian';
                showToast(`Switched to ${langName} — detected from text`, 'info');
                updateLangWarning();
            }
        }, 0);
    });

    // Also re-check warning when language selector changes
    document.getElementById('parse-lang')
        ?.addEventListener('change', updateLangWarning);

    document.getElementById('lang-switch-btn')
        ?.addEventListener('click', () => {
            const text = (document.getElementById('parse-text') as HTMLTextAreaElement).value;
            const langSelect = document.getElementById('parse-lang') as HTMLSelectElement;
            const warningState = getLangWarningState(text, langSelect.value);
            if (warningState.canSwitch) {
                langSelect.value = warningState.detected;
                updateLangWarning();
            }
        });

    // Load file into textarea
    document.getElementById('parse-file')
        ?.addEventListener('change', async (e) => {
            const input = e.target as HTMLInputElement;
            const file  = input.files?.[0];
            if (!file) return;
            const text = await file.text();
            textarea.value = text.slice(0, MAX_CHARS);
            updateCharCount();
            updateLangWarning();
        });

    // Parse form submit
    document.getElementById('parse-btn-basic')
        ?.addEventListener('click', (e) => handleParseSubmit(e, 'basic'));
    document.getElementById('parse-btn-custom')
        ?.addEventListener('click', (e) => handleParseSubmit(e, 'custom'));

    document.getElementById('parse-form')
        ?.addEventListener('submit', (e) => handleParseSubmit(e, 'basic'));

    document.querySelectorAll<HTMLButtonElement>('.sort-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            if (!currentResults) return;

            const key = btn.dataset.sort as SortKey | undefined;
            if (!key) return;

            currentSort = currentSort.key === key
                ? { key, dir: currentSort.dir === 'asc' ? 'desc' : 'asc' }
                : { key, dir: key === 'tokens' ? 'desc' : 'asc' };

            renderResultsTable(currentResults);
        });
    });

    // POS filter chips
    document.querySelectorAll<HTMLButtonElement>('.pos-filter-chip').forEach(btn => {
        btn.addEventListener('click', () => {
            if (!currentResults) return;

            const filter = btn.dataset.filter as POSFilter | undefined;
            if (!filter) return;

            currentPOSFilter = filter;
            renderResultsTable(currentResults);
        });
    });

    updateSortButtons();
    updatePOSFilterButtons();
});
