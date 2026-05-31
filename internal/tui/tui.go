package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"ilonasin/internal/config"
	"ilonasin/internal/credentials"
	"ilonasin/internal/management"
	"ilonasin/internal/metadata"
	"ilonasin/internal/provider"
)

type Model struct {
	cfg              config.Config
	registry         provider.Registry
	snapshot         management.SnapshotClient
	tokens           management.LocalTokenClient
	upstreams        credentials.UpstreamCredentialManager
	oauth            credentials.OAuthMetadataReader
	oauthRefresh     credentials.OAuthRefreshController
	oauthLogin       credentials.OAuthDeviceLoginController
	modelCache       ModelCacheReader
	observability    ObservabilityReader
	pruner           TelemetryPruner
	logger           *slog.Logger
	now              func() time.Time
	tokenRows        []management.LocalToken
	providers        []provider.Instance
	credentials      []credentials.UpstreamCredentialMetadata
	fallbackPolicies []credentials.FallbackPolicyMetadata
	oauthRows        []credentials.OAuthCredentialMetadata
	accountRows      []credentials.ProviderAccountMetadata
	modelRows        []provider.ModelMetadata
	requestRows      []metadata.RequestSummary
	usageRows        []metadata.UsageSummary
	latencyRows      []metadata.LatencySummary
	streamRows       []metadata.StreamSummary
	healthRows       []metadata.HealthSummary
	fallbackRows     []metadata.FallbackSummary
	pruneResult      *metadata.PruneResult
	pruningAvailable bool
	selected         int
	oauthSelected    int
	reveal           string
	revealTokenID    int64
	apiKeyMode       bool
	apiKeyProvider   string
	apiKeyInput      string
	oauthChallenge   *credentials.OAuthDeviceLoginChallenge
	oauthCtx         context.Context
	oauthCancel      context.CancelFunc
	err              string
	quitOnInit       bool
	checkMode        bool
}

type ModelCacheReader interface {
	ListModelCache(ctx context.Context) ([]provider.ModelMetadata, error)
}

type ObservabilityReader interface {
	RecentRequests(ctx context.Context, limit int) ([]metadata.RequestSummary, error)
	UsageByProvider(ctx context.Context) ([]metadata.UsageSummary, error)
	LatencyByProvider(ctx context.Context) ([]metadata.LatencySummary, error)
	StreamSummary(ctx context.Context) ([]metadata.StreamSummary, error)
	LatestHealth(ctx context.Context) ([]metadata.HealthSummary, error)
	RecentFallbacks(ctx context.Context, limit int) ([]metadata.FallbackSummary, error)
}

type TelemetryPruner interface {
	PruneTelemetryBefore(ctx context.Context, cutoff time.Time) (metadata.PruneResult, error)
}

func NewModel(cfg config.Config, registry provider.Registry, tokens management.LocalTokenClient, upstreams credentials.UpstreamCredentialManager, oauth credentials.OAuthMetadataReader, oauthRefresh credentials.OAuthRefreshController, oauthLogin credentials.OAuthDeviceLoginController, modelCache ModelCacheReader, observability ObservabilityReader, pruner TelemetryPruner, now func() time.Time, loggers ...*slog.Logger) Model {
	return Model{cfg: cfg, registry: registry, tokens: tokens, upstreams: upstreams, oauth: oauth, oauthRefresh: oauthRefresh, oauthLogin: oauthLogin, modelCache: modelCache, observability: observability, pruner: pruner, now: now, logger: firstLogger(loggers)}
}

func newCheckModel(cfg config.Config, registry provider.Registry, tokens management.LocalTokenClient, upstreams credentials.UpstreamCredentialManager, oauth credentials.OAuthMetadataReader, oauthRefresh credentials.OAuthRefreshController, oauthLogin credentials.OAuthDeviceLoginController, modelCache ModelCacheReader, observability ObservabilityReader, pruner TelemetryPruner, now func() time.Time, loggers ...*slog.Logger) Model {
	return Model{cfg: cfg, registry: registry, tokens: tokens, upstreams: upstreams, oauth: oauth, oauthRefresh: oauthRefresh, oauthLogin: oauthLogin, modelCache: modelCache, observability: observability, pruner: pruner, now: now, logger: firstLogger(loggers), quitOnInit: true, checkMode: true}
}

func (m Model) Init() tea.Cmd {
	if m.quitOnInit {
		return tea.Quit
	}
	return nil
}

type oauthLoginStartedMsg struct {
	challenge credentials.OAuthDeviceLoginChallenge
	err       error
}

type oauthLoginCompletedMsg struct {
	err error
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case oauthLoginStartedMsg:
		if msg.err != nil {
			m.logError(context.Background(), "tui_oauth_login_start_failed", msg.err)
			m.err = oauthLoginErrorMessage(msg.err)
			m.oauthChallenge = nil
			m.cancelOAuthLogin()
			return m, nil
		}
		m.logInfo(context.Background(), "tui_oauth_login_started")
		m.err = ""
		challenge := msg.challenge
		m.oauthChallenge = &challenge
		return m, m.completeOAuthLoginCmd(challenge.Handle)
	case oauthLoginCompletedMsg:
		m.oauthChallenge = nil
		m.cancelOAuthLogin()
		if msg.err != nil {
			m.logError(context.Background(), "tui_oauth_login_complete_failed", msg.err)
			m.err = oauthLoginErrorMessage(msg.err)
			return m, nil
		}
		m.logInfo(context.Background(), "tui_oauth_login_completed")
		_ = m.reload()
		return m, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		if m.apiKeyMode {
			return m.updateAPIKeyInput(key)
		}
		switch key.String() {
		case "q", "ctrl+c":
			m.clearReveal()
			m.cancelOAuthLogin()
			return m, tea.Quit
		case "n":
			m.clearReveal()
			created, err := m.tokens.CreateLocalToken(context.Background(), management.CreateLocalTokenRequest{Label: "local client"})
			if err != nil {
				m.logError(context.Background(), "tui_local_token_create_failed", err)
				m.err = err.Error()
				return m, nil
			}
			m.logInfo(context.Background(), "tui_local_token_created", slog.Int64("local_id", created.Metadata.ID))
			m.reveal = created.Token
			m.revealTokenID = created.Metadata.ID
			_ = m.reload()
		case "d":
			m.clearReveal()
			if len(m.tokenRows) == 0 {
				return m, nil
			}
			if _, err := m.tokens.DisableLocalToken(context.Background(), management.DisableLocalTokenRequest{ID: m.tokenRows[m.selected].ID}); err != nil {
				m.logError(context.Background(), "tui_local_token_disable_failed", err, slog.Int64("local_id", m.tokenRows[m.selected].ID))
				m.err = err.Error()
				return m, nil
			}
			m.logInfo(context.Background(), "tui_local_token_disabled", slog.Int64("local_id", m.tokenRows[m.selected].ID))
			_ = m.reload()
		case "x":
			m.clearReveal()
			if err := m.disableFirstUpstreamCredential(); err != nil {
				m.logError(context.Background(), "tui_upstream_credential_disable_failed", err)
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
				m.logError(context.Background(), "tui_upstream_credential_create_failed", err)
				m.err = err.Error()
				return m, nil
			}
			_ = m.reload()
		case "p":
			m.clearReveal()
			if err := m.pruneTelemetry(); err != nil {
				m.logError(context.Background(), "tui_telemetry_prune_failed", err)
				m.err = "telemetry prune failed"
				return m, nil
			}
			_ = m.reload()
		case "l":
			m.clearReveal()
			m.cancelOAuthLogin()
			providerID, ok := firstOAuthLoginProvider(m.registry)
			if !ok || m.oauthLogin == nil {
				m.logInfo(context.Background(), "tui_oauth_login_unavailable")
				m.err = "OAuth login failed"
				return m, nil
			}
			loginCtx, cancel := context.WithCancel(context.Background())
			m.oauthCtx = loginCtx
			m.oauthCancel = cancel
			return m, m.startOAuthLoginCmd(loginCtx, providerID)
		case "r":
			m.clearReveal()
			if err := m.refreshSelectedOAuthCredential(); err != nil {
				m.logError(context.Background(), "tui_oauth_refresh_failed", err)
				m.err = "OAuth refresh failed"
				return m, nil
			}
			_ = m.reload()
		case "o":
			m.clearReveal()
			if len(m.oauthRows) > 0 {
				m.oauthSelected = (m.oauthSelected + 1) % len(m.oauthRows)
			}
		case "f":
			m.clearReveal()
			if err := m.enableFirstFallbackPolicy(); err != nil {
				m.logError(context.Background(), "tui_fallback_policy_update_failed", err)
				m.err = "fallback policy update failed"
				return m, nil
			}
			_ = m.reload()
		case "F":
			m.clearReveal()
			if err := m.disableFirstFallbackPolicy(); err != nil {
				m.logError(context.Background(), "tui_fallback_policy_update_failed", err)
				m.err = "fallback policy update failed"
				return m, nil
			}
			_ = m.reload()
		case "esc":
			m.clearReveal()
			m.oauthChallenge = nil
			m.cancelOAuthLogin()
		case "enter":
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
		oauth := "oauth disabled"
		if instance.OAuth {
			oauth = "oauth"
		}
		fmt.Fprintf(&b, "- %s %s %s %s %s\n", instance.ID, instance.Type, instance.BaseURL, apiKey, oauth)
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
		fmt.Fprintf(&b, "- %d %s %s %s...%s group %s %s\n", cred.ID, safeDisplay(cred.ProviderInstanceID),
			safeDisplay(cred.Label), safeDisplay(cred.SecretPrefix), safeDisplay(cred.SecretLast4),
			safeDisplay(cred.FallbackGroup), state)
	}
	m.writeFallbackPolicies(&b)
	m.writeOAuth(&b)
	b.WriteString("\nModel cache\n")
	summaries := modelCacheSummaries(m.modelRows)
	if len(summaries) == 0 {
		b.WriteString("No cached models.\n")
	}
	for _, summary := range summaries {
		fmt.Fprintf(&b, "- %s %d models updated %s\n", summary.ProviderInstanceID, summary.Count, summary.UpdatedAt)
	}
	m.writeObservability(&b)
	m.writePruning(&b)
	b.WriteString("\nPress n to create local token, a to add API key, d to disable local token, x to disable API key, l to login OAuth, o/r to select/refresh OAuth, f/F to toggle fallback, p to prune telemetry, q to quit.\n")
	return b.String()
}

func Run(cfg config.Config, registry provider.Registry, snapshot management.SnapshotClient, tokens management.LocalTokenClient, upstreams credentials.UpstreamCredentialManager, oauth credentials.OAuthMetadataReader, modelCache ModelCacheReader, observability ObservabilityReader, pruner TelemetryPruner, loggers ...*slog.Logger) error {
	if snapshot == nil {
		return fmt.Errorf("management snapshot client is required")
	}
	model := NewModel(cfg, registry, tokens, upstreams, oauth, oauthRefreshController(oauth), oauthLoginController(oauth), modelCache, observability, pruner, nil, loggers...)
	model.snapshot = snapshot
	if err := model.reload(); err != nil {
		return err
	}
	_, err := tea.NewProgram(model).Run()
	return err
}

func oauthRefreshController(oauth credentials.OAuthMetadataReader) credentials.OAuthRefreshController {
	controller, _ := oauth.(credentials.OAuthRefreshController)
	return controller
}

func oauthLoginController(oauth credentials.OAuthMetadataReader) credentials.OAuthDeviceLoginController {
	controller, _ := oauth.(credentials.OAuthDeviceLoginController)
	return controller
}

func Check(cfg config.Config, registry provider.Registry, snapshot management.SnapshotClient, tokens management.LocalTokenClient, upstreams credentials.UpstreamCredentialManager, oauth credentials.OAuthMetadataReader, modelCache ModelCacheReader, observability ObservabilityReader, pruner TelemetryPruner, out io.Writer, loggers ...*slog.Logger) error {
	if snapshot == nil {
		return fmt.Errorf("management snapshot client is required")
	}
	model := newCheckModel(cfg, registry, tokens, upstreams, oauth, oauthRefreshController(oauth), oauthLoginController(oauth), modelCache, observability, pruner, nil, loggers...)
	model.snapshot = snapshot
	if err := model.reload(); err != nil {
		return err
	}
	program := tea.NewProgram(model, tea.WithoutRenderer(), tea.WithInput(nil), tea.WithOutput(io.Discard))
	if _, err := program.Run(); err != nil {
		return err
	}
	_, err := io.WriteString(out, model.View())
	return err
}

func ExerciseTokenLifecycle(ctx context.Context, tokens management.LocalTokenClient) error {
	model := NewModel(config.Config{}, provider.Registry{}, tokens, nil, nil, nil, nil, nil, nil, nil, nil)
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
	model := newCheckModel(cfg, registry, nil, upstreams, nil, nil, nil, nil, nil, nil, nil)
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

func ExerciseFallbackPolicyLifecycle(ctx context.Context, cfg config.Config, registry provider.Registry, upstreams credentials.UpstreamCredentialManager, resolver credentials.UpstreamCredentialResolver) error {
	instance, ok := firstAPIKeyProvider(registry)
	if !ok {
		return nil
	}
	model := newCheckModel(cfg, registry, nil, upstreams, nil, nil, nil, nil, nil, nil, nil)
	_ = model.reload()
	view := model.View()
	if !strings.Contains(view, "Fallback policies") ||
		!strings.Contains(view, instance.ID+" default disabled credentials 2") {
		return fmt.Errorf("fallback policy summary missing")
	}
	for _, forbidden := range []string{
		"sk-fallback-policy",
		"raw-provider-payload",
		"prompt marker",
		"secret",
		"acct_",
		"stale-provider",
		"codex default",
	} {
		if strings.Contains(view, forbidden) {
			return fmt.Errorf("fallback policy summary leaked forbidden marker")
		}
	}
	resolved, err := resolver.ResolveAPIKeys(ctx, instance.ID)
	if err != nil {
		return err
	}
	if len(resolved) != 1 {
		return fmt.Errorf("fallback policy default resolved %d credentials", len(resolved))
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	m := updated.(Model)
	if !fallbackPolicyEnabled(m.fallbackPolicies, instance.ID, credentials.CredentialKindAPIKey, credentials.DefaultFallbackGroup) {
		return fmt.Errorf("fallback policy enable did not update view")
	}
	resolved, err = resolver.ResolveAPIKeys(ctx, instance.ID)
	if err != nil {
		return err
	}
	if len(resolved) != 2 {
		return fmt.Errorf("fallback policy enable resolved %d credentials", len(resolved))
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'F'}})
	m = updated.(Model)
	if fallbackPolicyEnabled(m.fallbackPolicies, instance.ID, credentials.CredentialKindAPIKey, credentials.DefaultFallbackGroup) {
		return fmt.Errorf("fallback policy disable did not update view")
	}
	resolved, err = resolver.ResolveAPIKeys(ctx, instance.ID)
	if err != nil {
		return err
	}
	if len(resolved) != 1 {
		return fmt.Errorf("fallback policy disable resolved %d credentials", len(resolved))
	}
	failing := newCheckModel(cfg, registry, nil, failingFallbackPolicyManager{}, nil, nil, nil, nil, nil, nil, nil)
	_ = failing.reload()
	updated, _ = failing.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	failed := updated.(Model)
	failedView := failed.View()
	if !strings.Contains(failedView, "Error: fallback policy update failed") {
		return fmt.Errorf("fallback policy failure message missing")
	}
	if strings.Contains(failedView, "sk-fallback-policy") || strings.Contains(failedView, "raw-provider-payload") {
		return fmt.Errorf("fallback policy failure leaked forbidden marker")
	}
	return nil
}

func ExerciseOAuthFallbackPolicySummary(ctx context.Context, cfg config.Config, registry provider.Registry, upstreams credentials.UpstreamCredentialManager) error {
	codexID := ""
	for _, instance := range registry.List() {
		if instance.Type == "codex" && instance.OAuth {
			codexID = instance.ID
			break
		}
	}
	if codexID == "" {
		return nil
	}
	model := newCheckModel(cfg, registry, nil, upstreams, nil, nil, nil, nil, nil, nil, nil)
	_ = model.reload()
	view := model.View()
	if !strings.Contains(view, codexID+" default disabled credentials 2") {
		return fmt.Errorf("codex oauth fallback policy summary missing")
	}
	for _, forbidden := range []string{
		"oauth-fallback-policy",
		"codex-fallback-policy",
		"refresh-primary",
		"refresh-secondary",
	} {
		if strings.Contains(view, forbidden) {
			return fmt.Errorf("codex oauth fallback policy summary leaked forbidden marker")
		}
	}
	return nil
}

type failingFallbackPolicyManager struct{}

func (failingFallbackPolicyManager) AddAPIKey(context.Context, string, string, string) (credentials.UpstreamCredentialMetadata, error) {
	return credentials.UpstreamCredentialMetadata{}, fmt.Errorf("sk-fallback-policy raw-provider-payload")
}

func (failingFallbackPolicyManager) List(context.Context) ([]credentials.UpstreamCredentialMetadata, error) {
	return nil, nil
}

func (failingFallbackPolicyManager) ListFallbackPolicies(context.Context) ([]credentials.FallbackPolicyMetadata, error) {
	return []credentials.FallbackPolicyMetadata{{
		ProviderInstanceID: "deepseek",
		CredentialKind:     credentials.CredentialKindAPIKey,
		GroupLabel:         credentials.DefaultFallbackGroup,
		CredentialCount:    2,
	}}, nil
}

func (failingFallbackPolicyManager) Disable(context.Context, int64) error {
	return fmt.Errorf("sk-fallback-policy raw-provider-payload")
}

func (failingFallbackPolicyManager) EnableFallbackGroup(context.Context, string, string, string) error {
	return fmt.Errorf("sk-fallback-policy raw-provider-payload")
}

func (failingFallbackPolicyManager) DisableFallbackGroup(context.Context, string, string, string) error {
	return fmt.Errorf("sk-fallback-policy raw-provider-payload")
}

func ExerciseModelCacheSummary(ctx context.Context, cfg config.Config, registry provider.Registry, cache ModelCacheReader) error {
	model := newCheckModel(cfg, registry, nil, nil, nil, nil, nil, cache, nil, nil, nil)
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

func ExerciseObservabilitySummary(ctx context.Context, cfg config.Config, registry provider.Registry, observability ObservabilityReader) error {
	model := newCheckModel(cfg, registry, nil, nil, nil, nil, nil, nil, observability, nil, nil)
	_ = model.reload()
	view := model.View()
	required := []string{
		"Recent requests",
		"Usage totals",
		"Latency",
		"Streams",
		"Health",
		"Fallbacks",
		"deepseek 2 req prompt 11 completion 7 total 18 reasoning 3 cache_hit 5 cache_write 3 cost_microunits 126",
		"avg latency 125ms ttft 50ms tps 9.00",
		"completed 1 streams 3 chunks",
		"deepseek/deepseek-router -> deepseek/deepseek-v4-pro status 200",
		"deepseek/models upstream_failure",
		"retry_after 2026-05-30T12:10:00Z",
		"availability_retry",
	}
	for _, text := range required {
		if !strings.Contains(view, text) {
			return fmt.Errorf("observability summary missing %q", text)
		}
	}
	for _, forbidden := range []string{
		"sk-observe-secret",
		"Bearer ",
		"iln_observe",
		"raw-provider-payload",
		"prompt marker",
		"completion marker",
		"acct_",
		"req_",
		"eyJ",
	} {
		if strings.Contains(view, forbidden) {
			return fmt.Errorf("observability summary leaked %q", forbidden)
		}
	}
	return nil
}

func ExerciseTelemetryPrune(ctx context.Context, cfg config.Config, registry provider.Registry, observability ObservabilityReader, pruner TelemetryPruner, now func() time.Time, expected metadata.PruneResult) error {
	model := newCheckModel(cfg, registry, nil, nil, nil, nil, nil, nil, observability, pruner, now)
	_ = model.reload()
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	m := updated.(Model)
	if m.pruneResult == nil {
		return fmt.Errorf("telemetry prune result missing")
	}
	got := *m.pruneResult
	if !got.Cutoff.Equal(expected.Cutoff) || got.Requests != expected.Requests ||
		got.Streams != expected.Streams || got.Fallbacks != expected.Fallbacks ||
		got.Health != expected.Health {
		return fmt.Errorf("telemetry prune result mismatch got=%+v want=%+v", got, expected)
	}
	view := m.View()
	for _, text := range []string{
		"Telemetry pruning",
		"Retention keep forever until pruned.",
		"Manual prune cutoff older than 30 days.",
		fmt.Sprintf("Last prune before %s: requests %d streams %d fallbacks %d health %d",
			formatPreciseTime(expected.Cutoff), expected.Requests, expected.Streams, expected.Fallbacks, expected.Health),
	} {
		if !strings.Contains(view, text) {
			return fmt.Errorf("telemetry prune summary missing %q", text)
		}
	}
	for _, forbidden := range []string{
		"sk-prune-secret",
		"Bearer ",
		"raw-provider-payload",
		"prompt marker",
		"completion marker",
		"body marker",
		"acct_",
		"req_",
		"balance",
		"credit",
	} {
		if strings.Contains(view, forbidden) {
			return fmt.Errorf("telemetry prune summary leaked forbidden marker")
		}
	}
	failing := newCheckModel(cfg, registry, nil, nil, nil, nil, nil, nil, observability, failingTelemetryPruner{}, now)
	updated, _ = failing.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	failed := updated.(Model)
	failedView := failed.View()
	if !strings.Contains(failedView, "Error: telemetry prune failed") {
		return fmt.Errorf("telemetry prune failure message missing")
	}
	if strings.Contains(failedView, "sk-prune-secret") || strings.Contains(failedView, "raw-provider-payload") {
		return fmt.Errorf("telemetry prune failure leaked forbidden marker")
	}
	return nil
}

type failingTelemetryPruner struct{}

func (failingTelemetryPruner) PruneTelemetryBefore(context.Context, time.Time) (metadata.PruneResult, error) {
	return metadata.PruneResult{}, fmt.Errorf("sk-prune-secret raw-provider-payload")
}

func ExerciseOAuthSummary(ctx context.Context, cfg config.Config, registry provider.Registry, oauth credentials.OAuthMetadataReader) error {
	model := newCheckModel(cfg, registry, nil, nil, oauth, nil, nil, nil, nil, nil, nil)
	_ = model.reload()
	view := model.View()
	for _, text := range []string{
		"OAuth accounts",
		"codex oauth account Codex Safe plan team expires 2026-05-30T13:00:00Z refresh refresh_token_expired",
		"Provider accounts",
		"codex credential 1 Codex Safe plan team",
	} {
		if !strings.Contains(view, text) {
			return fmt.Errorf("oauth summary missing %q", text)
		}
	}
	for _, forbidden := range []string{
		"oauth-access-secret-marker",
		"oauth-refresh-secret-marker",
		"acct_raw_forbidden",
		"eyJ",
		"callback",
		"Authorization:",
		"Bearer ",
		"token_endpoint_body",
		"cookie",
		"stdout",
		"req_",
	} {
		if strings.Contains(view, forbidden) {
			return fmt.Errorf("oauth summary leaked %q", forbidden)
		}
	}
	return nil
}

func ExerciseOAuthRefresh(ctx context.Context, cfg config.Config, registry provider.Registry, oauth credentials.OAuthMetadataReader, refresher credentials.OAuthRefreshController) error {
	model := newCheckModel(cfg, registry, nil, nil, oauth, refresher, nil, nil, nil, nil, nil)
	_ = model.reload()
	if len(model.oauthRows) < 2 {
		return fmt.Errorf("oauth refresh check needs at least two oauth credentials")
	}
	model.oauthSelected = 1
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m := updated.(Model)
	if m.err != "" {
		return fmt.Errorf("oauth refresh update failed")
	}
	if len(m.oauthRows) < 2 || m.oauthRows[1].ExpiresAt == nil {
		return fmt.Errorf("oauth refresh did not update selected row")
	}
	view := m.View()
	for _, forbidden := range []string{
		"oauth-refresh",
		"oauth-access",
		"token_endpoint_body",
		"raw-provider-payload",
		"acct_",
		"request_id",
		"balance",
		"credit",
		"id-token-drop-marker",
	} {
		if strings.Contains(view, forbidden) {
			return fmt.Errorf("oauth refresh view leaked forbidden marker")
		}
	}
	failing := newCheckModel(cfg, registry, nil, nil, oauth, failingOAuthRefreshController{}, nil, nil, nil, nil, nil)
	_ = failing.reload()
	failing.oauthSelected = 1
	updated, _ = failing.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	failed := updated.(Model)
	failedView := failed.View()
	if !strings.Contains(failedView, "Error: OAuth refresh failed") {
		return fmt.Errorf("oauth refresh failure message missing")
	}
	if strings.Contains(failedView, "oauth-refresh") || strings.Contains(failedView, "raw-provider-payload") {
		return fmt.Errorf("oauth refresh failure leaked forbidden marker")
	}
	return nil
}

type failingOAuthRefreshController struct{}

func (failingOAuthRefreshController) RefreshOAuthCredential(context.Context, int64) error {
	return fmt.Errorf("oauth-refresh raw-provider-payload token_endpoint_body")
}

func ExerciseOAuthDeviceLogin(ctx context.Context, cfg config.Config, registry provider.Registry, oauth credentials.OAuthMetadataReader, login credentials.OAuthDeviceLoginController) error {
	model := newCheckModel(cfg, registry, nil, nil, oauth, nil, login, nil, nil, nil, nil)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if cmd == nil {
		return fmt.Errorf("oauth login did not start")
	}
	msg := cmd()
	updated, cmd = updated.(Model).Update(msg)
	m := updated.(Model)
	if m.err != "" || m.oauthChallenge == nil || cmd == nil {
		return fmt.Errorf("oauth login challenge missing")
	}
	view := m.View()
	if !strings.Contains(view, "/codex/device") || !strings.Contains(view, "USER-CODE") {
		return fmt.Errorf("oauth login challenge view missing")
	}
	for _, forbidden := range []string{
		"device-auth-marker",
		"authorization-code-marker",
		"code-verifier-marker",
		"oauth-login-access-marker",
		"oauth-login-refresh-marker",
		"id-token-marker",
		"acct_device_raw",
		"Bearer ",
		"cookie",
	} {
		if strings.Contains(view, forbidden) {
			return fmt.Errorf("oauth login challenge leaked %q", forbidden)
		}
	}
	msg = cmd()
	updated, _ = m.Update(msg)
	m = updated.(Model)
	if m.err != "" || m.oauthChallenge != nil {
		return fmt.Errorf("oauth login did not complete")
	}
	_ = m.reload()
	view = m.View()
	if !strings.Contains(view, "Codex Login") || !strings.Contains(view, "plan pro") {
		return fmt.Errorf("oauth login row missing")
	}
	for _, forbidden := range []string{
		"device-auth-marker",
		"authorization-code-marker",
		"code-verifier-marker",
		"oauth-login-access-marker",
		"oauth-login-refresh-marker",
		"id-token-marker",
		"acct_device_raw",
		"token_endpoint_body",
	} {
		if strings.Contains(view, forbidden) {
			return fmt.Errorf("oauth login view leaked %q", forbidden)
		}
	}
	failing := newCheckModel(cfg, registry, nil, nil, oauth, nil, failingOAuthLoginController{}, nil, nil, nil, nil)
	updated, cmd = failing.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if cmd != nil {
		updated, _ = updated.(Model).Update(cmd())
	}
	if !strings.Contains(updated.(Model).View(), "Error: OAuth login failed") {
		return fmt.Errorf("oauth login failure message missing")
	}
	eventFailing := newCheckModel(cfg, registry, nil, nil, oauth, nil, eventOAuthLoginController{}, nil, nil, nil, nil)
	updated, cmd = eventFailing.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if cmd != nil {
		updated, _ = updated.(Model).Update(cmd())
	}
	if !strings.Contains(updated.(Model).View(), "Error: OAuth login failed: oauth_login_http_error event_id=event-check") {
		return fmt.Errorf("oauth login event id message missing")
	}
	cancelAware := &cancelAwareOAuthLoginController{}
	cancelModel := newCheckModel(cfg, registry, nil, nil, nil, nil, cancelAware, nil, nil, nil, nil)
	updated, cmd = cancelModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if cmd == nil {
		return fmt.Errorf("oauth login cancel check did not start")
	}
	updated, cmd = updated.(Model).Update(cmd())
	if cmd == nil {
		return fmt.Errorf("oauth login cancel check did not begin completion")
	}
	updated, quitCmd := updated.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if quitCmd == nil {
		return fmt.Errorf("oauth login cancel check did not quit")
	}
	_ = updated
	_ = cmd()
	if !cancelAware.canceled {
		return fmt.Errorf("oauth login completion was not canceled")
	}
	return nil
}

func ExerciseOAuthDeviceLoginFailure(ctx context.Context, cfg config.Config, registry provider.Registry, oauth credentials.OAuthMetadataReader, login credentials.OAuthDeviceLoginController, wantClass string) (string, error) {
	model := newCheckModel(cfg, registry, nil, nil, oauth, nil, login, nil, nil, nil, nil)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if cmd == nil {
		return "", fmt.Errorf("oauth login failure check did not start")
	}
	updated, cmd = updated.(Model).Update(cmd())
	if cmd == nil {
		return "", fmt.Errorf("oauth login failure check did not begin completion")
	}
	updated, _ = updated.(Model).Update(cmd())
	view := updated.(Model).View()
	needle := "Error: OAuth login failed: " + wantClass + " event_id="
	start := strings.Index(view, needle)
	if start < 0 {
		return "", fmt.Errorf("oauth login failure event id message missing")
	}
	eventID := view[start+len(needle):]
	if idx := strings.IndexByte(eventID, '\n'); idx >= 0 {
		eventID = eventID[:idx]
	}
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return "", fmt.Errorf("oauth login failure event id empty")
	}
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	return eventID, nil
}

type failingOAuthLoginController struct{}

func (failingOAuthLoginController) StartOAuthDeviceLogin(context.Context, string) (credentials.OAuthDeviceLoginChallenge, error) {
	return credentials.OAuthDeviceLoginChallenge{}, fmt.Errorf("oauth-login-access-marker raw-provider-payload")
}

func (failingOAuthLoginController) CompleteOAuthDeviceLogin(context.Context, string) (credentials.OAuthCredentialMetadata, error) {
	return credentials.OAuthCredentialMetadata{}, fmt.Errorf("oauth-login-refresh-marker raw-provider-payload")
}

type eventOAuthLoginController struct{}

func (eventOAuthLoginController) StartOAuthDeviceLogin(context.Context, string) (credentials.OAuthDeviceLoginChallenge, error) {
	return credentials.OAuthDeviceLoginChallenge{}, provider.OAuthDeviceLoginError{Class: "oauth_login_http_error", EventID: "event-check"}
}

func (eventOAuthLoginController) CompleteOAuthDeviceLogin(context.Context, string) (credentials.OAuthCredentialMetadata, error) {
	return credentials.OAuthCredentialMetadata{}, nil
}

type cancelAwareOAuthLoginController struct {
	canceled bool
}

func (c *cancelAwareOAuthLoginController) StartOAuthDeviceLogin(context.Context, string) (credentials.OAuthDeviceLoginChallenge, error) {
	return credentials.OAuthDeviceLoginChallenge{
		ProviderInstanceID: "codex",
		VerificationURL:    "https://auth.example/codex/device",
		UserCode:           "USER-CODE",
		Handle:             "safe-handle",
	}, nil
}

func (c *cancelAwareOAuthLoginController) CompleteOAuthDeviceLogin(ctx context.Context, _ string) (credentials.OAuthCredentialMetadata, error) {
	select {
	case <-ctx.Done():
		c.canceled = true
		return credentials.OAuthCredentialMetadata{}, ctx.Err()
	case <-time.After(50 * time.Millisecond):
		return credentials.OAuthCredentialMetadata{}, nil
	}
}

func (m *Model) reload() error {
	if m.snapshot != nil {
		snapshot, err := m.snapshot.LoadManagementSnapshot(context.Background())
		if err != nil {
			m.err = err.Error()
			return err
		}
		m.applySnapshot(snapshot)
		return nil
	}
	return m.reloadDirect()
}

func (m *Model) reloadDirect() error {
	if m.tokens == nil {
		m.tokenRows = nil
	} else {
		rows, err := m.tokens.ListLocalTokens(context.Background())
		if err != nil {
			m.err = err.Error()
			return err
		}
		m.tokenRows = rows.Tokens
	}
	m.providers = m.registry.List()
	if m.upstreams != nil {
		rows, err := m.upstreams.List(context.Background())
		if err != nil {
			m.err = err.Error()
			return err
		}
		m.credentials = m.visibleUpstreamCredentials(rows)
		policies, err := m.upstreams.ListFallbackPolicies(context.Background())
		if err != nil {
			m.err = err.Error()
			return err
		}
		m.fallbackPolicies = m.visibleFallbackPolicies(policies)
	}
	if m.oauth != nil {
		oauthRows, err := m.oauth.ListOAuthCredentials(context.Background())
		if err != nil {
			m.err = err.Error()
			return err
		}
		m.oauthRows = oauthRows
		accountRows, err := m.oauth.ListProviderAccounts(context.Background())
		if err != nil {
			m.err = err.Error()
			return err
		}
		m.accountRows = accountRows
	}
	if m.modelCache != nil {
		rows, err := m.modelCache.ListModelCache(context.Background())
		if err != nil {
			m.err = err.Error()
			return err
		}
		m.modelRows = rows
	}
	if m.observability != nil {
		requestRows, err := m.observability.RecentRequests(context.Background(), 5)
		if err != nil {
			m.err = err.Error()
			return err
		}
		m.requestRows = requestRows
		usageRows, err := m.observability.UsageByProvider(context.Background())
		if err != nil {
			m.err = err.Error()
			return err
		}
		m.usageRows = usageRows
		latencyRows, err := m.observability.LatencyByProvider(context.Background())
		if err != nil {
			m.err = err.Error()
			return err
		}
		m.latencyRows = latencyRows
		streamRows, err := m.observability.StreamSummary(context.Background())
		if err != nil {
			m.err = err.Error()
			return err
		}
		m.streamRows = streamRows
		healthRows, err := m.observability.LatestHealth(context.Background())
		if err != nil {
			m.err = err.Error()
			return err
		}
		m.healthRows = healthRows
		fallbackRows, err := m.observability.RecentFallbacks(context.Background(), 5)
		if err != nil {
			m.err = err.Error()
			return err
		}
		m.fallbackRows = fallbackRows
	}
	if m.selected >= len(m.tokenRows) {
		m.selected = len(m.tokenRows) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
	if m.oauthSelected >= len(m.oauthRows) {
		m.oauthSelected = len(m.oauthRows) - 1
	}
	if m.oauthSelected < 0 {
		m.oauthSelected = 0
	}
	return nil
}

func (m *Model) applySnapshot(snapshot management.ManagementSnapshotResponse) {
	m.tokenRows = snapshot.LocalTokens
	m.providers = providersFromSnapshot(snapshot.Providers)
	m.credentials = upstreamCredentialsFromSnapshot(snapshot.UpstreamCredentials)
	m.fallbackPolicies = fallbackPoliciesFromSnapshot(snapshot.FallbackPolicies)
	m.oauthRows = oauthCredentialsFromSnapshot(snapshot.OAuthCredentials)
	m.accountRows = providerAccountsFromSnapshot(snapshot.ProviderAccounts)
	m.modelRows = modelMetadataFromSnapshot(snapshot.ModelCache)
	m.requestRows = requestSummariesFromSnapshot(snapshot.RecentRequests)
	m.usageRows = usageSummariesFromSnapshot(snapshot.Usage)
	m.latencyRows = latencySummariesFromSnapshot(snapshot.Latency)
	m.streamRows = streamSummariesFromSnapshot(snapshot.Streams)
	m.healthRows = healthSummariesFromSnapshot(snapshot.Health)
	m.fallbackRows = fallbackSummariesFromSnapshot(snapshot.Fallbacks)
	m.pruningAvailable = snapshot.PruningAvailable
	m.credentials = m.visibleUpstreamCredentials(m.credentials)
	m.fallbackPolicies = m.visibleFallbackPolicies(m.fallbackPolicies)
	if m.selected >= len(m.tokenRows) {
		m.selected = len(m.tokenRows) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
	if m.oauthSelected >= len(m.oauthRows) {
		m.oauthSelected = len(m.oauthRows) - 1
	}
	if m.oauthSelected < 0 {
		m.oauthSelected = 0
	}
}

func providersFromSnapshot(rows []management.ProviderInstance) []provider.Instance {
	out := make([]provider.Instance, 0, len(rows))
	for _, row := range rows {
		out = append(out, provider.Instance{
			ID:             row.ID,
			Type:           row.Type,
			BaseURL:        row.BaseURL,
			AuthStyle:      row.AuthStyle,
			Placeholder:    row.Placeholder,
			APIKey:         row.APIKey,
			OAuth:          row.OAuth,
			OAuthRefresh:   row.OAuthRefresh,
			Chat:           row.Chat,
			ModelDiscovery: row.ModelDiscovery,
		})
	}
	return out
}

func upstreamCredentialsFromSnapshot(rows []management.UpstreamCredential) []credentials.UpstreamCredentialMetadata {
	out := make([]credentials.UpstreamCredentialMetadata, 0, len(rows))
	for _, row := range rows {
		out = append(out, credentials.UpstreamCredentialMetadata{
			ID:                 row.ID,
			ProviderInstanceID: row.ProviderInstanceID,
			Kind:               row.Kind,
			Label:              row.Label,
			SecretPrefix:       row.SecretPrefix,
			SecretLast4:        row.SecretLast4,
			FallbackGroup:      row.FallbackGroup,
			CreatedAt:          row.CreatedAt,
			DisabledAt:         row.DisabledAt,
			Disabled:           row.Disabled,
		})
	}
	return out
}

func fallbackPoliciesFromSnapshot(rows []management.FallbackPolicy) []credentials.FallbackPolicyMetadata {
	out := make([]credentials.FallbackPolicyMetadata, 0, len(rows))
	for _, row := range rows {
		out = append(out, credentials.FallbackPolicyMetadata{
			ProviderInstanceID: row.ProviderInstanceID,
			CredentialKind:     row.CredentialKind,
			GroupLabel:         row.GroupLabel,
			Enabled:            row.Enabled,
			CredentialCount:    row.CredentialCount,
			Explicit:           row.Explicit,
		})
	}
	return out
}

func oauthCredentialsFromSnapshot(rows []management.OAuthCredential) []credentials.OAuthCredentialMetadata {
	out := make([]credentials.OAuthCredentialMetadata, 0, len(rows))
	for _, row := range rows {
		out = append(out, credentials.OAuthCredentialMetadata{
			ID:                  row.ID,
			ProviderInstanceID:  row.ProviderInstanceID,
			Label:               row.Label,
			AccountDisplayLabel: row.AccountDisplayLabel,
			PlanLabel:           row.PlanLabel,
			Scopes:              row.Scopes,
			ExpiresAt:           row.ExpiresAt,
			LastRefreshAt:       row.LastRefreshAt,
			RefreshFailureClass: row.RefreshFailureClass,
			CreatedAt:           row.CreatedAt,
			DisabledAt:          row.DisabledAt,
			Disabled:            row.Disabled,
		})
	}
	return out
}

func providerAccountsFromSnapshot(rows []management.ProviderAccount) []credentials.ProviderAccountMetadata {
	out := make([]credentials.ProviderAccountMetadata, 0, len(rows))
	for _, row := range rows {
		out = append(out, credentials.ProviderAccountMetadata{
			ID:                 row.ID,
			ProviderInstanceID: row.ProviderInstanceID,
			CredentialID:       row.CredentialID,
			DisplayLabel:       row.DisplayLabel,
			PlanLabel:          row.PlanLabel,
			CreatedAt:          row.CreatedAt,
		})
	}
	return out
}

func modelMetadataFromSnapshot(rows []management.ModelMetadata) []provider.ModelMetadata {
	out := make([]provider.ModelMetadata, 0, len(rows))
	for _, row := range rows {
		out = append(out, provider.ModelMetadata{
			ProviderInstanceID: row.ProviderInstanceID,
			ModelID:            row.ModelID,
			DisplayName:        row.DisplayName,
			CapabilityFlags:    row.Capabilities,
			UpdatedAt:          row.UpdatedAt,
		})
	}
	return out
}

func requestSummariesFromSnapshot(rows []management.RequestSummary) []metadata.RequestSummary {
	out := make([]metadata.RequestSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, metadata.RequestSummary{
			ID:                     row.ID,
			StartedAt:              row.StartedAt,
			ProviderInstanceID:     row.ProviderInstanceID,
			ModelID:                row.ModelID,
			RequestedProviderID:    row.RequestedProviderID,
			RequestedModelID:       row.RequestedModelID,
			ResolvedProviderID:     row.ResolvedProviderID,
			ResolvedModelID:        row.ResolvedModelID,
			CredentialID:           row.CredentialID,
			CredentialLabel:        row.CredentialLabel,
			HTTPStatus:             row.HTTPStatus,
			ErrorClass:             row.ErrorClass,
			RetryCount:             row.RetryCount,
			FallbackCount:          row.FallbackCount,
			FallbackReason:         row.FallbackReason,
			PromptTokens:           row.PromptTokens,
			CompletionTokens:       row.CompletionTokens,
			TotalTokens:            row.TotalTokens,
			ReasoningTokens:        row.ReasoningTokens,
			CacheHitTokens:         row.CacheHitTokens,
			CacheWriteTokens:       row.CacheWriteTokens,
			CostMicrounits:         row.CostMicrounits,
			TotalLatencyMS:         row.TotalLatencyMS,
			TimeToFirstTokenMS:     row.TimeToFirstTokenMS,
			OutputTokensPerSecond:  row.OutputTokensPerSecond,
			StreamCompletionStatus: row.StreamCompletionStatus,
			StreamChunkCount:       row.StreamChunkCount,
		})
	}
	return out
}

func usageSummariesFromSnapshot(rows []management.UsageSummary) []metadata.UsageSummary {
	out := make([]metadata.UsageSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, metadata.UsageSummary{
			ProviderInstanceID: row.ProviderInstanceID,
			RequestCount:       row.RequestCount,
			PromptTokens:       row.PromptTokens,
			CompletionTokens:   row.CompletionTokens,
			TotalTokens:        row.TotalTokens,
			ReasoningTokens:    row.ReasoningTokens,
			CacheHitTokens:     row.CacheHitTokens,
			CacheWriteTokens:   row.CacheWriteTokens,
			CostMicrounits:     row.CostMicrounits,
		})
	}
	return out
}

func latencySummariesFromSnapshot(rows []management.LatencySummary) []metadata.LatencySummary {
	out := make([]metadata.LatencySummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, metadata.LatencySummary{
			ProviderInstanceID:        row.ProviderInstanceID,
			RequestCount:              row.RequestCount,
			AverageLatencyMS:          row.AverageLatencyMS,
			AverageTimeToFirstTokenMS: row.AverageTimeToFirstTokenMS,
			AverageOutputTPS:          row.AverageOutputTPS,
		})
	}
	return out
}

func streamSummariesFromSnapshot(rows []management.StreamSummary) []metadata.StreamSummary {
	out := make([]metadata.StreamSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, metadata.StreamSummary{
			CompletionStatus: row.CompletionStatus,
			StreamCount:      row.StreamCount,
			ChunkCount:       row.ChunkCount,
		})
	}
	return out
}

func healthSummariesFromSnapshot(rows []management.HealthSummary) []metadata.HealthSummary {
	out := make([]metadata.HealthSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, metadata.HealthSummary{
			ProviderInstanceID: row.ProviderInstanceID,
			ModelID:            row.ModelID,
			CredentialID:       row.CredentialID,
			CredentialLabel:    row.CredentialLabel,
			EventClass:         row.EventClass,
			HTTPStatus:         row.HTTPStatus,
			ErrorClass:         row.ErrorClass,
			OccurredAt:         row.OccurredAt,
			RetryAfter:         row.RetryAfter,
		})
	}
	return out
}

func fallbackSummariesFromSnapshot(rows []management.FallbackSummary) []metadata.FallbackSummary {
	out := make([]metadata.FallbackSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, metadata.FallbackSummary{
			ID:                  row.ID,
			RequestMetadataID:   row.RequestMetadataID,
			OccurredAt:          row.OccurredAt,
			ProviderInstanceID:  row.ProviderInstanceID,
			ModelID:             row.ModelID,
			FromCredentialID:    row.FromCredentialID,
			FromCredentialLabel: row.FromCredentialLabel,
			ToCredentialID:      row.ToCredentialID,
			ToCredentialLabel:   row.ToCredentialLabel,
			Reason:              row.Reason,
		})
	}
	return out
}

func (m Model) writeOAuth(b *strings.Builder) {
	if m.oauth == nil && m.snapshot == nil {
		return
	}
	b.WriteString("\nOAuth accounts\n")
	if m.oauthChallenge != nil {
		fmt.Fprintf(b, "Login %s at %s code %s\n", safeDisplay(m.oauthChallenge.ProviderInstanceID),
			safeDisplay(m.oauthChallenge.VerificationURL), safeDisplay(m.oauthChallenge.UserCode))
	}
	if len(m.oauthRows) == 0 {
		b.WriteString("No OAuth accounts.\n")
	}
	for i, row := range m.oauthRows {
		cursor := " "
		if i == m.oauthSelected {
			cursor = ">"
		}
		state := "enabled"
		if row.Disabled {
			state = "disabled"
		}
		expires := "none"
		if row.ExpiresAt != nil {
			expires = formatTime(*row.ExpiresAt)
		}
		refresh := safeRefreshFailureClass(row.RefreshFailureClass)
		if refresh == "" {
			refresh = "none"
		}
		fmt.Fprintf(b, "%s %d %s oauth account %s plan %s expires %s refresh %s %s\n",
			cursor, row.ID, safeDisplay(row.ProviderInstanceID), safeDisplay(row.AccountDisplayLabel),
			safeDisplay(row.PlanLabel), expires, refresh, state)
	}
	b.WriteString("\nProvider accounts\n")
	if len(m.accountRows) == 0 {
		b.WriteString("No provider accounts.\n")
	}
	for _, row := range m.accountRows {
		fmt.Fprintf(b, "- %s credential %d %s plan %s\n",
			safeDisplay(row.ProviderInstanceID), row.CredentialID,
			safeDisplay(row.DisplayLabel), safeDisplay(row.PlanLabel))
	}
}

func (m Model) writeFallbackPolicies(b *strings.Builder) {
	if m.upstreams == nil && m.snapshot == nil {
		return
	}
	b.WriteString("\nFallback policies\n")
	if len(m.fallbackPolicies) == 0 {
		b.WriteString("No eligible fallback groups.\n")
	}
	for _, row := range m.fallbackPolicies {
		state := "disabled"
		if row.Enabled {
			state = "enabled"
		}
		fmt.Fprintf(b, "- %s %s %s credentials %d\n",
			safeDisplay(row.ProviderInstanceID), safeDisplay(row.GroupLabel), state, row.CredentialCount)
	}
}

func (m Model) writeObservability(b *strings.Builder) {
	if m.observability == nil && m.snapshot == nil {
		return
	}
	b.WriteString("\nRecent requests\n")
	if len(m.requestRows) == 0 {
		b.WriteString("No request metadata.\n")
	}
	for _, row := range m.requestRows {
		credential := credentialDisplay(row.CredentialID, row.CredentialLabel)
		fallbackReason := ""
		if row.FallbackReason != "" {
			fallbackReason = " reason " + safeDisplay(row.FallbackReason)
		}
		fmt.Fprintf(b, "- %s %s status %d %s %s retry %d fallback %d%s latency %dms\n",
			formatTime(row.StartedAt), requestModelDisplay(row),
			row.HTTPStatus, safeDisplay(row.ErrorClass), credential, row.RetryCount,
			row.FallbackCount, fallbackReason, row.TotalLatencyMS)
	}
	b.WriteString("\nUsage totals\n")
	if len(m.usageRows) == 0 {
		b.WriteString("No usage metadata.\n")
	}
	for _, row := range m.usageRows {
		fmt.Fprintf(b, "- %s %d req prompt %d completion %d total %d reasoning %d cache_hit %d cache_write %d cost_microunits %d\n",
			safeDisplay(row.ProviderInstanceID), row.RequestCount, row.PromptTokens,
			row.CompletionTokens, row.TotalTokens, row.ReasoningTokens,
			row.CacheHitTokens, row.CacheWriteTokens, row.CostMicrounits)
	}
	b.WriteString("\nLatency\n")
	if len(m.latencyRows) == 0 {
		b.WriteString("No latency metadata.\n")
	}
	for _, row := range m.latencyRows {
		fmt.Fprintf(b, "- %s %d req avg latency %dms ttft %dms tps %.2f\n",
			safeDisplay(row.ProviderInstanceID), row.RequestCount, row.AverageLatencyMS,
			row.AverageTimeToFirstTokenMS, row.AverageOutputTPS)
	}
	b.WriteString("\nStreams\n")
	if len(m.streamRows) == 0 {
		b.WriteString("No stream metadata.\n")
	}
	for _, row := range m.streamRows {
		fmt.Fprintf(b, "- %s %d streams %d chunks\n", safeDisplay(row.CompletionStatus), row.StreamCount, row.ChunkCount)
	}
	b.WriteString("\nHealth\n")
	if len(m.healthRows) == 0 {
		b.WriteString("No health metadata.\n")
	}
	for _, row := range m.healthRows {
		retryAfter := ""
		if row.RetryAfter != nil {
			retryAfter = " retry_after " + formatTime(*row.RetryAfter)
		}
		fmt.Fprintf(b, "- %s/%s %s status %d %s %s at %s%s\n",
			safeDisplay(row.ProviderInstanceID), healthModelDisplay(row.ModelID),
			safeDisplay(row.EventClass), row.HTTPStatus, safeDisplay(row.ErrorClass),
			credentialDisplay(row.CredentialID, row.CredentialLabel), formatTime(row.OccurredAt), retryAfter)
	}
	b.WriteString("\nFallbacks\n")
	if len(m.fallbackRows) == 0 {
		b.WriteString("No fallback metadata.\n")
	}
	for _, row := range m.fallbackRows {
		fmt.Fprintf(b, "- %s %s/%s %s -> %s %s\n",
			formatTime(row.OccurredAt), safeDisplay(row.ProviderInstanceID), safeDisplay(row.ModelID),
			credentialDisplay(row.FromCredentialID, row.FromCredentialLabel),
			credentialDisplay(row.ToCredentialID, row.ToCredentialLabel), safeDisplay(row.Reason))
	}
}

func (m Model) writePruning(b *strings.Builder) {
	if m.pruner == nil && !m.pruningAvailable {
		return
	}
	b.WriteString("\nTelemetry pruning\n")
	b.WriteString("Retention keep forever until pruned.\n")
	b.WriteString("Manual prune cutoff older than 30 days.\n")
	if m.pruneResult != nil {
		fmt.Fprintf(b, "Last prune before %s: requests %d streams %d fallbacks %d health %d\n",
			formatPreciseTime(m.pruneResult.Cutoff), m.pruneResult.Requests, m.pruneResult.Streams,
			m.pruneResult.Fallbacks, m.pruneResult.Health)
	}
}

func (m *Model) pruneTelemetry() error {
	if m.pruner == nil {
		return nil
	}
	cutoff := m.nowTime().Add(-30 * 24 * time.Hour).UTC()
	result, err := m.pruner.PruneTelemetryBefore(context.Background(), cutoff)
	if err != nil {
		return err
	}
	m.pruneResult = &result
	m.logInfo(context.Background(), "tui_telemetry_pruned",
		slog.Int("requests", result.Requests),
		slog.Int("streams", result.Streams),
		slog.Int("fallbacks", result.Fallbacks),
		slog.Int("health", result.Health),
	)
	return nil
}

func (m Model) startOAuthLoginCmd(ctx context.Context, providerInstanceID string) tea.Cmd {
	return func() tea.Msg {
		challenge, err := m.oauthLogin.StartOAuthDeviceLogin(ctx, providerInstanceID)
		return oauthLoginStartedMsg{challenge: challenge, err: err}
	}
}

func (m Model) completeOAuthLoginCmd(handle string) tea.Cmd {
	return func() tea.Msg {
		ctx := m.oauthCtx
		if ctx == nil {
			ctx = context.Background()
		}
		_, err := m.oauthLogin.CompleteOAuthDeviceLogin(ctx, handle)
		return oauthLoginCompletedMsg{err: err}
	}
}

func (m *Model) cancelOAuthLogin() {
	if m.oauthCancel != nil {
		m.oauthCancel()
		m.oauthCancel = nil
	}
	m.oauthCtx = nil
}

func (m *Model) refreshSelectedOAuthCredential() error {
	if m.oauthRefresh == nil || len(m.oauthRows) == 0 {
		return nil
	}
	if m.oauthSelected < 0 || m.oauthSelected >= len(m.oauthRows) {
		return nil
	}
	row := m.oauthRows[m.oauthSelected]
	if row.Disabled {
		return nil
	}
	if err := m.oauthRefresh.RefreshOAuthCredential(context.Background(), row.ID); err != nil {
		return err
	}
	m.logInfo(context.Background(), "tui_oauth_refreshed",
		slog.String("provider_instance", row.ProviderInstanceID),
		slog.Int64("credential_id", row.ID),
	)
	return nil
}

func firstOAuthLoginProvider(registry provider.Registry) (string, bool) {
	for _, instance := range registry.List() {
		if instance.Type == "codex" && instance.OAuth {
			return instance.ID, true
		}
	}
	return "", false
}

func oauthLoginErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	var loginErr provider.OAuthDeviceLoginError
	if errors.As(err, &loginErr) && loginErr.Class != "" {
		if loginErr.EventID != "" {
			return "OAuth login failed: " + loginErr.Class + " event_id=" + loginErr.EventID
		}
		return "OAuth login failed: " + loginErr.Class
	}
	if errors.Is(err, context.Canceled) {
		return "OAuth login failed: oauth_login_canceled"
	}
	if errors.Is(err, credentials.ErrNoEligibleCredential) {
		return "OAuth login failed: oauth_login_expired"
	}
	return "OAuth login failed: " + safeErrorMessage(err.Error())
}

func safeErrorMessage(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(value))
	if value == "" {
		return "unknown"
	}
	if safeErrorMessagePattern.MatchString(value) {
		return value
	}
	return "details_redacted"
}

func (m Model) nowTime() time.Time {
	if m.now != nil {
		return m.now().UTC()
	}
	return time.Now().UTC()
}

func (m Model) logInfo(ctx context.Context, event string, attrs ...slog.Attr) {
	if m.logger == nil {
		return
	}
	all := append([]slog.Attr{slog.String("event", event)}, attrs...)
	m.logger.LogAttrs(ctx, slog.LevelInfo, "tui operation", all...)
}

func (m Model) logError(ctx context.Context, event string, err error, attrs ...slog.Attr) {
	if m.logger == nil {
		return
	}
	all := []slog.Attr{
		slog.String("event", event),
		slog.String("error_class", tuiErrorClass(err)),
	}
	all = append(all, attrs...)
	m.logger.LogAttrs(ctx, slog.LevelError, "tui operation failed", all...)
}

func tuiErrorClass(err error) string {
	if err == nil {
		return "none"
	}
	var loginErr provider.OAuthDeviceLoginError
	if errors.As(err, &loginErr) && loginErr.Class != "" {
		return loginErr.Class
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(err, credentials.ErrNoEligibleCredential) {
		return "no_eligible_credential"
	}
	return "operation_failed"
}

func firstLogger(loggers []*slog.Logger) *slog.Logger {
	if len(loggers) == 0 {
		return nil
	}
	return loggers[0]
}

func credentialDisplay(id int64, label string) string {
	if id == 0 {
		return "credential none"
	}
	safe := safeDisplay(label)
	if safe == "" || safe == "[redacted]" {
		return fmt.Sprintf("credential %d", id)
	}
	return fmt.Sprintf("credential %d %s", id, safe)
}

func healthModelDisplay(modelID string) string {
	if modelID == "" {
		return "models"
	}
	return safeDisplay(modelID)
}

func requestModelDisplay(row metadata.RequestSummary) string {
	requestedProvider := row.RequestedProviderID
	requestedModel := row.RequestedModelID
	resolvedProvider := row.ResolvedProviderID
	resolvedModel := row.ResolvedModelID
	if requestedProvider == "" {
		requestedProvider = row.ProviderInstanceID
	}
	if requestedModel == "" {
		requestedModel = row.ModelID
	}
	if resolvedProvider == "" {
		resolvedProvider = row.ProviderInstanceID
	}
	if resolvedModel == "" {
		resolvedModel = row.ModelID
	}
	requested := safeDisplay(requestedProvider) + "/" + safeDisplay(requestedModel)
	resolved := safeDisplay(resolvedProvider) + "/" + safeDisplay(resolvedModel)
	if requested != resolved {
		return requested + " -> " + resolved
	}
	return resolved
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02T15:04:05Z")
}

func formatPreciseTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

var unsafeDisplayPattern = regexp.MustCompile(`(?i)(bearer|sk-|iln_|oauth|token|secret|authorization|raw|payload|prompt|completion|body|account|acct_|request[_ -]?id|requestid|req_|balance|credit|eyj[a-z0-9_-]*\.[a-z0-9_-]*\.)`)
var safeErrorMessagePattern = regexp.MustCompile(`^[a-z0-9_ .:-]+$`)

func safeDisplay(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	if unsafeDisplayPattern.MatchString(value) {
		return "[redacted]"
	}
	const maxDisplayRunes = 64
	runes := []rune(value)
	if len(runes) > maxDisplayRunes {
		return string(runes[:maxDisplayRunes]) + "..."
	}
	return value
}

func safeRefreshFailureClass(value string) string {
	switch value {
	case "refresh_token_expired", "refresh_token_invalidated", "refresh_token_reused",
		"refresh_unauthorized", "refresh_network_error", "refresh_timeout",
		"refresh_unavailable", "refresh_invalid_response":
		return value
	default:
		return safeDisplay(value)
	}
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
		created, err := m.upstreams.AddAPIKey(context.Background(), providerID, "api key", apiKey)
		if err != nil {
			m.logError(context.Background(), "tui_upstream_credential_create_failed", err, slog.String("provider_instance", providerID))
			m.err = err.Error()
			return m, nil
		}
		m.logInfo(context.Background(), "tui_upstream_credential_created",
			slog.String("provider_instance", providerID),
			slog.Int64("credential_id", created.ID),
		)
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
	if err == nil {
		m.logInfo(context.Background(), "tui_upstream_credential_created", slog.String("provider_instance", instance.ID))
	}
	return err
}

func (m *Model) disableFirstUpstreamCredential() error {
	if m.upstreams == nil {
		return nil
	}
	for _, cred := range m.credentials {
		if !cred.Disabled {
			if err := m.upstreams.Disable(context.Background(), cred.ID); err != nil {
				return err
			}
			m.logInfo(context.Background(), "tui_upstream_credential_disabled",
				slog.String("provider_instance", cred.ProviderInstanceID),
				slog.Int64("credential_id", cred.ID),
			)
			return nil
		}
	}
	return nil
}

func (m *Model) enableFirstFallbackPolicy() error {
	if m.upstreams == nil {
		return nil
	}
	for _, row := range m.fallbackPolicies {
		if !row.Enabled {
			if err := m.upstreams.EnableFallbackGroup(context.Background(), row.ProviderInstanceID, row.CredentialKind, row.GroupLabel); err != nil {
				return err
			}
			m.logInfo(context.Background(), "tui_fallback_policy_changed",
				slog.String("provider_instance", row.ProviderInstanceID),
				slog.String("credential_kind", row.CredentialKind),
				slog.String("group", row.GroupLabel),
				slog.Bool("enabled", true),
			)
			return nil
		}
	}
	return nil
}

func (m *Model) disableFirstFallbackPolicy() error {
	if m.upstreams == nil {
		return nil
	}
	for _, row := range m.fallbackPolicies {
		if row.Enabled {
			if err := m.upstreams.DisableFallbackGroup(context.Background(), row.ProviderInstanceID, row.CredentialKind, row.GroupLabel); err != nil {
				return err
			}
			m.logInfo(context.Background(), "tui_fallback_policy_changed",
				slog.String("provider_instance", row.ProviderInstanceID),
				slog.String("credential_kind", row.CredentialKind),
				slog.String("group", row.GroupLabel),
				slog.Bool("enabled", false),
			)
			return nil
		}
	}
	return nil
}

func (m Model) visibleFallbackPolicies(rows []credentials.FallbackPolicyMetadata) []credentials.FallbackPolicyMetadata {
	allowed := map[string]map[string]bool{}
	for _, instance := range m.visibleProviderRows() {
		if instance.APIKey && !instance.Placeholder {
			allowed[instance.ID] = map[string]bool{credentials.CredentialKindAPIKey: true}
		}
		if instance.OAuth && instance.Type == "codex" {
			if allowed[instance.ID] == nil {
				allowed[instance.ID] = map[string]bool{}
			}
			allowed[instance.ID][credentials.CredentialKindOAuth] = true
		}
	}
	out := rows[:0]
	for _, row := range rows {
		if allowed[row.ProviderInstanceID][row.CredentialKind] && row.CredentialCount >= 2 {
			out = append(out, row)
		}
	}
	return out
}

func (m Model) visibleUpstreamCredentials(rows []credentials.UpstreamCredentialMetadata) []credentials.UpstreamCredentialMetadata {
	allowed := map[string]bool{}
	for _, instance := range m.visibleProviderRows() {
		if instance.APIKey && !instance.Placeholder {
			allowed[instance.ID] = true
		}
	}
	out := rows[:0]
	for _, row := range rows {
		if allowed[row.ProviderInstanceID] {
			out = append(out, row)
		}
	}
	return out
}

func (m Model) visibleProviderRows() []provider.Instance {
	if len(m.providers) > 0 {
		return m.providers
	}
	return m.registry.List()
}

func fallbackPolicyEnabled(rows []credentials.FallbackPolicyMetadata, providerInstanceID, credentialKind, groupLabel string) bool {
	for _, row := range rows {
		if row.ProviderInstanceID == providerInstanceID && row.CredentialKind == credentialKind && row.GroupLabel == groupLabel {
			return row.Enabled
		}
	}
	return false
}

func firstAPIKeyProvider(registry provider.Registry) (provider.Instance, bool) {
	for _, instance := range registry.List() {
		if instance.APIKey && !instance.Placeholder {
			return instance, true
		}
	}
	return provider.Instance{}, false
}
