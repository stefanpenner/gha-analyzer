package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseGitHubURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		url        string
		expectType string
		expectID   string
		wantError  bool
	}{
		{name: "pr url", url: "https://github.com/owner/repo/pull/123", expectType: "pr", expectID: "123"},
		{name: "commit url", url: "https://github.com/owner/repo/commit/abc123def", expectType: "commit", expectID: "abc123def"},
		{name: "invalid url", url: "https://github.com/owner/repo/issues/123", wantError: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ParseGitHubURL(tc.url)
			if tc.wantError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.expectType, result.Type)
			assert.Equal(t, tc.expectID, result.Identifier)
		})
	}
}

func TestHumanizeTime(t *testing.T) {
	t.Parallel()

	cases := []struct {
		seconds  float64
		expected string
	}{
		{seconds: 0, expected: "0s"},
		{seconds: 0.5, expected: "500ms"},
		{seconds: 1, expected: "1s"},
		{seconds: 65, expected: "1m 5s"},
		{seconds: 3661, expected: "1h 1m 1s"},
		{seconds: 86400, expected: "24h"},
	}

	for _, tc := range cases {
		t.Run(tc.expected, func(t *testing.T) {
			assert.Equal(t, tc.expected, HumanizeTime(tc.seconds))
		})
	}
}

func TestStepCategorizationAndIcons(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		category string
		icon     string
	}{
		{name: "Checkout code", category: "step_checkout", icon: "📥"},
		{name: "Setup Node.js", category: "step_setup", icon: "⚙️"},
		{name: "Build project", category: "step_build", icon: "🔨"},
		{name: "Run tests", category: "step_test", icon: "🧪"},
		{name: "Lint code", category: "step_lint", icon: "🔍"},
		{name: "Deploy to prod", category: "step_deploy", icon: "🚀"},
		{name: "Upload artifacts", category: "step_artifact", icon: "📤"},
		{name: "Security scan", category: "step_security", icon: "🔒"},
		{name: "Send notification", category: "step_notify", icon: "📢"},
		{name: "Custom step", category: "step_other", icon: "▶️"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.category, CategorizeStep(tc.name))
			assert.Equal(t, tc.icon, GetStepIcon(tc.name, "success"))
		})
	}
	assert.Equal(t, "❌", GetStepIcon("Any step", "failure"))
}
