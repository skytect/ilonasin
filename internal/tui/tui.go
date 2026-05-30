package tui

import (
	"fmt"
	"io"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"ilonasin/internal/config"
)

type Model struct {
	cfg        config.Config
	quitOnInit bool
}

func NewModel(cfg config.Config) Model {
	return Model{cfg: cfg}
}

func newCheckModel(cfg config.Config) Model {
	return Model{cfg: cfg, quitOnInit: true}
}

func (m Model) Init() tea.Cmd {
	if m.quitOnInit {
		return tea.Quit
	}
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m Model) View() string {
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).Render("ilonasin")
	return fmt.Sprintf("%s\n\nProviders: %d\nBind: %s\n\nPress q to quit.\n", title, len(m.cfg.Providers), m.cfg.Server.Bind)
}

func Run(cfg config.Config) error {
	_, err := tea.NewProgram(NewModel(cfg)).Run()
	return err
}

func Check(cfg config.Config, out io.Writer) error {
	model := newCheckModel(cfg)
	program := tea.NewProgram(model, tea.WithoutRenderer(), tea.WithInput(nil), tea.WithOutput(io.Discard))
	if _, err := program.Run(); err != nil {
		return err
	}
	_, err := io.WriteString(out, model.View())
	return err
}
