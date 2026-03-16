package enrichment

// CICDEnricher recognizes OTel CI/CD semantic conventions (v1.27+).
// It matches spans with cicd.pipeline.* attributes from any CI system
// (Jenkins, GitLab CI, Dagger, Buildkite, Gradle, etc.).
type CICDEnricher struct{}

// Enrich produces SpanHints for CI/CD spans using OTel semantic conventions.
// Returns empty hints (Category=="") if no cicd.pipeline.* attributes are found.
func (e *CICDEnricher) Enrich(name string, attrs map[string]string, isZeroDuration bool) SpanHints {
	pipelineName := attrs["cicd.pipeline.name"]
	taskName := attrs["cicd.pipeline.task.name"]
	taskType := attrs["cicd.pipeline.task.type"]

	if pipelineName == "" && taskName == "" {
		return SpanHints{}
	}

	h := SpanHints{
		BarChar: "█",
	}

	// URL from VCS attributes
	if url := attrs["vcs.repository.url.full"]; url != "" {
		h.URL = url
	}

	// Pipeline-level span (root)
	if pipelineName != "" && taskName == "" {
		h.Category = "pipeline"
		h.IsRoot = true
		h.Icon = "▶ "
		h.Color = "blue"
		enrichCICDOutcome(&h, attrs)
		return h
	}

	// Task-level span (intermediate)
	if taskName != "" {
		h.Category = "task"
		h.Icon = taskIconFromType(taskType)
		h.Color = "blue"
		enrichCICDOutcome(&h, attrs)
		return h
	}

	return SpanHints{}
}

// taskIconFromType returns an icon based on the CI/CD task type.
func taskIconFromType(taskType string) string {
	switch taskType {
	case "build":
		return "🔨 "
	case "test":
		return "🧪 "
	case "deploy":
		return "🚀 "
	default:
		return "⚙ "
	}
}

// enrichCICDOutcome maps cicd.pipeline.task.run.result to outcome/color.
func enrichCICDOutcome(h *SpanHints, attrs map[string]string) {
	result := attrs["cicd.pipeline.task.run.result"]
	switch result {
	case "success":
		h.Outcome = "success"
		h.Color = "green"
	case "failure":
		h.Outcome = "failure"
		h.Color = "red"
	case "cancelled":
		h.Outcome = "skipped"
		h.Color = "gray"
	case "error":
		h.Outcome = "failure"
		h.Color = "red"
	}

	// Also check OTel status code as fallback
	if h.Outcome == "" {
		switch attrs["otel.status_code"] {
		case "OK":
			h.Outcome = "success"
			h.Color = "green"
		case "ERROR":
			h.Outcome = "failure"
			h.Color = "red"
		}
	}
}
