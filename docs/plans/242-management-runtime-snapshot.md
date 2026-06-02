# 242 Management Runtime Snapshot

## Context

`docs/ilonasin-architecture.md` says `ilonasin manage` should be a client of the
daemon-owned management API. Recent TUI slices moved provider rows and provider
selection to management snapshot DTOs, but the TUI still stores raw
`config.Config` and reads it for:

- daemon bind display;
- local API summary bind chip;
- IO capture policy display in logs.

Those values are daemon runtime state. The TUI should get them from the
management snapshot, not from config carried into the client model.

## Goal

Expose a small management runtime DTO in `ManagementSnapshotResponse` and make
the TUI render bind and IO policy from that DTO.

## Scope

1. Add a management DTO such as:
   - `RuntimeStatus.Bind`;
   - `RuntimeStatus.CaptureIO`.
2. Add a `Runtime` or similarly named field to `ManagementSnapshotResponse`.
3. Add corresponding fields to `management.Service`, populated by the app when
   starting the management server:
   - server bind from config;
   - capture IO enabled from the IO logger/config state.
4. Sanitize runtime DTO strings through the management snapshot sanitizer.
5. Add a TUI runtime field populated from `snapshot.Runtime`.
6. Change TUI header/API summary/logs policy rendering to use the runtime DTO
   instead of `m.cfg.Server.Bind` and `m.cfg.Logging.CaptureIO`.
7. Remove raw `config.Config` from `Model`, `NewModel`, and `tui.Run` if no TUI
   code needs it after this migration.
8. Update `app.Manage` call site accordingly.
9. Preserve visible behavior: bind and IO capture state should render the same
   values as before when the daemon snapshot contains them.
10. Do not change config loading, provider registry construction, server
    listener binding, IO logging semantics, storage schema, public API behavior,
    provider adapters, or TUI layout/navigation.
11. Do not add permanent tests.

## Non-Goals

- No management route additions beyond extending the existing snapshot response.
- No config mutation.
- No IO logging behavior change.
- No route preflight refactor.
- No TUI visual redesign.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
if rg -n 'config\\.Config|m\\.cfg|cfg\\.Server|cfg\\.Logging|Server\\.Bind|Logging\\.CaptureIO' internal/tui; then
  echo "TUI config runtime references remain" >&2
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
snapshot="$(curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/snapshot)"
printf '%s' "$snapshot" | jq -e --arg bind "127.0.0.1:$port" '.runtime.bind == $bind and .runtime.capture_io == false' >/dev/null
for cols in 80 120 160; do
  timeout 4s script -q -e -c "stty cols $cols rows 32; exec env ILONASIN_HOME=$tmp/home $tmpbin/ilonasin manage --config $tmp/config.toml" /dev/null >"$tmp/manage-$cols.out" || true
  rg "api|providers|usage|logs" "$tmp/manage-$cols.out" >/dev/null
  rg "127.0.0.1:$port|metadata-only|capture" "$tmp/manage-$cols.out" >/dev/null
done
```

Also run a temporary focused TUI/management smoke, then remove it before
commit. It should:

- build a `management.Service` with runtime status and assert
  `LoadManagementSnapshot` emits `runtime.bind` and `runtime.capture_io`;
- assert unsafe bind-like strings are sanitized or redacted by snapshot
  sanitizer;
- construct a TUI model from a snapshot and assert bind and IO policy render
  from `snapshot.Runtime`, not config;
- assert no raw config type or `m.cfg` references remain under `internal/tui`.

Remove temporary smoke files and artifacts before commit.

## Acceptance

- Management snapshot exposes daemon runtime bind and IO capture state.
- TUI renderers use snapshot runtime DTO values for bind and IO policy.
- `internal/tui` no longer stores or receives raw `config.Config`.
- Existing CLI behavior, management actions, storage, provider routing, IO
  logging semantics, and public APIs are unchanged.
- Compile, vet, whitespace checks, serve/manage smoke, focused smoke, and
  implementation reviews pass.
