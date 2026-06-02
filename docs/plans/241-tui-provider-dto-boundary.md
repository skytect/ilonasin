# 241 TUI Provider DTO Boundary

## Context

`docs/ilonasin-architecture.md` says `ilonasin manage` should be a client of the
daemon-owned management API. Provider instances are configured in `config.toml`,
but the TUI should consume daemon-owned management snapshots and use management
operations for mutation.

Recent TUI work already moved mutable operations to the management client and
keeps the TUI from editing `config.toml`. One stale boundary remains:

- `tui.Run` and `NewModel` still accept a `provider.Registry`;
- `Model` stores `provider.Registry`;
- `Model` stores provider rows as `[]provider.Instance`;
- `applySnapshot` converts `management.ProviderInstance` rows back into
  `provider.Instance`;
- API-key and OAuth provider selection helpers read `m.registry`.

The management snapshot already exposes the provider fields the TUI needs:
provider ID, type, base URL, auth style, API-key support, OAuth support, refresh
support, chat support, and model discovery support.

## Goal

Make the TUI provider display and provider-selection actions use management
snapshot DTOs directly instead of provider-domain registry objects.

## Scope

1. Change `Model.providers` from `[]provider.Instance` to
   `[]management.ProviderInstance`.
2. Remove the `provider.Registry` field from `Model`.
3. Remove the `registry provider.Registry` parameter from `tui.Run` and
   `NewModel`, then update the `app.Manage` call site.
4. In `applySnapshot`, copy `snapshot.Providers` directly and delete
   `providersFromSnapshot`.
5. Update provider rendering to use `management.ProviderInstance`.
6. Update header/API summary provider counts to use snapshot provider rows
   (`len(m.providers)`), not `cfg.Providers`.
7. Update API-key provider selection:
   - use snapshot provider rows;
   - select the first provider with `APIKey`;
   - preserve existing error behavior when none exists.
8. Update OAuth login provider selection:
   - use snapshot provider rows;
   - select the first Codex OAuth provider;
   - preserve existing error behavior when none exists.
9. Update upstream/fallback visibility helpers to use snapshot provider rows.
10. Preserve existing TUI visible behavior, keybindings, pane layout, management
   clients, and privacy redaction.
11. Do not change management DTOs, management routes, storage, provider
    adapters, server routes, config loading, logging, subscription keepalive,
    public API behavior, or TUI visual layout.
12. Do not add permanent tests.

## Non-Goals

- No removal of `config.Config` from the TUI in this slice. The TUI still uses
  config-derived bind and IO-capture state until those are exposed through a
  management DTO.
- No provider registry changes outside the TUI.
- No server route preflight refactor.
- No TUI visual redesign.
- No management API additions.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
if rg -n 'registry|provider\\.Registry|provider\\.Instance|providersFromSnapshot' internal/tui; then
  echo "provider-domain TUI references remain" >&2
  exit 1
fi
git diff --check
go test ./...
go vet ./...
tmp=$(mktemp -d)
tmpbin="$tmp/bin"
mkdir -p "$tmpbin"
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
port=$(python - <<'PY'
import socket
s=socket.socket()
s.bind(('127.0.0.1',0))
print(s.getsockname()[1])
s.close()
PY
)
cat >"$tmp/config.toml" <<EOF
[server]
bind = "127.0.0.1:$port"

[paths]
database = "$tmp/home/ilonasin.sqlite"
log_dir = "$tmp/home/logs"
cache_dir = "$tmp/home/cache"

[logging]
capture_io = false

[subscription_keepalive]
enabled = false

[providers.deepseek]
type = "deepseek"

[providers.codex]
type = "codex"
EOF
cleanup() {
  if [ -n "${pid:-}" ]; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
  rm -rf "$tmp"
}
trap cleanup EXIT
ILONASIN_HOME="$tmp/home" "$tmpbin/ilonasin" serve --config "$tmp/config.toml" >"$tmp/serve.log" 2>&1 &
pid=$!
for i in $(seq 1 50); do
  if [ -d "$tmp/home/run" ] && find "$tmp/home/run" -name 'manage-*.sock' -type s | rg . >/dev/null; then
    break
  fi
  sleep 0.1
done
sock="$(find "$tmp/home/run" -name 'manage-*.sock' -type s | head -n 1)"
test -S "$sock"
curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/snapshot >/dev/null
for cols in 80 120 160; do
  timeout 4s script -q -e -c "stty cols $cols rows 32; exec env ILONASIN_HOME=$tmp/home $tmpbin/ilonasin manage --config $tmp/config.toml" /dev/null >"$tmp/manage-$cols.out" || true
  rg "api|providers|usage|logs" "$tmp/manage-$cols.out" >/dev/null
done
```

Also run a temporary focused TUI smoke, then remove it before commit. It should
construct a `Model` with seeded `management.ProviderInstance` rows and assert:

- provider rendering still shows provider ID, type, auth style, base URL, chat,
  discovery, API-key, OAuth, and refresh state;
- header/API summary provider counts come from seeded snapshot provider rows,
  not `cfg.Providers`;
- `firstAPIKeyProvider` chooses the first API-key-capable snapshot row;
- `firstOAuthLoginProvider` chooses the first Codex OAuth snapshot row;
- upstream credential visibility keeps only API-key-capable provider rows;
- no `provider.Registry`, `provider.Instance`, or `providersFromSnapshot`
  references remain under `internal/tui`;
- unsafe provider labels are redacted by existing display helpers.

Remove temporary smoke files and artifacts before commit.

## Acceptance

- TUI provider display and provider-selection actions consume management
  provider DTOs directly.
- `internal/tui` no longer imports provider-domain types for provider rows or
  registry selection.
- Existing management operations, keybindings, pane layout, and visible provider
  behavior are preserved.
- No management, storage, provider, server, config, logging, subscription, or
  public API behavior changes.
- Compile, vet, whitespace checks, serve/manage smoke, focused render smoke,
  and implementation reviews pass.
