package results

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines all key bindings for the TUI
type KeyMap struct {
	Up          key.Binding
	Down        key.Binding
	ShiftUp     key.Binding
	ShiftDown   key.Binding
	Left        key.Binding
	Right       key.Binding
	Enter       key.Binding
	Space       key.Binding
	Open        key.Binding
	Info        key.Binding
	Focus       key.Binding
	Reload      key.Binding
	ExpandAll   key.Binding
	CollapseAll key.Binding
	Perfetto    key.Binding
	Search      key.Binding
	Mouse       key.Binding
	Help        key.Binding
	Quit        key.Binding
}

// DefaultKeyMap returns the default key bindings
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		ShiftUp: key.NewBinding(
			key.WithKeys("shift+up", "K"),
			key.WithHelp("shift+↑", "select up"),
		),
		ShiftDown: key.NewBinding(
			key.WithKeys("shift+down", "J"),
			key.WithHelp("shift+↓", "select down"),
		),
		Left: key.NewBinding(
			key.WithKeys("left", "h"),
			key.WithHelp("←/h", "collapse"),
		),
		Right: key.NewBinding(
			key.WithKeys("right", "l"),
			key.WithHelp("→/l", "expand"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "toggle"),
		),
		Space: key.NewBinding(
			key.WithKeys(" "),
			key.WithHelp("space", "toggle chart"),
		),
		Open: key.NewBinding(
			key.WithKeys("o"),
			key.WithHelp("o", "open"),
		),
		Info: key.NewBinding(
			key.WithKeys("i"),
			key.WithHelp("i", "info"),
		),
		Focus: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "focus"),
		),
		Reload: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "reload"),
		),
		ExpandAll: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "expand all"),
		),
		CollapseAll: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "collapse all"),
		),
		Perfetto: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "perfetto"),
		),
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		Mouse: key.NewBinding(
			key.WithKeys("m"),
			key.WithHelp("m", "toggle mouse"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
}

// ShortHelp returns a short help string for the footer
func (k KeyMap) ShortHelp() string {
	return "↑↓ nav • ←→ expand/collapse • space toggle • / search • o open • ? help • q quit"
}

// FullHelp returns all key bindings for the help modal
func (k KeyMap) FullHelp() [][]string {
	return [][]string{
		{"↑/k", "Move up"},
		{"↓/j", "Move down"},
		{"shift+↑/K", "Select up"},
		{"shift+↓/J", "Select down"},
		{"←/h", "Collapse / go to parent"},
		{"→/l", "Expand"},
		{"enter", "Toggle expand"},
		{"space", "Toggle chart visibility"},
		{"o", "Open in browser"},
		{"i", "Item info"},
		{"f", "Focus on selection"},
		{"e", "Expand all"},
		{"c", "Collapse all"},
		{"r", "Reload data"},
		{"p", "Open in Perfetto"},
		{"/", "Search/filter"},
		{"m", "Toggle mouse mode"},
		{"?", "Show this help"},
		{"q", "Quit"},
	}
}
