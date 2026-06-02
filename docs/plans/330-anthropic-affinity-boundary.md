# 330 Anthropic Affinity Boundary

## Context

`docs/ilonasin-architecture.md` keeps local API parsing, routing, provider
adapters, metadata, and TUI as separate boundaries. Plan 329 added local-only
Anthropic metadata-derived affinity, but the helper now lives inside
`internal/anthropic/types.go`, which already owns request decoding, request
conversion, validation helpers, tool conversion, and JSON helpers.

This slice keeps the behavior from plan 329 and moves the affinity-specific
logic to its own Anthropic package file.

## Scope

1. Keep this slice limited to `internal/anthropic` and this plan.
   - Do not touch currently dirty `internal/server` or `internal/provider`
     files.
   - Do not change server routing, provider adapters, storage, management API,
     TUI, config, or logging.
2. Add a focused `internal/anthropic/affinity.go` boundary file.
   - Move `anthropicAffinityKey`, `anthropicUserIDSession`, and
     `safeAnthropicAffinityValue` there.
   - Keep the helpers package-private.
   - Keep `Request.ToChatCompletion` using only `anthropicAffinityKey`.
3. Preserve plan 329 behavior exactly.
   - `metadata.user_id` JSON string may provide nested `session_id`.
   - plain `metadata.session_id` may provide affinity.
   - plain `metadata.user_id` without nested `session_id` remains ignored.
   - blank, malformed, non-string, and overlong values remain ignored.
   - no affinity value is forwarded, logged, stored, displayed, or exposed.
4. Avoid new abstractions or permanent tests.

## Verification

Use a temporary focused Anthropic package test, then remove it before commit,
covering:

- Claude Code style `metadata.user_id` JSON extracts only nested `session_id`;
- JSON `metadata.user_id` without `session_id` is ignored;
- plain `metadata.session_id` still works;
- marshaled upstream Chat request has no top-level `metadata`, `session_id`, or
  `user`, and no raw `device_id`, `account_uuid`, or nested session value.

Then run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/anthropic
go test ./...
go vet ./...
```

Build `cmd/ilonasin`, start `ilonasin serve` with temporary `ILONASIN_HOME` and
`[server] bind = "127.0.0.1:0"`, check `/_ilonasin/manage/health` over the
management socket, run a short `ilonasin manage` TUI smoke, and clean up.

Because unrelated dirty server/provider files exist, verify the staged patch in
an isolated worktree from `HEAD` before commit.

## Acceptance

- Anthropic affinity behavior is unchanged.
- Affinity-specific code has a focused package boundary.
- The slice does not touch or depend on concurrent server/provider edits.
