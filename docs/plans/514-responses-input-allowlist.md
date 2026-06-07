# 514 Responses Input Allowlist

## Context

Plan 512 found that unknown Responses input item types are accepted as
`ResponseInputItem{Type: typ}` and can be forwarded upstream unchanged on Codex
routes through `CodexResponsesInput`. This violates the local compatibility
boundary in `docs/ilonasin-architecture.md`: unsupported fields and
unimplemented transcript or output families should fail locally before provider
dispatch.

The previous slice, plan 513, removed raw client shape strings from normal
Codex provider logs. This slice closes the remaining forwarding issue by
rejecting unknown input item families at decode time.

## Goal

Reject unknown Responses `input[n].type` values locally while preserving the
currently implemented and validated Responses input families.

## Scope

1. Update `internal/openai/responses.go`.
2. Change `parseResponsesInputItem` so the default branch returns a clear
   `input[n].type is unsupported` error instead of accepting
   `ResponseInputItem{Type: typ}`.
3. Preserve existing parsing and validation for:
   - normalized message items;
   - `function_call`;
   - `function_call_output`;
   - `tool_search_call`;
   - `tool_search_output`;
   - `custom_tool_call`;
   - `custom_tool_call_output`.
4. Preserve call-pair validation, Responses-to-chat conversion, Codex raw input
   preservation for validated known families, provider route policy behavior,
   storage, management APIs, TUI, config, logging, and IO logging.
5. Do not add permanent tests.

## Out Of Scope

- Adding support for new Responses item families.
- Changing Codex-native preservation for known validated families.
- Changing Responses tool declaration validation.
- Changing provider adapters, routing policy, storage, management, TUI, config,
  logging, or IO logging.

## Verification

Use a temporary focused harness, then remove it before commit, to verify:

- `DecodeResponses` rejects an unknown `input[0].type` with a clear
  unsupported error.
- Normalized message input without explicit `type` still decodes.
- Known Codex-preserved item families still decode and are preserved by
  Codex-style conversion.
- Non-Codex conversion still rejects unrepresentable known families through the
  existing conversion boundary.

Run:

```sh
rg -n 'parseResponsesInputItem|input\\[.*\\]\\.type is unsupported|ResponseInputItem\\{Type: typ\\}|CodexResponsesInput' internal/openai/responses.go docs/plans/514-responses-input-allowlist.md
gofmt -w internal/openai/responses.go
git diff --check
git diff --no-index --check "$tmpempty" docs/plans/514-responses-input-allowlist.md
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

- Unknown Responses input item types are rejected locally before provider
  dispatch.
- Existing supported and validated input item families are preserved.
- Codex raw input preservation remains limited to known validated families.
- No permanent tests are added.
- Compile, vet, serve smoke, manage smoke, senior plan review, and senior
  implementation review pass.

## Implementation Record

- Updated `internal/openai/responses.go` so `parseResponsesInputItem` rejects
  unknown `input[n].type` values with `input[n].type is unsupported`.
- Preserved all existing known Responses input family parsers and transcript
  validation.
- Preserved Codex raw input preservation for known validated families and
  non-Codex conversion behavior for representable function call paths.

## Verification Record

- Senior plan review: three reviewers reported no findings.
- Temporary focused harness: passed. It verified unknown input type decode
  rejection, normalized message decoding, Codex preservation of a known
  `tool_search` pair, and non-Codex rejection of that known unrepresentable
  family. Temporary harness was removed before commit.
- `rg -n 'parseResponsesInputItem|input\\[.*\\]\\.type is unsupported|ResponseInputItem\\{Type: typ\\}|CodexResponsesInput' internal/openai/responses.go docs/plans/514-responses-input-allowlist.md`:
  passed. Code no longer contains `ResponseInputItem{Type: typ}` and retains
  the explicit unsupported errors.
- `gofmt -w internal/openai/responses.go`: passed.
- `git diff --check`: passed.
- `git diff --no-index --check "$tmpempty" docs/plans/514-responses-input-allowlist.md`:
  passed for the new untracked plan file. Git returned status `1` only because
  the files differ, with no whitespace findings.
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
- TUI color capture: passed. The 80-column capture contained 108 256-color SGR
  foreground sequences, and the 140-column capture contained 175.
- Cleanup: temporary home, binary, config, terminal captures, temporary harness,
  and daemon process were removed.
- Senior implementation review: three reviewers reported no findings.
