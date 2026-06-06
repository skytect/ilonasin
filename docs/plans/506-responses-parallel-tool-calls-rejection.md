# 506 Responses Parallel Tool Calls Rejection

## Context

Plan 499 found that Responses `parallel_tool_calls` is silently dropped when
the selected provider route policy disallows it. In current code,
`ResponsesRequest.ToChatCompletionRequest` forwards the field only when
`ResponsesConversionPolicy.AllowParallelToolCalls` is true; otherwise the field
is accepted by the local Responses decoder and omitted before provider dispatch.

`docs/ilonasin-architecture.md` requires compatibility routes to either
convert requests into the strict local model or reject unsupported features
before provider dispatch. Silently omitting provider-disallowed fields makes
clients believe a requested behavior was honored when it was not.

## Goal

Reject Responses `parallel_tool_calls` for providers whose route policy does
not allow forwarding it, while preserving existing forwarding for providers
whose route policy allows it.

## Scope

1. Update `ResponsesRequest.ToChatCompletionRequest` so a present
   `parallel_tool_calls` value returns a clear unsupported error unless
   `policy.AllowParallelToolCalls` is true.
2. Preserve existing OpenRouter behavior, where route policy allows
   `parallel_tool_calls` and the field is forwarded into the chat request.
3. Preserve current behavior for requests where `parallel_tool_calls` is
   absent.
4. Keep Codex-native input/tool preservation, provider adapters, routing,
   storage, management APIs, TUI, config, logging, and IO logging unchanged.
5. Do not add permanent tests.

## Out Of Scope

- Changing which provider route policies allow `parallel_tool_calls`.
- Adding provider/model capability checks.
- Changing Responses tool conversion behavior from plan 505.
- Changing Codex provider request building.

## Verification

Use a temporary focused harness, then remove it before commit, to verify:

- Default/DeepSeek-style Responses conversion rejects present
  `parallel_tool_calls` and names the field.
- OpenRouter-style Responses conversion forwards present
  `parallel_tool_calls` and marks the field present in the chat request.
- Requests without `parallel_tool_calls` continue to convert.
- Codex-style Responses conversion still preserves raw input/tools and rejects
  `parallel_tool_calls` unless the route policy is later changed to allow it.

Run:

```sh
rg -n 'parallel_tool_calls|AllowParallelToolCalls|ToChatCompletionRequest|AllowParallelTools' internal/openai/responses.go internal/provider/route_policy.go internal/server/provider_policy.go
git diff --check
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run direct CLI smoke:

1. Build a temporary `ilonasin` binary.
2. Start `ilonasin serve` with isolated `ILONASIN_HOME`, temporary config,
   temporary SQLite, IO capture disabled, and keepalive disabled.
3. Verify management health and snapshot over the Unix management socket.
4. Run bounded `ilonasin manage` at 80 and 140 columns under a pseudo-terminal.
5. Remove all temporary files and terminate the daemon.

## Acceptance

- Responses `parallel_tool_calls` is no longer silently dropped when route
  policy disallows it.
- OpenRouter forwarding behavior remains intact.
- Requests without `parallel_tool_calls` continue to work.
- No permanent tests are added.
- Compile, vet, serve smoke, manage smoke, senior plan review, and senior
  implementation review pass.

## Implementation Record

- Changed Responses conversion to reject present `parallel_tool_calls` when the
  selected conversion policy does not allow forwarding it.
- Preserved existing forwarding for policies that allow `parallel_tool_calls`,
  including marking the field present on the generated chat request.
- Preserved conversion behavior when `parallel_tool_calls` is absent.
- Left provider route policies, adapters, storage, management APIs, TUI, config,
  logging, and IO logging unchanged.

## Verification Record

- Senior plan review: three reviewers reported no findings.
- Temporary focused harness: passed for default/DeepSeek-style rejection,
  OpenRouter-style forwarding, absent-field conversion, and Codex-style
  rejection. Temporary harness was removed before commit.
- `rg -n 'parallel_tool_calls|AllowParallelToolCalls|ToChatCompletionRequest|AllowParallelTools' internal/openai/responses.go internal/provider/route_policy.go internal/server/provider_policy.go`:
  passed.
- `git diff --check`: passed.
- `find . -name '*_test.go' -type f -print`: passed, no files found.
- `go test ./...`: passed as a compile/package check; all packages reported no
  test files.
- `go vet ./...`: passed.
- Temporary `go build -o "$tmpbin/ilonasin" ./cmd/ilonasin`: passed.
- `ilonasin serve` smoke: passed with isolated `ILONASIN_HOME`, temporary
  config, free local bind port, IO capture disabled, keepalive disabled, and
  management health plus snapshot checked over the Unix socket.
- `ilonasin manage` smoke: passed at 80 and 140 columns under a pseudo-terminal;
  both captures included ANSI 256-color sequences.
- Senior implementation review: three reviewers reported no findings.
- Cleanup: temporary home, binary, config, harness, TUI captures, marker files,
  and daemon process were removed.
