package tui

import (
	"context"
	"log/slog"
	"time"

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
	tabAPI tuiTab = iota
	tabProviders
	tabUsage
	tabLogs
	tuiTabCount
)

var tuiTabs = []struct {
	id    tuiTab
	label string
}{
	{tabAPI, "api"},
	{tabProviders, "providers"},
	{tabUsage, "usage"},
	{tabLogs, "logs"},
}

func NewModel(cfg config.Config, registry provider.Registry, tokens management.LocalTokenClient, upstreams management.UpstreamCredentialClient, oauth management.OAuthClient, pruner management.TelemetryPruneClient, subscriptionUsage management.SubscriptionUsageClient, now func() time.Time, loggers ...*slog.Logger) Model {
	return Model{cfg: cfg, registry: registry, tokens: tokens, upstreams: upstreams, oauth: oauth, pruner: pruner, subscriptionUsage: subscriptionUsage, now: now, logger: firstLogger(loggers)}
}

func (m Model) Init() tea.Cmd {
	return nil
}
