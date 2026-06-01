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
	"strings"
	"time"
)

const MaxUpstreamModelsBodyBytes int64 = 64 << 20

func (a HTTPChatAdapter) ListModels(ctx context.Context, req ModelRequest) (ModelResult, error) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, a.modelTimeout())
	defer cancel()
	endpoint, err := modelsURL(req.Instance)
	if err != nil {
		return ModelResult{ErrorClass: "provider_config_error"}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ModelResult{ErrorClass: "upstream_request_error"}, err
	}
	if req.Instance.Type == "codex" {
		addCodexRequestHeaders(httpReq, req.Credential.BearerToken, req.Credential.ChatGPTAccountID, req.Credential.ChatGPTAccountIsFedRAMP)
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

func modelsURL(instance Instance) (string, error) {
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
	q.Set("client_version", CodexClientVersion)
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
			meta.CapabilityFlags = "chat,json_object,logprobs,reasoning,stream,tools"
		case "codex":
			meta.CapabilityFlags = "chat,reasoning,stream"
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
	// Mirrors OpenAI Codex rust-v0.135.0:
	// codex-rs/codex-api/src/endpoint/models.rs decodes ModelsResponse,
	// whose envelope is protocol/src/openai_models.rs ModelsResponse { models }.
	var resp struct {
		Models []map[string]any `json:"models"`
	}
	if err := jsonUnmarshal(body, &resp); err != nil {
		return nil, err
	}
	if resp.Models == nil {
		return nil, errors.New("upstream codex models list is missing")
	}
	now := time.Now().UTC()
	models := make([]ModelMetadata, 0, len(resp.Models))
	seen := map[string]bool{}
	for _, item := range resp.Models {
		id, _ := item["slug"].(string)
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
			CapabilityFlags:    codexCapabilityFlags(item),
			UpdatedAt:          now,
		}
		if name, ok := item["display_name"].(string); ok {
			meta.DisplayName = safeDisplayName(name)
		}
		meta.ContextLength = safeInt(item["context_window"])
		meta.DefaultServiceTier = safeCodexServiceTierID(codexStringField(item, "default_service_tier"))
		meta.ServiceTiers = codexServiceTiers(item)
		meta.InputModalities = codexInputModalities(item)
		models = append(models, meta)
	}
	if len(models) == 0 {
		return nil, errors.New("upstream codex models list is empty")
	}
	sort.Slice(models, func(i, j int) bool {
		return models[i].ModelID < models[j].ModelID
	})
	return models, nil
}

func codexCapabilityFlags(item map[string]any) string {
	flags := map[string]bool{
		"chat":      true,
		"responses": true,
		"stream":    true,
		"tools":     true,
	}
	if codexStringField(item, "default_reasoning_level") != "" || len(codexArrayField(item, "supported_reasoning_levels")) > 0 {
		flags["reasoning"] = true
	}
	if codexBoolField(item, "supports_parallel_tool_calls") {
		flags["parallel_tool_calls"] = true
	}
	if hasCodexServiceTier(item) {
		flags["service_tier"] = true
	}
	if hasCodexVisionCapability(item) {
		flags["vision"] = true
	}
	out := make([]string, 0, len(flags))
	for flag := range flags {
		out = append(out, flag)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

func hasCodexServiceTier(item map[string]any) bool {
	return len(codexServiceTiers(item)) > 0
}

func codexServiceTiers(item map[string]any) []ModelServiceTier {
	var out []ModelServiceTier
	seen := map[string]bool{}
	for _, raw := range codexArrayField(item, "service_tiers") {
		tier, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		id := safeCodexServiceTierID(codexStringField(tier, "id"))
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		switch id {
		case "priority":
			out = append(out, ModelServiceTier{
				ID:          "priority",
				Name:        "Fast",
				Description: "1.5x speed, increased usage",
			})
		case "flex":
			out = append(out, ModelServiceTier{
				ID:          "flex",
				Name:        "flex",
				Description: "Flexible inference tier",
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func safeCodexServiceTierID(value string) string {
	switch value {
	case "priority", "flex":
		return value
	default:
		return ""
	}
}

func codexInputModalities(item map[string]any) []string {
	if _, ok := item["input_modalities"]; !ok {
		return []string{"text", "image"}
	}
	seen := map[string]bool{}
	for _, raw := range codexArrayField(item, "input_modalities") {
		value, ok := raw.(string)
		if !ok {
			continue
		}
		switch value {
		case "text", "image":
			seen[value] = true
		}
	}
	out := make([]string, 0, len(seen))
	for _, value := range []string{"text", "image"} {
		if seen[value] {
			out = append(out, value)
		}
	}
	return out
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func hasCodexVisionCapability(item map[string]any) bool {
	return containsString(codexInputModalities(item), "image")
}

func codexStringField(item map[string]any, key string) string {
	value, _ := item[key].(string)
	return value
}

func codexBoolField(item map[string]any, key string) bool {
	value, _ := item[key].(bool)
	return value
}

func codexArrayField(item map[string]any, key string) []any {
	value, _ := item[key].([]any)
	return value
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

func safeInt(value any) int {
	switch v := value.(type) {
	case json.Number:
		i, err := v.Int64()
		if err == nil && i > 0 && i <= int64(^uint(0)>>1) {
			return int(i)
		}
	case float64:
		if v > 0 && v == float64(int(v)) {
			return int(v)
		}
	}
	return 0
}

func (a HTTPChatAdapter) modelTimeout() time.Duration {
	if a.ModelTimeout > 0 {
		return a.ModelTimeout
	}
	return 30 * time.Second
}
