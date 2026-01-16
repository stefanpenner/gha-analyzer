package githubapi

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httputil"
	"os"
	"path/filepath"
)

// CachedTransport implements http.RoundTripper and caches GET requests to disk.
type CachedTransport struct {
	Base     http.RoundTripper
	CacheDir string
}

func NewCachedTransport(base http.RoundTripper, cacheDir string) *CachedTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &CachedTransport{
		Base:     base,
		CacheDir: cacheDir,
	}
}

func (t *CachedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Only cache GET requests
	if req.Method != http.MethodGet {
		return t.Base.RoundTrip(req)
	}

	cacheKey := t.getCacheKey(req)
	cachePath := filepath.Join(t.CacheDir, cacheKey)

	// Try to read from cache
	if cachedResp := t.getFromCache(cachePath, req); cachedResp != nil {
		return cachedResp, nil
	}

	// Not in cache, perform request
	resp, err := t.Base.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	// Only cache successful responses
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		t.saveToCache(cachePath, resp)
	}

	return resp, nil
}

func (t *CachedTransport) getCacheKey(req *http.Request) string {
	// Use URL and certain headers to create a unique hash
	h := sha256.New()
	h.Write([]byte(req.URL.String()))
	h.Write([]byte(req.Header.Get("Accept")))
	// We don't include Authorization header to avoid leaking tokens in hash, 
	// though the file content itself is more sensitive.
	return hex.EncodeToString(h.Sum(nil))
}

func (t *CachedTransport) getFromCache(path string, req *http.Request) *http.Response {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	// Parse the cached response
	resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(data)), req)
	if err != nil {
		return nil
	}

	return resp
}

func (t *CachedTransport) saveToCache(path string, resp *http.Response) {
	if err := os.MkdirAll(t.CacheDir, 0755); err != nil {
		return
	}

	// We need to dump the response to save it. 
	// DumpResponse reads and CLOSES the body if it's not already buffered.
	// But it actually returns the full bytes including the body.
	dump, err := httputil.DumpResponse(resp, true)
	if err != nil {
		return
	}

	_ = os.WriteFile(path, dump, 0644)

	// Since DumpResponse consumed the body, we MUST restore it so the 
	// caller of RoundTrip can still read it.
	// We parse it back to a temporary response to get the body out.
	restored, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(dump)), resp.Request)
	if err == nil {
		resp.Body = restored.Body
	}
}

// ClearCache removes all cached files.
func (t *CachedTransport) ClearCache() error {
	return os.RemoveAll(t.CacheDir)
}

func (t *CachedTransport) IsCached(req *http.Request) bool {
	cacheKey := t.getCacheKey(req)
	cachePath := filepath.Join(t.CacheDir, cacheKey)
	_, err := os.Stat(cachePath)
	return err == nil
}
