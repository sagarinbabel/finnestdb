package main

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeGoldFile(t *testing.T, dataset Dataset) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gold.json")
	b, err := json.Marshal(dataset)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func sampleDataset() Dataset {
	return Dataset{
		Name:     "fi-ambiguity",
		Version:  "v1",
		Language: "FI",
		Slice:    "ambiguity",
		Cases: []Case{
			{
				ID:             "fi-amb-kuusi-1",
				Text:           "Pihalla kasvaa suuri kuusi.",
				AmbiguityClass: "kuusi",
				Tokens: []Token{
					{
						Surface:    "kuusi",
						Occurrence: 1,
						Target:     true,
						Lemma:      "kuusi",
						POS:        "NOUN",
						ExpectedCandidates: []Candidate{
							{Lemma: "kuusi", POS: "NOUN", GlossHint: "spruce"},
							{Lemma: "kuusi", POS: "NUM", GlossHint: "six"},
						},
					},
				},
			},
		},
	}
}

func TestLoadDataset_ValidFile(t *testing.T) {
	path := writeGoldFile(t, sampleDataset())
	got, err := LoadDataset(path)
	if err != nil {
		t.Fatalf("LoadDataset: %v", err)
	}
	if got.Name != "fi-ambiguity" || got.Language != "FI" || len(got.Cases) != 1 {
		t.Fatalf("dataset=%+v", got)
	}
}

func TestLoadDataset_ReadsGzip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gold.json.gz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	zw := gzip.NewWriter(f)
	b, err := json.Marshal(sampleDataset())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := zw.Write(b); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	got, err := LoadDataset(path)
	if err != nil {
		t.Fatalf("LoadDataset: %v", err)
	}
	if len(got.Cases) != 1 {
		t.Fatalf("dataset=%+v", got)
	}
}

func TestLoadDataset_RejectsWrongSlice(t *testing.T) {
	d := sampleDataset()
	d.Slice = "" // not the plain gold shape's business to be loaded here
	path := writeGoldFile(t, d)
	if _, err := LoadDataset(path); err == nil {
		t.Fatal("expected error for missing/wrong slice, got nil")
	}
}

func TestLoadDataset_RejectsMissingAmbiguityClass(t *testing.T) {
	d := sampleDataset()
	d.Cases[0].AmbiguityClass = ""
	path := writeGoldFile(t, d)
	if _, err := LoadDataset(path); err == nil {
		t.Fatal("expected error for missing ambiguity_class, got nil")
	}
}

func TestLoadDataset_RejectsNoTargetToken(t *testing.T) {
	d := sampleDataset()
	d.Cases[0].Tokens[0].Target = false
	path := writeGoldFile(t, d)
	if _, err := LoadDataset(path); err == nil {
		t.Fatal("expected error for no target token, got nil")
	}
}

func TestLoadDataset_RejectsMultipleTargetTokens(t *testing.T) {
	d := sampleDataset()
	d.Cases[0].Tokens = append(d.Cases[0].Tokens, d.Cases[0].Tokens[0])
	path := writeGoldFile(t, d)
	if _, err := LoadDataset(path); err == nil {
		t.Fatal("expected error for multiple target tokens, got nil")
	}
}

func TestLoadDataset_RejectsEmptyExpectedCandidates(t *testing.T) {
	d := sampleDataset()
	d.Cases[0].Tokens[0].ExpectedCandidates = nil
	path := writeGoldFile(t, d)
	if _, err := LoadDataset(path); err == nil {
		t.Fatal("expected error for empty expected_candidates, got nil")
	}
}

func TestLoadDataset_RejectsUnsupportedLanguage(t *testing.T) {
	d := sampleDataset()
	d.Language = "SV"
	path := writeGoldFile(t, d)
	if _, err := LoadDataset(path); err == nil {
		t.Fatal("expected error for unsupported language, got nil")
	}
}
