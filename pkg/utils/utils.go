package utils

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type ParsedGitHubURL struct {
	Owner      string
	Repo       string
	Type       string
	Identifier string
}

func HumanizeTime(seconds float64) string {
	if seconds == 0 {
		return "0s"
	}
	if seconds < 1 {
		return fmt.Sprintf("%dms", int(seconds*1000+0.5))
	}

	hours := int(seconds) / 3600
	minutes := (int(seconds) % 3600) / 60
	secs := int(seconds) % 60

	var parts []string
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if secs > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%ds", secs))
	}
	return strings.Join(parts, " ")
}

func ParseGitHubURL(raw string) (ParsedGitHubURL, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return ParsedGitHubURL{}, fmt.Errorf("Invalid GitHub URL: %s. Expected format: PR: https://github.com/owner/repo/pull/123 or Commit: https://github.com/owner/repo/commit/abc123...", raw)
	}

	parts := strings.FieldsFunc(parsed.Path, func(r rune) bool { return r == '/' })
	if len(parts) == 4 && parts[2] == "pull" {
		return ParsedGitHubURL{Owner: parts[0], Repo: parts[1], Type: "pr", Identifier: parts[3]}, nil
	}
	if len(parts) == 4 && parts[2] == "commit" {
		return ParsedGitHubURL{Owner: parts[0], Repo: parts[1], Type: "commit", Identifier: parts[3]}, nil
	}

	return ParsedGitHubURL{}, fmt.Errorf("Invalid GitHub URL: %s. Expected format: PR: https://github.com/owner/repo/pull/123 or Commit: https://github.com/owner/repo/commit/abc123...", raw)
}

func GetJobGroup(jobName string) string {
	parts := strings.Split(jobName, " / ")
	if len(parts) > 1 {
		return parts[0]
	}
	return jobName
}

func MakeClickableLink(urlValue, text string) string {
	displayText := text
	if displayText == "" {
		displayText = urlValue
	}
	if !isGitHubURL(urlValue) {
		return displayText
	}
	return fmt.Sprintf("\u001b]8;;%s\u0007%s\u001b]8;;\u0007", urlValue, displayText)
}

func GrayText(text string) string {
	return fmt.Sprintf("\u001b[90m%s\u001b[0m", text)
}

func GreenText(text string) string {
	return fmt.Sprintf("\u001b[32m%s\u001b[0m", text)
}

func RedText(text string) string {
	return fmt.Sprintf("\u001b[31m%s\u001b[0m", text)
}

func YellowText(text string) string {
	return fmt.Sprintf("\u001b[33m%s\u001b[0m", text)
}

func BlueText(text string) string {
	return fmt.Sprintf("\u001b[34m%s\u001b[0m", text)
}

func CategorizeStep(stepName string) string {
	name := strings.ToLower(stepName)
	switch {
	case strings.Contains(name, "checkout") || strings.Contains(name, "clone"):
		return "step_checkout"
	case strings.Contains(name, "setup") || strings.Contains(name, "install") || strings.Contains(name, "cache"):
		return "step_setup"
	case strings.Contains(name, "build") || strings.Contains(name, "compile") || strings.Contains(name, "make"):
		return "step_build"
	case strings.Contains(name, "test") || strings.Contains(name, "spec") || strings.Contains(name, "coverage"):
		return "step_test"
	case strings.Contains(name, "lint") || strings.Contains(name, "format") || strings.Contains(name, "check"):
		return "step_lint"
	case strings.Contains(name, "deploy") || strings.Contains(name, "publish") || strings.Contains(name, "release"):
		return "step_deploy"
	case strings.Contains(name, "upload") || strings.Contains(name, "artifact") || strings.Contains(name, "store"):
		return "step_artifact"
	case strings.Contains(name, "security") || strings.Contains(name, "scan") || strings.Contains(name, "audit"):
		return "step_security"
	case strings.Contains(name, "notification") || strings.Contains(name, "slack") || strings.Contains(name, "email"):
		return "step_notify"
	default:
		return "step_other"
	}
}

func GetStepIcon(stepName, conclusion string) string {
	name := strings.ToLower(stepName)
	switch conclusion {
	case "failure":
		return "❌"
	case "cancelled":
		return "🚫"
	case "skipped":
		return "⏭️"
	}

	switch {
	case strings.Contains(name, "checkout") || strings.Contains(name, "clone"):
		return "📥"
	case strings.Contains(name, "setup") || strings.Contains(name, "install"):
		return "⚙️"
	case strings.Contains(name, "cache"):
		return "💾"
	case strings.Contains(name, "build") || strings.Contains(name, "compile"):
		return "🔨"
	case strings.Contains(name, "test") || strings.Contains(name, "spec"):
		return "🧪"
	case strings.Contains(name, "lint") || strings.Contains(name, "format"):
		return "🔍"
	case strings.Contains(name, "deploy") || strings.Contains(name, "publish"):
		return "🚀"
	case strings.Contains(name, "upload") || strings.Contains(name, "artifact"):
		return "📤"
	case strings.Contains(name, "security") || strings.Contains(name, "scan"):
		return "🔒"
	case strings.Contains(name, "notification") || strings.Contains(name, "slack"):
		return "📢"
	case strings.Contains(name, "docker") || strings.Contains(name, "container"):
		return "🐳"
	case strings.Contains(name, "database") || strings.Contains(name, "migrate"):
		return "🗄️"
	default:
		return "▶️"
	}
}

func ParseTime(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func isGitHubURL(urlValue string) bool {
	return strings.HasPrefix(urlValue, "https://github.com/") || strings.HasPrefix(urlValue, "http://github.com/")
}

func OpenBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default: // linux, freebsd, openbsd, netbsd
		cmd = "xdg-open"
		args = []string{url}
	}
	return exec.Command(cmd, args...).Start()
}
