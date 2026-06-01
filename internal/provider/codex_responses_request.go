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
		slog.String("codex_last_input_type", shape.lastType),
		slog.String("codex_last_input_role", shape.lastRole),
		slog.String("codex_last_content_types", strings.Join(shape.lastContentTypes, ",")),
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
