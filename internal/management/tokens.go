package management

import (
	"context"
	"time"

	"ilonasin/internal/credentials"
	"ilonasin/internal/metadata"
	"ilonasin/internal/provider"
)

type LocalToken struct {
	ID          int64      `json:"id"`
	Label       string     `json:"label"`
	TokenPrefix string     `json:"token_prefix"`
	TokenLast4  string     `json:"token_last4"`
	CreatedAt   time.Time  `json:"created_at"`
	DisabledAt  *time.Time `json:"disabled_at,omitempty"`
	Disabled    bool       `json:"disabled"`
}

type ListLocalTokensResponse struct {
	Tokens []LocalToken `json:"tokens"`
}

type CreateLocalTokenRequest struct {
	Label string `json:"label"`
}

type CreateLocalTokenResponse struct {
	Token    string     `json:"token"`
	Metadata LocalToken `json:"metadata"`
}

type DisableLocalTokenRequest struct {
	ID int64 `json:"id"`
}

type DisableLocalTokenResponse struct {
	Disabled bool `json:"disabled"`
}

type LocalTokenClient interface {
	ListLocalTokens(ctx context.Context) (ListLocalTokensResponse, error)
	CreateLocalToken(ctx context.Context, req CreateLocalTokenRequest) (CreateLocalTokenResponse, error)
	DisableLocalToken(ctx context.Context, req DisableLocalTokenRequest) (DisableLocalTokenResponse, error)
}

type Service struct {
	Tokens            credentials.LocalTokenManager
	Registry          provider.Registry
	Upstreams         UpstreamMetadataReader
	UpstreamMutations UpstreamMutationManager
	OAuth             OAuthMetadataReader
	OAuthMutations    OAuthMutationManager
	ModelCache        ModelCacheReader
	Observability     ObservabilityReader
}

type UpstreamMetadataReader interface {
	List(ctx context.Context) ([]credentials.UpstreamCredentialMetadata, error)
	ListFallbackPolicies(ctx context.Context) ([]credentials.FallbackPolicyMetadata, error)
}

type OAuthMetadataReader interface {
	ListOAuthCredentials(ctx context.Context) ([]credentials.OAuthCredentialMetadata, error)
	ListProviderAccounts(ctx context.Context) ([]credentials.ProviderAccountMetadata, error)
}

type ModelCacheReader interface {
	ListModelCache(ctx context.Context) ([]provider.ModelMetadata, error)
}

type ObservabilityReader interface {
	RecentRequests(ctx context.Context, limit int) ([]metadata.RequestSummary, error)
	UsageByProvider(ctx context.Context) ([]metadata.UsageSummary, error)
	LatencyByProvider(ctx context.Context) ([]metadata.LatencySummary, error)
	StreamSummary(ctx context.Context) ([]metadata.StreamSummary, error)
	LatestHealth(ctx context.Context) ([]metadata.HealthSummary, error)
	RecentFallbacks(ctx context.Context, limit int) ([]metadata.FallbackSummary, error)
	QuotaByProvider(ctx context.Context) ([]metadata.QuotaSummary, error)
}

func (s Service) ListLocalTokens(ctx context.Context) (ListLocalTokensResponse, error) {
	rows, err := s.Tokens.List(ctx)
	if err != nil {
		return ListLocalTokensResponse{}, err
	}
	out := make([]LocalToken, 0, len(rows))
	for _, row := range rows {
		out = append(out, localTokenFromCredentials(row))
	}
	return ListLocalTokensResponse{Tokens: out}, nil
}

func (s Service) CreateLocalToken(ctx context.Context, req CreateLocalTokenRequest) (CreateLocalTokenResponse, error) {
	created, err := s.Tokens.Create(ctx, req.Label)
	if err != nil {
		return CreateLocalTokenResponse{}, err
	}
	return CreateLocalTokenResponse{
		Token:    created.Token,
		Metadata: localTokenFromCredentials(created.Metadata),
	}, nil
}

func (s Service) DisableLocalToken(ctx context.Context, req DisableLocalTokenRequest) (DisableLocalTokenResponse, error) {
	if err := s.Tokens.Disable(ctx, req.ID); err != nil {
		return DisableLocalTokenResponse{}, err
	}
	return DisableLocalTokenResponse{Disabled: true}, nil
}

func localTokenFromCredentials(row credentials.LocalTokenMetadata) LocalToken {
	return LocalToken{
		ID:          row.ID,
		Label:       row.Label,
		TokenPrefix: row.TokenPrefix,
		TokenLast4:  row.TokenLast4,
		CreatedAt:   row.CreatedAt,
		DisabledAt:  row.DisabledAt,
		Disabled:    row.Disabled,
	}
}
