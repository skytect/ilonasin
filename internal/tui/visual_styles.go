package tui

import "github.com/charmbracelet/lipgloss"

var (
	cardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63")).
			Padding(0, 1).
			MarginBottom(1)
	cardTitleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("219"))
	mutedStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	labelStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("87"))
	valueStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	appTitleStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("93")).Padding(0, 1)
	tabActiveStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("16")).Background(lipgloss.Color("219"))
	tabInactiveStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("111"))
	chipStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(lipgloss.Color("60")).Padding(0, 1)
	goodBadgeStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("16")).Background(lipgloss.Color("48")).Padding(0, 1)
	warnBadgeStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("16")).Background(lipgloss.Color("220")).Padding(0, 1)
	badBadgeStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(lipgloss.Color("196")).Padding(0, 1)
	goodBarStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("48"))
	warnBarStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	badBarStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	emptyBarStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	heroStyle         = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("93")).Padding(0, 1)
	identityStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("30")).Padding(0, 1)
	windowStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("16")).Background(lipgloss.Color("117")).Padding(0, 1)
	paneStyle         = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("60")).Padding(0, 1)
	focusedPaneStyle  = paneStyle.BorderForeground(lipgloss.Color("219"))
	paneTitleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("117"))
	focusedTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("93")).Padding(0, 1)
)
