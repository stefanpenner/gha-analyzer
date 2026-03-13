package githubapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCachedTransport(t *testing.T) {
	cacheDir, err := os.MkdirTemp("", "gha-cache-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(cacheDir)

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello world"))
	}))
	defer server.Close()

	transport := NewCachedTransport(http.DefaultTransport, cacheDir)
	client := &http.Client{Transport: transport}

	// First call - should hit the server
	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	resp, err := client.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, 1, callCount)

	// Second call - should hit the cache
	resp2, err := client.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp2.StatusCode)
	assert.Equal(t, 1, callCount) // callCount should still be 1

	// Verify file exists
	files, _ := os.ReadDir(cacheDir)
	assert.Equal(t, 1, len(files))
}

func TestCachedTransportTTLExpiration(t *testing.T) {
	cacheDir, err := os.MkdirTemp("", "gha-cache-ttl-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(cacheDir)

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello world"))
	}))
	defer server.Close()

	transport := NewCachedTransport(http.DefaultTransport, cacheDir)
	client := &http.Client{Transport: transport}

	// First call - should hit the server
	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	resp, err := client.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, 1, callCount)

	// Verify cache file exists
	files, _ := os.ReadDir(cacheDir)
	assert.Equal(t, 1, len(files))
	cachedFileName := files[0].Name()

	// Backdate the cache file's modification time to be older than cacheTTL
	cachePath := cacheDir + "/" + cachedFileName
	staleTime := time.Now().Add(-(cacheTTL + time.Minute))
	err = os.Chtimes(cachePath, staleTime, staleTime)
	assert.NoError(t, err)

	// Second call - cache should be expired, should hit the server again
	resp2, err := client.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp2.StatusCode)
	assert.Equal(t, 2, callCount)

	// Verify stale file was removed and a new cache file was created
	files2, _ := os.ReadDir(cacheDir)
	assert.Equal(t, 1, len(files2))
	assert.Equal(t, cachedFileName, files2[0].Name())

	// Verify the new cache file has a recent modification time
	info, err := os.Stat(cachePath)
	assert.NoError(t, err)
	assert.True(t, time.Since(info.ModTime()) < time.Minute, "new cache file should have a recent modification time")
}
