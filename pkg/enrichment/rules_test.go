package enrichment

import (
	"testing"
)

func TestGlobMatch(t *testing.T) {
	tests := []struct {
		pattern string
		value   string
		want    bool
	}{
		{"*", "anything", true},
		{"*", "", true},
		{"exact", "exact", true},
		{"exact", "other", false},
		{"prefix*", "prefix-something", true},
		{"prefix*", "other", false},
		{"*suffix", "something-suffix", true},
		{"*suffix", "other", false},
		{"*middle*", "has-middle-here", true},
		{"*middle*", "no match", false},
	}
	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.value, func(t *testing.T) {
			got := globMatch(tt.pattern, tt.value)
			if got != tt.want {
				t.Errorf("globMatch(%q, %q) = %v, want %v", tt.pattern, tt.value, got, tt.want)
			}
		})
	}
}

func TestRuleEnricher_Match(t *testing.T) {
	e := &RuleEnricher{
		Rules: []Rule{
			{
				Name: "k8s-deploy",
				Match: RuleMatch{
					Attributes: map[string]string{
						"k8s.deployment.name": "*",
					},
				},
				Hints: RuleHints{
					Category: "deployment",
					Icon:     "🚀 ",
					Color:    "blue",
					IsRoot:   true,
				},
			},
			{
				Name: "http-errors",
				Match: RuleMatch{
					Attributes: map[string]string{
						"http.response.status_code": "5*",
					},
				},
				Hints: RuleHints{
					Category: "http",
					Outcome:  "failure",
					Color:    "red",
				},
			},
		},
	}

	// Should match k8s rule
	h := e.Enrich("deploy-nginx", map[string]string{"k8s.deployment.name": "nginx"}, false)
	if h.Category != "deployment" {
		t.Errorf("expected category 'deployment', got %q", h.Category)
	}
	if !h.IsRoot {
		t.Error("expected IsRoot=true")
	}

	// Should match http error rule
	h = e.Enrich("GET /api", map[string]string{"http.response.status_code": "503"}, false)
	if h.Category != "http" {
		t.Errorf("expected category 'http', got %q", h.Category)
	}
	if h.Outcome != "failure" {
		t.Errorf("expected outcome 'failure', got %q", h.Outcome)
	}

	// Should not match anything
	h = e.Enrich("other", map[string]string{"foo": "bar"}, false)
	if h.Category != "" {
		t.Errorf("expected empty category, got %q", h.Category)
	}
}

func TestRuleEnricher_SpanNameMatch(t *testing.T) {
	e := &RuleEnricher{
		Rules: []Rule{
			{
				Name: "health-check",
				Match: RuleMatch{
					SpanName: "GET /health*",
				},
				Hints: RuleHints{
					Category: "health",
					Color:    "gray",
				},
			},
		},
	}

	h := e.Enrich("GET /healthz", map[string]string{}, false)
	if h.Category != "health" {
		t.Errorf("expected category 'health', got %q", h.Category)
	}

	h = e.Enrich("GET /api/users", map[string]string{}, false)
	if h.Category != "" {
		t.Errorf("expected empty category, got %q", h.Category)
	}
}
