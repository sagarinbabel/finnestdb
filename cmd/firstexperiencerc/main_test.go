package main

import (
	"os"
	"path/filepath"
	"testing"

	"finnestdb/internal/parsecore"
)

// repoRootManifestPath resolves defaultManifestPath against the repo root
// rather than the package directory, since `go test` runs with the package
// directory as cwd but defaultManifestPath is written relative to repo root
// (matching how -manifest's default is documented and how `make
// first-experience-rc` invokes the binary from repo root).
func repoRootManifestPath(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, defaultManifestPath)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find repo root (go.mod) above %s", dir)
		}
		dir = parent
	}
}

// TestLoadManifest_Canonical exercises the loader against the real checked-in
// manifest so a schema drift (renamed field, wrong array shape) fails this
// test instead of surfacing only as a silent zero-case run.
func TestLoadManifest_Canonical(t *testing.T) {
	m, err := loadManifest(repoRootManifestPath(t))
	if err != nil {
		t.Fatalf("loadManifest(%q): %v", defaultManifestPath, err)
	}
	if len(m.Cases) == 0 {
		t.Fatalf("canonical manifest has no cases")
	}

	seenLangs := map[string]bool{}
	seenJourneys := map[string]bool{}
	for _, c := range m.Cases {
		if c.ID == "" {
			t.Errorf("case with empty id: %+v", c)
		}
		if c.Language != "fi" && c.Language != "et" {
			t.Errorf("case %s: language must be fi or et, got %q", c.ID, c.Language)
		}
		switch c.Automation {
		case "parser", "playwright", "manual", "pending":
		default:
			t.Errorf("case %s: unsupported automation %q", c.ID, c.Automation)
		}
		if c.Automation == "parser" && c.Fixture == "" {
			t.Errorf("case %s: automation:parser requires a fixture path", c.ID)
		}
		seenLangs[c.Language] = true
		seenJourneys[c.Journey] = true
	}

	// The spec requires both FI and ET cases for every journey, even if
	// pending. This is the drift guard that catches a journey losing its
	// ET (or FI) parity case when the manifest is edited later.
	journeyLangs := map[string]map[string]bool{}
	for _, c := range m.Cases {
		if journeyLangs[c.Journey] == nil {
			journeyLangs[c.Journey] = map[string]bool{}
		}
		journeyLangs[c.Journey][c.Language] = true
	}
	for journey, langs := range journeyLangs {
		if !langs["fi"] {
			t.Errorf("journey %q has no fi case", journey)
		}
		if !langs["et"] {
			t.Errorf("journey %q has no et case", journey)
		}
	}
}

func TestLoadManifest_Errors(t *testing.T) {
	dir := t.TempDir()

	t.Run("missing file", func(t *testing.T) {
		_, err := loadManifest(filepath.Join(dir, "does-not-exist.json"))
		if err == nil {
			t.Fatal("expected error for missing manifest file")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		path := filepath.Join(dir, "bad.json")
		if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := loadManifest(path)
		if err == nil {
			t.Fatal("expected error for invalid json")
		}
	})

	t.Run("empty cases", func(t *testing.T) {
		path := filepath.Join(dir, "empty.json")
		if err := os.WriteFile(path, []byte(`{"cases": []}`), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := loadManifest(path)
		if err == nil {
			t.Fatal("expected error for empty cases")
		}
	})
}

func TestUDLang(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "fi", want: "FI"},
		{in: "et", want: "ET"},
		{in: "FI", wantErr: true}, // manifest values are lowercase; uppercase must not silently pass
		{in: "de", wantErr: true},
		{in: "", wantErr: true},
	}
	for _, tc := range tests {
		got, err := udLang(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("udLang(%q): expected error, got %q", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("udLang(%q): unexpected error: %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("udLang(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestHasLemmaPOS(t *testing.T) {
	words := []parsecore.WordEntry{
		{Lemma: "kissa", POS: "NOUN"},
		{Lemma: "juoda", POS: "VERB"},
	}
	tests := []struct {
		lemma, pos string
		want       bool
	}{
		{"kissa", "NOUN", true},
		{"juoda", "VERB", true},
		{"kissa", "VERB", false}, // right lemma, wrong POS must not match
		{"koira", "NOUN", false}, // absent lemma
	}
	for _, tc := range tests {
		if got := hasLemmaPOS(words, tc.lemma, tc.pos); got != tc.want {
			t.Errorf("hasLemmaPOS(words, %q, %q) = %v, want %v", tc.lemma, tc.pos, got, tc.want)
		}
	}
}

// TestRunParserCase_MissingFixture exercises the fixture-not-found path
// without needing a real database handle, since runParserCase returns before
// touching db when the fixture read fails.
func TestRunParserCase_MissingFixture(t *testing.T) {
	c := rcCase{
		ID:         "x",
		Language:   "fi",
		Automation: "parser",
		Fixture:    "does-not-exist.txt",
	}
	ok, reason := runParserCase(nil, t.TempDir(), c)
	if ok {
		t.Fatal("expected failure for missing fixture")
	}
	if reason == "" {
		t.Fatal("expected a non-empty failure reason")
	}
}

func TestRunParserCase_UnsupportedLanguage(t *testing.T) {
	dir := t.TempDir()
	fixture := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(fixture, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := rcCase{
		ID:         "x",
		Language:   "de",
		Automation: "parser",
		Fixture:    "f.txt",
	}
	ok, reason := runParserCase(nil, dir, c)
	if ok {
		t.Fatal("expected failure for unsupported language")
	}
	if reason == "" {
		t.Fatal("expected a non-empty failure reason")
	}
}

func TestRunParserCase_NoFixturePath(t *testing.T) {
	c := rcCase{ID: "x", Language: "fi", Automation: "parser"}
	ok, reason := runParserCase(nil, t.TempDir(), c)
	if ok {
		t.Fatal("expected failure when fixture path is empty")
	}
	if reason == "" {
		t.Fatal("expected a non-empty failure reason")
	}
}
