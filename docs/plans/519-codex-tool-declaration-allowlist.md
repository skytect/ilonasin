# 519 Codex Tool Declaration Allowlist

## Context

Plan 518 found that Codex Responses tool preservation accepts unknown tool
declaration families as raw upstream payload. `validateCodexResponsesTool`
validates `function` and `namespace` declarations, then accepts every other
`tools[n].type` in its default case. `responsesToolsToChatTools` then preserves
that raw declaration for Codex routes.

`docs/ilonasin-architecture.md` says provider-specific escape hatches must be
explicit and namespaced, and only validated Codex-native Responses tool
declarations may be preserved for Codex provider routing. Broader hosted,
deferred, namespaced, MCP, shell, tool-search, and custom tool-family parity is
explicitly unproven in `docs/codex-compatibility-audit.md` and
`docs/plans/395-bounded-tool-parity-architecture.md`.

## Goal

Reject unknown or unproven Codex Responses tool declaration families before raw
Codex tool preservation, while preserving the currently validated `function`
declarations.

## Scope

1. Update `internal/openai/responses_tools.go`.
2. Change `validateCodexResponsesTool` so only explicitly handled tool types
   are accepted for Codex preservation:
   - `function`.
3. Reject every other `tools[n].type`, including `namespace`, with
   `tools[n].type is unsupported`.
4. Add a top-level field allowlist for preserved `function` declarations so
   unknown fields on that accepted family fail locally:
   - `function`: `type`, `name`, `description`, `parameters`.
5. Preserve existing validation for function names, description types, and
   parameters objects.
6. Preserve non-Codex Responses tool conversion behavior from plan 505.
7. Preserve provider adapters, routing, storage, management APIs, TUI, config,
   logging, IO logging, and SQLite behavior.
8. Do not add permanent tests.

## Out Of Scope

- Implementing or forwarding hosted, web-search, image-generation, deferred,
  namespace, MCP, shell, tool-search, custom, freeform, or other unproven Codex
  tool declaration schemas.
- Changing Responses input transcript item validation.
- Changing Codex output/tool-call parsing.
- Changing non-Codex tool conversion.
- Changing provider request bodies except that previously unknown Codex tool
  declaration families now fail locally before dispatch.

## Verification

Use a temporary focused harness, then remove it before commit, to verify:

- Codex conversion preserves a valid `function` tool declaration.
- Codex conversion rejects unproven tool types such as `namespace` and
  `web_search` with `tools[0].type is unsupported`.
- Codex conversion rejects extra fields on accepted `function` declarations.
- Non-Codex conversion still converts valid function tools and still rejects
  non-function, deferred, and strict declarations as before.

Run:

```sh
rg -n 'validateCodexResponsesTool|responsesToolsToChatTools|firstUnsupportedAnyField|tools\\[.*type is unsupported' internal/openai/responses_tools.go docs/plans/519-codex-tool-declaration-allowlist.md
gofmt -w internal/openai/responses_tools.go
git diff --check
git diff --no-index --check "$tmpempty" docs/plans/519-codex-tool-declaration-allowlist.md
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

- Unknown Codex Responses tool declaration families are rejected locally before
  provider dispatch.
- Preserved Codex tool declarations are limited to explicitly validated
  `function` shapes.
- Existing non-Codex Responses tool behavior is preserved.
- No permanent tests are added.
- Compile, vet, serve smoke, manage smoke, senior plan review, and senior
  implementation review pass.

## Implementation Record

- Updated `internal/openai/responses_tools.go`.
- Changed Codex Responses tool preservation to reject every non-`function`
  declaration type, including previously accepted `namespace` declarations,
  with `tools[n].type is unsupported`.
- Added a Codex-preserved function declaration field allowlist for `type`,
  `name`, `description`, and `parameters`.
- Preserved existing function name, description type, and parameters object
  validation.
- Preserved non-Codex Responses tool conversion behavior.

## Verification Record

- Senior plan review: two reviewers reported no findings; one reviewer found
  that preserving `namespace` remained inconsistent with the current
  unproven-tool-family docs, and that namespace child fields needed explicit
  validation if namespace stayed accepted. The plan was tightened before
  implementation to reject `namespace` and preserve only validated `function`
  declarations.
- Temporary focused harness: passed. It verified Codex function preservation,
  Codex rejection of `web_search`, Codex rejection of `namespace`, Codex
  rejection of extra fields on preserved function declarations, non-Codex valid
  function conversion, and non-Codex rejection of non-function, deferred, and
  strict tool declarations. The temporary harness was removed before commit.
- `rg -n 'validateCodexResponsesTool|responsesToolsToChatTools|firstUnsupportedAnyField|tools\\[.*type is unsupported' internal/openai/responses_tools.go docs/plans/519-codex-tool-declaration-allowlist.md`:
  passed.
- `gofmt -w internal/openai/responses_tools.go`: passed.
- `git diff --check`: passed.
- `git diff --no-index --check "$tmpempty" docs/plans/519-codex-tool-declaration-allowlist.md`:
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
- TUI color capture: passed. The 80-column capture contained 436 SGR sequences
  across 9 unique 256-color foreground/background codes, and the 140-column
  capture contained 578 SGR sequences across 10 unique 256-color
  foreground/background codes.
- Cleanup: temporary home, binary, config, terminal captures, temporary
  harness, and daemon process were removed.
- Senior implementation review: three reviewers reported no findings.
