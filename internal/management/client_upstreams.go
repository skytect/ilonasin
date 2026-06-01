package management

import (
	"context"
	"net/http"
)

func (c Client) AddUpstreamAPIKey(ctx context.Context, req AddUpstreamAPIKeyRequest) (AddUpstreamAPIKeyResponse, error) {
	var out AddUpstreamAPIKeyResponse
	if err := c.do(ctx, http.MethodPost, PathUpstreamCredentials, req, &out); err != nil {
		return AddUpstreamAPIKeyResponse{}, err
	}
	return out, nil
}

func (c Client) DisableUpstreamCredential(ctx context.Context, req DisableUpstreamCredentialRequest) (DisableUpstreamCredentialResponse, error) {
	var out DisableUpstreamCredentialResponse
	if err := c.do(ctx, http.MethodPost, PathUpstreamCredentials+"/disable", req, &out); err != nil {
		return DisableUpstreamCredentialResponse{}, err
	}
	return out, nil
}

func (c Client) EnableFallbackPolicy(ctx context.Context, req FallbackPolicyRequest) (FallbackPolicyResponse, error) {
	var out FallbackPolicyResponse
	if err := c.do(ctx, http.MethodPost, PathFallbackPolicies+"/enable", req, &out); err != nil {
		return FallbackPolicyResponse{}, err
	}
	return out, nil
}

func (c Client) DisableFallbackPolicy(ctx context.Context, req FallbackPolicyRequest) (FallbackPolicyResponse, error) {
	var out FallbackPolicyResponse
	if err := c.do(ctx, http.MethodPost, PathFallbackPolicies+"/disable", req, &out); err != nil {
		return FallbackPolicyResponse{}, err
	}
	return out, nil
}
