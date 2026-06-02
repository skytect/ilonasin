# 235 TUI Command Bar Density

## Context

The TUI now uses the intended top-level control-plane sections: API,
providers, usage, and logs. Pane-local scrolling and adaptive columns are in
place, but command guidance is still text-heavy:

- the footer is a long sentence on each tab;
- some pane bodies repeat local key maps;
- compact panes spend visible rows on instructions instead of state.

This slice should improve density without changing management behavior.

## Goal

Replace verbose command prose with a compact command bar and remove duplicated
pane-local key maps where the footer can carry the same actions.

## Scope

1. Keep the top-level tabs as API, providers, usage, and logs.
2. Keep existing keybindings and actions unchanged.
3. Replace `footerLine` with a compact key-hint command bar using existing TUI
   visual helpers where practical.
4. Include common controls in the footer:
   - section switching;
   - pane focus;
   - pane scrolling;
   - quit.
5. Include tab-specific controls:
   - API: new/disable local token;
   - providers: add/disable upstream key, OAuth login/refresh, fallback;
   - usage: subscription refresh;
   - logs: pruning.
6. Remove duplicated key-map blocks from pane bodies when those actions are
   now visible in the command bar.
7. Do not change management DTOs, routes, storage, provider behavior, auth,
   config mutation, logging policy, or server code.
8. Do not add permanent tests.
9. Do not modify or stage unrelated concurrent work.

## Non-Goals

- No new tab model.
- No new Bubble Tea dependency.
- No redesign of usage cards, quota bars, or provider account cards in this
  slice.
- No server-side route, metadata, or credential refresh changes.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
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
  if rg "tab switch|scroll pane|new local token|disable local token|up/down select token" "$tmp/manage-$cols.out"; then
    echo "old verbose command prose remains visible" >&2
    exit 1
  fi
done
```

Also inspect the diff before checks and run a disposable focused render smoke.
Remove temporary smoke artifacts before commit. The focused smoke must render
API, providers, usage, and logs at 80, 120, and 160 columns and assert:

- the footer line width does not exceed the viewport width after ANSI-aware
  measurement;
- common hints remain visible: `tab`, `1-4`, `[/]`, `j/k`, `pg`,
  `home/end`, and `q`;
- tab-specific hints remain visible: API `n` and `d`, providers `a`, `x`, `l`,
  `o/r`, and `f/F`, usage `u`, logs `p`;
- removed pane-local key maps are absent from API and providers pane bodies;
- old verbose footer phrases such as `tab switch`, `scroll pane`,
  `new local token`, `disable local token`, and `up/down select token` are
  absent.

## Acceptance

- Footer guidance is compact at narrow and wide widths.
- Pane bodies no longer spend rows on duplicated command prose.
- All existing keybindings remain routed to the same actions.
- No server, config, storage, route, provider, or logging behavior changes.
- Compile, vet, serve/manage smoke, whitespace checks, and implementation
  review pass.
