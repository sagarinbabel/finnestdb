// Application state
let currentUser: { userID: number; email: string } | null = null;
let currentCard: any = null;

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

function updateThemeIcon(theme: string) {
    const icons = document.querySelectorAll('.theme-icon');
    icons.forEach(icon => {
        icon.textContent = theme === 'light' ? '🌙' : '☀️';
    });
}

// Page navigation
function showPage(pageId: string) {
    document.querySelectorAll('.page').forEach(page => {
        page.classList.remove('active');
    });
    const page = document.getElementById(pageId);
    if (page) {
        page.classList.add('active');
    }
}

// API calls
async function apiCall(endpoint: string, options: RequestInit = {}) {
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

async function login(email: string, password: string) {
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

async function createDeck(title: string, lang: string, text: string) {
    return apiCall('/decks', {
        method: 'POST',
        body: JSON.stringify({ title, lang, text }),
    });
}

async function getNextCard() {
    return apiCall('/review/next');
}

async function answerCard(cardID: string, quality: number) {
    return apiCall('/review/answer', {
        method: 'POST',
        body: JSON.stringify({ card_id: cardID, quality }),
    });
}

async function ignoreCard(lemma: string, pos: string) {
    return apiCall('/card/ignore', {
        method: 'POST',
        body: JSON.stringify({ lemma, pos }),
    });
}

async function markKnown(lemma: string, pos: string) {
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
        document.getElementById('known-count')!.textContent = data.known_count.toString();
        document.getElementById('due-count')!.textContent = data.due_count.toString();
        document.getElementById('new-capacity')!.textContent = data.new_capacity_today.toString();
        
        // Update user email
        if (currentUser) {
            document.getElementById('user-email')!.textContent = currentUser.email;
        }
        
        // Render decks
        renderDecks(data.decks);
    } catch (error) {
        console.error('Failed to load dashboard:', error);
        alert('Failed to load dashboard');
    }
}

function renderDecks(decks: any[]) {
    const decksList = document.getElementById('decks-list')!;
    
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

function escapeHtml(text: string): string {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

// Upload modal
function showUploadModal() {
    const modal = document.getElementById('upload-modal')!;
    modal.classList.add('active');
}

function hideUploadModal() {
    const modal = document.getElementById('upload-modal')!;
    modal.classList.remove('active');
    const form = document.getElementById('upload-form') as HTMLFormElement;
    form.reset();
}

async function handleUpload(e: Event) {
    e.preventDefault();
    const form = e.target as HTMLFormElement;
    const formData = new FormData(form);
    
    const title = (document.getElementById('deck-title') as HTMLInputElement).value;
    const lang = (document.getElementById('deck-lang') as HTMLSelectElement).value;
    let text = (document.getElementById('deck-text') as HTMLTextAreaElement).value;
    
    // Handle file upload
    const fileInput = document.getElementById('deck-file') as HTMLInputElement;
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
async function startReview(deckID?: string) {
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

function renderCard(card: any) {
    const cardElement = document.getElementById('card')!;
    cardElement.classList.remove('flipped');
    
    // Front
    const frontText = document.getElementById('card-front-text')!;
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
    const badge = document.getElementById('card-badge')!;
    if (card.mode === 'word') {
        badge.textContent = 'WORD';
    } else if (card.mode === 'generated') {
        badge.textContent = 'GENERATED';
    } else {
        badge.textContent = 'NEW';
    }
    
    // Back
    document.getElementById('card-lemma')!.textContent = card.back.lemma;
    document.getElementById('card-meaning')!.textContent = card.back.meaning;
    document.getElementById('card-grammar')!.textContent = card.back.grammar;
    
    // Examples
    const examplesList = document.getElementById('card-examples')!;
    examplesList.innerHTML = card.back.examples.map((ex: any) => 
        `<li>${escapeHtml(ex.text)} <span style="color: var(--text-secondary);">(${escapeHtml(ex.source_deck)})</span></li>`
    ).join('');
    
    // Deck counts
    const deckCounts = document.getElementById('card-deck-counts')!;
    if (card.deck_counts && card.deck_counts.length > 0) {
        deckCounts.textContent = card.deck_counts.map(([deck, count]: [string, string]) => 
            `${count}× ${deck}`
        ).join(', ');
    } else {
        deckCounts.textContent = '';
    }
}

function flipCard() {
    const card = document.getElementById('card')!;
    card.classList.add('flipped');
}

async function handleAnswer(quality: number) {
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
function deleteDeck(deckID: string) {
    if (confirm('Delete this deck?')) {
        // Stub: just reload dashboard
        loadDashboard();
    }
}

// Event listeners
document.addEventListener('DOMContentLoaded', () => {
    initTheme();
    
    // Theme toggles
    document.getElementById('theme-toggle')?.addEventListener('click', toggleTheme);
    document.getElementById('header-theme-toggle')?.addEventListener('click', toggleTheme);
    
    // Login form
    document.getElementById('login-form')?.addEventListener('submit', async (e) => {
        e.preventDefault();
        const email = (document.getElementById('email') as HTMLInputElement).value;
        const password = (document.getElementById('password') as HTMLInputElement).value;
        
        try {
            await login(email, password);
            showPage('dashboard-page');
            await loadDashboard();
        } catch (error) {
            console.error('Login failed:', error);
            alert('Login failed');
        }
    });
    
    // Dashboard actions
    document.getElementById('learn-btn')?.addEventListener('click', () => startReview());
    document.getElementById('upload-btn')?.addEventListener('click', showUploadModal);
    
    // Upload modal
    document.getElementById('upload-modal-close')?.addEventListener('click', hideUploadModal);
    document.getElementById('upload-cancel')?.addEventListener('click', hideUploadModal);
    document.getElementById('upload-form')?.addEventListener('submit', handleUpload);
    
    // File input handler
    document.getElementById('deck-file')?.addEventListener('change', async (e) => {
        const input = e.target as HTMLInputElement;
        if (input.files && input.files.length > 0) {
            const file = input.files[0];
            const text = await file.text();
            (document.getElementById('deck-text') as HTMLTextAreaElement).value = text;
        }
    });
    
    // Review page
    document.getElementById('review-back')?.addEventListener('click', () => {
        showPage('dashboard-page');
        loadDashboard();
    });
    
    document.getElementById('flip-card')?.addEventListener('click', flipCard);
    
    // Review buttons
    document.getElementById('btn-again')?.addEventListener('click', () => handleAnswer(0));
    document.getElementById('btn-hard')?.addEventListener('click', () => handleAnswer(1));
    document.getElementById('btn-good')?.addEventListener('click', () => handleAnswer(2));
    document.getElementById('btn-easy')?.addEventListener('click', () => handleAnswer(3));
    document.getElementById('btn-ignore')?.addEventListener('click', handleIgnore);
    document.getElementById('btn-known')?.addEventListener('click', handleMarkKnown);
    
    // Modal backdrop click
    document.getElementById('upload-modal')?.addEventListener('click', (e) => {
        if (e.target === e.currentTarget) {
            hideUploadModal();
        }
    });
});

// Make functions available globally for onclick handlers
(window as any).startReview = startReview;
(window as any).deleteDeck = deleteDeck;

