package results

import "github.com/charmbracelet/lipgloss"

// Color palette - inspired by Claude Code UI
var (
	ColorGreen      = lipgloss.Color("#4EC970") // bright green for success/additions
	ColorMagenta    = lipgloss.Color("#E05299") // magenta for failures/deletions
	ColorBlue       = lipgloss.Color("#5C8DFF") // soft blue for pending/info
	ColorYellow     = lipgloss.Color("#E5C07B") // yellow for warnings
	ColorGray       = lipgloss.Color("#6B6B6B") // muted gray for secondary
	ColorGrayMuted  = lipgloss.Color("#555555") // more muted gray for footer
	ColorGrayLight  = lipgloss.Color("#8B8B8B") // lighter gray for borders
	ColorGrayDim    = lipgloss.Color("#3B3B3B") // dim gray for backgrounds
	ColorWhite      = lipgloss.Color("#FFFFFF") // white for primary text
	ColorOffWhite   = lipgloss.Color("#D4D4D4") // off-white for content
)

// Header styles
var (
	HeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorWhite)

	HeaderCountStyle = lipgloss.NewStyle().
				Foreground(ColorOffWhite)

	FooterStyle = lipgloss.NewStyle().
			Foreground(ColorGrayMuted)

	BorderStyle = lipgloss.NewStyle().
			Foreground(ColorGrayLight)

	// Subtle separator between tree and timeline columns
	SeparatorStyle = lipgloss.NewStyle().
			Foreground(ColorGrayDim)
)

// Tree item styles
var (
	SelectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorWhite).
			Background(ColorGrayDim)

	// HiddenSelectedStyle combines hidden (gray text) with selection background
	HiddenSelectedStyle = lipgloss.NewStyle().
				Foreground(ColorGray).
				Background(ColorGrayDim)

	NormalStyle = lipgloss.NewStyle().
			Foreground(ColorOffWhite)

	HiddenStyle = lipgloss.NewStyle().
			Foreground(ColorGray)

	WorkflowStyle = lipgloss.NewStyle().
			Foreground(ColorOffWhite)

	GroupStyle = lipgloss.NewStyle().
			Foreground(ColorBlue)

	JobStyle = lipgloss.NewStyle().
			Foreground(ColorOffWhite)
)

// Status styles
var (
	SuccessStyle = lipgloss.NewStyle().
			Foreground(ColorGreen)

	FailureStyle = lipgloss.NewStyle().
			Foreground(ColorMagenta)

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
			Foreground(ColorMagenta)

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

// Modal styles
var (
	ModalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorGrayLight).
			Padding(1, 2)

	ModalTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorWhite)

	ModalLabelStyle = lipgloss.NewStyle().
			Foreground(ColorGray).
			Width(14)

	ModalValueStyle = lipgloss.NewStyle().
			Foreground(ColorOffWhite)
)
