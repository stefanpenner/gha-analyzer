package receiver

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// validOTLPSpanJSON is a single newline-delimited stdouttrace span
// that otlpfile.Parse can decode.
const validOTLPSpanJSON = `{"Name":"test-span","SpanContext":{"TraceID":"0af7651916cd43dd8448eb211c80319c","SpanID":"b7ad6b7169203331","TraceFlags":"01"},"Parent":{"TraceID":"","SpanID":""},"SpanKind":1,"StartTime":"2024-01-15T10:00:00Z","EndTime":"2024-01-15T10:05:00Z","Attributes":[{"Key":"type","Value":{"Type":"STRING","Value":"workflow"}}],"Events":null,"Links":null,"Status":{"Code":"OK","Description":""}}`

// twoSpansJSON contains two newline-delimited spans.
const twoSpansJSON = `{"Name":"span-a","SpanContext":{"TraceID":"0af7651916cd43dd8448eb211c80319c","SpanID":"b7ad6b7169203331","TraceFlags":"01"},"Parent":{"TraceID":"","SpanID":""},"SpanKind":1,"StartTime":"2024-01-15T10:00:00Z","EndTime":"2024-01-15T10:01:00Z","Attributes":[],"Events":null,"Links":null,"Status":{"Code":"","Description":""}}
{"Name":"span-b","SpanContext":{"TraceID":"0af7651916cd43dd8448eb211c80319c","SpanID":"00f067aa0ba902b7","TraceFlags":"01"},"Parent":{"TraceID":"0af7651916cd43dd8448eb211c80319c","SpanID":"b7ad6b7169203331"},"SpanKind":1,"StartTime":"2024-01-15T10:01:00Z","EndTime":"2024-01-15T10:02:00Z","Attributes":[],"Events":null,"Links":null,"Status":{"Code":"","Description":""}}`

// mux returns the same HTTP handler that Start() would register,
// without actually starting a TCP listener.
func setupMux(r *Receiver) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/traces", r.handleTraces)
	mux.HandleFunc("/health", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return mux
}

func TestPostValidTracesAccumulatesSpans(t *testing.T) {
	r := New(":0")
	handler := setupMux(r)

	req := httptest.NewRequest(http.MethodPost, "/v1/traces", strings.NewReader(twoSpansJSON))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if r.SpanCount() != 2 {
		t.Fatalf("expected 2 spans, got %d", r.SpanCount())
	}

	spans := r.Spans()
	if spans[0].Name() != "span-a" {
		t.Errorf("span[0] name = %q, want %q", spans[0].Name(), "span-a")
	}
	if spans[1].Name() != "span-b" {
		t.Errorf("span[1] name = %q, want %q", spans[1].Name(), "span-b")
	}
}

func TestGetTracesReturns405(t *testing.T) {
	r := New(":0")
	handler := setupMux(r)

	req := httptest.NewRequest(http.MethodGet, "/v1/traces", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", rec.Code)
	}
}

func TestPostMalformedBodyAccumulatesNoSpans(t *testing.T) {
	r := New(":0")
	handler := setupMux(r)

	// otlpfile.Parse is lenient and doesn't error on garbage — it just returns
	// zero spans. The receiver should still respond 200 (no parse error) but
	// accumulate nothing.
	req := httptest.NewRequest(http.MethodPost, "/v1/traces", strings.NewReader("{{{not json at all"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 (parser is lenient), got %d: %s", rec.Code, rec.Body.String())
	}

	if r.SpanCount() != 0 {
		t.Errorf("expected 0 spans after malformed request, got %d", r.SpanCount())
	}
}

func TestPostCorruptGzipReturns400(t *testing.T) {
	r := New(":0")
	handler := setupMux(r)

	// Bytes starting with gzip magic (0x1f 0x8b) but otherwise corrupt.
	// This causes maybeDecompress to fail, which surfaces as a parse error (400).
	corrupt := []byte{0x1f, 0x8b, 0x00, 0x00, 0xff, 0xff}
	req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(corrupt))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for corrupt gzip, got %d: %s", rec.Code, rec.Body.String())
	}

	if r.SpanCount() != 0 {
		t.Errorf("expected 0 spans, got %d", r.SpanCount())
	}
}

func TestHealthReturns200(t *testing.T) {
	r := New(":0")
	handler := setupMux(r)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

func TestSpanCountAndSpans(t *testing.T) {
	r := New(":0")

	if r.SpanCount() != 0 {
		t.Fatalf("expected 0 spans initially, got %d", r.SpanCount())
	}
	if len(r.Spans()) != 0 {
		t.Fatalf("expected empty Spans() initially, got %d", len(r.Spans()))
	}

	handler := setupMux(r)

	// Post one span.
	req := httptest.NewRequest(http.MethodPost, "/v1/traces", strings.NewReader(validOTLPSpanJSON))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if r.SpanCount() != 1 {
		t.Fatalf("expected 1 span, got %d", r.SpanCount())
	}

	spans := r.Spans()
	if len(spans) != 1 {
		t.Fatalf("expected Spans() length 1, got %d", len(spans))
	}
	if spans[0].Name() != "test-span" {
		t.Errorf("span name = %q, want %q", spans[0].Name(), "test-span")
	}

	// Spans() returns a copy, so mutating it should not affect the receiver.
	spans[0] = nil
	if r.SpanCount() != 1 {
		t.Errorf("mutating Spans() return value should not affect receiver, but SpanCount changed to %d", r.SpanCount())
	}
}

func TestConcurrentPosts(t *testing.T) {
	r := New(":0")
	handler := setupMux(r)

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/v1/traces", strings.NewReader(validOTLPSpanJSON))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("expected status 200, got %d", rec.Code)
			}
		}()
	}

	wg.Wait()

	if r.SpanCount() != goroutines {
		t.Errorf("expected %d spans, got %d", goroutines, r.SpanCount())
	}
}
