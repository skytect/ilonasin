package management

import (
	"context"
	"strings"
	"time"

	"ilonasin/internal/credentials"
	"ilonasin/internal/metadata"
	"ilonasin/internal/provider"
)

const PathSubscriptionUsage = "/_ilonasin/manage/subscription-usage"

type SubscriptionUsageClient interface {
	GetSubscriptionUsage(ctx context.Context) (SubscriptionUsageResponse, error)
	RefreshSubscriptionUsage(ctx context.Context) (SubscriptionUsageResponse, error)
}

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

func (s Service) GetSubscriptionUsage(ctx context.Context) (SubscriptionUsageResponse, error) {
	var rows []metadata.SubscriptionUsageSnapshot
	if s.SubscriptionUsage != nil {
		var err error
		rows, err = s.SubscriptionUsage.LatestSubscriptionUsageSnapshots(ctx)
		if err != nil {
			return SubscriptionUsageResponse{}, err
		}
	}
	resp := subscriptionUsageResponse(rows, s.keepaliveStatus())
	sanitizeSubscriptionUsageResponse(&resp)
	return resp, nil
}

func (s Service) RefreshSubscriptionUsage(ctx context.Context) (SubscriptionUsageResponse, error) {
	if s.SubscriptionUsage == nil || s.OAuthResolver == nil || s.UsageClient == nil {
		return SubscriptionUsageResponse{}, errManagementUnavailable
	}
	now := time.Now().UTC()
	oauthRows, err := s.oauthRows(ctx)
	if err != nil {
		return SubscriptionUsageResponse{}, err
	}
	var successes int
	var recordedErrors int
	var firstErr error
	for _, instance := range s.Registry.List() {
		if instance.Type != "codex" || !instance.OAuth {
			continue
		}
		for _, meta := range oauthRows {
			if meta.ProviderInstanceID != instance.ID || meta.Disabled {
				continue
			}
			bearer, err := s.resolveUsageBearer(ctx, meta.ID, now)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			snapshots, err := s.refreshCredentialUsage(ctx, instance, bearer, meta, now)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				if snapshots > 0 {
					recordedErrors += snapshots
				}
				continue
			}
			successes += snapshots
		}
	}
	if recordedErrors > 0 {
		return s.GetSubscriptionUsage(ctx)
	}
	if successes == 0 && firstErr != nil {
		return SubscriptionUsageResponse{}, firstErr
	}
	return s.GetSubscriptionUsage(ctx)
}

func (s Service) resolveUsageBearer(ctx context.Context, credentialID int64, now time.Time) (credentials.ResolvedOAuthBearerCredential, error) {
	bearer, err := s.OAuthResolver.ResolveOAuthBearerByID(ctx, credentialID, now)
	if err == nil {
		return bearer, nil
	}
	if refreshErr := s.OAuthResolver.RefreshOAuthCredentialIfBearer(ctx, credentialID, ""); refreshErr != nil {
		return credentials.ResolvedOAuthBearerCredential{}, refreshErr
	}
	return s.OAuthResolver.ResolveOAuthBearerByID(ctx, credentialID, time.Now().UTC())
}

func (s Service) oauthRows(ctx context.Context) ([]credentials.OAuthCredentialMetadata, error) {
	if s.OAuth == nil {
		return nil, nil
	}
	rows, err := s.OAuth.ListOAuthCredentials(ctx)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (s Service) refreshCredentialUsage(ctx context.Context, instance provider.Instance, bearer credentials.ResolvedOAuthBearerCredential, meta credentials.OAuthCredentialMetadata, now time.Time) (int, error) {
	credential := provider.BearerCredential{
		ID:                      bearer.ID,
		ProviderInstanceID:      bearer.ProviderInstanceID,
		Kind:                    provider.CredentialKindOAuthAccess,
		BearerToken:             bearer.BearerToken,
		ChatGPTAccountID:        bearer.ChatGPTAccountID,
		ChatGPTAccountIsFedRAMP: bearer.ChatGPTAccountIsFedRAMP,
	}
	result, err := s.UsageClient.FetchCodexSubscriptionUsage(ctx, provider.CodexSubscriptionUsageRequest{
		Instance:   instance,
		Credential: credential,
	})
	if err != nil && (result.ErrorClass == "auth_failed" || result.ErrorClass == "upstream_auth_failed") {
		_ = s.OAuthResolver.RefreshOAuthCredentialIfBearer(ctx, bearer.ID, bearer.BearerToken)
		refreshed, refreshErr := s.OAuthResolver.ResolveOAuthBearerByID(ctx, bearer.ID, time.Now().UTC())
		if refreshErr == nil {
			credential.BearerToken = refreshed.BearerToken
			credential.ChatGPTAccountID = refreshed.ChatGPTAccountID
			credential.ChatGPTAccountIsFedRAMP = refreshed.ChatGPTAccountIsFedRAMP
			result, err = s.UsageClient.FetchCodexSubscriptionUsage(ctx, provider.CodexSubscriptionUsageRequest{
				Instance:   instance,
				Credential: credential,
			})
		}
	}
	if err != nil {
		class := result.ErrorClass
		if class == "" {
			class = "unavailable"
		}
		recordErr := s.SubscriptionUsage.UpsertSubscriptionUsageSnapshot(ctx, metadata.SubscriptionUsageSnapshot{
			ObservedAt:          now,
			ProviderInstanceID:  instance.ID,
			CredentialID:        bearer.ID,
			AccountDisplayLabel: meta.AccountDisplayLabel,
			PlanLabel:           meta.PlanLabel,
			LimitID:             "codex",
			Source:              "codex_usage",
			ErrorClass:          safeErrorToken(class),
			Stale:               true,
		})
		if recordErr != nil {
			return 0, recordErr
		}
		return 1, err
	}
	count := 0
	for _, snapshot := range result.Snapshots {
		limitID := safeErrorToken(snapshot.LimitID)
		if limitID == "" || limitID == "details_redacted" {
			limitID = "codex"
		}
		if err := s.SubscriptionUsage.UpsertSubscriptionUsageSnapshot(ctx, metadata.SubscriptionUsageSnapshot{
			ObservedAt:             now,
			ProviderInstanceID:     instance.ID,
			CredentialID:           bearer.ID,
			AccountDisplayLabel:    meta.AccountDisplayLabel,
			PlanLabel:              meta.PlanLabel,
			LimitID:                limitID,
			LimitName:              safeSnapshotString(snapshot.LimitName),
			PlanType:               safeErrorToken(snapshot.PlanType),
			ReachedType:            safeErrorToken(snapshot.ReachedType),
			PrimaryUsedPercent:     windowUsed(snapshot.Primary),
			PrimaryWindowMinutes:   windowMinutes(snapshot.Primary),
			PrimaryResetAt:         windowReset(snapshot.Primary),
			SecondaryUsedPercent:   windowUsed(snapshot.Secondary),
			SecondaryWindowMinutes: windowMinutes(snapshot.Secondary),
			SecondaryResetAt:       windowReset(snapshot.Secondary),
			Source:                 "codex_usage",
		}); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func windowUsed(window *provider.CodexRateLimitWindow) float64 {
	if window == nil {
		return 0
	}
	return window.UsedPercent
}

func windowMinutes(window *provider.CodexRateLimitWindow) int {
	if window == nil {
		return 0
	}
	return window.WindowMinutes
}

func windowReset(window *provider.CodexRateLimitWindow) *time.Time {
	if window == nil || window.ResetsAt == nil {
		return nil
	}
	reset := window.ResetsAt.UTC()
	return &reset
}

func (s Service) keepaliveStatus() KeepaliveStatus {
	status := "disabled"
	if s.Keepalive.Enabled {
		status = "unavailable_output_cap_unverified"
	}
	times := append([]string(nil), s.Keepalive.ScheduleTimes...)
	for i := range times {
		times[i] = safeScheduleTime(times[i])
	}
	if len(times) == 0 {
		times = []string{"07:00", "12:00", "17:00", "22:00"}
	}
	return KeepaliveStatus{
		Enabled:           s.Keepalive.Enabled,
		Status:            status,
		OutputCapVerified: false,
		ScheduleTimes:     times,
	}
}

func safeScheduleTime(value string) string {
	value = strings.TrimSpace(value)
	if len(value) != 5 || value[2] != ':' {
		return ""
	}
	for _, i := range []int{0, 1, 3, 4} {
		if value[i] < '0' || value[i] > '9' {
			return ""
		}
	}
	return value
}
