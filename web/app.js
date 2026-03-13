// FinEstDB — parse word list view

const MAX_CHARS = 10_000;

const POS_LABELS = {
    NOUN: 'Noun',
    VERB: 'Verb',
    ADJ:  'Adjective',
    ADV:  'Adverb',
    PRON: 'Pronoun',
    DET:  'Determiner',
    ADP:  'Adposition',
    NUM:  'Numeral',
    PUNCT:'Punctuation',
    SYM:  'Symbol',
    INTJ: 'Interjection',
    CCONJ:'Conjunction',
    SCONJ:'Conjunction',
    PART: 'Particle',
    AUX:  'Auxiliary',
    PROPN:'Proper noun',
    X:    'Other',
};

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
    const letters = (lower.match(/[a-zäöüõ]/g) || []);
    if (letters.length === 0) return 'unknown';

    if (/õ/.test(lower)) return 'ET';

    const nordicCount = (lower.match(/[äö]/g) || []).length;
    if (nordicCount / letters.length > 0.015) return 'FI';

    return 'unknown';
}

function getLangWarning(text, selectedLang) {
    const detected = detectLang(text.trim());
    if (detected === 'unknown') {
        const langName = selectedLang === 'FI' ? 'Finnish' : 'Estonian';
        return `Warning: this text doesn't contain Finnish or Estonian characters (ä, ö, õ). Is it really ${langName}?`;
    }
    if (detected === 'FI' && selectedLang === 'ET') {
        return 'Warning: this text looks like Finnish (has ä/ö, no õ), but Estonian is selected.';
    }
    if (detected === 'ET' && selectedLang === 'FI') {
        return 'Warning: this text looks like Estonian (contains õ), but Finnish is selected.';
    }
    return null;
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

    if (text.trim().length < 20) {
        warningEl.classList.add('hidden');
        return;
    }

    const warning = getLangWarning(text, lang);
    if (warning) {
        warningEl.textContent = warning;
        warningEl.classList.remove('hidden');
    } else {
        warningEl.classList.add('hidden');
    }
}

async function handleParseSubmit(e) {
    e.preventDefault();

    const text = document.getElementById('parse-text').value.trim();
    const lang = document.getElementById('parse-lang').value;
    const btn  = document.getElementById('parse-btn');

    if (!text) return;
    if (text.length > MAX_CHARS) {
        alert(`Text must be ${MAX_CHARS.toLocaleString()} characters or fewer.`);
        return;
    }

    btn.disabled = true;
    btn.textContent = 'Parsing…';

    try {
        const resp = await fetch('/api/parse', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ lang, text }),
        });

        if (!resp.ok) {
            const msg = await resp.text();
            throw new Error(msg || resp.statusText);
        }

        const data = await resp.json();
        showResults(data, text.slice(0, 60));
    } catch (err) {
        alert(`Parse failed: ${err.message}`);
    } finally {
        btn.disabled = false;
        btn.textContent = 'Parse Text';
    }
}

// ── Results rendering ──────────────────────────────────────────────────────

function showResults(data, textPreview) {
    const langName = data.lang === 'FI' ? 'Finnish' : 'Estonian';
    const preview  = textPreview.replace(/\s+/g, ' ').trim();
    const ellipsis = preview.length >= 60 ? '…' : '';

    document.getElementById('results-title').textContent =
        `"${preview}${ellipsis}" (${langName})`;

    document.getElementById('results-stats').textContent =
        `${data.words.length} unique lemmas · ${data.total_tokens} tokens`;

    const tbody = document.getElementById('word-table-body');
    tbody.innerHTML = data.words.map(w => {
        const forms = w.forms.slice(0, 3).map(escapeHtml).join(', ')
            + (w.forms.length > 3 ? ` +${w.forms.length - 3}` : '');
        return `<tr>
            <td class="col-lemma">${escapeHtml(w.lemma)}</td>
            <td class="col-pos">${escapeHtml(posLabel(w.pos))}</td>
            <td class="col-forms">${forms}</td>
            <td class="col-count">${w.count}</td>
        </tr>`;
    }).join('');

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

    // Load file into textarea
    document.getElementById('parse-file')
        ?.addEventListener('change', async (e) => {
            const file = e.target.files?.[0];
            if (!file) return;
            const text = await file.text();
            textarea.value = text.slice(0, MAX_CHARS);
            updateCharCount();
            updateLangWarning();
        });

    // Parse form submit
    document.getElementById('parse-form')
        ?.addEventListener('submit', handleParseSubmit);
});
