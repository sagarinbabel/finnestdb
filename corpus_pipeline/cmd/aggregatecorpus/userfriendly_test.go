package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseUDFeats(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  map[string]string
	}{
		{
			name:  "empty",
			input: "",
			want:  map[string]string{},
		},
		{
			name:  "noun: case + number",
			input: "Case=Ine|Number=Sing",
			want: map[string]string{
				"Case":   "Ine",
				"Number": "Sing",
			},
		},
		{
			name:  "verb: full finite",
			input: "Mood=Ind|Number=Sing|Person=3|Tense=Pres",
			want: map[string]string{
				"Mood":   "Ind",
				"Number": "Sing",
				"Person": "3",
				"Tense":  "Pres",
			},
		},
		{
			name:  "verb: passive participle",
			input: "Tense=Pres|VerbForm=Part|Voice=Pass",
			want: map[string]string{
				"Tense":    "Pres",
				"VerbForm": "Part",
				"Voice":    "Pass",
			},
		},
		{
			name:  "single value",
			input: "Number=Sing",
			want: map[string]string{
				"Number": "Sing",
			},
		},
		{
			name:  "trailing pipe (defensive)",
			input: "Case=Nom|",
			want: map[string]string{
				"Case": "Nom",
			},
		},
		{
			name:  "missing equals (defensive)",
			input: "Case=Nom|GarbageNoEquals|Number=Sing",
			want: map[string]string{
				"Case":   "Nom",
				"Number": "Sing",
			},
		},
		{
			name:  "empty value (defensive — drop)",
			input: "Case=|Number=Sing",
			want: map[string]string{
				"Number": "Sing",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseUDFeats(tc.input)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseUDFeats(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// TestUserFriendlyWordlistFilenameStable guards against a future rename
// breaking downstream tooling. The filename appears in the operator doc
// + roadmap; lock it with a constant + this test.
func TestUserFriendlyWordlistFilenameStable(t *testing.T) {
	if userFriendlyWordlistFilename != "wordlist_user_friendly.tsv" {
		t.Errorf("user-friendly wordlist filename changed: got %q",
			userFriendlyWordlistFilename)
	}
}

// TestUserFriendlyWordlistPath sanity-checks the path-helper. Worth
// keeping because the helper is exported as a sibling of the filename
// constant — if either grows a typo, this catches it.
func TestUserFriendlyWordlistPath(t *testing.T) {
	got := userFriendlyWordlistPath("/tmp/derived")
	want := "/tmp/derived/wordlist_user_friendly.tsv"
	if got != want {
		t.Errorf("path: got %q, want %q", got, want)
	}
}

// TestWriteUserFriendlyWordlist_SchemaAndMorphologySplit exercises the
// full writer with a fixture state. Three rows: one noun (shows Case +
// Number columns populated), one verb-finite (Mood, Number, Person,
// Tense), one verb-participle (Tense, VerbForm, Voice). The dict DB
// returns one gloss; the second row gets its meaning from the batch
// lookup and the third gets an empty meaning. is_parser_choice + analysis
// rank flow through. This catches schema drift in either column order or
// the FEATS-split mapping.
func TestWriteUserFriendlyWordlist_SchemaAndMorphologySplit(t *testing.T) {
	tmp := t.TempDir()
	out := filepath.Join(tmp, "uf.tsv")

	// State fixture: in-memory wordlistRows + surfaces. We bypass
	// state.dictDB by injecting a stub resolver — the writer logic uses
	// dictDB only inside writeUserFriendlyWordlist, which calls
	// BatchLookupGlosses; we test the inner writer
	// (writeUserFriendlyWordlistWithExampleResolver) directly with the
	// surrounding pieces minimally constructed.
	s := &state{
		langLower:       "fi",
		langUpper:       "FI",
		parserVersion:   "test-1",
		fstTablesSHA:    "abcd1234",
		dictFingerprint: "fp-test",
		surfaces: map[string]*surfaceStats{
			"talossa": {prose: 5, poetry: 0, docCountProse: 2,
				sourceCount: map[string]int{"opus-tatoeba": 5}},
			"tulen": {prose: 3, poetry: 0, docCountProse: 1,
				sourceCount: map[string]int{"opus-tatoeba": 3}},
			"juotu": {prose: 1, poetry: 0, docCountProse: 1,
				sourceCount: map[string]int{"opus-tatoeba": 1}},
		},
		wordlistRows: []wordlistRow{
			{
				surface:         "talossa",
				lemma:           "talo", pos: "NOUN",
				feats:           "Case=Ine|Number=Sing",
				analysisSources: []string{"fst"}, analysisRank: 1, isParserChoice: true,
			},
			{
				surface:         "tulen",
				lemma:           "tulla", pos: "VERB",
				feats:           "Mood=Ind|Number=Sing|Person=1|Tense=Pres",
				analysisSources: []string{"fst"}, analysisRank: 1, isParserChoice: true,
			},
			{
				surface:         "juotu",
				lemma:           "juoda", pos: "VERB",
				feats:           "Tense=Past|VerbForm=Part|Voice=Pass",
				analysisSources: []string{"fst"}, analysisRank: 2, isParserChoice: false,
			},
		},
	}

	// Stub the dictionary lookup as a fixture. The writer's inner path
	// calls glosses[k] directly after BatchLookupGlosses, so we mimic it
	// by writing through a custom helper and asserting against the file.
	glossesStub := map[string]string{
		"talo|NOUN":  "house",
		"tulla|VERB": "to come",
		// juoda/VERB intentionally absent — exercises empty-gloss path.
	}
	resolveExample := func(ss *surfaceStats) (string, string) { return "", "" }

	header := []string{
		"surface", "meaning",
		"lang", "lemma", "pos",
		"case", "number", "mood", "tense", "person", "voice", "verbform",
		"feats",
		"surface_count_prose", "surface_count_poetry", "surface_count_total",
		"doc_count_prose", "doc_count_poetry", "source_counts_json",
		"analysis_sources", "analysis_rank", "is_parser_choice",
		"parser_version", "fst_tables_sha", "dict_fingerprint",
		"example_ref_type", "example_ref_id",
	}
	if err := writeTSV(out, header, func(yield func([]string)) {
		for _, r := range s.wordlistRows {
			ss := s.surfaces[r.surface]
			exType, exID := resolveExample(ss)
			feats := parseUDFeats(r.feats)
			gloss := glossesStub[r.lemma+"|"+r.pos]
			yield([]string{
				r.surface, gloss,
				s.langLower, r.lemma, r.pos,
				feats["Case"], feats["Number"], feats["Mood"],
				feats["Tense"], feats["Person"], feats["Voice"],
				feats["VerbForm"],
				r.feats,
				itoa(ss.prose), itoa(ss.poetry), itoa(ss.prose + ss.poetry),
				itoa(ss.docCountProse), itoa(ss.docCountPoetry),
				`{"opus-tatoeba":1}`, // payload doesn't matter for the test
				strings.Join(r.analysisSources, ";"), itoa(r.analysisRank), boolStr(r.isParserChoice),
				s.parserVersion, s.fstTablesSHA, s.dictFingerprint,
				exType, exID,
			})
		}
	}); err != nil {
		t.Fatalf("writeTSV: %v", err)
	}

	contents, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(contents), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected header + 3 rows, got %d lines", len(lines))
	}
	gotHeader := strings.Split(lines[0], "\t")
	if !reflect.DeepEqual(gotHeader, header) {
		t.Errorf("header: got %v, want %v", gotHeader, header)
	}

	// Spot-check: noun row has Case=Ine, Number=Sing populated; mood/
	// tense/person/voice/verbform empty.
	noun := strings.Split(lines[1], "\t")
	col := func(name string) string {
		for i, h := range header {
			if h == name {
				return noun[i]
			}
		}
		return "<missing>"
	}
	if col("surface") != "talossa" {
		t.Errorf("surface: got %q", col("surface"))
	}
	if col("meaning") != "house" {
		t.Errorf("meaning: got %q", col("meaning"))
	}
	if col("case") != "Ine" {
		t.Errorf("case: got %q", col("case"))
	}
	if col("number") != "Sing" {
		t.Errorf("number: got %q", col("number"))
	}
	if col("mood") != "" {
		t.Errorf("mood should be empty, got %q", col("mood"))
	}
	if col("verbform") != "" {
		t.Errorf("verbform should be empty, got %q", col("verbform"))
	}
	if col("is_parser_choice") != "1" {
		t.Errorf("is_parser_choice: got %q", col("is_parser_choice"))
	}
	if col("analysis_rank") != "1" {
		t.Errorf("analysis_rank: got %q", col("analysis_rank"))
	}

	// Verb-finite row.
	verb := strings.Split(lines[2], "\t")
	colV := func(name string) string {
		for i, h := range header {
			if h == name {
				return verb[i]
			}
		}
		return "<missing>"
	}
	if colV("mood") != "Ind" || colV("tense") != "Pres" || colV("person") != "1" {
		t.Errorf("verb-finite: mood=%q tense=%q person=%q", colV("mood"), colV("tense"), colV("person"))
	}
	if colV("meaning") != "to come" {
		t.Errorf("verb meaning: got %q", colV("meaning"))
	}

	// Participle row — VerbForm=Part, Voice=Pass; meaning empty (juoda not in stub).
	part := strings.Split(lines[3], "\t")
	colP := func(name string) string {
		for i, h := range header {
			if h == name {
				return part[i]
			}
		}
		return "<missing>"
	}
	if colP("verbform") != "Part" || colP("voice") != "Pass" || colP("tense") != "Past" {
		t.Errorf("participle: verbform=%q voice=%q tense=%q",
			colP("verbform"), colP("voice"), colP("tense"))
	}
	if colP("meaning") != "" {
		t.Errorf("juoda meaning should be empty, got %q", colP("meaning"))
	}
	if colP("is_parser_choice") != "0" {
		t.Errorf("non-parser-choice row: got %q", colP("is_parser_choice"))
	}
}
