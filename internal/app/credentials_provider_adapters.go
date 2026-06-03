package app

import (
	"context"

	"ilonasin/internal/credentials"
	"ilonasin/internal/provider"
)

type credentialProviderRegistry struct {
	registry provider.Registry
}

func (r credentialProviderRegistry) Get(id string) (credentials.ProviderInstance, bool) {
	instance, ok := r.registry.Get(id)
	if !ok {
		return credentials.ProviderInstance{}, false
	}
	return credentials.ProviderInstance{
		ID:           instance.ID,
		Type:         instance.Type,
		AuthIssuer:   instance.AuthIssuer,
		APIKey:       instance.APIKey,
		OAuth:        instance.OAuth,
		OAuthRefresh: instance.OAuthRefresh,
	}, true
}

type credentialOAuthRefresher struct {
	client provider.OAuthTokenRefresher
}

func (r credentialOAuthRefresher) RefreshOAuthToken(ctx context.Context, req credentials.OAuthRefreshRequest) (credentials.OAuthRefreshResult, error) {
	result, err := r.client.RefreshOAuthToken(ctx, provider.OAuthRefreshRequest{
		ProviderType: req.ProviderType,
		AuthIssuer:   req.AuthIssuer,
		RefreshToken: req.RefreshToken,
		Now:          req.Now,
	})
	return credentials.OAuthRefreshResult{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		IDToken:      result.IDToken,
		ExpiresAt:    result.ExpiresAt,
	}, adaptCredentialProviderError(err)
}

type credentialOAuthDeviceLogin struct {
	client provider.OAuthDeviceLoginProvider
}

func (l credentialOAuthDeviceLogin) RequestOAuthDeviceCode(ctx context.Context, req credentials.OAuthDeviceCodeRequest) (credentials.OAuthDeviceCodeChallenge, error) {
	challenge, err := l.client.RequestOAuthDeviceCode(ctx, provider.OAuthDeviceCodeRequest{
		ProviderInstanceID: req.ProviderInstanceID,
		ProviderType:       req.ProviderType,
		AuthIssuer:         req.AuthIssuer,
	})
	return credentials.OAuthDeviceCodeChallenge{
		VerificationURL: challenge.VerificationURL,
		UserCode:        challenge.UserCode,
		DeviceAuthID:    challenge.DeviceAuthID,
		IntervalSeconds: challenge.IntervalSeconds,
	}, adaptCredentialProviderError(err)
}

func (l credentialOAuthDeviceLogin) CompleteOAuthDeviceLogin(ctx context.Context, req credentials.OAuthDeviceLoginRequest) (credentials.OAuthDeviceLoginResult, error) {
	result, err := l.client.CompleteOAuthDeviceLogin(ctx, provider.OAuthDeviceLoginRequest{
		ProviderInstanceID: req.ProviderInstanceID,
		ProviderType:       req.ProviderType,
		AuthIssuer:         req.AuthIssuer,
		DeviceAuthID:       req.DeviceAuthID,
		UserCode:           req.UserCode,
		IntervalSeconds:    req.IntervalSeconds,
		Now:                req.Now,
	})
	return credentials.OAuthDeviceLoginResult{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		IDToken:      result.IDToken,
		ExpiresAt:    result.ExpiresAt,
	}, adaptCredentialProviderError(err)
}

type credentialProviderError struct {
	err                error
	deviceLoginClass   string
	deviceLoginEventID string
	refreshClass       string
	refreshDescription string
}

func adaptCredentialProviderError(err error) error {
	if err == nil {
		return nil
	}
	wrapped := credentialProviderError{err: err}
	if deviceErr, ok := err.(provider.OAuthDeviceLoginError); ok {
		wrapped.deviceLoginClass = deviceErr.Class
		wrapped.deviceLoginEventID = deviceErr.EventID
	}
	if refreshErr, ok := err.(provider.OAuthRefreshError); ok {
		wrapped.refreshClass = refreshErr.Class
		wrapped.refreshDescription = refreshErr.Description
	}
	return wrapped
}

func (e credentialProviderError) Error() string {
	return e.err.Error()
}

func (e credentialProviderError) Unwrap() error {
	return e.err
}

func (e credentialProviderError) OAuthDeviceLoginErrorClass() string {
	return e.deviceLoginClass
}

func (e credentialProviderError) OAuthDeviceLoginErrorEventID() string {
	return e.deviceLoginEventID
}

func (e credentialProviderError) OAuthRefreshErrorClass() string {
	return e.refreshClass
}

func (e credentialProviderError) RefreshFailureClass() string {
	return e.refreshClass
}

func (e credentialProviderError) RefreshFailureDescription() string {
	return e.refreshDescription
}

func credentialsProviderRegistry(registry provider.Registry) credentialProviderRegistry {
	return credentialProviderRegistry{registry: registry}
}

func credentialsOAuthRefresher(client provider.OAuthTokenRefresher) credentialOAuthRefresher {
	return credentialOAuthRefresher{client: client}
}

func credentialsOAuthDeviceLogin(client provider.OAuthDeviceLoginProvider) credentialOAuthDeviceLogin {
	return credentialOAuthDeviceLogin{client: client}
}
