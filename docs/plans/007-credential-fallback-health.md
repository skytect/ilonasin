# Plan 007: Credential Fallback and Health Metadata

## Goal

Implement constrained same-provider-instance and same-model API-key credential
fallback for chat requests, and start recording provider/credential health plus
fallback decisions as metadata-only SQLite state.

This slice turns the existing single-credential resolver into an auditable
availability fallback path without adding cross-provider routing, cross-model
routing, quota evasion, OAuth, or Codex credential behavior.

## Architecture Inputs

- `docs/ilonasin-architecture.md`
- `docs/deepseek-api.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- `docs/codex-auth.md`
- `docs/codex-endpoints.md`
- `docs/plans/001-initial-go-scaffold.md`
- `docs/plans/002-local-api-tokens.md`
- `docs/plans/003-upstream-api-key-credentials.md`
- `docs/plans/004-nonstreaming-chat-adapters.md`
- `docs/plans/005-streaming-chat-adapters.md`
- `docs/plans/006-model-discovery-cache.md`
- `AGENTS.md`

## Scope

1. Add an explicit API-key credential fallback policy boundary:
   - credential switching is disabled by default,
   - API-key credentials belong to a provider-instance fallback group,
   - the initial group label is `default`,
   - a group must be explicitly enabled before the resolver may return more
     than one credential for fallback attempts,
   - smoke checks may enable the group through the credential service in an
     isolated DB,
   - no hidden cross-provider, cross-model, or cross-account cycling policy is
     introduced.
2. Extend the upstream credential resolver boundary:
   - keep the existing single `ResolveAPIKey` method for existing callers,
   - add a method that returns the explicitly allowed API-key attempt set for
     one provider instance in deterministic order,
   - when fallback policy is disabled, the attempt set contains only the oldest
     eligible credential,
   - when fallback policy is enabled for the credential group, the attempt set
     contains all enabled credentials in that group,
   - return secret material only through the resolver boundary, not through
     list/metadata paths.
3. Implement SQLite credential and policy storage:
   - add migration columns or a small policy table for fallback group metadata,
   - keep existing credentials in group `default`,
   - keep fallback disabled unless an explicit service method enables it,
   - select enabled `api_key` credentials for one configured provider instance,
   - join `credential_secrets` only for `api_key`,
   - order by credential ID ascending,
   - return `ErrNoEligibleCredential` when no rows exist.
4. Add metadata recorder methods for:
   - credential/provider health events,
   - fallback events.
5. Use existing `health_events` and `fallback_events` tables without storing raw
   bodies, provider payloads, prompts, completions, bearer tokens, request IDs,
   account IDs, balances, or credit data.
6. Implement non-streaming chat availability fallback:
   - parse the local model address once,
   - keep the same provider instance and same provider model for every attempt,
   - attempt only the credential set allowed by the explicit fallback policy,
   - retry/switch only for availability-class failures:
     `upstream_network_error`, `upstream_timeout`, and upstream HTTP `500`,
     `502`, `503`, or `504`,
   - do not retry or switch credentials for local validation errors,
     unsupported providers, missing credentials, `400`, `401`, `402`, `403`,
     `404`, `408`, `422`, `429`, malformed successful upstream bodies, or
     upstream response bodies that exceed limits,
   - return the first successful result,
   - if every allowed credential fails availability checks, return local status
     `502` with exactly
     `{"error":{"message":"upstream request failed","type":"api_error","code":"upstream_unavailable"}}`,
   - for all upstream non-2xx responses, return a local OpenAI-style error
     envelope instead of forwarding raw upstream response bodies.
7. Implement streaming chat pre-stream availability fallback only:
   - try another eligible credential only when the adapter fails before any
     local SSE data has been written,
   - use the same retryable error classes/statuses as non-streaming,
   - never switch credentials after any local stream event has started,
   - if every allowed credential fails before stream start, return local status
     `502` with the existing coarse upstream stream error envelope,
   - preserve existing client disconnect, idle timeout, invalid stream, event
     limit, and mid-stream error semantics.
8. Record health events:
   - one success event for the credential that completed the request,
   - one failure event for each failed upstream attempt,
   - include provider instance, credential ID, provider model, HTTP status when
     known, and normalized error class only.
   - use only these `event_class` values in this slice:
     `upstream_success`, `upstream_failure`,
   - do not write health failure events for local validation failures,
     unsupported-provider errors, missing local auth, missing upstream
     credentials, local response-writer failures, or `client_disconnected`.
9. Record fallback events:
   - one event for each credential switch,
   - buffer fallback decisions in memory while the request is running,
   - insert fallback events only after the local request metadata row is
     created,
   - require non-null `request_metadata_id` for chat fallback events,
   - include provider instance, model ID, from credential ID, to credential ID,
     reason `availability_retry`, and `allowed_by_policy = 1`,
   - never record cross-provider or cross-model fallback because this slice
     does not perform it.
10. Update request metadata:
   - `credential_id` is the final attempted credential,
   - `retry_count` is the number of extra attempts after the first credential,
   - `fallback_count` is the number of credential switches,
   - existing token, usage, latency, and stream metric fields continue to work.
11. Extend `serve --check` fake upstream coverage:
    - seed two API-key credentials for each API-key provider in the isolated DB,
    - verify no credential switch occurs while fallback policy is disabled,
    - explicitly enable fallback group `default`,
    - make the first credential fail with a retryable `503`,
    - verify the second credential succeeds for the same provider/model,
    - verify request metadata records retry/fallback counts,
    - verify `health_events.event_class` uses only
      `upstream_success`/`upstream_failure`,
    - verify `fallback_events.reason` is `availability_retry`,
    - verify chat fallback events have non-null `request_metadata_id`,
    - verify no fallback happens for `429`, `401`, malformed successful JSON, or
      too-large upstream bodies,
    - verify streaming fallback occurs before local stream start and never after
      a stream has started,
    - verify `health_events`, `fallback_events`, `request_metadata`, and check
      output do not contain API keys, raw upstream bodies, raw provider
      payloads, full request IDs, account IDs, prompts, or completions.

## Out of Scope

- Cross-provider fallback.
- Cross-model fallback.
- OpenRouter native `models` fallback or `route` controls.
- Fallback on quota, payment, abuse, policy, auth, or rate-limit failures.
- Provider-specific retry-after scheduling.
- TUI controls for enabling or disabling fallback groups.
- Background health scoring or credential avoidance based on prior events.
- OAuth/subscription credentials.
- Codex provider implementation or Codex credential import.
- TUI health/fallback views beyond data being present for a later view.
- Permanent test files.

## Design Constraints

- No permanent `*_test.go` files.
- `go test ./...` is used only as a package compile check.
- Provider adapters must not import SQLite, TUI, or routing policy code.
- Server must depend on narrow resolver and metadata interfaces, not concrete
  SQLite types.
- The resolver may return API-key secret material only to the server/provider
  adapter path. TUI and metadata paths receive metadata only.
- Request/response body bytes and SSE payload bytes may exist only transiently
  for parsing, validation, upstream forwarding, response writing, and safe
  usage extraction.
- Never log or persist prompts, completions, raw bodies, raw provider payloads,
  raw stream chunks, tool arguments/results, full bearer tokens, full provider
  request IDs, full account IDs, balances, credit totals, raw upstream error
  bodies, or raw fallback payloads.
- Fallback is disabled by default and must be explicitly enabled for a provider
  instance fallback group before more than one credential is attempted.
- Fallback is constrained to API-key credentials attached to the already
  requested configured provider instance and enabled fallback group. The
  provider model string is unchanged across attempts.
- Credential switching is allowed only for availability failures. It must not be
  used to evade quota, payment, auth, abuse, policy, provider privacy, or route
  constraints.
- Streaming fallback is allowed only before any local SSE data has been written.
  After the first local event, the chosen credential is fixed for that stream.
- Model discovery remains single-credential in this slice. Discovery cache
  fallback from slice 006 remains separate from chat credential fallback.

## Proposed Package Changes

```text
internal/credentials/
  upstream.go   # add ResolveAPIKeys and shared eligible credential semantics
internal/metadata/
  metadata.go   # add health/fallback metadata types
internal/server/
  server.go     # credential attempt loop and metadata recording
internal/storage/sqlite/
  migrations.go # explicit fallback group policy migration
  db.go         # list allowed credentials, health/fallback inserts
internal/app/
  app.go        # fake upstream smoke checks for fallback and health
```

Interface shape:

```go
type UpstreamCredentialResolver interface {
    ResolveAPIKey(ctx context.Context, providerInstanceID string) (ResolvedAPIKeyCredential, error)
    ResolveAPIKeys(ctx context.Context, providerInstanceID string) ([]ResolvedAPIKeyCredential, error)
}

type UpstreamCredentialManager interface {
    AddAPIKey(ctx context.Context, providerInstanceID, label, apiKey string) (UpstreamCredentialMetadata, error)
    List(ctx context.Context) ([]UpstreamCredentialMetadata, error)
    Disable(ctx context.Context, id int64) error
    EnableFallbackGroup(ctx context.Context, providerInstanceID, groupLabel string) error
    DisableFallbackGroup(ctx context.Context, providerInstanceID, groupLabel string) error
}

type MetadataRecorder interface {
    RecordRequestMetadata(context.Context, metadata.Request) (int64, error)
    RecordStreamMetrics(context.Context, metadata.Stream) error
    RecordHealthEvent(context.Context, metadata.HealthEvent) error
    RecordFallbackEvent(context.Context, metadata.FallbackEvent) error
}
```

Retryability helper:

```text
retryable availability failure =
  error class is upstream_network_error or upstream_timeout
  OR HTTP status is exactly 500, 502, 503, or 504
```

Non-retryable examples:

```text
400, 401, 402, 403, 404, 408, 422, 429,
upstream_invalid_response,
upstream_body_too_large,
upstream_stream_invalid,
upstream_stream_too_large,
upstream_event_limit,
client_disconnected,
local validation errors,
unsupported providers,
missing credentials
```

## Verification

Run:

```text
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
tmp="$(mktemp -d)"
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check
git diff --check
```

Manual review checks:

- no permanent test files exist,
- no fallback path changes provider instance or provider model,
- no fallback path attempts a second credential unless fallback group policy is
  explicitly enabled,
- no fallback happens for `429`, `401`, `402`, `403`, or malformed successful
  upstream responses,
- `fallback_events` contains only metadata IDs, credential IDs, and
  `availability_retry`,
- `health_events` contains only metadata, `upstream_success` or
  `upstream_failure`, and normalized error classes,
- chat fallback rows have non-null `request_metadata_id`,
- stream fallback cannot occur after `streamSink.started` is true,
- request metadata retry/fallback counts match the attempted credentials.

## Review Questions

1. Is availability-only credential fallback acceptable for the MVP without
   adding cross-provider/model policy machinery?
2. Are the non-retryable status classes strict enough to avoid quota/payment or
   policy evasion?
3. Is pre-stream-only fallback for streaming the right semantic boundary?
4. Are health/fallback metadata writes placed behind clean enough interfaces?
5. What must change before implementation?
