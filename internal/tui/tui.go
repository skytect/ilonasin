package tui

import (
	"context"
	"errors"
	"fmt"
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
	"ilonasin/internal/provider"
)

type Model struct {
	cfg              config.Config
	registry         provider.Registry
	snapshot         management.SnapshotClient
	tokens           management.LocalTokenClient
	upstreams        management.UpstreamCredentialClient
	oauth            management.OAuthClient
	pruner           management.TelemetryPruneClient
	logger           *slog.Logger
	now              func() time.Time
	tokenRows        []management.LocalToken
	providers        []provider.Instance
	credentials      []management.UpstreamCredential
	fallbackPolicies []management.FallbackPolicy
	oauthRows        []management.OAuthCredential
	accountRows      []management.ProviderAccount
	modelRows        []management.ModelMetadata
	requestRows      []management.RequestSummary
	usageRows        []management.UsageSummary
	latencyRows      []management.LatencySummary
	streamRows       []management.StreamSummary
	healthRows       []management.HealthSummary
	fallbackRows     []management.FallbackSummary
	quotaRows        []management.QuotaSummary
	pruneResult      *management.PruneResult
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
}

func NewModel(cfg config.Config, registry provider.Registry, tokens management.LocalTokenClient, upstreams management.UpstreamCredentialClient, oauth management.OAuthClient, pruner management.TelemetryPruneClient, now func() time.Time, loggers ...*slog.Logger) Model {
	return Model{cfg: cfg, registry: registry, tokens: tokens, upstreams: upstreams, oauth: oauth, pruner: pruner, now: now, logger: firstLogger(loggers)}
}

func (m Model) Init() tea.Cmd {
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
			m.clearReveal()
			if err := m.refreshSelectedOAuthCredential(); err != nil {
				m.logError(context.Background(), "tui_oauth_refresh_failed", err)
				m.err = "OAuth refresh failed"
				_ = m.reload()
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
	b.WriteString("\nPress n to create local token, a to add API key, d to disable local token, x to disable API key, l to login/relogin OAuth, o/r to select/refresh OAuth, f/F to mark credential group, p to prune telemetry, q to quit.\n")
	return b.String()
}

func Run(cfg config.Config, registry provider.Registry, snapshot management.SnapshotClient, tokens management.LocalTokenClient, upstreams management.UpstreamCredentialClient, oauth management.OAuthClient, pruner management.TelemetryPruneClient, loggers ...*slog.Logger) error {
	if snapshot == nil {
		return fmt.Errorf("management snapshot client is required")
	}
	model := NewModel(cfg, registry, tokens, upstreams, oauth, pruner, nil, loggers...)
	model.snapshot = snapshot
	if err := model.reload(); err != nil {
		return err
	}
	_, err := tea.NewProgram(model).Run()
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

func oauthChallengeFromManagement(row management.OAuthDeviceLoginChallenge) credentials.OAuthDeviceLoginChallenge {
	return credentials.OAuthDeviceLoginChallenge{
		ProviderInstanceID: row.ProviderInstanceID,
		VerificationURL:    row.VerificationURL,
		UserCode:           row.UserCode,
		Handle:             row.Handle,
	}
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
		refreshDescription := safeRefreshFailureDescriptionDisplay(row.RefreshFailureDescription)
		if refreshDescription != "" {
			refresh = refresh + " " + refreshDescription
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
	b.WriteString("\nCredential groups\n")
	if len(m.fallbackPolicies) == 0 {
		b.WriteString("No credential group metadata.\n")
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
	if m.snapshot == nil {
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
		route := safeEndpointDisplay(row.Endpoint)
		if row.Stream {
			route += " stream"
		}
		options := ""
		if row.RequestedServiceTier != "" {
			options += " service_tier " + safeDisplay(row.RequestedServiceTier)
		}
		if row.EffectiveServiceTier != "" && row.EffectiveServiceTier != row.RequestedServiceTier {
			options += " effective_tier " + safeDisplay(row.EffectiveServiceTier)
		}
		if row.ReasoningEffort != "" {
			options += " reasoning " + safeDisplay(row.ReasoningEffort)
		}
		if row.ThinkingType != "" {
			options += " thinking " + safeDisplay(row.ThinkingType)
		}
		fmt.Fprintf(b, "- %s %s %s status %d %s %s attempts %d auth_retry %d fallback %d%s msg %d tools %d images %d%s prompt %d completion %d total %d reasoning %d reasoning_rate %.2f cache_hit %d cache_hit_rate %.2f cache_miss %d cache_miss_rate %.2f cache_write %d cache_write_rate %.2f latency %dms upstream %dms ttft %dms tps_total %.2f tps_after_ttft %.2f\n",
			formatTime(row.StartedAt), route, requestModelDisplay(row),
			row.HTTPStatus, safeDisplay(row.ErrorClass), credential, row.AttemptCount, row.AuthRetryCount,
			row.FallbackCount, fallbackReason, row.MessageCount, row.ToolCount, row.ImageCount, options,
			row.PromptTokens, row.CompletionTokens, row.TotalTokens, row.ReasoningTokens, row.ReasoningTokenRate,
			row.CacheHitTokens, row.CacheHitRate, row.CacheMissTokens, row.CacheMissRate, row.CacheWriteTokens, row.CacheWriteRate,
			row.TotalLatencyMS, row.UpstreamLatencyMS, row.TimeToFirstTokenMS,
			row.OutputTokensPerSecondTotal, row.OutputTokensPerSecondAfterTTFT)
	}
	b.WriteString("\nUsage totals\n")
	if len(m.usageRows) == 0 {
		b.WriteString("No usage metadata.\n")
	}
	for _, row := range m.usageRows {
		fmt.Fprintf(b, "- %s %d req prompt %d completion %d total %d reasoning %d reasoning_rate %.2f cache_hit %d cache_hit_rate %.2f cache_miss %d cache_miss_rate %.2f cache_write %d cache_write_rate %.2f cost_microunits %d\n",
			safeDisplay(row.ProviderInstanceID), row.RequestCount, row.PromptTokens,
			row.CompletionTokens, row.TotalTokens, row.ReasoningTokens,
			row.ReasoningTokenRate, row.CacheHitTokens, row.CacheHitRate, row.CacheMissTokens, row.CacheMissRate,
			row.CacheWriteTokens, row.CacheWriteRate, row.CostMicrounits)
	}
	b.WriteString("\nLatency\n")
	if len(m.latencyRows) == 0 {
		b.WriteString("No latency metadata.\n")
	}
	for _, row := range m.latencyRows {
		fmt.Fprintf(b, "- %s %d req avg latency %dms upstream %dms ttft %dms tps %.2f tps_total %.2f tps_after_ttft %.2f\n",
			safeDisplay(row.ProviderInstanceID), row.RequestCount, row.AverageLatencyMS,
			row.AverageUpstreamLatencyMS, row.AverageTimeToFirstTokenMS, row.AverageOutputTPS,
			row.AverageOutputTPSTotal, row.AverageOutputTPSAfterTTFT)
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
	b.WriteString("\nQuota\n")
	if len(m.quotaRows) == 0 {
		b.WriteString("No quota metadata.\n")
	}
	for _, row := range m.quotaRows {
		retryAfter := ""
		if row.RetryAfter != nil {
			retryAfter = " retry_after " + formatTime(*row.RetryAfter)
		}
		resetAt := ""
		if row.ResetAt != nil {
			resetAt = " reset " + formatTime(*row.ResetAt)
		}
		fmt.Fprintf(b, "- %s/%s %s status %d %s %s count %d at %s%s%s\n",
			safeDisplay(row.ProviderInstanceID), healthModelDisplay(row.ModelID),
			safeDisplay(row.Source), row.HTTPStatus, safeDisplay(row.ErrorClass),
			credentialDisplay(row.CredentialID, row.CredentialLabel), row.Count,
			formatTime(row.ObservedAt), retryAfter, resetAt)
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
		fmt.Fprintf(b, "Last prune before %s: requests %d streams %d fallbacks %d health %d quotas %d\n",
			formatPreciseTime(m.pruneResult.Cutoff), m.pruneResult.Requests, m.pruneResult.Streams,
			m.pruneResult.Fallbacks, m.pruneResult.Health, m.pruneResult.Quotas)
	}
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

func (m Model) startOAuthLoginCmd(ctx context.Context, providerInstanceID string) tea.Cmd {
	return func() tea.Msg {
		resp, err := m.oauth.StartOAuthDeviceLogin(ctx, management.StartOAuthDeviceLoginRequest{ProviderInstanceID: providerInstanceID})
		return oauthLoginStartedMsg{challenge: oauthChallengeFromManagement(resp.Challenge), err: err}
	}
}

func (m Model) completeOAuthLoginCmd(handle string) tea.Cmd {
	return func() tea.Msg {
		ctx := m.oauthCtx
		if ctx == nil {
			ctx = context.Background()
		}
		_, err := m.oauth.CompleteOAuthDeviceLogin(ctx, management.CompleteOAuthDeviceLoginRequest{Handle: handle})
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
	if m.oauth == nil || len(m.oauthRows) == 0 {
		return nil
	}
	if m.oauthSelected < 0 || m.oauthSelected >= len(m.oauthRows) {
		return nil
	}
	row := m.oauthRows[m.oauthSelected]
	if row.Disabled {
		return nil
	}
	if _, err := m.oauth.RefreshOAuthCredential(context.Background(), management.RefreshOAuthCredentialRequest{ID: row.ID}); err != nil {
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

func requestModelDisplay(row management.RequestSummary) string {
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

func safeEndpointDisplay(value string) string {
	value = strings.TrimSpace(value)
	switch value {
	case "chat_completions", "responses":
		return value
	default:
		return ""
	}
}

func safeRefreshFailureDescriptionDisplay(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, strings.TrimSpace(value))
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return ""
	}
	const maxDisplayRunes = 160
	runes := []rune(value)
	if len(runes) > maxDisplayRunes {
		return string(runes[:maxDisplayRunes]) + "..."
	}
	return value
}

func safeRefreshFailureClass(value string) string {
	switch value {
	case "refresh_token_expired", "refresh_token_invalidated", "refresh_token_reused",
		"refresh_invalid_grant", "refresh_invalid_client", "refresh_invalid_request",
		"refresh_unauthorized_client", "refresh_access_denied",
		"refresh_unsupported_grant_type", "refresh_invalid_scope",
		"refresh_server_error", "refresh_temporarily_unavailable",
		"refresh_unauthorized", "refresh_network_error", "refresh_timeout",
		"refresh_http_error", "refresh_body_too_large", "refresh_unavailable",
		"refresh_invalid_response":
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

func modelCacheSummaries(rows []management.ModelMetadata) []modelCacheSummary {
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
		if m.upstreams == nil {
			m.err = "upstream credential management is unavailable"
			return m, nil
		}
		created, err := m.upstreams.AddUpstreamAPIKey(context.Background(), management.AddUpstreamAPIKeyRequest{
			ProviderInstanceID: providerID,
			Label:              "api key",
			APIKey:             apiKey,
		})
		if err != nil {
			m.logError(context.Background(), "tui_upstream_credential_create_failed", err, slog.String("provider_instance", providerID))
			m.err = err.Error()
			return m, nil
		}
		m.logInfo(context.Background(), "tui_upstream_credential_created",
			slog.String("provider_instance", providerID),
			slog.Int64("credential_id", created.Credential.ID),
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

func (m *Model) disableFirstUpstreamCredential() error {
	if m.upstreams == nil {
		return nil
	}
	for _, cred := range m.credentials {
		if !cred.Disabled {
			if _, err := m.upstreams.DisableUpstreamCredential(context.Background(), management.DisableUpstreamCredentialRequest{ID: cred.ID}); err != nil {
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
			if _, err := m.upstreams.EnableFallbackPolicy(context.Background(), management.FallbackPolicyRequest{
				ProviderInstanceID: row.ProviderInstanceID,
				CredentialKind:     row.CredentialKind,
				GroupLabel:         row.GroupLabel,
			}); err != nil {
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
			if _, err := m.upstreams.DisableFallbackPolicy(context.Background(), management.FallbackPolicyRequest{
				ProviderInstanceID: row.ProviderInstanceID,
				CredentialKind:     row.CredentialKind,
				GroupLabel:         row.GroupLabel,
			}); err != nil {
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

func (m Model) visibleFallbackPolicies(rows []management.FallbackPolicy) []management.FallbackPolicy {
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
	out := make([]management.FallbackPolicy, 0, len(rows))
	for _, row := range rows {
		if allowed[row.ProviderInstanceID][row.CredentialKind] && row.CredentialCount >= 2 {
			out = append(out, row)
		}
	}
	return out
}

func (m Model) visibleUpstreamCredentials(rows []management.UpstreamCredential) []management.UpstreamCredential {
	allowed := map[string]bool{}
	for _, instance := range m.visibleProviderRows() {
		if instance.APIKey && !instance.Placeholder {
			allowed[instance.ID] = true
		}
	}
	out := make([]management.UpstreamCredential, 0, len(rows))
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

func fallbackPolicyEnabled(rows []management.FallbackPolicy, providerInstanceID, credentialKind, groupLabel string) bool {
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
