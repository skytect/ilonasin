package management

import (
	"context"

	"ilonasin/internal/credentials"
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
