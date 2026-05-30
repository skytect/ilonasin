package metadata

import "time"

type Request struct {
	StartedAt                 time.Time
	ClientTokenID             int64
	RequestedProviderInstance string
	RequestedModel            string
	ResolvedProviderInstance  string
	ResolvedModel             string
	HTTPStatus                int
	ErrorClass                string
	RetryCount                int
	FallbackCount             int
	PromptTokens              int
	CompletionTokens          int
	TotalTokens               int
	ReasoningTokens           int
	TotalLatencyMS            int64
}
