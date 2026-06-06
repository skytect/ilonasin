# 488 IO Log Metadata Boundary

## Context

`docs/ilonasin-architecture.md` requires normal observability to stay
metadata-only, while explicit `[logging].capture_io = true` may write local and
debug upstream bodies to the bounded IO log. Recent work bounded IO log
retention, but IO log entries still use ad hoc `Meta` maps for request shape,
truncation state, provider identity, and stream-event indexes.

Those maps make the debug file less queryable and blur the distinction between
safe metadata and captured body content. A typed metadata shape can make the IO
log easier to inspect without expanding what is captured.

## Goal

Replace ad hoc IO log metadata maps with a fixed typed metadata DTO that covers
only the existing safe metadata fields:

- local request shape: model, stream, message count, input count, tool count,
  input item types, and input message roles;
- output truncation state;
- upstream provider identity: provider instance, provider type, and local
  credential ID;
- upstream stream event index.

## Scope

1. Add a typed metadata struct in `internal/logging/io.go`.
   - The struct is intentionally union-shaped with optional fields for local
     request shape, output truncation, and upstream identity/event metadata.
   - Unknown or extension metadata is intentionally not supported in this
     slice.
2. Change `logging.IORecord.Meta` to use that typed struct pointer.
3. Update server and provider IO log call sites to populate the struct.
4. Preserve the existing flat `meta` JSON field names:
   - `model`
   - `stream`
   - `message_count`
   - `input_count`
   - `tool_count`
   - `input_item_types`
   - `input_message_roles`
   - `body_truncated`
   - `provider_instance`
   - `provider_type`
   - `credential_id`
   - `stream_event`
5. Keep `Body`, `Bytes`, route, status, method, content type, event ID, and
   scrubber behavior unchanged.
6. Do not change capture policy:
   - no IO log is written unless `capture_io` is enabled;
   - upstream IO capture still additionally requires debug logging;
   - normal metadata tables and normal logs still must not store bodies.
7. Do not add permanent tests.

This slice must not alter logging config, retention, TUI rendering, management
DTOs, routing, request parsing, or provider behavior.

The typed metadata struct must not add body-like text fields. It may keep the
existing scrubbed shape labels (`model`, input item types, input message roles),
but it must not add prompts, completions, message content, tool names, tool
arguments, tool results, response text, raw event JSON, raw request metadata
values, headers, provider payload fragments, request IDs, account IDs, auth
values, tokens, or arbitrary maps.

Changing `logging.IORecord.Meta` is an internal source break. Current call sites
are limited to server and provider IO logging. All assignments should move to
the fixed typed struct or small constructor helpers in those packages.

## Verification

Run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run direct CLI smokes with temporary binaries and isolated homes:

- `ilonasin serve` with default `capture_io = false`, create a local token over
  the management socket, call a local request route with a tiny JSON body that
  fails before upstream, then stop the daemon and confirm no `ilonasin-io.log`
  exists;
- `ilonasin serve` with `capture_io = true`, create a local token, call a local
  request route with a tiny JSON body containing sentinel prompt text, a local
  bearer token marker, a fake upstream-key/token-like marker, and a tool
  argument marker, then confirm `ilonasin-io.log` exists, is valid JSONL,
  contains typed `meta` fields for the local request shape, preserves the exact
  flat JSON field names above, does not contain the full local token or sentinel
  secret markers, and respects existing body scrubber behavior;
- in the same `capture_io = true` smoke, confirm management snapshot output,
  SQLite metadata tables, and TUI output do not expose typed IO metadata or
  captured body sentinel text;
- with `capture_io = true` and normal non-debug log level, route a request far
  enough to attempt provider dispatch and confirm upstream provider request or
  response bodies are not written to the IO log;
- with `capture_io = true` and debug log level, use a temporary fake upstream or
  direct temporary harness to exercise provider IO records, then confirm the IO
  JSONL contains `provider_instance`, `provider_type`, `credential_id`, and
  `stream_event` fields where applicable without exposing upstream auth values;
- run bounded `ilonasin manage` against a daemon through a PTY to confirm the
  TUI still starts from the daemon-owned management API.

## Acceptance

- IO log metadata is typed at the logging boundary instead of assembled as
  unstructured maps at call sites.
- Existing JSON field names remain stable where possible.
- The slice does not increase IO capture scope or move body-like data into
  normal logs, normal metadata tables, management snapshots, or the TUI.
- Serve and manage smoke checks pass.
