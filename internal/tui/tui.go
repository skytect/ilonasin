package tui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"

	"ilonasin/internal/config"
	"ilonasin/internal/credentials"
	"ilonasin/internal/management"
	"ilonasin/internal/provider"
)

type Model struct {
	cfg               config.Config
	registry          provider.Registry
	snapshot          management.SnapshotClient
	tokens            management.LocalTokenClient
	upstreams         management.UpstreamCredentialClient
	oauth             management.OAuthClient
	pruner            management.TelemetryPruneClient
	subscriptionUsage management.SubscriptionUsageClient
	logger            *slog.Logger
	now               func() time.Time
	tokenRows         []management.LocalToken
	providers         []provider.Instance
	credentials       []management.UpstreamCredential
	fallbackPolicies  []management.FallbackPolicy
	oauthRows         []management.OAuthCredential
	accountRows       []management.ProviderAccount
	modelRows         []management.ModelMetadata
	requestRows       []management.RequestSummary
	usageRows         []management.UsageSummary
	latencyRows       []management.LatencySummary
	streamRows        []management.StreamSummary
	healthRows        []management.HealthSummary
	fallbackRows      []management.FallbackSummary
	quotaRows         []management.QuotaSummary
	subscriptionRows  []management.SubscriptionUsageRow
	subscriptionPools []management.SubscriptionUsageAggregate
	keepaliveStatus   management.KeepaliveStatus
	pruneResult       *management.PruneResult
	pruningAvailable  bool
	width             int
	height            int
	activeTab         tuiTab
	scrollOffsets     [tuiTabCount]int
	selected          int
	oauthSelected     int
	revealTokenID     int64
	revealTokenPrefix string
	revealTokenLast4  string
	apiKeyMode        bool
	apiKeyProvider    string
	apiKeyInput       string
	oauthChallenge    *credentials.OAuthDeviceLoginChallenge
	oauthCtx          context.Context
	oauthCancel       context.CancelFunc
	err               string
}

type tuiTab int

const (
	tabOverview tuiTab = iota
	tabAccounts
	tabObservability
	tabHelp
	tuiTabCount
)

var tuiTabs = []struct {
	id    tuiTab
	label string
}{
	{tabOverview, "overview"},
	{tabAccounts, "accounts"},
	{tabObservability, "observability"},
	{tabHelp, "help"},
}

func NewModel(cfg config.Config, registry provider.Registry, tokens management.LocalTokenClient, upstreams management.UpstreamCredentialClient, oauth management.OAuthClient, pruner management.TelemetryPruneClient, subscriptionUsage management.SubscriptionUsageClient, now func() time.Time, loggers ...*slog.Logger) Model {
	return Model{cfg: cfg, registry: registry, tokens: tokens, upstreams: upstreams, oauth: oauth, pruner: pruner, subscriptionUsage: subscriptionUsage, now: now, logger: firstLogger(loggers)}
}

func (m Model) Init() tea.Cmd {
	return nil
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
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.clampScrolls()
		return m, nil
	case tea.MouseMsg:
		switch msg.Type {
		case tea.MouseWheelUp:
			m.scrollActive(-3)
		case tea.MouseWheelDown:
			m.scrollActive(3)
		}
		return m, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		if m.apiKeyMode {
			return m.updateAPIKeyInput(key)
		}
		switch key.String() {
		case "tab", "right":
			m.activeTab = (m.activeTab + 1) % tuiTabCount
			m.clampScrolls()
		case "shift+tab", "left":
			m.activeTab = (m.activeTab + tuiTabCount - 1) % tuiTabCount
			m.clampScrolls()
		case "1":
			m.activeTab = tabOverview
			m.clampScrolls()
		case "2":
			m.activeTab = tabAccounts
			m.clampScrolls()
		case "3":
			m.activeTab = tabObservability
			m.clampScrolls()
		case "4":
			m.activeTab = tabHelp
			m.clampScrolls()
		case "pgdown", "ctrl+d":
			m.scrollActive(m.viewportHeight())
		case "pgup", "ctrl+u":
			m.scrollActive(-m.viewportHeight())
		case "home":
			m.setActiveScroll(0)
		case "end":
			m.setActiveScroll(m.activeScrollMax())
		case "q", "ctrl+c":
			m.clearReveal()
			m.cancelOAuthLogin()
			return m, tea.Quit
		case "n":
			if m.activeTab != tabAccounts {
				return m, nil
			}
			m.clearReveal()
			created, err := m.tokens.CreateLocalToken(context.Background(), management.CreateLocalTokenRequest{Label: "local client"})
			if err != nil {
				m.logError(context.Background(), "tui_local_token_create_failed", err)
				m.err = err.Error()
				return m, nil
			}
			m.logInfo(context.Background(), "tui_local_token_created", slog.Int64("local_id", created.Metadata.ID))
			m.revealTokenID = created.Metadata.ID
			m.revealTokenPrefix = created.Metadata.TokenPrefix
			m.revealTokenLast4 = created.Metadata.TokenLast4
			_ = m.reload()
		case "d":
			if m.activeTab != tabAccounts {
				return m, nil
			}
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
			if m.activeTab != tabAccounts {
				return m, nil
			}
			m.clearReveal()
			if err := m.disableFirstUpstreamCredential(); err != nil {
				m.logError(context.Background(), "tui_upstream_credential_disable_failed", err)
				m.err = err.Error()
				return m, nil
			}
			_ = m.reload()
		case "a":
			if m.activeTab != tabAccounts {
				return m, nil
			}
			m.clearReveal()
			instance, ok := firstAPIKeyProvider(m.registry)
			if !ok {
				m.err = "no API-key provider instance is configured"
				return m, nil
			}
			m.apiKeyMode = true
			m.apiKeyProvider = instance.ID
			m.apiKeyInput = ""
			return m, nil
		case "p":
			if m.activeTab != tabObservability {
				return m, nil
			}
			m.clearReveal()
			if err := m.pruneTelemetry(); err != nil {
				m.logError(context.Background(), "tui_telemetry_prune_failed", err)
				m.err = "telemetry prune failed"
				return m, nil
			}
			_ = m.reload()
		case "u":
			if m.activeTab != tabObservability {
				return m, nil
			}
			m.clearReveal()
			if m.subscriptionUsage == nil {
				return m, nil
			}
			resp, err := m.subscriptionUsage.RefreshSubscriptionUsage(context.Background())
			if err != nil {
				m.logError(context.Background(), "tui_subscription_usage_refresh_failed", err)
				m.err = "subscription usage refresh failed"
				return m, nil
			}
			m.applySubscriptionUsage(resp)
			_ = m.reload()
		case "l":
			if m.activeTab != tabAccounts {
				return m, nil
			}
			m.clearReveal()
			m.cancelOAuthLogin()
			providerID, ok := firstOAuthLoginProvider(m.registry)
			if !ok || m.oauth == nil {
				m.logInfo(context.Background(), "tui_oauth_login_unavailable")
				m.err = "OAuth login failed"
				return m, nil
			}
			loginCtx, cancel := context.WithCancel(context.Background())
			m.oauthCtx = loginCtx
			m.oauthCancel = cancel
			return m, m.startOAuthLoginCmd(loginCtx, providerID)
		case "r":
			if m.activeTab != tabAccounts {
				return m, nil
			}
			m.clearReveal()
			if err := m.refreshSelectedOAuthCredential(); err != nil {
				m.logError(context.Background(), "tui_oauth_refresh_failed", err)
				m.err = "OAuth refresh failed"
				_ = m.reload()
				return m, nil
			}
			_ = m.reload()
		case "o":
			if m.activeTab != tabAccounts {
				return m, nil
			}
			m.clearReveal()
			if len(m.oauthRows) > 0 {
				m.oauthSelected = (m.oauthSelected + 1) % len(m.oauthRows)
			}
		case "f":
			if m.activeTab != tabAccounts {
				return m, nil
			}
			m.clearReveal()
			if err := m.enableFirstFallbackPolicy(); err != nil {
				m.logError(context.Background(), "tui_fallback_policy_update_failed", err)
				m.err = "fallback policy update failed"
				return m, nil
			}
			_ = m.reload()
		case "F":
			if m.activeTab != tabAccounts {
				return m, nil
			}
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
			if m.activeTab == tabAccounts {
				m.selectNextLocalToken()
			} else {
				m.scrollActive(1)
			}
		case "up", "k":
			m.clearReveal()
			if m.activeTab == tabAccounts {
				m.selectPreviousLocalToken()
			} else {
				m.scrollActive(-1)
			}
		}
	}
	return m, nil
}

func Run(cfg config.Config, registry provider.Registry, snapshot management.SnapshotClient, tokens management.LocalTokenClient, upstreams management.UpstreamCredentialClient, oauth management.OAuthClient, pruner management.TelemetryPruneClient, loggers ...*slog.Logger) error {
	if snapshot == nil {
		return fmt.Errorf("management snapshot client is required")
	}
	var subscriptionUsage management.SubscriptionUsageClient
	if client, ok := snapshot.(management.SubscriptionUsageClient); ok {
		subscriptionUsage = client
	}
	model := NewModel(cfg, registry, tokens, upstreams, oauth, pruner, subscriptionUsage, nil, loggers...)
	model.snapshot = snapshot
	if err := model.reload(); err != nil {
		return err
	}
	_, err := tea.NewProgram(model, tea.WithMouseCellMotion()).Run()
	return err
}

func (m *Model) reload() error {
	if m.snapshot == nil {
		err := fmt.Errorf("management snapshot client is required")
		m.err = err.Error()
		return err
	}
	snapshot, err := m.snapshot.LoadManagementSnapshot(context.Background())
	if err != nil {
		m.err = err.Error()
		return err
	}
	m.applySnapshot(snapshot)
	return nil
}

func (m *Model) applySnapshot(snapshot management.ManagementSnapshotResponse) {
	m.tokenRows = snapshot.LocalTokens
	m.providers = providersFromSnapshot(snapshot.Providers)
	m.credentials = m.visibleUpstreamCredentials(snapshot.UpstreamCredentials)
	m.fallbackPolicies = m.visibleFallbackPolicies(snapshot.FallbackPolicies)
	m.oauthRows = append([]management.OAuthCredential(nil), snapshot.OAuthCredentials...)
	m.accountRows = append([]management.ProviderAccount(nil), snapshot.ProviderAccounts...)
	m.modelRows = append([]management.ModelMetadata(nil), snapshot.ModelCache...)
	m.requestRows = append([]management.RequestSummary(nil), snapshot.RecentRequests...)
	m.usageRows = append([]management.UsageSummary(nil), snapshot.Usage...)
	m.latencyRows = append([]management.LatencySummary(nil), snapshot.Latency...)
	m.streamRows = append([]management.StreamSummary(nil), snapshot.Streams...)
	m.healthRows = append([]management.HealthSummary(nil), snapshot.Health...)
	m.fallbackRows = append([]management.FallbackSummary(nil), snapshot.Fallbacks...)
	m.quotaRows = append([]management.QuotaSummary(nil), snapshot.Quotas...)
	m.applySubscriptionUsage(snapshot.SubscriptionUsage)
	m.pruningAvailable = snapshot.PruningAvailable
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
	m.clampScrolls()
}

func (m *Model) applySubscriptionUsage(resp management.SubscriptionUsageResponse) {
	m.subscriptionRows = append([]management.SubscriptionUsageRow(nil), resp.Accounts...)
	m.subscriptionPools = append([]management.SubscriptionUsageAggregate(nil), resp.Pools...)
	m.keepaliveStatus = resp.Keepalive
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

func (m *Model) pruneTelemetry() error {
	if m.pruner == nil {
		return nil
	}
	cutoff := m.nowTime().Add(-30 * 24 * time.Hour).UTC()
	resp, err := m.pruner.PruneTelemetry(context.Background(), management.PruneTelemetryRequest{Cutoff: cutoff})
	if err != nil {
		return err
	}
	result := resp.Result
	m.pruneResult = &result
	m.logInfo(context.Background(), "tui_telemetry_pruned",
		slog.Int("requests", result.Requests),
		slog.Int("streams", result.Streams),
		slog.Int("fallbacks", result.Fallbacks),
		slog.Int("health", result.Health),
	)
	return nil
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
