package management

import (
	"context"
	"net/http"
)

func (c Client) StartOAuthDeviceLogin(ctx context.Context, req StartOAuthDeviceLoginRequest) (StartOAuthDeviceLoginResponse, error) {
	var out StartOAuthDeviceLoginResponse
	if err := c.do(ctx, http.MethodPost, PathOAuthDeviceLogin+"/start", req, &out); err != nil {
		return StartOAuthDeviceLoginResponse{}, err
	}
	return out, nil
}

func (c Client) CompleteOAuthDeviceLogin(ctx context.Context, req CompleteOAuthDeviceLoginRequest) (CompleteOAuthDeviceLoginResponse, error) {
	var out CompleteOAuthDeviceLoginResponse
	if err := c.doWithClient(ctx, c.longPollClient(), http.MethodPost, PathOAuthDeviceLogin+"/complete", req, &out); err != nil {
		return CompleteOAuthDeviceLoginResponse{}, err
	}
	return out, nil
}

func (c Client) RefreshOAuthCredential(ctx context.Context, req RefreshOAuthCredentialRequest) (RefreshOAuthCredentialResponse, error) {
	var out RefreshOAuthCredentialResponse
	if err := c.do(ctx, http.MethodPost, PathOAuthCredentials+"/refresh", req, &out); err != nil {
		return RefreshOAuthCredentialResponse{}, err
	}
	return out, nil
}
