package parserffi

/*
#cgo LDFLAGS: -L${SRCDIR}/../../parser/target/release -lparser -ldl -lm
#include <stdlib.h>
#include <string.h>
*/
import "C"
import (
	"encoding/json"
	"unsafe"
)

type Token struct {
	Form         string          `json:"form"`
	Lemma        string          `json:"lemma"`
	POS          string          `json:"pos"`
	Feats        json.RawMessage `json:"feats"`
	GrammarLabel string          `json:"grammar_label"`
	MWEID        *int            `json:"mwe_id"`
}

type Sentence struct {
	Tokens []Token `json:"tokens"`
}

type AnalysisResult struct {
	Sentences []Sentence `json:"sentences"`
}

// AnalyzeText calls the Rust parser library to analyze text
func AnalyzeText(lang, text string) (*AnalysisResult, error) {
	langC := C.CString(lang)
	defer C.free(unsafe.Pointer(langC))

	textC := C.CString(text)
	defer C.free(unsafe.Pointer(textC))

	resultC := C.analyze_text(langC, textC)
	defer C.free_string(resultC)

	if resultC == nil {
		return nil, nil
	}

	resultStr := C.GoString(resultC)
	if resultStr == "" {
		return nil, nil
	}

	var result AnalysisResult
	if err := json.Unmarshal([]byte(resultStr), &result); err != nil {
		return nil, err
	}

	return &result, nil
}

