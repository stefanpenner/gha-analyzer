package enrichment

// MessagingDomain returns the OTel messaging semantic convention enricher.
// Covers: messaging.system, messaging.destination.*, messaging.operation
// Semconv version: 1.20+ (experimental)
func MessagingDomain() *DomainEnricher {
	return &DomainEnricher{
		Domain:   "messaging",
		Version:  "1.20.0",
		Priority: 50,
		DetectKeys: []string{
			"messaging.system",
		},
		EnrichFunc: enrichMessaging,
	}
}

func enrichMessaging(name string, attrs map[string]string, isZeroDuration bool) SpanHints {
	msgSystem := attrs["messaging.system"]
	if msgSystem == "" {
		return SpanHints{}
	}

	h := SpanHints{
		Category: "messaging",
		Icon:     "✉ ",
		BarChar:  "█",
		Color:    "blue",
		Detail:   msgSystem,
	}

	if dest := attrs["messaging.destination.name"]; dest != "" {
		h.Detail += " " + dest
	}
	if op := attrs["messaging.operation"]; op != "" {
		h.Detail += " (" + op + ")"
	}

	if svc := attrs["service.name"]; svc != "" {
		h.ServiceName = svc
	}

	return h
}
