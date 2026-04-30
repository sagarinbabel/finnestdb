package store

import (
	"path/filepath"
	"testing"
)

func TestGetNextReviewCardRespectsDailyNewCardLimit(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	user, err := db.GetOrCreateUser("limit@example.com")
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}
	if _, err := db.db.Exec(
		`UPDATE users SET settings_json = ? WHERE id = ?`,
		`{"new_per_day":1,"retention":0.9,"theme":"system"}`,
		user.ID,
	); err != nil {
		t.Fatalf("update user settings: %v", err)
	}

	if _, err := db.EnsureCard(user.ID, "FI", "kissa", "NOUN"); err != nil {
		t.Fatalf("EnsureCard(kissa): %v", err)
	}
	if _, err := db.EnsureCard(user.ID, "FI", "koira", "NOUN"); err != nil {
		t.Fatalf("EnsureCard(koira): %v", err)
	}

	card, err := db.GetNextReviewCard(user.ID, nil)
	if err != nil {
		t.Fatalf("GetNextReviewCard(before answer): %v", err)
	}
	if card == nil {
		t.Fatal("expected first new card before limit is exhausted")
	}

	if err := db.RecordReviewAnswer(user.ID, card.CardID, "good"); err != nil {
		t.Fatalf("RecordReviewAnswer: %v", err)
	}

	remaining, err := db.CountNewCards(user.ID)
	if err != nil {
		t.Fatalf("CountNewCards: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("remaining new cards=%d want 0 after hitting daily limit", remaining)
	}

	next, err := db.GetNextReviewCard(user.ID, nil)
	if err != nil {
		t.Fatalf("GetNextReviewCard(after answer): %v", err)
	}
	if next != nil {
		t.Fatalf("expected no additional new card after daily limit, got %+v", next)
	}
}
