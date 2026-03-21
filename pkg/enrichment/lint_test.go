package enrichment

import (
	"strings"
	"testing"
)

func TestLintSpans_DeprecatedHTTP(t *testing.T) {
	spans := []SpanData{
		{
			Name: "GET /api",
			Attrs: map[string]string{
				"http.method":      "GET",
				"http.status_code": "200",
				"http.url":         "http://example.com/api",
			},
			SpanKind: "CLIENT",
		},
	}

	results := LintSpans(spans)
	if len(results) == 0 {
		t.Fatal("expected lint results for deprecated HTTP attributes")
	}

	foundMethod := false
	foundStatusCode := false
	foundURL := false
	for _, r := range results {
		if strings.Contains(r.Message, "http.method") {
			foundMethod = true
		}
		if strings.Contains(r.Message, "http.status_code") {
			foundStatusCode = true
		}
		if strings.Contains(r.Message, "http.url") {
			foundURL = true
		}
	}
	if !foundMethod {
		t.Error("expected warning for deprecated http.method")
	}
	if !foundStatusCode {
		t.Error("expected warning for deprecated http.status_code")
	}
	if !foundURL {
		t.Error("expected warning for deprecated http.url")
	}
}

func TestLintSpans_CleanSpan(t *testing.T) {
	spans := []SpanData{
		{
			Name: "GET /api",
			Attrs: map[string]string{
				"http.request.method":       "GET",
				"http.response.status_code": "200",
				"server.address":            "api.example.com",
				"http.route":                "/api",
			},
			SpanKind: "SERVER",
		},
	}

	results := LintSpans(spans)
	// Should have no warnings (only info-level at most)
	for _, r := range results {
		if r.Level == "warning" || r.Level == "error" {
			t.Errorf("unexpected %s lint result: %s", r.Level, r.Message)
		}
	}
}

func TestLintSpans_MissingDBSystem(t *testing.T) {
	spans := []SpanData{
		{
			Name: "SELECT users",
			Attrs: map[string]string{
				"db.statement": "SELECT * FROM users",
			},
		},
	}

	results := LintSpans(spans)
	found := false
	for _, r := range results {
		if strings.Contains(r.Message, "db.system") {
			found = true
		}
	}
	if !found {
		t.Error("expected warning for missing db.system")
	}
}

func TestFormatLintResults_Empty(t *testing.T) {
	out := FormatLintResults(nil)
	if !strings.Contains(out, "No semconv issues") {
		t.Errorf("expected 'No semconv issues' message, got %q", out)
	}
}

func TestFormatLintResults_Dedup(t *testing.T) {
	results := []LintResult{
		{SpanName: "span1", Level: "warning", Message: "same message", Suggestion: "fix it"},
		{SpanName: "span2", Level: "warning", Message: "same message", Suggestion: "fix it"},
		{SpanName: "span3", Level: "warning", Message: "same message", Suggestion: "fix it"},
	}

	out := FormatLintResults(results)
	if !strings.Contains(out, "×3") {
		t.Errorf("expected deduplicated count (×3), got:\n%s", out)
	}
}
