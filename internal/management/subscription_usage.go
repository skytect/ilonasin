package management

import (
	"context"
	"fmt"
	"math"
	"sort"
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
	ObservedAt                time.Time  `json:"observed_at"`
	ProviderInstanceID        string     `json:"provider_instance_id"`
	CredentialID              int64      `json:"credential_id"`
	AccountDisplayLabel       string     `json:"account_display_label"`
	PlanLabel                 string     `json:"plan_label"`
	LimitID                   string     `json:"limit_id"`
	LimitName                 string     `json:"limit_name"`
	PlanType                  string     `json:"plan_type"`
	ReachedType               string     `json:"reached_type"`
	PrimaryLabel              string     `json:"primary_label"`
	PrimaryUsedPercent        float64    `json:"primary_used_percent"`
	PrimaryRemainingPercent   float64    `json:"primary_remaining_percent"`
	PrimaryWindowMinutes      int        `json:"primary_window_minutes"`
	PrimaryResetAt            *time.Time `json:"primary_reset_at,omitempty"`
	SecondaryLabel            string     `json:"secondary_label"`
	SecondaryUsedPercent      float64    `json:"secondary_used_percent"`
	SecondaryRemainingPercent float64    `json:"secondary_remaining_percent"`
	SecondaryWindowMinutes    int        `json:"secondary_window_minutes"`
	SecondaryResetAt          *time.Time `json:"secondary_reset_at,omitempty"`
	Source                    string     `json:"source"`
	ErrorClass                string     `json:"error_class"`
	Stale                     bool       `json:"stale"`
}

type SubscriptionUsageAggregate struct {
	ProviderInstanceID               string     `json:"provider_instance_id"`
	LimitID                          string     `json:"limit_id"`
	LimitName                        string     `json:"limit_name"`
	AccountCount                     int        `json:"account_count"`
	StaleCount                       int        `json:"stale_count"`
	AveragePrimaryUsedPercent        float64    `json:"average_primary_used_percent"`
	MinimumPrimaryRemainingPercent   float64    `json:"minimum_primary_remaining_percent"`
	EarliestPrimaryResetAt           *time.Time `json:"earliest_primary_reset_at,omitempty"`
	AverageSecondaryUsedPercent      float64    `json:"average_secondary_used_percent"`
	MinimumSecondaryRemainingPercent float64    `json:"minimum_secondary_remaining_percent"`
	EarliestSecondaryResetAt         *time.Time `json:"earliest_secondary_reset_at,omitempty"`
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
	return subscriptionUsageResponse(rows, s.keepaliveStatus()), nil
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

func subscriptionUsageResponse(rows []metadata.SubscriptionUsageSnapshot, keepalive KeepaliveStatus) SubscriptionUsageResponse {
	accountRows := make([]SubscriptionUsageRow, 0, len(rows))
	for _, row := range rows {
		accountRows = append(accountRows, subscriptionUsageRow(row))
	}
	return SubscriptionUsageResponse{
		ObservedAt: latestSubscriptionObserved(accountRows),
		Accounts:   accountRows,
		Pools:      subscriptionUsageAggregates(accountRows),
		Keepalive:  keepalive,
	}
}

func subscriptionUsageRow(row metadata.SubscriptionUsageSnapshot) SubscriptionUsageRow {
	return SubscriptionUsageRow{
		ObservedAt:                row.ObservedAt,
		ProviderInstanceID:        row.ProviderInstanceID,
		CredentialID:              row.CredentialID,
		AccountDisplayLabel:       row.AccountDisplayLabel,
		PlanLabel:                 row.PlanLabel,
		LimitID:                   row.LimitID,
		LimitName:                 row.LimitName,
		PlanType:                  row.PlanType,
		ReachedType:               row.ReachedType,
		PrimaryLabel:              windowLabel(row.PrimaryWindowMinutes),
		PrimaryUsedPercent:        boundedPercent(row.PrimaryUsedPercent),
		PrimaryRemainingPercent:   remainingPercent(row.PrimaryUsedPercent),
		PrimaryWindowMinutes:      row.PrimaryWindowMinutes,
		PrimaryResetAt:            cloneTime(row.PrimaryResetAt),
		SecondaryLabel:            windowLabel(row.SecondaryWindowMinutes),
		SecondaryUsedPercent:      boundedPercent(row.SecondaryUsedPercent),
		SecondaryRemainingPercent: remainingPercent(row.SecondaryUsedPercent),
		SecondaryWindowMinutes:    row.SecondaryWindowMinutes,
		SecondaryResetAt:          cloneTime(row.SecondaryResetAt),
		Source:                    row.Source,
		ErrorClass:                row.ErrorClass,
		Stale:                     row.Stale,
	}
}

func subscriptionUsageAggregates(rows []SubscriptionUsageRow) []SubscriptionUsageAggregate {
	type bucket struct {
		agg          SubscriptionUsageAggregate
		primarySum   float64
		secondarySum float64
	}
	buckets := map[string]*bucket{}
	for _, row := range rows {
		key := row.ProviderInstanceID + "\x00" + row.LimitID
		b := buckets[key]
		if b == nil {
			b = &bucket{agg: SubscriptionUsageAggregate{
				ProviderInstanceID:               row.ProviderInstanceID,
				LimitID:                          row.LimitID,
				LimitName:                        row.LimitName,
				MinimumPrimaryRemainingPercent:   100,
				MinimumSecondaryRemainingPercent: 100,
			}}
			buckets[key] = b
		}
		b.agg.AccountCount++
		if row.Stale || row.ErrorClass != "" {
			b.agg.StaleCount++
		}
		b.primarySum += row.PrimaryUsedPercent
		b.secondarySum += row.SecondaryUsedPercent
		b.agg.MinimumPrimaryRemainingPercent = math.Min(b.agg.MinimumPrimaryRemainingPercent, row.PrimaryRemainingPercent)
		b.agg.MinimumSecondaryRemainingPercent = math.Min(b.agg.MinimumSecondaryRemainingPercent, row.SecondaryRemainingPercent)
		b.agg.EarliestPrimaryResetAt = earliestTime(b.agg.EarliestPrimaryResetAt, row.PrimaryResetAt)
		b.agg.EarliestSecondaryResetAt = earliestTime(b.agg.EarliestSecondaryResetAt, row.SecondaryResetAt)
	}
	out := make([]SubscriptionUsageAggregate, 0, len(buckets))
	for _, b := range buckets {
		if b.agg.AccountCount > 0 {
			b.agg.AveragePrimaryUsedPercent = b.primarySum / float64(b.agg.AccountCount)
			b.agg.AverageSecondaryUsedPercent = b.secondarySum / float64(b.agg.AccountCount)
		}
		out = append(out, b.agg)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ProviderInstanceID != out[j].ProviderInstanceID {
			return out[i].ProviderInstanceID < out[j].ProviderInstanceID
		}
		return out[i].LimitID < out[j].LimitID
	})
	return out
}

func latestSubscriptionObserved(rows []SubscriptionUsageRow) time.Time {
	var out time.Time
	for _, row := range rows {
		if row.ObservedAt.After(out) {
			out = row.ObservedAt
		}
	}
	return out
}

func earliestTime(current, candidate *time.Time) *time.Time {
	if candidate == nil {
		return current
	}
	next := candidate.UTC()
	if current == nil || next.Before(*current) {
		return &next
	}
	return current
}

func windowLabel(minutes int) string {
	switch minutes {
	case 300:
		return "5h"
	case 10080:
		return "weekly"
	case 0:
		return ""
	default:
		if minutes%1440 == 0 {
			return fmt.Sprintf("%dd", minutes/1440)
		}
		if minutes%60 == 0 {
			return fmt.Sprintf("%dh", minutes/60)
		}
		return fmt.Sprintf("%dm", minutes)
	}
}

func remainingPercent(used float64) float64 {
	return 100 - boundedPercent(used)
}

func boundedPercent(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	clone := value.UTC()
	return &clone
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
