package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Cache-busting for shipped web assets.
//
// The owner ran stale app.js after a `git pull` because static assets carried no
// version busting: a fixed `?v=` query stamped into index.html never changed
// across rebuilds, so a browser (or an intermediary that ignores no-store)
// could keep serving the old JS/CSS. The fix (USER_FLOWS / DEPLOYMENT.md
// "Asset versioning") is scheme (a): compute a content hash of app.js and
// styles.css at server start, stamp `?v=<hash>` into index.html's references
// when index.html is served, and serve index.html itself with `no-cache` so the
// browser always revalidates and picks up the current hashes. The hashed assets
// additionally carry `no-cache, must-revalidate` + an ETag, so even an
// intermediary that drops the query still revalidates instead of serving stale
// bytes. A rebuild changes the content hash, so the stamped URL changes and the
// old cached copy is never requested again.

// versionedAsset is a hashable static asset referenced from index.html.
type versionedAsset struct {
	name string // file name relative to webDir, e.g. "app.js"
	hash string // short content hash computed at server start
	etag string // strong ETag ("<hash>") for conditional requests
}

// staticHandler serves the web directory with content-hash cache-busting.
type staticHandler struct {
	webDir string
	fs     http.Handler
	assets map[string]*versionedAsset // keyed by asset name
	// indexRefRe matches an app.js/styles.css reference in index.html, with or
	// without an existing `?v=...` query, so re-serving is idempotent across
	// restarts and independent of the checked-in placeholder version.
	indexRefRe *regexp.Regexp
}

// newStaticHandler hashes the versioned assets once and returns a handler.
// Assets that can't be read (missing during local iteration) are skipped; their
// references are then served unstamped rather than failing the whole server.
func newStaticHandler(webDir string) *staticHandler {
	h := &staticHandler{
		webDir: webDir,
		fs:     http.FileServer(http.Dir(webDir)),
		assets: map[string]*versionedAsset{},
		// Capture the asset name and any existing query so we can replace the
		// whole `name(?v=...)?` token with `name?v=<hash>`.
		indexRefRe: regexp.MustCompile(`(app\.js|styles\.css)(\?v=[^"'\s]*)?`),
	}
	for _, name := range []string{"app.js", "styles.css"} {
		hash, err := hashFile(filepath.Join(webDir, name))
		if err != nil {
			// Non-fatal: serve the reference unstamped rather than 500 the app.
			continue
		}
		h.assets[name] = &versionedAsset{name: name, hash: hash, etag: `"` + hash + `"`}
	}
	return h
}

// hashFile returns a short (first 12 hex chars) sha256 of the file's contents.
func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:12], nil
}

func (h *staticHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	clean := path(r.URL.Path)

	// index.html (and the SPA root) is rewritten to carry current asset hashes
	// and must always revalidate so new hashes reach the browser immediately.
	if clean == "" || clean == "index.html" {
		h.serveIndex(w, r)
		return
	}

	// Versioned assets: no-cache + ETag so the browser revalidates and never
	// serves stale bytes even if an intermediary strips the `?v=` query.
	if asset, ok := h.assets[clean]; ok {
		w.Header().Set("Cache-Control", "no-cache, must-revalidate")
		w.Header().Set("ETag", asset.etag)
		if match := r.Header.Get("If-None-Match"); match != "" && strings.Contains(match, asset.hash) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		h.fs.ServeHTTP(w, r)
		return
	}

	// Everything else (fonts, images, other files): keep the prior policy of not
	// caching so local UI iteration never shows stale assets.
	w.Header().Set("Cache-Control", "no-store")
	h.fs.ServeHTTP(w, r)
}

// serveIndex reads index.html, stamps the current asset hashes into the app.js /
// styles.css references, and serves it with no-cache.
func (h *staticHandler) serveIndex(w http.ResponseWriter, r *http.Request) {
	raw, err := os.ReadFile(filepath.Join(h.webDir, "index.html"))
	if err != nil {
		http.Error(w, "index.html not found", http.StatusInternalServerError)
		return
	}
	stamped := h.indexRefRe.ReplaceAllFunc(raw, func(m []byte) []byte {
		// The first captured group is the asset name; look it up and re-stamp.
		name := string(h.indexRefRe.FindSubmatch(m)[1])
		asset, ok := h.assets[name]
		if !ok {
			return m // asset unreadable at start: leave the reference untouched.
		}
		return []byte(fmt.Sprintf("%s?v=%s", name, asset.hash))
	})
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(stamped)
}

// path normalizes a request path to a webDir-relative slash path with no leading
// slash. "/" and "/index.html" both normalize toward the index.
func path(p string) string {
	return strings.TrimPrefix(filepath.ToSlash(p), "/")
}
