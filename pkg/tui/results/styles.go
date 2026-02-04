package results

import "github.com/charmbracelet/lipgloss"

// Color palette
var (
	ColorPurple     = lipgloss.Color("#7D56F4")
	ColorPurpleDim  = lipgloss.Color("#5A3FC2")
	ColorGreen      = lipgloss.Color("#25A065")
	ColorBlue       = lipgloss.Color("#4285F4")
	ColorRed        = lipgloss.Color("#E05252")
	ColorYellow     = lipgloss.Color("#E5C07B")
	ColorGray       = lipgloss.Color("#626262")
	ColorGrayLight  = lipgloss.Color("#888888")
	ColorGrayDim    = lipgloss.Color("#444444")
	ColorWhite      = lipgloss.Color("#FFFFFF")
	ColorOffWhite   = lipgloss.Color("#E0E0E0")
)

// Header styles
var (
	HeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPurple)

	HeaderCountStyle = lipgloss.NewStyle().
				Foreground(ColorOffWhite)

	FooterStyle = lipgloss.NewStyle().
			Foreground(ColorGrayLight)

	BorderStyle = lipgloss.NewStyle().
			Foreground(ColorGrayDim)
)

// Tree item styles
var (
	SelectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorWhite).
			Background(ColorPurpleDim)

	// HiddenSelectedStyle combines hidden (gray text) with selection background
	HiddenSelectedStyle = lipgloss.NewStyle().
				Foreground(ColorGrayLight).
				Background(ColorPurpleDim)

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

// Modal styles
var (
	ModalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorGrayLight).
			Padding(1, 2)

	ModalTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPurple)

	ModalLabelStyle = lipgloss.NewStyle().
			Foreground(ColorGrayLight).
			Width(14)

	ModalValueStyle = lipgloss.NewStyle().
			Foreground(ColorOffWhite)
)
