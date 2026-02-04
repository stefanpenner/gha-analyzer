package utils

import (
	"testing"
	"time"

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
		{name: "short pr url", url: "owner/repo/pull/123", expectType: "pr", expectID: "123"},
		{name: "short commit url", url: "owner/repo/commit/abc123def", expectType: "commit", expectID: "abc123def"},
		{name: "github.com pr url", url: "github.com/owner/repo/pull/123", expectType: "pr", expectID: "123"},
		{name: "invalid url", url: "https://github.com/owner/repo/issues/123", wantError: true},
		{name: "completely invalid", url: "not-a-url", wantError: true},
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

func TestGetStepIconConclusions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		conclusion string
		expected   string
	}{
		{"failure", "❌"},
		{"cancelled", "🚫"},
		{"skipped", "⏭️"},
	}

	for _, tc := range cases {
		t.Run(tc.conclusion, func(t *testing.T) {
			assert.Equal(t, tc.expected, GetStepIcon("any step", tc.conclusion))
		})
	}
}

func TestGetJobGroup(t *testing.T) {
	t.Parallel()

	cases := []struct {
		jobName  string
		expected string
	}{
		{"build / linux", "build"},
		{"test / unit / fast", "test"},
		{"single-job", "single-job"},
		{"", ""},
	}

	for _, tc := range cases {
		t.Run(tc.jobName, func(t *testing.T) {
			assert.Equal(t, tc.expected, GetJobGroup(tc.jobName))
		})
	}
}

func TestMakeClickableLink(t *testing.T) {
	t.Parallel()

	t.Run("returns display text for non-github URL", func(t *testing.T) {
		result := MakeClickableLink("https://example.com", "click me")
		assert.Equal(t, "click me", result)
	})

	t.Run("returns URL when text is empty for non-github", func(t *testing.T) {
		result := MakeClickableLink("https://example.com", "")
		assert.Equal(t, "https://example.com", result)
	})

	t.Run("wraps github URL in OSC 8 hyperlink", func(t *testing.T) {
		result := MakeClickableLink("https://github.com/owner/repo", "repo link")
		assert.Contains(t, result, "\u001b]8;;https://github.com/owner/repo\u0007")
		assert.Contains(t, result, "repo link")
		assert.Contains(t, result, "\u001b]8;;\u0007")
	})
}

func TestParseTime(t *testing.T) {
	t.Parallel()

	t.Run("parses valid RFC3339 time", func(t *testing.T) {
		parsed, ok := ParseTime("2026-01-15T10:30:00Z")
		assert.True(t, ok)
		assert.Equal(t, 2026, parsed.Year())
		assert.Equal(t, time.January, parsed.Month())
		assert.Equal(t, 15, parsed.Day())
	})

	t.Run("returns false for empty string", func(t *testing.T) {
		_, ok := ParseTime("")
		assert.False(t, ok)
	})

	t.Run("returns false for invalid format", func(t *testing.T) {
		_, ok := ParseTime("not a date")
		assert.False(t, ok)
	})
}

func TestStripANSI(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{"plain text unchanged", "hello world", "hello world"},
		{"strips SGR codes", "\u001b[32mgreen\u001b[0m", "green"},
		{"strips OSC hyperlink", "\u001b]8;;https://example.com\u0007link\u001b]8;;\u0007", "link"},
		{"preserves tabs and newlines", "line1\n\tline2", "line1\n\tline2"},
		{"strips control chars", "hello\x01world", "helloworld"},
		{"complex ANSI sequence", "\u001b[1;31mred bold\u001b[0m normal", "red bold normal"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, StripANSI(tc.input))
		})
	}
}

func TestColorFormatters(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		fn       func(string) string
		expected string
	}{
		{"GrayText", GrayText, "\u001b[90mtest\u001b[0m"},
		{"GreenText", GreenText, "\u001b[32mtest\u001b[0m"},
		{"RedText", RedText, "\u001b[31mtest\u001b[0m"},
		{"YellowText", YellowText, "\u001b[33mtest\u001b[0m"},
		{"BlueText", BlueText, "\u001b[34mtest\u001b[0m"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.fn("test"))
		})
	}
}
