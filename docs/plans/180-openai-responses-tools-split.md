# 180 OpenAI Responses Tools Split

## Context

`internal/openai/responses.go` owns the local Responses API contract. It now
mixes envelope decoding, input transcript parsing, function/custom/tool-search
transcript validation, tool declaration conversion, Responses-to-Chat
translation, and small JSON helpers in one file.

`docs/ilonasin-architecture.md` expects the OpenAI-compatible API boundary to
stay modular and provider-neutral where possible. The Responses tool
declaration path is a coherent piece of the contract: it parses client tool
definitions, validates Codex-native tool declarations, and converts
representable tools into Chat Completions function tools.

## Goal

Move local Responses tool-declaration parsing and conversion into
`internal/openai/responses_tools.go` without changing behavior.

After this slice:

- `responses.go` keeps Responses request DTOs, envelope decoding, input item
  parsing, transcript validation, input-to-chat conversion, and shared helpers.
- `responses_tools.go` owns Responses `tools` parsing, Responses tool
  declaration conversion to Chat Completions tools, Codex Responses tool
  validation, and the local UseNumber object decoder used by that path.
  It intentionally uses same-package OpenAI contract helpers such as
  `isJSONNull`, `isFunctionName`, and raw JSON string/object validators.

## Scope

1. Add `internal/openai/responses_tools.go`.
2. Move these declarations from `responses.go`:
   - `parseResponsesTools`
   - `responsesToolsToChatTools`
   - `validateCodexResponsesTool`
   - `decodeJSONObjectUseNumber`
3. Keep in `responses.go`:
   - `ResponsesRequest`
   - `ResponseInputItem`
   - `ResponseContentItem`
   - `DecodeResponses`
   - Responses top-level/stateless validation
   - input item and transcript parsing/validation
   - `validateResponsesInclude`
   - `(ResponsesRequest).ToChatCompletionRequest`
   - input-to-chat conversion helpers
   - optional raw helper functions and `mustRawJSONString`
4. Keep privacy behavior unchanged.
   - Tool names, schemas, arguments, and outputs remain transient request data.
   - Do not log, store, or render tool declarations or tool payloads.
5. Run `gofmt`.
6. Do not change public behavior, JSON shapes, errors, validation,
   provider routing, server routes, storage, management, TUI, or config.
7. Do not add permanent tests.

## Out of Scope

- Extending Responses tool support.
- Splitting input transcript parsing.
- Changing Codex tool validation or unsupported tool behavior.
- Changing local Responses SSE output.
- Adding dependencies.

## Implementation Steps

1. Create `responses_tools.go` with the moved declarations and minimal imports.
2. Remove the moved declarations from `responses.go`.
3. Clean up imports in `responses.go`.
4. Run `gofmt`.
5. Review the diff to confirm it is relocation plus import cleanup only.
6. Run compile, vet, daemon route, direct Responses tool smoke, and manage PTY
   smokes.

## Smoke Checks

Run:

```sh
set -euo pipefail
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
smokedir="$(mktemp -d ./.tmp-responses-tools-smoke.XXXXXX)"
tmpbin="$(mktemp -d)"
tmp="$(mktemp -d)"
pid=""
cleanup() {
  rm -rf "$smokedir"
  if [ -n "$pid" ]; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
  rm -rf "$tmp" "$tmpbin"
}
trap cleanup EXIT
cat >"$smokedir/responses_tools_smoke.go" <<'EOF'
package main

import (
  "strings"

  "ilonasin/internal/openai"
)

func main() {
  raw := `{
    "model":"codex/gpt-5",
    "stream":true,
    "store":false,
    "input":[{"role":"user","content":"hi"}],
    "tools":[{"type":"function","name":"lookup","description":"lookup","parameters":{"type":"object"}}],
    "tool_choice":"auto"
  }`
  req, err := openai.DecodeResponses(strings.NewReader(raw))
  if err != nil {
    panic(err)
  }
  codexChat, err := req.ToChatCompletionRequest("codex")
  if err != nil {
    panic(err)
  }
  if len(codexChat.CodexResponsesTools) != 1 || len(codexChat.Tools) != 0 {
    panic("codex tool preservation mismatch")
  }
  namespaceRaw := strings.Replace(raw,
    `"tools":[{"type":"function","name":"lookup","description":"lookup","parameters":{"type":"object"}}]`,
    `"tools":[{"type":"namespace","name":"shell","tools":[{"type":"function","name":"run"}]}]`, 1)
  namespaceReq, err := openai.DecodeResponses(strings.NewReader(namespaceRaw))
  if err != nil {
    panic(err)
  }
  namespaceChat, err := namespaceReq.ToChatCompletionRequest("codex")
  if err != nil {
    panic(err)
  }
  if len(namespaceChat.CodexResponsesTools) != 1 || len(namespaceChat.Tools) != 0 {
    panic("codex namespace tool preservation mismatch")
  }
  hostedRaw := strings.Replace(raw,
    `"tools":[{"type":"function","name":"lookup","description":"lookup","parameters":{"type":"object"}}]`,
    `"tools":[{"type":"web_search"}]`, 1)
  hostedReq, err := openai.DecodeResponses(strings.NewReader(hostedRaw))
  if err != nil {
    panic(err)
  }
  hostedChat, err := hostedReq.ToChatCompletionRequest("codex")
  if err != nil {
    panic(err)
  }
  if len(hostedChat.CodexResponsesTools) != 1 || len(hostedChat.Tools) != 0 {
    panic("codex hosted tool preservation mismatch")
  }
  genericChat, err := req.ToChatCompletionRequest("openrouter")
  if err != nil {
    panic(err)
  }
  if len(genericChat.Tools) != 1 || genericChat.ToolChoice != "auto" {
    panic("generic function tool conversion mismatch")
  }
  hostedGeneric, err := hostedReq.ToChatCompletionRequest("openrouter")
  if err != nil {
    panic(err)
  }
  if len(hostedGeneric.Tools) != 0 {
    panic("generic hosted tool filtering mismatch")
  }
  bad := strings.Replace(raw, `"name":"lookup"`, `"name":"bad name"`, 1)
  parsed, err := openai.DecodeResponses(strings.NewReader(bad))
  if err == nil {
    _, err = parsed.ToChatCompletionRequest("openrouter")
  }
  if err == nil {
    panic("invalid tool name accepted")
  }
}
EOF
go run "$smokedir/responses_tools_smoke.go"
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
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
curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/snapshot >/dev/null
curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/subscription-usage >/dev/null
set +e
printf '\tq' | timeout 3s script -q -e -c \
  "env ILONASIN_HOME='$tmp/home' '$tmpbin/ilonasin' manage --config '$cfg'" \
  "$tmp/manage.typescript" >/dev/null
manage_status="$?"
set -e
if [ "$manage_status" -ne 0 ] && [ "$manage_status" -ne 124 ]; then
  cat "$tmp/manage.typescript" 2>/dev/null || true
  exit "$manage_status"
fi
git diff --check
tool_declarations="^func parseResponsesTools|^func responsesToolsToChatTools|^func validateCodexResponsesTool|^func decodeJSONObjectUseNumber"
rg -n "$tool_declarations" internal/openai/responses_tools.go
if rg -n "$tool_declarations" internal/openai/responses.go; then
  echo "responses tool declarations remain in responses.go"
  exit 1
fi
kept_declarations="^type ResponsesRequest|^type ResponseInputItem|^type ResponseContentItem|^func DecodeResponses|^func validateResponsesTopLevelKeys|^func validateResponsesStatelessFields|^func parseResponsesInput|^func rawResponsesInputItems|^func parseResponsesInputItem|^func parseResponsesMessageItem|^func parseResponsesFunctionCallItem|^func parseResponsesFunctionCallOutputItem|^func parseResponsesToolSearchCallItem|^func parseResponsesToolSearchOutputItem|^func parseResponsesCustomToolCallItem|^func parseResponsesCustomToolCallOutputItem|^func parseResponsesOutput|^func validateResponsesToolTranscript|^func responsesOutputMatchesCall|^func validateResponsesInclude|^func \\(r ResponsesRequest\\) ToChatCompletionRequest|^func codexResponsesInputAndInstructions|^func responsesInputToChatMessages|^func optionalRawBool|^func mustRawJSONString"
rg -n "$kept_declarations" internal/openai/responses.go
if rg -n '"(log/slog|ilonasin/internal/(storage|management|tui|server))"' internal/openai/responses_tools.go; then
  echo "responses_tools.go imports a forbidden boundary"
  exit 1
fi
```

## Acceptance

- Responses tool declaration helpers compile from `responses_tools.go`.
- `responses.go` no longer owns those tool-specific declarations.
- Behavior is unchanged.
- Direct compile, vet, Responses tool smoke, serve, management route, manage
  PTY, source-layout, and whitespace checks pass.
- Existing unrelated dirty files are not staged or committed.
