# 489 Routing Policy Snapshot

## Context

`docs/ilonasin-architecture.md` says credential pooling must remain auditable
through metadata-only management surfaces. The routing implementation already
uses same-provider/same-model credential pools, safe affinity signals, quota
blocks, in-flight pressure, and token-scoped cursor tie breaking. The TUI shows
credential pool groups, but it does not show the active routing strategy or
cache-affinity status as daemon-owned state.

Earlier TUI feedback also asked for a visible caching/routing strategy status.
This should be exposed from the daemon management snapshot rather than inferred
inside the TUI.

## Goal

Expose safe, static routing policy metadata in management snapshots and render
it compactly in the Providers pane.

## Scope

1. Add a `RoutingPolicyStatus` DTO to `internal/management/snapshot_dto.go`.
2. Populate it from `management.Service.LoadManagementSnapshot`.
3. Include only safe policy metadata:
   - `scope`: same provider and same model;
   - `pooling`: enabled;
   - `affinity`: safe client signal, otherwise local token plus route;
   - `pressure`: least in-flight;
   - `tie_breaker`: token-scoped cursor;
   - `quota`: quota blocks considered during routing;
   - `fallback`: same provider/model credential fallback only;
   - `cache`: prompt cache key is preferred when safely provided;
   - `exposes_body_values`: false.
4. Render one compact routing/pooling strip in the TUI Providers pane near
   credential pool groups.
5. Do not change routing behavior, credential selection, affinity extraction,
   quota handling, storage schema, config, local API routes, logs, or provider
   adapters.
6. Do not add permanent tests.

The DTO must expose static enum/string/boolean policy labels only. It must not
expose live pool state, client-provided affinity values, prompt cache key
values, session IDs, user IDs, client metadata values, route-derived
per-request keys, local token IDs, local token labels, local token fragments,
credential IDs, credential labels, account labels, account emails, account
hashes, request IDs, provider request IDs, prompts, completions, tool payloads,
or request bodies. It should describe policy, not live per-request state.

`exposes_body_values` means the management snapshot exposes no values from
request bodies. It must not be interpreted as saying routing ignores safe
body-provided affinity signals such as `prompt_cache_key`.

## Verification

Run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run direct CLI smokes with a temporary binary and isolated home:

- start `ilonasin serve`;
- create at least one local token over the management socket;
- send a sentinel-bearing local request containing `prompt_cache_key`,
  `session_id`, `user`, selected `metadata` or `client_metadata`, a distinctive
  local token label, and prompt text;
- fetch `/_ilonasin/manage/snapshot` and confirm `routing_policy` contains the
  safe static strategy fields and none of the sentinel prompt-cache, session,
  user, client metadata, local token, credential, account, request ID, provider
  ID, prompt, or body values;
- run bounded `ilonasin manage` through a PTY and confirm the Providers pane
  renders the routing strip without reading SQLite directly;
- while `ilonasin manage` is running, create another local token over the
  management socket and confirm the pane updates from the live daemon snapshot
  without pressing a manual refresh key;
- stop the daemon and remove the temporary home.

## Acceptance

- Management snapshot exposes safe routing/pooling strategy metadata.
- TUI renders routing strategy status compactly from the snapshot.
- No routing behavior changes.
- No new mutable management operation, direct TUI SQLite access, config
  mutation, body capture, or secret-bearing field is introduced.
