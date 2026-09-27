package main

import (
	"github.com/charmbracelet/lipgloss"
)

// Semantic color tokens. Never hardcode ANSI elsewhere.
//
// Every field here is read by at least one render site. The earlier
// version also carried Accent, Success and plain, none of which was
// ever read: dead tokens look like a palette and invite a caller to use
// one that renders identically to Title (review F1).
type Theme struct {
	Title    lipgloss.Style
	Selected lipgloss.Style
	Dim      lipgloss.Style
	Border   lipgloss.Style
	Active   lipgloss.Style
	Error    lipgloss.Style
	Warning  lipgloss.Style
	Status   lipgloss.Style
	Footer   lipgloss.Style
	Toast    lipgloss.Style
}

func NewTheme(plain bool, accentHex string) Theme {
	if accentHex == "" {
		accentHex = defaultAccent
	}
	if plain {
		// No colour at all, but the two weights stay: bold marks the
		// title and the selected row still has to be distinguishable
		// without a background colour.
		b := lipgloss.NewStyle().Bold(true)
		d := lipgloss.NewStyle().Bold(false)
		n := lipgloss.NewStyle()
		return Theme{
			Title: b, Selected: b.Reverse(true),
			Dim: d, Border: n, Active: b, Error: b, Warning: b,
			Status: n, Footer: d, Toast: b,
		}
	}
	accent := lipgloss.Color(accentHex)
	// The title is the accent, so a custom accent themes the whole UI
	// instead of leaving a purple title on a green theme.
	return Theme{
		Title:    lipgloss.NewStyle().Bold(true).Foreground(accent),
		Selected: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#1A1B26")).Background(accent),
		Dim:      lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")),
		Border:   lipgloss.NewStyle().Foreground(lipgloss.Color("#3B3F4E")),
		Active:   lipgloss.NewStyle().Foreground(accent).Bold(true),
		Error:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F87171")),
		Warning:  lipgloss.NewStyle().Foreground(lipgloss.Color("#FBBF24")),
		Status:   lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")),
		Footer:   lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")),
		Toast:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#1A1B26")).Background(lipgloss.Color("#A7F3D0")),
	}
}
