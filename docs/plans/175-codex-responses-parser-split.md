# 175 Codex Responses Parser Split

## Context

`docs/ilonasin-architecture.md` keeps provider-specific request and response
behavior inside provider adapters. The Codex `/responses` adapter still has
non-streaming execution, non-streaming SSE parsing, local ID helpers, response
headers, streaming normalization, usage mapping, and stream chunk marshaling in
one large file.

Slice 174 moved request construction and model metadata into
`internal/provider/codex_responses_request.go`. The next narrow split is the
non-streaming response parser and custom-tool aggregation. This parser is
independent of streaming except for a few shared helpers that both paths still
use.

## Goal

Move the non-streaming Codex `/responses` parser into
`internal/provider/codex_responses_parse.go` without changing behavior.

After this slice:

- `codex_responses_parse.go` owns non-streaming SSE parsing, parser state,
  event failure classification, custom-tool aggregation, and final-text
  selection.
- `codex_responses.go` continues to own non-streaming execution, local IDs,
  response headers, streaming normalization, usage mapping, shared read-error
  classification, shared tool classifiers/helpers, stream chunk marshaling, and
  aggregate byte defaults.
- Codex request construction remains in `codex_responses_request.go`.

## Scope

1. Create `internal/provider/codex_responses_parse.go`.
2. Move these declarations from `codex_responses.go`:
   - `codexResponsesResult`
   - `codexResponseParseState`
   - `codexResponseToolState`
   - `codexResponseToolCall`
   - `codexResponseCustomToolState`
   - `codexResponseCustomToolCall`
   - `(a HTTPChatAdapter) readCodexResponses`
   - `(state *codexResponseParseState) codexResponsesResult`
   - `(state *codexResponseParseState) aggregateBytes`
   - `codexEventFailure`
   - `(codexEventFailure) Error`
   - `codexEventErrorClass`
   - `codexReadErrorReason`
   - `handleCodexEvent`
   - all `codexResponseParseState` parser/custom-tool methods
   - `codexFunctionArgumentsText`
   - `codexFinalText`
3. Leave shared stream/non-stream helpers in `codex_responses.go`:
   - `classifyCodexReadError`
   - `codexToolCall`
   - `codexToolCallKey`
   - `codexToolCallArguments`
   - `codexToolEvent`
   - `unsupportedCodexToolEvent`
   - `unsupportedCodexOutputItem`
4. Preserve all imports through move-only cleanup.
5. Do not change request construction, streaming behavior, usage mapping,
   logging, storage, management routes, TUI behavior, or config.
6. Do not add permanent tests.

## Out of Scope

- Splitting streaming normalization.
- Splitting shared tool helper declarations.
- Changing parser validation or error classes.
- Changing custom tool aggregation semantics.
- Changing subscription usage, keepalive, logging, or TUI behavior.

## Implementation Steps

1. Add `codex_responses_parse.go` with the moved parser declarations.
2. Remove the moved parser declarations from `codex_responses.go`.
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
parser_declarations="^type codexResponsesResult|^type codexResponseParseState|^type codexResponseToolState|^type codexResponseToolCall|^type codexResponseCustomToolState|^type codexResponseCustomToolCall|^func \\(a HTTPChatAdapter\\) readCodexResponses|^func \\(state \\*codexResponseParseState\\) codexResponsesResult|^func \\(state \\*codexResponseParseState\\) aggregateBytes|^type codexEventFailure|^func \\(e codexEventFailure\\) Error|^func codexEventErrorClass|^func codexReadErrorReason|^func handleCodexEvent|^func \\(state \\*codexResponseParseState\\)|^func codexFunctionArgumentsText|^func codexFinalText"
rg -n "$parser_declarations" internal/provider/codex_responses_parse.go
if rg -n "$parser_declarations" internal/provider/codex_responses.go; then
  echo "parser declarations remain in codex_responses.go"
  exit 1
fi
rg -n "readCodexResponses|codexReadErrorReason|classifyCodexReadError" internal/provider/codex_responses.go
retained_declarations="^func codexToolCall\\(|^func classifyCodexReadError|^func \\(a HTTPChatAdapter\\) streamCodexChat|^func \\(a HTTPChatAdapter\\) readCodexStream|^func \\(a HTTPChatAdapter\\) handleCodexStreamEvent|^func codexUsageFromResponse|^func codexToolEvent|^func unsupportedCodexToolEvent|^func unsupportedCodexOutputItem|^func codexToolCallKey|^func codexToolCallArguments|^func \\(a HTTPChatAdapter\\) maxCodexAggregateBytes"
rg -n "$retained_declarations" internal/provider/codex_responses.go
```

## Acceptance

- Non-streaming parser declarations compile from `codex_responses_parse.go`.
- `codex_responses.go` no longer owns parser declarations.
- Shared stream/non-stream helpers remain available to both files.
- Direct compile, vet, serve, management route, and manage PTY smokes pass.
- The unrelated dirty `AGENTS.md` file is not staged or committed.
