package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- publicWords ------------------------------------------------------------
//
// Locks in the safety guarantees the refresh-queue path relies on:
//   - non-Estonian rows are filtered out (the upstream dataset is multilingual)
//   - duplicate wordIds in a single response collapse to one entry
//   - empty list, all-non-est, and any wordId=0 are *errors*, not silent
//     truncations of the snapshot
//
// publicWords is a method on apiClient, so the tests drive it through
// httptest. That also exercises the HTTP error path (non-200) and the JSON
// decode error path.

func newTestClient(t *testing.T, handler http.Handler) (*apiClient, func()) {
	t.Helper()
	srv := httptest.NewServer(handler)
	c := newAPIClient(srv.URL, "test-key")
	return c, srv.Close
}

func TestPublicWords(t *testing.T) {
	tests := []struct {
		name       string
		body       any    // marshalled to JSON; raw JSON string allowed too
		rawBody    string // overrides body when non-empty (for malformed JSON)
		status     int
		wantErr    string // substring match; empty == no error
		wantLen    int
		wantIDs    []int64
		wantLemmas []string
	}{
		{
			name:   "happy path filters non-est and preserves order",
			status: 200,
			body: []apiPublicWord{
				{WordID: 1, Value: "koer", Lang: "est", MorphExists: true},
				{WordID: 2, Value: "собака", Lang: "rus"},
				{WordID: 3, Value: "maja", Lang: "est"},
				{WordID: 4, Value: "kala", Lang: "fin"},
			},
			wantLen:    2,
			wantIDs:    []int64{1, 3},
			wantLemmas: []string{"koer", "maja"},
		},
		{
			name:   "lang match is case-insensitive",
			status: 200,
			body: []apiPublicWord{
				{WordID: 10, Value: "üks", Lang: "EST"},
				{WordID: 11, Value: "kaks", Lang: "Est"},
			},
			wantLen: 2,
		},
		{
			name:   "duplicate wordIds collapse to one entry",
			status: 200,
			body: []apiPublicWord{
				{WordID: 100, Value: "kuusk", Lang: "est"},
				{WordID: 100, Value: "kuusk", Lang: "est"},
				{WordID: 101, Value: "mänd", Lang: "est"},
			},
			wantLen: 2,
			wantIDs: []int64{100, 101},
		},
		{
			name:    "empty response is rejected (don't overwrite snapshot with nothing)",
			status:  200,
			body:    []apiPublicWord{},
			wantErr: "empty list",
		},
		{
			name:   "all non-est is rejected (likely schema rename)",
			status: 200,
			body: []apiPublicWord{
				{WordID: 1, Value: "x", Lang: "rus"},
				{WordID: 2, Value: "y", Lang: "fin"},
			},
			wantErr: "none matched lang=est",
		},
		{
			name:   "wordId=0 in any est row is rejected (partial decode)",
			status: 200,
			body: []apiPublicWord{
				{WordID: 1, Value: "ok", Lang: "est"},
				{WordID: 0, Value: "broken", Lang: "est"},
			},
			wantErr: "wordId=0",
		},
		{
			name:    "non-200 surfaces as an error",
			status:  503,
			rawBody: `service unavailable`,
			wantErr: "HTTP 503",
		},
		{
			name:    "malformed JSON surfaces as an error",
			status:  200,
			rawBody: `not json`,
			wantErr: "decode public_word",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				if tc.rawBody != "" {
					_, _ = w.Write([]byte(tc.rawBody))
					return
				}
				_ = json.NewEncoder(w).Encode(tc.body)
			})
			c, stop := newTestClient(t, h)
			defer stop()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			got, err := c.publicWords(ctx, "eki")

			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("want error containing %q, got nil (result=%v)", tc.wantErr, got)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tc.wantLen {
				t.Fatalf("len=%d, want %d (got=%+v)", len(got), tc.wantLen, got)
			}
			for i, id := range tc.wantIDs {
				if got[i].WordID != id {
					t.Errorf("[%d] WordID=%d, want %d", i, got[i].WordID, id)
				}
			}
			for i, lemma := range tc.wantLemmas {
				if got[i].Lemma != lemma {
					t.Errorf("[%d] Lemma=%q, want %q", i, got[i].Lemma, lemma)
				}
			}
			// Lang and SourceDataset should be normalized regardless of input.
			for i, w := range got {
				if w.Lang != "ET" {
					t.Errorf("[%d] Lang=%q, want ET", i, w.Lang)
				}
				if w.SourceDataset != "eki" {
					t.Errorf("[%d] SourceDataset=%q, want eki", i, w.SourceDataset)
				}
			}
		})
	}
}

// --- writeQueue -------------------------------------------------------------
//
// The malformed-row guard is what stops a future regression from silently
// blowing away the tracked snapshot. These tests assert that any rejected
// input leaves the destination file untouched (no half-written file, no .tmp
// renamed into place).

func TestWriteQueue(t *testing.T) {
	good := []publicWord{
		{WordID: 1, Lemma: "koer", Lang: "ET", SourceDataset: "eki"},
		{WordID: 2, Lemma: "maja", Lang: "ET", SourceDataset: "eki"},
	}

	tests := []struct {
		name    string
		words   []publicWord
		wantErr string // empty == success
	}{
		{
			name:    "empty slice refuses to write",
			words:   nil,
			wantErr: "empty queue",
		},
		{
			name: "wordId=0 anywhere refuses to write the whole batch",
			words: []publicWord{
				{WordID: 1, Lemma: "ok"},
				{WordID: 0, Lemma: "broken"},
				{WordID: 3, Lemma: "ok2"},
			},
			wantErr: "malformed entry at index 1",
		},
		{
			name: "empty lemma refuses to write",
			words: []publicWord{
				{WordID: 5, Lemma: ""},
			},
			wantErr: "malformed entry at index 0",
		},
		{
			name: "whitespace-only lemma refuses to write",
			words: []publicWord{
				{WordID: 6, Lemma: "   \t  "},
			},
			wantErr: "malformed entry at index 0",
		},
		{
			name:  "happy path writes every row",
			words: good,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "queue.jsonl")
			err := writeQueue(path, tc.words)

			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("want error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
				}
				// The whole point of the guard: target file must NOT have been written.
				if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
					t.Fatalf("guard rejected input but %s exists (statErr=%v)", path, statErr)
				}
				// The .tmp staging file must not have been left behind either.
				if _, statErr := os.Stat(path + ".tmp"); !os.IsNotExist(statErr) {
					t.Fatalf("guard rejected input but %s.tmp exists", path)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// Round-trip: read it back and confirm wordIds match.
			got, err := readQueue(path)
			if err != nil {
				t.Fatalf("readQueue: %v", err)
			}
			if len(got) != len(tc.words) {
				t.Fatalf("round-trip len=%d, want %d", len(got), len(tc.words))
			}
			for i, w := range got {
				if w.WordID != tc.words[i].WordID || w.Lemma != tc.words[i].Lemma {
					t.Errorf("[%d] got %+v, want %+v", i, w, tc.words[i])
				}
			}
		})
	}
}

// --- filterPending ----------------------------------------------------------
//
// The contract: only rows with status='ok' or status='not_found' in the
// checkpoint are skipped on resume. Everything else (errored, partial,
// unknown future statuses) must be re-attempted. A one-character bug here
// silently turns a resumable scraper into a do-nothing scraper.

func TestFilterPending(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "checkpoint.sqlite")
	db, err := openCheckpoint(dbPath)
	if err != nil {
		t.Fatalf("openCheckpoint: %v", err)
	}
	defer db.Close()

	// Seed every status we care about. Anything other than 'ok'/'not_found'
	// must be retried.
	seed := []struct {
		id     int64
		status string
	}{
		{1, "ok"},
		{2, "not_found"},
		{3, "error"},
		{4, "auth_error"},
		{5, "exhausted"},
		{6, "future_unknown_status"},
	}
	for _, s := range seed {
		if _, err := db.Exec(
			`INSERT INTO fetch_status(word_id, status) VALUES (?, ?)`,
			s.id, s.status,
		); err != nil {
			t.Fatalf("seed %d/%s: %v", s.id, s.status, err)
		}
	}

	// Queue includes every seeded id plus an unseen id (7).
	queue := []publicWord{
		{WordID: 1, Lemma: "ok-row"},
		{WordID: 2, Lemma: "not-found-row"},
		{WordID: 3, Lemma: "error-row"},
		{WordID: 4, Lemma: "auth-error-row"},
		{WordID: 5, Lemma: "exhausted-row"},
		{WordID: 6, Lemma: "unknown-status-row"},
		{WordID: 7, Lemma: "never-attempted-row"},
	}

	todo, alreadyDone, err := filterPending(db, queue)
	if err != nil {
		t.Fatalf("filterPending: %v", err)
	}

	wantSkipped := map[int64]bool{1: true, 2: true} // ok + not_found only
	wantTodoIDs := []int64{}
	for _, w := range queue {
		if !wantSkipped[w.WordID] {
			wantTodoIDs = append(wantTodoIDs, w.WordID)
		}
	}
	if alreadyDone != len(wantSkipped) {
		t.Errorf("alreadyDone=%d, want %d", alreadyDone, len(wantSkipped))
	}
	if len(todo) != len(wantTodoIDs) {
		t.Fatalf("todo len=%d, want %d (todo=%+v)", len(todo), len(wantTodoIDs), todo)
	}
	for i, w := range todo {
		if w.WordID != wantTodoIDs[i] {
			t.Errorf("[%d] WordID=%d, want %d", i, w.WordID, wantTodoIDs[i])
		}
		if wantSkipped[w.WordID] {
			t.Errorf("WordID=%d should have been skipped (status was ok/not_found)", w.WordID)
		}
	}

	// Sanity: empty queue → empty result.
	todo2, done2, err := filterPending(db, nil)
	if err != nil {
		t.Fatalf("empty queue: %v", err)
	}
	if len(todo2) != 0 || done2 != 0 {
		t.Errorf("empty queue: todo=%v done=%d, want empty/0", todo2, done2)
	}
}

// --- empty checkpoint -------------------------------------------------------

func TestFilterPendingEmptyCheckpoint(t *testing.T) {
	dir := t.TempDir()
	db, err := openCheckpoint(filepath.Join(dir, "checkpoint.sqlite"))
	if err != nil {
		t.Fatalf("openCheckpoint: %v", err)
	}
	defer db.Close()

	queue := []publicWord{
		{WordID: 1, Lemma: "a"},
		{WordID: 2, Lemma: "b"},
	}
	todo, done, err := filterPending(db, queue)
	if err != nil {
		t.Fatalf("filterPending: %v", err)
	}
	if done != 0 {
		t.Errorf("alreadyDone=%d, want 0", done)
	}
	if len(todo) != len(queue) {
		t.Fatalf("todo len=%d, want %d", len(todo), len(queue))
	}
}
