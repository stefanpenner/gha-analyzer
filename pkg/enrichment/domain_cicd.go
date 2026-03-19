package enrichment

// CICDDomain returns the OTel CI/CD semantic convention enricher.
// Covers: cicd.pipeline.*, vcs.*, deployment.*
// Semconv version: 1.27+
// Priority 20: OTel semconv, after platform-specific.
func CICDDomain() *DomainEnricher {
	e := &CICDEnricher{}
	return &DomainEnricher{
		Domain:   "cicd",
		Version:  "1.27.0",
		Priority: 20,
		DetectKeys: []string{
			"cicd.pipeline.name",
			"cicd.pipeline.task.name",
			"cicd.pipeline.run.id",
		},
		EnrichFunc: e.Enrich,
	}
}
