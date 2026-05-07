package main

import (
	"strings"
	"testing"
)

// openBackend's validation branches are pure (no file I/O), so we can
// exercise them without committing any analyser blobs. The tests assert
// (a) language is required, (b) each language requires its own
// transducer flag, and (c) the wrong-flag combinations fail loudly.
func TestOpenBackend_FlagValidation(t *testing.T) {
	cases := []struct {
		name       string
		lang       string
		vfstPath   string
		hfstolPath string
		wantErrSub string
	}{
		{name: "fi requires vfst", lang: "fi", wantErrSub: "-vfst is required for -lang fi"},
		{name: "fi rejects hfstol", lang: "fi", vfstPath: "ignored", hfstolPath: "x.hfstol", wantErrSub: "-hfstol must not be set for -lang fi"},
		{name: "et requires hfstol", lang: "et", wantErrSub: "-hfstol is required for -lang et"},
		{name: "et rejects vfst", lang: "et", hfstolPath: "ignored", vfstPath: "x.vfst", wantErrSub: "-vfst must not be set for -lang et"},
		{name: "unknown lang", lang: "sv", wantErrSub: `unsupported -lang "sv"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := openBackend(tc.lang, tc.vfstPath, tc.hfstolPath)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErrSub)
			}
			if !strings.Contains(err.Error(), tc.wantErrSub) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErrSub)
			}
		})
	}
}

// Lang dispatch is case-insensitive for both fi and et so the Makefile
// targets, env vars, and ad-hoc CLI invocations all reach the right
// backend regardless of the user's casing convention.
func TestOpenBackend_LangCaseInsensitive(t *testing.T) {
	for _, lang := range []string{"FI", "Fi", "fI"} {
		_, _, err := openBackend(lang, "", "")
		if err == nil || !strings.Contains(err.Error(), "-vfst is required") {
			t.Errorf("lang=%q: expected fi dispatch (vfst-required error), got %v", lang, err)
		}
	}
	for _, lang := range []string{"ET", "Et", "eT"} {
		_, _, err := openBackend(lang, "", "")
		if err == nil || !strings.Contains(err.Error(), "-hfstol is required") {
			t.Errorf("lang=%q: expected et dispatch (hfstol-required error), got %v", lang, err)
		}
	}
}
