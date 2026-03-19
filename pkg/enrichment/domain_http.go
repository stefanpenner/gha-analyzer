package enrichment

import (
	"fmt"
	"strconv"
)

// HTTPDomain returns the OTel HTTP semantic convention enricher.
// Covers: http.request.*, http.response.*, url.*, server.*, client.*
// Semconv version: 1.20+ (stable)
func HTTPDomain() *DomainEnricher {
	return &DomainEnricher{
		Domain:   "http",
		Version:  "1.20.0",
		Priority: 50,
		DetectKeys: []string{
			"http.request.method",
			"http.method", // deprecated but still detected
		},
		EnrichFunc: enrichHTTP,
	}
}

func enrichHTTP(name string, attrs map[string]string, isZeroDuration bool) SpanHints {
	method := attrs["http.request.method"]
	if method == "" {
		method = attrs["http.method"]
	}
	if method == "" {
		return SpanHints{}
	}

	h := SpanHints{
		Category: "http",
		Icon:     "⇄ ",
		BarChar:  "█",
		Color:    "blue",
	}

	// Build detail: "METHOD route → server:port [status]"
	route := attrs["http.route"]
	if route == "" {
		route = attrs["url.path"]
	}
	if method != "" && route != "" {
		h.Detail = method + " " + route
	} else if method != "" {
		h.Detail = method
	}
	if server := attrs["server.address"]; server != "" {
		if port := attrs["server.port"]; port != "" {
			h.Detail += fmt.Sprintf(" → %s:%s", server, port)
		}
	}

	// Status code
	statusStr := attrs["http.response.status_code"]
	if statusStr == "" {
		statusStr = attrs["http.status_code"]
	}
	if code, err := strconv.Atoi(statusStr); err == nil {
		if code >= 400 {
			h.Outcome = "failure"
			h.Color = "red"
		}
		if h.Detail != "" {
			h.Detail += fmt.Sprintf(" [%d]", code)
		}
	}

	// Service context
	if svc := attrs["service.name"]; svc != "" {
		h.ServiceName = svc
	}
	if env := attrs["deployment.environment"]; env != "" {
		h.Environment = env
	}

	return h
}
