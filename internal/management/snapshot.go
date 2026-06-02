package management

import (
	"context"
)

const PathSnapshot = "/_ilonasin/manage/snapshot"

type SnapshotClient interface {
	LoadManagementSnapshot(ctx context.Context) (ManagementSnapshotResponse, error)
}

func (s Service) LoadManagementSnapshot(ctx context.Context) (ManagementSnapshotResponse, error) {
	out := ManagementSnapshotResponse{Runtime: s.Runtime}
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
		out.ModelCache = modelMetadataFromMetadata(rows)
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
