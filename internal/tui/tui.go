package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"ilonasin/internal/config"
	"ilonasin/internal/credentials"
	"ilonasin/internal/provider"
)

type Model struct {
	cfg            config.Config
	registry       provider.Registry
	tokens         credentials.LocalTokenManager
	upstreams      credentials.UpstreamCredentialManager
	modelCache     ModelCacheReader
	tokenRows      []credentials.LocalTokenMetadata
	providers      []provider.Instance
	credentials    []credentials.UpstreamCredentialMetadata
	modelRows      []provider.ModelMetadata
	selected       int
	reveal         string
	revealTokenID  int64
	apiKeyMode     bool
	apiKeyProvider string
	apiKeyInput    string
	err            string
	quitOnInit     bool
	checkMode      bool
}

type ModelCacheReader interface {
	ListModelCache(ctx context.Context) ([]provider.ModelMetadata, error)
}

func NewModel(cfg config.Config, registry provider.Registry, tokens credentials.LocalTokenManager, upstreams credentials.UpstreamCredentialManager, modelCache ModelCacheReader) Model {
	return Model{cfg: cfg, registry: registry, tokens: tokens, upstreams: upstreams, modelCache: modelCache}
}

func newCheckModel(cfg config.Config, registry provider.Registry, tokens credentials.LocalTokenManager, upstreams credentials.UpstreamCredentialManager, modelCache ModelCacheReader) Model {
	return Model{cfg: cfg, registry: registry, tokens: tokens, upstreams: upstreams, modelCache: modelCache, quitOnInit: true, checkMode: true}
}

func (m Model) Init() tea.Cmd {
	if m.quitOnInit {
		return tea.Quit
	}
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		if m.apiKeyMode {
			return m.updateAPIKeyInput(key)
		}
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
		case "x":
			m.clearReveal()
			if err := m.disableFirstUpstreamCredential(); err != nil {
				m.err = err.Error()
				return m, nil
			}
			_ = m.reload()
		case "a":
			m.clearReveal()
			if !m.checkMode {
				instance, ok := firstAPIKeyProvider(m.registry)
				if !ok {
					m.err = "no API-key provider instance is configured"
					return m, nil
				}
				m.apiKeyMode = true
				m.apiKeyProvider = instance.ID
				m.apiKeyInput = ""
				return m, nil
			}
			if err := m.addCheckUpstreamCredential(); err != nil {
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
	if m.apiKeyMode {
		fmt.Fprintf(&b, "\nAdding API key for %s: %s\n", m.apiKeyProvider, strings.Repeat("*", len(m.apiKeyInput)))
	}
	b.WriteString("\nProvider instances\n")
	for _, instance := range m.providers {
		apiKey := "api-key disabled"
		if instance.APIKey {
			apiKey = "api-key"
		}
		fmt.Fprintf(&b, "- %s %s %s %s\n", instance.ID, instance.Type, instance.BaseURL, apiKey)
	}
	b.WriteString("\nUpstream credentials\n")
	if len(m.credentials) == 0 {
		b.WriteString("No upstream credentials.\n")
	}
	for _, cred := range m.credentials {
		state := "enabled"
		if cred.Disabled {
			state = "disabled"
		}
		fmt.Fprintf(&b, "- %d %s %s %s...%s %s\n", cred.ID, cred.ProviderInstanceID, cred.Label, cred.SecretPrefix, cred.SecretLast4, state)
	}
	b.WriteString("\nModel cache\n")
	summaries := modelCacheSummaries(m.modelRows)
	if len(summaries) == 0 {
		b.WriteString("No cached models.\n")
	}
	for _, summary := range summaries {
		fmt.Fprintf(&b, "- %s %d models updated %s\n", summary.ProviderInstanceID, summary.Count, summary.UpdatedAt)
	}
	b.WriteString("\nPress n to create local token, a to add API key, d to disable local token, x to disable API key, q to quit.\n")
	return b.String()
}

func Run(cfg config.Config, registry provider.Registry, tokens credentials.LocalTokenManager, upstreams credentials.UpstreamCredentialManager, modelCache ModelCacheReader) error {
	model := NewModel(cfg, registry, tokens, upstreams, modelCache)
	_ = model.reload()
	_, err := tea.NewProgram(model).Run()
	return err
}

func Check(cfg config.Config, registry provider.Registry, tokens credentials.LocalTokenManager, upstreams credentials.UpstreamCredentialManager, modelCache ModelCacheReader, out io.Writer) error {
	model := newCheckModel(cfg, registry, tokens, upstreams, modelCache)
	_ = model.reload()
	program := tea.NewProgram(model, tea.WithoutRenderer(), tea.WithInput(nil), tea.WithOutput(io.Discard))
	if _, err := program.Run(); err != nil {
		return err
	}
	_, err := io.WriteString(out, model.View())
	return err
}

func ExerciseTokenLifecycle(ctx context.Context, tokens credentials.LocalTokenManager) error {
	model := NewModel(config.Config{}, provider.Registry{}, tokens, nil, nil)
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

func ExerciseUpstreamCredentialLifecycle(ctx context.Context, cfg config.Config, registry provider.Registry, upstreams credentials.UpstreamCredentialManager) error {
	instance, ok := firstAPIKeyProvider(registry)
	if !ok {
		return nil
	}
	model := newCheckModel(cfg, registry, nil, upstreams, nil)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m := updated.(Model)
	_ = m.reload()
	for _, cred := range m.credentials {
		if cred.ProviderInstanceID == instance.ID && cred.Label == "manage-check-upstream" {
			if cred.Disabled {
				return fmt.Errorf("check upstream credential unexpectedly disabled")
			}
			if err := upstreams.Disable(ctx, cred.ID); err != nil {
				return err
			}
			_ = m.reload()
			for _, row := range m.credentials {
				if row.ID == cred.ID {
					if !row.Disabled {
						return fmt.Errorf("upstream credential disable did not mark disabled")
					}
					return nil
				}
			}
		}
	}
	return fmt.Errorf("check upstream credential missing")
}

func ExerciseModelCacheSummary(ctx context.Context, cfg config.Config, registry provider.Registry, cache ModelCacheReader) error {
	model := newCheckModel(cfg, registry, nil, nil, cache)
	_ = model.reload()
	view := model.View()
	if !strings.Contains(view, "Model cache") || !strings.Contains(view, "deepseek 1 models") || !strings.Contains(view, "2026-05-30T12:00:00Z") {
		return fmt.Errorf("model cache summary missing")
	}
	for _, forbidden := range []string{"raw description marker", "pricing", "secret"} {
		if strings.Contains(view, forbidden) {
			return fmt.Errorf("model cache summary leaked %s", forbidden)
		}
	}
	return nil
}

func (m *Model) reload() error {
	if m.tokens == nil {
		m.tokenRows = nil
	} else {
		rows, err := m.tokens.List(context.Background())
		if err != nil {
			m.err = err.Error()
			return err
		}
		m.tokenRows = rows
	}
	m.providers = m.registry.List()
	if m.upstreams != nil {
		rows, err := m.upstreams.List(context.Background())
		if err != nil {
			m.err = err.Error()
			return err
		}
		m.credentials = rows
	}
	if m.modelCache != nil {
		rows, err := m.modelCache.ListModelCache(context.Background())
		if err != nil {
			m.err = err.Error()
			return err
		}
		m.modelRows = rows
	}
	if m.selected >= len(m.tokenRows) {
		m.selected = len(m.tokenRows) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
	return nil
}

type modelCacheSummary struct {
	ProviderInstanceID string
	Count              int
	UpdatedAt          string
}

func modelCacheSummaries(rows []provider.ModelMetadata) []modelCacheSummary {
	byProvider := map[string]modelCacheSummary{}
	for _, row := range rows {
		summary := byProvider[row.ProviderInstanceID]
		summary.ProviderInstanceID = row.ProviderInstanceID
		summary.Count++
		updated := row.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z")
		if updated > summary.UpdatedAt {
			summary.UpdatedAt = updated
		}
		byProvider[row.ProviderInstanceID] = summary
	}
	out := make([]modelCacheSummary, 0, len(byProvider))
	for _, summary := range byProvider {
		out = append(out, summary)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ProviderInstanceID < out[j].ProviderInstanceID
	})
	return out
}

func (m *Model) clearReveal() {
	m.reveal = ""
	m.revealTokenID = 0
}

func (m Model) updateAPIKeyInput(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		m.clearAPIKeyInput()
		if key.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		return m, nil
	case tea.KeyEnter:
		apiKey := m.apiKeyInput
		providerID := m.apiKeyProvider
		m.clearAPIKeyInput()
		if apiKey == "" {
			m.err = "API key is required"
			return m, nil
		}
		if _, err := m.upstreams.AddAPIKey(context.Background(), providerID, "api key", apiKey); err != nil {
			m.err = err.Error()
			return m, nil
		}
		_ = m.reload()
		return m, nil
	case tea.KeyBackspace:
		if len(m.apiKeyInput) > 0 {
			m.apiKeyInput = m.apiKeyInput[:len(m.apiKeyInput)-1]
		}
		return m, nil
	case tea.KeyRunes:
		m.apiKeyInput += string(key.Runes)
		return m, nil
	default:
		return m, nil
	}
}

func (m *Model) clearAPIKeyInput() {
	m.apiKeyMode = false
	m.apiKeyProvider = ""
	m.apiKeyInput = ""
}

func (m *Model) addCheckUpstreamCredential() error {
	if m.upstreams == nil {
		return nil
	}
	instance, ok := firstAPIKeyProvider(m.registry)
	if !ok {
		return nil
	}
	_, err := m.upstreams.AddAPIKey(context.Background(), instance.ID, "manage-check-upstream", "sk-manage-check-upstream")
	if errors.Is(err, credentials.ErrDuplicateCredential) {
		return nil
	}
	return err
}

func (m *Model) disableFirstUpstreamCredential() error {
	if m.upstreams == nil {
		return nil
	}
	for _, cred := range m.credentials {
		if !cred.Disabled {
			return m.upstreams.Disable(context.Background(), cred.ID)
		}
	}
	return nil
}

func firstAPIKeyProvider(registry provider.Registry) (provider.Instance, bool) {
	for _, instance := range registry.List() {
		if instance.APIKey && !instance.Placeholder {
			return instance, true
		}
	}
	return provider.Instance{}, false
}
