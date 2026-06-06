package tui

import "github.com/charmbracelet/lipgloss"

var (
	cardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("99")).
			Padding(0, 1).
			MarginBottom(1)
	cardTitleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213"))
	mutedStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	labelStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("51"))
	valueStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("231"))
	appTitleStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("57")).Padding(0, 1)
	tabActiveStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("16")).Background(lipgloss.Color("45"))
	tabInactiveStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("75"))
	chipStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(lipgloss.Color("25")).Padding(0, 1)
	goodBadgeStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("16")).Background(lipgloss.Color("40")).Padding(0, 1)
	warnBadgeStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("16")).Background(lipgloss.Color("214")).Padding(0, 1)
	badBadgeStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(lipgloss.Color("160")).Padding(0, 1)
	goodBarStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("40"))
	warnBarStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	badBarStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	emptyBarStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	heroStyle         = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("57")).Padding(0, 1)
	identityStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("29")).Padding(0, 1)
	windowStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("16")).Background(lipgloss.Color("39")).Padding(0, 1)
	paneStyle         = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("24")).Padding(0, 1)
	focusedPaneStyle  = paneStyle.BorderForeground(lipgloss.Color("213"))
	paneTitleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("45"))
	focusedTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("125")).Padding(0, 1)
)
