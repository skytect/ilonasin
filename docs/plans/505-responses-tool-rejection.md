# 505 Responses Tool Rejection

## Context

Plan 499 found that non-Codex chat-adapter Responses routes silently drop some
tool declarations. `responsesToolsToChatTools` currently skips non-`function`
tools, `defer_loading: true` tools, and `strict: true` tools when Codex-native
tool preservation is disabled. That violates the architecture requirement that
Responses routes either convert into the strict local model or reject
unsupported features before provider dispatch.

The target behavior is:

- Codex provider routing may preserve validated Codex-native Responses tool
  declarations.
- Non-Codex chat-adapter routing may convert representable function tools into
  OpenAI chat tools.
- Non-Codex chat-adapter routing must reject hosted, deferred, strict,
  namespaced, MCP, shell, tool-search, and other unrepresentable tool families
  instead of silently dropping them.

## Goal

Reject unrepresentable Responses tool declarations for non-Codex providers,
while preserving Codex-native Responses tool handling.

## Scope

1. Update `responsesToolsToChatTools` so, when `preserveCodexTools` is false:
   - `tools[n].type != "function"` returns a clear unsupported error;
   - `tools[n].defer_loading: true` returns a clear unsupported error;
   - `tools[n].strict: true` returns a clear unsupported error.
2. Preserve existing validation for function tool names, descriptions,
   parameters, duplicate names, unsupported fields, and boolean field types.
3. Preserve Codex policy behavior where validated raw Responses tools are
   retained for provider routing.
4. Keep top-level Responses parsing, input conversion, provider adapters,
   routing, storage, management APIs, TUI, config, logging, and IO logging
   unchanged.
5. Do not add permanent tests.

## Out Of Scope

- Implementing hosted, deferred, strict, namespaced, MCP, shell, or tool-search
  tool support for non-Codex providers.
- Changing Codex-native Responses tool validation or schemas.
- Changing `parallel_tool_calls` behavior; that remains a separate selected
  follow-up finding.
- Reworking route preflight or provider capability checks.

## Verification

Use a temporary focused harness, then remove it before commit, to verify:

- Non-Codex Responses conversion rejects a hosted/non-function tool and names
  `tools[0].type`.
- Non-Codex Responses conversion rejects `defer_loading: true` and names
  `tools[0].defer_loading`.
- Non-Codex Responses conversion rejects `strict: true` and names
  `tools[0].strict`.
- Non-Codex Responses conversion still converts a valid function tool.
- Codex Responses conversion still preserves raw Codex/native tools.

Run:

```sh
rg -n 'continue|defer_loading|strict|tools\\[.*type|responsesToolsToChatTools' internal/openai/responses_tools.go internal/openai/responses.go
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

- Non-Codex Responses tool conversion rejects all unrepresentable tool
  declarations that were previously skipped.
- Valid non-Codex function tool conversion is preserved.
- Codex-native Responses tool preservation is preserved.
- No permanent tests are added.
- Compile, vet, serve smoke, manage smoke, senior plan review, and senior
  implementation review pass.

## Implementation Record

- Changed non-Codex Responses tool conversion to reject non-`function` tool
  types instead of skipping them.
- Changed `defer_loading: true` and `strict: true` non-Codex function tools to
  return explicit unsupported-field errors instead of being skipped.
- Removed duplicate later strict/defer checks that became unreachable after the
  early validated rejection.
- Preserved valid function tool conversion and Codex raw Responses tool
  preservation.

## Verification Record

- Senior plan review: three reviewers reported no findings.
- Temporary focused harness: passed for hosted/non-function rejection,
  `defer_loading: true` rejection, `strict: true` rejection, valid function
  conversion, and Codex raw tool preservation. Temporary harness was removed
  before commit.
- `rg -n 'continue|defer_loading|strict|tools\\[.*type|responsesToolsToChatTools' internal/openai/responses_tools.go internal/openai/responses.go`:
  passed; the only `continue` in `responsesToolsToChatTools` is the Codex raw
  preservation path.
- `git diff --check`: passed.
- `find . -name '*_test.go' -type f -print`: passed, no files found.
- `go test ./...`: passed as a compile/package check; all packages reported
  no test files.
- `go vet ./...`: passed.
- Temporary `go build -o "$tmpbin/ilonasin" ./cmd/ilonasin`: passed.
- `ilonasin serve` smoke: passed with isolated `ILONASIN_HOME`, temporary
  config, and management health plus snapshot checked over the Unix socket.
- `ilonasin manage` smoke: passed at 80 and 140 columns under a pseudo-terminal.
- Senior implementation review: three reviewers reported no findings.
- Cleanup: temporary home, binary, config, harness, captures, and daemon process
  were removed.
