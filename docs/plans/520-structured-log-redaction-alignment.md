# 520 Structured Log Redaction Alignment

## Context

Plan 518 found that `IsSensitiveLogKey` redacts credential keys plus only a
small set of normal-log keys: `header`, `body`, `payload`, `raw`, and `stdout`.
Plan 077 requires the normal slog guard to redact broader sensitive key
families, including account identifiers, request and generation identifiers,
URLs and URL components, prompts, completions, and stderr.

`docs/ilonasin-architecture.md` requires metadata-only normal observability and
forbids normal persistence of prompts, completions, request bodies, response
bodies, raw provider payloads, raw SSE chunks, tool payloads, full bearer
tokens, full provider request IDs, and full account IDs.

## Goal

Align the normal structured-log key guard with the documented sensitive
attribute families so future accidental slog attributes are defensively
redacted.

## Scope

1. Update `internal/logging/secrets.go`.
2. Expand `IsSensitiveLogKey` to cover the documented sensitive key families:
   - account identifiers and account-like keys;
   - request and generation identifiers;
   - URL and URL component keys;
   - prompt and completion keys;
   - tool argument and result keys;
   - SSE chunk keys;
   - headers, bodies, payloads, raw values, stdout, and stderr.
3. Preserve `IsCredentialKey` behavior.
4. Preserve `IsIOSensitiveKey` behavior.
5. Preserve existing explicit normal log call sites, logging setup, event IDs,
   IO logging, provider adapters, server routes, storage, management APIs, TUI,
   config, and SQLite behavior.
6. Keep operational normal-log keys that are already intentionally safe, such
   as static `endpoint` labels, `route` labels, provider instance IDs, provider
   types, event names, status values, and error classes.
7. Do not add permanent tests.

## Out Of Scope

- Auditing or changing every slog call site.
- Changing IO logging scrub behavior.
- Changing request metadata persistence or management snapshots.
- Changing logger output format, truncation, event IDs, or config.
- Broad privacy sanitizer refactors.

## Verification

Use a temporary focused harness, then remove it before commit, to verify:

- `IsSensitiveLogKey` returns true for representative documented sensitive
  keys:
  - `account_id`;
  - `request_id`;
  - `generation_id`;
  - `callback_url`;
  - `query`;
  - `prompt`;
  - `completion`;
  - `stderr`;
  - `tool_arguments`;
  - `tool_result`;
  - `tool_results`;
  - `sse_chunk`;
  - `sse_chunks`.
- `IsSensitiveLogKey` returns true for representative compound sensitive keys:
  - `oauth_callback_url`;
  - `base_url`;
  - `endpoint_host`;
  - `raw_path`;
  - `request_body_bytes`;
  - `tool_result_body`.
- `IsSensitiveLogKey` still returns true for credential keys such as
  `authorization` and `api_key`.
- `IsSensitiveLogKey` returns false for representative safe operational keys:
  - `event`;
  - `endpoint`;
  - `route`;
  - `provider_instance`;
  - `provider_type`;
  - `error_class`;
  - `status`.
- The slog guard redacts sensitive keys inside nested slog groups.
- Long non-sensitive strings are still truncated, not redacted.

Run:

```sh
rg -n 'IsSensitiveLogKey|IsIOSensitiveKey|normalizedSecretKey|account_id|request_id|generation_id|callback_url|stderr|tool_arguments|sse_chunk' internal/logging/secrets.go docs/plans/520-structured-log-redaction-alignment.md
gofmt -w internal/logging/secrets.go
git diff --check
git diff --no-index --check "$tmpempty" docs/plans/520-structured-log-redaction-alignment.md
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

- The normal slog guard defensively redacts the documented sensitive key
  families.
- Existing safe operational log keys remain usable.
- IO logging behavior is unchanged.
- No permanent tests are added.
- Compile, vet, serve smoke, manage smoke, senior plan review, and senior
  implementation review pass.

## Implementation Record

- Updated `internal/logging/secrets.go` only.
- Expanded `IsSensitiveLogKey` to defensively redact documented normal-log
  sensitive key families:
  - account and user identifiers;
  - request and generation identifiers;
  - URL and URL component keys;
  - prompt, completion, body, payload, and raw keys;
  - stdout and stderr keys;
  - tool argument and tool result keys;
  - SSE chunk keys.
- Added tokenized sensitive-family matching with explicit safe operational key
  exceptions for `event`, `endpoint`, `route`, `provider_instance`,
  `provider_type`, `error_class`, and `status`.
- Kept safe token usage counters visible with explicit metadata exceptions,
  while still redacting credential token keys and prompt/completion content
  key families such as `system_prompt`, `user_prompt`, `completion_content`,
  `prompts`, and `completions`.
- Preserved `IsCredentialKey` and `IsIOSensitiveKey` behavior.
- Preserved existing logging setup, explicit slog call sites, IO logging,
  provider, server, storage, management, TUI, config, and SQLite behavior.

## Verification Record

- Senior plan review: two reviewers reported no findings; one reviewer found
  that the plan did not explicitly include raw tool-result and plural SSE chunk
  keys. The plan was updated before implementation.
- Temporary focused harness: passed. It verified representative sensitive keys,
  representative compound sensitive keys, prompt and completion family keys,
  representative safe operational keys, safe token usage counters, credential
  and token secret keys, nested slog group redaction, and long safe string
  truncation. The temporary harness was removed before commit.
- `rg -n 'IsSensitiveLogKey|sensitiveLogKeyFamily|containsSensitiveLogPhrase|prompt_tokens|completion_tokens|prompt_body|completion_body|endpoint_host|raw_path|request_body_bytes|tool_result_body' internal/logging/secrets.go docs/plans/520-structured-log-redaction-alignment.md`:
  passed.
- `gofmt -w internal/logging/secrets.go`: passed.
- `git diff --check`: passed.
- `git diff --no-index --check "$tmpempty" docs/plans/520-structured-log-redaction-alignment.md`:
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
- Senior implementation review: one reviewer reported no findings; two
  reviewers found that exact-key matching was still too narrow for documented
  sensitive key families such as URL components and request body variants. The
  classifier was updated to use tokenized sensitive-family matching with safe
  operational exceptions, and the focused harness, compile, vet, serve smoke,
  and manage smoke were rerun successfully.
- Senior implementation re-review: two reviewers reported no findings; one
  reviewer found that broad `prompt` and `completion` token matching would
  redact safe metadata counters such as `prompt_tokens` and
  `completion_tokens`. The matcher was narrowed to exact `prompt` and
  `completion` keys plus payload-like prompt/completion phrases, and the
  focused harness, compile, vet, serve smoke, and manage smoke were rerun
  successfully.
- Senior implementation final re-review: one reviewer reported no findings; two
  reviewers found that the narrowed matcher missed compound and plural prompt
  and completion content keys such as `system_prompt`, `completion_content`,
  `prompts`, and `completions`. The matcher now uses explicit safe metadata
  exceptions for prompt and completion token counters, then redacts prompt and
  completion key families. The focused harness was rerun successfully.
- Senior implementation final re-review after that correction: two reviewers
  reported no findings; one reviewer found that the singular `token` key-family
  match would also redact normal usage metadata such as `reasoning_token_rate`
  and `time_to_first_token_ms`.
  The safe metadata exceptions now cover the current token-count and token-rate
  usage fields plus the existing time-to-first-token timing field while
  credential token keys still redact. The focused harness was rerun
  successfully.
- Senior implementation final re-review after adding `time_to_first_token_ms`:
  three reviewers reported no findings.
