# 513 Codex Response Shape Log Privacy

## Context

Plan 512 found that Codex Responses upstream 400 logging records
client-supplied input shape strings in normal `provider_http` logs:

- `codex_last_input_type`;
- `codex_last_input_role`;
- `codex_last_content_types`.

These values are parsed from raw client request items and can contain arbitrary
strings when the client sends unexpected Responses input. The architecture
requires normal logs to stay metadata-only; raw payload details and
client-provided free text belong only in IO logging when IO logging is enabled.

## Goal

Preserve useful Codex Responses 400 diagnostics while preventing
client-supplied free-text input shape values from reaching normal provider HTTP
logs.

## Scope

1. Update `internal/provider/codex_responses_request.go` only.
2. Keep count-based shape metadata:
   - `codex_input_items`;
   - `codex_tools`;
   - `codex_input_missing_type`;
   - `codex_message_items`;
   - `codex_assistant_input_text_parts`.
3. Replace free-text shape attributes with fixed allowlisted buckets or
   booleans:
   - last input type;
   - last input role;
   - last content type summary.
4. Use only categories implemented or intentionally handled by local Responses
   parsing and Codex preservation. Unknown values must be emitted as fixed
   `other` or counted, never as the original string.
5. Preserve when shape attrs are appended: only Codex upstream 400 responses in
   the existing non-stream and stream paths.
6. Preserve request marshaling, provider routing, IO logging behavior, storage,
   management API, TUI, config, and SQLite behavior.
7. Do not implement Responses unknown input rejection in this slice; that is a
   separate selected follow-up from plan 512.
8. Do not add permanent tests.

## Out Of Scope

- Changing Responses parsing, validation, or unknown input forwarding.
- Changing upstream request JSON.
- Changing `provider_http` event names, status behavior, retry handling, or
  error classes.
- Changing logging redaction globally.
- Changing IO logging capture or scrubbing.

## Verification

Use a temporary focused harness, then remove it before commit, to verify:

- known shape values are represented only by fixed safe bucket names;
- unknown item types, roles, and content types are represented as `other` or
  counts, not as the original client strings;
- count-based shape attributes remain present;
- stream and non-stream callers still append the same safe attrs on upstream
  400 paths through the shared helper.

Run:

```sh
rg -n 'codex_last_input_type|codex_last_input_role|codex_last_content_types|codexResponsesRequestShapeAttrs|uniqueCodexContentTypes' internal/provider/codex_responses_request.go docs/plans/513-codex-response-shape-log-privacy.md
gofmt -w internal/provider/codex_responses_request.go
git diff --check
git diff --no-index --check "$tmpempty" docs/plans/513-codex-response-shape-log-privacy.md
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

- Normal `provider_http` shape attrs no longer include raw client-supplied item
  type, role, or content type strings.
- Existing count diagnostics are preserved.
- The fix is limited to the Codex Responses request-shape logging helper and
  this plan.
- No permanent tests are added.
- Compile, vet, serve smoke, manage smoke, senior plan review, and senior
  implementation review pass.

## Implementation Record

- Updated `internal/provider/codex_responses_request.go` only.
- Replaced `codex_last_input_type`, `codex_last_input_role`, and
  `codex_last_content_types` normal-log attrs with fixed bucket attrs:
  `codex_last_input_type_bucket`, `codex_last_input_role_bucket`, and
  `codex_last_content_type_bucket`.
- Preserved existing count attrs for input items, tools, missing type,
  message items, and assistant input text parts.
- Preserved the existing stream and non-stream call sites by keeping the shared
  `codexResponsesRequestShapeAttrs` helper.

## Verification Record

- Senior plan review: three reviewers reported no findings.
- Temporary focused harness: passed. It verified that unknown client item type,
  role, and content type markers are emitted as fixed `other` buckets and that
  the old raw attr keys are absent. Temporary harness was removed before
  commit.
- `rg -n 'codex_last_input_type|codex_last_input_role|codex_last_content_types|codexResponsesRequestShapeAttrs|uniqueCodexContentTypes' internal/provider/codex_responses_request.go docs/plans/513-codex-response-shape-log-privacy.md`:
  passed. Code matches contain only the new bucket attrs and shared helper
  references; old raw attr names remain only in this plan text.
- `gofmt -w internal/provider/codex_responses_request.go`: passed.
- `git diff --check`: passed.
- `git diff --no-index --check "$tmpempty" docs/plans/513-codex-response-shape-log-privacy.md`:
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
