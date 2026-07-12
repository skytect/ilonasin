package management

import "time"

type SubscriptionUsageRow struct {
	ObservedAt           time.Time                              `json:"observed_at"`
	ProviderInstanceID   string                                 `json:"provider_instance_id"`
	CredentialID         int64                                  `json:"credential_id"`
	AccountDisplayLabel  string                                 `json:"account_display_label"`
	PlanLabel            string                                 `json:"plan_label"`
	LimitID              string                                 `json:"limit_id"`
	LimitName            string                                 `json:"limit_name"`
	PlanType             string                                 `json:"plan_type"`
	ReachedType          string                                 `json:"reached_type"`
	Source               string                                 `json:"source"`
	ErrorClass           string                                 `json:"error_class"`
	Stale                bool                                   `json:"stale"`
	Windows              []SubscriptionUsageWindow              `json:"windows"`
	BankedResetInventory *SubscriptionUsageBankedResetInventory `json:"banked_reset_inventory,omitempty"`
}

type SubscriptionUsageAggregate struct {
	ProviderInstanceID   string                                     `json:"provider_instance_id"`
	LimitID              string                                     `json:"limit_id"`
	LimitName            string                                     `json:"limit_name"`
	AccountCount         int                                        `json:"account_count"`
	StaleCount           int                                        `json:"stale_count"`
	Windows              []SubscriptionUsagePoolWindow              `json:"windows"`
	BankedResetInventory *SubscriptionUsagePoolBankedResetInventory `json:"banked_reset_inventory,omitempty"`
}

type SubscriptionUsageWindow struct {
	Kind             string     `json:"kind"`
	Label            string     `json:"label"`
	UsedPercent      float64    `json:"used_percent"`
	RemainingPercent float64    `json:"remaining_percent"`
	WindowMinutes    int        `json:"window_minutes"`
	ResetAt          *time.Time `json:"reset_at,omitempty"`
}

type SubscriptionUsagePoolWindow struct {
	Kind                        string     `json:"kind"`
	Label                       string     `json:"label"`
	AccountCount                int        `json:"account_count"`
	FreshAccountCount           int        `json:"fresh_account_count"`
	StaleCount                  int        `json:"stale_count"`
	TotalUsedPercentPoints      float64    `json:"total_used_percent_points"`
	TotalRemainingPercentPoints float64    `json:"total_remaining_percent_points"`
	TotalCapacityPercentPoints  float64    `json:"total_capacity_percent_points"`
	EarliestResetAt             *time.Time `json:"earliest_reset_at,omitempty"`
}

type SubscriptionUsageBankedResetInventory struct {
	AvailableCount   *int                                 `json:"available_count"`
	DetailsAvailable bool                                 `json:"details_available"`
	DetailErrorClass string                               `json:"detail_error_class,omitempty"`
	Details          []SubscriptionUsageBankedResetDetail `json:"details"`
}

type SubscriptionUsageBankedResetDetail struct {
	ResetType string     `json:"reset_type"`
	Status    string     `json:"status"`
	GrantedAt time.Time  `json:"granted_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type SubscriptionUsagePoolBankedResetInventory struct {
	AvailableCount          int        `json:"available_count"`
	KnownAccountCount       int        `json:"known_account_count"`
	UnknownAccountCount     int        `json:"unknown_account_count"`
	DetailsUnavailableCount int        `json:"details_unavailable_count"`
	EarliestExpiresAt       *time.Time `json:"earliest_expires_at,omitempty"`
}

type KeepaliveStatus struct {
	Enabled           bool     `json:"enabled"`
	Status            string   `json:"status"`
	OutputCapVerified bool     `json:"output_cap_verified"`
	ScheduleTimes     []string `json:"schedule_times"`
}

type SubscriptionUsageResponse struct {
	ObservedAt time.Time                    `json:"observed_at"`
	Accounts   []SubscriptionUsageRow       `json:"accounts"`
	Pools      []SubscriptionUsageAggregate `json:"pools"`
	Keepalive  KeepaliveStatus              `json:"keepalive"`
}
