# 187 Codex Responses Output Item Fidelity

## Context

Plans 091 through 099 moved Codex CLI compatibility from plain Chat
Completions toward the Responses wire shape used by `codex exec`. The current
local Responses route can relay function calls, custom tool calls, tool-search
items, and web-search items, but the in-memory output item DTO still loses some
Responses-specific identity and payload fields.

That loss is visible for hosted or client-mediated output items:

- local Responses output items are assigned synthetic IDs even when upstream
  Codex already provided stable item IDs,
- `tool_search_call` items cannot preserve the upstream item ID separately from
  the call ID,
- `web_search_call` items cannot relay the upstream `action` payload,
- the local Responses writer currently emits `call_id` for every output item
  before item-type handling, even though hosted web-search items are keyed by
  item ID and status/action.

The architecture requires provider-specific adapter behavior, strict local
shape handling, and metadata-only observability. Preserving safe structural
Responses fields in memory and on the local Responses SSE surface improves
Codex compatibility without storing prompts, completions, raw provider
payloads, raw SSE chunks, tool arguments, or tool results.

## Goal

Preserve safe Responses output item identity and web-search action fields for
Codex-provider local Responses output.

After this slice:

- `ResponsesOutputItem` can carry an upstream output item ID,
- `tool_search_call` output preserves both item ID and call ID,
- `web_search_call` output preserves item ID, status, and action payload,
- local `/responses` and `/v1/responses` SSE output uses upstream item IDs
  when present and synthetic IDs only as fallback,
- `call_id` is emitted only for item types that actually carry call IDs,
- Chat Completions output remains unchanged,
- no new storage, logging, TUI, config, routing, or credential behavior is
  introduced.

## Scope

1. Update `internal/openai/chat_response.go`.
   - Add safe in-memory fields to `ResponsesOutputItem`:
     - `ID string`
     - `Action json.RawMessage`
   - Keep this DTO internal and non-persistent.
2. Update `internal/provider/codex_responses_parse.go`.
   - Parse `action` from Codex output item event payloads.
   - Preserve `item.id` for `tool_search_call` while preserving
     `item.call_id` separately as the call ID.
   - Preserve `item.id`, `status`, and trimmed `action` for `web_search_call`.
   - Keep existing unsupported-tool guards.
   - Treat missing web-search item ID as `upstream_invalid_response`.
   - Count preserved `action` payload bytes in the existing Codex aggregate
     response-size guard.
   - Do not log or store action payload contents.
3. Update `internal/server/responses_route.go`.
   - Use `ResponsesOutputItem.ID` as the local output item ID when present.
   - Fall back to the existing synthetic ID when upstream ID is absent.
   - Emit `call_id` only for `function_call`, `tool_search_call`, and
     `custom_tool_call`.
   - Emit `action` only for `web_search_call` when present.
   - Keep the existing web-search `response.output_item.added` event behavior,
     but omit `action` from the added event and emit it on the done item.
4. Do not change:
   - local Responses input validation,
   - Chat Completions response shape,
   - streaming Chat Completions behavior,
   - provider request translation,
   - storage, logs, management snapshots, TUI rendering, config, or credential
     handling.
5. Do not add permanent tests.

## Out of Scope

- Full hosted web-search execution semantics.
- Local execution of `tool_search`, web search, shell, MCP, or custom tools.
- Persistence of Responses output items.
- Support for new Responses input item types.
- Any TUI, management, config, or SQLite changes.
- Any compatibility claim beyond preserving these safe output item fields.

## Implementation Steps

1. Add the `ID` and `Action` fields to `ResponsesOutputItem`.
2. Parse and preserve the fields in the Codex non-stream Responses parser.
3. Adjust local Responses SSE output item rendering.
4. Run `gofmt`.
5. Review the diff before smoke checks.
6. Run compile, vet, direct `serve` and `manage` smokes, source/privacy guards,
   a temporary focused parser/SSE output check, and whitespace checks.

## Smoke Checks

Run:

```sh
set -euo pipefail
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
tmp="$(mktemp -d)"
pid=""
cleanup() {
  if [ -n "$pid" ]; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
  rm -f internal/provider/codex_output_items_tmp_test.go
  rm -f internal/server/responses_output_items_tmp_test.go
  rm -rf "$tmp" "$tmpbin"
}
trap cleanup EXIT
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
cat > internal/provider/codex_output_items_tmp_test.go <<'EOF'
package provider

import (
  "context"
  "io"
  "strings"
  "testing"
)

func TestCodexOutputItemFidelitySmoke(t *testing.T) {
  adapter := NewHTTPChatAdapter(nil)
  body := io.NopCloser(strings.NewReader(strings.Join([]string{
    `data: {"type":"response.output_item.done","item":{"id":"ts_1","type":"tool_search_call","call_id":"call_1","execution":"client","arguments":{"query":"safe"},"tools":[{"type":"web_search"}]}}`,
    ``,
    `data: {"type":"response.output_item.done","item":{"id":"ws_1","type":"web_search_call","status":"completed","action":{"query":"safe"}}}`,
    ``,
    `data: {"type":"response.completed","response":{"id":"resp_1","usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`,
    ``,
  }, "\n")))
  result, err := adapter.readCodexResponses(context.Background(), body)
  if err != nil {
    t.Fatal(err)
  }
  if len(result.OutputItems) != 2 {
    t.Fatalf("got %d output items", len(result.OutputItems))
  }
  if result.OutputItems[0].ID != "ts_1" || result.OutputItems[0].CallID != "call_1" {
    t.Fatalf("tool search identity not preserved: %#v", result.OutputItems[0])
  }
  if result.OutputItems[1].ID != "ws_1" || result.OutputItems[1].CallID != "" || len(result.OutputItems[1].Action) == 0 {
    t.Fatalf("web search item not preserved: %#v", result.OutputItems[1])
  }
  if result.OutputItems[1].Action[0] == ' ' {
    t.Fatalf("web search action was not trimmed: %q", string(result.OutputItems[1].Action))
  }
}
EOF
go test ./internal/provider -run TestCodexOutputItemFidelitySmoke -count=1
rm -f internal/provider/codex_output_items_tmp_test.go
cat > internal/server/responses_output_items_tmp_test.go <<'EOF'
package server

import (
  "encoding/json"
  "testing"

  "ilonasin/internal/openai"
)

func TestResponsesOutputItemSSEFidelitySmoke(t *testing.T) {
  action := json.RawMessage(`{"query":"safe"}`)
  web, err := responseOutputItem("resp_1", 0, openai.ResponsesOutputItem{
    ID:     "ws_1",
    Type:   "web_search_call",
    Status: "completed",
    Action: action,
  })
  if err != nil {
    t.Fatal(err)
  }
  if web["id"] != "ws_1" {
    t.Fatalf("web search id not preserved: %#v", web)
  }
  if _, ok := web["call_id"]; ok {
    t.Fatalf("web search emitted call_id: %#v", web)
  }
  if string(web["action"].(json.RawMessage)) != string(action) {
    t.Fatalf("web search action not preserved: %#v", web)
  }
  added := map[string]any{}
  for key, value := range web {
    added[key] = value
  }
  delete(added, "action")
  if _, ok := added["action"]; ok {
    t.Fatalf("web search added item includes action: %#v", added)
  }
  tool, err := responseOutputItem("resp_1", 1, openai.ResponsesOutputItem{
    ID:        "ts_1",
    Type:      "tool_search_call",
    CallID:    "call_1",
    Execution: "client",
  })
  if err != nil {
    t.Fatal(err)
  }
  if tool["id"] != "ts_1" || tool["call_id"] != "call_1" {
    t.Fatalf("tool search identity not preserved: %#v", tool)
  }
}
EOF
go test ./internal/server -run TestResponsesOutputItemSSEFidelitySmoke -count=1
rm -f internal/server/responses_output_items_tmp_test.go
cfg="$tmp/config.toml"
cat >"$cfg" <<'EOF'
[server]
bind = "127.0.0.1:0"
[providers.codex]
type = "codex"
[providers.deepseek]
type = "deepseek"
[providers.openrouter]
type = "openrouter"
EOF
ILONASIN_HOME="$tmp/home" "$tmpbin/ilonasin" serve --config "$cfg" >"$tmp/serve.log" 2>&1 &
pid="$!"
for _ in $(seq 1 80); do
  sock="$(find "$tmp/home/run" -type s -name 'manage-*.sock' -print 2>/dev/null | head -n 1 || true)"
  if [ -n "$sock" ] &&
    curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/snapshot >/dev/null &&
    curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/subscription-usage >/dev/null; then
    break
  fi
  sleep 0.1
done
if [ -z "${sock:-}" ]; then
  echo "management socket not found"
  cat "$tmp/serve.log"
  exit 1
fi
set +e
printf 'q' | timeout 3s script -q -e -c \
  "sh -c 'stty cols 100 rows 40; exec env ILONASIN_HOME=\"$tmp/home\" \"$tmpbin/ilonasin\" manage --config \"$cfg\"'" \
  "$tmp/manage.typescript" >/dev/null
manage_status="$?"
set -e
if [ "$manage_status" -ne 0 ] && [ "$manage_status" -ne 124 ]; then
  cat "$tmp/manage.typescript"
  exit "$manage_status"
fi
rg -q 'overview' "$tmp/manage.typescript"
if rg -n 'raw provider payload|raw SSE|Authorization:|Bearer sk|account_id' internal/openai internal/provider internal/server; then
  exit 1
fi
git diff --check
```

## Acceptance

- Codex non-stream Responses parsing preserves upstream output item IDs for
  `tool_search_call` and `web_search_call`.
- Codex non-stream Responses parsing preserves distinct `tool_search_call`
  item ID and call ID values.
- Local Responses SSE output uses preserved IDs when present.
- Web-search `action` is relayed only on the final done item.
- Preserved web-search action payloads count against the existing aggregate
  response-size guard.
- A focused smoke proves preserved IDs, omitted web-search `call_id`, and
  final-only `action` behavior.
- Chat Completions behavior is unchanged.
- Compile, vet, direct `serve`, direct `manage`, source/privacy guards, and
  whitespace checks pass.
- Existing unrelated files are not staged or committed.
