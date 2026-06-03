package metadata

import "strings"

const (
	EndpointChatCompletions      = "chat_completions"
	EndpointResponses            = "responses"
	EndpointAnthropicMessages    = "anthropic_messages"
	EndpointAnthropicCountTokens = "anthropic_count_tokens"
)

func SafeEndpoint(value string) string {
	value = strings.TrimSpace(value)
	switch value {
	case EndpointChatCompletions, EndpointResponses, EndpointAnthropicMessages, EndpointAnthropicCountTokens:
		return value
	default:
		return ""
	}
}
