# 526 Codex Non-Stream Function Call Namespace Rejection

## Context

Plan 525 found that Codex non-stream response parsing still preserves
namespaced upstream `function_call` output items. Streaming Codex response
parsing already rejects non-empty `function_call.namespace`, and Responses input
parsing now rejects non-empty `function_call.namespace` on follow-up
transcripts.

`docs/ilonasin-architecture.md` requires unsupported hosted, deferred,
namespaced, MCP, shell, tool-search, and other unproven tool families to fail
locally rather than be partially relayed or lossy-converted.

## Goal

Reject non-empty Codex non-stream `function_call.namespace` output before it can
be emitted to local Responses clients.

## Scope

1. Update `internal/provider/codex_responses_parse.go`.
2. Make `emitCodexFunctionCall` fail with `upstream_invalid_response` when the
   parsed Codex function call has a non-empty namespace.
3. Preserve supported non-namespaced function calls, including tool-call
   conversion and argument handling.
4. Preserve streaming parser behavior, Responses input parsing, request
   conversion, server SSE output shaping, routing, TUI, storage, config,
   logging, and SQLite behavior.
5. Do not add permanent tests.

## Out Of Scope

- Implementing namespaced function calls.
- Changing Codex request construction.
- Changing custom tool, tool-search, web-search, message, or reasoning output
  behavior.
- Removing server-side `ResponsesOutputItem.Namespace` shaping for any future
  implemented output family.
- Provider live-network probing.

## Verification

Use a temporary focused harness, then remove it before commit, to verify:

- Codex non-stream parsing rejects a namespaced `function_call` output with
  error class `upstream_invalid_response`.
- Supported non-namespaced `function_call` output remains accepted.
- Streaming parser namespace rejection remains unchanged by inspection.

Run:

```sh
rg -n 'emitCodexFunctionCall|unsupported namespaced function_call|Namespace' internal/provider/codex_responses_parse.go internal/provider/codex_responses_stream.go docs/plans/526-codex-nonstream-function-call-namespace-rejection.md
gofmt -w internal/provider/codex_responses_parse.go
git diff --check
git diff --no-index --check "$tmpempty" docs/plans/526-codex-nonstream-function-call-namespace-rejection.md
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run direct CLI smoke:

1. Build a temporary `ilonasin` binary.
2. Start `ilonasin serve` with isolated `ILONASIN_HOME`, temporary config,
   temporary SQLite, IO capture disabled, keepalive disabled, and configured
   provider instances.
3. Verify management health and snapshot over the Unix management socket.
4. Run bounded `ilonasin manage` at 80 and 140 columns under a pseudo-terminal.
5. Confirm TUI output includes ANSI color sequences.
6. Remove all temporary files and terminate the daemon.

## Acceptance

- Namespaced Codex non-stream `function_call` output fails before local
  Responses output emission.
- Supported non-namespaced function calls remain compatible.
- Existing streaming namespace rejection remains compatible.
- No permanent tests are added.
- Compile, vet, serve smoke, manage smoke, senior plan review, and senior
  implementation review pass.

## Implementation Record

- Senior plan review: Euclid, Avicenna, and Ampere approved the plan as-is.
- Updated `internal/provider/codex_responses_parse.go` only.
- Changed `emitCodexFunctionCall` so a non-empty parsed namespace returns
  `upstream_invalid_response` with reason `unsupported namespaced
  function_call`.
- Removed the stale helper that emitted namespaced Codex function calls as local
  Responses output items.
- Preserved supported non-namespaced function-call conversion and argument
  handling.

## Verification Record

- Temporary focused harness: passed. It verified namespaced Codex non-stream
  `function_call` output returns error class `upstream_invalid_response`, and a
  supported non-namespaced function call still emits one tool call. The
  temporary harness was removed before commit.
- Streaming parser namespace rejection: inspected
  `internal/provider/codex_responses_stream.go`, which still rejects non-empty
  `function_call.namespace` on both output-item added and done events.
- `rg -n 'emitCodexFunctionCall|unsupported namespaced function_call|Namespace'
  internal/provider/codex_responses_parse.go
  internal/provider/codex_responses_stream.go
  docs/plans/526-codex-nonstream-function-call-namespace-rejection.md`:
  passed.
- `gofmt -w internal/provider/codex_responses_parse.go`: passed.
- `git diff --check`: passed.
- `git diff --no-index --check "$tmpempty"
  docs/plans/526-codex-nonstream-function-call-namespace-rejection.md`: passed
  for the new untracked plan file. Git returned status `1` only because the
  files differ, with no whitespace findings.
- `find . -name '*_test.go' -type f -print`: passed, no files found.
- `go test ./...`: passed as a compile/package check; all packages reported no
  test files.
- `go vet ./...`: passed.
- Temporary `go build -o "$tmpbin/ilonasin" ./cmd/ilonasin`: passed.
- `ilonasin serve` smoke: passed with isolated `ILONASIN_HOME`, temporary
  config, free local bind port, IO capture disabled, keepalive disabled, and
  management health plus snapshot checked over the Unix socket.
- `ilonasin manage` smoke: passed at 80 and 140 columns under a
  pseudo-terminal. Both bounded runs exited by timeout with accepted status.
- TUI color capture: passed. Both captures were non-empty and contained ANSI
  SGR sequences with at least three unique 256-color codes.
- Senior implementation review: Euclid, Avicenna, and Ampere reported no
  findings.
- Cleanup: temporary home, binary, config, terminal captures, temporary
  harness, and daemon process were removed.
