# Plan 002: Local API Token Management

## Goal

Implement the first useful management path for ilonasin local client tokens so
`ilonasin serve` can be used without any auth bypass or default reusable token.
The TUI remains the local control plane and mutates SQLite only.

## Architecture Inputs

- `docs/ilonasin-architecture.md`
- `docs/codex-auth.md`
- `docs/deepseek-openrouter-comparison.md`
- Slice 001 code and plan.

## Scope

1. Add a credentials service for local client token lifecycle:
   - create a high-entropy token,
   - compute a domain-separated hash,
   - store only a one-way hash plus display metadata,
   - list token metadata,
   - disable a token by local ID.
   SQLite must never receive or hash raw local token plaintext.
2. Keep local ilonasin tokens strictly separate from upstream provider
   credentials. No provider API key or OAuth token can authenticate to the local
   OpenAI-compatible API.
3. Extend `ilonasin manage` with a real Bubble Tea view for local API tokens:
   - show token labels, prefixes/last4, created time, and disabled state,
   - create a token with a default label,
   - show the full generated token exactly once in the TUI session,
   - disable the selected token,
   - never write generated token plaintext to SQLite, logs, config, or tests.
4. Add a headless TUI test/check path that exercises the same model update logic
   for token creation and disabling without needing a terminal. This check must
   assert behavior through state and SQLite metadata, and must not print the
   generated plaintext token to stdout, stderr, test logs, snapshots, or failure
   messages.
5. Extend `serve --check` to verify that:
   - a token created through the same token service in an isolated temporary DB
     authenticates `/v1/models`,
   - disabling that token through the same service causes `/v1/models` to return
     `401`,
   - the selected home DB remains free of check-created client token rows.
6. Keep config static. The TUI must not edit `config.toml`.
7. Preserve the existing metadata-only storage boundary.

## Out of Scope

- Upstream provider API keys.
- OAuth flows.
- Admin sockets or HTTP admin APIs.
- Token pruning policies.
- Cross-user or multi-profile support.
- Copy-to-clipboard integration.

## Design Constraints

- Generated local client tokens must have at least 256 bits of entropy.
- Generated local client tokens must use an ilonasin-only format prefix, for
  example `iln_`.
- SHA-256 token hashes remain acceptable only for generated high-entropy tokens.
- Hashing must be domain-separated, for example
  `sha256("ilonasin-local-client-v1\x00" + token)`.
- The verifier must reject non-local token formats before DB lookup.
- Never compare plaintext tokens. Compare fixed-length digest strings with
  `subtle.ConstantTimeCompare` where comparison is performed.
- Token plaintext may exist only in process memory long enough to display it
  once after creation.
- In Bubble Tea terms, plaintext may exist only in a transient create-result
  reveal state. It must never be copied into list state, repository state,
  config state, logs, or test output, and must be cleared on dismiss, next
  action, navigation away from the reveal, or quit.
- Tests must not assert on or print full generated tokens.
- The token service should live outside the TUI so future admin surfaces can
  reuse it without duplicating token logic.
- `internal/server` must not import `internal/storage/sqlite`.

## Proposed Package Changes

```text
internal/credentials/
  service.go        # token lifecycle service and repository interfaces
internal/storage/sqlite/
  client_tokens.go # SQLite implementation
internal/tui/
  model.go         # token management model/view/update
```

`internal/tui` may depend on the token service interface, but not on SQLite
details. `internal/app` wires the SQLite repository into the TUI and smoke
checks.

Service and repository contracts:

```go
type LocalTokenManager interface {
    Create(ctx context.Context, label string) (CreatedLocalToken, error)
    List(ctx context.Context) ([]LocalTokenMetadata, error)
    Disable(ctx context.Context, id int64) error
}

type LocalTokenVerifier interface {
    VerifyBearer(ctx context.Context, authorization string) (VerifiedLocalToken, error)
}

type LocalTokenRepository interface {
    InsertLocalToken(ctx context.Context, meta NewLocalTokenMetadata) (LocalTokenMetadata, error)
    ListLocalTokens(ctx context.Context) ([]LocalTokenMetadata, error)
    DisableLocalToken(ctx context.Context, id int64, disabledAt time.Time) error
    FindLocalTokenByHash(ctx context.Context, hash string) (LocalTokenAuthRecord, error)
}
```

Repository semantics:

- `NewLocalTokenMetadata` contains only hash, prefix, last4, label, and
  timestamps. It must not contain plaintext.
- `LocalTokenMetadata` contains ID, label, prefix, last4, created time, disabled
  time, and disabled bool. It must not contain `TokenHash`.
- `LocalTokenAuthRecord` may contain hash for verification, but is not exposed
  to the TUI.
- `List` returns enabled and disabled tokens in deterministic order, newest
  first or oldest first as long as the choice is documented in code.
- `Disable` is idempotent for already disabled rows and returns a not-found
  error for missing IDs.
- `VerifyBearer` rejects disabled rows immediately.
- `internal/tui` receives `LocalTokenManager`.
- `internal/server` receives only `LocalTokenVerifier`. It must not receive
  create/list/disable methods.
- `internal/app` may wire both interfaces to the same concrete credentials
  service.

## Verification

Run:

```text
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
tmp="$(mktemp -d)"
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check
```

Automated and manual review checks:

- no raw token column exists,
- no SQLite method accepts raw local token plaintext,
- no logs, smoke checks, or tests print full generated tokens,
- no config file edits happen from the TUI path,
- disabling a token changes auth behavior to `401`.
- a provider credential secret row cannot authenticate to the local
  OpenAI-compatible API,
- `client_tokens` has no raw token column,
- generated token format has at least 32 random bytes of entropy,
- `manage --check` output does not contain the generated token.

## Review Questions

1. Is the TUI/token-service boundary clean enough for future admin surfaces?
2. Does the one-time plaintext token display rule avoid creating secret-storage
   debt?
3. Are the smoke tests strong enough to prove token creation and disabling
   behavior without adding a separate CLI admin command?
4. Is anything in this slice likely to blur local API auth with upstream
   provider credentials?
