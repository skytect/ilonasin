package credentials

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"ilonasin/internal/provider"
)

var (
	ErrCredentialNotFound    = errors.New("credential not found")
	ErrNoEligibleCredential  = errors.New("no eligible credential")
	ErrUnsupportedCredential = errors.New("unsupported credential")
	ErrDuplicateCredential   = errors.New("duplicate credential")
	ErrInvalidSecretDomain   = errors.New("invalid secret domain")
	ErrInvalidOAuthInput     = errors.New("invalid oauth input")
	ErrOAuthRefreshFailed    = errors.New("oauth refresh failed")
)

const DefaultFallbackGroup = "default"

type UpstreamCredentialManager interface {
	AddAPIKey(ctx context.Context, providerInstanceID, label, apiKey string) (UpstreamCredentialMetadata, error)
	List(ctx context.Context) ([]UpstreamCredentialMetadata, error)
	ListFallbackPolicies(ctx context.Context) ([]FallbackPolicyMetadata, error)
	Disable(ctx context.Context, id int64) error
	EnableFallbackGroup(ctx context.Context, providerInstanceID, groupLabel string) error
	DisableFallbackGroup(ctx context.Context, providerInstanceID, groupLabel string) error
}

type UpstreamCredentialResolver interface {
	ResolveAPIKey(ctx context.Context, providerInstanceID string) (ResolvedAPIKeyCredential, error)
	ResolveAPIKeys(ctx context.Context, providerInstanceID string) ([]ResolvedAPIKeyCredential, error)
}

type OAuthBearerResolver interface {
	ResolveOAuthBearer(ctx context.Context, providerInstanceID string, now time.Time) (ResolvedOAuthBearerCredential, error)
}

type UpstreamCredentialRepository interface {
	InsertAPIKeyCredential(ctx context.Context, meta NewUpstreamCredential, apiKey string) (UpstreamCredentialMetadata, error)
	ListUpstreamCredentials(ctx context.Context) ([]UpstreamCredentialMetadata, error)
	DisableUpstreamCredential(ctx context.Context, id int64, disabledAt time.Time) error
	ResolveAPIKeyCredential(ctx context.Context, providerInstanceID string) (ResolvedAPIKeyCredential, error)
	ResolveAPIKeyCredentials(ctx context.Context, providerInstanceID string) ([]ResolvedAPIKeyCredential, error)
	ResolveOAuthBearerCredential(ctx context.Context, providerInstanceID string, now time.Time) (ResolvedOAuthBearerCredential, error)
	ResolveOAuthRefreshCredential(ctx context.Context, credentialID int64) (ResolvedOAuthRefreshCredential, error)
	ResolveOAuthRefreshToken(ctx context.Context, credentialID, refreshSecretID int64) (string, error)
	UpdateOAuthTokens(ctx context.Context, credentialID int64, update OAuthTokenUpdate) error
	ListFallbackPolicies(ctx context.Context) ([]FallbackPolicyMetadata, error)
	SetFallbackGroupEnabled(ctx context.Context, providerInstanceID, groupLabel string, enabled bool, now time.Time) error
	InsertOAuthCredential(ctx context.Context, meta NewOAuthCredential, accessToken, refreshToken string) (OAuthCredentialMetadata, error)
	ListOAuthCredentials(ctx context.Context) ([]OAuthCredentialMetadata, error)
	ListProviderAccounts(ctx context.Context) ([]ProviderAccountMetadata, error)
	MarkOAuthRefreshFailure(ctx context.Context, credentialID int64, failureClass string, now time.Time) error
}

type OAuthCredentialManager interface {
	AddOAuthCredential(ctx context.Context, input NewOAuthCredentialInput) (OAuthCredentialMetadata, error)
	MarkOAuthRefreshFailure(ctx context.Context, credentialID int64, failureClass string) error
	RefreshOAuthCredential(ctx context.Context, credentialID int64) error
}

type OAuthMetadataReader interface {
	ListOAuthCredentials(ctx context.Context) ([]OAuthCredentialMetadata, error)
	ListProviderAccounts(ctx context.Context) ([]ProviderAccountMetadata, error)
}

type OAuthRefreshController interface {
	RefreshOAuthCredential(ctx context.Context, credentialID int64) error
}

type UpstreamService struct {
	Registry       provider.Registry
	Repo           UpstreamCredentialRepository
	OAuthRefresher provider.OAuthTokenRefresher
	Now            func() time.Time
}

type NewUpstreamCredential struct {
	ProviderInstanceID string
	Kind               string
	Label              string
	SecretPrefix       string
	SecretLast4        string
	FallbackGroup      string
	CreatedAt          time.Time
}

type UpstreamCredentialMetadata struct {
	ID                 int64
	ProviderInstanceID string
	Kind               string
	Label              string
	SecretPrefix       string
	SecretLast4        string
	FallbackGroup      string
	CreatedAt          time.Time
	DisabledAt         *time.Time
	Disabled           bool
}

type NewOAuthCredentialInput struct {
	ProviderInstanceID  string
	Label               string
	AccessToken         string
	RefreshToken        string
	AccountID           string
	AccountDisplayLabel string
	PlanLabel           string
	Scopes              string
	ExpiresAt           *time.Time
}

type NewOAuthCredential struct {
	ProviderInstanceID  string
	Label               string
	AccountHash         string
	AccountDisplayLabel string
	PlanLabel           string
	Scopes              string
	ExpiresAt           *time.Time
	CreatedAt           time.Time
}

type OAuthCredentialMetadata struct {
	ID                  int64
	ProviderInstanceID  string
	Label               string
	AccountDisplayLabel string
	PlanLabel           string
	Scopes              string
	ExpiresAt           *time.Time
	LastRefreshAt       *time.Time
	RefreshFailureClass string
	CreatedAt           time.Time
	DisabledAt          *time.Time
	Disabled            bool
}

type ProviderAccountMetadata struct {
	ID                 int64
	ProviderInstanceID string
	CredentialID       int64
	DisplayLabel       string
	PlanLabel          string
	CreatedAt          time.Time
}

type FallbackPolicyMetadata struct {
	ProviderInstanceID string
	GroupLabel         string
	Enabled            bool
	CredentialCount    int
	Explicit           bool
}

type ResolvedAPIKeyCredential struct {
	ID                 int64
	ProviderInstanceID string
	Label              string
	FallbackGroup      string
	APIKey             string
}

type ResolvedOAuthBearerCredential struct {
	ID                 int64
	ProviderInstanceID string
	BearerToken        string
	ExpiresAt          *time.Time
}

type ResolvedOAuthRefreshCredential struct {
	ID                   int64
	ProviderInstanceID   string
	AccessTokenSecretID  int64
	RefreshTokenSecretID int64
}

type OAuthTokenUpdate struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    *time.Time
	RefreshedAt  time.Time
}

func (c ResolvedOAuthBearerCredential) String() string {
	return fmt.Sprintf("ResolvedOAuthBearerCredential{ID:%d ProviderInstanceID:%q BearerToken:%q}", c.ID, c.ProviderInstanceID, RedactSecret(c.BearerToken))
}

func (c ResolvedOAuthBearerCredential) GoString() string {
	return c.String()
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
		FallbackGroup:      DefaultFallbackGroup,
		CreatedAt:          s.now(),
	}, apiKey)
}

func (s UpstreamService) AddOAuthCredential(ctx context.Context, input NewOAuthCredentialInput) (OAuthCredentialMetadata, error) {
	if input.Label == "" {
		input.Label = "oauth account"
	}
	instance, ok := s.Registry.Get(input.ProviderInstanceID)
	if !ok {
		return OAuthCredentialMetadata{}, ErrCredentialNotFound
	}
	if !instance.OAuth {
		return OAuthCredentialMetadata{}, fmt.Errorf("%w: provider %q does not support oauth credentials", ErrUnsupportedCredential, input.ProviderInstanceID)
	}
	accessToken := strings.TrimSpace(input.AccessToken)
	refreshToken := strings.TrimSpace(input.RefreshToken)
	if err := validateOAuthSecret(accessToken); err != nil {
		return OAuthCredentialMetadata{}, err
	}
	if err := validateOAuthSecret(refreshToken); err != nil {
		return OAuthCredentialMetadata{}, err
	}
	accountID, err := canonicalAccountID(input.AccountID)
	if err != nil {
		return OAuthCredentialMetadata{}, err
	}
	meta := NewOAuthCredential{
		ProviderInstanceID:  input.ProviderInstanceID,
		Label:               sanitizeOAuthDisplay(input.Label, accessToken, refreshToken),
		AccountHash:         AccountHash(instance.Type, input.ProviderInstanceID, accountID),
		AccountDisplayLabel: sanitizeOAuthDisplay(input.AccountDisplayLabel, accessToken, refreshToken),
		PlanLabel:           sanitizeOAuthDisplay(input.PlanLabel, accessToken, refreshToken),
		Scopes:              sanitizeOAuthDisplay(input.Scopes, accessToken, refreshToken),
		ExpiresAt:           input.ExpiresAt,
		CreatedAt:           s.now(),
	}
	return s.Repo.InsertOAuthCredential(ctx, meta, accessToken, refreshToken)
}

func (s UpstreamService) List(ctx context.Context) ([]UpstreamCredentialMetadata, error) {
	return s.Repo.ListUpstreamCredentials(ctx)
}

func (s UpstreamService) ListFallbackPolicies(ctx context.Context) ([]FallbackPolicyMetadata, error) {
	return s.Repo.ListFallbackPolicies(ctx)
}

func (s UpstreamService) ListOAuthCredentials(ctx context.Context) ([]OAuthCredentialMetadata, error) {
	return s.Repo.ListOAuthCredentials(ctx)
}

func (s UpstreamService) ListProviderAccounts(ctx context.Context) ([]ProviderAccountMetadata, error) {
	return s.Repo.ListProviderAccounts(ctx)
}

func (s UpstreamService) Disable(ctx context.Context, id int64) error {
	return s.Repo.DisableUpstreamCredential(ctx, id, s.now())
}

func (s UpstreamService) MarkOAuthRefreshFailure(ctx context.Context, credentialID int64, failureClass string) error {
	return s.Repo.MarkOAuthRefreshFailure(ctx, credentialID, normalizeRefreshFailureClass(failureClass), s.now())
}

func (s UpstreamService) EnableFallbackGroup(ctx context.Context, providerInstanceID, groupLabel string) error {
	return s.setFallbackGroup(ctx, providerInstanceID, groupLabel, true)
}

func (s UpstreamService) DisableFallbackGroup(ctx context.Context, providerInstanceID, groupLabel string) error {
	return s.setFallbackGroup(ctx, providerInstanceID, groupLabel, false)
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

func (s UpstreamService) ResolveAPIKeys(ctx context.Context, providerInstanceID string) ([]ResolvedAPIKeyCredential, error) {
	instance, ok := s.Registry.Get(providerInstanceID)
	if !ok {
		return nil, ErrCredentialNotFound
	}
	if !instance.APIKey || instance.Placeholder {
		return nil, fmt.Errorf("%w: provider %q does not support api-key credentials", ErrUnsupportedCredential, providerInstanceID)
	}
	return s.Repo.ResolveAPIKeyCredentials(ctx, providerInstanceID)
}

func (s UpstreamService) ResolveOAuthBearer(ctx context.Context, providerInstanceID string, now time.Time) (ResolvedOAuthBearerCredential, error) {
	instance, ok := s.Registry.Get(providerInstanceID)
	if !ok {
		return ResolvedOAuthBearerCredential{}, ErrCredentialNotFound
	}
	if !instance.OAuth {
		return ResolvedOAuthBearerCredential{}, fmt.Errorf("%w: provider %q does not support oauth credentials", ErrUnsupportedCredential, providerInstanceID)
	}
	if now.IsZero() {
		now = s.now()
	}
	return s.Repo.ResolveOAuthBearerCredential(ctx, providerInstanceID, now.UTC())
}

func (s UpstreamService) RefreshOAuthCredential(ctx context.Context, credentialID int64) error {
	credential, err := s.Repo.ResolveOAuthRefreshCredential(ctx, credentialID)
	if err != nil {
		return err
	}
	instance, ok := s.Registry.Get(credential.ProviderInstanceID)
	if !ok {
		return ErrCredentialNotFound
	}
	if !instance.OAuth || !instance.OAuthRefresh || instance.Type != "codex" {
		return fmt.Errorf("%w: provider %q does not support oauth refresh", ErrUnsupportedCredential, credential.ProviderInstanceID)
	}
	if s.OAuthRefresher == nil {
		return fmt.Errorf("%w: oauth refresh adapter is unavailable", ErrUnsupportedCredential)
	}
	refreshToken, err := s.Repo.ResolveOAuthRefreshToken(ctx, credential.ID, credential.RefreshTokenSecretID)
	if err != nil {
		return err
	}
	now := s.now()
	result, err := s.OAuthRefresher.RefreshOAuthToken(ctx, provider.OAuthRefreshRequest{
		ProviderType: instance.Type,
		AuthIssuer:   instance.AuthIssuer,
		RefreshToken: refreshToken,
		Now:          now,
	})
	if err != nil {
		return s.recordOAuthRefreshFailure(ctx, credentialID, refreshFailureClass(err, "refresh_unavailable"))
	}
	accessToken := strings.TrimSpace(result.AccessToken)
	if err := validateOAuthSecret(accessToken); err != nil {
		return s.recordOAuthRefreshFailure(ctx, credentialID, "refresh_invalid_response")
	}
	newRefreshToken := strings.TrimSpace(result.RefreshToken)
	if newRefreshToken != "" {
		if err := validateOAuthSecret(newRefreshToken); err != nil {
			return s.recordOAuthRefreshFailure(ctx, credentialID, "refresh_invalid_response")
		}
	}
	if err := s.Repo.UpdateOAuthTokens(ctx, credentialID, OAuthTokenUpdate{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		ExpiresAt:    result.ExpiresAt,
		RefreshedAt:  now,
	}); err != nil {
		return err
	}
	return nil
}

func (s UpstreamService) recordOAuthRefreshFailure(ctx context.Context, credentialID int64, failureClass string) error {
	if err := s.Repo.MarkOAuthRefreshFailure(ctx, credentialID, normalizeRefreshFailureClass(failureClass), s.now()); err != nil {
		return err
	}
	return fmt.Errorf("%w: %s", ErrOAuthRefreshFailed, normalizeRefreshFailureClass(failureClass))
}

func refreshFailureClass(err error, fallback string) string {
	type classified interface {
		RefreshFailureClass() string
	}
	var c classified
	if errors.As(err, &c) {
		return normalizeRefreshFailureClass(c.RefreshFailureClass())
	}
	return normalizeRefreshFailureClass(fallback)
}

func LooksLikeLocalToken(value string) bool {
	return len(value) > 4 && value[:4] == "iln_"
}

func AccountHash(providerType, providerInstanceID, canonicalAccountID string) string {
	sum := sha256.Sum256([]byte("ilonasin-provider-account-v1\x00" + providerType + "\x00" + providerInstanceID + "\x00" + canonicalAccountID))
	return hex.EncodeToString(sum[:])
}

func canonicalAccountID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ErrInvalidOAuthInput
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return "", ErrInvalidOAuthInput
		}
	}
	if containsForbiddenOAuthMarker(value) {
		return "", ErrInvalidOAuthInput
	}
	return value, nil
}

func validateOAuthSecret(value string) error {
	value = strings.TrimSpace(value)
	if value == "" || LooksLikeLocalToken(value) || looksLikeJWT(value) || containsForbiddenOAuthMarker(value) || looksStructuredOAuthMaterial(value) {
		return ErrInvalidOAuthInput
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return ErrInvalidOAuthInput
		}
	}
	return nil
}

func looksStructuredOAuthMaterial(value string) bool {
	lower := strings.ToLower(value)
	for _, marker := range []string{
		"{", "}", "[", "]", "\"", ";", "&",
		"access_token", "refresh_token", "token_type", "expires_in",
		"set-cookie", "cookie:", "grant_type", "application/json",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func looksLikeJWT(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(lower, "eyj") || strings.Count(value, ".") >= 2
}

func containsForbiddenOAuthMarker(value string) bool {
	lower := strings.ToLower(value)
	for _, marker := range []string{
		"authorization:", "bearer ", "http://", "https://", "callback",
		"id_token", "agent_identity", "private_key", "cookie", "stdout",
		"token_endpoint_body", "raw-provider-payload", "req_", "request_id",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func sanitizeOAuthDisplay(value, accessToken, refreshToken string) string {
	value = strings.TrimSpace(value)
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
	if containsOAuthSecretValue(value, accessToken, refreshToken) || containsForbiddenOAuthMarker(value) || containsForbiddenOAuthMetadataMarker(value) || looksLikeJWT(value) || strings.Contains(strings.ToLower(value), "acct_") {
		return ""
	}
	if len([]rune(value)) > 64 {
		return string([]rune(value)[:64])
	}
	return value
}

func containsOAuthSecretValue(value, accessToken, refreshToken string) bool {
	for _, secret := range []string{accessToken, refreshToken} {
		secret = strings.TrimSpace(secret)
		if secret != "" && strings.Contains(value, secret) {
			return true
		}
	}
	return false
}

func containsForbiddenOAuthMetadataMarker(value string) bool {
	lower := strings.ToLower(value)
	for _, marker := range []string{
		"sk-", "iln_", "secret", "oauth-access", "oauth-refresh", "token",
		"prompt", "completion", "body", "raw", "payload", "account",
		"balance", "credit",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func normalizeRefreshFailureClass(value string) string {
	switch value {
	case "refresh_token_expired", "refresh_token_invalidated", "refresh_token_reused",
		"refresh_unauthorized", "refresh_network_error", "refresh_timeout",
		"refresh_http_error", "refresh_body_too_large", "refresh_unavailable",
		"refresh_invalid_response":
		return value
	default:
		return "refresh_unavailable"
	}
}

func (s UpstreamService) setFallbackGroup(ctx context.Context, providerInstanceID, groupLabel string, enabled bool) error {
	if groupLabel == "" {
		groupLabel = DefaultFallbackGroup
	}
	instance, ok := s.Registry.Get(providerInstanceID)
	if !ok {
		return ErrCredentialNotFound
	}
	if !instance.APIKey || instance.Placeholder {
		return fmt.Errorf("%w: provider %q does not support api-key credentials", ErrUnsupportedCredential, providerInstanceID)
	}
	return s.Repo.SetFallbackGroupEnabled(ctx, providerInstanceID, groupLabel, enabled, s.now())
}

func (s UpstreamService) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
