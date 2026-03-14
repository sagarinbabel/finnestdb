"use strict";
// FinEstDB — parse word list view
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
let currentResults = null;
let currentSort = { key: 'row', dir: 'asc' };
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
// ── Page navigation ────────────────────────────────────────────────────────
function showPage(id) {
    document.querySelectorAll('.page').forEach(p => p.classList.remove('active'));
    document.getElementById(id).classList.add('active');
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
    return {
        detected,
        message: null,
        canSwitch: false,
        blocksParse: false,
    };
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
function setParseButtonsDisabled(disabled) {
    document.querySelectorAll('.btn-parse').forEach(btn => {
        btn.disabled = disabled;
    });
}
function compareStrings(a, b) {
    return a.localeCompare(b, undefined, { sensitivity: 'base' });
}
function formatParserMode(parserMode) {
    return parserMode === 'custom' ? 'Custom parser' : 'Basic parser';
}
function formatParseDuration(parseDurationMs) {
    if (parseDurationMs < 1000) {
        return `${parseDurationMs} ms`;
    }
    return `${(parseDurationMs / 1000).toFixed(parseDurationMs >= 10000 ? 1 : 2)} s`;
}
function computeCoverageScore(data) {
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
                if (cmp === 0) {
                    cmp = compareStrings(a.gloss || '', b.gloss || '');
                }
                break;
            }
            case 'grammar': {
                const aMissing = a.grammar_label ? 0 : 1;
                const bMissing = b.grammar_label ? 0 : 1;
                cmp = aMissing - bMissing;
                if (cmp === 0) {
                    cmp = compareStrings(a.grammar_label || '', b.grammar_label || '');
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
function updateSortButtons() {
    document.querySelectorAll('.sort-btn').forEach(btn => {
        const key = btn.dataset.sort;
        if (!key)
            return;
        const active = currentSort.key === key;
        btn.classList.toggle('active', active);
        btn.setAttribute('aria-sort', active ? currentSort.dir : 'none');
        // Write the indicator into the dedicated <span> so the label text is never touched.
        const arrow = btn.querySelector('.sort-arrow');
        if (arrow) {
            arrow.textContent = active ? (currentSort.dir === 'asc' ? ' ↑' : ' ↓') : '';
        }
    });
}
function renderResultsTable(data) {
    const tbody = document.getElementById('word-table-body');
    const help = document.getElementById('results-help');
    const grammarCells = document.querySelectorAll('.col-grammar');
    const baseWords = data.words.map((word, index) => ({ ...word, originalIndex: index }));
    const sortedWords = sortWords(baseWords, currentSort);
    const hasGrammar = data.words.some(word => Boolean(word.grammar_label));
    grammarCells.forEach(cell => cell.classList.toggle('is-hidden', !hasGrammar));
    help.textContent = hasGrammar
        ? `Coverage score = how much of this text produced usable dictionary-backed output. Rows are unique lemma entries. Tokens are per-row occurrences in the parsed text. Grammar appears only when the custom enrichment inferred a case or related label.`
        : `Coverage score = how much of this text produced usable dictionary-backed output. Rows are unique lemma entries. Tokens are per-row occurrences in the parsed text. Grammar is hidden because this parse did not produce any grammar labels.`;
    tbody.innerHTML = sortedWords.map((w, index) => {
        const forms = w.forms.slice(0, 3).map(escapeHtml).join(', ')
            + (w.forms.length > 3 ? ` +${w.forms.length - 3}` : '');
        const exampleHtml = w.example_sentence
            ? `<details class="example-details">
                <summary class="example-toggle">▸ example</summary>
                <span class="example-text">${escapeHtml(w.example_sentence)}</span>
               </details>`
            : '';
        const glossHtml = w.gloss ? escapeHtml(w.gloss) : '<span class="no-gloss">Missing</span>';
        const grammarHtml = w.grammar_label
            ? `<span class="grammar-label">${escapeHtml(w.grammar_label)}</span>`
            : '<span class="no-grammar">—</span>';
        return `<tr>
            <td class="col-row">${index + 1}</td>
            <td class="col-lemma">${escapeHtml(w.lemma)}${exampleHtml}</td>
            <td class="col-pos">${escapeHtml(posLabel(w.pos))}</td>
            <td class="col-forms">${forms}</td>
            <td class="col-def">${glossHtml}</td>
            <td class="col-grammar">${grammarHtml}</td>
            <td class="col-count">${w.count}</td>
        </tr>`;
    }).join('');
    updateSortButtons();
}
// ── Parse form ─────────────────────────────────────────────────────────────
function updateCharCount() {
    const text = document.getElementById('parse-text').value;
    const count = text.length;
    const el = document.getElementById('char-count');
    el.textContent = `${count.toLocaleString()} / ${MAX_CHARS.toLocaleString()}`;
    el.classList.toggle('char-count-warn', count > MAX_CHARS * 0.9);
    el.classList.toggle('char-count-over', count >= MAX_CHARS);
}
function updateLangWarning() {
    const text = document.getElementById('parse-text').value;
    const lang = document.getElementById('parse-lang').value;
    const warningEl = document.getElementById('lang-warning');
    const switchBtn = document.getElementById('lang-switch-btn');
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
        }
        else {
            switchBtn.classList.add('hidden');
        }
    }
    else {
        warningEl.classList.add('hidden');
        switchBtn.classList.add('hidden');
    }
    setParseButtonsDisabled(warningState.blocksParse);
}
async function handleParseSubmit(e, parserMode) {
    e.preventDefault();
    const text = document.getElementById('parse-text').value.trim();
    const lang = document.getElementById('parse-lang').value;
    const warningState = getLangWarningState(text, lang);
    const btnBasic = document.getElementById('parse-btn-basic');
    const btnCustom = document.getElementById('parse-btn-custom');
    if (!text)
        return;
    if (text.length > MAX_CHARS) {
        alert(`Text must be ${MAX_CHARS.toLocaleString()} characters or fewer.`);
        return;
    }
    if (warningState.blocksParse)
        return;
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
        const data = await resp.json();
        showResults(data, text.slice(0, 60), parserMode);
    }
    catch (err) {
        alert(`Parse failed: ${err.message}`);
    }
    finally {
        updateLangWarning();
        activeBtn.textContent = origLabel;
    }
}
// ── Results rendering ──────────────────────────────────────────────────────
function showResults(data, textPreview, parserMode) {
    const langName = data.lang === 'FI' ? 'Finnish' : 'Estonian';
    const preview = textPreview.replace(/\s+/g, ' ').trim();
    const ellipsis = preview.length >= 60 ? '…' : '';
    const uniqueLemmas = data.words.length;
    const coverage = computeCoverageScore(data);
    document.getElementById('results-title').textContent =
        `"${preview}${ellipsis}" (${langName})`;
    document.getElementById('results-parser').textContent = formatParserMode(parserMode);
    document.getElementById('results-score').textContent =
        `Coverage score ${coverage.score}%`;
    document.getElementById('results-duration').textContent =
        `Parse time ${formatParseDuration(data.parse_duration_ms)}`;
    document.getElementById('results-stats').textContent =
        `${uniqueLemmas} unique lemmas (rows) · ${data.total_tokens} total tokens · `
            + `${coverage.definedRows}/${uniqueLemmas} rows with definitions · `
            + `${coverage.definedTokens}/${data.total_tokens} tokens covered by definitions`;
    currentResults = data;
    currentSort = { key: 'row', dir: 'asc' };
    renderResultsTable(data);
    showPage('results-page');
}
// ── Init ───────────────────────────────────────────────────────────────────
document.addEventListener('DOMContentLoaded', () => {
    initTheme();
    // Theme toggles
    document.getElementById('theme-toggle')
        ?.addEventListener('click', toggleTheme);
    document.getElementById('results-theme-toggle')
        ?.addEventListener('click', toggleTheme);
    // Back button
    document.getElementById('results-back')
        ?.addEventListener('click', () => showPage('parse-page'));
    // Char counter + lang warning on textarea input
    const textarea = document.getElementById('parse-text');
    textarea?.addEventListener('input', () => {
        updateCharCount();
        updateLangWarning();
    });
    // Also re-check warning when language selector changes
    document.getElementById('parse-lang')
        ?.addEventListener('change', updateLangWarning);
    document.getElementById('lang-switch-btn')
        ?.addEventListener('click', () => {
        const text = document.getElementById('parse-text').value;
        const langSelect = document.getElementById('parse-lang');
        const warningState = getLangWarningState(text, langSelect.value);
        if (warningState.canSwitch) {
            langSelect.value = warningState.detected;
            updateLangWarning();
        }
    });
    // Load file into textarea
    document.getElementById('parse-file')
        ?.addEventListener('change', async (e) => {
        const input = e.target;
        const file = input.files?.[0];
        if (!file)
            return;
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
    document.querySelectorAll('.sort-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            if (!currentResults)
                return;
            const key = btn.dataset.sort;
            if (!key)
                return;
            currentSort = currentSort.key === key
                ? { key, dir: currentSort.dir === 'asc' ? 'desc' : 'asc' }
                : { key, dir: key === 'tokens' ? 'desc' : 'asc' };
            renderResultsTable(currentResults);
        });
    });
    updateSortButtons();
});
