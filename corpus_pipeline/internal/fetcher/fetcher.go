// Package fetcher is a tiny HTTP downloader for the corpus pipeline.
// Inlined ~30 LoC instead of lifting from main repo's cmd/fetchfrequency
// to keep the cleanliness invariant (no main-repo modifications).
package fetcher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	UserAgent = "finnestdb/corpus-pipeline (+https://github.com/sagarinbabel/finnestdb; local-only research)"
	// MaxRetries is the number of attempts per Download call. Transient
	// network errors retry with exponential backoff; permanent errors
	// (4xx, malformed URL) bail immediately.
	MaxRetries = 3
)

// httpClient is shared across all Download calls. It has NO total Timeout
// because multi-GB sources (e.g. opus-dochplt at 5 GB) trivially blow past
// any single-digit-minute budget. Instead, the underlying Transport sets
// per-operation deadlines (dial, TLS handshake, response-headers) and we
// rely on TCP keepalives to detect dead connections during long body reads.
var httpClient = &http.Client{
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   30 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
		ExpectContinueTimeout: 5 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		ForceAttemptHTTP2:     true,
	},
	// No total Timeout - multi-GB downloads regularly exceed minutes.
}

// Download writes urlStr to outPath atomically. Idempotent: if outPath
// exists and its sha256 matches the .sha256 sidecar, we skip the
// download. Returns the bytes-on-disk count.
//
// Retries transient errors up to MaxRetries with exponential backoff.
// On non-2xx HTTP status the call fails immediately (no retry).
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

	var lastErr error
	for attempt := 1; attempt <= MaxRetries; attempt++ {
		n, err := downloadOnce(urlStr, outPath, sidecar)
		if err == nil {
			return n, nil
		}
		lastErr = err
		// Don't retry on permanent errors (bad URL, 4xx).
		if isPermanent(err) {
			return 0, err
		}
		if attempt < MaxRetries {
			backoff := time.Duration(attempt*attempt) * 5 * time.Second
			fmt.Fprintf(os.Stderr, "[fetcher] %s: attempt %d/%d failed (%v), retrying in %s\n",
				filepath.Base(outPath), attempt, MaxRetries, err, backoff)
			time.Sleep(backoff)
		}
	}
	return 0, fmt.Errorf("download %s: gave up after %d attempts: %w", urlStr, MaxRetries, lastErr)
}

func downloadOnce(urlStr, outPath, sidecar string) (int64, error) {
	tmp := outPath + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return 0, err
	}
	cleanedUp := false
	defer func() {
		out.Close()
		if !cleanedUp {
			os.Remove(tmp)
		}
	}()

	req, err := http.NewRequestWithContext(context.Background(), "GET", urlStr, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("GET %s: %w", urlStr, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return 0, &httpStatusError{Status: resp.Status, Code: resp.StatusCode, URL: urlStr}
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
	cleanedUp = true
	if err := os.WriteFile(sidecar, []byte(hex.EncodeToString(h.Sum(nil))+"\n"), 0o644); err != nil {
		return 0, err
	}
	return n, nil
}

// httpStatusError signals a non-2xx HTTP status. Treated as permanent.
type httpStatusError struct {
	Status string
	Code   int
	URL    string
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("GET %s: status %s", e.URL, e.Status)
}

// isPermanent returns true for errors that won't be fixed by retrying:
// HTTP 4xx, malformed URLs, etc. Network-level errors (timeout, EOF,
// reset) are considered transient and retried.
func isPermanent(err error) bool {
	if hse, ok := err.(*httpStatusError); ok {
		return hse.Code >= 400 && hse.Code < 500
	}
	return false
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
