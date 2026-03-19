package enrichment

// DatabaseDomain returns the OTel database semantic convention enricher.
// Covers: db.system, db.statement, db.operation, db.name
// Semconv version: 1.20+ (experimental → stable)
func DatabaseDomain() *DomainEnricher {
	return &DomainEnricher{
		Domain:   "db",
		Version:  "1.20.0",
		Priority: 50,
		DetectKeys: []string{
			"db.system",
		},
		EnrichFunc: enrichDatabase,
	}
}

func enrichDatabase(name string, attrs map[string]string, isZeroDuration bool) SpanHints {
	dbSystem := attrs["db.system"]
	if dbSystem == "" {
		return SpanHints{}
	}

	h := SpanHints{
		Category: "database",
		Icon:     "⛁ ",
		BarChar:  "█",
		Color:    "blue",
		Detail:   dbSystem,
	}

	if stmt := attrs["db.statement"]; stmt != "" {
		if len(stmt) > 80 {
			stmt = stmt[:77] + "..."
		}
		h.Detail = dbSystem + ": " + stmt
	} else if op := attrs["db.operation"]; op != "" {
		h.Detail = dbSystem + ": " + op
		if table := attrs["db.sql.table"]; table != "" {
			h.Detail += " " + table
		}
	}

	if dbName := attrs["db.name"]; dbName != "" {
		h.Detail += " (" + dbName + ")"
	}

	if svc := attrs["service.name"]; svc != "" {
		h.ServiceName = svc
	}

	return h
}
