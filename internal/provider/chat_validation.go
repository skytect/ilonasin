package provider

import (
	"errors"
	"fmt"

	"ilonasin/internal/openai"
)

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
