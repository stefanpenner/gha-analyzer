// Package traceapi fetches traces from OTLP-compatible backends
// (Tempo, Jaeger v2) via their HTTP APIs and returns ReadOnlySpans.
//
// Supported backends:
//   - Grafana Tempo: GET /api/traces/{traceID}
//   - Jaeger v2:     GET /api/traces/{traceID}
//
// Both return OTLP protobuf-JSON (ExportTraceServiceRequest) when
// the Accept header requests it.
package traceapi

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/stefanpenner/gha-analyzer/pkg/ingest/otlpfile"
)

// Client fetches traces from an OTLP-compatible HTTP backend.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// New creates a Client for the given backend base URL.
// The baseURL should not include a trailing slash or path
// (e.g. "http://localhost:3200" for Tempo).
func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// FetchTrace retrieves a trace by its ID and returns parsed ReadOnlySpans.
func (c *Client) FetchTrace(traceID string) ([]sdktrace.ReadOnlySpan, error) {
	url := fmt.Sprintf("%s/api/traces/%s", c.baseURL, traceID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	// Request OTLP JSON format (works with both Tempo and Jaeger v2)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch trace %s: %w", traceID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fetch trace %s: HTTP %d: %s", traceID, resp.StatusCode, string(body))
	}

	spans, err := otlpfile.ParseProto(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("parse trace %s: %w", traceID, err)
	}
	return spans, nil
}
