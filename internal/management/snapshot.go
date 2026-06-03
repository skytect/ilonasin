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
	out.Providers = snapshotProviderInstances(s.Providers)
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
		rows = visibleUpstreamCredentials(rows, s.Providers)
		out.UpstreamCredentials = upstreamCredentialsFromCredentials(rows)
		poolGroups, err := s.Upstreams.ListCredentialPoolGroups(ctx)
		if err != nil {
			return ManagementSnapshotResponse{}, err
		}
		poolGroups = visibleCredentialPoolGroups(poolGroups, s.Providers)
		out.CredentialPoolGroups = credentialPoolGroupsFromCredentials(poolGroups)
		out.FallbackPolicies = append([]CredentialPoolGroup(nil), out.CredentialPoolGroups...)
	}
	if s.OAuth != nil {
		rows, err := s.OAuth.ListOAuthCredentials(ctx)
		if err != nil {
			return ManagementSnapshotResponse{}, err
		}
		rows = visibleOAuthCredentials(rows, s.Providers)
		out.OAuthCredentials = oauthCredentialsFromCredentials(rows)
		accounts, err := s.OAuth.ListProviderAccounts(ctx)
		if err != nil {
			return ManagementSnapshotResponse{}, err
		}
		accounts = visibleProviderAccounts(accounts, s.Providers)
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
