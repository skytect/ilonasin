package tui

import "github.com/charmbracelet/lipgloss"

var (
	cardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("171")).
			Padding(0, 1).
			MarginBottom(1)
	cardTitleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("219"))
	mutedStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	labelStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("87"))
	valueStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("231"))
	appTitleStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("93")).Padding(0, 1)
	tabActiveStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("16")).Background(lipgloss.Color("51"))
	tabInactiveStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("87"))
	chipStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(lipgloss.Color("62")).Padding(0, 1)
	goodBadgeStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("16")).Background(lipgloss.Color("46")).Padding(0, 1)
	warnBadgeStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("16")).Background(lipgloss.Color("220")).Padding(0, 1)
	badBadgeStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(lipgloss.Color("196")).Padding(0, 1)
	goodBarStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
	warnBarStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	badBarStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	emptyBarStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	heroStyle         = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("93")).Padding(0, 1)
	identityStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("16")).Background(lipgloss.Color("48")).Padding(0, 1)
	windowStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("16")).Background(lipgloss.Color("45")).Padding(0, 1)
	paneStyle         = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("75")).Padding(0, 1)
	focusedPaneStyle  = paneStyle.BorderForeground(lipgloss.Color("219"))
	paneTitleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51"))
	focusedTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("161")).Padding(0, 1)
)
