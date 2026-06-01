package management

import (
	"context"
	"net/http"
)

func (c Client) PruneTelemetry(ctx context.Context, req PruneTelemetryRequest) (PruneTelemetryResponse, error) {
	var out PruneTelemetryResponse
	if err := c.doWithClient(ctx, c.longPollClient(), http.MethodPost, PathTelemetryPrune, req, &out); err != nil {
		return PruneTelemetryResponse{}, err
	}
	return out, nil
}

func (c Client) GetSubscriptionUsage(ctx context.Context) (SubscriptionUsageResponse, error) {
	var out SubscriptionUsageResponse
	if err := c.do(ctx, http.MethodGet, PathSubscriptionUsage, nil, &out); err != nil {
		return SubscriptionUsageResponse{}, err
	}
	return out, nil
}

func (c Client) RefreshSubscriptionUsage(ctx context.Context) (SubscriptionUsageResponse, error) {
	var out SubscriptionUsageResponse
	if err := c.doWithClient(ctx, c.longPollClient(), http.MethodPost, PathSubscriptionUsage+"/refresh", nil, &out); err != nil {
		return SubscriptionUsageResponse{}, err
	}
	return out, nil
}
