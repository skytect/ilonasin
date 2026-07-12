package management

import (
	"context"
	"time"

	"ilonasin/internal/credentials"
	"ilonasin/internal/metadata"
)

const PathSubscriptionUsage = "/_ilonasin/manage/subscription-usage"

type SubscriptionUsageClient interface {
	GetSubscriptionUsage(ctx context.Context) (SubscriptionUsageResponse, error)
	RefreshSubscriptionUsage(ctx context.Context) (SubscriptionUsageResponse, error)
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
	for _, instance := range s.Providers {
		if !SupportsCodexOAuth(instance) {
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

func (s Service) refreshCredentialUsage(ctx context.Context, instance ProviderInstance, bearer credentials.ResolvedOAuthBearerCredential, meta credentials.OAuthCredentialMetadata, now time.Time) (int, error) {
	req := subscriptionUsageFetchRequest(instance, bearer)
	result, err := s.UsageClient.FetchSubscriptionUsage(ctx, req)
	if err != nil && (result.ErrorClass == "auth_failed" || result.ErrorClass == "upstream_auth_failed") {
		_ = s.OAuthResolver.RefreshOAuthCredentialIfBearer(ctx, bearer.ID, bearer.BearerToken)
		refreshed, refreshErr := s.OAuthResolver.ResolveOAuthBearerByID(ctx, bearer.ID, time.Now().UTC())
		if refreshErr == nil {
			req.Credential.BearerToken = refreshed.BearerToken
			req.Credential.ChatGPTAccountID = refreshed.ChatGPTAccountID
			req.Credential.ChatGPTAccountIsFedRAMP = refreshed.ChatGPTAccountIsFedRAMP
			result, err = s.UsageClient.FetchSubscriptionUsage(ctx, req)
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
		var bankedResetInventory metadata.BankedResetInventory
		if limitID == "codex" {
			bankedResetInventory = subscriptionUsageBankedResetInventory(result.BankedResetInventory)
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
			BankedResetInventory:   bankedResetInventory,
		}); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func subscriptionUsageFetchRequest(instance ProviderInstance, bearer credentials.ResolvedOAuthBearerCredential) SubscriptionUsageFetchRequest {
	return SubscriptionUsageFetchRequest{
		Provider: SubscriptionUsageProvider{
			ID:             instance.ID,
			Type:           instance.Type,
			BaseURL:        instance.BaseURL,
			AuthIssuer:     instance.AuthIssuer,
			AuthStyle:      instance.AuthStyle,
			APIKey:         instance.APIKey,
			OAuth:          instance.OAuth,
			OAuthRefresh:   instance.OAuthRefresh,
			Chat:           instance.Chat,
			ModelDiscovery: instance.ModelDiscovery,
		},
		Credential: SubscriptionUsageBearerCredential{
			ID:                      bearer.ID,
			ProviderInstanceID:      bearer.ProviderInstanceID,
			BearerToken:             bearer.BearerToken,
			ChatGPTAccountID:        bearer.ChatGPTAccountID,
			ChatGPTAccountIsFedRAMP: bearer.ChatGPTAccountIsFedRAMP,
		},
	}
}

func subscriptionUsageBankedResetInventory(in SubscriptionUsageFetchBankedResetInventory) metadata.BankedResetInventory {
	out := metadata.BankedResetInventory{
		AvailableCount:   in.AvailableCount,
		DetailsAvailable: in.DetailsAvailable,
		DetailErrorClass: safeErrorToken(in.DetailErrorClass),
	}
	if out.DetailErrorClass == "details_redacted" {
		out.DetailErrorClass = ""
	}
	out.Details = make([]metadata.BankedResetDetail, 0, len(in.Details))
	for _, detail := range in.Details {
		resetType := safeErrorToken(detail.ResetType)
		if resetType == "details_redacted" || resetType == "" {
			resetType = "unknown"
		}
		status := safeErrorToken(detail.Status)
		if status == "details_redacted" || status == "" {
			status = "unknown"
		}
		out.Details = append(out.Details, metadata.BankedResetDetail{
			ResetType: resetType,
			Status:    status,
			GrantedAt: detail.GrantedAt.UTC(),
			ExpiresAt: cloneTime(detail.ExpiresAt),
		})
	}
	return out
}
