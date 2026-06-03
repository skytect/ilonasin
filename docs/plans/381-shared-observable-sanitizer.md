# 381 Shared Observable Sanitizer

## Context

`docs/ilonasin-architecture.md` requires normal telemetry, management
snapshots, and the TUI to stay metadata-only. Normal operation must not persist
or render prompts, completions, request bodies, response bodies, raw provider
payloads, raw stream chunks, tool data, full bearer tokens, full provider
request IDs, full account IDs, balances, or credits.

Current sanitizer policy is split across three surfaces:

- `internal/server/request_metadata_sanitize.go`;
- `internal/management/snapshot_sanitize.go`;
- `internal/tui/display_sanitize.go`.

Those files duplicate unsafe marker regexes with small differences. That makes
the privacy boundary harder to audit and easier to drift from the architecture.

## Goal

Move shared unsafe observable-marker policy into `internal/privacy` and have
server request metadata, management snapshots, and TUI display sanitizers use
that shared policy.

Keep each caller's existing surface behavior:

- request metadata returns an empty string for unsafe addresses;
- management snapshots return `[redacted]` for unsafe display strings;
- TUI display returns `[redacted]` for unsafe rendered strings;
- existing truncation limits and token-fragment handling remain local.

## Scope

1. Add focused helpers in `internal/privacy` that answer whether a value
   contains unsafe observable markers.
2. Preserve the current distinction between request-metadata addresses and
   user-facing display strings:
   - request metadata uses a narrow address predicate that rejects core unsafe
     markers plus explicit raw payload/body, prompt body/text/payload,
     completion body/text/payload, request body, and response body markers;
   - management and TUI display use a broader display predicate that also
     rejects bare `raw`, `payload`, `prompt`, `completion`, and `body` markers.
3. Include the architecture-sensitive marker families already duplicated across
   the three surfaces:
   - bearer, API key, ilonasin token, OAuth, token, secret, authorization;
   - raw payload/body, prompt/completion/body payload fields;
   - account/account-id/acct markers;
   - request-id/request/req markers;
   - balance and credit;
   - raw SSE chunk and tool argument/result markers;
   - JWT-like dotted token markers.
4. Replace the duplicated unsafe regex use in:
   - `internal/server/request_metadata_sanitize.go`;
   - `internal/management/snapshot_sanitize.go`;
   - `internal/management/oauth.go`;
   - `internal/tui/display_sanitize.go`.
5. Keep local cleaning and formatting semantics in the existing packages:
   - request metadata still applies `safeMetadataToken` before checking;
   - management still strips controls, trims, redacts, and truncates to 128
     runes;
   - TUI still strips controls, trims, redacts, and truncates to 64 runes
     where the current helper does so;
   - TUI full wrapped display remains untruncated.
6. Keep account-display and refresh-failure sanitizers using their existing
   specialized `privacy` helpers.
7. Keep OAuth management error-class behavior local:
   - unsafe error classes still return `details_redacted`;
   - `refresh_token_expired`, `refresh_token_invalidated`, and
     `refresh_token_reused` remain allowed despite containing `token`;
   - only `safeEventID` should stop depending on the management snapshot regex
     and use the shared display predicate instead.

## Out Of Scope

- No public API, route, DTO, schema, storage, provider, logging, routing,
  account pooling, keepalive, or TUI layout changes.
- No new permanent tests.
- No exact configured-secret IO scrubber changes.
- No attempt to detect arbitrary secrets in prompts or completions.
- No management or TUI behavior changes beyond using the shared predicate.

## Verification

Run:

```sh
rg -n "unsafeMetadataAddressPattern|unsafePayloadMarkerPattern|unsafeSnapshotStringPattern|unsafeDisplayPattern" internal/server internal/management internal/tui
rg -n "unsafeManagementErrorClassPattern|safeManagementErrorClassWithMarkerPattern" internal/management/oauth.go
rg -n "UnsafeObservableMetadata|safeMetadataAddress|safeSnapshotString|safeDisplay" internal/privacy internal/server internal/management internal/tui
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/privacy ./internal/server ./internal/management ./internal/tui
go test ./...
go vet ./...
```

The first `rg` should show no duplicated unsafe observable regexes in server,
management, or TUI.

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage` at narrow and wide terminal
widths, and cleaning up all temporary files and processes.

Also perform a temporary focused sanitizer smoke without keeping a permanent
test file. It should prove:

- ordinary provider/model strings survive request metadata sanitization;
- safe near-marker request-metadata address strings that currently survive
  still survive, including `chat-completion`, `completion-model`,
  `prompt-cache`, `rawhide-model`, and `bodywork-model`;
- `bearer`, `sk-`, `iln_`, `prompt_body`, `completion_body`, `request_body`,
  `response_body`, `account`, `request-id`, `balance`, `credit`, `sse chunk`,
  `tool argument`, and JWT-like markers are rejected or redacted through the
  shared policy;
- management and TUI still redact broad display-only markers such as
  `prompt`, `completion`, `raw`, `payload`, and `body`;
- management and TUI retain their different unsafe return values and truncation
  behavior.
- OAuth management error classes keep their specialized behavior:
  `refresh_token_expired`, `refresh_token_invalidated`, and
  `refresh_token_reused` survive, while unsafe unallowlisted marker classes
  still return `details_redacted`.
- OAuth event IDs keep their existing shape: safe token-like event IDs survive,
  while marker-bearing values return an empty string.

Remove any temporary check before commit.

## Acceptance

- Server, management, and TUI no longer carry duplicated unsafe observable
  marker regex policy.
- Shared unsafe observable marker policy lives in `internal/privacy`.
- Request metadata keeps its narrower address behavior, while management and
  TUI keep their broader display behavior.
- OAuth management error-class behavior remains local and unchanged.
- Surface-specific return shapes and truncation behavior are preserved.
- Compile/package checks, vet, direct serve/manage smokes, and the temporary
  focused sanitizer smoke pass.
