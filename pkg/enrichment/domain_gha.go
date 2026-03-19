package enrichment

// GHADomain returns the GitHub Actions domain enricher.
// Priority 10: platform-specific, takes precedence over generic OTel semconv.
func GHADomain() *DomainEnricher {
	e := &GHAEnricher{}
	return &DomainEnricher{
		Domain:     "github-actions",
		Version:    "custom",
		Priority:   10,
		DetectKeys: []string{"type"},
		EnrichFunc: e.Enrich,
	}
}
