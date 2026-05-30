package openai

import (
	"strings"
	"testing"
)

func TestDecodeChatCompletionRejectsUnknownField(t *testing.T) {
	_, err := DecodeChatCompletion(strings.NewReader(`{"model":"deepseek/x","messages":[],"unknown":true}`))
	if err == nil {
		t.Fatal("expected unknown field error")
	}
}

func TestChatCompletionRejectsUnsupportedResponseFormat(t *testing.T) {
	req, err := DecodeChatCompletion(strings.NewReader(`{
		"model":"openrouter/openai/gpt-5.1-chat",
		"messages":[{"role":"user","content":"{}"}],
		"response_format":{"type":"json_schema"}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := req.Validate(); err == nil {
		t.Fatal("expected unsupported response_format error")
	}
}
