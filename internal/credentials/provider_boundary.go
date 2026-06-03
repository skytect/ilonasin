package credentials

import (
	"context"
	"time"
)

type ProviderInstanceLookup interface {
	Get(id string) (ProviderInstance, bool)
}

type ProviderInstance struct {
	ID           string
	Type         string
	AuthIssuer   string
	APIKey       bool
	OAuth        bool
	OAuthRefresh bool
}

func supportsCodexOAuthCredentials(instance ProviderInstance) bool {
	return instance.Type == "codex" && instance.OAuth
}

func supportsCodexOAuthRefresh(instance ProviderInstance) bool {
	return supportsCodexOAuthCredentials(instance) && instance.OAuthRefresh
}

type OAuthTokenRefresher interface {
	RefreshOAuthToken(ctx context.Context, req OAuthRefreshRequest) (OAuthRefreshResult, error)
}

type OAuthRefreshRequest struct {
	ProviderType string
	AuthIssuer   string
	RefreshToken string
	Now          time.Time
}

type OAuthRefreshResult struct {
	AccessToken  string
	RefreshToken string
	IDToken      string
	ExpiresAt    *time.Time
}

type OAuthDeviceLoginProvider interface {
	RequestOAuthDeviceCode(ctx context.Context, req OAuthDeviceCodeRequest) (OAuthDeviceCodeChallenge, error)
	CompleteOAuthDeviceLogin(ctx context.Context, req OAuthDeviceLoginRequest) (OAuthDeviceLoginResult, error)
}

type OAuthDeviceCodeRequest struct {
	ProviderInstanceID string
	ProviderType       string
	AuthIssuer         string
}

type OAuthDeviceCodeChallenge struct {
	VerificationURL string
	UserCode        string
	DeviceAuthID    string
	IntervalSeconds int
}

type OAuthDeviceLoginRequest struct {
	ProviderInstanceID string
	ProviderType       string
	AuthIssuer         string
	DeviceAuthID       string
	UserCode           string
	IntervalSeconds    int
	Now                time.Time
}

type OAuthDeviceLoginResult struct {
	AccessToken  string
	RefreshToken string
	IDToken      string
	ExpiresAt    *time.Time
}
