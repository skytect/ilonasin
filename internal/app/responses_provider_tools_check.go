package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"ilonasin/internal/config"
	"ilonasin/internal/storage/sqlite"
)

const responsesProviderFilteredToolMarker = "responses-provider-filtered-tool-marker"

func exerciseLocalResponsesProviderToolCheck(ctx context.Context, cfg config.Config, base, token string, fakeUpstream *serveCheckUpstream, store *sqlite.Store) error {
	for _, tc := range []struct {
		providerID string
		path       string
	}{
		{providerID: "deepseek", path: "/chat/completions"},
		{providerID: "openrouter", path: "/api/v1/chat/completions"},
	} {
		body := []byte(fmt.Sprintf(`{"model":%q,"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"check"}]}],%s,"tool_choice":"auto","parallel_tool_calls":true,"store":false,"stream":true}`,
			tc.providerID+"/responses-tools-mixed", codexResponsesProviderMixedToolsExtra()))
		status, _, events, respBody, err := postStream(base+"/v1/responses", token, body)
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("local responses provider mixed tools provider=%s status=%d err=%v body_len=%d", tc.providerID, status, err, len(respBody))
		}
		if err := assertLocalResponsesSSE(events, respBody, "ok"); err != nil {
			return err
		}
		if !fakeUpstream.sawExpected(tc.path, "responses-tools-mixed") {
			return fmt.Errorf("local responses provider mixed tools did not reach provider=%s", tc.providerID)
		}
	}

	allFilteredBody := []byte(fmt.Sprintf(`{"model":%q,"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"check"}]}],%s,"tool_choice":"auto","parallel_tool_calls":true,"store":false,"stream":true}`,
		"deepseek/responses-tools-all-filtered", codexResponsesProviderAllFilteredToolsExtra()))
	status, _, events, respBody, err := postStream(base+"/v1/responses", token, allFilteredBody)
	if err != nil || status != http.StatusOK {
		return fmt.Errorf("local responses provider all-filtered tools status=%d err=%v body_len=%d", status, err, len(respBody))
	}
	if err := assertLocalResponsesSSE(events, respBody, "ok"); err != nil {
		return err
	}
	if !fakeUpstream.sawExpected("/chat/completions", "responses-tools-all-filtered") {
		return fmt.Errorf("local responses provider all-filtered tools did not reach upstream")
	}

	duplicateBody := []byte(fmt.Sprintf(`{"model":%q,"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"check"}]}],"tools":[{"type":"function","name":"%s","parameters":{"type":"object","properties":{}}},{"type":"function","name":"%s","parameters":{"type":"object","properties":{}}}],"tool_choice":"auto","store":false,"stream":true}`,
		"deepseek/responses-tools-duplicate", toolNameMarker, toolNameMarker))
	if status, err := postStatus(base+"/v1/responses", token, duplicateBody); err != nil || status != http.StatusBadRequest {
		return fmt.Errorf("local responses duplicate provider tools status=%d err=%v", status, err)
	}
	if fakeUpstream.sawExpected("/chat/completions", "responses-tools-duplicate") {
		return fmt.Errorf("local responses duplicate provider tools reached upstream")
	}

	for _, marker := range []string{responsesProviderFilteredToolMarker, toolNameMarker, toolDescriptionMarker, toolSchemaNumberMarker} {
		if err := assertServeCheckMarkerAbsentOutsideSecrets(ctx, store, marker); err != nil {
			return err
		}
		if err := assertLogFileDoesNotContain(ctx, cfg, marker); err != nil {
			return err
		}
	}
	return nil
}

func codexResponsesProviderMixedToolsExtra() string {
	return `"tools":[` +
		`{"type":"namespace","name":"mcp__private__","description":"` + responsesProviderFilteredToolMarker + `","tools":[{"type":"function","name":"filtered_namespace","parameters":{"type":"object","properties":{}}}]},` +
		`{"type":"tool_search","name":"filtered_search","description":"` + responsesProviderFilteredToolMarker + `"},` +
		`{"type":"custom","name":"filtered_custom","description":"` + responsesProviderFilteredToolMarker + `"},` +
		`{"type":"freeform","name":"filtered_freeform","description":"` + responsesProviderFilteredToolMarker + `"},` +
		`{"type":"function","name":"` + toolNameMarker + `","description":"deferred duplicate","parameters":{"type":"object","properties":{}},"strict":false,"defer_loading":true},` +
		`{"type":"function","name":"strict_skipped","description":"` + responsesProviderFilteredToolMarker + `","parameters":{"type":"object","properties":{}},"strict":true},` +
		`{"type":"function","name":"` + toolNameMarker + `","description":"` + toolDescriptionMarker + `","parameters":{"type":"object","properties":{"value":{"type":"number","minimum":` + toolSchemaNumberMarker + `}},"required":["value"]},"strict":false}` +
		`]`
}

func codexResponsesProviderAllFilteredToolsExtra() string {
	return `"tools":[` +
		`{"type":"namespace","name":"mcp__private__","description":"` + responsesProviderFilteredToolMarker + `","tools":[{"type":"function","name":"filtered_namespace","parameters":{"type":"object","properties":{}}}]},` +
		`{"type":"tool_search","name":"filtered_search","description":"` + responsesProviderFilteredToolMarker + `"},` +
		`{"type":"function","name":"deferred_only","description":"` + responsesProviderFilteredToolMarker + `","parameters":{"type":"object","properties":{}},"strict":false,"defer_loading":true},` +
		`{"type":"function","name":"strict_only","description":"` + responsesProviderFilteredToolMarker + `","parameters":{"type":"object","properties":{}},"strict":true}` +
		`]`
}

func validateServeCheckResponsesProviderTools(path, model string, body map[string]any) error {
	switch model {
	case "responses-tools-mixed":
		tools, ok := body["tools"].([]any)
		if !ok || len(tools) != 1 {
			return fmt.Errorf("invalid responses provider tools forwarding")
		}
		tool, ok := tools[0].(map[string]any)
		if !ok || tool["type"] != "function" || len(tool) != 2 {
			return fmt.Errorf("invalid responses provider tool wrapper")
		}
		function, ok := tool["function"].(map[string]any)
		if !ok || function["name"] != toolNameMarker || function["description"] != toolDescriptionMarker {
			return fmt.Errorf("invalid responses provider function forwarding")
		}
		if _, ok := function["strict"]; ok {
			return fmt.Errorf("responses provider strict flag was forwarded")
		}
		parameters, ok := function["parameters"].(map[string]any)
		if !ok {
			return fmt.Errorf("missing responses provider parameters")
		}
		properties, _ := parameters["properties"].(map[string]any)
		value, _ := properties["value"].(map[string]any)
		if !jsonNumberEquals(value["minimum"], toolSchemaNumberMarker) {
			return fmt.Errorf("invalid responses provider schema forwarding")
		}
		if body["tool_choice"] != "auto" {
			return fmt.Errorf("missing responses provider tool_choice")
		}
		if bytes.Contains(mustMarshalAny(body), []byte(responsesProviderFilteredToolMarker)) {
			return fmt.Errorf("filtered responses provider tool marker was forwarded")
		}
		switch path {
		case "/chat/completions":
			if _, ok := body["parallel_tool_calls"]; ok {
				return fmt.Errorf("DeepSeek received responses parallel_tool_calls")
			}
		case "/api/v1/chat/completions":
			if value, ok := body["parallel_tool_calls"].(bool); !ok || !value {
				return fmt.Errorf("OpenRouter missing responses parallel_tool_calls")
			}
		default:
			return fmt.Errorf("responses provider tools reached unsupported path")
		}
	case "responses-tools-all-filtered":
		if _, ok := body["tools"]; ok {
			return fmt.Errorf("all-filtered responses tools were forwarded")
		}
		if _, ok := body["tool_choice"]; ok {
			return fmt.Errorf("all-filtered responses tool_choice was forwarded")
		}
		if _, ok := body["parallel_tool_calls"]; ok {
			return fmt.Errorf("all-filtered DeepSeek received parallel_tool_calls")
		}
		if bytes.Contains(mustMarshalAny(body), []byte(responsesProviderFilteredToolMarker)) {
			return fmt.Errorf("all-filtered responses tool marker was forwarded")
		}
	default:
		return fmt.Errorf("unexpected responses provider tools model")
	}
	return nil
}

func mustMarshalAny(value any) []byte {
	body, _ := json.Marshal(value)
	return body
}
