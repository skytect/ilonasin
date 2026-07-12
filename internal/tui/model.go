package tui

import (
	"context"
	"log/slog"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"ilonasin/internal/management"
)

type Model struct {
	snapshot                    management.SnapshotClient
	tokens                      management.LocalTokenClient
	upstreams                   management.UpstreamCredentialClient
	oauth                       management.OAuthClient
	pruner                      management.TelemetryPruneClient
	subscriptionUsage           management.SubscriptionUsageClient
	logger                      *slog.Logger
	now                         func() time.Time
	runtime                     management.RuntimeStatus
	tokenRows                   []management.LocalToken
	localTokenUsage             []management.LocalTokenUsageSummary
	providers                   []management.ProviderInstance
	credentials                 []management.UpstreamCredential
	credentialPoolGroups        []management.CredentialPoolGroup
	routingPolicy               management.RoutingPolicyStatus
	oauthRows                   []management.OAuthCredential
	accountRows                 []management.ProviderAccount
	modelRows                   []management.ModelMetadata
	requestRows                 []management.RequestSummary
	usageRows                   []management.UsageSummary
	latencyRows                 []management.LatencySummary
	streamRows                  []management.StreamSummary
	healthRows                  []management.HealthSummary
	fallbackRows                []management.FallbackSummary
	activeQuotaBlocks           []management.ActiveQuotaBlock
	quotaRows                   []management.QuotaSummary
	subscriptionRows            []management.SubscriptionUsageRow
	subscriptionPools           []management.SubscriptionUsageAggregate
	keepaliveStatus             management.KeepaliveStatus
	subscriptionObservedAt      time.Time
	snapshotRefreshInFlight     bool
	snapshotForegroundPending   bool
	subscriptionRefreshInFlight bool
	mutationInFlight            bool
	pruneResult                 *management.PruneResult
	pruningAvailable            bool
	width                       int
	height                      int
	renderWidth                 int
	activeTab                   tuiTab
	paneFocus                   [tuiTabCount]int
	paneScrollOffsets           [tuiTabCount][maxDashboardPanes]int
	selected                    int
	oauthSelected               int
	revealTokenID               int64
	revealTokenPrefix           string
	revealTokenLast4            string
	apiKeyMode                  bool
	apiKeyProvider              string
	apiKeyInput                 string
	oauthChallenge              *management.OAuthDeviceLoginChallenge
	oauthCtx                    context.Context
	oauthCancel                 context.CancelFunc
	err                         string
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

func NewModel(tokens management.LocalTokenClient, upstreams management.UpstreamCredentialClient, oauth management.OAuthClient, pruner management.TelemetryPruneClient, subscriptionUsage management.SubscriptionUsageClient, now func() time.Time, loggers ...*slog.Logger) Model {
	m := Model{tokens: tokens, upstreams: upstreams, oauth: oauth, pruner: pruner, subscriptionUsage: subscriptionUsage, now: now, logger: firstLogger(loggers)}
	m.paneFocus[tabAPI] = apiPaneTokens
	m.paneFocus[tabUsage] = usagePaneSubscriptions
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		subscriptionUsageAutoRefreshTickCmd(0),
		snapshotAutoRefreshTickCmd(snapshotAutoRefreshInterval),
	)
}
