// Smoke test for the replace-directive setup.
// Verifies we can import finnestdb/internal/parserffi from the local module
// and that the Rust FFI link works. Run from corpus_pipeline/:
//
//	go run ./cmd/_smoke/
//
// Expects to print sentence count + first token's surface form.
package main

import (
	"fmt"
	"os"

	"finnestdb/internal/parserffi"
)

func main() {
	result, err := parserffi.AnalyzeText("FI", "Hei maailma. Tulen kotiin.")
	if err != nil {
		fmt.Fprintf(os.Stderr, "AnalyzeText failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("sentences=%d\n", len(result.Sentences))
	if len(result.Sentences) == 0 || len(result.Sentences[0].Tokens) == 0 {
		fmt.Fprintln(os.Stderr, "no tokens returned")
		os.Exit(1)
	}
	for i, s := range result.Sentences {
		forms := make([]string, 0, len(s.Tokens))
		for _, t := range s.Tokens {
			forms = append(forms, t.Form)
		}
		fmt.Printf("sentence[%d]: %v\n", i, forms)
	}
}
