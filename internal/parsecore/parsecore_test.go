package parsecore

import (
	"slices"
	"testing"
)

func TestSupportedParsers(t *testing.T) {
	got := SupportedParsers()
	want := []string{"basic", "custom"}
	if !slices.Equal(got, want) {
		t.Fatalf("supported parsers=%v want %v", got, want)
	}
}
