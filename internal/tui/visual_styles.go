package tui

import "github.com/charmbracelet/lipgloss"

var (
	cardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("238")).
			Padding(0, 1).
			MarginBottom(1)
	cardTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("219"))
	mutedStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	labelStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("110"))
	valueStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	chipStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("238")).Padding(0, 1)
	goodBadgeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("16")).Background(lipgloss.Color("42")).Padding(0, 1)
	warnBadgeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("16")).Background(lipgloss.Color("214")).Padding(0, 1)
	badBadgeStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("160")).Padding(0, 1)
	goodBarStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	warnBarStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	badBarStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	emptyBarStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("236"))
	heroStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("57")).Padding(0, 1)
	identityStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("24")).Padding(0, 1)
	windowStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("16")).Background(lipgloss.Color("110")).Padding(0, 1)
)
