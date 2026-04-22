// Package app hosts the root bubbletea Program.
package app

import "github.com/charmbracelet/lipgloss"

// Theme is the installer color palette.
type Theme struct {
	Title  lipgloss.Style
	Step   lipgloss.Style
	Help   lipgloss.Style
	Error  lipgloss.Style
	Border lipgloss.Style
}

// DefaultTheme returns the installer's default palette.
func DefaultTheme() Theme {
	return Theme{
		Title:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213")),
		Step:   lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
		Help:   lipgloss.NewStyle().Faint(true),
		Error:  lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true),
		Border: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2),
	}
}
