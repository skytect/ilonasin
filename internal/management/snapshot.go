package management

import (
	"context"

	"ilonasin/internal/credentials"
	"ilonasin/internal/metadata"
	"ilonasin/internal/provider"
)

const PathSnapshot = "/_ilonasin/manage/snapshot"

type SnapshotClient interface {
	LoadManagementSnapshot(ctx context.Context) (ManagementSnapshotResponse, error)
}

func (s Service) LoadManagementSnapshot(ctx context.Context) (ManagementSnapshotResponse, error) {
	var out ManagementSnapshotResponse
	for _, row := range s.Registry.List() {
		out.Providers = append(out.Providers, providerInstanceFromProvider(row))
	}
	if s.Tokens != nil {
		tokens, err := s.ListLocalTokens(ctx)
		if err != nil {
			return ManagementSnapshotResponse{}, err
		}
		out.LocalTokens = tokens.Tokens
	}
	if s.Upstreams != nil {
		rows, err := s.Upstreams.List(ctx)
		if err != nil {
			return ManagementSnapshotResponse{}, err
		}
		rows = visibleUpstreamCredentials(rows, s.Registry)
		out.UpstreamCredentials = upstreamCredentialsFromCredentials(rows)
		policies, err := s.Upstreams.ListFallbackPolicies(ctx)
		if err != nil {
			return ManagementSnapshotResponse{}, err
		}
		policies = visibleFallbackPolicies(policies, s.Registry)
		out.FallbackPolicies = fallbackPoliciesFromCredentials(policies)
	}
	if s.OAuth != nil {
		rows, err := s.OAuth.ListOAuthCredentials(ctx)
		if err != nil {
			return ManagementSnapshotResponse{}, err
		}
		rows = visibleOAuthCredentials(rows, s.Registry)
		out.OAuthCredentials = oauthCredentialsFromCredentials(rows)
		accounts, err := s.OAuth.ListProviderAccounts(ctx)
		if err != nil {
			return ManagementSnapshotResponse{}, err
		}
		accounts = visibleProviderAccounts(accounts, s.Registry)
		out.ProviderAccounts = providerAccountsFromCredentials(accounts)
	}
	if s.ModelCache != nil {
		rows, err := s.ModelCache.ListModelCache(ctx)
		if err != nil {
			return ManagementSnapshotResponse{}, err
		}
		out.ModelCache = modelMetadataFromProvider(rows)
	}
	if s.Observability != nil {
		if err := loadObservabilitySnapshot(ctx, s.Observability, &out); err != nil {
			return ManagementSnapshotResponse{}, err
		}
	}
	if s.SubscriptionUsage != nil {
		usage, err := s.GetSubscriptionUsage(ctx)
		if err != nil {
			return ManagementSnapshotResponse{}, err
		}
		out.SubscriptionUsage = usage
	}
	out.PruningAvailable = true
	sanitizeSnapshot(&out)
	return out, nil
}

func loadObservabilitySnapshot(ctx context.Context, reader ObservabilityReader, out *ManagementSnapshotResponse) error {
	requests, err := reader.RecentRequests(ctx, 5)
	if err != nil {
		return err
	}
	out.RecentRequests = requestSummariesFromMetadata(requests)
	usage, err := reader.UsageByProvider(ctx)
	if err != nil {
		return err
	}
	out.Usage = usageSummariesFromMetadata(usage)
	latency, err := reader.LatencyByProvider(ctx)
	if err != nil {
		return err
	}
	out.Latency = latencySummariesFromMetadata(latency)
	streams, err := reader.StreamSummary(ctx)
	if err != nil {
		return err
	}
	out.Streams = streamSummariesFromMetadata(streams)
	health, err := reader.LatestHealth(ctx)
	if err != nil {
		return err
	}
	out.Health = healthSummariesFromMetadata(health)
	fallbacks, err := reader.RecentFallbacks(ctx, 5)
	if err != nil {
		return err
	}
	out.Fallbacks = fallbackSummariesFromMetadata(fallbacks)
	quotas, err := reader.QuotaByProvider(ctx)
	if err != nil {
		return err
	}
	out.Quotas = quotaSummariesFromMetadata(quotas)
	return nil
}

func providerInstanceFromProvider(row provider.Instance) ProviderInstance {
	return ProviderInstance{
		ID:             row.ID,
		Type:           row.Type,
		BaseURL:        safeBaseURL(row.BaseURL),
		AuthStyle:      row.AuthStyle,
		Placeholder:    row.Placeholder,
		APIKey:         row.APIKey,
		OAuth:          row.OAuth,
		OAuthRefresh:   row.OAuthRefresh,
		Chat:           row.Chat,
		ModelDiscovery: row.ModelDiscovery,
	}
}

func visibleUpstreamCredentials(rows []credentials.UpstreamCredentialMetadata, registry provider.Registry) []credentials.UpstreamCredentialMetadata {
	allowed := apiKeyProviderIDs(registry)
	out := rows[:0]
	for _, row := range rows {
		if allowed[row.ProviderInstanceID] {
			out = append(out, row)
		}
	}
	return out
}

func visibleFallbackPolicies(rows []credentials.FallbackPolicyMetadata, registry provider.Registry) []credentials.FallbackPolicyMetadata {
	allowed := fallbackPolicyProviderKinds(registry)
	out := rows[:0]
	for _, row := range rows {
		if allowed[row.ProviderInstanceID][row.CredentialKind] && row.CredentialCount >= 2 {
			out = append(out, row)
		}
	}
	return out
}

func fallbackPolicyProviderKinds(registry provider.Registry) map[string]map[string]bool {
	allowed := map[string]map[string]bool{}
	for _, instance := range registry.List() {
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
	return allowed
}

func apiKeyProviderIDs(registry provider.Registry) map[string]bool {
	allowed := map[string]bool{}
	for _, instance := range registry.List() {
		if instance.APIKey && !instance.Placeholder {
			allowed[instance.ID] = true
		}
	}
	return allowed
}

func visibleOAuthCredentials(rows []credentials.OAuthCredentialMetadata, registry provider.Registry) []credentials.OAuthCredentialMetadata {
	allowed := oauthProviderIDs(registry)
	out := rows[:0]
	for _, row := range rows {
		if allowed[row.ProviderInstanceID] {
			out = append(out, row)
		}
	}
	return out
}

func visibleProviderAccounts(rows []credentials.ProviderAccountMetadata, registry provider.Registry) []credentials.ProviderAccountMetadata {
	allowed := oauthProviderIDs(registry)
	out := rows[:0]
	for _, row := range rows {
		if allowed[row.ProviderInstanceID] {
			out = append(out, row)
		}
	}
	return out
}

func oauthProviderIDs(registry provider.Registry) map[string]bool {
	allowed := map[string]bool{}
	for _, instance := range registry.List() {
		if instance.OAuth {
			allowed[instance.ID] = true
		}
	}
	return allowed
}

func upstreamCredentialsFromCredentials(rows []credentials.UpstreamCredentialMetadata) []UpstreamCredential {
	out := make([]UpstreamCredential, 0, len(rows))
	for _, row := range rows {
		out = append(out, upstreamCredentialFromCredentials(row))
	}
	return out
}

func fallbackPoliciesFromCredentials(rows []credentials.FallbackPolicyMetadata) []FallbackPolicy {
	out := make([]FallbackPolicy, 0, len(rows))
	for _, row := range rows {
		out = append(out, fallbackPolicyFromCredentials(row))
	}
	return out
}

func oauthCredentialsFromCredentials(rows []credentials.OAuthCredentialMetadata) []OAuthCredential {
	out := make([]OAuthCredential, 0, len(rows))
	for _, row := range rows {
		out = append(out, OAuthCredential{
			ID:                        row.ID,
			ProviderInstanceID:        row.ProviderInstanceID,
			Label:                     row.Label,
			AccountDisplayLabel:       row.AccountDisplayLabel,
			PlanLabel:                 row.PlanLabel,
			Scopes:                    row.Scopes,
			ExpiresAt:                 row.ExpiresAt,
			LastRefreshAt:             row.LastRefreshAt,
			RefreshFailureClass:       row.RefreshFailureClass,
			RefreshFailureDescription: row.RefreshFailureDescription,
			CreatedAt:                 row.CreatedAt,
			DisabledAt:                row.DisabledAt,
			Disabled:                  row.Disabled,
		})
	}
	return out
}

func providerAccountsFromCredentials(rows []credentials.ProviderAccountMetadata) []ProviderAccount {
	out := make([]ProviderAccount, 0, len(rows))
	for _, row := range rows {
		out = append(out, ProviderAccount{
			ID:                 row.ID,
			ProviderInstanceID: row.ProviderInstanceID,
			CredentialID:       row.CredentialID,
			DisplayLabel:       row.DisplayLabel,
			PlanLabel:          row.PlanLabel,
			CreatedAt:          row.CreatedAt,
		})
	}
	return out
}

func modelMetadataFromProvider(rows []provider.ModelMetadata) []ModelMetadata {
	out := make([]ModelMetadata, 0, len(rows))
	for _, row := range rows {
		out = append(out, ModelMetadata{
			ProviderInstanceID: row.ProviderInstanceID,
			ModelID:            row.ModelID,
			DisplayName:        row.DisplayName,
			Capabilities:       row.CapabilityFlags,
			UpdatedAt:          row.UpdatedAt,
		})
	}
	return out
}

func requestSummariesFromMetadata(rows []metadata.RequestSummary) []RequestSummary {
	out := make([]RequestSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, RequestSummary{
			ID:                             row.ID,
			StartedAt:                      row.StartedAt,
			ProviderInstanceID:             row.ProviderInstanceID,
			ModelID:                        row.ModelID,
			Endpoint:                       row.Endpoint,
			Stream:                         row.Stream,
			ProviderType:                   row.ProviderType,
			MessageCount:                   row.MessageCount,
			ToolCount:                      row.ToolCount,
			ImageCount:                     row.ImageCount,
			RequestedServiceTier:           row.RequestedServiceTier,
			EffectiveServiceTier:           row.EffectiveServiceTier,
			ReasoningEffort:                row.ReasoningEffort,
			ReasoningSummary:               row.ReasoningSummary,
			ReasoningMaxTokens:             row.ReasoningMaxTokens,
			ReasoningEnabled:               row.ReasoningEnabled,
			ReasoningExclude:               row.ReasoningExclude,
			ThinkingType:                   row.ThinkingType,
			MaxOutputTokens:                row.MaxOutputTokens,
			RequestedProviderID:            row.RequestedProviderID,
			RequestedModelID:               row.RequestedModelID,
			ResolvedProviderID:             row.ResolvedProviderID,
			ResolvedModelID:                row.ResolvedModelID,
			CredentialID:                   row.CredentialID,
			CredentialLabel:                row.CredentialLabel,
			HTTPStatus:                     row.HTTPStatus,
			ErrorClass:                     row.ErrorClass,
			RetryCount:                     row.RetryCount,
			AuthRetryCount:                 row.AuthRetryCount,
			AttemptCount:                   row.AttemptCount,
			FallbackCount:                  row.FallbackCount,
			FallbackReason:                 row.FallbackReason,
			PromptTokens:                   row.PromptTokens,
			CompletionTokens:               row.CompletionTokens,
			TotalTokens:                    row.TotalTokens,
			ReasoningTokens:                row.ReasoningTokens,
			CacheHitTokens:                 row.CacheHitTokens,
			CacheMissTokens:                row.CacheMissTokens,
			CacheWriteTokens:               row.CacheWriteTokens,
			ReasoningTokenRate:             row.ReasoningTokenRate,
			CacheHitRate:                   row.CacheHitRate,
			CacheMissRate:                  row.CacheMissRate,
			CacheWriteRate:                 row.CacheWriteRate,
			CostMicrounits:                 row.CostMicrounits,
			TotalLatencyMS:                 row.TotalLatencyMS,
			UpstreamLatencyMS:              row.UpstreamLatencyMS,
			TimeToFirstTokenMS:             row.TimeToFirstTokenMS,
			OutputTokensPerSecond:          row.OutputTokensPerSecond,
			OutputTokensPerSecondTotal:     row.OutputTokensPerSecondTotal,
			OutputTokensPerSecondAfterTTFT: row.OutputTokensPerSecondAfterTTFT,
			StreamCompletionStatus:         row.StreamCompletionStatus,
			StreamChunkCount:               row.StreamChunkCount,
		})
	}
	return out
}

func usageSummariesFromMetadata(rows []metadata.UsageSummary) []UsageSummary {
	out := make([]UsageSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, UsageSummary{
			ProviderInstanceID: row.ProviderInstanceID,
			RequestCount:       row.RequestCount,
			PromptTokens:       row.PromptTokens,
			CompletionTokens:   row.CompletionTokens,
			TotalTokens:        row.TotalTokens,
			ReasoningTokens:    row.ReasoningTokens,
			CacheHitTokens:     row.CacheHitTokens,
			CacheMissTokens:    row.CacheMissTokens,
			CacheWriteTokens:   row.CacheWriteTokens,
			ReasoningTokenRate: row.ReasoningTokenRate,
			CacheHitRate:       row.CacheHitRate,
			CacheMissRate:      row.CacheMissRate,
			CacheWriteRate:     row.CacheWriteRate,
			CostMicrounits:     row.CostMicrounits,
		})
	}
	return out
}

func latencySummariesFromMetadata(rows []metadata.LatencySummary) []LatencySummary {
	out := make([]LatencySummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, LatencySummary{
			ProviderInstanceID:        row.ProviderInstanceID,
			RequestCount:              row.RequestCount,
			AverageLatencyMS:          row.AverageLatencyMS,
			AverageUpstreamLatencyMS:  row.AverageUpstreamLatencyMS,
			AverageTimeToFirstTokenMS: row.AverageTimeToFirstTokenMS,
			AverageOutputTPS:          row.AverageOutputTPS,
			AverageOutputTPSTotal:     row.AverageOutputTPSTotal,
			AverageOutputTPSAfterTTFT: row.AverageOutputTPSAfterTTFT,
		})
	}
	return out
}

func streamSummariesFromMetadata(rows []metadata.StreamSummary) []StreamSummary {
	out := make([]StreamSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, StreamSummary{
			CompletionStatus: row.CompletionStatus,
			StreamCount:      row.StreamCount,
			ChunkCount:       row.ChunkCount,
		})
	}
	return out
}

func healthSummariesFromMetadata(rows []metadata.HealthSummary) []HealthSummary {
	out := make([]HealthSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, HealthSummary{
			ProviderInstanceID: row.ProviderInstanceID,
			ModelID:            row.ModelID,
			CredentialID:       row.CredentialID,
			CredentialLabel:    row.CredentialLabel,
			EventClass:         row.EventClass,
			HTTPStatus:         row.HTTPStatus,
			ErrorClass:         row.ErrorClass,
			OccurredAt:         row.OccurredAt,
			RetryAfter:         row.RetryAfter,
		})
	}
	return out
}

func fallbackSummariesFromMetadata(rows []metadata.FallbackSummary) []FallbackSummary {
	out := make([]FallbackSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, FallbackSummary{
			ID:                  row.ID,
			RequestMetadataID:   row.RequestMetadataID,
			OccurredAt:          row.OccurredAt,
			ProviderInstanceID:  row.ProviderInstanceID,
			ModelID:             row.ModelID,
			FromCredentialID:    row.FromCredentialID,
			FromCredentialLabel: row.FromCredentialLabel,
			ToCredentialID:      row.ToCredentialID,
			ToCredentialLabel:   row.ToCredentialLabel,
			Reason:              row.Reason,
		})
	}
	return out
}

func quotaSummariesFromMetadata(rows []metadata.QuotaSummary) []QuotaSummary {
	out := make([]QuotaSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, QuotaSummary{
			ObservedAt:         row.ObservedAt,
			ProviderInstanceID: row.ProviderInstanceID,
			ModelID:            row.ModelID,
			CredentialID:       row.CredentialID,
			CredentialLabel:    row.CredentialLabel,
			Source:             row.Source,
			HTTPStatus:         row.HTTPStatus,
			ErrorClass:         row.ErrorClass,
			RetryAfter:         row.RetryAfter,
			ResetAt:            row.ResetAt,
			Count:              row.Count,
		})
	}
	return out
}
