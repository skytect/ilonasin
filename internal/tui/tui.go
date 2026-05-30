package tui

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"ilonasin/internal/config"
	"ilonasin/internal/credentials"
)

type Model struct {
	cfg           config.Config
	tokens        credentials.LocalTokenManager
	tokenRows     []credentials.LocalTokenMetadata
	selected      int
	reveal        string
	revealTokenID int64
	err           string
	quitOnInit    bool
}

func NewModel(cfg config.Config, tokens credentials.LocalTokenManager) Model {
	return Model{cfg: cfg, tokens: tokens}
}

func newCheckModel(cfg config.Config, tokens credentials.LocalTokenManager) Model {
	return Model{cfg: cfg, tokens: tokens, quitOnInit: true}
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
			m.clearReveal()
			return m, tea.Quit
		case "n":
			m.clearReveal()
			created, err := m.tokens.Create(context.Background(), "local client")
			if err != nil {
				m.err = err.Error()
				return m, nil
			}
			m.reveal = created.Token
			m.revealTokenID = created.Metadata.ID
			_ = m.reload()
		case "d":
			m.clearReveal()
			if len(m.tokenRows) == 0 {
				return m, nil
			}
			if err := m.tokens.Disable(context.Background(), m.tokenRows[m.selected].ID); err != nil {
				m.err = err.Error()
				return m, nil
			}
			_ = m.reload()
		case "esc", "enter":
			m.clearReveal()
		case "down", "j":
			m.clearReveal()
			if m.selected < len(m.tokenRows)-1 {
				m.selected++
			}
		case "up", "k":
			m.clearReveal()
			if m.selected > 0 {
				m.selected--
			}
		}
	}
	return m, nil
}

func (m Model) View() string {
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).Render("ilonasin")
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\nProviders: %d\nBind: %s\n\nLocal API tokens\n", title, len(m.cfg.Providers), m.cfg.Server.Bind)
	if m.err != "" {
		fmt.Fprintf(&b, "Error: %s\n", m.err)
	}
	if len(m.tokenRows) == 0 {
		b.WriteString("No local API tokens.\n")
	}
	for i, token := range m.tokenRows {
		cursor := " "
		if i == m.selected {
			cursor = ">"
		}
		state := "enabled"
		if token.Disabled {
			state = "disabled"
		}
		fmt.Fprintf(&b, "%s %d %s %s...%s %s\n", cursor, token.ID, token.Label, token.TokenPrefix, token.TokenLast4, state)
	}
	if m.reveal != "" {
		fmt.Fprintf(&b, "\nNew token %s: %s\n", strconv.FormatInt(m.revealTokenID, 10), m.reveal)
	}
	b.WriteString("\nPress n to create, d to disable, q to quit.\n")
	return b.String()
}

func Run(cfg config.Config, tokens credentials.LocalTokenManager) error {
	model := NewModel(cfg, tokens)
	_ = model.reload()
	_, err := tea.NewProgram(model).Run()
	return err
}

func Check(cfg config.Config, tokens credentials.LocalTokenManager, out io.Writer) error {
	model := newCheckModel(cfg, tokens)
	_ = model.reload()
	program := tea.NewProgram(model, tea.WithoutRenderer(), tea.WithInput(nil), tea.WithOutput(io.Discard))
	if _, err := program.Run(); err != nil {
		return err
	}
	_, err := io.WriteString(out, model.View())
	return err
}

func ExerciseTokenLifecycle(ctx context.Context, tokens credentials.LocalTokenManager) error {
	model := NewModel(config.Config{}, tokens)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m := updated.(Model)
	if m.reveal == "" || m.revealTokenID == 0 {
		return fmt.Errorf("token create did not enter reveal state")
	}
	id := m.revealTokenID
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.reveal != "" || m.revealTokenID != 0 {
		return fmt.Errorf("token reveal state was not cleared")
	}
	if strings.Contains(m.View(), "New token") {
		return fmt.Errorf("token reveal view was not cleared")
	}
	_ = m.reload()
	for i, row := range m.tokenRows {
		if row.ID == id {
			m.selected = i
			break
		}
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = updated.(Model)
	for _, row := range m.tokenRows {
		if row.ID == id {
			if !row.Disabled {
				return fmt.Errorf("token disable did not mark token disabled")
			}
			return nil
		}
	}
	return fmt.Errorf("created token missing from token list")
}

func (m *Model) reload() error {
	if m.tokens == nil {
		return nil
	}
	rows, err := m.tokens.List(context.Background())
	if err != nil {
		m.err = err.Error()
		return err
	}
	m.tokenRows = rows
	if m.selected >= len(m.tokenRows) {
		m.selected = len(m.tokenRows) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
	return nil
}

func (m *Model) clearReveal() {
	m.reveal = ""
	m.revealTokenID = 0
}
