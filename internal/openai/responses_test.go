package openai

import (
	"strings"
	"testing"
)

func TestDecodeResponsesAllowsInterleavedFunctionOutput(t *testing.T) {
	body := `{
		"model":"pragnition-codex/gpt-5.5",
		"stream":true,
		"input":[
			{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{}"},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"continue"}]},
			{"type":"function_call_output","call_id":"call_1","output":"done"}
		]
	}`
	if _, err := DecodeResponses(strings.NewReader(body)); err != nil {
		t.Fatalf("DecodeResponses returned error: %v", err)
	}
}

func TestDecodeResponsesAllowsInterleavedParallelFunctionOutput(t *testing.T) {
	body := `{
		"model":"pragnition-codex/gpt-5.5",
		"stream":true,
		"input":[
			{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{}"},
			{"type":"function_call","call_id":"call_2","name":"read_file","arguments":"{}"},
			{"type":"function_call_output","call_id":"call_1","output":"done"},
			{"type":"message","role":"developer","content":[{"type":"input_text","text":"keep policy active"}]},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"continue"}]},
			{"type":"function_call_output","call_id":"call_2","output":"done"}
		]
	}`
	if _, err := DecodeResponses(strings.NewReader(body)); err != nil {
		t.Fatalf("DecodeResponses returned error: %v", err)
	}
}

func TestDecodeResponsesRejectsUnmatchedFunctionOutput(t *testing.T) {
	body := `{
		"model":"pragnition-codex/gpt-5.5",
		"stream":true,
		"input":[
			{"type":"function_call_output","call_id":"call_1","output":"done"}
		]
	}`
	_, err := DecodeResponses(strings.NewReader(body))
	if err == nil {
		t.Fatal("DecodeResponses succeeded, want unmatched call_id error")
	}
	if !strings.Contains(err.Error(), "does not match a prior function_call") {
		t.Fatalf("DecodeResponses error = %v, want unmatched call_id error", err)
	}
}

func TestDecodeResponsesRejectsMissingFunctionOutput(t *testing.T) {
	body := `{
		"model":"pragnition-codex/gpt-5.5",
		"stream":true,
		"input":[
			{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{}"}
		]
	}`
	_, err := DecodeResponses(strings.NewReader(body))
	if err == nil {
		t.Fatal("DecodeResponses succeeded, want missing output error")
	}
	if !strings.Contains(err.Error(), "is missing function_call_output") {
		t.Fatalf("DecodeResponses error = %v, want missing output error", err)
	}
}
