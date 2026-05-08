package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCheckRequiredFilesRequiresSentencesUserFriendly(t *testing.T) {
	derived := t.TempDir()
	for _, name := range requiredDerivedFiles() {
		if name == "sentences_user_friendly.tsv" {
			continue
		}
		path := filepath.Join(derived, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("header\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	failures := checkRequiredFiles(derived)
	if !slices.Contains(failures, "missing_required: sentences_user_friendly.tsv") {
		t.Fatalf("failures = %#v, want missing sentences_user_friendly.tsv", failures)
	}
}

func TestCheckRequiredFilesPassesWithSentencesUserFriendly(t *testing.T) {
	derived := t.TempDir()
	for _, name := range requiredDerivedFiles() {
		path := filepath.Join(derived, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("header\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	failures := checkRequiredFiles(derived)
	if len(failures) > 0 {
		t.Fatalf("checkRequiredFiles failures = %s", strings.Join(failures, ", "))
	}
}
