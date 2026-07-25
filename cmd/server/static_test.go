package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestStaticHandlerServesOnlyShippedAssets(t *testing.T) {
	webDir := t.TempDir()
	for name, body := range map[string]string{
		"index.html": `<link href="styles.css"><script src="app.js"></script>`,
		"app.js":     `console.log("app")`,
		"styles.css": `body { color: black; }`,
		"app.ts":     `const developmentOnly = true;`,
	} {
		if err := os.WriteFile(filepath.Join(webDir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	h := newStaticHandler(webDir)
	for _, path := range []string{"/", "/index.html", "/app.js", "/styles.css"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s status=%d want %d", path, rec.Code, http.StatusOK)
		}
	}

	for _, path := range []string{"/app.ts", "/package-lock.json", "/tests/landing-prototype.spec.ts"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s status=%d want %d", path, rec.Code, http.StatusNotFound)
		}
	}
}
