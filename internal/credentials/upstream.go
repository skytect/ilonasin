package credentials

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"ilonasin/internal/logging"
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

const (
	CredentialKindAPIKey = "api_key"
	CredentialKindOAuth  = "oauth"
)

type UpstreamCredentialManager interface {
	AddAPIKey(ctx context.Context, providerInstanceID, label, apiKey string) (UpstreamCredentialMetadata, error)
	List(ctx context.Context) ([]UpstreamCredentialMetadata, error)
	ListFallbackPolicies(ctx context.Context) ([]FallbackPolicyMetadata, error)
	Disable(ctx context.Context, id int64) error
	EnableFallbackGroup(ctx context.Context, providerInstanceID, credentialKind, groupLabel string) error
	DisableFallbackGroup(ctx context.Context, providerInstanceID, credentialKind, groupLabel string) error
}

type UpstreamCredentialResolver interface {
	ResolveAPIKey(ctx context.Context, providerInstanceID string) (ResolvedAPIKeyCredential, error)
	ResolveAPIKeys(ctx context.Context, providerInstanceID string) ([]ResolvedAPIKeyCredential, error)
}

type OAuthBearerResolver interface {
	ResolveOAuthBearer(ctx context.Context, providerInstanceID string, now time.Time) (ResolvedOAuthBearerCredential, error)
	ResolveOAuthBearers(ctx context.Context, providerInstanceID string, now time.Time) ([]ResolvedOAuthBearerCredential, error)
}

type OAuthProviderRefreshController interface {
	ResolveOAuthBearerByID(ctx context.Context, credentialID int64, now time.Time) (ResolvedOAuthBearerCredential, error)
	RefreshOAuthProviderCredential(ctx context.Context, providerInstanceID string) error
	RefreshOAuthCredentialIfBearer(ctx context.Context, credentialID int64, staleBearerToken string) error
	RefreshOAuthCredential(ctx context.Context, credentialID int64) error
}

type UpstreamCredentialRepository interface {
	InsertAPIKeyCredential(ctx context.Context, meta NewUpstreamCredential, apiKey string) (UpstreamCredentialMetadata, error)
	ListUpstreamCredentials(ctx context.Context) ([]UpstreamCredentialMetadata, error)
	DisableUpstreamCredential(ctx context.Context, id int64, disabledAt time.Time) error
	ResolveAPIKeyCredential(ctx context.Context, providerInstanceID string) (ResolvedAPIKeyCredential, error)
	ResolveAPIKeyCredentials(ctx context.Context, providerInstanceID string) ([]ResolvedAPIKeyCredential, error)
	ResolveOAuthBearerCredential(ctx context.Context, providerInstanceID string, now time.Time) (ResolvedOAuthBearerCredential, error)
	ResolveOAuthBearerCredentials(ctx context.Context, providerInstanceID string, now time.Time) ([]ResolvedOAuthBearerCredential, error)
	ResolveOAuthBearerCredentialByID(ctx context.Context, credentialID int64, now time.Time) (ResolvedOAuthBearerCredential, error)
	ResolveOAuthRefreshCredential(ctx context.Context, credentialID int64) (ResolvedOAuthRefreshCredential, error)
	ResolveOAuthRefreshCredentialForProvider(ctx context.Context, providerInstanceID string) (ResolvedOAuthRefreshCredential, error)
	ResolveOAuthRefreshToken(ctx context.Context, credentialID, refreshSecretID int64) (string, error)
	UpdateOAuthTokens(ctx context.Context, credentialID int64, update OAuthTokenUpdate) error
	UpdateOAuthAccountMetadata(ctx context.Context, credentialID int64, displayLabel, planLabel string, updatedAt time.Time) error
	ListFallbackPolicies(ctx context.Context) ([]FallbackPolicyMetadata, error)
	SetFallbackGroupEnabled(ctx context.Context, providerInstanceID, credentialKind, groupLabel string, enabled bool, now time.Time) error
	UpsertOAuthCredentialForAccountHash(ctx context.Context, meta NewOAuthCredential, accessToken, refreshToken string) (OAuthCredentialMetadata, error)
	ListOAuthCredentials(ctx context.Context) ([]OAuthCredentialMetadata, error)
	ListProviderAccounts(ctx context.Context) ([]ProviderAccountMetadata, error)
	MarkOAuthRefreshFailure(ctx context.Context, credentialID int64, failureClass, failureDescription string, now time.Time) error
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

type OAuthDeviceLoginController interface {
	StartOAuthDeviceLogin(ctx context.Context, providerInstanceID string) (OAuthDeviceLoginChallenge, error)
	CompleteOAuthDeviceLogin(ctx context.Context, handle string) (OAuthCredentialMetadata, error)
}

type UpstreamService struct {
	Registry       provider.Registry
	Repo           UpstreamCredentialRepository
	OAuthRefresher provider.OAuthTokenRefresher
	OAuthLogin     provider.OAuthDeviceLoginProvider
	Now            func() time.Time
	DeviceLogins   *OAuthDeviceLoginSessions
	Logger         *slog.Logger
	SecretsChanged func(context.Context, ...string)
	refreshMu      sync.Mutex
	refreshes      *OAuthRefreshCalls
}

type OAuthDeviceLoginSessions struct {
	mu       sync.Mutex
	sessions map[string]oauthDeviceLoginSession
	max      int
	ttl      time.Duration
}

type OAuthRefreshCalls struct {
	mu    sync.Mutex
	calls map[string]*oauthRefreshCall
}

type oauthRefreshCall struct {
	done chan struct{}
	err  error
}

func NewOAuthDeviceLoginSessions(max int, ttl time.Duration) *OAuthDeviceLoginSessions {
	return &OAuthDeviceLoginSessions{max: max, ttl: ttl}
}

type oauthDeviceLoginSession struct {
	ProviderInstanceID string
	ProviderType       string
	AuthIssuer         string
	DeviceAuthID       string
	UserCode           string
	IntervalSeconds    int
	ExpiresAt          time.Time
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
	ID                        int64
	ProviderInstanceID        string
	Label                     string
	AccountDisplayLabel       string
	PlanLabel                 string
	Scopes                    string
	ExpiresAt                 *time.Time
	LastRefreshAt             *time.Time
	RefreshFailureClass       string
	RefreshFailureDescription string
	CreatedAt                 time.Time
	DisabledAt                *time.Time
	Disabled                  bool
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
	CredentialKind     string
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
	ID                      int64
	ProviderInstanceID      string
	FallbackGroup           string
	BearerToken             string
	ChatGPTAccountID        string
	ChatGPTAccountIsFedRAMP bool
	ExpiresAt               *time.Time
}

type ResolvedOAuthRefreshCredential struct {
	ID                   int64
	ProviderInstanceID   string
	AccountHash          string
	AccessTokenSecretID  int64
	RefreshTokenSecretID int64
}

type OAuthTokenUpdate struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    *time.Time
	RefreshedAt  time.Time
}

type OAuthDeviceLoginChallenge struct {
	ProviderInstanceID string
	VerificationURL    string
	UserCode           string
	Handle             string
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

func (s *UpstreamService) AddAPIKey(ctx context.Context, providerInstanceID, label, apiKey string) (UpstreamCredentialMetadata, error) {
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
	if !instance.APIKey {
		return UpstreamCredentialMetadata{}, fmt.Errorf("%w: provider %q does not support api-key credentials", ErrUnsupportedCredential, providerInstanceID)
	}
	meta, err := s.Repo.InsertAPIKeyCredential(ctx, NewUpstreamCredential{
		ProviderInstanceID: providerInstanceID,
		Kind:               CredentialKindAPIKey,
		Label:              label,
		SecretPrefix:       Prefix(apiKey),
		SecretLast4:        Last4(apiKey),
		FallbackGroup:      DefaultFallbackGroup,
		CreatedAt:          s.now(),
	}, apiKey)
	if err == nil {
		s.logInfo(ctx, "credential_created", slog.String("kind", "api_key"), slog.String("provider_instance", providerInstanceID), slog.Int64("credential_id", meta.ID))
		s.notifySecretsChanged(ctx, apiKey)
	}
	return meta, err
}

func (s *UpstreamService) AddOAuthCredential(ctx context.Context, input NewOAuthCredentialInput) (OAuthCredentialMetadata, error) {
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
	created, err := s.Repo.UpsertOAuthCredentialForAccountHash(ctx, meta, accessToken, refreshToken)
	if err == nil {
		s.logInfo(ctx, "credential_created", slog.String("kind", "oauth"), slog.String("provider_instance", input.ProviderInstanceID), slog.Int64("credential_id", created.ID))
		s.notifySecretsChanged(ctx, accessToken, refreshToken)
	}
	return created, err
}

func (s *UpstreamService) List(ctx context.Context) ([]UpstreamCredentialMetadata, error) {
	return s.Repo.ListUpstreamCredentials(ctx)
}

func (s *UpstreamService) ListFallbackPolicies(ctx context.Context) ([]FallbackPolicyMetadata, error) {
	return s.Repo.ListFallbackPolicies(ctx)
}

func (s *UpstreamService) ListOAuthCredentials(ctx context.Context) ([]OAuthCredentialMetadata, error) {
	return s.Repo.ListOAuthCredentials(ctx)
}

func (s *UpstreamService) ListProviderAccounts(ctx context.Context) ([]ProviderAccountMetadata, error) {
	return s.Repo.ListProviderAccounts(ctx)
}

func (s *UpstreamService) Disable(ctx context.Context, id int64) error {
	err := s.Repo.DisableUpstreamCredential(ctx, id, s.now())
	if err == nil {
		s.logInfo(ctx, "credential_disabled", slog.Int64("credential_id", id))
	}
	return err
}

func (s *UpstreamService) MarkOAuthRefreshFailure(ctx context.Context, credentialID int64, failureClass string) error {
	return s.Repo.MarkOAuthRefreshFailure(ctx, credentialID, normalizeRefreshFailureClass(failureClass), "", s.now())
}

func (s *UpstreamService) EnableFallbackGroup(ctx context.Context, providerInstanceID, credentialKind, groupLabel string) error {
	err := s.setFallbackGroup(ctx, providerInstanceID, credentialKind, groupLabel, true)
	if err == nil {
		s.logInfo(ctx, "fallback_policy_changed", slog.String("provider_instance", providerInstanceID), slog.String("credential_kind", credentialKind), slog.Bool("enabled", true))
	}
	return err
}

func (s *UpstreamService) DisableFallbackGroup(ctx context.Context, providerInstanceID, credentialKind, groupLabel string) error {
	err := s.setFallbackGroup(ctx, providerInstanceID, credentialKind, groupLabel, false)
	if err == nil {
		s.logInfo(ctx, "fallback_policy_changed", slog.String("provider_instance", providerInstanceID), slog.String("credential_kind", credentialKind), slog.Bool("enabled", false))
	}
	return err
}

func (s *UpstreamService) ResolveAPIKey(ctx context.Context, providerInstanceID string) (ResolvedAPIKeyCredential, error) {
	instance, ok := s.Registry.Get(providerInstanceID)
	if !ok {
		return ResolvedAPIKeyCredential{}, ErrCredentialNotFound
	}
	if !instance.APIKey {
		return ResolvedAPIKeyCredential{}, fmt.Errorf("%w: provider %q does not support api-key credentials", ErrUnsupportedCredential, providerInstanceID)
	}
	return s.Repo.ResolveAPIKeyCredential(ctx, providerInstanceID)
}

func (s *UpstreamService) ResolveAPIKeys(ctx context.Context, providerInstanceID string) ([]ResolvedAPIKeyCredential, error) {
	instance, ok := s.Registry.Get(providerInstanceID)
	if !ok {
		return nil, ErrCredentialNotFound
	}
	if !instance.APIKey {
		return nil, fmt.Errorf("%w: provider %q does not support api-key credentials", ErrUnsupportedCredential, providerInstanceID)
	}
	return s.Repo.ResolveAPIKeyCredentials(ctx, providerInstanceID)
}

func (s *UpstreamService) ResolveOAuthBearer(ctx context.Context, providerInstanceID string, now time.Time) (ResolvedOAuthBearerCredential, error) {
	instance, ok := s.Registry.Get(providerInstanceID)
	if !ok {
		return ResolvedOAuthBearerCredential{}, ErrCredentialNotFound
	}
	if !instance.OAuth || instance.Type != "codex" {
		return ResolvedOAuthBearerCredential{}, fmt.Errorf("%w: provider %q does not support oauth credentials", ErrUnsupportedCredential, providerInstanceID)
	}
	if now.IsZero() {
		now = s.now()
	}
	return s.Repo.ResolveOAuthBearerCredential(ctx, providerInstanceID, now.UTC())
}

func (s *UpstreamService) ResolveOAuthBearers(ctx context.Context, providerInstanceID string, now time.Time) ([]ResolvedOAuthBearerCredential, error) {
	instance, ok := s.Registry.Get(providerInstanceID)
	if !ok {
		return nil, ErrCredentialNotFound
	}
	if !instance.OAuth || instance.Type != "codex" {
		return nil, fmt.Errorf("%w: provider %q does not support oauth credential pooling", ErrUnsupportedCredential, providerInstanceID)
	}
	if now.IsZero() {
		now = s.now()
	}
	out, err := s.Repo.ResolveOAuthBearerCredentials(ctx, providerInstanceID, now.UTC())
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *UpstreamService) ResolveOAuthBearerByID(ctx context.Context, credentialID int64, now time.Time) (ResolvedOAuthBearerCredential, error) {
	if now.IsZero() {
		now = s.now()
	}
	credential, err := s.Repo.ResolveOAuthBearerCredentialByID(ctx, credentialID, now.UTC())
	if err != nil {
		return ResolvedOAuthBearerCredential{}, err
	}
	instance, ok := s.Registry.Get(credential.ProviderInstanceID)
	if !ok {
		return ResolvedOAuthBearerCredential{}, ErrCredentialNotFound
	}
	if !instance.OAuth || instance.Type != "codex" {
		return ResolvedOAuthBearerCredential{}, fmt.Errorf("%w: provider %q does not support oauth credentials", ErrUnsupportedCredential, credential.ProviderInstanceID)
	}
	return credential, nil
}

func (s *UpstreamService) StartOAuthDeviceLogin(ctx context.Context, providerInstanceID string) (OAuthDeviceLoginChallenge, error) {
	if s.OAuthLogin == nil {
		return OAuthDeviceLoginChallenge{}, fmt.Errorf("%w: oauth login adapter is unavailable", ErrUnsupportedCredential)
	}
	instance, ok := s.Registry.Get(providerInstanceID)
	if !ok {
		return OAuthDeviceLoginChallenge{}, ErrCredentialNotFound
	}
	if !instance.OAuth || instance.Type != "codex" {
		return OAuthDeviceLoginChallenge{}, fmt.Errorf("%w: provider %q does not support oauth device login", ErrUnsupportedCredential, providerInstanceID)
	}
	sessions := s.loginSessions()
	now := s.now()
	if err := sessions.checkCapacity(now); err != nil {
		return OAuthDeviceLoginChallenge{}, err
	}
	challenge, err := s.OAuthLogin.RequestOAuthDeviceCode(ctx, provider.OAuthDeviceCodeRequest{
		ProviderInstanceID: providerInstanceID,
		ProviderType:       instance.Type,
		AuthIssuer:         instance.AuthIssuer,
	})
	if err != nil {
		s.logError(ctx, "oauth_device_login_start_failed", err, slog.String("provider_instance", providerInstanceID))
		return OAuthDeviceLoginChallenge{}, err
	}
	handle, err := randomHandle()
	if err != nil {
		return OAuthDeviceLoginChallenge{}, err
	}
	if err := sessions.put(handle, oauthDeviceLoginSession{
		ProviderInstanceID: providerInstanceID,
		ProviderType:       instance.Type,
		AuthIssuer:         instance.AuthIssuer,
		DeviceAuthID:       challenge.DeviceAuthID,
		UserCode:           challenge.UserCode,
		IntervalSeconds:    challenge.IntervalSeconds,
		ExpiresAt:          now.Add(sessions.ttlDuration()),
	}, now); err != nil {
		return OAuthDeviceLoginChallenge{}, err
	}
	s.logInfo(ctx, "oauth_device_login_started", slog.String("provider_instance", providerInstanceID))
	return OAuthDeviceLoginChallenge{
		ProviderInstanceID: providerInstanceID,
		VerificationURL:    challenge.VerificationURL,
		UserCode:           challenge.UserCode,
		Handle:             handle,
	}, nil
}

func (s *UpstreamService) CompleteOAuthDeviceLogin(ctx context.Context, handle string) (OAuthCredentialMetadata, error) {
	if s.OAuthLogin == nil {
		return OAuthCredentialMetadata{}, fmt.Errorf("%w: oauth login adapter is unavailable", ErrUnsupportedCredential)
	}
	session, ok := s.loginSessions().take(handle, s.now())
	if !ok {
		return OAuthCredentialMetadata{}, ErrNoEligibleCredential
	}
	now := s.now()
	result, err := s.OAuthLogin.CompleteOAuthDeviceLogin(ctx, provider.OAuthDeviceLoginRequest{
		ProviderInstanceID: session.ProviderInstanceID,
		ProviderType:       session.ProviderType,
		AuthIssuer:         session.AuthIssuer,
		DeviceAuthID:       session.DeviceAuthID,
		UserCode:           session.UserCode,
		IntervalSeconds:    session.IntervalSeconds,
		Now:                now,
	})
	if err != nil {
		s.logError(ctx, "oauth_device_login_complete_failed", err, slog.String("provider_instance", session.ProviderInstanceID))
		return OAuthCredentialMetadata{}, err
	}
	accessToken := strings.TrimSpace(result.AccessToken)
	refreshToken := strings.TrimSpace(result.RefreshToken)
	if err := validateOAuthSecret(accessToken); err != nil {
		return OAuthCredentialMetadata{}, ErrInvalidOAuthInput
	}
	if err := validateOAuthSecret(refreshToken); err != nil {
		return OAuthCredentialMetadata{}, ErrInvalidOAuthInput
	}
	claims, err := parseChatGPTIDTokenClaims(result.IDToken)
	if err != nil {
		return OAuthCredentialMetadata{}, ErrInvalidOAuthInput
	}
	if claims.AccountID == "" {
		return OAuthCredentialMetadata{}, ErrInvalidOAuthInput
	}
	labelHash := AccountHash(session.ProviderType, session.ProviderInstanceID, claims.AccountID)
	return s.AddOAuthCredential(ctx, NewOAuthCredentialInput{
		ProviderInstanceID:  session.ProviderInstanceID,
		Label:               "codex device login " + labelHash[:8],
		AccessToken:         accessToken,
		RefreshToken:        refreshToken,
		AccountID:           claims.AccountID,
		AccountDisplayLabel: safeOAuthLoginDisplay(claims.Email, "Codex account", accessToken, refreshToken, result.IDToken, claims.AccountID),
		PlanLabel:           safeOAuthLoginDisplay(claims.PlanLabel, "", accessToken, refreshToken, result.IDToken, claims.AccountID),
		ExpiresAt:           result.ExpiresAt,
	})
}

func (s *UpstreamService) RefreshOAuthCredential(ctx context.Context, credentialID int64) error {
	err := s.refreshCalls().do(ctx, fmt.Sprintf("credential:%d", credentialID), func() error {
		return s.refreshOAuthCredential(ctx, credentialID)
	})
	if err != nil {
		s.logError(ctx, "oauth_refresh_failed", err, slog.Int64("credential_id", credentialID))
	} else {
		s.logInfo(ctx, "oauth_refresh_completed", slog.Int64("credential_id", credentialID))
	}
	return err
}

func (s *UpstreamService) RefreshOAuthCredentialIfBearer(ctx context.Context, credentialID int64, staleBearerToken string) error {
	return s.refreshCalls().do(ctx, fmt.Sprintf("credential:%d", credentialID), func() error {
		if staleBearerToken != "" {
			current, err := s.Repo.ResolveOAuthBearerCredentialByID(ctx, credentialID, time.Time{})
			if err == nil && current.BearerToken != "" && current.BearerToken != staleBearerToken {
				return nil
			}
		}
		return s.refreshOAuthCredential(ctx, credentialID)
	})
}

func (s *UpstreamService) RefreshOAuthProviderCredential(ctx context.Context, providerInstanceID string) error {
	return s.refreshCalls().do(ctx, "provider:"+providerInstanceID, func() error {
		instance, ok := s.Registry.Get(providerInstanceID)
		if !ok {
			return ErrCredentialNotFound
		}
		if !instance.OAuth || !instance.OAuthRefresh || instance.Type != "codex" {
			return fmt.Errorf("%w: provider %q does not support oauth refresh", ErrUnsupportedCredential, providerInstanceID)
		}
		if _, err := s.ResolveOAuthBearer(ctx, providerInstanceID, s.now()); err == nil {
			return nil
		}
		credential, err := s.Repo.ResolveOAuthRefreshCredentialForProvider(ctx, providerInstanceID)
		if err != nil {
			return err
		}
		return s.RefreshOAuthCredential(ctx, credential.ID)
	})
}

func (s *UpstreamService) refreshOAuthCredential(ctx context.Context, credentialID int64) error {
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
		return s.recordOAuthRefreshFailure(ctx, credentialID, refreshFailureClass(err, "refresh_unavailable"), refreshFailureDescription(err))
	}
	accessToken := strings.TrimSpace(result.AccessToken)
	if err := validateOAuthSecret(accessToken); err != nil {
		return s.recordOAuthRefreshFailure(ctx, credentialID, "refresh_invalid_response", "")
	}
	newRefreshToken := strings.TrimSpace(result.RefreshToken)
	if newRefreshToken != "" {
		if err := validateOAuthSecret(newRefreshToken); err != nil {
			return s.recordOAuthRefreshFailure(ctx, credentialID, "refresh_invalid_response", "")
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
	s.notifySecretsChanged(ctx, accessToken, newRefreshToken)
	if strings.TrimSpace(result.IDToken) != "" {
		claims, err := parseChatGPTIDTokenClaims(result.IDToken)
		if err == nil && claims.AccountID != "" && AccountHash(instance.Type, credential.ProviderInstanceID, claims.AccountID) == credential.AccountHash {
			displayLabel := safeOAuthLoginDisplay(claims.Email, "", accessToken, firstNonEmpty(newRefreshToken, refreshToken), result.IDToken, claims.AccountID)
			planLabel := safeOAuthLoginDisplay(claims.PlanLabel, "", accessToken, firstNonEmpty(newRefreshToken, refreshToken), result.IDToken, claims.AccountID)
			if displayLabel != "" || planLabel != "" {
				if err := s.Repo.UpdateOAuthAccountMetadata(ctx, credentialID, displayLabel, planLabel, now); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (s *UpstreamService) notifySecretsChanged(ctx context.Context, secrets ...string) {
	if s.SecretsChanged != nil {
		s.SecretsChanged(ctx, secrets...)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (s *UpstreamService) refreshCalls() *OAuthRefreshCalls {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	if s.refreshes == nil {
		s.refreshes = &OAuthRefreshCalls{}
	}
	return s.refreshes
}

func (c *OAuthRefreshCalls) do(ctx context.Context, key string, fn func() error) error {
	c.mu.Lock()
	if c.calls == nil {
		c.calls = map[string]*oauthRefreshCall{}
	}
	if existing := c.calls[key]; existing != nil {
		c.mu.Unlock()
		select {
		case <-existing.done:
			return existing.err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	call := &oauthRefreshCall{done: make(chan struct{})}
	c.calls[key] = call
	c.mu.Unlock()

	call.err = fn()
	close(call.done)

	c.mu.Lock()
	delete(c.calls, key)
	c.mu.Unlock()
	return call.err
}

func (s *UpstreamService) recordOAuthRefreshFailure(ctx context.Context, credentialID int64, failureClass, failureDescription string) error {
	if err := s.Repo.MarkOAuthRefreshFailure(ctx, credentialID, normalizeRefreshFailureClass(failureClass), safeRefreshFailureDescription(failureDescription), s.now()); err != nil {
		return err
	}
	return fmt.Errorf("%w: %s", ErrOAuthRefreshFailed, normalizeRefreshFailureClass(failureClass))
}

func (s *OAuthDeviceLoginSessions) checkCapacity(now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions == nil {
		s.sessions = map[string]oauthDeviceLoginSession{}
	}
	s.pruneLocked(now)
	max := s.max
	if max <= 0 {
		max = 16
	}
	if len(s.sessions) >= max {
		return ErrNoEligibleCredential
	}
	return nil
}

func (s *UpstreamService) loginSessions() *OAuthDeviceLoginSessions {
	if s.DeviceLogins == nil {
		s.DeviceLogins = &OAuthDeviceLoginSessions{}
	}
	return s.DeviceLogins
}

func (s *OAuthDeviceLoginSessions) put(handle string, session oauthDeviceLoginSession, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions == nil {
		s.sessions = map[string]oauthDeviceLoginSession{}
	}
	s.pruneLocked(now)
	max := s.max
	if max <= 0 {
		max = 16
	}
	if len(s.sessions) >= max {
		return ErrNoEligibleCredential
	}
	s.sessions[handle] = session
	return nil
}

func (s *OAuthDeviceLoginSessions) take(handle string, now time.Time) (oauthDeviceLoginSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(now)
	session, ok := s.sessions[handle]
	if ok {
		delete(s.sessions, handle)
	}
	return session, ok
}

func (s *OAuthDeviceLoginSessions) pruneLocked(now time.Time) {
	for handle, session := range s.sessions {
		if !session.ExpiresAt.After(now) {
			delete(s.sessions, handle)
		}
	}
}

func (s *OAuthDeviceLoginSessions) ttlDuration() time.Duration {
	if s.ttl > 0 {
		return s.ttl
	}
	return 15 * time.Minute
}

func randomHandle() (string, error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "oauth_login_" + hex.EncodeToString(b[:]), nil
}

type chatGPTIDTokenClaims struct {
	AccountID string
	Email     string
	PlanLabel string
}

type ChatGPTRoutingClaims struct {
	AccountID string
	FedRAMP   bool
}

func ParseChatGPTRoutingClaims(jwt string) ChatGPTRoutingClaims {
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 || parts[1] == "" {
		return ChatGPTRoutingClaims{}
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ChatGPTRoutingClaims{}
	}
	// Best-effort compatibility only. Codex stores TokenData.account_id
	// separately and uses it as the durable ChatGPT-Account-ID source.
	var claims struct {
		Auth struct {
			AccountID string `json:"chatgpt_account_id"`
			FedRAMP   bool   `json:"chatgpt_account_is_fedramp"`
		} `json:"https://api.openai.com/auth"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ChatGPTRoutingClaims{}
	}
	return ChatGPTRoutingClaims{
		AccountID: strings.TrimSpace(claims.Auth.AccountID),
		FedRAMP:   claims.Auth.FedRAMP,
	}
}

func parseChatGPTIDTokenClaims(jwt string) (chatGPTIDTokenClaims, error) {
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return chatGPTIDTokenClaims{}, ErrInvalidOAuthInput
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return chatGPTIDTokenClaims{}, ErrInvalidOAuthInput
	}
	// Mirrors OpenAI Codex rust-v0.135.0:
	// codex-rs/login/src/token_data.rs parse_chatgpt_jwt_claims.
	var claims struct {
		Email   string `json:"email"`
		Profile struct {
			Email string `json:"email"`
		} `json:"https://api.openai.com/profile"`
		Auth struct {
			AccountID string `json:"chatgpt_account_id"`
			Plan      any    `json:"chatgpt_plan_type"`
		} `json:"https://api.openai.com/auth"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return chatGPTIDTokenClaims{}, ErrInvalidOAuthInput
	}
	email := strings.TrimSpace(claims.Email)
	if email == "" {
		email = strings.TrimSpace(claims.Profile.Email)
	}
	return chatGPTIDTokenClaims{
		AccountID: strings.TrimSpace(claims.Auth.AccountID),
		Email:     email,
		PlanLabel: planLabelString(claims.Auth.Plan),
	}, nil
}

func planLabelString(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]any:
		if known, _ := v["known"].(string); known != "" {
			return strings.TrimSpace(known)
		}
		if unknown, _ := v["unknown"].(string); unknown != "" {
			return strings.TrimSpace(unknown)
		}
	}
	return ""
}

func safeOAuthLoginDisplay(value, fallback, accessToken, refreshToken, idToken, accountID string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	value = sanitizeOAuthDisplay(value, accessToken, refreshToken)
	if value == "" && fallback != "" {
		value = fallback
	}
	for _, marker := range []string{idToken, accountID, "access_token", "refresh_token", "device_auth", "authorization_code", "code_verifier", "cookie", "Bearer "} {
		if marker != "" && strings.Contains(value, marker) {
			return fallback
		}
	}
	return value
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

func refreshFailureDescription(err error) string {
	type described interface {
		RefreshFailureDescription() string
	}
	var d described
	if errors.As(err, &d) {
		return d.RefreshFailureDescription()
	}
	return ""
}

func safeRefreshFailureDescription(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	if unsafeRefreshFailureDescription(value) {
		return "[redacted]"
	}
	const maxRunes = 1024
	runes := []rune(value)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes])
	}
	return value
}

var unsafeRefreshFailureDescriptionPattern = regexp.MustCompile(`(?i)(bearer\s+[A-Za-z0-9._~+/=-]+|sk-[A-Za-z0-9._~+/=-]*|iln_[A-Za-z0-9._~+/=-]*|refresh[_-]?token\s*[:=]|access[_-]?token\s*[:=]?|id[_-]?token\s*[:=]?|authorization[_-]?code\s*[:=]?|code[_-]?verifier\s*[:=]?|raw([_:./ -](payload|body))?|payload|request[-_:./ ]?body|response[-_:./ ]?body|prompt[-_:./ ](text|body|payload)|completion[-_:./ ](text|body|payload)|account[_-]?id\s*[:=]?|acct[-_][A-Za-z0-9._~+/=-]+|request[-_ ]?id\s*[:=]?|requestid\s*[:=]?|req[_-][A-Za-z0-9._~+/=-]+|sse[_ -]?chunk|tool[_ -]?(argument|result)|eyj[a-z0-9_-]*\.[a-z0-9_-]*\.)`)

func unsafeRefreshFailureDescription(value string) bool {
	return unsafeRefreshFailureDescriptionPattern.MatchString(value)
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
	if value == "" || LooksLikeLocalToken(value) || containsForbiddenOAuthMarker(value) || looksStructuredOAuthMaterial(value) {
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
		"device-auth", "device_auth", "authorization-code", "authorization_code",
		"code-verifier", "code_verifier",
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
		"refresh_invalid_grant", "refresh_invalid_client", "refresh_invalid_request",
		"refresh_unauthorized_client", "refresh_access_denied",
		"refresh_unsupported_grant_type", "refresh_invalid_scope",
		"refresh_server_error", "refresh_temporarily_unavailable",
		"refresh_unauthorized", "refresh_network_error", "refresh_timeout",
		"refresh_http_error", "refresh_body_too_large", "refresh_unavailable",
		"refresh_invalid_response":
		return value
	default:
		return "refresh_unavailable"
	}
}

func (s *UpstreamService) setFallbackGroup(ctx context.Context, providerInstanceID, credentialKind, groupLabel string, enabled bool) error {
	if groupLabel == "" {
		groupLabel = DefaultFallbackGroup
	}
	instance, ok := s.Registry.Get(providerInstanceID)
	if !ok {
		return ErrCredentialNotFound
	}
	switch credentialKind {
	case CredentialKindAPIKey:
		if !instance.APIKey {
			return fmt.Errorf("%w: provider %q does not support api-key fallback groups", ErrUnsupportedCredential, providerInstanceID)
		}
	case CredentialKindOAuth:
		if !instance.OAuth || instance.Type != "codex" {
			return fmt.Errorf("%w: provider %q does not support oauth fallback groups", ErrUnsupportedCredential, providerInstanceID)
		}
	default:
		return fmt.Errorf("%w: provider %q does not support fallback groups", ErrUnsupportedCredential, providerInstanceID)
	}
	return s.Repo.SetFallbackGroupEnabled(ctx, providerInstanceID, credentialKind, groupLabel, enabled, s.now())
}

func (s *UpstreamService) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *UpstreamService) logInfo(ctx context.Context, event string, attrs ...slog.Attr) {
	if s.Logger == nil {
		return
	}
	attrs = append([]slog.Attr{slog.String("event", event)}, attrs...)
	s.Logger.LogAttrs(ctx, slog.LevelInfo, "credential event", attrs...)
}

func (s *UpstreamService) logError(ctx context.Context, event string, err error, attrs ...slog.Attr) {
	if s.Logger == nil {
		return
	}
	eventID := logging.EventID()
	var loginErr provider.OAuthDeviceLoginError
	if errors.As(err, &loginErr) && loginErr.EventID != "" {
		eventID = loginErr.EventID
	}
	attrs = append([]slog.Attr{
		slog.String("event", event),
		logging.EventIDAttr(eventID),
		slog.String("error_class", safeCredentialErrorClass(err)),
	}, attrs...)
	s.Logger.LogAttrs(ctx, slog.LevelError, "credential error", attrs...)
}

func safeCredentialErrorClass(err error) string {
	if err == nil {
		return ""
	}
	var loginErr provider.OAuthDeviceLoginError
	if errors.As(err, &loginErr) && loginErr.Class != "" {
		return loginErr.Class
	}
	var refreshErr provider.OAuthRefreshError
	if errors.As(err, &refreshErr) && refreshErr.Class != "" {
		return refreshErr.Class
	}
	switch {
	case errors.Is(err, ErrCredentialNotFound):
		return "credential_not_found"
	case errors.Is(err, ErrNoEligibleCredential):
		return "no_eligible_credential"
	case errors.Is(err, ErrUnsupportedCredential):
		return "unsupported_credential"
	case errors.Is(err, ErrInvalidOAuthInput):
		return "invalid_oauth_input"
	case errors.Is(err, ErrOAuthRefreshFailed):
		return "oauth_refresh_failed"
	default:
		return "credential_error"
	}
}
