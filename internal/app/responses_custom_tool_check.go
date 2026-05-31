package app

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"

	"ilonasin/internal/config"
	"ilonasin/internal/storage/sqlite"
)

const (
	customToolCallIDMarker = "custom-tool-call-marker"
	customToolInputMarker  = "custom-tool-input-marker"
	customToolOutputMarker = "custom-tool-output-marker"
)

func exerciseLocalResponsesCustomToolCheck(ctx context.Context, cfg config.Config, base, token string, fakeUpstream *serveCheckUpstream, store *sqlite.Store) error {
	for _, model := range []string{"codex-custom-tool-response", "codex-custom-tool-response-delta"} {
		body := []byte(fmt.Sprintf(`{"model":"codex/%s","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"check"}]}],"store":false,"stream":true}`, model))
		status, _, events, respBody, err := postStream(base+"/v1/responses", token, body)
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("local responses custom tool %s status=%d err=%v body_len=%d", model, status, err, len(respBody))
		}
		if err := assertLocalResponsesCustomToolSSE(events, respBody); err != nil {
			return err
		}
		if !fakeUpstream.sawExpected("/responses", model) {
			return fmt.Errorf("local responses custom tool %s did not reach codex upstream", model)
		}
	}

	followupBody := []byte(fmt.Sprintf(`{"model":"codex/codex-custom-tool-followup","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"check"}]},{"id":"custom-tool-item-marker","type":"custom_tool_call","call_id":"%s","name":"apply_patch","input":"%s"},{"type":"custom_tool_call_output","call_id":"%s","name":"apply_patch","output":"%s"}],"store":false,"stream":true}`,
		customToolCallIDMarker, customToolInputMarker, customToolCallIDMarker, customToolOutputMarker))
	status, _, events, respBody, err := postStream(base+"/v1/responses", token, followupBody)
	if err != nil || status != http.StatusOK {
		return fmt.Errorf("local responses custom tool followup status=%d err=%v body_len=%d", status, err, len(respBody))
	}
	if err := assertLocalResponsesSSE(events, respBody, "codex ok"); err != nil {
		return err
	}
	if !fakeUpstream.sawExpected("/responses", "codex-custom-tool-followup") {
		return fmt.Errorf("local responses custom tool followup did not reach codex upstream")
	}

	for _, tc := range []struct {
		name  string
		model string
		body  string
		path  string
	}{
		{
			name:  "deepseek-custom-tool",
			model: "custom-tool-noncodex",
			path:  "/chat/completions",
			body: fmt.Sprintf(`{"model":"deepseek/custom-tool-noncodex","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"check"}]},{"type":"custom_tool_call","call_id":"%s","name":"apply_patch","input":"%s"},{"type":"custom_tool_call_output","call_id":"%s","output":"%s"}],"store":false,"stream":true}`,
				customToolCallIDMarker, customToolInputMarker, customToolCallIDMarker, customToolOutputMarker),
		},
		{
			name:  "openrouter-custom-tool",
			model: "custom-tool-noncodex",
			path:  "/api/v1/chat/completions",
			body: fmt.Sprintf(`{"model":"openrouter/custom-tool-noncodex","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"check"}]},{"type":"custom_tool_call","call_id":"%s","name":"apply_patch","input":"%s"},{"type":"custom_tool_call_output","call_id":"%s","output":"%s"}],"store":false,"stream":true}`,
				customToolCallIDMarker, customToolInputMarker, customToolCallIDMarker, customToolOutputMarker),
		},
		{
			name:  "structured-custom-output",
			model: "custom-tool-structured-output",
			path:  "/responses",
			body: fmt.Sprintf(`{"model":"codex/custom-tool-structured-output","input":[{"type":"custom_tool_call","call_id":"%s","name":"apply_patch","input":"%s"},{"type":"custom_tool_call_output","call_id":"%s","output":[{"type":"input_text","text":"%s"}]}],"store":false,"stream":true}`,
				customToolCallIDMarker, customToolInputMarker, customToolCallIDMarker, customToolOutputMarker),
		},
		{
			name:  "duplicate-custom-call",
			model: "custom-tool-duplicate",
			path:  "/responses",
			body: fmt.Sprintf(`{"model":"codex/custom-tool-duplicate","input":[{"type":"custom_tool_call","call_id":"%s","name":"apply_patch","input":"%s"},{"type":"custom_tool_call","call_id":"%s","name":"apply_patch","input":"%s"},{"type":"custom_tool_call_output","call_id":"%s","output":"%s"}],"store":false,"stream":true}`,
				customToolCallIDMarker, customToolInputMarker, customToolCallIDMarker, customToolInputMarker, customToolCallIDMarker, customToolOutputMarker),
		},
		{
			name:  "orphan-custom-output",
			model: "custom-tool-orphan",
			path:  "/responses",
			body: fmt.Sprintf(`{"model":"codex/custom-tool-orphan","input":[{"type":"custom_tool_call_output","call_id":"%s","output":"%s"}],"store":false,"stream":true}`,
				customToolCallIDMarker, customToolOutputMarker),
		},
		{
			name:  "missing-custom-output",
			model: "custom-tool-missing-output",
			path:  "/responses",
			body: fmt.Sprintf(`{"model":"codex/custom-tool-missing-output","input":[{"type":"custom_tool_call","call_id":"%s","name":"apply_patch","input":"%s"},{"type":"message","role":"user","content":[{"type":"input_text","text":"after"}]}],"store":false,"stream":true}`,
				customToolCallIDMarker, customToolInputMarker),
		},
	} {
		if status, err := postStatus(base+"/v1/responses", token, []byte(tc.body)); err != nil || status != http.StatusBadRequest {
			return fmt.Errorf("local responses custom tool rejected %s status=%d err=%v", tc.name, status, err)
		}
		if fakeUpstream.sawExpected(tc.path, tc.model) {
			return fmt.Errorf("local responses custom tool rejected %s reached upstream", tc.name)
		}
	}

	for _, marker := range []string{customToolInputMarker, customToolOutputMarker} {
		if err := assertServeCheckMarkerAbsentOutsideSecrets(ctx, store, marker); err != nil {
			return err
		}
		if err := assertLogFileDoesNotContain(ctx, cfg, marker); err != nil {
			return err
		}
	}
	return nil
}

func assertLocalResponsesCustomToolSSE(events []string, body []byte) error {
	if len(events) != 3 {
		return fmt.Errorf("local responses custom tool emitted %d events", len(events))
	}
	for _, typ := range []string{"response.created", "response.output_item.done", "response.completed"} {
		if !bytes.Contains(body, []byte(`"type":"`+typ+`"`)) {
			return fmt.Errorf("local responses custom tool missing %s", typ)
		}
	}
	for _, marker := range []string{`"type":"custom_tool_call"`, customToolCallIDMarker, "apply_patch", customToolInputMarker} {
		if !bytes.Contains(body, []byte(marker)) {
			return fmt.Errorf("local responses custom tool missing expected output")
		}
	}
	if bytes.Contains(body, []byte(customToolOutputMarker)) || strings.Contains(string(body), "function_call_output") {
		return fmt.Errorf("local responses custom tool leaked followup-only marker")
	}
	return nil
}

func writeCodexCustomToolCallSSE(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	_, _ = fmt.Fprintf(w, "data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"custom_tool_call\",\"call_id\":%q,\"name\":\"apply_patch\",\"input\":%q}}\n\n", customToolCallIDMarker, customToolInputMarker)
	_, _ = w.Write([]byte(`data: {"type":"response.completed","response":{"id":"raw-provider-response-id-marker","usage":{"input_tokens":3,"output_tokens":4,"total_tokens":7}}}` + "\n\n"))
}

func writeCodexCustomToolCallDeltaSSE(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	_, _ = fmt.Fprintf(w, "data: {\"type\":\"response.output_item.added\",\"item\":{\"id\":\"custom_tool_item\",\"type\":\"custom_tool_call\",\"call_id\":%q,\"name\":\"apply_patch\",\"input\":\"\",\"status\":\"in_progress\"}}\n\n", customToolCallIDMarker)
	_, _ = fmt.Fprintf(w, "data: {\"type\":\"response.custom_tool_call_input.delta\",\"item_id\":\"custom_tool_item\",\"call_id\":%q,\"delta\":%q}\n\n", customToolCallIDMarker, customToolInputMarker)
	_, _ = fmt.Fprintf(w, "data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"custom_tool_call\",\"call_id\":%q,\"name\":\"apply_patch\",\"input\":%q}}\n\n", customToolCallIDMarker, customToolInputMarker)
	_, _ = w.Write([]byte(`data: {"type":"response.completed","response":{"id":"raw-provider-response-id-marker","usage":{"input_tokens":3,"output_tokens":4,"total_tokens":7}}}` + "\n\n"))
}
