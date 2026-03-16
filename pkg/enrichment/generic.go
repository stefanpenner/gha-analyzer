package enrichment

import "strconv"

// GenericEnricher handles any OTel span that wasn't recognized by a more
// specific enricher. It recognizes OTel semantic conventions for HTTP, database,
// RPC, and messaging spans, and provides sensible defaults for everything else.
type GenericEnricher struct{}

// Enrich produces SpanHints for any span using OTel-standard attributes.
func (e *GenericEnricher) Enrich(name string, attrs map[string]string, isZeroDuration bool) SpanHints {
	h := SpanHints{
		Category: "operation",
		Icon:     "● ",
		BarChar:  "█",
		Color:    "blue",
	}

	// Zero-duration spans are markers
	if isZeroDuration {
		h.IsMarker = true
		h.Category = "marker"
		h.SortPriority = -1
		h.Icon = "▲ "
		h.BarChar = "▲"
	}

	// OTel status code → Outcome
	statusCode := attrs["otel.status_code"]
	switch statusCode {
	case "OK":
		h.Outcome = "success"
		h.Color = "green"
	case "ERROR":
		h.Outcome = "failure"
		h.Color = "red"
	}

	// Artifact trace spans get grouped under their parent workflow
	if artifactName, ok := attrs["github.artifact_name"]; ok && artifactName != "" {
		h.GroupKey = "artifact"
	}

	// Recognize OTel semantic conventions for common span types.
	// These set category/icon but do NOT set IsRoot/IsLeaf — tree position
	// is determined structurally, not by semconv.
	if !h.IsMarker {
		switch {
		case attrs["http.request.method"] != "" || attrs["http.method"] != "":
			h.Category = "http"
			h.Icon = "⇄ "
			// HTTP error status codes
			if code, err := strconv.Atoi(attrs["http.response.status_code"]); err == nil && code >= 400 {
				h.Outcome = "failure"
				h.Color = "red"
			}
		case attrs["db.system"] != "":
			h.Category = "database"
			h.Icon = "⛁ "
		case attrs["rpc.system"] != "":
			h.Category = "rpc"
			h.Icon = "⇌ "
		case attrs["messaging.system"] != "":
			h.Category = "messaging"
			h.Icon = "✉ "
		}
	}

	// Use span kind for icon variation (only if not already set by semconv)
	if h.Icon == "● " {
		spanKind := attrs["otel.span_kind"]
		switch spanKind {
		case "SERVER":
			h.Icon = "⇣ "
		case "CLIENT":
			h.Icon = "⇢ "
		case "PRODUCER":
			h.Icon = "⇡ "
		case "CONSUMER":
			h.Icon = "⇠ "
		}
	}

	return h
}
