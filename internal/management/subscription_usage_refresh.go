package management

import (
	"context"
	"time"
)

type SubscriptionUsageFetcher interface {
	FetchSubscriptionUsage(ctx context.Context, req SubscriptionUsageFetchRequest) (SubscriptionUsageFetchResult, error)
}

type SubscriptionUsageFetchRequest struct {
	Provider   SubscriptionUsageProvider
	Credential SubscriptionUsageBearerCredential
}

type SubscriptionUsageProvider struct {
	ID             string
	Type           string
	BaseURL        string
	AuthIssuer     string
	AuthStyle      string
	APIKey         bool
	OAuth          bool
	OAuthRefresh   bool
	Chat           bool
	ModelDiscovery bool
}

type SubscriptionUsageBearerCredential struct {
	ID                      int64
	ProviderInstanceID      string
	BearerToken             string
	ChatGPTAccountID        string
	ChatGPTAccountIsFedRAMP bool
}

type SubscriptionUsageFetchResult struct {
	Snapshots            []SubscriptionUsageFetchSnapshot
	BankedResetInventory SubscriptionUsageFetchBankedResetInventory
	ErrorClass           string
	StatusCode           int
}

type SubscriptionUsageFetchSnapshot struct {
	LimitID     string
	LimitName   string
	PlanType    string
	ReachedType string
	Primary     *SubscriptionUsageFetchWindow
	Secondary   *SubscriptionUsageFetchWindow
}

type SubscriptionUsageFetchWindow struct {
	UsedPercent   float64
	WindowMinutes int
	ResetsAt      *time.Time
}

type SubscriptionUsageFetchBankedResetInventory struct {
	AvailableCount   *int
	DetailsAvailable bool
	DetailErrorClass string
	Details          []SubscriptionUsageFetchBankedResetDetail
}

type SubscriptionUsageFetchBankedResetDetail struct {
	ResetType string
	Status    string
	GrantedAt time.Time
	ExpiresAt *time.Time
}
