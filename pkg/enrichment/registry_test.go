package enrichment

import (
	"testing"
)

func TestRegistryPriorityOrder(t *testing.T) {
	r := NewRegistry()
	r.Register(&DomainEnricher{
		Domain:   "low",
		Priority: 200,
		EnrichFunc: func(name string, attrs map[string]string, isZeroDuration bool) SpanHints {
			return SpanHints{Category: "low"}
		},
	})
	r.Register(&DomainEnricher{
		Domain:   "high",
		Priority: 10,
		EnrichFunc: func(name string, attrs map[string]string, isZeroDuration bool) SpanHints {
			return SpanHints{Category: "high"}
		},
	})

	chain := r.Build()
	h := chain.Enrich("test", map[string]string{}, false)
	if h.Category != "high" {
		t.Errorf("expected high-priority enricher to win, got %q", h.Category)
	}
}

func TestRegistryDefaultHasAllDomains(t *testing.T) {
	r := DefaultRegistry()
	domains := r.Domains()

	expected := map[string]bool{
		"github-actions": false,
		"cicd":           false,
		"http":           false,
		"db":             false,
		"rpc":            false,
		"messaging":      false,
		"faas":           false,
		"k8s":            false,
		"cloud":          false,
		"container":      false,
		"process":        false,
		"network":        false,
		"generic":        false,
	}

	for _, d := range domains {
		if _, ok := expected[d.Domain]; ok {
			expected[d.Domain] = true
		}
	}

	for domain, found := range expected {
		if !found {
			t.Errorf("expected domain %q not found in registry", domain)
		}
	}
}

func TestRegistryEnricherHTTP(t *testing.T) {
	chain := RegistryEnricher()
	h := chain.Enrich("GET /api", map[string]string{
		"http.request.method":       "GET",
		"http.route":                "/api/users",
		"http.response.status_code": "200",
	}, false)

	if h.Category != "http" {
		t.Errorf("expected category 'http', got %q", h.Category)
	}
}

func TestRegistryEnricherDatabase(t *testing.T) {
	chain := RegistryEnricher()
	h := chain.Enrich("SELECT", map[string]string{
		"db.system":    "postgresql",
		"db.statement": "SELECT * FROM users",
	}, false)

	if h.Category != "database" {
		t.Errorf("expected category 'database', got %q", h.Category)
	}
}

func TestRegistryEnricherRPC(t *testing.T) {
	chain := RegistryEnricher()
	h := chain.Enrich("grpc", map[string]string{
		"rpc.system":  "grpc",
		"rpc.service": "UserService",
		"rpc.method":  "GetUser",
	}, false)

	if h.Category != "rpc" {
		t.Errorf("expected category 'rpc', got %q", h.Category)
	}
	if h.Detail != "grpc UserService/GetUser" {
		t.Errorf("expected detail 'grpc UserService/GetUser', got %q", h.Detail)
	}
}

func TestRegistryEnricherK8s(t *testing.T) {
	chain := RegistryEnricher()
	h := chain.Enrich("deploy", map[string]string{
		"k8s.deployment.name": "nginx",
		"k8s.namespace.name":  "production",
	}, false)

	if h.Category != "k8s" {
		t.Errorf("expected category 'k8s', got %q", h.Category)
	}
	if h.Detail != "production/deploy/nginx" {
		t.Errorf("expected detail 'production/deploy/nginx', got %q", h.Detail)
	}
}

func TestRegistryEnricherCloud(t *testing.T) {
	chain := RegistryEnricher()
	h := chain.Enrich("aws", map[string]string{
		"cloud.provider": "aws",
		"cloud.region":   "us-east-1",
	}, false)

	if h.Category != "cloud" {
		t.Errorf("expected category 'cloud', got %q", h.Category)
	}
}

func TestRegistryEnricherMessaging(t *testing.T) {
	chain := RegistryEnricher()
	h := chain.Enrich("publish", map[string]string{
		"messaging.system":           "kafka",
		"messaging.destination.name": "orders",
		"messaging.operation":        "publish",
	}, false)

	if h.Category != "messaging" {
		t.Errorf("expected category 'messaging', got %q", h.Category)
	}
}

func TestRegistryEnricherFaaS(t *testing.T) {
	chain := RegistryEnricher()
	h := chain.Enrich("invoke", map[string]string{
		"faas.trigger": "http",
		"faas.name":    "my-function",
	}, false)

	if h.Category != "faas" {
		t.Errorf("expected category 'faas', got %q", h.Category)
	}
}

func TestRegistryGHATakesPrecedence(t *testing.T) {
	chain := RegistryEnricher()
	h := chain.Enrich("CI", map[string]string{
		"type":              "workflow",
		"github.conclusion": "success",
	}, false)

	if h.Category != "workflow" {
		t.Errorf("GHA should take precedence, got category %q", h.Category)
	}
}

func TestRegistryCICDBeforeGeneric(t *testing.T) {
	chain := RegistryEnricher()
	h := chain.Enrich("build", map[string]string{
		"cicd.pipeline.name": "my-pipeline",
	}, false)

	if h.Category != "pipeline" {
		t.Errorf("CICD should match before generic, got category %q", h.Category)
	}
}

func TestRegistryGenericFallback(t *testing.T) {
	chain := RegistryEnricher()
	h := chain.Enrich("unknown-op", map[string]string{
		"otel.status_code": "OK",
	}, false)

	if h.Category != "operation" {
		t.Errorf("generic should catch unrecognized spans, got category %q", h.Category)
	}
	if h.Outcome != "success" {
		t.Errorf("expected outcome 'success' from status OK, got %q", h.Outcome)
	}
}

func TestDomainInfoMetadata(t *testing.T) {
	r := DefaultRegistry()
	domains := r.Domains()

	for _, d := range domains {
		if d.Domain == "" {
			t.Error("domain name should not be empty")
		}
		if d.Version == "" {
			t.Errorf("domain %q should have a version", d.Domain)
		}
		if d.Priority == 0 {
			t.Errorf("domain %q should have non-zero priority", d.Domain)
		}
	}
}
