package results

import "github.com/charmbracelet/lipgloss"

// Color palette
var (
	ColorPurple   = lipgloss.Color("#7D56F4")
	ColorGreen    = lipgloss.Color("#25A065")
	ColorBlue     = lipgloss.Color("#4285F4")
	ColorRed      = lipgloss.Color("#E05252")
	ColorYellow   = lipgloss.Color("#E5C07B")
	ColorGray     = lipgloss.Color("#626262")
	ColorGrayDim  = lipgloss.Color("#404040")
	ColorWhite    = lipgloss.Color("#FFFFFF")
	ColorOffWhite    = lipgloss.Color("#D0D0D0")
	ColorMagenta     = lipgloss.Color("#C678DD")
	ColorSelectionBg = lipgloss.Color("#2D3B4D")

	// Dimmed colors for selected state (darker shades that still show status)
	ColorGreenDim  = lipgloss.Color("#1A7048")
	ColorBlueDim   = lipgloss.Color("#2E5EA8")
	ColorRedDim    = lipgloss.Color("#9E3A3A")
	ColorYellowDim = lipgloss.Color("#A08856")

	// Search match row background (subtle purple tint)
	ColorSearchRowBg  = lipgloss.Color("#1E1A2E")
	ColorSearchCharBg = lipgloss.Color("#2E2545")
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

	SeparatorStyle = lipgloss.NewStyle().
			Foreground(ColorGrayDim)
)

// Tree item styles
var (
	SelectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorWhite).
			Background(ColorSelectionBg)

	// HiddenSelectedStyle combines hidden (gray text) with selection background
	HiddenSelectedStyle = lipgloss.NewStyle().
				Foreground(ColorGray).
				Background(ColorSelectionBg)

	// SelectedBgStyle applies only the selection background (no foreground override)
	SelectedBgStyle = lipgloss.NewStyle().
			Background(ColorSelectionBg)

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

// Timeline bar colors for selected state (dimmed but still show status, with selection background)
var (
	BarSuccessSelectedStyle = lipgloss.NewStyle().
				Foreground(ColorGreenDim).
				Background(ColorSelectionBg)

	BarFailureSelectedStyle = lipgloss.NewStyle().
				Foreground(ColorRedDim).
				Background(ColorSelectionBg)

	BarFailureNonBlockingSelectedStyle = lipgloss.NewStyle().
						Foreground(ColorYellowDim).
						Background(ColorSelectionBg)

	BarPendingSelectedStyle = lipgloss.NewStyle().
				Foreground(ColorBlueDim).
				Background(ColorSelectionBg)

	BarSkippedSelectedStyle = lipgloss.NewStyle().
				Foreground(ColorGrayDim).
				Background(ColorSelectionBg)
)

// Time header style
var (
	TimeHeaderStyle = lipgloss.NewStyle().
			Foreground(ColorGray)

	DurationStyle = lipgloss.NewStyle().
			Foreground(ColorGray)
)

// Search styles
var (
	SearchBarStyle = lipgloss.NewStyle().
			Foreground(ColorWhite)

	// SearchRowStyle is the subtle background for the entire matching row
	SearchRowStyle = lipgloss.NewStyle().
			Background(ColorSearchRowBg)

	// SearchRowBgStyle applies only the row background (no foreground override)
	SearchRowBgStyle = lipgloss.NewStyle().
				Background(ColorSearchRowBg)

	// SearchCharStyle highlights the exact matching characters (stronger)
	SearchCharStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPurple).
			Background(ColorSearchCharBg)

	// SearchCharSelectedStyle highlights matching chars when row is also selected
	SearchCharSelectedStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorPurple).
				Background(ColorSelectionBg)

	SearchCountStyle = lipgloss.NewStyle().
				Foreground(ColorGray)
)

// Modal styles
var (
	ModalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPurple).
			Padding(1, 2)

	ModalTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPurple)

	ModalLabelStyle = lipgloss.NewStyle().
			Foreground(ColorGray).
			Width(14)

	ModalValueStyle = lipgloss.NewStyle().
			Foreground(ColorWhite)
)
