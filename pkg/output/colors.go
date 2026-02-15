package output

import "github.com/charmbracelet/lipgloss"

// Color palette — duplicated from pkg/tui/results/styles.go to avoid
// pulling in bubbletea via that package.
var (
	colorPurple  = lipgloss.Color("#7D56F4")
	colorGreen   = lipgloss.Color("#25A065")
	colorBlue    = lipgloss.Color("#4285F4")
	colorRed     = lipgloss.Color("#E05252")
	colorYellow  = lipgloss.Color("#E5C07B")
	colorGray    = lipgloss.Color("#626262")
	colorWhite   = lipgloss.Color("#FFFFFF")
	colorMagenta = lipgloss.Color("#C678DD")
)

// Reusable styles for styled terminal output.
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPurple)

	subheaderStyle = lipgloss.NewStyle().
			Foreground(colorBlue).
			Bold(true)

	labelStyle = lipgloss.NewStyle().
			Foreground(colorGray)

	valueStyle = lipgloss.NewStyle().
			Foreground(colorWhite)

	successStyle = lipgloss.NewStyle().
			Foreground(colorGreen)

	failureStyle = lipgloss.NewStyle().
			Foreground(colorRed)

	warningStyle = lipgloss.NewStyle().
			Foreground(colorYellow)

	dimStyle = lipgloss.NewStyle().
			Foreground(colorGray)

	borderStyle = lipgloss.NewStyle().
			Foreground(colorGray)

	numStyle = lipgloss.NewStyle().
			Foreground(colorBlue)
)

// colorForSuccessRate returns a style based on the success rate value.
func colorForSuccessRate(rate float64) lipgloss.Style {
	switch {
	case rate >= 100:
		return successStyle
	case rate >= 80:
		return lipgloss.NewStyle().Foreground(colorWhite)
	case rate >= 50:
		return warningStyle
	default:
		return lipgloss.NewStyle().Foreground(colorMagenta)
	}
}
