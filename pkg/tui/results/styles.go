package results

import "github.com/charmbracelet/lipgloss"

// Color palette
var (
	ColorPurple = lipgloss.Color("#7D56F4")
	ColorGreen  = lipgloss.Color("#25A065")
	ColorBlue   = lipgloss.Color("#4285F4")
	ColorRed    = lipgloss.Color("#E05252")
	ColorYellow = lipgloss.Color("#E5C07B")
	ColorGray   = lipgloss.Color("#626262")
	ColorWhite  = lipgloss.Color("#FFFFFF")
)

// Header styles
var (
	HeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPurple)

	HeaderCountStyle = lipgloss.NewStyle().
				Foreground(ColorGray)

	FooterStyle = lipgloss.NewStyle().
			Foreground(ColorGray)

	BorderStyle = lipgloss.NewStyle().
			Foreground(ColorGray)
)

// Tree item styles
var (
	SelectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorWhite).
			Background(lipgloss.Color("#3D3D3D"))

	NormalStyle = lipgloss.NewStyle()

	HiddenStyle = lipgloss.NewStyle().
			Foreground(ColorGray)

	WorkflowStyle = lipgloss.NewStyle().
			Foreground(ColorPurple)

	GroupStyle = lipgloss.NewStyle().
			Foreground(ColorBlue)

	JobStyle = lipgloss.NewStyle()
)

// Status styles
var (
	SuccessStyle = lipgloss.NewStyle().
			Foreground(ColorGreen)

	FailureStyle = lipgloss.NewStyle().
			Foreground(ColorRed)

	PendingStyle = lipgloss.NewStyle().
			Foreground(ColorBlue)

	SkippedStyle = lipgloss.NewStyle().
			Foreground(ColorGray)
)

// Timeline bar colors
var (
	BarSuccessStyle = lipgloss.NewStyle().
			Foreground(ColorGreen)

	BarFailureStyle = lipgloss.NewStyle().
			Foreground(ColorRed)

	BarFailureNonBlockingStyle = lipgloss.NewStyle().
					Foreground(ColorYellow)

	BarPendingStyle = lipgloss.NewStyle().
			Foreground(ColorBlue)

	BarSkippedStyle = lipgloss.NewStyle().
			Foreground(ColorGray)
)

// Time header style
var (
	TimeHeaderStyle = lipgloss.NewStyle().
			Foreground(ColorGray)

	DurationStyle = lipgloss.NewStyle().
			Foreground(ColorGray)
)
