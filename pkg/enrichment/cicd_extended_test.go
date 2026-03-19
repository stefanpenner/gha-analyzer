package enrichment

import (
	"testing"
)

func TestCICDEnricher_PipelineURL(t *testing.T) {
	e := &CICDEnricher{}

	// Task-level URL takes priority
	attrs := map[string]string{
		"cicd.pipeline.name":              "deploy",
		"cicd.pipeline.task.run.url.full": "https://jenkins.example.com/job/deploy/1",
		"vcs.repository.url.full":         "https://github.com/owner/repo",
	}
	h := e.Enrich("deploy", attrs, false)
	if h.URL != "https://jenkins.example.com/job/deploy/1" {
		t.Errorf("expected task URL, got %q", h.URL)
	}

	// Pipeline-level URL fallback
	attrs2 := map[string]string{
		"cicd.pipeline.name":              "build",
		"cicd.pipeline.run.url.full":      "https://gitlab.example.com/project/-/pipelines/42",
	}
	h2 := e.Enrich("build", attrs2, false)
	if h2.URL != "https://gitlab.example.com/project/-/pipelines/42" {
		t.Errorf("expected pipeline URL, got %q", h2.URL)
	}
}

func TestCICDEnricher_VCSContext(t *testing.T) {
	e := &CICDEnricher{}

	attrs := map[string]string{
		"cicd.pipeline.name": "build",
		"vcs.ref.head.name":  "feature/auth",
		"vcs.revision":       "abc123def456789",
	}
	h := e.Enrich("build", attrs, false)

	if h.VCSBranch != "feature/auth" {
		t.Errorf("expected branch 'feature/auth', got %q", h.VCSBranch)
	}
	if h.VCSRevision != "abc123def456789" {
		t.Errorf("expected revision, got %q", h.VCSRevision)
	}
}

func TestCICDEnricher_RunID(t *testing.T) {
	e := &CICDEnricher{}

	// Pipeline run ID
	attrs := map[string]string{
		"cicd.pipeline.name":   "build",
		"cicd.pipeline.run.id": "run-42",
	}
	h := e.Enrich("build", attrs, false)
	if h.RunID != "run-42" {
		t.Errorf("expected RunID 'run-42', got %q", h.RunID)
	}

	// Task run ID
	attrs2 := map[string]string{
		"cicd.pipeline.task.name":   "unit-tests",
		"cicd.pipeline.task.run.id": "task-99",
	}
	h2 := e.Enrich("unit-tests", attrs2, false)
	if h2.RunID != "task-99" {
		t.Errorf("expected RunID 'task-99', got %q", h2.RunID)
	}
}

func TestCICDEnricher_PipelineType(t *testing.T) {
	e := &CICDEnricher{}

	tests := []struct {
		pipelineType string
		icon         string
	}{
		{"deploy", "🚀 "},
		{"build", "🔨 "},
		{"test", "🧪 "},
		{"unknown", "▶ "},
	}

	for _, tt := range tests {
		attrs := map[string]string{
			"cicd.pipeline.name": "my-pipeline",
			"cicd.pipeline.type": tt.pipelineType,
		}
		h := e.Enrich("my-pipeline", attrs, false)
		if h.Icon != tt.icon {
			t.Errorf("pipelineType %q: expected icon %q, got %q", tt.pipelineType, tt.icon, h.Icon)
		}
	}
}

func TestCICDEnricher_RunResult(t *testing.T) {
	e := &CICDEnricher{}

	// cicd.pipeline.run.result (pipeline-level result)
	attrs := map[string]string{
		"cicd.pipeline.name":       "build",
		"cicd.pipeline.run.result": "success",
	}
	h := e.Enrich("build", attrs, false)
	if h.Outcome != "success" {
		t.Errorf("expected outcome 'success' from run.result, got %q", h.Outcome)
	}
}
