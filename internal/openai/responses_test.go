package openai

import (
	"strings"
	"testing"
)

func TestDecodeResponsesAllowsInstructionMessageBeforeFunctionOutput(t *testing.T) {
	body := `{
		"model":"pragnition-codex/gpt-5.5",
		"stream":true,
		"input":[
			{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{}"},
			{"type":"message","role":"developer","content":[{"type":"input_text","text":"keep policy active"}]},
			{"type":"function_call_output","call_id":"call_1","output":"done"},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"continue"}]}
		]
	}`
	if _, err := DecodeResponses(strings.NewReader(body)); err != nil {
		t.Fatalf("DecodeResponses returned error: %v", err)
	}
}

func TestDecodeResponsesRejectsUserMessageBeforeFunctionOutput(t *testing.T) {
	body := `{
		"model":"pragnition-codex/gpt-5.5",
		"stream":true,
		"input":[
			{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{}"},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"too early"}]},
			{"type":"function_call_output","call_id":"call_1","output":"done"}
		]
	}`
	_, err := DecodeResponses(strings.NewReader(body))
	if err == nil {
		t.Fatal("DecodeResponses succeeded, want ordering error")
	}
	if !strings.Contains(err.Error(), "cannot appear before function_call_output") {
		t.Fatalf("DecodeResponses error = %v, want function_call_output ordering error", err)
	}
}
