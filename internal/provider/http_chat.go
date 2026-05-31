package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"ilonasin/internal/openai"
)

const MaxUpstreamChatBodyBytes int64 = 16 << 20
const MaxUpstreamModelsBodyBytes int64 = 16 << 20
const DefaultMaxStreamLineBytes = 1 << 20
const DefaultMaxStreamEventBytes = 1 << 20
const DefaultMaxStreamEvents = 1_000_000
const DefaultStreamIdleTimeout = 120 * time.Second
const DefaultStreamHeaderTimeout = 30 * time.Second
const maxRetryAfter = 365 * 24 * time.Hour

func readLimitedUpstreamBody(body io.Reader, limit int64) ([]byte, bool, error) {
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if int64(len(data)) > limit {
		return nil, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	return data, false, nil
}

type HTTPChatAdapter struct {
	Client                 *http.Client
	StreamIdleTimeout      time.Duration
	StreamHeaderTimeout    time.Duration
	MaxStreamLineBytes     int
	MaxStreamEventBytes    int
	MaxStreamEvents        int
	MaxCodexAggregateBytes int
	ModelTimeout           time.Duration
}

func (a HTTPChatAdapter) ListModels(ctx context.Context, req ModelRequest) (ModelResult, error) {
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
	httpReq.Header.Set("Authorization", "Bearer "+req.Credential.BearerToken)
	httpReq.Header.Set("Accept", "application/json")
	resp, err := a.Client.Do(httpReq)
	if err != nil {
		return ModelResult{ErrorClass: classifyTransportError(err)}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errorClass := "upstream_http_error"
		if resp.StatusCode == http.StatusUnauthorized {
			errorClass = "upstream_auth_failed"
		}
		return ModelResult{ErrorClass: errorClass, StatusCode: resp.StatusCode, RetryAfter: retryAfterFromHeader(resp.Header, time.Now())}, fmt.Errorf("upstream models status %d", resp.StatusCode)
	}
	if resp.ContentLength > MaxUpstreamModelsBodyBytes {
		return ModelResult{ErrorClass: "upstream_body_too_large", StatusCode: resp.StatusCode}, fmt.Errorf("upstream models body exceeded limit")
	}
	body, tooLarge, readErr := readLimitedUpstreamBody(resp.Body, MaxUpstreamModelsBodyBytes)
	if tooLarge {
		return ModelResult{ErrorClass: "upstream_body_too_large", StatusCode: resp.StatusCode}, fmt.Errorf("upstream models body exceeded limit")
	}
	if readErr != nil {
		return ModelResult{ErrorClass: "upstream_network_error", StatusCode: resp.StatusCode}, readErr
	}
	models, err := normalizeModels(req.Instance, body)
	if err != nil {
		return ModelResult{ErrorClass: "upstream_invalid_response", StatusCode: resp.StatusCode}, err
	}
	return ModelResult{Models: models, StatusCode: resp.StatusCode}, nil
}

func NewHTTPChatAdapter(client *http.Client) HTTPChatAdapter {
	if client == nil {
		client = &http.Client{Timeout: 90 * time.Second}
	} else if client.Timeout == 0 {
		clone := *client
		clone.Timeout = 90 * time.Second
		client = &clone
	}
	return HTTPChatAdapter{Client: client}
}

func (a HTTPChatAdapter) ValidateChatRequest(instance Instance, req openai.ChatCompletionRequest) error {
	commonUnsupported := []string{}
	openRouterOnlyFields := []string{"top_k", "min_p", "top_a", "repetition_penalty", "seed", "logit_bias", "service_tier", "session_id", "metadata"}
	switch instance.Type {
	case "deepseek", "openrouter":
		if err := rejectPresentFields(req, commonUnsupported...); err != nil {
			return err
		}
		if instance.Type == "deepseek" {
			if err := rejectPresentFields(req, "presence_penalty", "frequency_penalty", "parallel_tool_calls", "prediction", "user"); err != nil {
				return err
			}
			if err := rejectPresentFields(req, openRouterOnlyFields...); err != nil {
				return err
			}
		}
		if err := validateChatResponseFormat(instance.Type, req); err != nil {
			return err
		}
		return validateProviderOptions(instance.Type, req)
	case "codex":
		unsupported := append(commonUnsupported, "tools", "tool_choice", "parallel_tool_calls", "prediction", "user", "logprobs", "top_logprobs", "provider_options", "max_tokens", "max_completion_tokens", "temperature", "top_p", "presence_penalty", "frequency_penalty", "stop", "response_format")
		unsupported = append(unsupported, openRouterOnlyFields...)
		if err := rejectPresentFields(req, unsupported...); err != nil {
			return err
		}
		if hasToolMessages(req) {
			return fmt.Errorf("tool messages are not supported")
		}
		return nil
	default:
		return fmt.Errorf("provider type %q does not support chat validation", instance.Type)
	}
}

func hasToolMessages(req openai.ChatCompletionRequest) bool {
	for _, msg := range req.Messages {
		if msg.Role == "tool" || len(msg.ToolCalls) > 0 || msg.ToolCallID != "" {
			return true
		}
	}
	return false
}

func rejectPresentFields(req openai.ChatCompletionRequest, fields ...string) error {
	for _, field := range fields {
		if req.HasField(field) {
			return fmt.Errorf("%s is not supported", field)
		}
	}
	return nil
}

func validateChatResponseFormat(providerType string, req openai.ChatCompletionRequest) error {
	if !req.HasField("response_format") {
		return nil
	}
	if req.ResponseFormat == nil {
		return errors.New("response_format must be an object")
	}
	typ, _ := req.ResponseFormat["type"].(string)
	switch typ {
	case "text", "json_object":
		if len(req.ResponseFormat) != 1 {
			return errors.New("response_format only supports the type field")
		}
		return nil
	case "json_schema":
		if providerType != "openrouter" {
			return errors.New("response_format.type is unsupported")
		}
		return validateOpenRouterJSONSchemaResponseFormat(req.ResponseFormat)
	default:
		return errors.New("response_format.type is unsupported")
	}
}

func validateOpenRouterJSONSchemaResponseFormat(format map[string]any) error {
	if len(format) != 2 {
		return errors.New("response_format only supports type and json_schema")
	}
	rawSchema, ok := format["json_schema"]
	if !ok {
		return errors.New("response_format.json_schema is required")
	}
	schema, ok := rawSchema.(map[string]any)
	if !ok || schema == nil {
		return errors.New("response_format.json_schema must be an object")
	}
	if len(schema) == 0 {
		return errors.New("response_format.json_schema must not be empty")
	}
	name, ok := schema["name"].(string)
	if !ok {
		return errors.New("response_format.json_schema.name must be a string")
	}
	if name == "" || len(name) > 64 || !isOpenRouterJSONSchemaName(name) {
		return errors.New("response_format.json_schema.name is invalid")
	}
	rawBody, ok := schema["schema"]
	if !ok {
		return errors.New("response_format.json_schema.schema is required")
	}
	body, ok := rawBody.(map[string]any)
	if !ok || body == nil {
		return errors.New("response_format.json_schema.schema must be an object")
	}
	_ = body
	for key, value := range schema {
		switch key {
		case "name", "schema":
		case "strict":
			if _, ok := value.(bool); !ok {
				return errors.New("response_format.json_schema.strict must be a boolean")
			}
		case "description":
			if _, ok := value.(string); !ok {
				return errors.New("response_format.json_schema.description must be a string")
			}
		default:
			return errors.New("response_format.json_schema contains an unsupported field")
		}
	}
	return nil
}

func isOpenRouterJSONSchemaName(value string) bool {
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-':
		default:
			return false
		}
	}
	return true
}

func validateProviderOptions(providerType string, req openai.ChatCompletionRequest) error {
	if !req.HasField("provider_options") {
		return nil
	}
	if req.ReasoningOptions == nil {
		return errors.New("provider_options must be an object")
	}
	if len(req.ReasoningOptions) != 1 {
		return fmt.Errorf("provider_options must contain only %s", providerType)
	}
	raw, ok := req.ReasoningOptions[providerType]
	if !ok {
		return fmt.Errorf("provider_options must contain only %s", providerType)
	}
	switch providerType {
	case "deepseek":
		return validateDeepSeekOptions(raw)
	case "openrouter":
		return validateOpenRouterOptions(raw)
	default:
		return fmt.Errorf("provider_options is not supported for %s", providerType)
	}
}

func validateDeepSeekOptions(raw any) error {
	opts, ok := raw.(map[string]any)
	if !ok || opts == nil {
		return errors.New("provider_options.deepseek must be an object")
	}
	if len(opts) == 0 {
		return errors.New("provider_options.deepseek must not be empty")
	}
	for key, value := range opts {
		switch key {
		case "thinking":
			thinking, ok := value.(map[string]any)
			if !ok || thinking == nil {
				return errors.New("provider_options.deepseek.thinking must be an object")
			}
			if len(thinking) != 1 {
				return errors.New("provider_options.deepseek.thinking only supports type")
			}
			typ, ok := thinking["type"].(string)
			if !ok {
				return errors.New("provider_options.deepseek.thinking.type must be a string")
			}
			if typ != "enabled" && typ != "disabled" {
				return errors.New("provider_options.deepseek.thinking.type is unsupported")
			}
		case "reasoning_effort":
			effort, ok := value.(string)
			if !ok {
				return errors.New("provider_options.deepseek.reasoning_effort must be a string")
			}
			if effort != "high" && effort != "max" {
				return errors.New("provider_options.deepseek.reasoning_effort is unsupported")
			}
		case "user_id":
			userID, ok := value.(string)
			if !ok {
				return errors.New("provider_options.deepseek.user_id must be a string")
			}
			if !isDeepSeekUserID(userID) {
				return errors.New("provider_options.deepseek.user_id is invalid")
			}
		default:
			return errors.New("provider_options.deepseek contains an unsupported field")
		}
	}
	return nil
}

func isDeepSeekUserID(value string) bool {
	if value == "" || len(value) > 512 {
		return false
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-':
		default:
			return false
		}
	}
	return true
}

func marshalChatCompletionsRequest(providerType string, req openai.ChatCompletionRequest, upstreamModel string) ([]byte, error) {
	body, err := openai.MarshalUpstreamChatRequest(req, upstreamModel)
	if err != nil {
		return nil, err
	}
	if !req.HasField("provider_options") && req.MaxCompletionTokens == nil {
		return body, nil
	}
	var out map[string]any
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return nil, fmt.Errorf("chat request body must contain a single JSON object")
	}
	if req.MaxCompletionTokens != nil {
		switch providerType {
		case "deepseek":
			out["max_tokens"] = *req.MaxCompletionTokens
		case "openrouter":
			out["max_completion_tokens"] = *req.MaxCompletionTokens
		default:
			return nil, fmt.Errorf("max_completion_tokens is not supported for %s", providerType)
		}
	}
	if req.HasField("provider_options") {
		if err := validateProviderOptions(providerType, req); err != nil {
			return nil, err
		}
		switch providerType {
		case "deepseek":
			opts := req.ReasoningOptions["deepseek"].(map[string]any)
			if thinking, ok := opts["thinking"]; ok {
				out["thinking"] = thinking
			}
			if effort, ok := opts["reasoning_effort"]; ok {
				out["reasoning_effort"] = effort
			}
			if userID, ok := opts["user_id"]; ok {
				out["user_id"] = userID
			}
		case "openrouter":
			opts := req.ReasoningOptions["openrouter"].(map[string]any)
			if reasoning, ok := opts["reasoning"]; ok {
				out["reasoning"] = reasoning
			}
			if models, ok := opts["models"]; ok {
				out["models"] = models
			}
			if cacheControl, ok := opts["cache_control"]; ok {
				out["cache_control"] = cacheControl
			}
			if provider, ok := opts["provider"]; ok {
				out["provider"] = provider
			}
		default:
			return nil, fmt.Errorf("provider_options is not supported for %s", providerType)
		}
	}
	return json.Marshal(out)
}

func openRouterCostMicrounitsFromChatCompletion(body []byte) int64 {
	var payload struct {
		Usage json.RawMessage `json:"usage"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0
	}
	return openRouterCostMicrounitsFromUsage(payload.Usage)
}

func openRouterCostMicrounitsFromStreamChunk(body []byte) int64 {
	var payload struct {
		Usage json.RawMessage `json:"usage"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0
	}
	return openRouterCostMicrounitsFromUsage(payload.Usage)
}

func openRouterCostMicrounitsFromUsage(rawUsage json.RawMessage) int64 {
	rawUsage = bytes.TrimSpace(rawUsage)
	if len(rawUsage) == 0 || bytes.Equal(rawUsage, []byte("null")) {
		return 0
	}
	var usage map[string]json.RawMessage
	if err := json.Unmarshal(rawUsage, &usage); err != nil {
		return 0
	}
	rawCost, ok := usage["cost"]
	if !ok {
		return 0
	}
	return openRouterCostMicrounitsFromRawCost(rawCost)
}

func openRouterCostMicrounitsFromRawCost(rawCost json.RawMessage) int64 {
	rawCost = bytes.TrimSpace(rawCost)
	if len(rawCost) == 0 || bytes.Equal(rawCost, []byte("null")) {
		return 0
	}
	if (rawCost[0] < '0' || rawCost[0] > '9') && rawCost[0] != '-' {
		return 0
	}
	dec := json.NewDecoder(bytes.NewReader(rawCost))
	dec.UseNumber()
	var cost json.Number
	if err := dec.Decode(&cost); err != nil {
		return 0
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return 0
	}
	return openRouterCreditMicrounits(cost.String())
}

func openRouterCreditMicrounits(value string) int64 {
	if value == "" || len(value) > 128 || value[0] == '-' {
		return 0
	}
	mantissa, exponent, ok := strings.Cut(value, "e")
	if !ok {
		mantissa, exponent, ok = strings.Cut(value, "E")
	}
	exp := 0
	if ok {
		if exponent == "" || len(exponent) > 4 {
			return 0
		}
		parsed, err := strconv.Atoi(exponent)
		if err != nil {
			return 0
		}
		exp = parsed
	}
	digits, fractionDigits, ok := decimalDigits(mantissa)
	if !ok {
		return 0
	}
	digits = strings.TrimLeft(digits, "0")
	if digits == "" {
		return 0
	}
	decimalExp := exp - fractionDigits + 6
	if decimalExp > 19 || decimalExp < -128 {
		return 0
	}
	valueInt, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		return 0
	}
	if decimalExp >= 0 {
		valueInt.Mul(valueInt, pow10(decimalExp))
		if valueInt.Cmp(new(big.Int).SetInt64(math.MaxInt64)) > 0 {
			return 0
		}
		return valueInt.Int64()
	}
	divisor := pow10(-decimalExp)
	quotient, remainder := new(big.Int).QuoRem(valueInt, divisor, new(big.Int))
	if new(big.Int).Mul(remainder, big.NewInt(2)).Cmp(divisor) >= 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	if quotient.Cmp(new(big.Int).SetInt64(math.MaxInt64)) > 0 {
		return 0
	}
	return quotient.Int64()
}

func decimalDigits(value string) (string, int, bool) {
	if value == "" {
		return "", 0, false
	}
	var digits strings.Builder
	fractionDigits := 0
	seenDot := false
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
			digits.WriteRune(r)
			if seenDot {
				fractionDigits++
			}
		case r == '.':
			if seenDot {
				return "", 0, false
			}
			seenDot = true
		default:
			return "", 0, false
		}
	}
	if digits.Len() == 0 {
		return "", 0, false
	}
	return digits.String(), fractionDigits, true
}

func pow10(exp int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exp)), nil)
}

func (a HTTPChatAdapter) CompleteChat(ctx context.Context, req ChatRequest) (ChatResult, error) {
	start := time.Now()
	if req.Instance.Type == "codex" {
		return a.completeCodexChat(ctx, req, start)
	}
	body, err := marshalChatCompletionsRequest(req.Instance.Type, req.Request, req.UpstreamModel)
	if err != nil {
		return ChatResult{ErrorClass: "invalid_request"}, err
	}
	endpoint, err := chatCompletionsURL(req.Instance.BaseURL)
	if err != nil {
		return ChatResult{ErrorClass: "provider_config_error"}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ChatResult{ErrorClass: "upstream_request_error"}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+req.Credential.BearerToken)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := a.Client.Do(httpReq)
	if err != nil {
		return ChatResult{ErrorClass: classifyTransportError(err), Latency: time.Since(start)}, err
	}
	defer resp.Body.Close()

	if resp.ContentLength > MaxUpstreamChatBodyBytes {
		latency := time.Since(start)
		return ChatResult{
			StatusCode:    http.StatusBadGateway,
			ContentType:   "application/json",
			ErrorClass:    "upstream_body_too_large",
			Latency:       latency,
			RetryAfter:    retryAfterFromHeader(resp.Header, time.Now()),
			BodyTruncated: true,
		}, fmt.Errorf("upstream response body exceeded limit")
	}
	respBody, tooLarge, readErr := readLimitedUpstreamBody(resp.Body, MaxUpstreamChatBodyBytes)
	latency := time.Since(start)
	if tooLarge {
		return ChatResult{
			StatusCode:    http.StatusBadGateway,
			ContentType:   "application/json",
			ErrorClass:    "upstream_body_too_large",
			Latency:       latency,
			RetryAfter:    retryAfterFromHeader(resp.Header, time.Now()),
			BodyTruncated: true,
		}, fmt.Errorf("upstream response body exceeded limit")
	}
	if readErr != nil {
		return ChatResult{
			StatusCode:  http.StatusBadGateway,
			ContentType: "application/json",
			ErrorClass:  "upstream_network_error",
			Latency:     latency,
			RetryAfter:  retryAfterFromHeader(resp.Header, time.Now()),
		}, readErr
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}

	result := ChatResult{
		StatusCode:  resp.StatusCode,
		ContentType: contentType,
		Body:        respBody,
		Latency:     latency,
		RetryAfter:  retryAfterFromHeader(resp.Header, time.Now()),
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.ErrorClass = "upstream_http_error"
		return result, nil
	}
	metadata, err := openai.ExtractChatCompletionMetadata(respBody)
	if err != nil {
		result.StatusCode = http.StatusBadGateway
		result.ContentType = "application/json"
		result.Body = nil
		result.ErrorClass = "upstream_invalid_response"
		result.InvalidBody = true
		return result, err
	}
	result.Usage = metadata.Usage
	if req.Instance.Type == "openrouter" {
		result.Usage.CostMicrounits = openRouterCostMicrounitsFromChatCompletion(respBody)
	}
	result.ResolvedModel = metadata.ResolvedModel
	return result, nil
}

func (a HTTPChatAdapter) StreamChat(ctx context.Context, req ChatRequest, sink ChatStreamSink) (ChatStreamSummary, error) {
	start := time.Now()
	if req.Instance.Type == "codex" {
		return a.streamCodexChat(ctx, req, sink, start)
	}
	body, err := marshalChatCompletionsRequest(req.Instance.Type, req.Request, req.UpstreamModel)
	if err != nil {
		return ChatStreamSummary{ErrorClass: "invalid_request", CompletionStatus: "upstream_invalid"}, err
	}
	endpoint, err := chatCompletionsURL(req.Instance.BaseURL)
	if err != nil {
		return ChatStreamSummary{ErrorClass: "provider_config_error", CompletionStatus: "upstream_invalid"}, err
	}
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(streamCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ChatStreamSummary{ErrorClass: "upstream_request_error", CompletionStatus: "upstream_invalid"}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+req.Credential.BearerToken)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := a.doStreamRequest(streamCtx, cancel, httpReq)
	if err != nil {
		errorClass := classifyTransportError(err)
		return ChatStreamSummary{
			StatusCode:       http.StatusBadGateway,
			ErrorClass:       errorClass,
			CompletionStatus: streamStatusForError(errorClass),
			PreStreamError:   true,
		}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ChatStreamSummary{
			StatusCode:       resp.StatusCode,
			ErrorClass:       "upstream_http_error",
			CompletionStatus: "upstream_error",
			RetryAfter:       retryAfterFromHeader(resp.Header, time.Now()),
			PreStreamError:   true,
		}, fmt.Errorf("upstream stream status %d", resp.StatusCode)
	}

	summary := ChatStreamSummary{
		StatusCode:       http.StatusOK,
		CompletionStatus: "completed",
	}
	if err := a.readStream(streamCtx, resp.Body, sink, &summary, start, req.Instance.Type); err != nil {
		if summary.ErrorClass == "" {
			summary.ErrorClass = classifyStreamReadError(streamCtx, err)
		}
		if summary.CompletionStatus == "" || summary.CompletionStatus == "completed" {
			summary.CompletionStatus = streamStatusForError(summary.ErrorClass)
		}
		if summary.StatusCode == 0 || (!summary.Started && summary.StatusCode < 400) {
			summary.StatusCode = http.StatusBadGateway
		}
		if !summary.Started {
			summary.PreStreamError = true
		}
		return summary, err
	}
	return summary, nil
}

func chatCompletionsURL(base string) (string, error) {
	return joinBasePath(base, "/chat/completions")
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
	q.Set("client_version", "ilonasin")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func joinBasePath(base, suffix string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	u.Path = strings.TrimRight(u.Path, "/") + suffix
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

func normalizeModels(instance Instance, body []byte) ([]ModelMetadata, error) {
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

func jsonUnmarshal(body []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(v); err != nil {
		return err
	}
	var extra struct{}
	if err := dec.Decode(&extra); err != io.EOF {
		return errors.New("upstream response contains trailing data")
	}
	return nil
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

func openRouterCapabilityFlags(item map[string]any) string {
	flags := map[string]bool{"chat": true}
	params, _ := item["supported_parameters"].([]any)
	for _, raw := range params {
		param, ok := raw.(string)
		if !ok {
			continue
		}
		switch param {
		case "temperature", "top_p", "frequency_penalty", "presence_penalty", "stop":
			flags["sampling"] = true
		case "top_k", "min_p", "top_a", "repetition_penalty", "seed":
			flags["advanced_sampling"] = true
		case "response_format":
			flags["json_object"] = true
		case "tools", "tool_choice":
			flags["tools"] = true
		case "parallel_tool_calls":
			flags["parallel_tool_calls"] = true
		case "prediction":
			flags["prediction"] = true
		case "logprobs", "top_logprobs":
			flags["logprobs"] = true
		case "logit_bias":
			flags["logit_bias"] = true
		case "reasoning":
			flags["reasoning"] = true
		case "stream":
			flags["stream"] = true
		case "user":
			flags["user"] = true
		case "service_tier":
			flags["service_tier"] = true
		case "session_id":
			flags["session_id"] = true
		case "metadata":
			flags["metadata"] = true
		case "models":
			flags["model_fallbacks"] = true
		case "cache_control":
			flags["cache_control"] = true
		}
	}
	out := make([]string, 0, len(flags))
	for flag := range flags {
		out = append(out, flag)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

func classifyTransportError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "upstream_timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "client_disconnected"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "upstream_timeout"
	}
	return "upstream_network_error"
}

func retryAfterFromHeader(header http.Header, now time.Time) *time.Time {
	values := header.Values("Retry-After")
	if len(values) != 1 {
		return nil
	}
	value := strings.TrimSpace(values[0])
	if value == "" {
		return nil
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		if seconds < 0 || seconds > int64(maxRetryAfter/time.Second) {
			return nil
		}
		delay := time.Duration(seconds) * time.Second
		out := now.UTC().Add(delay)
		return &out
	}
	parsed, err := http.ParseTime(value)
	if err != nil {
		return nil
	}
	parsed = parsed.UTC()
	now = now.UTC()
	if !parsed.After(now) || parsed.Sub(now) > maxRetryAfter {
		return nil
	}
	return &parsed
}

func (a HTTPChatAdapter) streamingClient() *http.Client {
	client := a.Client
	if client == nil {
		client = &http.Client{}
	}
	clone := *client
	clone.Timeout = 0
	return &clone
}

func (a HTTPChatAdapter) streamIdleTimeout() time.Duration {
	if a.StreamIdleTimeout > 0 {
		return a.StreamIdleTimeout
	}
	return DefaultStreamIdleTimeout
}

func (a HTTPChatAdapter) streamHeaderTimeout() time.Duration {
	if a.StreamHeaderTimeout > 0 {
		return a.StreamHeaderTimeout
	}
	return DefaultStreamHeaderTimeout
}

func (a HTTPChatAdapter) maxStreamLineBytes() int {
	if a.MaxStreamLineBytes > 0 {
		return a.MaxStreamLineBytes
	}
	return DefaultMaxStreamLineBytes
}

func (a HTTPChatAdapter) maxStreamEventBytes() int {
	if a.MaxStreamEventBytes > 0 {
		return a.MaxStreamEventBytes
	}
	return DefaultMaxStreamEventBytes
}

func (a HTTPChatAdapter) maxStreamEvents() int {
	if a.MaxStreamEvents > 0 {
		return a.MaxStreamEvents
	}
	return DefaultMaxStreamEvents
}

func (a HTTPChatAdapter) modelTimeout() time.Duration {
	if a.ModelTimeout > 0 {
		return a.ModelTimeout
	}
	return 30 * time.Second
}

func (a HTTPChatAdapter) doStreamRequest(ctx context.Context, cancel context.CancelFunc, req *http.Request) (*http.Response, error) {
	type result struct {
		resp *http.Response
		err  error
	}
	req = req.WithContext(ctx)
	done := make(chan result, 1)
	go func() {
		resp, err := a.streamingClient().Do(req)
		done <- result{resp: resp, err: err}
	}()
	timer := time.NewTimer(a.streamHeaderTimeout())
	defer timer.Stop()
	select {
	case res := <-done:
		return res.resp, res.err
	case <-timer.C:
		cancel()
		return nil, context.DeadlineExceeded
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (a HTTPChatAdapter) readStream(ctx context.Context, body io.ReadCloser, sink ChatStreamSink, summary *ChatStreamSummary, start time.Time, providerType string) error {
	reader := bufio.NewReaderSize(body, a.maxStreamLineBytes()+1)
	var parts [][]byte
	eventBytes := 0
	for {
		line, err := a.readStreamLine(ctx, body, reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				if len(parts) > 0 {
					if err := a.handleStreamEvent(ctx, bytes.Join(parts, []byte("\n")), sink, summary, start, providerType); err != nil {
						return err
					}
					if summary.Done {
						return nil
					}
				}
				if summary.Done {
					return nil
				}
				summary.ErrorClass = "upstream_stream_invalid"
				summary.CompletionStatus = "upstream_invalid"
				return io.ErrUnexpectedEOF
			}
			return err
		}
		line = bytes.TrimRight(line, "\r\n")
		if len(line) == 0 {
			if len(parts) == 0 {
				continue
			}
			if err := a.handleStreamEvent(ctx, bytes.Join(parts, []byte("\n")), sink, summary, start, providerType); err != nil {
				return err
			}
			parts = nil
			eventBytes = 0
			if summary.Done {
				return nil
			}
			continue
		}
		if line[0] == ':' {
			continue
		}
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		data := bytes.TrimPrefix(line, []byte("data:"))
		if len(data) > 0 && data[0] == ' ' {
			data = data[1:]
		}
		eventBytes += len(data) + 1
		if eventBytes > a.maxStreamEventBytes() {
			summary.ErrorClass = "upstream_stream_too_large"
			summary.CompletionStatus = "too_large"
			return bufio.ErrBufferFull
		}
		part := make([]byte, len(data))
		copy(part, data)
		parts = append(parts, part)
	}
}

func (a HTTPChatAdapter) readStreamLine(ctx context.Context, body io.Closer, reader *bufio.Reader) ([]byte, error) {
	type result struct {
		b   byte
		err error
	}
	var line []byte
	for {
		done := make(chan result, 1)
		go func() {
			b, err := reader.ReadByte()
			done <- result{b: b, err: err}
		}()
		timer := time.NewTimer(a.streamIdleTimeout())
		select {
		case res := <-done:
			timer.Stop()
			if res.err != nil {
				return line, res.err
			}
			line = append(line, res.b)
			if len(line) > a.maxStreamLineBytes() {
				return nil, bufio.ErrBufferFull
			}
			if res.b == '\n' {
				return line, nil
			}
		case <-timer.C:
			_ = body.Close()
			return nil, context.DeadlineExceeded
		case <-ctx.Done():
			timer.Stop()
			_ = body.Close()
			return nil, ctx.Err()
		}
	}
}

func (a HTTPChatAdapter) handleStreamEvent(ctx context.Context, data []byte, sink ChatStreamSink, summary *ChatStreamSummary, start time.Time, providerType string) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("[DONE]")) {
		if err := sink.WriteDone(ctx); err != nil {
			summary.ErrorClass = "client_disconnected"
			summary.CompletionStatus = "client_disconnected"
			return err
		}
		summary.Done = true
		return nil
	}
	if openai.IsStreamError(data) {
		summary.ErrorClass = "upstream_stream_error"
		summary.CompletionStatus = "upstream_error"
		if !summary.Started {
			summary.StatusCode = http.StatusBadGateway
			summary.PreStreamError = true
			return fmt.Errorf("upstream stream error")
		}
		if err := sink.WriteEvent(ctx, ChatStreamEvent{Data: normalizedStreamErrorData()}); err != nil {
			summary.ErrorClass = "client_disconnected"
			summary.CompletionStatus = "client_disconnected"
			return err
		}
		summary.NormalizedErrorSent = true
		return fmt.Errorf("upstream stream error")
	}
	chunk, err := openai.NormalizeStreamChunk(data)
	if err != nil {
		summary.ErrorClass = "upstream_stream_invalid"
		summary.CompletionStatus = "upstream_invalid"
		return err
	}
	if summary.ChunkCount >= a.maxStreamEvents() {
		summary.ErrorClass = "upstream_event_limit"
		summary.CompletionStatus = "event_limit"
		return fmt.Errorf("upstream stream event limit exceeded")
	}
	if err := sink.WriteEvent(ctx, ChatStreamEvent{Data: chunk.Body}); err != nil {
		summary.ErrorClass = "client_disconnected"
		summary.CompletionStatus = "client_disconnected"
		return err
	}
	summary.Started = true
	summary.ChunkCount++
	if summary.ResolvedModel == "" && chunk.ResolvedModel != "" {
		summary.ResolvedModel = chunk.ResolvedModel
	}
	if chunk.HasUsage {
		if providerType == "openrouter" {
			chunk.Usage.CostMicrounits = openRouterCostMicrounitsFromStreamChunk(data)
		}
		summary.Usage = chunk.Usage
		if summary.TimeToFirstTokenMS > 0 {
			elapsedSeconds := time.Since(start).Seconds()
			if elapsedSeconds > 0 {
				summary.OutputTokensPerSecond = float64(chunk.Usage.CompletionTokens) / elapsedSeconds
			}
		}
	}
	if chunk.OutputToken && summary.TimeToFirstTokenMS == 0 {
		summary.TimeToFirstTokenMS = time.Since(start).Milliseconds()
	}
	return nil
}

func normalizedStreamErrorData() []byte {
	return []byte(`{"error":{"message":"upstream stream failed","type":"api_error","code":"upstream_stream_error"}}`)
}

func classifyStreamReadError(ctx context.Context, err error) string {
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return "client_disconnected"
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "upstream_timeout"
	}
	if errors.Is(err, bufio.ErrBufferFull) {
		return "upstream_stream_too_large"
	}
	return classifyTransportError(err)
}

func streamStatusForError(errorClass string) string {
	switch errorClass {
	case "client_disconnected":
		return "client_disconnected"
	case "canceled":
		return "canceled"
	case "upstream_timeout":
		return "upstream_timeout"
	case "upstream_stream_too_large":
		return "too_large"
	case "upstream_event_limit":
		return "event_limit"
	case "upstream_http_error", "upstream_stream_error", "upstream_network_error":
		return "upstream_error"
	default:
		return "upstream_invalid"
	}
}
