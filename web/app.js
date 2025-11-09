// Application state
let currentUser = null;
let currentCard = null;

// API base URL
const API_BASE = '/api';

// Theme management
function initTheme() {
    const savedTheme = localStorage.getItem('theme') || 'light';
    document.documentElement.setAttribute('data-theme', savedTheme);
    updateThemeIcon(savedTheme);
}

function toggleTheme() {
    const currentTheme = document.documentElement.getAttribute('data-theme') || 'light';
    const newTheme = currentTheme === 'light' ? 'dark' : 'light';
    document.documentElement.setAttribute('data-theme', newTheme);
    localStorage.setItem('theme', newTheme);
    updateThemeIcon(newTheme);
}

function updateThemeIcon(theme) {
    const icons = document.querySelectorAll('.theme-icon');
    icons.forEach(icon => {
        icon.textContent = theme === 'light' ? '🌙' : '☀️';
    });
}

// Page navigation
function showPage(pageId) {
    document.querySelectorAll('.page').forEach(page => {
        page.classList.remove('active');
    });
    const page = document.getElementById(pageId);
    if (page) {
        page.classList.add('active');
    }
}

// API calls
async function apiCall(endpoint, options = {}) {
    const response = await fetch(`${API_BASE}${endpoint}`, {
        ...options,
        headers: {
            'Content-Type': 'application/json',
            ...options.headers,
        },
    });
    if (!response.ok) {
        throw new Error(`API error: ${response.statusText}`);
    }
    return response.json();
}

async function login(email, password) {
    const data = await apiCall('/auth/login', {
        method: 'POST',
        body: JSON.stringify({ email, password }),
    });
    currentUser = { userID: data.user_id, email: data.email };
    return data;
}

async function getDashboard() {
    return apiCall('/me');
}

async function createDeck(title, lang, text) {
    return apiCall('/decks', {
        method: 'POST',
        body: JSON.stringify({ title, lang, text }),
    });
}

async function getNextCard() {
    return apiCall('/review/next');
}

async function answerCard(cardID, quality) {
    return apiCall('/review/answer', {
        method: 'POST',
        body: JSON.stringify({ card_id: cardID, quality }),
    });
}

async function ignoreCard(lemma, pos) {
    return apiCall('/card/ignore', {
        method: 'POST',
        body: JSON.stringify({ lemma, pos }),
    });
}

async function markKnown(lemma, pos) {
    return apiCall('/card/known', {
        method: 'POST',
        body: JSON.stringify({ lemma, pos }),
    });
}

// Dashboard rendering
async function loadDashboard() {
    try {
        const data = await getDashboard();
        
        // Update KPIs
        document.getElementById('known-count').textContent = data.known_count.toString();
        document.getElementById('due-count').textContent = data.due_count.toString();
        document.getElementById('new-capacity').textContent = data.new_capacity_today.toString();
        
        // Update user email
        if (currentUser) {
            document.getElementById('user-email').textContent = currentUser.email;
        }
        
        // Render decks
        renderDecks(data.decks);
    } catch (error) {
        console.error('Failed to load dashboard:', error);
        alert('Failed to load dashboard');
    }
}

function renderDecks(decks) {
    const decksList = document.getElementById('decks-list');
    
    if (decks.length === 0) {
        decksList.innerHTML = '<p style="color: var(--text-secondary);">No decks yet. Create your first deck!</p>';
        return;
    }
    
    decksList.innerHTML = decks.map(deck => `
        <div class="deck-item">
            <div class="deck-info">
                <h3>${escapeHtml(deck.title)}</h3>
                <p>${deck.lang} • ${deck.known}/${deck.unique} known • ${deck.due} due</p>
            </div>
            <div class="deck-actions">
                <button class="btn btn-secondary" onclick="startReview('${deck.id}')">Review</button>
                <button class="btn btn-secondary" onclick="deleteDeck('${deck.id}')">Delete</button>
            </div>
        </div>
    `).join('');
}

function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

// Upload modal
function showUploadModal() {
    const modal = document.getElementById('upload-modal');
    modal.classList.add('active');
}

function hideUploadModal() {
    const modal = document.getElementById('upload-modal');
    modal.classList.remove('active');
    const form = document.getElementById('upload-form');
    form.reset();
}

async function handleUpload(e) {
    e.preventDefault();
    const form = e.target;
    
    const title = document.getElementById('deck-title').value;
    const lang = document.getElementById('deck-lang').value;
    let text = document.getElementById('deck-text').value;
    
    // Handle file upload
    const fileInput = document.getElementById('deck-file');
    if (fileInput.files && fileInput.files.length > 0) {
        const file = fileInput.files[0];
        if (file.size > 2 * 1024 * 1024) {
            alert('File size must be less than 2 MB');
            return;
        }
        text = await file.text();
    }
    
    if (!text.trim()) {
        alert('Please provide text or upload a file');
        return;
    }
    
    try {
        await createDeck(title, lang, text);
        hideUploadModal();
        await loadDashboard();
        alert('Deck created successfully!');
    } catch (error) {
        console.error('Failed to create deck:', error);
        alert('Failed to create deck');
    }
}

// Review/Learn interface
async function startReview(deckID) {
    try {
        await loadNextCard();
        showPage('review-page');
    } catch (error) {
        console.error('Failed to start review:', error);
        alert('No cards available');
    }
}

async function loadNextCard() {
    try {
        currentCard = await getNextCard();
        renderCard(currentCard);
    } catch (error) {
        console.error('Failed to load card:', error);
        throw error;
    }
}

function renderCard(card) {
    const cardElement = document.getElementById('card');
    cardElement.classList.remove('flipped');
    
    // Front
    const frontText = document.getElementById('card-front-text');
    if (card.front.type === 'sentence') {
        const text = card.front.text;
        const highlight = card.front.highlight || '';
        if (highlight) {
            const highlighted = text.replace(
                new RegExp(highlight, 'gi'),
                `<span class="highlight">${highlight}</span>`
            );
            frontText.innerHTML = highlighted;
        } else {
            frontText.textContent = text;
        }
    } else {
        frontText.textContent = card.front.text;
    }
    
    // Badge
    const badge = document.getElementById('card-badge');
    if (card.mode === 'word') {
        badge.textContent = 'WORD';
    } else if (card.mode === 'generated') {
        badge.textContent = 'GENERATED';
    } else {
        badge.textContent = 'NEW';
    }
    
    // Back
    document.getElementById('card-lemma').textContent = card.back.lemma;
    document.getElementById('card-meaning').textContent = card.back.meaning;
    document.getElementById('card-grammar').textContent = card.back.grammar;
    
    // Examples
    const examplesList = document.getElementById('card-examples');
    examplesList.innerHTML = card.back.examples.map(ex => 
        `<li>${escapeHtml(ex.text)} <span style="color: var(--text-secondary);">(${escapeHtml(ex.source_deck)})</span></li>`
    ).join('');
    
    // Deck counts
    const deckCounts = document.getElementById('card-deck-counts');
    if (card.deck_counts && card.deck_counts.length > 0) {
        deckCounts.textContent = card.deck_counts.map(([deck, count]) => 
            `${count}× ${deck}`
        ).join(', ');
    } else {
        deckCounts.textContent = '';
    }
}

function flipCard() {
    const card = document.getElementById('card');
    card.classList.add('flipped');
}

async function handleAnswer(quality) {
    if (!currentCard) return;
    
    try {
        await answerCard(currentCard.card_id, quality);
        await loadNextCard();
    } catch (error) {
        console.error('Failed to answer card:', error);
    }
}

async function handleIgnore() {
    if (!currentCard) return;
    
    try {
        await ignoreCard(currentCard.back.lemma, 'NOUN'); // Stub: use NOUN
        await loadNextCard();
    } catch (error) {
        console.error('Failed to ignore card:', error);
    }
}

async function handleMarkKnown() {
    if (!currentCard) return;
    
    try {
        await markKnown(currentCard.back.lemma, 'NOUN'); // Stub: use NOUN
        await loadNextCard();
    } catch (error) {
        console.error('Failed to mark as known:', error);
    }
}

// Stub functions for deck actions
function deleteDeck(deckID) {
    if (confirm('Delete this deck?')) {
        // Stub: just reload dashboard
        loadDashboard();
    }
}

// Event listeners
document.addEventListener('DOMContentLoaded', () => {
    initTheme();
    
    // Theme toggles
    const themeToggle = document.getElementById('theme-toggle');
    if (themeToggle) themeToggle.addEventListener('click', toggleTheme);
    
    const headerThemeToggle = document.getElementById('header-theme-toggle');
    if (headerThemeToggle) headerThemeToggle.addEventListener('click', toggleTheme);
    
    // Login form
    const loginForm = document.getElementById('login-form');
    if (loginForm) {
        loginForm.addEventListener('submit', async (e) => {
            e.preventDefault();
            const email = document.getElementById('email').value;
            const password = document.getElementById('password').value;
            
            try {
                await login(email, password);
                showPage('dashboard-page');
                await loadDashboard();
            } catch (error) {
                console.error('Login failed:', error);
                alert('Login failed');
            }
        });
    }
    
    // Dashboard actions
    const learnBtn = document.getElementById('learn-btn');
    if (learnBtn) learnBtn.addEventListener('click', () => startReview());
    
    const uploadBtn = document.getElementById('upload-btn');
    if (uploadBtn) uploadBtn.addEventListener('click', showUploadModal);
    
    // Upload modal
    const uploadModalClose = document.getElementById('upload-modal-close');
    if (uploadModalClose) uploadModalClose.addEventListener('click', hideUploadModal);
    
    const uploadCancel = document.getElementById('upload-cancel');
    if (uploadCancel) uploadCancel.addEventListener('click', hideUploadModal);
    
    const uploadForm = document.getElementById('upload-form');
    if (uploadForm) uploadForm.addEventListener('submit', handleUpload);
    
    // File input handler
    const deckFile = document.getElementById('deck-file');
    if (deckFile) {
        deckFile.addEventListener('change', async (e) => {
            const input = e.target;
            if (input.files && input.files.length > 0) {
                const file = input.files[0];
                const text = await file.text();
                document.getElementById('deck-text').value = text;
            }
        });
    }
    
    // Review page
    const reviewBack = document.getElementById('review-back');
    if (reviewBack) {
        reviewBack.addEventListener('click', () => {
            showPage('dashboard-page');
            loadDashboard();
        });
    }
    
    const flipCardBtn = document.getElementById('flip-card');
    if (flipCardBtn) flipCardBtn.addEventListener('click', flipCard);
    
    // Review buttons
    const btnAgain = document.getElementById('btn-again');
    if (btnAgain) btnAgain.addEventListener('click', () => handleAnswer(0));
    
    const btnHard = document.getElementById('btn-hard');
    if (btnHard) btnHard.addEventListener('click', () => handleAnswer(1));
    
    const btnGood = document.getElementById('btn-good');
    if (btnGood) btnGood.addEventListener('click', () => handleAnswer(2));
    
    const btnEasy = document.getElementById('btn-easy');
    if (btnEasy) btnEasy.addEventListener('click', () => handleAnswer(3));
    
    const btnIgnore = document.getElementById('btn-ignore');
    if (btnIgnore) btnIgnore.addEventListener('click', handleIgnore);
    
    const btnKnown = document.getElementById('btn-known');
    if (btnKnown) btnKnown.addEventListener('click', handleMarkKnown);
    
    // Modal backdrop click
    const uploadModal = document.getElementById('upload-modal');
    if (uploadModal) {
        uploadModal.addEventListener('click', (e) => {
            if (e.target === e.currentTarget) {
                hideUploadModal();
            }
        });
    }
});

// Make functions available globally for onclick handlers
window.startReview = startReview;
window.deleteDeck = deleteDeck;

