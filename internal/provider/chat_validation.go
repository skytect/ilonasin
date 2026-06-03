package provider

import (
	"errors"
	"fmt"

	"ilonasin/internal/openai"
)

type chatValidationPolicy struct {
	providerType                string
	unsupported                 []string
	unsupportedOpenRouterOnly   bool
	rejectMultimodal            bool
	allowJSONSchemaFormat       bool
	validateCodexServiceTier    bool
	validateCodexToolChoice     bool
	validateCodexToolTranscript bool
}

func chatValidationPolicyForInstance(instance Instance) (chatValidationPolicy, bool) {
	switch instance.Type {
	case "deepseek":
		return chatValidationPolicy{
			providerType:              "deepseek",
			unsupported:               []string{"presence_penalty", "frequency_penalty", "parallel_tool_calls", "prediction", "user"},
			unsupportedOpenRouterOnly: true,
			rejectMultimodal:          true,
		}, true
	case "openrouter":
		return chatValidationPolicy{
			providerType:          "openrouter",
			allowJSONSchemaFormat: true,
		}, true
	case "codex":
		codexUnsupportedOpenRouterFields := []string{"top_k", "min_p", "top_a", "repetition_penalty", "seed", "logit_bias", "session_id", "metadata"}
		unsupported := []string{"parallel_tool_calls", "prediction", "user", "logprobs", "top_logprobs", "max_tokens", "max_completion_tokens", "temperature", "top_p", "presence_penalty", "frequency_penalty", "stop", "response_format"}
		unsupported = append(unsupported, codexUnsupportedOpenRouterFields...)
		return chatValidationPolicy{
			providerType:                "codex",
			unsupported:                 unsupported,
			validateCodexServiceTier:    true,
			validateCodexToolChoice:     true,
			validateCodexToolTranscript: true,
		}, true
	default:
		return chatValidationPolicy{}, false
	}
}

func (a HTTPChatAdapter) ValidateChatRequest(instance Instance, req openai.ChatCompletionRequest) error {
	policy, ok := chatValidationPolicyForInstance(instance)
	if !ok {
		return fmt.Errorf("provider type %q does not support chat validation", instance.Type)
	}
	if err := rejectPresentFields(req, policy.unsupported...); err != nil {
		return err
	}
	if policy.unsupportedOpenRouterOnly {
		openRouterOnlyFields := []string{"top_k", "min_p", "top_a", "repetition_penalty", "seed", "logit_bias", "service_tier", "session_id", "metadata"}
		if err := rejectPresentFields(req, openRouterOnlyFields...); err != nil {
			return err
		}
	}
	if policy.rejectMultimodal && hasContentArrays(req) {
		return fmt.Errorf("multimodal content is not supported")
	}
	if err := validateChatResponseFormat(policy.allowJSONSchemaFormat, req); err != nil {
		return err
	}
	if policy.validateCodexServiceTier {
		if err := validateCodexTopLevelServiceTier(req); err != nil {
			return err
		}
	}
	if policy.validateCodexToolChoice && req.HasField("tool_choice") {
		choice, ok := req.ToolChoice.(string)
		if !ok || choice != "auto" {
			return fmt.Errorf("tool_choice is not supported")
		}
	}
	if policy.validateCodexToolTranscript {
		if err := validateCodexToolTranscript(req); err != nil {
			return err
		}
	}
	if err := rejectStrictTools(req); err != nil {
		return err
	}
	return validateProviderOptions(policy.providerType, req)
}

func validateCodexTopLevelServiceTier(req openai.ChatCompletionRequest) error {
	if req.ServiceTier == nil {
		return nil
	}
	switch *req.ServiceTier {
	case "default", "priority", "flex":
		return nil
	default:
		return fmt.Errorf("service_tier is not supported")
	}
}

func rejectStrictTools(req openai.ChatCompletionRequest) error {
	for i, tool := range req.Tools {
		function, ok := tool["function"].(map[string]any)
		if !ok {
			continue
		}
		strict, ok := function["strict"].(bool)
		if ok && strict {
			return fmt.Errorf("tools[%d].function.strict is unsupported", i)
		}
	}
	return nil
}

func validateCodexToolTranscript(req openai.ChatCompletionRequest) error {
	pending := map[string]bool{}
	seenResults := map[string]bool{}
	for i, msg := range req.Messages {
		switch msg.Role {
		case "assistant":
			for _, call := range msg.ToolCalls {
				id, _ := call["id"].(string)
				if id != "" {
					pending[id] = true
				}
			}
		case "tool":
			if !pending[msg.ToolCallID] || seenResults[msg.ToolCallID] {
				return fmt.Errorf("messages[%d].tool_call_id does not match a prior assistant tool call", i)
			}
			seenResults[msg.ToolCallID] = true
			delete(pending, msg.ToolCallID)
		}
	}
	return nil
}

func hasContentArrays(req openai.ChatCompletionRequest) bool {
	for _, msg := range req.Messages {
		if openai.MessageContentIsArray(msg) {
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

func validateChatResponseFormat(allowJSONSchema bool, req openai.ChatCompletionRequest) error {
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
		if !allowJSONSchema {
			return errors.New("response_format.type is unsupported")
		}
		return validateOpenRouterJSONSchemaResponseFormat(req.ResponseFormat)
	default:
		return errors.New("response_format.type is unsupported")
	}
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
	case "codex":
		return validateCodexOptions(raw)
	default:
		return fmt.Errorf("provider_options is not supported for %s", providerType)
	}
}
