# 174 Codex Responses Request Split

## Context

`docs/ilonasin-architecture.md` keeps provider adapters as the owner of
provider-specific request and response behavior. The Codex `/responses` adapter
now works, but `internal/provider/codex_responses.go` has grown into a broad
module that contains:

- non-streaming and streaming execution,
- upstream request construction,
- Codex model metadata resolution for request options,
- non-streaming SSE parsing,
- custom tool aggregation,
- streaming normalization,
- usage mapping and chunk marshaling.

The request-construction and model-metadata cluster is independent enough to
move without changing behavior. Splitting it out makes later parser and
streaming slices smaller and keeps request semantics easier to audit.

## Goal

Move Codex `/responses` request construction and model metadata helpers into
`internal/provider/codex_responses_request.go` without changing behavior.

After this slice:

- `codex_responses_request.go` owns upstream request DTOs, request marshaling,
  Codex model metadata lookup, and request option selection.
- `codex_responses.go` continues to own execution, response parsing, streaming,
  usage mapping, local IDs, and response chunk marshaling.
- no request JSON, model metadata fallback, reasoning/service-tier behavior,
  privacy behavior, route behavior, or streaming behavior changes.

## Scope

1. Create `internal/provider/codex_responses_request.go`.
2. Move these declarations from `codex_responses.go`:
   - `errCodexModelAuthFailed`
   - `codexResponsesRequest`
   - `codexResponsesModel`
   - `codexReasoning`
   - `codexTextControls`
   - `codexResponseItem`
   - `codexContentItem`
   - `marshalCodexResponsesRequest`
   - `codexUserContent`
   - `codexResponsesTools`
   - `codexFunctionCallItems`
   - `codexRequestOptions`
   - `resolveCodexResponsesModel`
   - `(codexResponsesModel).reasoningEffort`
3. Keep the following in `codex_responses.go`:
   - `MaxCodexAggregateAssistantBytes`
   - `completeCodexChat`
   - `readCodexResponses` and parse state helpers
   - custom-tool aggregation helpers
   - `localChatCompletionID`
   - `codexRequestIDs` and response headers
   - `streamCodexChat` and stream normalization helpers
   - usage parsing and chunk marshaling
   - `maxCodexAggregateBytes`
4. Preserve all imports through move-only cleanup.
5. Do not change provider behavior, storage, server routes, TUI, config, OAuth,
   logging, or metadata.
6. Do not add permanent tests.

## Out of Scope

- Splitting non-streaming parser state.
- Splitting streaming normalization.
- Changing Codex model discovery behavior.
- Changing request option validation.
- Changing local Responses compatibility.
- Changing custom tool behavior.
- Changing provider logging or privacy policy.

## Implementation Steps

1. Add `codex_responses_request.go` with the moved request/model cluster.
2. Remove the moved declarations from `codex_responses.go`.
3. Run `gofmt`.
4. Review the diff before smoke checks and confirm this is relocation-only
   apart from import cleanup.
5. Run compile, vet, daemon, TUI, and source-layout guards.

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
  rm -rf "$tmp" "$tmpbin"
}
trap cleanup EXIT
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
request_declarations="^var errCodexModelAuthFailed|^type codexResponsesRequest|^type codexResponsesModel|^type codexReasoning|^type codexTextControls|^type codexResponseItem|^type codexContentItem|^func marshalCodexResponsesRequest|^func codexUserContent|^func codexResponsesTools|^func codexFunctionCallItems|^func codexRequestOptions|^func \\(a HTTPChatAdapter\\) resolveCodexResponsesModel|^func \\(model codexResponsesModel\\) reasoningEffort"
rg -n "$request_declarations" internal/provider/codex_responses_request.go
if rg -n "$request_declarations" internal/provider/codex_responses.go; then
  echo "request/model helpers remain in codex_responses.go"
  exit 1
fi
rg -n "errCodexModelAuthFailed|marshalCodexResponsesRequest|resolveCodexResponsesModel" internal/provider/codex_responses.go
rg -n "completeCodexChat|readCodexResponses|handleCodexEvent|streamCodexChat|readCodexStream|handleCodexStreamEvent|codexUsageFromResponse|maxCodexAggregateBytes" internal/provider/codex_responses.go
```

## Acceptance

- Codex request/model helpers compile from `codex_responses_request.go`.
- `codex_responses.go` no longer owns the request/model helper cluster.
- Codex execution, parsing, streaming, and usage helpers remain in
  `codex_responses.go`.
- Direct compile, vet, serve, management route, and manage PTY smokes pass.
- The unrelated dirty `AGENTS.md` file is not staged or committed.
