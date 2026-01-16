package githubapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

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
