# 385 Responses Metadata Affinity

## Context

The architecture says credential affinity should use fields that real clients
already send, then fall back to local token, provider, model, in-flight
pressure, and cursor state when no safe signal exists.

Current code already supports:

- Responses body `prompt_cache_key`;
- Codex-style Responses `client_metadata` allowlisted keys;
- fallback `session-id` and `thread-id` headers.

The remaining mismatch is generic OpenAI Responses metadata. Official Responses
clients expose top-level `metadata` and `prompt_cache_key`; Codex-specific
clients expose `client_metadata`. The current architecture table and parser only
name/use `client_metadata` for generic Responses affinity.

## Goal

Accept safe top-level Responses `metadata` as an affinity source without storing,
logging, forwarding, or displaying raw metadata values.

## Scope

1. Add a local-only `Metadata map[string]string` field to
   `openai.ResponsesRequest`.
2. Allow top-level `metadata` in `validateResponsesTopLevelKeys`.
3. Parse top-level `metadata` with the same bounded string-pair rules used for
   `client_metadata`.
4. Keep affinity priority:
   - safe body `prompt_cache_key`;
   - selected safe `client_metadata` keys for Codex compatibility;
   - selected safe top-level `metadata` keys for generic Responses clients.
5. Keep Responses `metadata` local-only through Responses-to-Chat conversion:
   do not set `ChatCompletionRequest.Metadata` or `PresentFields["metadata"]`
   from Responses metadata.
6. Reuse the existing selected key allowlist:
   - `prompt_cache_key`;
   - `session_id`;
   - `thread_id`;
   - `conversation_id`.
7. Extend IO capture scrubbing so request body `metadata`, `client_metadata`,
   `session_id`, `thread_id`, and `conversation_id` values are redacted. This
   is a capture-only privacy guard; it must not make these values visible in
   normal metadata or TUI output.
8. Update `docs/ilonasin-architecture.md` and
   `docs/codex-compatibility-audit.md` so generic Responses names top-level
   `metadata`, while Codex app-server remains `client_metadata`.

## Out Of Scope

- No provider adapter behavior changes.
- No forwarding Responses `metadata` or `client_metadata` upstream.
- No durable metadata, logs, management API, or TUI exposure of affinity values.
- No support for stateful Responses fields, response retrieval, or broader
  Responses parity.
- No use of request-id, installation-id, account-id, device-id, token, or secret
  shaped values as affinity.

## Verification

Run:

```sh
rg -n "metadata|client_metadata|prompt_cache_key|AffinityKey" internal/openai/responses.go docs/ilonasin-architecture.md docs/codex-compatibility-audit.md
git diff --check
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run a temporary direct parser smoke with `go test` or a temporary Go program
that proves:

- `metadata.session_id` is accepted as Responses affinity when
  `prompt_cache_key` and `client_metadata` are absent;
- `prompt_cache_key` wins over `metadata.session_id`;
- `client_metadata.thread_id` wins over `metadata.session_id`;
- unsafe metadata values are ignored for affinity;
- a marshaled upstream Chat request produced from a Responses request contains
  neither `metadata` nor the raw affinity value.
- IO body scrubbing redacts `metadata.session_id`, `client_metadata.thread_id`,
  top-level `session_id`, `thread_id`, and `conversation_id`;
- no temporary files remain.

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage` at narrow and wide terminal
widths, and cleaning up all temporary files and processes.

## Acceptance

- Generic Responses clients can supply top-level `metadata` for safe affinity.
- Codex Responses `prompt_cache_key` and `client_metadata` behavior is preserved.
- Invalid or unsafe affinity-shaped metadata does not become visible in logs,
  request metadata, management snapshots, or TUI output.
- The architecture and compatibility docs distinguish generic Responses
  `metadata` from Codex `client_metadata`.
- Compile/package checks, vet, parser smoke, and serve/manage smoke pass.
