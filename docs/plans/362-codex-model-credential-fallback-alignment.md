# 362 Codex Model Credential Fallback Alignment

## Context

Credential pooling is constrained to the requested provider instance, requested
provider model, and eligible credentials on that provider instance. Serving may
switch to another eligible credential on quota or availability pressure before
a response is committed.

Current chat execution keeps a separate `modelCredential` for Codex model
metadata discovery. The first attempt sets it to the actual attempted
credential, and auth retry updates it, but quota or availability fallback only
changes the chat credential. For Codex adapters, model discovery prefers
`ModelCredential`, so a fallback attempt can discover model metadata with one
OAuth account while sending the chat request with another.

## Goal

Keep Codex model metadata discovery aligned with the actual credential for
every chat attempt, including quota and availability fallback attempts.

## Scope

1. Update non-streaming chat execution so each reserved attempt uses that same
   credential as `modelCredential` before calling the provider adapter.
2. Update streaming chat execution with the same behavior.
3. Preserve refresh behavior:
   - refreshing the model credential still refreshes the same attempted
     credential when IDs match;
   - refreshing the attempted credential still updates `modelCredential` when
     IDs match.
4. Preserve fallback event recording, quota observations, health events,
   pressure reservation, retry selection, and metadata fields.
5. Do not change provider adapters, request parsing, storage, schema,
   management routes, TUI, config, or logging.

## Out Of Scope

- Changing model discovery for `/models`.
- Cross-provider or cross-model fallback.
- Persisted affinity or pressure state.
- New management or TUI surfaces.
- Permanent tests.

## Implementation Steps

1. In non-streaming execution, set `modelCredential` to the reserved
   `credential` for each attempt before handling pending fallback events and
   calling `completeChatAttempt`.
2. In streaming execution, mirror the same assignment before calling
   `streamChatAttempt`.
3. Remove any now-redundant fallback-reason-specific model credential update.
4. Review refresh branches to confirm refreshed credentials remain paired.
5. Review the diff for same-provider/model constraints and unchanged metadata
   behavior.

## Verification

Use temporary focused checks, then remove them before commit. They must cover:

- non-streaming quota or availability fallback where the first credential fails
  before response commit, the second credential is attempted, and the adapter
  receives matching `Credential.ID` and `ModelCredential.ID` on that fallback
  attempt;
- streaming quota or availability fallback where the first credential fails
  before response commit, the second credential is attempted, and the adapter
  receives matching `Credential.ID` and `ModelCredential.ID` on that fallback
  attempt.

Run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/server
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage` at narrow and wide widths, and
cleaning up all temporary files and processes.

## Acceptance

- Every non-streaming fallback attempt sends matching chat and model
  credentials to the provider adapter.
- Every streaming fallback attempt sends matching chat and model credentials to
  the provider adapter.
- Auth refresh behavior remains paired and retry behavior is unchanged.
- Privacy, same-provider/model routing, quota filtering, fallback metadata,
  health recording, provider behavior, storage, management, TUI, config, and
  logging are otherwise unchanged.
