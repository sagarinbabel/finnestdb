package main

import "testing"

func TestDatasetSlug_DistinguishesSameNameVersions(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		// The case that motivated the fix: both files carry
		// dataset.name == "fi-manual" but live in distinct files.
		{"testdata/parser-eval/fi/gold/fi-manual-v1.json", "fi-manual-v1"},
		{"testdata/parser-eval/fi/gold/fi-manual-v2.json", "fi-manual-v2"},
		// Other gold files retain their unique slugs.
		{"testdata/parser-eval/fi/gold/fi-core-v1.json", "fi-core-v1"},
		{"testdata/parser-eval/fi/gold/ud-fi-tdt-test-v1.json.gz", "ud-fi-tdt-test-v1"},
		// Bare and absolute paths.
		{"fi-manual-v2.json", "fi-manual-v2"},
		{"/abs/dir/fi-manual-v2.JSON", "fi-manual-v2"},
		{"/abs/dir/fi-manual-v2.JSON.GZ", "fi-manual-v2"},
		{"/abs/dir/no-extension", "no-extension"},
	}
	for _, c := range cases {
		got := datasetSlug(c.path)
		if got != c.want {
			t.Errorf("datasetSlug(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}
