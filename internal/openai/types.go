package openai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

type ChatCompletionRequest struct {
	Model               string                 `json:"model"`
	Messages            []Message              `json:"messages"`
	Stream              bool                   `json:"stream,omitempty"`
	MaxTokens           *int                   `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int                   `json:"max_completion_tokens,omitempty"`
	Temperature         *float64               `json:"temperature,omitempty"`
	TopP                *float64               `json:"top_p,omitempty"`
	TopK                *json.Number           `json:"top_k,omitempty"`
	MinP                *json.Number           `json:"min_p,omitempty"`
	TopA                *json.Number           `json:"top_a,omitempty"`
	RepetitionPenalty   *json.Number           `json:"repetition_penalty,omitempty"`
	Seed                *json.Number           `json:"seed,omitempty"`
	PresencePenalty     *float64               `json:"presence_penalty,omitempty"`
	FrequencyPenalty    *float64               `json:"frequency_penalty,omitempty"`
	Stop                any                    `json:"stop,omitempty"`
	StreamOptions       map[string]any         `json:"stream_options,omitempty"`
	ResponseFormat      map[string]any         `json:"response_format,omitempty"`
	Tools               []map[string]any       `json:"tools,omitempty"`
	ToolChoice          any                    `json:"tool_choice,omitempty"`
	ParallelToolCalls   *bool                  `json:"parallel_tool_calls,omitempty"`
	Prediction          map[string]any         `json:"prediction,omitempty"`
	User                *string                `json:"user,omitempty"`
	ServiceTier         *string                `json:"service_tier,omitempty"`
	SessionID           *string                `json:"session_id,omitempty"`
	Metadata            map[string]string      `json:"metadata,omitempty"`
	Logprobs            *bool                  `json:"logprobs,omitempty"`
	TopLogprobs         *int                   `json:"top_logprobs,omitempty"`
	LogitBias           map[string]json.Number `json:"logit_bias,omitempty"`
	ReasoningOptions    map[string]any         `json:"provider_options,omitempty"`
	AffinityKey         string                 `json:"-"`
	PresentFields       map[string]bool        `json:"-"`
	CodexResponsesInput []json.RawMessage      `json:"-"`
	CodexResponsesTools []json.RawMessage      `json:"-"`
	CodexInstructions   string                 `json:"-"`
}

type Message struct {
	Role       string           `json:"role"`
	Content    json.RawMessage  `json:"content"`
	Name       string           `json:"name,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []map[string]any `json:"tool_calls,omitempty"`
}

type ErrorEnvelope struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
}

func DecodeChatCompletion(r io.Reader) (ChatCompletionRequest, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	var raw map[string]json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return ChatCompletionRequest{}, fmt.Errorf("invalid request JSON: %w", err)
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return ChatCompletionRequest{}, errors.New("request body must contain a single JSON object")
	}
	if err := validateTopLevelKeys(raw); err != nil {
		return ChatCompletionRequest{}, err
	}
	if err := validateRawMessages(raw["messages"]); err != nil {
		return ChatCompletionRequest{}, err
	}
	if err := validateRawStreamOptions(raw); err != nil {
		return ChatCompletionRequest{}, err
	}
	if err := validateRawPenalties(raw); err != nil {
		return ChatCompletionRequest{}, err
	}
	if err := validateRawAdvancedSampling(raw); err != nil {
		return ChatCompletionRequest{}, err
	}
	if err := validateRawLogprobs(raw); err != nil {
		return ChatCompletionRequest{}, err
	}
	if err := validateRawLogitBias(raw); err != nil {
		return ChatCompletionRequest{}, err
	}
	if err := validateRawParallelToolCalls(raw); err != nil {
		return ChatCompletionRequest{}, err
	}
	if err := validateRawPrediction(raw); err != nil {
		return ChatCompletionRequest{}, err
	}
	if err := validateRawUser(raw); err != nil {
		return ChatCompletionRequest{}, err
	}
	if err := validateRawServiceTier(raw); err != nil {
		return ChatCompletionRequest{}, err
	}
	if err := validateRawSessionID(raw); err != nil {
		return ChatCompletionRequest{}, err
	}
	if err := validateRawMetadata(raw); err != nil {
		return ChatCompletionRequest{}, err
	}
	toolNames, hasTools, err := validateRawTools(raw)
	if err != nil {
		return ChatCompletionRequest{}, err
	}
	if err := validateRawToolChoice(raw, toolNames, hasTools); err != nil {
		return ChatCompletionRequest{}, err
	}
	var req ChatCompletionRequest
	body, err := json.Marshal(raw)
	if err != nil {
		return ChatCompletionRequest{}, err
	}
	typed := json.NewDecoder(bytes.NewReader(body))
	typed.UseNumber()
	if err := typed.Decode(&req); err != nil {
		return ChatCompletionRequest{}, fmt.Errorf("invalid request JSON: %w", err)
	}
	if typed.Decode(&struct{}{}) != io.EOF {
		return ChatCompletionRequest{}, errors.New("request body must contain a single JSON object")
	}
	req.PresentFields = map[string]bool{}
	for key := range raw {
		req.PresentFields[key] = true
	}
	req.AffinityKey = chatAffinityKey(req)
	return req, nil
}

func chatAffinityKey(req ChatCompletionRequest) string {
	if req.SessionID != nil {
		if value := strings.TrimSpace(*req.SessionID); safeChatAffinityValue(value) {
			return value
		}
	}
	if req.User != nil {
		if value := strings.TrimSpace(*req.User); safeChatAffinityValue(value) {
			return value
		}
	}
	return chatMetadataAffinityKey(req.Metadata)
}

func chatMetadataAffinityKey(metadata map[string]string) string {
	for _, key := range []string{"session_id", "thread_id", "conversation_id", "prompt_cache_key"} {
		value, ok := metadata[key]
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if safeChatAffinityValue(value) {
			return value
		}
	}
	return ""
}

func safeChatAffinityValue(value string) bool {
	if value == "" || utf8.RuneCountInString(value) > 256 {
		return false
	}
	if strings.HasPrefix(value, "{") {
		return false
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "eyj") && strings.Contains(lower, ".") {
		return false
	}
	for _, marker := range []string{
		"account", "acct_", "account_uuid", "device", "device_id", "bearer",
		"token", "secret", "authorization", "oauth", "sk-",
	} {
		if strings.Contains(lower, marker) {
			return false
		}
	}
	return true
}

func (r ChatCompletionRequest) HasField(key string) bool {
	return r.PresentFields != nil && r.PresentFields[key]
}

func Error(message, typ, code string) ErrorEnvelope {
	return ErrorEnvelope{Error: ErrorBody{Message: message, Type: typ, Code: code}}
}

type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	ReasoningTokens  int
	CachedTokens     int
	CacheWriteTokens int
	CostMicrounits   int64
}

func safeResolvedModelFromRaw(raw json.RawMessage) string {
	if len(raw) == 0 || isJSONNull(raw) {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return SafeResolvedModel(value)
}

func SafeResolvedModel(value string) string {
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value {
		return ""
	}
	for _, r := range value {
		if !safeResolvedModelRune(r) {
			return ""
		}
	}
	lower := strings.ToLower(value)
	for _, marker := range []string{
		"bearer", "sk-", "iln_", "oauth", "token", "secret", "authorization",
		"raw", "payload", "prompt", "completion", "body", "account", "acct_",
		"request_id", "request-id", "request id", "requestid", "req_", "balance", "credit",
	} {
		if strings.Contains(lower, marker) {
			return ""
		}
	}
	if strings.HasPrefix(lower, "eyj") && strings.Contains(lower, ".") {
		return ""
	}
	return value
}

func safeResolvedModelRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return true
	case r >= 'A' && r <= 'Z':
		return true
	case r >= '0' && r <= '9':
		return true
	case r == '.' || r == '_' || r == ':' || r == '/' || r == '+' || r == '-':
		return true
	default:
		return false
	}
}

func parseJSONNumberToken(field string, raw json.RawMessage, rangeLabel string) (json.Number, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || isJSONNull(trimmed) {
		return "", fmt.Errorf("%s must be %s", field, rangeLabel)
	}
	if (trimmed[0] < '0' || trimmed[0] > '9') && trimmed[0] != '-' {
		return "", fmt.Errorf("%s must be %s", field, rangeLabel)
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	var num json.Number
	if err := dec.Decode(&num); err != nil {
		return "", fmt.Errorf("%s must be %s", field, rangeLabel)
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return "", fmt.Errorf("%s must be %s", field, rangeLabel)
	}
	return num, nil
}

func validateRawAssistantToolCalls(raw json.RawMessage, messageIndex int) error {
	if isJSONNull(raw) {
		return fmt.Errorf("messages[%d].tool_calls must be an array", messageIndex)
	}
	var calls []json.RawMessage
	if err := json.Unmarshal(raw, &calls); err != nil {
		return fmt.Errorf("messages[%d].tool_calls must be an array", messageIndex)
	}
	if len(calls) == 0 {
		return fmt.Errorf("messages[%d].tool_calls must not be empty", messageIndex)
	}
	for i, rawCall := range calls {
		if err := validateRawAssistantToolCall(rawCall, messageIndex, i); err != nil {
			return err
		}
	}
	return nil
}

func validateRawAssistantToolCall(raw json.RawMessage, messageIndex, callIndex int) error {
	var call map[string]json.RawMessage
	if err := json.Unmarshal(raw, &call); err != nil {
		return fmt.Errorf("messages[%d].tool_calls[%d] must be an object", messageIndex, callIndex)
	}
	if key, ok := firstUnsupportedRawField(call, "id", "type", "function"); ok {
		return fmt.Errorf("messages[%d].tool_calls[%d].%s is unsupported", messageIndex, callIndex, key)
	}
	id, err := requiredRawString(call["id"], fmt.Sprintf("messages[%d].tool_calls[%d].id", messageIndex, callIndex))
	if err != nil {
		return err
	}
	if id == "" {
		return fmt.Errorf("messages[%d].tool_calls[%d].id is required", messageIndex, callIndex)
	}
	if err := requireRawStringValue(call["type"], "function", fmt.Sprintf("messages[%d].tool_calls[%d].type", messageIndex, callIndex)); err != nil {
		return err
	}
	rawFunction, ok := call["function"]
	if !ok || isJSONNull(rawFunction) {
		return fmt.Errorf("messages[%d].tool_calls[%d].function is required", messageIndex, callIndex)
	}
	var function map[string]json.RawMessage
	if err := json.Unmarshal(rawFunction, &function); err != nil {
		return fmt.Errorf("messages[%d].tool_calls[%d].function must be an object", messageIndex, callIndex)
	}
	if key, ok := firstUnsupportedRawField(function, "name", "arguments"); ok {
		return fmt.Errorf("messages[%d].tool_calls[%d].function.%s is unsupported", messageIndex, callIndex, key)
	}
	name, err := requiredRawString(function["name"], fmt.Sprintf("messages[%d].tool_calls[%d].function.name", messageIndex, callIndex))
	if err != nil {
		return err
	}
	if !isFunctionName(name) {
		return fmt.Errorf("messages[%d].tool_calls[%d].function.name is invalid", messageIndex, callIndex)
	}
	if _, err := requiredRawString(function["arguments"], fmt.Sprintf("messages[%d].tool_calls[%d].function.arguments", messageIndex, callIndex)); err != nil {
		return err
	}
	return nil
}

func isJSONString(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	return len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"'
}

func rawJSONStringValue(raw json.RawMessage) (string, bool) {
	if !isJSONString(raw) {
		return "", false
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	return value, true
}

func requiredRawString(raw json.RawMessage, field string) (string, error) {
	value, ok := rawJSONStringValue(raw)
	if !ok {
		return "", fmt.Errorf("%s must be a string", field)
	}
	return value, nil
}

func requireRawStringValue(raw json.RawMessage, want, field string) error {
	value, err := requiredRawString(raw, field)
	if err != nil {
		return err
	}
	if value != want {
		return fmt.Errorf("%s is unsupported", field)
	}
	return nil
}

func isFunctionName(value string) bool {
	if value == "" || len(value) > 64 {
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

func isJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func positiveInt(value int) int {
	if value > 0 {
		return value
	}
	return 0
}
