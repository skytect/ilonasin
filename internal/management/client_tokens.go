package management

import (
	"context"
	"net/http"
)

func (c Client) ListLocalTokens(ctx context.Context) (ListLocalTokensResponse, error) {
	var out ListLocalTokensResponse
	if err := c.do(ctx, http.MethodGet, PathLocalTokens, nil, &out); err != nil {
		return ListLocalTokensResponse{}, err
	}
	return out, nil
}

func (c Client) CreateLocalToken(ctx context.Context, req CreateLocalTokenRequest) (CreateLocalTokenResponse, error) {
	var out CreateLocalTokenResponse
	if err := c.do(ctx, http.MethodPost, PathLocalTokens, req, &out); err != nil {
		return CreateLocalTokenResponse{}, err
	}
	return out, nil
}

func (c Client) DisableLocalToken(ctx context.Context, req DisableLocalTokenRequest) (DisableLocalTokenResponse, error) {
	var out DisableLocalTokenResponse
	if err := c.do(ctx, http.MethodPost, PathLocalTokens+"/disable", req, &out); err != nil {
		return DisableLocalTokenResponse{}, err
	}
	return out, nil
}
