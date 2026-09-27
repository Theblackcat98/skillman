package main

import (
	"github.com/charmbracelet/lipgloss"
)

// Semantic color tokens. Never hardcode ANSI elsewhere.

type Theme struct {
	plain    bool
	Title    lipgloss.Style
	Accent   lipgloss.Style
	Selected lipgloss.Style
	Dim      lipgloss.Style
	Border   lipgloss.Style
	Active   lipgloss.Style
	Error    lipgloss.Style
	Warning  lipgloss.Style
	Success  lipgloss.Style
	Status   lipgloss.Style
	Footer   lipgloss.Style
	Toast    lipgloss.Style
}

func NewTheme(plain bool, accentHex string) Theme {
	if accentHex == "" {
		accentHex = defaultAccent
	}
	if plain {
		b := lipgloss.NewStyle().Bold(true)
		n := lipgloss.NewStyle()
		d := lipgloss.NewStyle().Bold(false)
		return Theme{
			plain: true, Title: b, Accent: b, Selected: b.Reverse(true),
			Dim: d, Border: n, Active: b, Error: b, Warning: b,
			Success: b, Status: n, Footer: d, Toast: b,
		}
	}
	accent := lipgloss.Color(accentHex)
	// The title is the accent, so a custom accent themes the whole UI
	// instead of leaving a purple title on a green theme.
	return Theme{
		Title:    lipgloss.NewStyle().Bold(true).Foreground(accent),
		Accent:   lipgloss.NewStyle().Bold(true).Foreground(accent),
		Selected: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#1A1B26")).Background(accent),
		Dim:      lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")),
		Border:   lipgloss.NewStyle().Foreground(lipgloss.Color("#3B3F4E")),
		Active:   lipgloss.NewStyle().Foreground(accent).Bold(true),
		Error:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F87171")),
		Warning:  lipgloss.NewStyle().Foreground(lipgloss.Color("#FBBF24")),
		Success:  lipgloss.NewStyle().Foreground(lipgloss.Color("#34D399")),
		Status:   lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")),
		Footer:   lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")),
		Toast:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#1A1B26")).Background(lipgloss.Color("#A7F3D0")),
	}
}
