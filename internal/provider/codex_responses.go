package provider

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"ilonasin/internal/openai"
)

const MaxCodexAggregateAssistantBytes = 64 << 20

var errCodexModelAuthFailed = errors.New("codex model metadata auth failed")

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
		return ChatResult{StatusCode: http.StatusBadGateway, ContentType: "application/json", ErrorClass: "model_discovery_failed", Latency: time.Since(start)}, err
	}
	body, effectiveServiceTier, err := marshalCodexResponsesRequest(req.Request, req.UpstreamModel, ids, modelMeta)
	if err != nil {
		return ChatResult{StatusCode: http.StatusBadRequest, ContentType: "application/json", Body: []byte("{}"), ErrorClass: "invalid_request", Latency: time.Since(start)}, err
	}
	endpoint, err := joinBasePath(req.Instance.BaseURL, "/responses")
	if err != nil {
		return ChatResult{ErrorClass: "provider_config_error", Latency: time.Since(start)}, err
	}
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(streamCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ChatResult{ErrorClass: "upstream_request_error", Latency: time.Since(start)}, err
	}
	addCodexRequestHeaders(httpReq, req.Credential.BearerToken, req.Credential.ChatGPTAccountID, req.Credential.ChatGPTAccountIsFedRAMP)
	addCodexResponsesHeaders(httpReq, ids)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := a.doStreamRequest(streamCtx, cancel, httpReq)
	if err != nil {
		errorClass := classifyTransportError(err)
		logProviderHTTP(ctx, a.Logger, slog.LevelError, "provider_http",
			slog.String("endpoint", "responses"),
			slog.String("method", http.MethodPost),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", req.Credential.ID),
			slog.Int64("duration_ms", durationMS(start)),
			slog.String("error_class", errorClass),
		)
		return ChatResult{StatusCode: http.StatusBadGateway, ContentType: "application/json", ErrorClass: errorClass, Latency: time.Since(start)}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		retryAfter := retryAfterFromHeader(resp.Header, time.Now())
		respBody, tooLarge, readErr := readLimitedUpstreamBody(resp.Body, MaxUpstreamChatBodyBytes)
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
			return ChatResult{StatusCode: http.StatusBadGateway, ContentType: "application/json", ErrorClass: "upstream_body_too_large", Latency: time.Since(start), RetryAfter: retryAfter, BodyTruncated: true}, fmt.Errorf("codex responses body exceeded limit")
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
			return ChatResult{StatusCode: http.StatusBadGateway, ContentType: "application/json", ErrorClass: "upstream_network_error", Latency: time.Since(start), RetryAfter: retryAfter}, readErr
		}
		if resp.StatusCode == http.StatusUnauthorized {
			logProviderHTTP(ctx, a.Logger, slog.LevelError, "provider_http",
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
			return ChatResult{StatusCode: http.StatusUnauthorized, ContentType: "application/json", ErrorClass: "upstream_auth_failed", Latency: time.Since(start), RetryAfter: retryAfter}, fmt.Errorf("codex responses status %d", resp.StatusCode)
		}
		errorClass := "upstream_http_error"
		if resp.StatusCode == http.StatusTooManyRequests {
			errorClass = "rate_limit_exceeded"
		}
		logProviderHTTP(ctx, a.Logger, statusLevel(resp.StatusCode, errorClass), "provider_http",
			slog.String("endpoint", "responses"),
			slog.String("method", http.MethodPost),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", req.Credential.ID),
			slog.Int("status", resp.StatusCode),
			slog.Int64("duration_ms", durationMS(start)),
			slog.Int("response_bytes", len(respBody)),
			slog.String("error_class", errorClass),
		)
		return ChatResult{StatusCode: http.StatusBadGateway, ContentType: "application/json", ErrorClass: errorClass, Latency: time.Since(start), RetryAfter: retryAfter}, fmt.Errorf("codex responses status %d", resp.StatusCode)
	}
	parsed, err := a.readCodexResponses(streamCtx, resp.Body)
	if err != nil {
		errorClass := parsed.ErrorClass
		if errorClass == "" {
			errorClass = classifyCodexReadError(streamCtx, err)
		}
		logProviderHTTP(ctx, a.Logger, slog.LevelError, "provider_http",
			slog.String("endpoint", "responses"),
			slog.String("method", http.MethodPost),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", req.Credential.ID),
			slog.Int("status", http.StatusBadGateway),
			slog.Int64("duration_ms", durationMS(start)),
			slog.String("error_class", errorClass),
		)
		return ChatResult{StatusCode: http.StatusBadGateway, ContentType: "application/json", ErrorClass: errorClass, Latency: time.Since(start)}, err
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
		)
		return ChatResult{StatusCode: http.StatusBadGateway, ContentType: "application/json", ErrorClass: "upstream_invalid_response", Latency: time.Since(start)}, err
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
	return ChatResult{
		StatusCode:           http.StatusOK,
		ContentType:          "application/json",
		Body:                 out,
		Usage:                parsed.Usage,
		ResolvedModel:        req.UpstreamModel,
		ResponsesOutputItems: parsed.OutputItems,
		Latency:              time.Since(start),
		EffectiveServiceTier: effectiveServiceTier,
	}, nil
}

type codexResponsesRequest struct {
	Model             string             `json:"model"`
	Instructions      string             `json:"instructions,omitempty"`
	Input             any                `json:"input"`
	Tools             any                `json:"tools"`
	ToolChoice        string             `json:"tool_choice"`
	ParallelToolCalls bool               `json:"parallel_tool_calls"`
	Reasoning         *codexReasoning    `json:"reasoning,omitempty"`
	Store             bool               `json:"store"`
	Stream            bool               `json:"stream"`
	Include           []string           `json:"include"`
	ServiceTier       string             `json:"service_tier,omitempty"`
	Text              *codexTextControls `json:"text,omitempty"`
	PromptCacheKey    string             `json:"prompt_cache_key,omitempty"`
	ClientMetadata    map[string]string  `json:"client_metadata,omitempty"`
}

type codexResponsesModel struct {
	BaseInstructions          string
	SupportsParallelToolCalls bool
	ServiceTiers              map[string]bool
	DefaultReasoningEffort    string
	SupportedReasoningEfforts []string
}

type codexReasoning struct {
	Effort  string `json:"effort,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type codexTextControls struct {
	Verbosity string `json:"verbosity,omitempty"`
}

type codexResponseItem struct {
	Type      string             `json:"type"`
	Role      string             `json:"role,omitempty"`
	Content   []codexContentItem `json:"content,omitempty"`
	Name      string             `json:"name,omitempty"`
	Namespace string             `json:"namespace,omitempty"`
	Arguments string             `json:"arguments,omitempty"`
	CallID    string             `json:"call_id,omitempty"`
	Output    string             `json:"output,omitempty"`
}

type codexContentItem struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

func marshalCodexResponsesRequest(req openai.ChatCompletionRequest, upstreamModel string, ids codexRequestIDs, model codexResponsesModel) ([]byte, string, error) {
	out := codexResponsesRequest{
		Model:             upstreamModel,
		Input:             []codexResponseItem{},
		Tools:             []any{},
		ToolChoice:        "auto",
		ParallelToolCalls: model.SupportsParallelToolCalls,
		// Privacy boundary: ilonasin remains stateless with Codex backends.
		// Do not ask upstream to persist prompts, completions, or response state.
		Store:          false,
		Stream:         true,
		Include:        []string{},
		PromptCacheKey: ids.ThreadID,
		ClientMetadata: map[string]string{
			"x-codex-installation-id": ids.InstallationID,
		},
	}
	if len(req.CodexResponsesTools) > 0 {
		out.Tools = req.CodexResponsesTools
	} else {
		tools, err := codexResponsesTools(req.Tools)
		if err != nil {
			return nil, "", err
		}
		out.Tools = tools
	}
	if req.HasField("tool_choice") {
		out.ToolChoice = "auto"
	}
	if req.HasField("provider_options") {
		reasoning, textControls, serviceTier, err := codexRequestOptions(req, model)
		if err != nil {
			return nil, "", err
		}
		out.Reasoning = reasoning
		if reasoning != nil {
			out.Include = []string{"reasoning.encrypted_content"}
		}
		out.Text = textControls
		out.ServiceTier = serviceTier
	}
	if len(req.CodexResponsesInput) > 0 {
		out.Input = req.CodexResponsesInput
		if req.CodexInstructions != "" {
			out.Instructions = req.CodexInstructions
		} else {
			out.Instructions = model.BaseInstructions
		}
		body, err := json.Marshal(out)
		return body, out.ServiceTier, err
	}
	var instructions []string
	inputItems := []codexResponseItem{}
	for _, msg := range req.Messages {
		switch msg.Role {
		case "system":
			text, err := openai.MessageContentString(msg)
			if err != nil {
				return nil, "", err
			}
			instructions = append(instructions, text)
		case "user":
			parts, err := openai.MessageContentParts(msg)
			if err != nil {
				return nil, "", err
			}
			inputItems = append(inputItems, codexResponseItem{
				Type:    "message",
				Role:    "user",
				Content: codexUserContent(parts),
			})
		case "assistant":
			if !openai.MessageContentIsArray(msg) && len(bytes.TrimSpace(msg.Content)) > 0 && !bytes.Equal(bytes.TrimSpace(msg.Content), []byte("null")) {
				text, err := openai.MessageContentString(msg)
				if err != nil {
					return nil, "", err
				}
				inputItems = append(inputItems, codexResponseItem{
					Type:    "message",
					Role:    "assistant",
					Content: []codexContentItem{{Type: "output_text", Text: text}},
				})
			}
			items, err := codexFunctionCallItems(msg.ToolCalls)
			if err != nil {
				return nil, "", err
			}
			inputItems = append(inputItems, items...)
		case "tool":
			text, err := openai.MessageContentString(msg)
			if err != nil {
				return nil, "", err
			}
			inputItems = append(inputItems, codexResponseItem{
				Type:   "function_call_output",
				CallID: msg.ToolCallID,
				Output: text,
			})
		default:
			return nil, "", fmt.Errorf("unsupported codex message role %q", msg.Role)
		}
	}
	if len(instructions) > 0 {
		out.Instructions = strings.Join(instructions, "\n\n")
	} else {
		out.Instructions = model.BaseInstructions
	}
	out.Input = inputItems
	body, err := json.Marshal(out)
	return body, out.ServiceTier, err
}

func codexUserContent(parts []openai.ChatContentPart) []codexContentItem {
	out := make([]codexContentItem, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case "text":
			out = append(out, codexContentItem{Type: "input_text", Text: part.Text})
		case "image_url":
			item := codexContentItem{Type: "input_image", ImageURL: part.ImageURL}
			if part.Detail != "" {
				item.Detail = part.Detail
			}
			out = append(out, item)
		}
	}
	return out
}

func codexResponsesTools(tools []map[string]any) ([]any, error) {
	out := make([]any, 0, len(tools))
	for _, tool := range tools {
		function, ok := tool["function"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid codex tool")
		}
		name, _ := function["name"].(string)
		description, _ := function["description"].(string)
		strict, _ := function["strict"].(bool)
		parameters, ok := function["parameters"].(map[string]any)
		if !ok || parameters == nil {
			parameters = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, map[string]any{
			"type":        "function",
			"name":        name,
			"description": description,
			"strict":      strict,
			"parameters":  parameters,
		})
	}
	return out, nil
}

func codexFunctionCallItems(calls []map[string]any) ([]codexResponseItem, error) {
	out := make([]codexResponseItem, 0, len(calls))
	for _, call := range calls {
		function, ok := call["function"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid codex function call")
		}
		callID, _ := call["id"].(string)
		name, _ := function["name"].(string)
		arguments, _ := function["arguments"].(string)
		out = append(out, codexResponseItem{
			Type:      "function_call",
			CallID:    callID,
			Name:      name,
			Arguments: arguments,
		})
	}
	return out, nil
}

func codexRequestOptions(req openai.ChatCompletionRequest, model codexResponsesModel) (*codexReasoning, *codexTextControls, string, error) {
	opts, _ := req.ReasoningOptions["codex"].(map[string]any)
	var reasoning *codexReasoning
	if rawReasoning, ok := opts["reasoning"].(map[string]any); ok {
		next := &codexReasoning{}
		if effort, ok := rawReasoning["effort"].(string); ok {
			next.Effort = model.reasoningEffort(effort)
		}
		if summary, ok := rawReasoning["summary"].(string); ok && summary != "none" {
			next.Summary = summary
		}
		if next.Effort != "" || next.Summary != "" {
			reasoning = next
		}
	}
	var textControls *codexTextControls
	if verbosity, ok := opts["verbosity"].(string); ok {
		textControls = &codexTextControls{Verbosity: verbosity}
	}
	serviceTier := ""
	if tier, ok := opts["service_tier"].(string); ok {
		switch tier {
		case "default":
			serviceTier = ""
		case "fast":
			serviceTier = "priority"
		default:
			serviceTier = tier
		}
		if serviceTier != "" && !model.ServiceTiers[serviceTier] {
			return nil, nil, "", fmt.Errorf("provider_options.codex.service_tier is not supported by model")
		}
	}
	return reasoning, textControls, serviceTier, nil
}

func (a HTTPChatAdapter) resolveCodexResponsesModel(ctx context.Context, req ChatRequest, start time.Time) (codexResponsesModel, error) {
	endpoint, err := modelsURL(req.Instance)
	if err != nil {
		return codexResponsesModel{}, err
	}
	modelCtx, cancel := context.WithTimeout(ctx, a.modelTimeout())
	defer cancel()
	httpReq, err := http.NewRequestWithContext(modelCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return codexResponsesModel{}, err
	}
	credential := req.ModelCredential
	if credential.ID == 0 {
		credential = BearerCredential{
			ID:                      req.Credential.ID,
			ProviderInstanceID:      req.Credential.ProviderInstanceID,
			Kind:                    req.Credential.Kind,
			BearerToken:             req.Credential.BearerToken,
			ChatGPTAccountID:        req.Credential.ChatGPTAccountID,
			ChatGPTAccountIsFedRAMP: req.Credential.ChatGPTAccountIsFedRAMP,
		}
	}
	addCodexRequestHeaders(httpReq, credential.BearerToken, credential.ChatGPTAccountID, credential.ChatGPTAccountIsFedRAMP)
	httpReq.Header.Set("Accept", "application/json")
	resp, err := a.Client.Do(httpReq)
	if err != nil {
		logProviderHTTP(ctx, a.Logger, slog.LevelError, "provider_http",
			slog.String("endpoint", "models"),
			slog.String("method", http.MethodGet),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", credential.ID),
			slog.Int64("duration_ms", durationMS(start)),
			slog.String("error_class", classifyTransportError(err)),
		)
		return codexResponsesModel{}, err
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
			slog.Int64("credential_id", credential.ID),
			slog.Int("status", resp.StatusCode),
			slog.Int64("duration_ms", durationMS(start)),
			slog.String("error_class", errorClass),
		)
		if resp.StatusCode == http.StatusUnauthorized {
			return codexResponsesModel{}, errCodexModelAuthFailed
		}
		return codexResponsesModel{}, fmt.Errorf("codex models status %d", resp.StatusCode)
	}
	body, tooLarge, readErr := readLimitedUpstreamBody(resp.Body, MaxUpstreamModelsBodyBytes)
	if tooLarge {
		return codexResponsesModel{}, fmt.Errorf("codex models body exceeded limit")
	}
	if readErr != nil {
		return codexResponsesModel{}, readErr
	}
	var parsed struct {
		Models []struct {
			Slug                      string `json:"slug"`
			BaseInstructions          string `json:"base_instructions"`
			SupportsParallelToolCalls bool   `json:"supports_parallel_tool_calls"`
			DefaultReasoningLevel     string `json:"default_reasoning_level"`
			SupportedReasoningLevels  []struct {
				Effort string `json:"effort"`
			} `json:"supported_reasoning_levels"`
			ServiceTiers []struct {
				ID string `json:"id"`
			} `json:"service_tiers"`
		} `json:"models"`
	}
	if err := jsonUnmarshal(body, &parsed); err != nil {
		return codexResponsesModel{}, err
	}
	for _, model := range parsed.Models {
		if model.Slug == req.UpstreamModel {
			serviceTiers := map[string]bool{}
			for _, tier := range model.ServiceTiers {
				if tier.ID != "" {
					serviceTiers[tier.ID] = true
				}
			}
			reasoningEfforts := make([]string, 0, len(model.SupportedReasoningLevels))
			for _, level := range model.SupportedReasoningLevels {
				if level.Effort != "" {
					reasoningEfforts = append(reasoningEfforts, level.Effort)
				}
			}
			return codexResponsesModel{
				BaseInstructions:          strings.TrimSpace(model.BaseInstructions),
				SupportsParallelToolCalls: model.SupportsParallelToolCalls,
				ServiceTiers:              serviceTiers,
				DefaultReasoningEffort:    model.DefaultReasoningLevel,
				SupportedReasoningEfforts: reasoningEfforts,
			}, nil
		}
	}
	return codexResponsesModel{SupportsParallelToolCalls: true, ServiceTiers: map[string]bool{}}, nil
}

func (model codexResponsesModel) reasoningEffort(requested string) string {
	if requested == "" || len(model.SupportedReasoningEfforts) == 0 {
		return requested
	}
	for _, effort := range model.SupportedReasoningEfforts {
		if effort == requested {
			return requested
		}
	}
	index := (len(model.SupportedReasoningEfforts) - 1) / 2
	if fallback := model.SupportedReasoningEfforts[index]; fallback != "" {
		return fallback
	}
	if model.DefaultReasoningEffort != "" {
		return model.DefaultReasoningEffort
	}
	return requested
}

type codexResponsesResult struct {
	Text        string
	ToolCalls   []map[string]any
	OutputItems []openai.ResponsesOutputItem
	Usage       openai.Usage
	ErrorClass  string
}

type codexResponseParseState struct {
	itemDoneText    strings.Builder
	textDoneText    strings.Builder
	deltaText       strings.Builder
	sawItemDoneText bool
	sawTextDoneText bool
	toolCalls       []map[string]any
	outputItems     []openai.ResponsesOutputItem
	toolState       codexResponseToolState
	customToolState codexResponseCustomToolState
	usage           openai.Usage
	completed       bool
}

type codexResponseToolState struct {
	order   []string
	calls   map[string]*codexResponseToolCall
	emitted map[string]bool
}

type codexResponseToolCall struct {
	CallID    string
	Name      string
	Namespace string
	Arguments strings.Builder
}

type codexResponseCustomToolState struct {
	order   []string
	calls   map[string]*codexResponseCustomToolCall
	aliases map[string]string
	emitted map[string]bool
}

type codexResponseCustomToolCall struct {
	CallID string
	Name   string
	Input  strings.Builder
}

func (a HTTPChatAdapter) readCodexResponses(ctx context.Context, body io.ReadCloser) (codexResponsesResult, error) {
	reader := bufio.NewReaderSize(body, a.maxStreamLineBytes()+1)
	var parts [][]byte
	eventBytes := 0
	events := 0
	var state codexResponseParseState
	for {
		line, err := a.readStreamLine(ctx, body, reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				if len(parts) > 0 {
					if err := handleCodexEvent(bytes.Join(parts, []byte("\n")), &state); err != nil {
						return codexResponsesResult{ErrorClass: "upstream_invalid_response"}, err
					}
				}
				if state.completed {
					return state.codexResponsesResult(), nil
				}
				return codexResponsesResult{ErrorClass: "upstream_invalid_response"}, io.ErrUnexpectedEOF
			}
			return codexResponsesResult{ErrorClass: classifyCodexReadError(ctx, err)}, err
		}
		line = bytes.TrimRight(line, "\r\n")
		if len(line) == 0 {
			if len(parts) == 0 {
				continue
			}
			events++
			if events > a.maxStreamEvents() {
				return codexResponsesResult{ErrorClass: "upstream_invalid_response"}, fmt.Errorf("codex response event limit exceeded")
			}
			if err := handleCodexEvent(bytes.Join(parts, []byte("\n")), &state); err != nil {
				return codexResponsesResult{ErrorClass: codexEventErrorClass(err)}, err
			}
			if state.aggregateBytes() > a.maxCodexAggregateBytes() {
				return codexResponsesResult{ErrorClass: "upstream_invalid_response"}, fmt.Errorf("codex response text too large")
			}
			if state.completed {
				return state.codexResponsesResult(), nil
			}
			parts = nil
			eventBytes = 0
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
			return codexResponsesResult{ErrorClass: "upstream_invalid_response"}, bufio.ErrBufferFull
		}
		part := make([]byte, len(data))
		copy(part, data)
		parts = append(parts, part)
	}
}

func (state *codexResponseParseState) codexResponsesResult() codexResponsesResult {
	return codexResponsesResult{
		Text:        codexFinalText(state.itemDoneText, state.textDoneText, state.deltaText, state.sawItemDoneText, state.sawTextDoneText),
		ToolCalls:   state.toolCalls,
		OutputItems: append(state.outputItems, state.customToolItems()...),
		Usage:       state.usage,
	}
}

func (state *codexResponseParseState) aggregateBytes() int {
	total := state.itemDoneText.Len() + state.textDoneText.Len() + state.deltaText.Len()
	for _, call := range state.toolState.calls {
		total += call.Arguments.Len()
	}
	for _, call := range state.customToolState.calls {
		total += call.Input.Len()
	}
	for _, item := range state.outputItems {
		total += len(item.Arguments)
		for _, tool := range item.Tools {
			total += len(tool)
		}
	}
	return total
}

type codexEventFailure struct {
	class string
}

func (e codexEventFailure) Error() string {
	return e.class
}

func codexEventErrorClass(err error) string {
	var failure codexEventFailure
	if errors.As(err, &failure) {
		return failure.class
	}
	return "upstream_invalid_response"
}

func handleCodexEvent(data []byte, state *codexResponseParseState) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("[DONE]")) {
		return nil
	}
	var event struct {
		Type      string `json:"type"`
		Delta     string `json:"delta"`
		Text      string `json:"text"`
		ItemID    string `json:"item_id"`
		CallID    string `json:"call_id"`
		Arguments string `json:"arguments"`
		Item      *struct {
			ID        string            `json:"id"`
			Type      string            `json:"type"`
			Role      string            `json:"role"`
			CallID    string            `json:"call_id"`
			Name      string            `json:"name"`
			Namespace string            `json:"namespace"`
			Arguments json.RawMessage   `json:"arguments"`
			Execution string            `json:"execution"`
			Status    string            `json:"status"`
			Input     string            `json:"input"`
			Tools     []json.RawMessage `json:"tools"`
			Content   []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"item"`
		Response *struct {
			ID      string `json:"id"`
			EndTurn *bool  `json:"end_turn"`
			Error   *struct {
				Code string `json:"code"`
			} `json:"error"`
			Usage *struct {
				InputTokens        *int `json:"input_tokens"`
				OutputTokens       *int `json:"output_tokens"`
				TotalTokens        *int `json:"total_tokens"`
				InputTokensDetails *struct {
					CachedTokens int `json:"cached_tokens"`
				} `json:"input_tokens_details"`
				OutputTokensDetails *struct {
					ReasoningTokens int `json:"reasoning_tokens"`
				} `json:"output_tokens_details"`
			} `json:"usage"`
		} `json:"response"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return err
	}
	if unsupportedCodexToolEvent(event.Type) || (event.Item != nil && unsupportedCodexOutputItem(event.Item.Type)) {
		return codexEventFailure{class: "upstream_invalid_response"}
	}
	switch event.Type {
	case "response.output_item.added":
		if event.Item == nil {
			return codexEventFailure{class: "upstream_invalid_response"}
		}
		switch event.Item.Type {
		case "function_call":
			return state.addCodexFunctionCall(codexToolCallKey(event.Item.ID, event.Item.CallID), event.Item.CallID, event.Item.Name, event.Item.Namespace, nil)
		case "tool_search_call":
			return nil
		case "custom_tool_call":
			return state.addCodexCustomToolCall(event.Item.ID, event.Item.CallID, event.Item.Name, event.Item.Input)
		case "message", "reasoning":
			return nil
		default:
			return codexEventFailure{class: "upstream_invalid_response"}
		}
	case "response.function_call_arguments.delta":
		return state.appendCodexFunctionCallArguments(codexToolCallKey(event.ItemID, event.CallID), event.Delta)
	case "response.function_call_arguments.done":
		return state.finishCodexFunctionCallArguments(codexToolCallKey(event.ItemID, event.CallID), event.Arguments)
	case "response.custom_tool_call_input.delta":
		return state.appendCodexCustomToolInput(event.ItemID, event.CallID, event.Delta)
	case "response.output_item.done":
		if event.Item == nil {
			return codexEventFailure{class: "upstream_invalid_response"}
		}
		switch event.Item.Type {
		case "function_call":
			if err := state.addCodexFunctionCall(codexToolCallKey(event.Item.ID, event.Item.CallID), event.Item.CallID, event.Item.Name, event.Item.Namespace, event.Item.Arguments); err != nil {
				return err
			}
			return state.emitCodexFunctionCall(codexToolCallKey(event.Item.ID, event.Item.CallID))
		case "tool_search_call":
			return state.addCodexToolSearchCall(event.Item.CallID, event.Item.Execution, event.Item.Arguments, event.Item.Tools)
		case "custom_tool_call":
			return state.finishCodexCustomToolCall(event.Item.ID, event.Item.CallID, event.Item.Name, event.Item.Input)
		case "message":
			if event.Item.Role == "assistant" {
				state.sawItemDoneText = true
				for _, content := range event.Item.Content {
					if content.Type == "output_text" {
						state.itemDoneText.WriteString(content.Text)
					}
				}
			}
		case "reasoning":
			return nil
		default:
			return codexEventFailure{class: "upstream_invalid_response"}
		}
	case "response.output_text.done":
		if event.Text != "" {
			state.sawTextDoneText = true
			state.textDoneText.WriteString(event.Text)
		}
	case "response.output_text.delta":
		state.deltaText.WriteString(event.Delta)
	case "response.completed":
		if state.completed {
			return fmt.Errorf("duplicate codex completion")
		}
		if event.Response == nil {
			return fmt.Errorf("missing codex response")
		}
		if err := state.flushCodexFunctionCalls(); err != nil {
			return err
		}
		state.completed = true
		if event.Response.Usage != nil {
			if event.Response.Usage.InputTokens == nil || event.Response.Usage.OutputTokens == nil || event.Response.Usage.TotalTokens == nil {
				return fmt.Errorf("invalid codex usage")
			}
			state.usage.PromptTokens = *event.Response.Usage.InputTokens
			state.usage.CompletionTokens = *event.Response.Usage.OutputTokens
			state.usage.TotalTokens = *event.Response.Usage.TotalTokens
			if event.Response.Usage.InputTokensDetails != nil {
				state.usage.CachedTokens = event.Response.Usage.InputTokensDetails.CachedTokens
			}
			if event.Response.Usage.OutputTokensDetails != nil {
				state.usage.ReasoningTokens = event.Response.Usage.OutputTokensDetails.ReasoningTokens
			}
		}
	case "response.failed":
		if event.Response != nil && event.Response.Error != nil && event.Response.Error.Code == "rate_limit_exceeded" {
			return codexEventFailure{class: "rate_limit_exceeded"}
		}
		return codexEventFailure{class: "upstream_response_failed"}
	case "response.incomplete":
		return codexEventFailure{class: "upstream_response_incomplete"}
	}
	return nil
}

func (state *codexResponseParseState) addCodexFunctionCall(key, callID, name, namespace string, arguments json.RawMessage) error {
	if key == "" || callID == "" || name == "" {
		return fmt.Errorf("invalid codex function_call")
	}
	if state.toolState.calls == nil {
		state.toolState.calls = map[string]*codexResponseToolCall{}
		state.toolState.emitted = map[string]bool{}
	}
	call := state.toolState.calls[key]
	if call == nil {
		call = &codexResponseToolCall{CallID: callID, Name: name, Namespace: namespace}
		state.toolState.calls[key] = call
		state.toolState.order = append(state.toolState.order, key)
	} else if call.CallID != callID || call.Name != name || call.Namespace != namespace {
		return fmt.Errorf("conflicting codex function_call")
	}
	if len(arguments) != 0 && !bytes.Equal(bytes.TrimSpace(arguments), []byte("null")) {
		call.Arguments.Reset()
		call.Arguments.Write(bytes.TrimSpace(arguments))
	}
	return nil
}

func (state *codexResponseParseState) appendCodexFunctionCallArguments(key, delta string) error {
	if key == "" || delta == "" {
		return codexEventFailure{class: "upstream_invalid_response"}
	}
	call := state.toolState.calls[key]
	if call == nil {
		return codexEventFailure{class: "upstream_invalid_response"}
	}
	call.Arguments.WriteString(delta)
	return nil
}

func (state *codexResponseParseState) finishCodexFunctionCallArguments(key, arguments string) error {
	if key == "" || arguments == "" {
		return codexEventFailure{class: "upstream_invalid_response"}
	}
	call := state.toolState.calls[key]
	if call == nil {
		return codexEventFailure{class: "upstream_invalid_response"}
	}
	written := call.Arguments.String()
	switch {
	case strings.HasPrefix(arguments, written):
		call.Arguments.WriteString(arguments[len(written):])
	case written == "":
		call.Arguments.WriteString(arguments)
	default:
		return codexEventFailure{class: "upstream_invalid_response"}
	}
	return nil
}

func (state *codexResponseParseState) emitCodexFunctionCall(key string) error {
	if state.toolState.emitted[key] {
		return fmt.Errorf("duplicate codex function_call")
	}
	call := state.toolState.calls[key]
	if call == nil {
		return fmt.Errorf("missing codex function_call")
	}
	if call.Namespace != "" {
		state.toolState.emitted[key] = true
		return state.addCodexResponsesFunctionCallItem(call)
	}
	out, err := codexToolCall(call.CallID, call.Name, call.Arguments.String())
	if err != nil {
		return err
	}
	state.toolCalls = append(state.toolCalls, out)
	state.toolState.emitted[key] = true
	return nil
}

func (state *codexResponseParseState) addCodexResponsesFunctionCallItem(call *codexResponseToolCall) error {
	if call == nil || call.CallID == "" || call.Name == "" {
		return fmt.Errorf("invalid codex function_call")
	}
	arguments := json.RawMessage(call.Arguments.String())
	if len(bytes.TrimSpace(arguments)) == 0 {
		arguments = json.RawMessage(`"{}"`)
	} else if bytes.TrimSpace(arguments)[0] != '"' {
		body, err := json.Marshal(call.Arguments.String())
		if err != nil {
			return err
		}
		arguments = body
	}
	stateOutput := openai.ResponsesOutputItem{
		Type:      "function_call",
		CallID:    call.CallID,
		Name:      call.Name,
		Namespace: call.Namespace,
		Arguments: arguments,
	}
	state.outputItem(stateOutput)
	return nil
}

func (state *codexResponseParseState) outputItem(item openai.ResponsesOutputItem) {
	state.outputItems = append(state.outputItems, item)
}

func (state *codexResponseParseState) addCodexToolSearchCall(callID, execution string, arguments json.RawMessage, tools []json.RawMessage) error {
	if callID == "" {
		return codexEventFailure{class: "upstream_invalid_response"}
	}
	if execution == "" {
		execution = "client"
	}
	if execution != "client" && execution != "server" {
		return codexEventFailure{class: "upstream_invalid_response"}
	}
	if len(bytes.TrimSpace(arguments)) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	state.outputItem(openai.ResponsesOutputItem{
		Type:      "tool_search_call",
		CallID:    callID,
		Arguments: json.RawMessage(bytes.TrimSpace(arguments)),
		Execution: execution,
		Tools:     tools,
	})
	return nil
}

func (state *codexResponseParseState) flushCodexFunctionCalls() error {
	for _, key := range state.toolState.order {
		if state.toolState.emitted[key] {
			continue
		}
		if err := state.emitCodexFunctionCall(key); err != nil {
			return err
		}
	}
	return nil
}

func (state *codexResponseParseState) addCodexCustomToolCall(itemID, callID, name, input string) error {
	if callID == "" || name == "" {
		return codexEventFailure{class: "upstream_invalid_response"}
	}
	if state.customToolState.calls == nil {
		state.customToolState.calls = map[string]*codexResponseCustomToolCall{}
		state.customToolState.aliases = map[string]string{}
		state.customToolState.emitted = map[string]bool{}
	}
	key := callID
	if itemID != "" {
		if alias := state.customToolState.aliases[itemID]; alias != "" {
			key = alias
		} else if state.customToolState.calls[itemID] != nil {
			key = itemID
		}
	}
	if key == "" {
		key = itemID
	}
	call := state.customToolState.calls[key]
	if call == nil {
		call = &codexResponseCustomToolCall{CallID: callID, Name: name}
		state.customToolState.calls[key] = call
		state.customToolState.order = append(state.customToolState.order, key)
	} else {
		if call.CallID == "" {
			call.CallID = callID
		} else if call.CallID != callID {
			return codexEventFailure{class: "upstream_invalid_response"}
		}
		if call.Name == "" {
			call.Name = name
		} else if call.Name != name {
			return codexEventFailure{class: "upstream_invalid_response"}
		}
	}
	if itemID != "" {
		state.customToolState.aliases[itemID] = key
	}
	if input != "" {
		written := call.Input.String()
		switch {
		case written == "":
			call.Input.WriteString(input)
		case written == input:
		case strings.HasPrefix(input, written):
			call.Input.WriteString(input[len(written):])
		default:
			return codexEventFailure{class: "upstream_invalid_response"}
		}
	}
	return nil
}

func (state *codexResponseParseState) appendCodexCustomToolInput(itemID, callID, delta string) error {
	if delta == "" {
		return nil
	}
	key := callID
	if key == "" && state.customToolState.aliases != nil {
		key = state.customToolState.aliases[itemID]
	}
	if key == "" {
		key = itemID
	}
	if key == "" {
		return codexEventFailure{class: "upstream_invalid_response"}
	}
	call := state.customToolState.calls[key]
	if call == nil {
		if callID == "" {
			return codexEventFailure{class: "upstream_invalid_response"}
		}
		if state.customToolState.calls == nil {
			state.customToolState.calls = map[string]*codexResponseCustomToolCall{}
			state.customToolState.aliases = map[string]string{}
			state.customToolState.emitted = map[string]bool{}
		}
		call = &codexResponseCustomToolCall{CallID: callID}
		state.customToolState.calls[key] = call
		state.customToolState.order = append(state.customToolState.order, key)
		if itemID != "" {
			state.customToolState.aliases[itemID] = key
		}
	}
	call.Input.WriteString(delta)
	return nil
}

func (state *codexResponseParseState) finishCodexCustomToolCall(itemID, callID, name, input string) error {
	if err := state.addCodexCustomToolCall(itemID, callID, name, input); err != nil {
		return err
	}
	key := callID
	if itemID != "" && state.customToolState.aliases != nil {
		if alias := state.customToolState.aliases[itemID]; alias != "" {
			key = alias
		} else if state.customToolState.calls[itemID] != nil {
			key = itemID
		}
	}
	if key == "" {
		return codexEventFailure{class: "upstream_invalid_response"}
	}
	if state.customToolState.emitted[key] {
		return fmt.Errorf("duplicate codex custom_tool_call")
	}
	state.customToolState.emitted[key] = true
	return nil
}

func (state *codexResponseParseState) customToolItems() []openai.ResponsesOutputItem {
	if len(state.customToolState.order) == 0 {
		return nil
	}
	out := make([]openai.ResponsesOutputItem, 0, len(state.customToolState.order))
	for _, key := range state.customToolState.order {
		if !state.customToolState.emitted[key] {
			continue
		}
		call := state.customToolState.calls[key]
		if call == nil || call.CallID == "" || call.Name == "" {
			continue
		}
		out = append(out, openai.ResponsesOutputItem{
			Type:   "custom_tool_call",
			CallID: call.CallID,
			Name:   call.Name,
			Input:  call.Input.String(),
		})
	}
	return out
}

func codexFinalText(itemDoneText, textDoneText, deltaText strings.Builder, sawItemDoneText, sawTextDoneText bool) string {
	if sawItemDoneText {
		return itemDoneText.String()
	}
	if sawTextDoneText {
		return textDoneText.String()
	}
	return deltaText.String()
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

func (a HTTPChatAdapter) streamCodexChat(ctx context.Context, req ChatRequest, sink ChatStreamSink, start time.Time) (ChatStreamSummary, error) {
	if req.Credential.Kind != CredentialKindOAuthAccess {
		return withStreamLatency(start, ChatStreamSummary{StatusCode: http.StatusUnauthorized, ErrorClass: "credential_unavailable", CompletionStatus: "upstream_error", PreStreamError: true}), fmt.Errorf("codex chat requires oauth access credential")
	}
	ids := newCodexRequestIDs()
	modelMeta, err := a.resolveCodexResponsesModel(ctx, req, start)
	if err != nil {
		if errors.Is(err, errCodexModelAuthFailed) {
			return withStreamLatency(start, ChatStreamSummary{StatusCode: http.StatusUnauthorized, ErrorClass: "model_discovery_auth_failed", CompletionStatus: "upstream_error", PreStreamError: true}), err
		}
		return withStreamLatency(start, ChatStreamSummary{StatusCode: http.StatusBadGateway, ErrorClass: "model_discovery_failed", CompletionStatus: "upstream_error", PreStreamError: true}), err
	}
	body, effectiveServiceTier, err := marshalCodexResponsesRequest(req.Request, req.UpstreamModel, ids, modelMeta)
	if err != nil {
		return withStreamLatency(start, ChatStreamSummary{StatusCode: http.StatusBadRequest, ErrorClass: "invalid_request", CompletionStatus: "upstream_invalid", PreStreamError: true}), err
	}
	endpoint, err := joinBasePath(req.Instance.BaseURL, "/responses")
	if err != nil {
		return withStreamLatency(start, ChatStreamSummary{ErrorClass: "provider_config_error", CompletionStatus: "upstream_invalid", PreStreamError: true}), err
	}
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(streamCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return withStreamLatency(start, ChatStreamSummary{ErrorClass: "upstream_request_error", CompletionStatus: "upstream_invalid", PreStreamError: true}), err
	}
	addCodexRequestHeaders(httpReq, req.Credential.BearerToken, req.Credential.ChatGPTAccountID, req.Credential.ChatGPTAccountIsFedRAMP)
	addCodexResponsesHeaders(httpReq, ids)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := a.doStreamRequest(streamCtx, cancel, httpReq)
	if err != nil {
		errorClass := classifyTransportError(err)
		logProviderHTTP(ctx, a.Logger, slog.LevelError, "provider_http",
			slog.String("endpoint", "responses_stream"),
			slog.String("method", http.MethodPost),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", req.Credential.ID),
			slog.Int64("duration_ms", durationMS(start)),
			slog.String("error_class", errorClass),
		)
		return withStreamLatency(start, ChatStreamSummary{
			StatusCode:       http.StatusBadGateway,
			ErrorClass:       errorClass,
			CompletionStatus: streamStatusForError(errorClass),
			PreStreamError:   true,
		}), err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errorClass := "upstream_http_error"
		if resp.StatusCode == http.StatusUnauthorized {
			errorClass = "upstream_auth_failed"
		}
		respBody, tooLarge, readErr := readLimitedUpstreamBody(resp.Body, MaxUpstreamChatBodyBytes)
		if tooLarge {
			errorClass = "upstream_body_too_large"
		} else if readErr != nil {
			errorClass = "upstream_network_error"
		}
		attrs := []slog.Attr{
			slog.String("endpoint", "responses_stream"),
			slog.String("method", http.MethodPost),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", req.Credential.ID),
			slog.Int("status", resp.StatusCode),
			slog.Int64("duration_ms", durationMS(start)),
			slog.String("error_class", errorClass),
		}
		if !tooLarge && readErr == nil {
			attrs = append(attrs, slog.Int("response_bytes", len(respBody)))
		}
		logProviderHTTP(ctx, a.Logger, statusLevel(resp.StatusCode, errorClass), "provider_http", attrs...)
		if tooLarge {
			resp.StatusCode = http.StatusBadGateway
		}
		return withStreamLatency(start, ChatStreamSummary{
			StatusCode:       resp.StatusCode,
			ErrorClass:       errorClass,
			CompletionStatus: "upstream_error",
			RetryAfter:       retryAfterFromHeader(resp.Header, time.Now()),
			PreStreamError:   true,
		}), fmt.Errorf("codex responses stream status %d", resp.StatusCode)
	}

	summary := ChatStreamSummary{
		StatusCode:           http.StatusOK,
		CompletionStatus:     "completed",
		ResolvedModel:        req.UpstreamModel,
		EffectiveServiceTier: effectiveServiceTier,
	}
	state := codexStreamState{
		id:           localChatCompletionID(),
		model:        req.Instance.ID + "/" + req.UpstreamModel,
		created:      time.Now().Unix(),
		includeUsage: includeStreamUsage(req.Request),
	}
	if err := a.readCodexStream(streamCtx, resp.Body, sink, &summary, &state, start); err != nil {
		if summary.ErrorClass == "" {
			summary.ErrorClass = classifyCodexReadError(streamCtx, err)
		}
		if summary.Started && summary.ErrorClass != "client_disconnected" && !summary.NormalizedErrorSent && !summary.Done {
			if writeErr := sink.WriteEvent(ctx, ChatStreamEvent{Data: normalizedStreamErrorData(summary.ErrorClass)}); writeErr != nil {
				summary.ErrorClass = "client_disconnected"
				summary.CompletionStatus = "client_disconnected"
				return withStreamLatency(start, summary), writeErr
			}
			summary.NormalizedErrorSent = true
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
		return withStreamLatency(start, summary), err
	}
	return withStreamLatency(start, summary), nil
}

type codexStreamState struct {
	id                string
	model             string
	created           int64
	includeUsage      bool
	wroteRole         bool
	sawDelta          bool
	sawOutputTextDone bool
	sawOutputItemDone bool
	outputTextDone    strings.Builder
	outputItemDone    strings.Builder
	usage             openai.Usage
	hasUsage          bool
	events            int
	sawToolCall       bool
	toolCallIndex     int
	toolCalls         map[string]*codexStreamToolCall
}

type codexStreamToolCall struct {
	Index     int
	CallID    string
	Name      string
	Arguments strings.Builder
	Finalized bool
}

func includeStreamUsage(req openai.ChatCompletionRequest) bool {
	if req.StreamOptions == nil {
		return false
	}
	include, _ := req.StreamOptions["include_usage"].(bool)
	return include
}

func (a HTTPChatAdapter) readCodexStream(ctx context.Context, body io.ReadCloser, sink ChatStreamSink, summary *ChatStreamSummary, state *codexStreamState, start time.Time) error {
	reader := bufio.NewReaderSize(body, a.maxStreamLineBytes()+1)
	var parts [][]byte
	eventBytes := 0
	for {
		line, err := a.readStreamLine(ctx, body, reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				if len(parts) > 0 {
					if err := a.handleCodexStreamEvent(ctx, bytes.Join(parts, []byte("\n")), sink, summary, state, start); err != nil {
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
			state.events++
			if state.events > a.maxStreamEvents() {
				summary.ErrorClass = "upstream_event_limit"
				summary.CompletionStatus = "event_limit"
				return fmt.Errorf("codex response event limit exceeded")
			}
			if err := a.handleCodexStreamEvent(ctx, bytes.Join(parts, []byte("\n")), sink, summary, state, start); err != nil {
				return err
			}
			if state.aggregateBytes() > a.maxCodexAggregateBytes() {
				summary.ErrorClass = "upstream_stream_too_large"
				summary.CompletionStatus = "too_large"
				return fmt.Errorf("codex response aggregate too large")
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

func (a HTTPChatAdapter) handleCodexStreamEvent(ctx context.Context, data []byte, sink ChatStreamSink, summary *ChatStreamSummary, state *codexStreamState, start time.Time) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("[DONE]")) {
		return nil
	}
	var event struct {
		Type      string `json:"type"`
		Delta     string `json:"delta"`
		Text      string `json:"text"`
		ItemID    string `json:"item_id"`
		CallID    string `json:"call_id"`
		Arguments string `json:"arguments"`
		Item      *struct {
			ID        string `json:"id"`
			Type      string `json:"type"`
			Role      string `json:"role"`
			CallID    string `json:"call_id"`
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
			Arguments string `json:"arguments"`
			Content   []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"item"`
		Response *struct {
			Error *struct {
				Code string `json:"code"`
			} `json:"error"`
			Usage *codexUsagePayload `json:"usage"`
		} `json:"response"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		summary.ErrorClass = "upstream_stream_invalid"
		summary.CompletionStatus = "upstream_invalid"
		return err
	}
	if codexToolEvent(event.Type) || (event.Item != nil && codexToolEvent(event.Item.Type)) {
		summary.ErrorClass = "upstream_invalid_response"
		summary.CompletionStatus = "upstream_invalid"
		return fmt.Errorf("codex stream contained unsupported tool event")
	}
	switch event.Type {
	case "response.output_text.delta":
		if event.Delta == "" {
			return nil
		}
		if err := writeCodexRoleChunk(ctx, sink, summary, state); err != nil {
			return err
		}
		if err := writeCodexContentChunk(ctx, sink, summary, state, event.Delta, start); err != nil {
			return err
		}
		state.sawDelta = true
	case "response.output_text.done":
		if event.Text != "" {
			state.sawOutputTextDone = true
			state.outputTextDone.WriteString(event.Text)
		}
	case "response.output_item.added":
		if event.Item == nil {
			summary.ErrorClass = "upstream_invalid_response"
			summary.CompletionStatus = "upstream_invalid"
			return fmt.Errorf("codex stream missing output item")
		}
		if event.Item.Type != "function_call" {
			switch event.Item.Type {
			case "message", "reasoning":
				return nil
			default:
				summary.ErrorClass = "upstream_invalid_response"
				summary.CompletionStatus = "upstream_invalid"
				return fmt.Errorf("codex stream contained unsupported output item")
			}
		}
		if event.Item.Namespace != "" {
			summary.ErrorClass = "upstream_invalid_response"
			summary.CompletionStatus = "upstream_invalid"
			return fmt.Errorf("codex stream contained unsupported namespaced function_call")
		}
		if err := writeCodexRoleChunk(ctx, sink, summary, state); err != nil {
			return err
		}
		if _, err := writeCodexToolCallStartChunk(ctx, sink, summary, state, codexToolCallKey(event.Item.ID, event.Item.CallID), event.Item.CallID, event.Item.Name, start); err != nil {
			return err
		}
		state.sawToolCall = true
	case "response.function_call_arguments.delta":
		key := codexToolCallKey(event.ItemID, event.CallID)
		if state.codexToolCall(key) == nil {
			summary.ErrorClass = "upstream_invalid_response"
			summary.CompletionStatus = "upstream_invalid"
			return fmt.Errorf("codex stream contained orphan function_call_arguments.delta")
		}
		if event.Delta == "" {
			return nil
		}
		if err := writeCodexRoleChunk(ctx, sink, summary, state); err != nil {
			return err
		}
		if err := writeCodexToolCallArgumentsChunk(ctx, sink, summary, state, key, event.Delta, start); err != nil {
			return err
		}
		state.sawToolCall = true
	case "response.function_call_arguments.done":
		key := codexToolCallKey(event.ItemID, event.CallID)
		if state.codexToolCall(key) == nil {
			summary.ErrorClass = "upstream_invalid_response"
			summary.CompletionStatus = "upstream_invalid"
			return fmt.Errorf("codex stream contained orphan function_call_arguments.done")
		}
		if event.Arguments == "" {
			return nil
		}
		if err := writeCodexRoleChunk(ctx, sink, summary, state); err != nil {
			return err
		}
		if err := writeCodexToolCallArgumentsDoneChunk(ctx, sink, summary, state, key, event.Arguments, start); err != nil {
			return err
		}
		state.sawToolCall = true
	case "response.output_item.done":
		if event.Item == nil {
			summary.ErrorClass = "upstream_invalid_response"
			summary.CompletionStatus = "upstream_invalid"
			return fmt.Errorf("codex stream missing output item")
		}
		if event.Item.Type == "function_call" {
			if event.Item.Namespace != "" {
				summary.ErrorClass = "upstream_invalid_response"
				summary.CompletionStatus = "upstream_invalid"
				return fmt.Errorf("codex stream contained unsupported namespaced function_call")
			}
			if err := writeCodexRoleChunk(ctx, sink, summary, state); err != nil {
				return err
			}
			call, err := codexToolCall(event.Item.CallID, event.Item.Name, event.Item.Arguments)
			if err != nil {
				summary.ErrorClass = "upstream_stream_invalid"
				summary.CompletionStatus = "upstream_invalid"
				return err
			}
			if err := writeCodexToolCallChunk(ctx, sink, summary, state, codexToolCallKey(event.Item.ID, event.Item.CallID), call, start); err != nil {
				return err
			}
			state.sawToolCall = true
			return nil
		}
		if event.Item.Type == "reasoning" {
			return nil
		}
		if event.Item.Type != "message" || event.Item.Role != "assistant" {
			summary.ErrorClass = "upstream_invalid_response"
			summary.CompletionStatus = "upstream_invalid"
			return fmt.Errorf("codex stream contained unsupported output item")
		}
		for _, content := range event.Item.Content {
			if content.Type == "output_text" && content.Text != "" {
				state.sawOutputItemDone = true
				state.outputItemDone.WriteString(content.Text)
			}
		}
	case "response.completed":
		if event.Response == nil {
			summary.ErrorClass = "upstream_stream_invalid"
			summary.CompletionStatus = "upstream_invalid"
			return fmt.Errorf("missing codex response")
		}
		if event.Response.Usage != nil {
			usage, err := codexUsageFromResponse(event.Response.Usage)
			if err != nil {
				summary.ErrorClass = "upstream_stream_invalid"
				summary.CompletionStatus = "upstream_invalid"
				return err
			}
			state.usage = usage
			state.hasUsage = true
			summary.Usage = usage
		}
		if !state.sawDelta && !state.sawToolCall {
			text := state.outputTextDone.String()
			if text == "" {
				text = state.outputItemDone.String()
			}
			if text != "" {
				if err := writeCodexRoleChunk(ctx, sink, summary, state); err != nil {
					return err
				}
				if err := writeCodexContentChunk(ctx, sink, summary, state, text, start); err != nil {
					return err
				}
			}
		}
		if err := writeCodexFinishChunk(ctx, sink, summary, state, start); err != nil {
			return err
		}
		if state.includeUsage && state.hasUsage {
			if err := writeCodexUsageChunk(ctx, sink, summary, state); err != nil {
				return err
			}
		}
		if err := sink.WriteDone(ctx); err != nil {
			summary.ErrorClass = "client_disconnected"
			summary.CompletionStatus = "client_disconnected"
			return err
		}
		summary.Done = true
	case "response.failed":
		if event.Response != nil && event.Response.Error != nil && event.Response.Error.Code == "rate_limit_exceeded" {
			summary.ErrorClass = "rate_limit_exceeded"
		} else {
			summary.ErrorClass = "upstream_response_failed"
		}
		summary.CompletionStatus = "upstream_error"
		return fmt.Errorf("codex response failed")
	case "response.incomplete":
		summary.ErrorClass = "upstream_response_incomplete"
		summary.CompletionStatus = "upstream_error"
		return fmt.Errorf("codex response incomplete")
	}
	return nil
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
	if typ == "response.custom_tool_call_input.delta" {
		return false
	}
	return codexToolEvent(typ)
}

func unsupportedCodexOutputItem(typ string) bool {
	typ = strings.ToLower(typ)
	if typ == "custom_tool_call" || typ == "tool_search_call" {
		return false
	}
	return codexToolEvent(typ)
}

func writeCodexRoleChunk(ctx context.Context, sink ChatStreamSink, summary *ChatStreamSummary, state *codexStreamState) error {
	if state.wroteRole {
		return nil
	}
	body, err := marshalCodexStreamChunk(state.id, state.model, state.created, map[string]any{"role": "assistant"}, nil)
	if err != nil {
		summary.ErrorClass = "upstream_stream_invalid"
		summary.CompletionStatus = "upstream_invalid"
		return err
	}
	if err := sink.WriteEvent(ctx, ChatStreamEvent{Data: body}); err != nil {
		summary.ErrorClass = "client_disconnected"
		summary.CompletionStatus = "client_disconnected"
		return err
	}
	state.wroteRole = true
	summary.Started = true
	summary.ChunkCount++
	return nil
}

func writeCodexContentChunk(ctx context.Context, sink ChatStreamSink, summary *ChatStreamSummary, state *codexStreamState, content string, start time.Time) error {
	body, err := marshalCodexStreamChunk(state.id, state.model, state.created, map[string]any{"content": content}, nil)
	if err != nil {
		summary.ErrorClass = "upstream_stream_invalid"
		summary.CompletionStatus = "upstream_invalid"
		return err
	}
	if err := sink.WriteEvent(ctx, ChatStreamEvent{Data: body}); err != nil {
		summary.ErrorClass = "client_disconnected"
		summary.CompletionStatus = "client_disconnected"
		return err
	}
	summary.Started = true
	summary.ChunkCount++
	if summary.TimeToFirstTokenMS == 0 {
		summary.TimeToFirstTokenMS = time.Since(start).Milliseconds()
	}
	if state.hasUsage && summary.TimeToFirstTokenMS > 0 {
		elapsedSeconds := time.Since(start).Seconds()
		if elapsedSeconds > 0 {
			summary.OutputTokensPerSecond = float64(state.usage.CompletionTokens) / elapsedSeconds
		}
	}
	return nil
}

func writeCodexToolCallChunk(ctx context.Context, sink ChatStreamSink, summary *ChatStreamSummary, state *codexStreamState, key string, call map[string]any, start time.Time) error {
	tracked := state.codexToolCall(key)
	if tracked == nil {
		call["index"] = state.toolCallIndex
		state.toolCallIndex++
		return writeCodexToolCallDelta(ctx, sink, summary, state, call, start)
	}
	if tracked.Finalized {
		summary.ErrorClass = "upstream_invalid_response"
		summary.CompletionStatus = "upstream_invalid"
		return fmt.Errorf("duplicate codex function_call")
	}
	args := codexToolCallArguments(call)
	if args != "" {
		written := tracked.Arguments.String()
		switch {
		case strings.HasPrefix(args, written):
			if suffix := args[len(written):]; suffix != "" {
				if err := writeCodexToolCallArgumentsByIndex(ctx, sink, summary, state, tracked.Index, suffix, start); err != nil {
					return err
				}
			}
		case written == "":
			if err := writeCodexToolCallArgumentsByIndex(ctx, sink, summary, state, tracked.Index, args, start); err != nil {
				return err
			}
		}
	}
	tracked.Finalized = true
	return nil
}

func writeCodexToolCallStartChunk(ctx context.Context, sink ChatStreamSink, summary *ChatStreamSummary, state *codexStreamState, key, callID, name string, start time.Time) (*codexStreamToolCall, error) {
	if callID == "" || name == "" {
		summary.ErrorClass = "upstream_stream_invalid"
		summary.CompletionStatus = "upstream_invalid"
		return nil, fmt.Errorf("invalid codex function_call")
	}
	if tracked := state.codexToolCall(key); tracked != nil {
		if tracked.CallID != callID || tracked.Name != name || tracked.Finalized {
			summary.ErrorClass = "upstream_invalid_response"
			summary.CompletionStatus = "upstream_invalid"
			return nil, fmt.Errorf("conflicting codex function_call")
		}
		return tracked, nil
	}
	tracked := &codexStreamToolCall{Index: state.toolCallIndex, CallID: callID, Name: name}
	state.toolCallIndex++
	if state.toolCalls == nil {
		state.toolCalls = map[string]*codexStreamToolCall{}
	}
	if key != "" {
		state.toolCalls[key] = tracked
	}
	call := map[string]any{
		"index": tracked.Index,
		"id":    callID,
		"type":  "function",
		"function": map[string]any{
			"name":      name,
			"arguments": "",
		},
	}
	if err := writeCodexToolCallDelta(ctx, sink, summary, state, call, start); err != nil {
		return nil, err
	}
	return tracked, nil
}

func writeCodexToolCallArgumentsChunk(ctx context.Context, sink ChatStreamSink, summary *ChatStreamSummary, state *codexStreamState, key, delta string, start time.Time) error {
	tracked := state.codexToolCall(key)
	if tracked == nil {
		summary.ErrorClass = "upstream_invalid_response"
		summary.CompletionStatus = "upstream_invalid"
		return fmt.Errorf("codex stream contained orphan function_call_arguments.delta")
	}
	return writeCodexToolCallArgumentsByIndex(ctx, sink, summary, state, tracked.Index, delta, start)
}

func writeCodexToolCallArgumentsDoneChunk(ctx context.Context, sink ChatStreamSink, summary *ChatStreamSummary, state *codexStreamState, key, arguments string, start time.Time) error {
	tracked := state.codexToolCall(key)
	if tracked == nil {
		summary.ErrorClass = "upstream_invalid_response"
		summary.CompletionStatus = "upstream_invalid"
		return fmt.Errorf("codex stream contained orphan function_call_arguments.done")
	}
	written := tracked.Arguments.String()
	switch {
	case strings.HasPrefix(arguments, written):
		if suffix := arguments[len(written):]; suffix != "" {
			if err := writeCodexToolCallArgumentsByIndex(ctx, sink, summary, state, tracked.Index, suffix, start); err != nil {
				return err
			}
		}
	case written == "":
		if err := writeCodexToolCallArgumentsByIndex(ctx, sink, summary, state, tracked.Index, arguments, start); err != nil {
			return err
		}
	default:
		summary.ErrorClass = "upstream_invalid_response"
		summary.CompletionStatus = "upstream_invalid"
		return fmt.Errorf("conflicting codex function_call arguments")
	}
	tracked.Finalized = true
	return nil
}

func writeCodexToolCallArgumentsByIndex(ctx context.Context, sink ChatStreamSink, summary *ChatStreamSummary, state *codexStreamState, index int, delta string, start time.Time) error {
	call := map[string]any{
		"index": index,
		"function": map[string]any{
			"arguments": delta,
		},
	}
	for _, tracked := range state.toolCalls {
		if tracked.Index == index {
			tracked.Arguments.WriteString(delta)
			break
		}
	}
	return writeCodexToolCallDelta(ctx, sink, summary, state, call, start)
}

func writeCodexToolCallDelta(ctx context.Context, sink ChatStreamSink, summary *ChatStreamSummary, state *codexStreamState, call map[string]any, start time.Time) error {
	body, err := marshalCodexStreamChunk(state.id, state.model, state.created, map[string]any{
		"tool_calls": []any{call},
	}, nil)
	if err != nil {
		summary.ErrorClass = "upstream_stream_invalid"
		summary.CompletionStatus = "upstream_invalid"
		return err
	}
	if err := sink.WriteEvent(ctx, ChatStreamEvent{Data: body}); err != nil {
		summary.ErrorClass = "client_disconnected"
		summary.CompletionStatus = "client_disconnected"
		return err
	}
	summary.Started = true
	summary.ChunkCount++
	if summary.TimeToFirstTokenMS == 0 {
		summary.TimeToFirstTokenMS = time.Since(start).Milliseconds()
	}
	return nil
}

func (state *codexStreamState) codexToolCall(key string) *codexStreamToolCall {
	if key == "" || state.toolCalls == nil {
		return nil
	}
	return state.toolCalls[key]
}

func (state *codexStreamState) aggregateBytes() int {
	total := state.outputTextDone.Len() + state.outputItemDone.Len()
	for _, call := range state.toolCalls {
		total += call.Arguments.Len()
	}
	return total
}

func codexToolCallKey(itemID, callID string) string {
	if itemID != "" {
		return itemID
	}
	return callID
}

func codexToolCallArguments(call map[string]any) string {
	function, ok := call["function"].(map[string]any)
	if !ok {
		return ""
	}
	args, _ := function["arguments"].(string)
	return args
}

func writeCodexFinishChunk(ctx context.Context, sink ChatStreamSink, summary *ChatStreamSummary, state *codexStreamState, start time.Time) error {
	finish := "stop"
	if state.sawToolCall {
		finish = "tool_calls"
	}
	body, err := marshalCodexStreamChunk(state.id, state.model, state.created, map[string]any{}, finish)
	if err != nil {
		summary.ErrorClass = "upstream_stream_invalid"
		summary.CompletionStatus = "upstream_invalid"
		return err
	}
	if err := sink.WriteEvent(ctx, ChatStreamEvent{Data: body}); err != nil {
		summary.ErrorClass = "client_disconnected"
		summary.CompletionStatus = "client_disconnected"
		return err
	}
	summary.Started = true
	summary.ChunkCount++
	if state.hasUsage && summary.TimeToFirstTokenMS > 0 {
		elapsedSeconds := time.Since(start).Seconds()
		if elapsedSeconds > 0 {
			summary.OutputTokensPerSecond = float64(state.usage.CompletionTokens) / elapsedSeconds
		}
	}
	return nil
}

func writeCodexUsageChunk(ctx context.Context, sink ChatStreamSink, summary *ChatStreamSummary, state *codexStreamState) error {
	body, err := marshalCodexUsageChunk(state.id, state.model, state.created, state.usage)
	if err != nil {
		summary.ErrorClass = "upstream_stream_invalid"
		summary.CompletionStatus = "upstream_invalid"
		return err
	}
	if err := sink.WriteEvent(ctx, ChatStreamEvent{Data: body}); err != nil {
		summary.ErrorClass = "client_disconnected"
		summary.CompletionStatus = "client_disconnected"
		return err
	}
	summary.Started = true
	summary.ChunkCount++
	return nil
}

func marshalCodexStreamChunk(id, model string, created int64, delta map[string]any, finish any) ([]byte, error) {
	return json.Marshal(map[string]any{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": created,
		"model":   model,
		"choices": []any{
			map[string]any{
				"index":         0,
				"delta":         delta,
				"finish_reason": finish,
			},
		},
	})
}

func marshalCodexUsageChunk(id, model string, created int64, usage openai.Usage) ([]byte, error) {
	usageBody := map[string]any{
		"prompt_tokens":     usage.PromptTokens,
		"completion_tokens": usage.CompletionTokens,
		"total_tokens":      usage.TotalTokens,
	}
	if usage.CachedTokens != 0 {
		usageBody["prompt_tokens_details"] = map[string]any{"cached_tokens": usage.CachedTokens}
	}
	if usage.ReasoningTokens != 0 {
		usageBody["completion_tokens_details"] = map[string]any{"reasoning_tokens": usage.ReasoningTokens}
	}
	return json.Marshal(map[string]any{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": created,
		"model":   model,
		"choices": []any{},
		"usage":   usageBody,
	})
}

func (a HTTPChatAdapter) maxCodexAggregateBytes() int {
	if a.MaxCodexAggregateBytes > 0 {
		return a.MaxCodexAggregateBytes
	}
	return MaxCodexAggregateAssistantBytes
}
