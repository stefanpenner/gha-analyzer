package enrichment

import "sort"

// DomainEnricher is an Enricher with metadata for registry-based composition.
type DomainEnricher struct {
	// Domain is the semconv domain name (e.g., "http", "db", "rpc", "cicd", "k8s").
	Domain string
	// Version is the semconv version this enricher targets (e.g., "1.27.0").
	Version string
	// Priority controls ordering: lower = tried first. Default 100.
	Priority int
	// DetectKeys are attribute keys that signal this domain applies.
	// If any of these keys are present, this enricher is a candidate.
	DetectKeys []string
	// Enrich is the enrichment function.
	EnrichFunc func(name string, attrs map[string]string, isZeroDuration bool) SpanHints
}

// Enrich implements the Enricher interface.
func (d *DomainEnricher) Enrich(name string, attrs map[string]string, isZeroDuration bool) SpanHints {
	return d.EnrichFunc(name, attrs, isZeroDuration)
}

// Registry holds domain enrichers and composes them into a chain.
type Registry struct {
	domains []*DomainEnricher
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// Register adds a domain enricher to the registry.
func (r *Registry) Register(d *DomainEnricher) {
	if d.Priority == 0 {
		d.Priority = 100
	}
	r.domains = append(r.domains, d)
}

// Build returns a ChainEnricher ordered by priority.
func (r *Registry) Build() *ChainEnricher {
	// Sort by priority (lower first)
	sorted := make([]*DomainEnricher, len(r.domains))
	copy(sorted, r.domains)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Priority < sorted[j].Priority
	})

	enrichers := make([]Enricher, len(sorted))
	for i, d := range sorted {
		enrichers[i] = d
	}
	return NewChainEnricher(enrichers...)
}

// Domains returns metadata about all registered domains.
func (r *Registry) Domains() []DomainInfo {
	var infos []DomainInfo
	for _, d := range r.domains {
		infos = append(infos, DomainInfo{
			Domain:     d.Domain,
			Version:    d.Version,
			Priority:   d.Priority,
			DetectKeys: d.DetectKeys,
		})
	}
	return infos
}

// DomainInfo describes a registered semconv domain.
type DomainInfo struct {
	Domain     string
	Version    string
	Priority   int
	DetectKeys []string
}

// DefaultRegistry returns a registry pre-populated with all known domain enrichers.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	// Custom / platform-specific (highest priority)
	r.Register(GHADomain())
	// OTel semconv domains
	r.Register(CICDDomain())
	r.Register(HTTPDomain())
	r.Register(DatabaseDomain())
	r.Register(RPCDomain())
	r.Register(MessagingDomain())
	r.Register(FaaSDomain())
	r.Register(K8sDomain())
	r.Register(CloudDomain())
	r.Register(ContainerDomain())
	r.Register(ProcessDomain())
	r.Register(NetworkDomain())
	// Generic fallback (lowest priority)
	r.Register(GenericDomain())
	return r
}

// RegistryEnricher returns the default registry-based enricher chain.
// This replaces DefaultEnricher() for new code.
func RegistryEnricher() *ChainEnricher {
	return DefaultRegistry().Build()
}
