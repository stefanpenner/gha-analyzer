package enrichment

// GenericEnricher handles any OTel span that wasn't recognized by a more
// specific enricher. It provides sensible defaults for non-GHA traces.
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

	// Use span kind for icon variation
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

	return h
}
