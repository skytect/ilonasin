package provider

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"ilonasin/internal/openai"
)

const MaxCodexAggregateAssistantBytes = 64 << 20

func (a HTTPChatAdapter) completeCodexChat(ctx context.Context, req ChatRequest, start time.Time) (ChatResult, error) {
	if req.Credential.Kind != CredentialKindOAuthAccess {
		return ChatResult{StatusCode: http.StatusUnauthorized, ContentType: "application/json", ErrorClass: "credential_unavailable", Latency: time.Since(start)}, fmt.Errorf("codex chat requires oauth access credential")
	}
	ids := newCodexRequestIDs()
	modelMeta, err := a.resolveCodexResponsesModel(ctx, req, start)
	if err != nil {
		if errors.Is(err, errCodexModelAuthFailed) {
			return ChatResult{StatusCode: http.StatusUnauthorized, ContentType: "application/json", ErrorClass: "model_discovery_auth_failed", Latency: time.Since(start)}, err
		}
		errorClass := codexModelDiscoveryErrorClass(err)
		status := codexModelDiscoveryErrorStatus(err)
		return ChatResult{StatusCode: status, ContentType: "application/json", ErrorClass: errorClass, Latency: time.Since(start)}, err
	}
	body, effectiveServiceTier, err := marshalCodexResponsesRequest(req.Request, req.UpstreamModel, ids, modelMeta)
	if err != nil {
		return ChatResult{StatusCode: http.StatusBadRequest, ContentType: "application/json", Body: []byte("{}"), ErrorClass: "invalid_request", Latency: time.Since(start)}, err
	}
	endpoint, err := joinBasePath(req.Instance.BaseURL, "/responses")
	if err != nil {
		return ChatResult{ErrorClass: "provider_config_error", Latency: time.Since(start)}, err
	}
	ioID := a.recordUpstreamBody(req.Instance, req.Credential.ID, "responses", http.MethodPost, "upstream_input", 0, "application/json", body, "")
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(streamCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ChatResult{ErrorClass: "upstream_request_error", Latency: time.Since(start)}, err
	}
	a.addCodexRequestHeaders(ctx, httpReq, req.Credential.BearerToken, req.Credential.ChatGPTAccountID, req.Credential.ChatGPTAccountIsFedRAMP)
	addCodexResponsesHeaders(httpReq, ids)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := a.doStreamRequest(streamCtx, cancel, httpReq)
	if err != nil {
		errorClass := classifyTransportError(err)
		status := providerStatusForError(http.StatusBadGateway, errorClass)
		logProviderHTTP(ctx, a.Logger, statusLevel(status, errorClass), "provider_http",
			slog.String("endpoint", "responses"),
			slog.String("method", http.MethodPost),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", req.Credential.ID),
			slog.Int("status", status),
			slog.Int64("duration_ms", durationMS(start)),
			slog.String("error_class", errorClass),
		)
		return ChatResult{StatusCode: status, ContentType: "application/json", ErrorClass: errorClass, Latency: time.Since(start)}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		retryAfter := retryAfterFromHeader(resp.Header, time.Now())
		respBody, tooLarge, readErr := readLimitedUpstreamBody(resp.Body, MaxUpstreamChatBodyBytes)
		a.recordUpstreamBody(req.Instance, req.Credential.ID, "responses", http.MethodPost, "upstream_output", resp.StatusCode, resp.Header.Get("Content-Type"), respBody, ioID)
		if tooLarge {
			logProviderHTTP(ctx, a.Logger, slog.LevelError, "provider_http",
				slog.String("endpoint", "responses"),
				slog.String("method", http.MethodPost),
				slog.String("provider_instance", req.Instance.ID),
				slog.String("provider_type", req.Instance.Type),
				slog.Int64("credential_id", req.Credential.ID),
				slog.Int("status", http.StatusBadGateway),
				slog.Int64("duration_ms", durationMS(start)),
				slog.String("error_class", "upstream_body_too_large"),
			)
			return ChatResult{StatusCode: http.StatusBadGateway, UpstreamStatusCode: resp.StatusCode, ContentType: "application/json", ResolvedModel: resolvedCodexModel(req.UpstreamModel, codexHTTPHeaderModel(resp.Header)), ErrorClass: "upstream_body_too_large", Latency: time.Since(start), RetryAfter: retryAfter, BodyTruncated: true}, fmt.Errorf("codex responses body exceeded limit")
		}
		if readErr != nil {
			logProviderHTTP(ctx, a.Logger, slog.LevelError, "provider_http",
				slog.String("endpoint", "responses"),
				slog.String("method", http.MethodPost),
				slog.String("provider_instance", req.Instance.ID),
				slog.String("provider_type", req.Instance.Type),
				slog.Int64("credential_id", req.Credential.ID),
				slog.Int("status", resp.StatusCode),
				slog.Int64("duration_ms", durationMS(start)),
				slog.String("error_class", "upstream_network_error"),
			)
			return ChatResult{StatusCode: http.StatusBadGateway, UpstreamStatusCode: resp.StatusCode, ContentType: "application/json", ResolvedModel: resolvedCodexModel(req.UpstreamModel, codexHTTPHeaderModel(resp.Header)), ErrorClass: "upstream_network_error", Latency: time.Since(start), RetryAfter: retryAfter}, readErr
		}
		if resp.StatusCode == http.StatusUnauthorized {
			logProviderHTTP(ctx, a.Logger, statusLevel(resp.StatusCode, "upstream_auth_failed"), "provider_http",
				slog.String("endpoint", "responses"),
				slog.String("method", http.MethodPost),
				slog.String("provider_instance", req.Instance.ID),
				slog.String("provider_type", req.Instance.Type),
				slog.Int64("credential_id", req.Credential.ID),
				slog.Int("status", resp.StatusCode),
				slog.Int64("duration_ms", durationMS(start)),
				slog.Int("response_bytes", len(respBody)),
				slog.String("error_class", "upstream_auth_failed"),
			)
			return ChatResult{StatusCode: http.StatusUnauthorized, UpstreamStatusCode: resp.StatusCode, ContentType: "application/json", ResolvedModel: resolvedCodexModel(req.UpstreamModel, codexHTTPHeaderModel(resp.Header)), ErrorClass: "upstream_auth_failed", Latency: time.Since(start), RetryAfter: retryAfter}, fmt.Errorf("codex responses status %d", resp.StatusCode)
		}
		errorClass := "upstream_http_error"
		if resp.StatusCode == http.StatusTooManyRequests {
			errorClass = "rate_limit_exceeded"
		}
		attrs := []slog.Attr{
			slog.String("endpoint", "responses"),
			slog.String("method", http.MethodPost),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", req.Credential.ID),
			slog.Int("status", resp.StatusCode),
			slog.Int64("duration_ms", durationMS(start)),
			slog.Int("response_bytes", len(respBody)),
			slog.String("error_class", errorClass),
		}
		if resp.StatusCode == http.StatusBadRequest {
			attrs = append(attrs, codexResponsesRequestShapeAttrs(req.Request)...)
		}
		logProviderHTTP(ctx, a.Logger, statusLevel(resp.StatusCode, errorClass), "provider_http", attrs...)
		return ChatResult{StatusCode: http.StatusBadGateway, UpstreamStatusCode: resp.StatusCode, ContentType: "application/json", ResolvedModel: resolvedCodexModel(req.UpstreamModel, codexHTTPHeaderModel(resp.Header)), ErrorClass: errorClass, Latency: time.Since(start), RetryAfter: retryAfter}, fmt.Errorf("codex responses status %d", resp.StatusCode)
	}
	capture := upstreamStreamCapture{instance: req.Instance, credentialID: req.Credential.ID, endpoint: "responses", status: resp.StatusCode, id: ioID}
	allowNativeOutputItems := len(req.Request.CodexResponsesInput) > 0
	parsed, err := a.readCodexResponses(streamCtx, resp.Body, capture, allowNativeOutputItems)
	if parsed.ServedModel == "" {
		parsed.ServedModel = codexHTTPHeaderModel(resp.Header)
	}
	if err != nil {
		errorClass := parsed.ErrorClass
		if errorClass == "" {
			errorClass = classifyCodexReadError(streamCtx, err)
		}
		status := providerStatusForError(http.StatusBadGateway, errorClass)
		attrs := []slog.Attr{
			slog.String("endpoint", "responses"),
			slog.String("method", http.MethodPost),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", req.Credential.ID),
			slog.Int("status", status),
			slog.Int64("duration_ms", durationMS(start)),
			slog.String("error_class", errorClass),
		}
		attrs = append(attrs, codexReadErrorAttrs(err)...)
		if reason := codexSafeReadErrorReason(err); reason != "" {
			attrs = append(attrs, slog.String("error_reason", reason))
		}
		logProviderHTTP(ctx, a.Logger, statusLevel(status, errorClass), "provider_http", attrs...)
		return ChatResult{StatusCode: status, ContentType: "application/json", ResolvedModel: resolvedCodexModel(req.UpstreamModel, parsed.ServedModel), ErrorClass: errorClass, Latency: time.Since(start), HealthEventClasses: codexResultHealthEvents(req.UpstreamModel, parsed.ServedModel, parsed.HealthEventClasses)}, err
	}
	var out []byte
	if len(parsed.ToolCalls) > 0 {
		out, err = openai.MarshalChatCompletionToolCallsContentResponse(localChatCompletionID(), req.UpstreamModel, parsed.Text, parsed.ToolCalls, parsed.Usage)
	} else {
		out, err = openai.MarshalChatCompletionResponse(localChatCompletionID(), req.UpstreamModel, parsed.Text, parsed.Usage)
	}
	if err != nil {
		logProviderHTTP(ctx, a.Logger, slog.LevelError, "provider_http",
			slog.String("endpoint", "responses"),
			slog.String("method", http.MethodPost),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", req.Credential.ID),
			slog.Int("status", http.StatusBadGateway),
			slog.Int64("duration_ms", durationMS(start)),
			slog.String("error_class", "upstream_invalid_response"),
			slog.String("error_reason", "codex response marshal failed"),
		)
		return ChatResult{StatusCode: http.StatusBadGateway, ContentType: "application/json", ResolvedModel: resolvedCodexModel(req.UpstreamModel, parsed.ServedModel), ErrorClass: "upstream_invalid_response", Latency: time.Since(start)}, err
	}
	logProviderHTTP(ctx, a.Logger, slog.LevelInfo, "provider_http",
		slog.String("endpoint", "responses"),
		slog.String("method", http.MethodPost),
		slog.String("provider_instance", req.Instance.ID),
		slog.String("provider_type", req.Instance.Type),
		slog.Int64("credential_id", req.Credential.ID),
		slog.Int("status", http.StatusOK),
		slog.Int64("duration_ms", durationMS(start)),
	)
	outputItems := []openai.ResponsesOutputItem(nil)
	if len(req.Request.CodexResponsesInput) > 0 {
		outputItems = parsed.OutputItems
	}
	return ChatResult{
		StatusCode:           http.StatusOK,
		ContentType:          "application/json",
		Body:                 out,
		Usage:                parsed.Usage,
		ResolvedModel:        resolvedCodexModel(req.UpstreamModel, parsed.ServedModel),
		ResponsesOutputItems: outputItems,
		Latency:              time.Since(start),
		EffectiveServiceTier: effectiveServiceTier,
		HealthEventClasses:   codexResultHealthEvents(req.UpstreamModel, parsed.ServedModel, parsed.HealthEventClasses),
	}, nil
}

func resolvedCodexModel(requestedModel, servedModel string) string {
	servedModel = strings.TrimSpace(servedModel)
	if validProviderModelID(servedModel) {
		return servedModel
	}
	return requestedModel
}

func codexResultHealthEvents(requestedModel, servedModel string, classes []string) []string {
	out := make([]string, 0, len(classes)+1)
	seen := map[string]bool{}
	for _, class := range classes {
		class = strings.TrimSpace(class)
		if class == "" || seen[class] {
			continue
		}
		seen[class] = true
		out = append(out, class)
	}
	if codexServedModelRerouted(requestedModel, servedModel) && !seen["codex_mitigated_rerouted"] {
		out = append(out, "codex_mitigated_rerouted")
	}
	sort.Strings(out)
	return out
}

func codexServedModelRerouted(requestedModel, servedModel string) bool {
	requestedModel = strings.TrimSpace(requestedModel)
	servedModel = strings.TrimSpace(servedModel)
	return requestedModel != "" && servedModel != "" && requestedModel != servedModel
}

func codexToolCall(callID, name, arguments string) (map[string]any, error) {
	if callID == "" || name == "" {
		return nil, fmt.Errorf("invalid codex function_call")
	}
	return map[string]any{
		"id":   callID,
		"type": "function",
		"function": map[string]any{
			"name":      name,
			"arguments": arguments,
		},
	}, nil
}

func classifyCodexReadError(ctx context.Context, err error) string {
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return "client_disconnected"
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "upstream_timeout"
	}
	if errors.Is(err, bufio.ErrBufferFull) {
		return "upstream_invalid_response"
	}
	return classifyTransportError(err)
}

func localChatCompletionID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "chatcmpl_000000000000000000000000"
	}
	return "chatcmpl_" + hex.EncodeToString(b[:])
}

type codexRequestIDs struct {
	SessionID      string
	ThreadID       string
	WindowID       string
	InstallationID string
}

func newCodexRequestIDs() codexRequestIDs {
	threadID := localCodexUUID()
	return codexRequestIDs{
		SessionID:      localCodexUUID(),
		ThreadID:       threadID,
		WindowID:       threadID + ":0",
		InstallationID: localCodexUUID(),
	}
}

func localCodexUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func addCodexResponsesHeaders(req *http.Request, ids codexRequestIDs) {
	// Mirrors OpenAI Codex rust-v0.135.0:
	// codex-rs/codex-api/src/endpoint/responses.rs adds session-id,
	// thread-id, and x-client-request-id; core/src/client.rs adds x-codex-window-id.
	req.Header.Set("session-id", ids.SessionID)
	req.Header.Set("thread-id", ids.ThreadID)
	req.Header.Set("x-client-request-id", ids.ThreadID)
	req.Header.Set("x-codex-window-id", ids.WindowID)
}

func codexToolCallKey(itemID, callID string) string {
	if itemID != "" {
		return itemID
	}
	return callID
}

func codexToolEvent(typ string) bool {
	typ = strings.ToLower(typ)
	return strings.Contains(typ, "tool") ||
		strings.Contains(typ, "code_interpreter") ||
		strings.Contains(typ, "file_search") ||
		strings.Contains(typ, "web_search") ||
		strings.Contains(typ, "image_generation") ||
		strings.Contains(typ, "computer_call") ||
		strings.Contains(typ, "mcp_call") ||
		strings.Contains(typ, "local_shell")
}

func unsupportedCodexToolEvent(typ string) bool {
	typ = strings.ToLower(typ)
	if typ == "response.custom_tool_call_input.delta" || typ == "response.custom_tool_call_input.done" {
		return false
	}
	if strings.HasPrefix(typ, "response.web_search_call.") {
		return false
	}
	return codexToolEvent(typ)
}

func unsupportedCodexOutputItem(typ string) bool {
	typ = strings.ToLower(typ)
	if typ == "custom_tool_call" || typ == "tool_search_call" || typ == "web_search_call" {
		return false
	}
	return codexToolEvent(typ)
}

type codexUsagePayload struct {
	InputTokens        *int `json:"input_tokens"`
	OutputTokens       *int `json:"output_tokens"`
	TotalTokens        *int `json:"total_tokens"`
	InputTokensDetails *struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"input_tokens_details"`
	OutputTokensDetails *struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
}

type codexResponseMetadataPayload struct {
	OpenAIVerificationRecommendation []string `json:"openai_verification_recommendation"`
}

type codexResponsePayload struct {
	ID       string                        `json:"id"`
	EndTurn  *bool                         `json:"end_turn"`
	Model    string                        `json:"model"`
	Headers  map[string]any                `json:"headers"`
	Error    *codexResponseError           `json:"error"`
	Metadata *codexResponseMetadataPayload `json:"metadata"`
	Usage    *codexUsagePayload            `json:"usage"`
}

func codexServerModelFromHeaders(response *codexResponsePayload, eventHeaders map[string]any) string {
	if response != nil {
		if model := codexHeaderModel(response.Headers); model != "" {
			return model
		}
	}
	return codexHeaderModel(eventHeaders)
}

func codexHeaderModel(headers map[string]any) string {
	for name, value := range headers {
		name = strings.TrimSpace(name)
		if !strings.EqualFold(name, "openai-model") && !strings.EqualFold(name, "x-openai-model") {
			continue
		}
		if model := codexHeaderValueModel(value); model != "" {
			return model
		}
	}
	return ""
}

func codexHeaderValueModel(value any) string {
	switch v := value.(type) {
	case string:
		return openai.SafeResolvedModel(v)
	case []any:
		for _, item := range v {
			if model := codexHeaderValueModel(item); model != "" {
				return model
			}
		}
	default:
		return ""
	}
	return ""
}

func codexHTTPHeaderModel(headers http.Header) string {
	for _, name := range []string{"OpenAI-Model", "X-OpenAI-Model"} {
		for _, value := range headers.Values(name) {
			if model := openai.SafeResolvedModel(value); model != "" {
				return model
			}
		}
	}
	return ""
}

func codexUsageFromResponse(usage *codexUsagePayload) (openai.Usage, error) {
	if usage == nil || usage.InputTokens == nil || usage.OutputTokens == nil || usage.TotalTokens == nil {
		return openai.Usage{}, fmt.Errorf("invalid codex usage")
	}
	out := openai.Usage{
		PromptTokens:     *usage.InputTokens,
		CompletionTokens: *usage.OutputTokens,
		TotalTokens:      *usage.TotalTokens,
	}
	if usage.InputTokensDetails != nil {
		out.CachedTokens = usage.InputTokensDetails.CachedTokens
	}
	if usage.OutputTokensDetails != nil {
		out.ReasoningTokens = usage.OutputTokensDetails.ReasoningTokens
	}
	return out, nil
}

func (a HTTPChatAdapter) maxCodexAggregateBytes() int {
	if a.MaxCodexAggregateBytes > 0 {
		return a.MaxCodexAggregateBytes
	}
	return MaxCodexAggregateAssistantBytes
}
