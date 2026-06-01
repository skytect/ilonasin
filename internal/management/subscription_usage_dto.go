package management

import "time"

type SubscriptionUsageRow struct {
	ObservedAt                time.Time                 `json:"observed_at"`
	ProviderInstanceID        string                    `json:"provider_instance_id"`
	CredentialID              int64                     `json:"credential_id"`
	AccountDisplayLabel       string                    `json:"account_display_label"`
	PlanLabel                 string                    `json:"plan_label"`
	LimitID                   string                    `json:"limit_id"`
	LimitName                 string                    `json:"limit_name"`
	PlanType                  string                    `json:"plan_type"`
	ReachedType               string                    `json:"reached_type"`
	PrimaryLabel              string                    `json:"primary_label"`
	PrimaryUsedPercent        float64                   `json:"primary_used_percent"`
	PrimaryRemainingPercent   float64                   `json:"primary_remaining_percent"`
	PrimaryWindowMinutes      int                       `json:"primary_window_minutes"`
	PrimaryResetAt            *time.Time                `json:"primary_reset_at,omitempty"`
	SecondaryLabel            string                    `json:"secondary_label"`
	SecondaryUsedPercent      float64                   `json:"secondary_used_percent"`
	SecondaryRemainingPercent float64                   `json:"secondary_remaining_percent"`
	SecondaryWindowMinutes    int                       `json:"secondary_window_minutes"`
	SecondaryResetAt          *time.Time                `json:"secondary_reset_at,omitempty"`
	Source                    string                    `json:"source"`
	ErrorClass                string                    `json:"error_class"`
	Stale                     bool                      `json:"stale"`
	Windows                   []SubscriptionUsageWindow `json:"windows"`
}

type SubscriptionUsageAggregate struct {
	ProviderInstanceID               string                        `json:"provider_instance_id"`
	LimitID                          string                        `json:"limit_id"`
	LimitName                        string                        `json:"limit_name"`
	AccountCount                     int                           `json:"account_count"`
	StaleCount                       int                           `json:"stale_count"`
	AveragePrimaryUsedPercent        float64                       `json:"average_primary_used_percent"`
	MinimumPrimaryRemainingPercent   float64                       `json:"minimum_primary_remaining_percent"`
	EarliestPrimaryResetAt           *time.Time                    `json:"earliest_primary_reset_at,omitempty"`
	AverageSecondaryUsedPercent      float64                       `json:"average_secondary_used_percent"`
	MinimumSecondaryRemainingPercent float64                       `json:"minimum_secondary_remaining_percent"`
	EarliestSecondaryResetAt         *time.Time                    `json:"earliest_secondary_reset_at,omitempty"`
	Windows                          []SubscriptionUsagePoolWindow `json:"windows"`
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
	StaleCount                  int        `json:"stale_count"`
	AverageUsedPercent          float64    `json:"average_used_percent"`
	MinimumRemainingPercent     float64    `json:"minimum_remaining_percent"`
	TotalUsedPercentPoints      float64    `json:"total_used_percent_points"`
	TotalRemainingPercentPoints float64    `json:"total_remaining_percent_points"`
	TotalCapacityPercentPoints  float64    `json:"total_capacity_percent_points"`
	EarliestResetAt             *time.Time `json:"earliest_reset_at,omitempty"`
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
