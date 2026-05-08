// Package fetcher is a tiny HTTP downloader for the corpus pipeline.
// Inlined ~30 LoC instead of lifting from main repo's cmd/fetchfrequency
// to keep the cleanliness invariant (no main-repo modifications).
package fetcher

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	UserAgent = "finnestdb/corpus-pipeline (+https://github.com/sagarinbabel/finnestdb; local-only research)"
	Timeout   = 5 * time.Minute
)

// Download writes urlStr to outPath atomically. Idempotent: if outPath
// exists and its sha256 matches the .sha256 sidecar, we skip the
// download. Returns the bytes-on-disk count.
func Download(urlStr, outPath string) (int64, error) {
	sidecar := outPath + ".sha256"
	if existing, err := os.Stat(outPath); err == nil && existing.Size() > 0 {
		if matches, _ := verifySidecar(outPath, sidecar); matches {
			return existing.Size(), nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return 0, err
	}
	tmp := outPath + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return 0, err
	}
	defer func() {
		out.Close()
		os.Remove(tmp)
	}()

	client := &http.Client{Timeout: Timeout}
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("GET %s: %w", urlStr, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return 0, fmt.Errorf("GET %s: status %s", urlStr, resp.Status)
	}
	h := sha256.New()
	mw := io.MultiWriter(out, h)
	n, err := io.Copy(mw, resp.Body)
	if err != nil {
		return 0, err
	}
	if err := out.Close(); err != nil {
		return 0, err
	}
	if err := os.Rename(tmp, outPath); err != nil {
		return 0, err
	}
	if err := os.WriteFile(sidecar, []byte(hex.EncodeToString(h.Sum(nil))+"\n"), 0o644); err != nil {
		return 0, err
	}
	return n, nil
}

// verifySidecar returns true if outPath's sha256 matches the contents of
// sidecar (a one-line hex digest).
func verifySidecar(outPath, sidecar string) (bool, error) {
	want, err := os.ReadFile(sidecar)
	if err != nil {
		return false, err
	}
	wantHex := string(want)
	for len(wantHex) > 0 && (wantHex[len(wantHex)-1] == '\n' || wantHex[len(wantHex)-1] == ' ') {
		wantHex = wantHex[:len(wantHex)-1]
	}
	f, err := os.Open(outPath)
	if err != nil {
		return false, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}
	return hex.EncodeToString(h.Sum(nil)) == wantHex, nil
}

// HEAD returns the Content-Length of urlStr without downloading the body.
// Used by the pre-fetch dry-run probe.
func HEAD(urlStr string) (int64, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("HEAD", urlStr, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return 0, fmt.Errorf("HEAD %s: status %s", urlStr, resp.Status)
	}
	return resp.ContentLength, nil
}
