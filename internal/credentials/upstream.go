package credentials

import (
	"context"
	"errors"
	"fmt"
	"time"

	"ilonasin/internal/provider"
)

var (
	ErrCredentialNotFound    = errors.New("credential not found")
	ErrNoEligibleCredential  = errors.New("no eligible credential")
	ErrUnsupportedCredential = errors.New("unsupported credential")
	ErrDuplicateCredential   = errors.New("duplicate credential")
	ErrInvalidSecretDomain   = errors.New("invalid secret domain")
)

type UpstreamCredentialManager interface {
	AddAPIKey(ctx context.Context, providerInstanceID, label, apiKey string) (UpstreamCredentialMetadata, error)
	List(ctx context.Context) ([]UpstreamCredentialMetadata, error)
	Disable(ctx context.Context, id int64) error
}

type UpstreamCredentialResolver interface {
	ResolveAPIKey(ctx context.Context, providerInstanceID string) (ResolvedAPIKeyCredential, error)
}

type UpstreamCredentialRepository interface {
	InsertAPIKeyCredential(ctx context.Context, meta NewUpstreamCredential, apiKey string) (UpstreamCredentialMetadata, error)
	ListUpstreamCredentials(ctx context.Context) ([]UpstreamCredentialMetadata, error)
	DisableUpstreamCredential(ctx context.Context, id int64, disabledAt time.Time) error
	ResolveAPIKeyCredential(ctx context.Context, providerInstanceID string) (ResolvedAPIKeyCredential, error)
}

type UpstreamService struct {
	Registry provider.Registry
	Repo     UpstreamCredentialRepository
	Now      func() time.Time
}

type NewUpstreamCredential struct {
	ProviderInstanceID string
	Kind               string
	Label              string
	SecretPrefix       string
	SecretLast4        string
	CreatedAt          time.Time
}

type UpstreamCredentialMetadata struct {
	ID                 int64
	ProviderInstanceID string
	Kind               string
	Label              string
	SecretPrefix       string
	SecretLast4        string
	CreatedAt          time.Time
	DisabledAt         *time.Time
	Disabled           bool
}

type ResolvedAPIKeyCredential struct {
	ID                 int64
	ProviderInstanceID string
	Label              string
	APIKey             string
}

func (c ResolvedAPIKeyCredential) String() string {
	return fmt.Sprintf("ResolvedAPIKeyCredential{ID:%d ProviderInstanceID:%q Label:%q APIKey:%q}", c.ID, c.ProviderInstanceID, c.Label, RedactSecret(c.APIKey))
}

func (c ResolvedAPIKeyCredential) GoString() string {
	return c.String()
}

func (s UpstreamService) AddAPIKey(ctx context.Context, providerInstanceID, label, apiKey string) (UpstreamCredentialMetadata, error) {
	if label == "" {
		label = "api key"
	}
	if len(apiKey) <= 12 {
		return UpstreamCredentialMetadata{}, ErrInvalidSecretDomain
	}
	if LooksLikeLocalToken(apiKey) {
		return UpstreamCredentialMetadata{}, ErrInvalidSecretDomain
	}
	instance, ok := s.Registry.Get(providerInstanceID)
	if !ok {
		return UpstreamCredentialMetadata{}, ErrCredentialNotFound
	}
	if !instance.APIKey || instance.Placeholder {
		return UpstreamCredentialMetadata{}, fmt.Errorf("%w: provider %q does not support api-key credentials", ErrUnsupportedCredential, providerInstanceID)
	}
	return s.Repo.InsertAPIKeyCredential(ctx, NewUpstreamCredential{
		ProviderInstanceID: providerInstanceID,
		Kind:               "api_key",
		Label:              label,
		SecretPrefix:       Prefix(apiKey),
		SecretLast4:        Last4(apiKey),
		CreatedAt:          s.now(),
	}, apiKey)
}

func (s UpstreamService) List(ctx context.Context) ([]UpstreamCredentialMetadata, error) {
	return s.Repo.ListUpstreamCredentials(ctx)
}

func (s UpstreamService) Disable(ctx context.Context, id int64) error {
	return s.Repo.DisableUpstreamCredential(ctx, id, s.now())
}

func (s UpstreamService) ResolveAPIKey(ctx context.Context, providerInstanceID string) (ResolvedAPIKeyCredential, error) {
	instance, ok := s.Registry.Get(providerInstanceID)
	if !ok {
		return ResolvedAPIKeyCredential{}, ErrCredentialNotFound
	}
	if !instance.APIKey || instance.Placeholder {
		return ResolvedAPIKeyCredential{}, fmt.Errorf("%w: provider %q does not support api-key credentials", ErrUnsupportedCredential, providerInstanceID)
	}
	return s.Repo.ResolveAPIKeyCredential(ctx, providerInstanceID)
}

func LooksLikeLocalToken(value string) bool {
	return len(value) > 4 && value[:4] == "iln_"
}

func (s UpstreamService) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
