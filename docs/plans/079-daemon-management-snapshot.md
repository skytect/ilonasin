# 079 Daemon Management Snapshot

## Context

The local-token management slice moved token list/create/disable behind the
daemon management socket, but `ilonasin manage` still reads most of its display
state through direct SQLite-backed interfaces:

- upstream credential list,
- fallback policies,
- OAuth credential and account summaries,
- model cache summaries,
- request, usage, latency, stream, health, and fallback telemetry.

This conflicts with the target architecture in `docs/ilonasin-architecture.md`:
`manage` should be a TUI client of a daemon-owned management API, with SQLite
inspection and mutation performed behind the daemon boundary.

This slice moves read-only TUI state loading behind the daemon management API.
It intentionally does not migrate the remaining non-token mutations yet.

## Goal

Add a daemon-owned management snapshot endpoint and make the normal TUI reload
path use that snapshot instead of calling direct SQLite-backed readers.

After this slice, local token mutations and all normal read-only manage state
loading go through the daemon management socket. Remaining direct manage
dependencies are limited to non-token mutation actions and are still legacy.

## Scope

1. Add a management snapshot contract under `internal/management`.
   - Request: no body.
   - Response: explicit `ManagementSnapshotResponse` with allowlisted
     management DTO row types.
   - Do not reuse broad domain/storage structs directly in JSON responses.
   - Include only the fields enumerated in "Snapshot Fields" below.
2. Build the snapshot on the daemon side from existing safe metadata interfaces:
   - `credentials.LocalTokenManager`,
   - `provider.Registry`,
   - upstream credential metadata readers,
   - OAuth metadata readers,
   - model cache reader,
   - observability reader.
3. Expose the snapshot only on the management Unix socket.
   - Do not mount it on the public OpenAI-compatible API.
   - Keep route isolation checks for public routes on the management socket and
     management routes on the public listener.
4. Extend the management HTTP client with `LoadManagementSnapshot`.
5. Change `internal/tui` so production `Run` and `Check` require a snapshot
   client.
   - Snapshot mode must populate the same view fields as the previous direct
     reader path.
   - If a production snapshot client is present, `reload` must not fall back to
     direct readers after a snapshot error.
   - Direct readers may remain only behind explicit helper constructors used by
     targeted smoke exercises for not-yet-migrated mutation actions.
6. Change production `app.Manage` and `app.ManageCheck` to pass the management
   snapshot client into the TUI.
7. Keep existing non-token actions unchanged for this slice:
   - API-key add/disable,
   - fallback enable/disable,
   - OAuth login and refresh,
   - telemetry prune.
8. Update architecture docs only if needed to clarify that read-only inspection
   has moved while non-token mutations remain legacy.
9. Do not add permanent tests.
10. Do not push.

## Non-Goals

- Do not migrate upstream credential mutation in this slice.
- Do not migrate fallback policy mutation in this slice.
- Do not migrate OAuth login, OAuth refresh, or telemetry prune in this slice.
- Do not remove `manage` SQLite bootstrap yet, because remaining legacy
  mutations still need it.
- Do not expose provider secrets, bearer tokens, request bodies, response
  bodies, raw provider payloads, raw SSE chunks, raw tool arguments, raw tool
  results, full provider request IDs, full account IDs, balances, or credits.
- Do not change SQLite schema or migrations.

## Snapshot Fields

Every JSON row type is defined under `internal/management` and populated by an
explicit mapping layer.

Provider instances:

- `id`
- `type`
- `base_url`, after existing provider config validation and without userinfo,
  query, or fragment
- `auth_style`
- `placeholder`
- `api_key`
- `oauth`
- `oauth_refresh`
- `chat`
- `model_discovery`

Local tokens:

- same fields as the existing `management.LocalToken` DTO:
  `id`, `label`, `token_prefix`, `token_last4`, `created_at`, `disabled_at`,
  and `disabled`

Upstream credentials:

- `id`, local SQLite request metadata ID only
- `provider_instance_id`
- `kind`
- `label`
- `secret_prefix`
- `secret_last4`
- `fallback_group`
- `created_at`
- `disabled_at`
- `disabled`

Fallback policies:

- `provider_instance_id`
- `group_label`
- `enabled`
- `credential_count`
- `explicit`

OAuth credentials:

- `id`, local SQLite fallback event ID only
- `provider_instance_id`
- `label`
- `account_display_label`
- `plan_label`
- `scopes`
- `expires_at`
- `last_refresh_at`
- `refresh_failure_class`
- `created_at`
- `disabled_at`
- `disabled`

Provider account summaries:

- `id`
- `provider_instance_id`
- `credential_id`
- `display_label`
- `plan_label`
- `created_at`

Model cache rows:

- `provider_instance_id`
- `model_id`
- `display_name`
- `capabilities`
- `updated_at`

Recent request summaries:

- `id`
- `started_at`
- `provider_instance_id`
- `model_id`
- `requested_provider_id`
- `requested_model_id`
- `resolved_provider_id`
- `resolved_model_id`
- `credential_id`
- `credential_label`
- `http_status`
- `error_class`
- `retry_count`
- `fallback_count`
- `fallback_reason`
- token and cost counters already stored as metadata:
  `prompt_tokens`, `completion_tokens`, `total_tokens`, `reasoning_tokens`,
  `cache_hit_tokens`, `cache_write_tokens`, `cost_microunits`
- timing counters:
  `total_latency_ms`, `time_to_first_token_ms`,
  `output_tokens_per_second`
- stream summary fields:
  `stream_completion_status`, `stream_chunk_count`

Usage summary rows:

- `provider_instance_id`
- `request_count`
- `prompt_tokens`
- `completion_tokens`
- `total_tokens`
- `reasoning_tokens`
- `cache_hit_tokens`
- `cache_write_tokens`
- `cost_microunits`

Latency summary rows:

- `provider_instance_id`
- `request_count`
- `average_latency_ms`
- `average_time_to_first_token_ms`
- `average_output_tps`

Stream summary rows:

- `completion_status`
- `stream_count`
- `chunk_count`

Health summary rows:

- `provider_instance_id`
- `model_id`
- `credential_id`
- `credential_label`
- `event_class`
- `http_status`
- `error_class`
- `occurred_at`
- `retry_after`

Fallback summary rows:

- `id`
- `request_metadata_id`
- `occurred_at`
- `provider_instance_id`
- `model_id`
- `from_credential_id`
- `from_credential_label`
- `to_credential_id`
- `to_credential_label`
- `reason`

Forbidden snapshot fields and values:

- raw request IDs,
- provider request IDs,
- raw error strings,
- raw account IDs,
- request bodies,
- response bodies,
- raw provider payloads,
- raw SSE chunks,
- prompts,
- completions,
- tool arguments,
- tool results,
- full bearer tokens,
- full provider account identifiers,
- balances,
- credits.

Telemetry pruning:

- `pruning_available` boolean only
- no prune result in the snapshot; the TUI keeps the most recent local result
  after a user-triggered prune until that mutation path is migrated

## Design Constraints

1. Snapshot response data must be safe display metadata only.
2. Snapshot errors must not contain secrets or raw payloads.
3. The TUI must not import `internal/storage/sqlite`.
4. Public HTTP clients must not reach the snapshot route through
   `server.bind`.
5. Management-socket clients must not reach public `/v1/*` routes through the
   management listener.
6. Snapshot loading should preserve the current TUI view output for equivalent
   data.
7. Snapshot loading must not require any provider HTTP calls.
8. Snapshot loading must not mutate SQLite.
9. The management socket directory and socket file permissions must remain
   private after adding the aggregate snapshot endpoint.
10. Production `tui.Run` and `tui.Check` must fail if no snapshot client is
    provided.

## Implementation Plan

1. Add `internal/management/snapshot.go`.
   - Define `PathSnapshot`.
   - Define `ManagementSnapshotResponse`.
   - Define `SnapshotClient`.
   - Define small source interfaces for upstream metadata, OAuth metadata,
     model cache, and observability.
   - Add `Service.LoadManagementSnapshot`.
   - Add explicit DTO row structs and mapping functions for every field listed
     above.
2. Extend `internal/management/http.go`.
   - Add `GET /_ilonasin/manage/snapshot`.
   - Return JSON with safe status-only errors.
3. Extend `internal/management/http_client.go`.
   - Add `LoadManagementSnapshot`.
   - Keep generic HTTP errors safe.
4. Extend `internal/app/management.go`.
   - Build the management service with registry and existing store-backed
     readers.
5. Extend `internal/tui`.
   - Add a required production `ManagementSnapshotClient` field.
   - `Run` and `Check` accept this client and error if it is nil.
   - In `reload`, call `LoadManagementSnapshot` when the snapshot client is
     set and populate all row fields from the response.
   - Do not call direct readers after a snapshot error.
   - Move the old direct-reader reload into an explicitly named helper path for
     smoke exercises that still need direct mutation dependencies.
6. Update `app.Manage` and `app.ManageCheck`.
   - Pass the Unix management client as both token client and snapshot client.
   - Keep the existing initial daemon reachability check, but prefer the
     snapshot call because it proves the read path.
7. Update smoke helpers.
   - Assert public listener returns not found for the snapshot route.
   - Assert management snapshot route returns rows and management socket still
     rejects public `/v1/models`.
   - Assert management socket directory remains `0700` and socket file remains
     private.
   - Add a check-only snapshot client that counts calls and fails the TUI smoke
     if `reload` does not call `LoadManagementSnapshot`.
   - Add a check-only snapshot client with sentinel values that differ from
     SQLite seed data, then assert every TUI section renders the snapshot
     sentinels, not the direct-reader seed values.
   - Include a provider instance sentinel that differs from the local registry
     passed to the TUI, proving the provider section also comes from the
     snapshot.
   - Include local token sentinels in the snapshot and a token client whose
     `ListLocalTokens` fails or returns conflicting data, proving snapshot
     reload does not call the separate token-list path.
   - Add a check-only snapshot client that returns an error while direct readers
     could otherwise satisfy data, then assert reload fails and does not use the
     legacy direct read path.
   - Seed representative rows for each snapshot section in `manage --check` and
     assert the TUI view includes those exact safe values after reload.
   - Seed forbidden markers for secrets, raw account IDs, provider request IDs,
     raw error text, request bodies, response bodies, raw provider payloads, raw
     SSE chunks, prompts, completions, tool arguments, tool results, full bearer
     tokens, full provider account identifiers, balances, and credits. Assert
     neither snapshot JSON nor TUI output contains those markers.
   - Add negative searches proving:
     - `internal/tui` does not import `internal/storage/sqlite`,
     - production `app.Manage` does not pass `rt.Store` as a read dependency to
       `tui.Run`,
     - production `tui.Run` and `tui.Check` require a snapshot client,
     - `reload` does not call the legacy direct read path when a snapshot
       client is set.
8. Review the written code before running commands.
9. Run:
   - `find . -name '*_test.go' -type f -print`
   - `git diff --check`
   - `go test ./...`
   - `go vet ./...`
   - `go build -o "$tmpbin/ilonasin" ./cmd/ilonasin`
   - `ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check`
   - `ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check`

## Review Questions

1. Is a single read-only snapshot the right next management boundary, or should
   read models be split per TUI section immediately?
2. Does keeping non-token mutation paths direct for one more slice still make
   forward progress without entrenching the legacy architecture?
