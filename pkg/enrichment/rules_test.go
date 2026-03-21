package enrichment

import (
	"os"
	"path/filepath"
	"testing"
)

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

func TestRuleEnricher_DefaultBarCharAndIcon(t *testing.T) {
	e := &RuleEnricher{
		Rules: []Rule{
			{
				Name:  "bare",
				Match: RuleMatch{SpanName: "test"},
				Hints: RuleHints{Category: "test"},
			},
		},
	}
	h := e.Enrich("test", nil, false)
	if h.BarChar != "█" {
		t.Errorf("expected default BarChar '█', got %q", h.BarChar)
	}
	if h.Icon != "● " {
		t.Errorf("expected default Icon '● ', got %q", h.Icon)
	}
}

func TestRuleEnricher_FirstMatchWins(t *testing.T) {
	e := &RuleEnricher{
		Rules: []Rule{
			{
				Name:  "first",
				Match: RuleMatch{SpanName: "test*"},
				Hints: RuleHints{Category: "first"},
			},
			{
				Name:  "second",
				Match: RuleMatch{SpanName: "test*"},
				Hints: RuleHints{Category: "second"},
			},
		},
	}
	h := e.Enrich("test-span", nil, false)
	if h.Category != "first" {
		t.Errorf("expected first match to win, got category %q", h.Category)
	}
}

func TestLoadRules_NonexistentFile(t *testing.T) {
	_, err := LoadRules("/nonexistent/path/rules.json")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestLoadRules_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("not json{{{"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadRules(path)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLoadRules_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.json")
	content := `{"enrichers": [{"name": "test", "match": {"span_name": "foo"}, "hints": {"category": "bar"}}]}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	e, err := LoadRules(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(e.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(e.Rules))
	}
	if e.Rules[0].Name != "test" {
		t.Errorf("expected rule name 'test', got %q", e.Rules[0].Name)
	}
}

func TestLoadRules_EmptyEnrichers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(path, []byte(`{"enrichers": []}`), 0644); err != nil {
		t.Fatal(err)
	}
	e, err := LoadRules(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(e.Rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(e.Rules))
	}
}
