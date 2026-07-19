package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"time"

	"ilonasin/internal/metadata"
)

const MaxUpstreamModelsBodyBytes int64 = 64 << 20

func (a HTTPChatAdapter) ListModels(ctx context.Context, req ModelRequest) (ModelResult, error) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, a.modelTimeout())
	defer cancel()
	endpoint, err := a.modelsURL(ctx, req.Instance)
	if err != nil {
		return ModelResult{ErrorClass: "provider_config_error"}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ModelResult{ErrorClass: "upstream_request_error"}, err
	}
	if req.Instance.Type == "codex" {
		a.addCodexRequestHeaders(ctx, httpReq, req.Credential.BearerToken, req.Credential.ChatGPTAccountID, req.Credential.ChatGPTAccountIsFedRAMP)
	} else {
		httpReq.Header.Set("Authorization", "Bearer "+req.Credential.BearerToken)
	}
	httpReq.Header.Set("Accept", "application/json")
	resp, err := a.Client.Do(httpReq)
	if err != nil {
		errorClass := classifyTransportError(err)
		logProviderHTTP(ctx, a.Logger, statusLevel(http.StatusBadGateway, errorClass), "provider_http",
			slog.String("endpoint", "models"),
			slog.String("method", http.MethodGet),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", req.Credential.ID),
			slog.Int64("duration_ms", durationMS(start)),
			slog.String("error_class", errorClass),
		)
		return ModelResult{ErrorClass: errorClass}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errorClass := "upstream_http_error"
		if resp.StatusCode == http.StatusUnauthorized {
			errorClass = "upstream_auth_failed"
		}
		logProviderHTTP(ctx, a.Logger, statusLevel(resp.StatusCode, errorClass), "provider_http",
			slog.String("endpoint", "models"),
			slog.String("method", http.MethodGet),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", req.Credential.ID),
			slog.Int("status", resp.StatusCode),
			slog.Int64("duration_ms", durationMS(start)),
			slog.String("error_class", errorClass),
		)
		return ModelResult{ErrorClass: errorClass, StatusCode: resp.StatusCode, RetryAfter: retryAfterFromHeader(resp.Header, time.Now())}, fmt.Errorf("upstream models status %d", resp.StatusCode)
	}
	if resp.ContentLength > MaxUpstreamModelsBodyBytes {
		logProviderHTTP(ctx, a.Logger, slog.LevelError, "provider_http",
			slog.String("endpoint", "models"),
			slog.String("method", http.MethodGet),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", req.Credential.ID),
			slog.Int("status", resp.StatusCode),
			slog.Int64("duration_ms", durationMS(start)),
			slog.String("error_class", "upstream_body_too_large"),
		)
		return ModelResult{ErrorClass: "upstream_body_too_large", StatusCode: resp.StatusCode}, fmt.Errorf("upstream models body exceeded limit")
	}
	body, tooLarge, readErr := readLimitedUpstreamBody(resp.Body, MaxUpstreamModelsBodyBytes)
	a.recordUpstreamBody(req.Instance, req.Credential.ID, "models", http.MethodGet, "upstream_output", resp.StatusCode, resp.Header.Get("Content-Type"), body, "")
	if tooLarge {
		logProviderHTTP(ctx, a.Logger, slog.LevelError, "provider_http",
			slog.String("endpoint", "models"),
			slog.String("method", http.MethodGet),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", req.Credential.ID),
			slog.Int("status", resp.StatusCode),
			slog.Int64("duration_ms", durationMS(start)),
			slog.String("error_class", "upstream_body_too_large"),
		)
		return ModelResult{ErrorClass: "upstream_body_too_large", StatusCode: resp.StatusCode}, fmt.Errorf("upstream models body exceeded limit")
	}
	if readErr != nil {
		logProviderHTTP(ctx, a.Logger, slog.LevelError, "provider_http",
			slog.String("endpoint", "models"),
			slog.String("method", http.MethodGet),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", req.Credential.ID),
			slog.Int("status", resp.StatusCode),
			slog.Int64("duration_ms", durationMS(start)),
			slog.String("error_class", "upstream_network_error"),
		)
		return ModelResult{ErrorClass: "upstream_network_error", StatusCode: resp.StatusCode}, readErr
	}
	models, err := normalizeModels(req.Instance, body)
	if err != nil {
		logProviderHTTP(ctx, a.Logger, slog.LevelError, "provider_http",
			slog.String("endpoint", "models"),
			slog.String("method", http.MethodGet),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", req.Credential.ID),
			slog.Int("status", resp.StatusCode),
			slog.Int64("duration_ms", durationMS(start)),
			slog.Int("response_bytes", len(body)),
			slog.String("error_class", "upstream_invalid_response"),
		)
		return ModelResult{ErrorClass: "upstream_invalid_response", StatusCode: resp.StatusCode}, err
	}
	logProviderHTTP(ctx, a.Logger, slog.LevelInfo, "provider_http",
		slog.String("endpoint", "models"),
		slog.String("method", http.MethodGet),
		slog.String("provider_instance", req.Instance.ID),
		slog.String("provider_type", req.Instance.Type),
		slog.Int64("credential_id", req.Credential.ID),
		slog.Int("status", resp.StatusCode),
		slog.Int64("duration_ms", durationMS(start)),
		slog.Int("response_bytes", len(body)),
		slog.Int("model_count", len(models)),
	)
	return ModelResult{Models: models, StatusCode: resp.StatusCode}, nil
}

func (a HTTPChatAdapter) modelsURL(ctx context.Context, instance Instance) (string, error) {
	endpoint, err := joinBasePath(instance.BaseURL, "/models")
	if err != nil {
		return "", err
	}
	if instance.Type != "codex" {
		return endpoint, nil
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("client_version", a.codexClientVersion(ctx))
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func normalizeModels(instance Instance, body []byte) ([]ModelMetadata, error) {
	if instance.Type == "codex" {
		return normalizeCodexModels(instance, body)
	}
	var resp struct {
		Data []map[string]any `json:"data"`
	}
	if err := jsonUnmarshal(body, &resp); err != nil {
		return nil, err
	}
	if resp.Data == nil {
		return nil, errors.New("upstream models data is missing")
	}
	now := time.Now().UTC()
	models := make([]ModelMetadata, 0, len(resp.Data))
	seen := map[string]bool{}
	for _, item := range resp.Data {
		id, _ := item["id"].(string)
		if !validProviderModelID(id) {
			continue
		}
		if seen[id] {
			return nil, fmt.Errorf("upstream models contains duplicate id")
		}
		seen[id] = true
		meta := ModelMetadata{
			ProviderInstanceID: instance.ID,
			ModelID:            id,
			UpdatedAt:          now,
		}
		switch instance.Type {
		case "openrouter":
			if name, ok := item["name"].(string); ok {
				meta.DisplayName = safeDisplayName(name)
			}
			meta.ContextLength = safeInt(item["context_length"])
			meta.CapabilityFlags = openRouterCapabilityFlags(item)
		case "deepseek":
			meta.CapabilityFlags = metadata.FormatModelCapabilities(
				metadata.ModelCapabilityChat,
				metadata.ModelCapabilityJSONObject,
				metadata.ModelCapabilityLogprobs,
				metadata.ModelCapabilityReasoning,
				metadata.ModelCapabilityStream,
				metadata.ModelCapabilityTools,
			)
		}
		models = append(models, meta)
	}
	if len(models) == 0 {
		return nil, errors.New("upstream models list is empty")
	}
	sort.Slice(models, func(i, j int) bool {
		return models[i].ModelID < models[j].ModelID
	})
	return models, nil
}

func normalizeCodexModels(instance Instance, body []byte) ([]ModelMetadata, error) {
	// Mirrors OpenAI Codex rust-v0.144.1 ModelInfo serde behavior through the
	// typed wire DTO in codex_models_wire.go.
	items, err := decodeCodexModels(body)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	models := make([]ModelMetadata, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		id := item.Slug.Value
		if !validProviderModelID(id) {
			continue
		}
		if seen[id] {
			return nil, fmt.Errorf("upstream models contains duplicate id")
		}
		seen[id] = true
		models = append(models, item.providerModelMetadata(instance.ID, now))
	}
	if len(models) == 0 {
		return nil, errors.New("upstream codex models list is empty")
	}
	sort.Slice(models, func(i, j int) bool {
		return models[i].ModelID < models[j].ModelID
	})
	return models, nil
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func validProviderModelID(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func safeDisplayName(name string) string {
	if len(name) > 256 {
		return name[:256]
	}
	return name
}

func safeReasoningEffort(value string) string {
	return safeBoundedText(value, 64)
}

func safeReasoningDescription(value string) string {
	return safeBoundedText(value, 1024)
}

func safeBoundedText(value string, maxLen int) string {
	if value == "" || len(value) > maxLen {
		return ""
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return ""
		}
	}
	return value
}

func safeInt(value any) *int64 {
	var integer int64
	switch v := value.(type) {
	case json.Number:
		parsed, err := v.Int64()
		if err != nil {
			return nil
		}
		integer = parsed
	case float64:
		if v != float64(int64(v)) {
			return nil
		}
		integer = int64(v)
	default:
		return nil
	}
	if integer <= 0 {
		return nil
	}
	return &integer
}

func (a HTTPChatAdapter) modelTimeout() time.Duration {
	if a.ModelTimeout > 0 {
		return a.ModelTimeout
	}
	return 30 * time.Second
}
