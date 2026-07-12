package provider

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const MaxCodexUsageBodyBytes int64 = 4 << 20
const MaxCodexResetInventoryBodyBytes int64 = 1 << 20
const CodexResetInventoryTimeout = 5 * time.Second

type CodexSubscriptionUsageClient interface {
	FetchCodexSubscriptionUsage(ctx context.Context, req CodexSubscriptionUsageRequest) (CodexSubscriptionUsageResult, error)
}

type CodexSubscriptionUsageRequest struct {
	Instance   Instance
	Credential BearerCredential
}

type CodexSubscriptionUsageResult struct {
	Snapshots            []CodexRateLimitSnapshot
	BankedResetInventory CodexBankedResetInventory
	ErrorClass           string
	StatusCode           int
}

type CodexRateLimitSnapshot struct {
	LimitID     string
	LimitName   string
	PlanType    string
	ReachedType string
	Primary     *CodexRateLimitWindow
	Secondary   *CodexRateLimitWindow
}

type CodexRateLimitWindow struct {
	UsedPercent   float64
	WindowMinutes int
	ResetsAt      *time.Time
}

type CodexBankedResetInventory struct {
	AvailableCount   *int
	DetailsAvailable bool
	DetailErrorClass string
	Details          []CodexBankedResetDetail
}

type CodexBankedResetDetail struct {
	ResetType string
	Status    string
	GrantedAt time.Time
	ExpiresAt *time.Time
}

func (a HTTPChatAdapter) FetchCodexSubscriptionUsage(ctx context.Context, req CodexSubscriptionUsageRequest) (CodexSubscriptionUsageResult, error) {
	start := time.Now()
	if req.Instance.Type != "codex" {
		return CodexSubscriptionUsageResult{ErrorClass: "unsupported_provider"}, fmt.Errorf("subscription usage requires codex provider")
	}
	if req.Credential.Kind != CredentialKindOAuthAccess {
		return CodexSubscriptionUsageResult{StatusCode: http.StatusUnauthorized, ErrorClass: "credential_unavailable"}, fmt.Errorf("subscription usage requires oauth access credential")
	}
	ctx, cancel := context.WithTimeout(ctx, a.modelTimeout())
	defer cancel()
	endpoint, err := codexUsageURL(req.Instance)
	if err != nil {
		return CodexSubscriptionUsageResult{ErrorClass: "provider_config_error"}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return CodexSubscriptionUsageResult{ErrorClass: "upstream_request_error"}, err
	}
	a.addCodexRequestHeaders(ctx, httpReq, req.Credential.BearerToken, req.Credential.ChatGPTAccountID, req.Credential.ChatGPTAccountIsFedRAMP)
	httpReq.Header.Set("Accept", "application/json")
	client := a.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		class := classifyTransportError(err)
		logProviderHTTP(ctx, a.Logger, statusLevel(http.StatusBadGateway, class), "provider_http",
			slog.String("endpoint", "subscription_usage"),
			slog.String("method", http.MethodGet),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", req.Credential.ID),
			slog.Int64("duration_ms", durationMS(start)),
			slog.String("error_class", class),
		)
		return CodexSubscriptionUsageResult{ErrorClass: class}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		class := "upstream_http_error"
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			class = "upstream_auth_failed"
		} else if resp.StatusCode == http.StatusTooManyRequests {
			class = "rate_limit_exceeded"
		}
		logProviderHTTP(ctx, a.Logger, statusLevel(resp.StatusCode, class), "provider_http",
			slog.String("endpoint", "subscription_usage"),
			slog.String("method", http.MethodGet),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", req.Credential.ID),
			slog.Int("status", resp.StatusCode),
			slog.Int64("duration_ms", durationMS(start)),
			slog.String("error_class", class),
		)
		return CodexSubscriptionUsageResult{StatusCode: resp.StatusCode, ErrorClass: class}, fmt.Errorf("codex usage status %d", resp.StatusCode)
	}
	if resp.ContentLength > MaxCodexUsageBodyBytes {
		logProviderHTTP(ctx, a.Logger, slog.LevelError, "provider_http",
			slog.String("endpoint", "subscription_usage"),
			slog.String("method", http.MethodGet),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", req.Credential.ID),
			slog.Int("status", resp.StatusCode),
			slog.Int64("duration_ms", durationMS(start)),
			slog.String("error_class", "upstream_body_too_large"),
		)
		return CodexSubscriptionUsageResult{StatusCode: resp.StatusCode, ErrorClass: "upstream_body_too_large"}, fmt.Errorf("codex usage body exceeded limit")
	}
	body, tooLarge, readErr := readLimitedUpstreamBody(resp.Body, MaxCodexUsageBodyBytes)
	a.recordUpstreamBody(req.Instance, req.Credential.ID, "subscription_usage", http.MethodGet, "upstream_output", resp.StatusCode, resp.Header.Get("Content-Type"), body, "")
	if tooLarge {
		logProviderHTTP(ctx, a.Logger, slog.LevelError, "provider_http",
			slog.String("endpoint", "subscription_usage"),
			slog.String("method", http.MethodGet),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", req.Credential.ID),
			slog.Int("status", resp.StatusCode),
			slog.Int64("duration_ms", durationMS(start)),
			slog.String("error_class", "upstream_body_too_large"),
		)
		return CodexSubscriptionUsageResult{StatusCode: resp.StatusCode, ErrorClass: "upstream_body_too_large"}, fmt.Errorf("codex usage body exceeded limit")
	}
	if readErr != nil {
		logProviderHTTP(ctx, a.Logger, slog.LevelError, "provider_http",
			slog.String("endpoint", "subscription_usage"),
			slog.String("method", http.MethodGet),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", req.Credential.ID),
			slog.Int("status", resp.StatusCode),
			slog.Int64("duration_ms", durationMS(start)),
			slog.String("error_class", "upstream_network_error"),
		)
		return CodexSubscriptionUsageResult{StatusCode: resp.StatusCode, ErrorClass: "upstream_network_error"}, readErr
	}
	snapshots, inventory, err := decodeCodexUsagePayload(body)
	if err != nil {
		logProviderHTTP(ctx, a.Logger, slog.LevelError, "provider_http",
			slog.String("endpoint", "subscription_usage"),
			slog.String("method", http.MethodGet),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", req.Credential.ID),
			slog.Int("status", resp.StatusCode),
			slog.Int64("duration_ms", durationMS(start)),
			slog.Int("response_bytes", len(body)),
			slog.String("error_class", "upstream_invalid_response"),
		)
		return CodexSubscriptionUsageResult{StatusCode: resp.StatusCode, ErrorClass: "upstream_invalid_response"}, err
	}
	inventory = a.fetchCodexBankedResetInventory(ctx, req, inventory)
	logProviderHTTP(ctx, a.Logger, slog.LevelInfo, "provider_http",
		slog.String("endpoint", "subscription_usage"),
		slog.String("method", http.MethodGet),
		slog.String("provider_instance", req.Instance.ID),
		slog.String("provider_type", req.Instance.Type),
		slog.Int64("credential_id", req.Credential.ID),
		slog.Int("status", resp.StatusCode),
		slog.Int64("duration_ms", durationMS(start)),
		slog.Int("response_bytes", len(body)),
	)
	return CodexSubscriptionUsageResult{StatusCode: resp.StatusCode, Snapshots: snapshots, BankedResetInventory: inventory}, nil
}

func codexUsageURL(instance Instance) (string, error) {
	u, err := url.Parse(instance.BaseURL)
	if err != nil {
		return "", err
	}
	path := strings.TrimRight(u.Path, "/")
	switch {
	case strings.HasSuffix(path, "/backend-api/codex"):
		u.Path = strings.TrimSuffix(path, "/codex") + "/wham/usage"
	case strings.HasSuffix(path, "/backend-api"):
		u.Path = path + "/wham/usage"
	case strings.HasSuffix(path, "/codex"):
		u.Path = strings.TrimSuffix(path, "/codex") + "/wham/usage"
	default:
		u.Path = path + "/api/codex/usage"
	}
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

func codexRateLimitResetCreditsURL(instance Instance) (string, error) {
	u, err := url.Parse(instance.BaseURL)
	if err != nil {
		return "", err
	}
	path := strings.TrimRight(u.Path, "/")
	switch {
	case strings.HasSuffix(path, "/backend-api/codex"):
		u.Path = strings.TrimSuffix(path, "/codex") + "/wham/rate-limit-reset-credits"
	case strings.HasSuffix(path, "/backend-api"):
		u.Path = path + "/wham/rate-limit-reset-credits"
	case strings.HasSuffix(path, "/codex"):
		u.Path = strings.TrimSuffix(path, "/codex") + "/wham/rate-limit-reset-credits"
	default:
		u.Path = path + "/api/codex/rate-limit-reset-credits"
	}
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

func (a HTTPChatAdapter) fetchCodexBankedResetInventory(ctx context.Context, req CodexSubscriptionUsageRequest, inventory CodexBankedResetInventory) CodexBankedResetInventory {
	if inventory.AvailableCount == nil {
		return inventory
	}
	if *inventory.AvailableCount == 0 {
		inventory.DetailsAvailable = true
		inventory.Details = []CodexBankedResetDetail{}
		return inventory
	}
	detailCtx, cancel := context.WithTimeout(ctx, CodexResetInventoryTimeout)
	defer cancel()
	start := time.Now()
	endpoint, err := codexRateLimitResetCreditsURL(req.Instance)
	if err != nil {
		inventory.DetailErrorClass = "provider_config_error"
		return inventory
	}
	httpReq, err := http.NewRequestWithContext(detailCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		inventory.DetailErrorClass = "upstream_request_error"
		return inventory
	}
	a.addCodexRequestHeaders(detailCtx, httpReq, req.Credential.BearerToken, req.Credential.ChatGPTAccountID, req.Credential.ChatGPTAccountIsFedRAMP)
	httpReq.Header.Set("Accept", "application/json")
	client := a.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		class := classifyTransportError(err)
		logProviderHTTP(ctx, a.Logger, statusLevel(http.StatusBadGateway, class), "provider_http",
			slog.String("endpoint", "rate_limit_reset_inventory"),
			slog.String("method", http.MethodGet),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", req.Credential.ID),
			slog.Int64("duration_ms", durationMS(start)),
			slog.String("error_class", class),
		)
		inventory.DetailErrorClass = class
		return inventory
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		class := "upstream_http_error"
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			class = "upstream_auth_failed"
		} else if resp.StatusCode == http.StatusTooManyRequests {
			class = "rate_limit_exceeded"
		}
		logProviderHTTP(ctx, a.Logger, statusLevel(resp.StatusCode, class), "provider_http",
			slog.String("endpoint", "rate_limit_reset_inventory"),
			slog.String("method", http.MethodGet),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", req.Credential.ID),
			slog.Int("status", resp.StatusCode),
			slog.Int64("duration_ms", durationMS(start)),
			slog.String("error_class", class),
		)
		inventory.DetailErrorClass = class
		return inventory
	}
	if resp.ContentLength > MaxCodexResetInventoryBodyBytes {
		inventory.DetailErrorClass = "upstream_body_too_large"
		return inventory
	}
	body, tooLarge, readErr := readLimitedUpstreamBody(resp.Body, MaxCodexResetInventoryBodyBytes)
	if tooLarge {
		inventory.DetailErrorClass = "upstream_body_too_large"
		return inventory
	}
	if readErr != nil {
		inventory.DetailErrorClass = "upstream_network_error"
		return inventory
	}
	details, err := decodeCodexBankedResetDetails(body)
	if err != nil {
		logProviderHTTP(ctx, a.Logger, slog.LevelError, "provider_http",
			slog.String("endpoint", "rate_limit_reset_inventory"),
			slog.String("method", http.MethodGet),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", req.Credential.ID),
			slog.Int("status", resp.StatusCode),
			slog.Int64("duration_ms", durationMS(start)),
			slog.Int("response_bytes", len(body)),
			slog.String("error_class", "upstream_invalid_response"),
		)
		inventory.DetailErrorClass = "upstream_invalid_response"
		return inventory
	}
	inventory.DetailsAvailable = true
	inventory.DetailErrorClass = ""
	inventory.Details = details
	logProviderHTTP(ctx, a.Logger, slog.LevelInfo, "provider_http",
		slog.String("endpoint", "rate_limit_reset_inventory"),
		slog.String("method", http.MethodGet),
		slog.String("provider_instance", req.Instance.ID),
		slog.String("provider_type", req.Instance.Type),
		slog.Int64("credential_id", req.Credential.ID),
		slog.Int("status", resp.StatusCode),
		slog.Int64("duration_ms", durationMS(start)),
		slog.Int("response_bytes", len(body)),
	)
	return inventory
}

type codexBankedResetDetailsPayload struct {
	Credits []codexBankedResetDetailPayload `json:"credits"`
}

type codexBankedResetDetailPayload struct {
	ResetType string  `json:"reset_type"`
	Status    string  `json:"status"`
	GrantedAt string  `json:"granted_at"`
	ExpiresAt *string `json:"expires_at"`
}

func decodeCodexBankedResetDetails(body []byte) ([]CodexBankedResetDetail, error) {
	var payload codexBankedResetDetailsPayload
	if err := jsonUnmarshal(body, &payload); err != nil {
		return nil, err
	}
	out := make([]CodexBankedResetDetail, 0, len(payload.Credits))
	for _, row := range payload.Credits {
		grantedAt, err := time.Parse(time.RFC3339, row.GrantedAt)
		if err != nil {
			return nil, err
		}
		var expiresAt *time.Time
		if row.ExpiresAt != nil {
			parsed, err := time.Parse(time.RFC3339, *row.ExpiresAt)
			if err != nil {
				return nil, err
			}
			parsed = parsed.UTC()
			expiresAt = &parsed
		}
		out = append(out, CodexBankedResetDetail{
			ResetType: codexBankedResetToken(row.ResetType),
			Status:    codexBankedResetToken(row.Status),
			GrantedAt: grantedAt.UTC(),
			ExpiresAt: expiresAt,
		})
	}
	return out, nil
}

func codexBankedResetToken(value string) string {
	value = safeCodexUsageToken(value)
	if value == "" {
		return "unknown"
	}
	return value
}

type codexSubscriptionUsagePayload struct {
	PlanType             string                                    `json:"plan_type"`
	RateLimit            *codexSubscriptionUsageRateLimitStatus    `json:"rate_limit"`
	AdditionalRateLimits []codexSubscriptionUsageAdditionalLimit   `json:"additional_rate_limits"`
	ReachedType          *codexSubscriptionUsageReachedTypePayload `json:"rate_limit_reached_type"`
	ResetCredits         *codexBankedResetSummaryPayload           `json:"rate_limit_reset_credits"`
}

type codexBankedResetSummaryPayload struct {
	AvailableCount *int `json:"available_count"`
}

type codexSubscriptionUsageRateLimitStatus struct {
	PrimaryWindow   *codexSubscriptionUsageWindow `json:"primary_window"`
	SecondaryWindow *codexSubscriptionUsageWindow `json:"secondary_window"`
}

type codexSubscriptionUsageWindow struct {
	UsedPercent        float64 `json:"used_percent"`
	LimitWindowSeconds int     `json:"limit_window_seconds"`
	ResetAt            int64   `json:"reset_at"`
}

type codexSubscriptionUsageAdditionalLimit struct {
	LimitName      string                                 `json:"limit_name"`
	MeteredFeature string                                 `json:"metered_feature"`
	RateLimit      *codexSubscriptionUsageRateLimitStatus `json:"rate_limit"`
}

type codexSubscriptionUsageReachedTypePayload struct {
	Kind string `json:"type"`
}

func decodeCodexUsagePayload(body []byte) ([]CodexRateLimitSnapshot, CodexBankedResetInventory, error) {
	var payload codexSubscriptionUsagePayload
	if err := jsonUnmarshal(body, &payload); err != nil {
		return nil, CodexBankedResetInventory{}, err
	}
	out := []CodexRateLimitSnapshot{codexRateLimitSnapshot("codex", "", payload.PlanType, reachedType(payload.ReachedType), payload.RateLimit)}
	for _, additional := range payload.AdditionalRateLimits {
		out = append(out, codexRateLimitSnapshot(additional.MeteredFeature, additional.LimitName, payload.PlanType, "", additional.RateLimit))
	}
	return out, codexBankedResetInventoryFromUsage(payload.ResetCredits), nil
}

func codexBankedResetInventoryFromUsage(summary *codexBankedResetSummaryPayload) CodexBankedResetInventory {
	if summary == nil || summary.AvailableCount == nil || *summary.AvailableCount < 0 {
		return CodexBankedResetInventory{}
	}
	count := *summary.AvailableCount
	return CodexBankedResetInventory{AvailableCount: &count}
}

func codexRateLimitSnapshot(limitID, limitName, planType, reachedType string, rateLimit *codexSubscriptionUsageRateLimitStatus) CodexRateLimitSnapshot {
	var primary, secondary *CodexRateLimitWindow
	if rateLimit != nil {
		primary = codexRateLimitWindow(rateLimit.PrimaryWindow)
		secondary = codexRateLimitWindow(rateLimit.SecondaryWindow)
	}
	return CodexRateLimitSnapshot{
		LimitID:     safeCodexUsageToken(limitID),
		LimitName:   safeCodexUsageLabel(limitName),
		PlanType:    safeCodexUsageToken(planType),
		ReachedType: safeCodexUsageToken(reachedType),
		Primary:     primary,
		Secondary:   secondary,
	}
}

func codexRateLimitWindow(window *codexSubscriptionUsageWindow) *CodexRateLimitWindow {
	if window == nil {
		return nil
	}
	var resetsAt *time.Time
	if window.ResetAt > 0 {
		t := time.Unix(window.ResetAt, 0).UTC()
		resetsAt = &t
	}
	return &CodexRateLimitWindow{
		UsedPercent:   clampPercent(window.UsedPercent),
		WindowMinutes: window.LimitWindowSeconds / 60,
		ResetsAt:      resetsAt,
	}
}

func reachedType(value *codexSubscriptionUsageReachedTypePayload) string {
	if value == nil {
		return ""
	}
	return value.Kind
}

func clampPercent(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func safeCodexUsageToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return ""
	}
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			continue
		}
		return ""
	}
	return value
}

func safeCodexUsageLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	out := b.String()
	runes := []rune(out)
	if len(runes) > 128 {
		out = string(runes[:128])
	}
	return out
}
