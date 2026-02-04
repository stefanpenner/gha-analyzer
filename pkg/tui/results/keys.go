package results

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines all key bindings for the TUI
type KeyMap struct {
	Up          key.Binding
	Down        key.Binding
	Left        key.Binding
	Right       key.Binding
	Enter       key.Binding
	Space       key.Binding
	Open        key.Binding
	ExpandAll   key.Binding
	CollapseAll key.Binding
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
		ExpandAll: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "expand all"),
		),
		CollapseAll: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "collapse all"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
}

// ShortHelp returns a short help string for the footer
func (k KeyMap) ShortHelp() string {
	return "↑↓ navigate • ←→ expand/collapse • space toggle chart • o open • e/c all • q quit"
}
