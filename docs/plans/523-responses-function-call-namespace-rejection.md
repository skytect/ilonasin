# 523 Responses Function Call Namespace Rejection

## Context

Plan 522 found that `function_call.namespace` is still accepted while parsing
Responses transcript input. Non-Codex chat conversion later rejects non-empty
namespaces, but Codex raw input preservation can forward the original
`function_call` item upstream.

`docs/ilonasin-architecture.md` requires unsupported hosted, namespaced, MCP,
shell, tool-search, and other unproven tool families to fail locally before
provider dispatch. Plans 519 and 522 both identify namespaced Codex tool-family
parity as unproven.

## Goal

Reject non-empty Responses `input[n].namespace` on `function_call` items during
input parsing, before Codex raw input preservation or non-Codex conversion can
dispatch the request.

## Scope

1. Update `internal/openai/responses.go`.
2. Change `parseResponsesFunctionCallItem` so a non-empty
   `input[n].namespace` returns `input[n].namespace is unsupported`.
3. Preserve current behavior for absent, `null`, or empty-string `namespace`
   values.
4. Preserve function call name, call ID, argument parsing, transcript
   call-pair validation, Codex raw preservation for supported function calls,
   and non-Codex conversion behavior.
5. Preserve provider adapters, routing, storage, management APIs, TUI, config,
   logging, IO logging, and SQLite behavior.
6. Do not add permanent tests.

## Out Of Scope

- Implementing namespaced function calls.
- Changing Responses tool declaration validation.
- Changing custom tool, tool-search, or function-call-output behavior.
- Rebuilding Codex raw input items from typed structs.
- Changing provider request bodies except that non-empty function-call
  namespace now fails locally before dispatch.

## Verification

Use a temporary focused harness, then remove it before commit, to verify:

- Responses decoding rejects a `function_call` item with non-empty
  `namespace` as `input[0].namespace is unsupported`.
- Responses decoding preserves accepted behavior for absent, `null`, and empty
  `namespace`.
- Codex-style conversion still preserves a supported function call without a
  namespace and still string-normalizes `arguments`.
- Non-Codex conversion still converts supported function call and output pairs.

Run:

```sh
rg -n 'parseResponsesFunctionCallItem|namespace is unsupported|codexResponsesFunctionCallInput|responsesInputToChatMessages' internal/openai/responses.go docs/plans/523-responses-function-call-namespace-rejection.md
gofmt -w internal/openai/responses.go
git diff --check
git diff --no-index --check "$tmpempty" docs/plans/523-responses-function-call-namespace-rejection.md
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
4. Run bounded `ilonasin manage` at 80 and 140 columns under a
   pseudo-terminal.
5. Confirm TUI output includes ANSI color sequences.
6. Remove all temporary files and terminate the daemon.

## Acceptance

- Non-empty Responses `function_call.namespace` fails locally before provider
  dispatch.
- Empty, null, or absent namespace remains compatible.
- Existing supported Responses function-call conversion behavior is preserved.
- No permanent tests are added.
- Compile, vet, serve smoke, manage smoke, senior plan review, and senior
  implementation review pass.

## Implementation Record

- Updated `internal/openai/responses.go` only.
- Changed `parseResponsesFunctionCallItem` to reject any non-empty
  `namespace` value with `input[n].namespace is unsupported`.
- Preserved absent, `null`, and empty-string namespace decoding.
- Preserved function name, call ID, argument parsing, transcript call-pair
  validation, Codex raw input preservation for supported function calls, and
  non-Codex conversion for supported function call/output pairs.

## Verification Record

- Senior plan review: three reviewers reported no findings.
- Temporary focused harness: passed. It verified rejection of non-empty
  `namespace`; accepted absent, `null`, and empty namespace; Codex preserved
  supported function calls and string-normalized `arguments`; and non-Codex
  conversion still emitted the supported function call/output pair. The
  temporary harness was removed before commit.
- `rg -n 'parseResponsesFunctionCallItem|namespace is unsupported|codexResponsesFunctionCallInput|responsesInputToChatMessages' internal/openai/responses.go docs/plans/523-responses-function-call-namespace-rejection.md`:
  passed.
- `gofmt -w internal/openai/responses.go`: passed.
- `git diff --check`: passed.
- `git diff --no-index --check "$tmpempty"
  docs/plans/523-responses-function-call-namespace-rejection.md`: passed for
  the new untracked plan file. Git returned status `1` only because the files
  differ, with no whitespace findings.
- `find . -name '*_test.go' -type f -print`: passed, no files found.
- `go test ./...`: passed as a compile/package check; all packages reported no
  test files.
- `go vet ./...`: passed.
- Temporary `go build -o "$tmpbin/ilonasin" ./cmd/ilonasin`: passed.
- `ilonasin serve` smoke: passed with isolated `ILONASIN_HOME`, temporary
  config, free local bind port, IO capture disabled, keepalive disabled, and
  management health plus snapshot checked over the Unix socket.
- `ilonasin manage` smoke: passed at 80 and 140 columns under a
  pseudo-terminal. Both bounded runs exited by timeout with status `124` as
  expected.
- TUI color capture: passed. The 80-column capture contained 436 SGR sequences
  across 9 unique 256-color foreground/background codes, and the 140-column
  capture contained 658 SGR sequences across 10 unique 256-color
  foreground/background codes.
- Senior implementation review: three reviewers reported no findings.
- Cleanup: temporary home, binary, config, terminal captures, temporary
  harness, and daemon process were removed.
