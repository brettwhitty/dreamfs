package theme

import "github.com/charmbracelet/lipgloss"

var (
	// Primary style derived from legacy "Pink 205"
	Primary = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)

	// Standard status styles
	Success = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)  // Green
	Error   = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true) // Red
	Info    = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))             // Blue
	Warning = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))            // Orange/Yellow

	// Component specific styles
	Spinner = Primary
)
