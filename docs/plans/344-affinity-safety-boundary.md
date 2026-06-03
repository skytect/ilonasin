# 344 Affinity Safety Boundary

## Context

`docs/ilonasin-architecture.md` constrains credential pooling to same provider
instance and same provider model, and it forbids storing full upstream account
IDs, tokens, request IDs, or other sensitive identifiers in metadata, logs, TUI,
or management snapshots. Plans 342 and 343 added strict safety checks for
header fallback and Responses body-derived affinity.

The strict affinity value predicate now exists twice:

- `internal/openai/responses.go` has `safeResponsesAffinityValue`.
- `internal/server/request_affinity.go` has `safeRequestAffinityValue`.

They intentionally match today, but keeping two marker lists for routing
privacy is easy to drift. `internal/privacy` already owns small shared privacy
helpers used across credentials, management, and TUI, so the strict affinity
safety predicate should live there.

## Goal

Move the strict affinity value safety predicate into a shared privacy boundary
without changing routing behavior.

## Scope

1. Add a small exported helper in `internal/privacy` for strict local affinity
   value safety.
2. Replace `safeResponsesAffinityValue` in `internal/openai/responses.go` with
   the shared helper.
3. Replace `safeRequestAffinityValue` in
   `internal/server/request_affinity.go` with the shared helper.
4. Preserve exact behavior for:
   - empty and oversized values;
   - JSON-like values;
   - JWT-like values;
   - account, device, bearer, token, secret, authorization, OAuth, API-key,
     request-id, and request-shaped markers;
   - existing `x-codex-window-id` colon-suffix trimming before validation.
5. Do not change Chat Completions body-affinity behavior. It should keep using
   the existing `safeChatAffinityValue` in this slice.

## Boundaries

- No credential-pool ordering changes.
- No request parsing, validation, public API, provider, storage, schema,
  management, TUI, metadata, logging, or config changes.
- No new stored or rendered affinity field.
- No broad privacy scrubber refactor.
- No permanent tests.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./internal/privacy ./internal/openai ./internal/server
go test ./...
go vet ./...
```

Run a temporary focused smoke, then remove it before commit:

- assert the shared helper accepts ordinary stable IDs;
- assert it rejects empty, oversized, JSON-like, JWT-like, account, device,
  bearer, token, secret, authorization, OAuth, API-key, request-id, and
  request-shaped markers;
- assert Responses top-level `prompt_cache_key` and `client_metadata`
  affinity still use the same accepted/rejected values;
- assert server header fallback still trims `x-codex-window-id` before applying
  the shared helper;
- assert Chat Completions body-affinity behavior is unchanged by this slice,
  including at least one value currently accepted by Chat but rejected by the
  strict shared helper, such as a `session_id` containing `request_id`.

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO capture
   disabled, keepalive disabled, and at least two provider instances.
3. Verify the management health endpoint over the management socket.
4. Run `manage` under a short timeout and verify the TUI renders.
5. Remove all temporary artifacts and stop background processes.

During diff review, explicitly verify:

- the strict marker list has a single implementation;
- no Chat body-affinity behavior changed;
- no metadata, logging, provider request, storage, management, or TUI surfaces
  gained an affinity value;
- no permanent smoke files remain.

## Acceptance

- Strict local affinity safety is owned by `internal/privacy`.
- Responses body affinity and server header fallback use the same helper.
- Behavior remains unchanged for existing accepted and rejected values.
- Compile, vet, focused smoke, serve smoke, manage smoke, senior plan review,
  and senior implementation review pass.
