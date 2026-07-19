package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"ilonasin/internal/openai"
)

var errCodexModelAuthFailed = errors.New("codex model metadata auth failed")

type codexModelDiscoveryError struct {
	class  string
	status int
	err    error
}

func (e codexModelDiscoveryError) Error() string {
	return e.err.Error()
}

func (e codexModelDiscoveryError) Unwrap() error {
	return e.err
}

func codexModelDiscoveryFailure(class string, status int, err error) error {
	if err == nil {
		err = errors.New(class)
	}
	return codexModelDiscoveryError{class: class, status: status, err: err}
}

func codexModelDiscoveryErrorClass(err error) string {
	var modelErr codexModelDiscoveryError
	if errors.As(err, &modelErr) && modelErr.class != "" {
		return modelErr.class
	}
	return "model_discovery_failed"
}

func codexModelDiscoveryErrorStatus(err error) int {
	var modelErr codexModelDiscoveryError
	if errors.As(err, &modelErr) && modelErr.status != 0 {
		return modelErr.status
	}
	return http.StatusBadGateway
}

type codexResponsesRequest struct {
	Model             string             `json:"model"`
	Instructions      string             `json:"instructions,omitempty"`
	Input             any                `json:"input"`
	Tools             any                `json:"tools"`
	ToolChoice        string             `json:"tool_choice"`
	ParallelToolCalls *bool              `json:"parallel_tool_calls,omitempty"`
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
	Verbosity string         `json:"verbosity,omitempty"`
	Format    map[string]any `json:"format,omitempty"`
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
	if req.HasField("max_tokens") {
		return nil, "", errors.New("max_tokens is unsupported for Codex chat translation; omit it because no equivalent Codex output cap is verified")
	}
	if req.HasField("max_completion_tokens") {
		return nil, "", errors.New("max_completion_tokens is unsupported for Codex chat translation; omit it because no equivalent Codex output cap is verified")
	}
	out := codexResponsesRequest{
		Model:             upstreamModel,
		Input:             []codexResponseItem{},
		Tools:             []any{},
		ToolChoice:        "auto",
		ParallelToolCalls: codexParallelToolCalls(req, model),
		// Privacy boundary: ilonasin remains stateless with Codex backends.
		// Do not ask upstream to persist prompts, completions, or response state.
		Store:          false,
		Stream:         true,
		Include:        []string{},
		PromptCacheKey: codexResponsesPromptCacheKey(req, ids),
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
	if req.HasField("provider_options") || req.ServiceTier != nil || req.ReasoningEffort != nil {
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

func codexParallelToolCalls(req openai.ChatCompletionRequest, model codexResponsesModel) *bool {
	// Codex CLI sends this Responses field, but Codex upstream rejects it.
	// Accept it at ilonasin's edge and omit it from the provider request.
	return nil
}

func codexResponsesPromptCacheKey(req openai.ChatCompletionRequest, ids codexRequestIDs) string {
	if key := strings.TrimSpace(req.CodexPromptCacheKey); key != "" {
		return key
	}
	return ids.ThreadID
}

func codexResponsesRequestShapeAttrs(req openai.ChatCompletionRequest) []slog.Attr {
	attrs := []slog.Attr{
		slog.Int("codex_input_items", len(req.CodexResponsesInput)),
		slog.Int("codex_tools", len(req.CodexResponsesTools)),
	}
	if len(req.CodexResponsesInput) == 0 {
		return attrs
	}
	shape := codexResponsesInputShape(req.CodexResponsesInput)
	attrs = append(attrs,
		slog.Int("codex_input_missing_type", shape.missingType),
		slog.Int("codex_message_items", shape.messageItems),
		slog.Int("codex_assistant_input_text_parts", shape.assistantInputTextParts),
		slog.String("codex_last_input_type_bucket", codexInputTypeBucket(shape.lastType)),
		slog.String("codex_last_input_role_bucket", codexInputRoleBucket(shape.lastRole)),
		slog.String("codex_last_content_type_bucket", codexContentTypeBucket(shape.lastContentTypes)),
	)
	return attrs
}

type codexInputShape struct {
	missingType             int
	messageItems            int
	assistantInputTextParts int
	lastType                string
	lastRole                string
	lastContentTypes        []string
}

func codexResponsesInputShape(input []json.RawMessage) codexInputShape {
	var shape codexInputShape
	for _, raw := range input {
		var item struct {
			Type    string `json:"type"`
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
			} `json:"content"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			continue
		}
		if item.Type == "" {
			shape.missingType++
		}
		if item.Type == "message" {
			shape.messageItems++
		}
		if item.Role == "assistant" {
			for _, part := range item.Content {
				if part.Type == "input_text" {
					shape.assistantInputTextParts++
				}
			}
		}
		shape.lastType = item.Type
		shape.lastRole = item.Role
		shape.lastContentTypes = uniqueCodexContentTypes(item.Content)
	}
	return shape
}

func codexInputTypeBucket(value string) string {
	switch value {
	case "":
		return "none"
	case "message", "function_call", "function_call_output", "tool_search_call", "tool_search_output", "custom_tool_call", "custom_tool_call_output":
		return value
	default:
		return "other"
	}
}

func codexInputRoleBucket(value string) string {
	switch value {
	case "":
		return "none"
	case "system", "developer", "user", "assistant", "tool":
		return value
	default:
		return "other"
	}
}

func codexContentTypeBucket(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	known := 0
	unknown := false
	for _, value := range values {
		switch value {
		case "input_text", "output_text", "text", "input_image":
			known++
		default:
			unknown = true
		}
	}
	switch {
	case unknown && known > 0:
		return "mixed"
	case unknown:
		return "other"
	case known > 1:
		return "multiple"
	default:
		return values[0]
	}
}

func uniqueCodexContentTypes(parts []struct {
	Type string `json:"type"`
}) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part.Type == "" || seen[part.Type] {
			continue
		}
		seen[part.Type] = true
		out = append(out, part.Type)
	}
	sort.Strings(out)
	return out
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
	if req.ReasoningEffort != nil {
		reasoning = &codexReasoning{Effort: model.reasoningEffort(*req.ReasoningEffort)}
	}
	if rawReasoning, ok := opts["reasoning"].(map[string]any); ok {
		next := &codexReasoning{}
		if reasoning != nil {
			*next = *reasoning
		}
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
	if format, ok := opts["format"].(map[string]any); ok {
		if textControls == nil {
			textControls = &codexTextControls{}
		}
		textControls.Format = format
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
	} else if req.ServiceTier != nil {
		switch *req.ServiceTier {
		case "default":
			serviceTier = ""
		case "priority", "flex":
			serviceTier = *req.ServiceTier
		default:
			return nil, nil, "", fmt.Errorf("service_tier is not supported")
		}
		if serviceTier != "" && !model.ServiceTiers[serviceTier] {
			return nil, nil, "", fmt.Errorf("service_tier is not supported by model")
		}
	}
	return reasoning, textControls, serviceTier, nil
}

func (a HTTPChatAdapter) resolveCodexResponsesModel(ctx context.Context, req ChatRequest, start time.Time) (codexResponsesModel, error) {
	endpoint, err := a.modelsURL(ctx, req.Instance)
	if err != nil {
		return codexResponsesModel{}, codexModelDiscoveryFailure("provider_config_error", http.StatusBadGateway, err)
	}
	modelCtx, cancel := context.WithTimeout(ctx, a.modelTimeout())
	defer cancel()
	httpReq, err := http.NewRequestWithContext(modelCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return codexResponsesModel{}, codexModelDiscoveryFailure("upstream_request_error", http.StatusBadGateway, err)
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
	a.addCodexRequestHeaders(ctx, httpReq, credential.BearerToken, credential.ChatGPTAccountID, credential.ChatGPTAccountIsFedRAMP)
	httpReq.Header.Set("Accept", "application/json")
	resp, err := a.Client.Do(httpReq)
	if err != nil {
		errorClass := classifyTransportError(err)
		status := providerStatusForError(http.StatusBadGateway, errorClass)
		logProviderHTTP(ctx, a.Logger, statusLevel(status, errorClass), "provider_http",
			slog.String("endpoint", "models"),
			slog.String("method", http.MethodGet),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", credential.ID),
			slog.Int("status", status),
			slog.Int64("duration_ms", durationMS(start)),
			slog.String("error_class", errorClass),
		)
		return codexResponsesModel{}, codexModelDiscoveryFailure(errorClass, status, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errorClass := "upstream_http_error"
		if resp.StatusCode == http.StatusUnauthorized {
			errorClass = "upstream_auth_failed"
		} else if resp.StatusCode == http.StatusTooManyRequests {
			errorClass = "rate_limit_exceeded"
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
		return codexResponsesModel{}, codexModelDiscoveryFailure(errorClass, resp.StatusCode, fmt.Errorf("codex models status %d", resp.StatusCode))
	}
	body, tooLarge, readErr := readLimitedUpstreamBody(resp.Body, MaxUpstreamModelsBodyBytes)
	a.recordUpstreamBody(req.Instance, credential.ID, "models", http.MethodGet, "upstream_output", resp.StatusCode, resp.Header.Get("Content-Type"), body, "")
	if tooLarge {
		errorClass := "upstream_body_too_large"
		logProviderHTTP(ctx, a.Logger, statusLevel(http.StatusBadGateway, errorClass), "provider_http",
			slog.String("endpoint", "models"),
			slog.String("method", http.MethodGet),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", credential.ID),
			slog.Int("status", http.StatusBadGateway),
			slog.Int("upstream_status", resp.StatusCode),
			slog.Int64("duration_ms", durationMS(start)),
			slog.String("error_class", errorClass),
		)
		return codexResponsesModel{}, codexModelDiscoveryFailure(errorClass, http.StatusBadGateway, fmt.Errorf("codex models body exceeded limit"))
	}
	if readErr != nil {
		errorClass := classifyCodexReadError(modelCtx, readErr)
		status := providerStatusForError(http.StatusBadGateway, errorClass)
		logProviderHTTP(ctx, a.Logger, statusLevel(status, errorClass), "provider_http",
			slog.String("endpoint", "models"),
			slog.String("method", http.MethodGet),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", credential.ID),
			slog.Int("status", status),
			slog.Int("upstream_status", resp.StatusCode),
			slog.Int64("duration_ms", durationMS(start)),
			slog.String("error_class", errorClass),
		)
		return codexResponsesModel{}, codexModelDiscoveryFailure(errorClass, status, readErr)
	}
	parsed, err := decodeCodexModels(body)
	if err != nil {
		errorClass := "upstream_invalid_response"
		logProviderHTTP(ctx, a.Logger, statusLevel(http.StatusBadGateway, errorClass), "provider_http",
			slog.String("endpoint", "models"),
			slog.String("method", http.MethodGet),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", credential.ID),
			slog.Int("status", http.StatusBadGateway),
			slog.Int("upstream_status", resp.StatusCode),
			slog.Int64("duration_ms", durationMS(start)),
			slog.Int("response_bytes", len(body)),
			slog.String("error_class", errorClass),
		)
		return codexResponsesModel{}, codexModelDiscoveryFailure(errorClass, http.StatusBadGateway, err)
	}
	for _, model := range parsed {
		if model.Slug.Value == req.UpstreamModel {
			serviceTiers := map[string]bool{}
			for _, tier := range model.ServiceTiers.Value {
				if tier.ID.Value != "" {
					serviceTiers[tier.ID.Value] = true
				}
			}
			reasoningEfforts := make([]string, len(model.SupportedReasoningLevels.Value))
			for i, level := range model.SupportedReasoningLevels.Value {
				reasoningEfforts[i] = string(level.Effort.Value)
			}
			return codexResponsesModel{
				BaseInstructions:          strings.TrimSpace(model.BaseInstructions.Value),
				SupportsParallelToolCalls: model.SupportsParallelToolCalls.Value,
				ServiceTiers:              serviceTiers,
				DefaultReasoningEffort:    stringValueFromWire(model.DefaultReasoningLevel),
				SupportedReasoningEfforts: reasoningEfforts,
			}, nil
		}
	}
	return codexResponsesModel{SupportsParallelToolCalls: true, ServiceTiers: map[string]bool{}}, nil
}

func stringValueFromWire(value *codexWireReasoningEffort) string {
	if value == nil {
		return ""
	}
	return string(*value)
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
