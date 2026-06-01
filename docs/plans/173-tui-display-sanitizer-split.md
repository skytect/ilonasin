# 173 TUI Display Sanitizer Split

## Context

`docs/ilonasin-architecture.md` requires the management TUI to render
metadata-only state and avoid prompts, completions, request bodies, response
bodies, raw streams, tool data, provider payloads, full tokens, provider request
IDs, full account IDs, balances, and credits.

Earlier TUI slices split rendering, action handling, model state, helpers, and
display helpers into focused files. `internal/tui/display.go` still mixes two
different concerns:

- formatting helpers such as credential, model, request route, and timestamp
  display,
- sanitizer policy helpers that decide what text fragments may be rendered.

Keeping sanitizer policy in its own same-package file makes the TUI privacy
boundary easier to audit without changing behavior.

## Goal

Move TUI display sanitizer helpers out of `display.go` into a dedicated
`internal/tui/display_sanitize.go` file without changing output.

After this slice:

- `display.go` owns display formatting helpers.
- `display_sanitize.go` owns display redaction and safe-fragment helpers.
- sanitizer regexes and function bodies are unchanged.

## Scope

1. Create `internal/tui/display_sanitize.go`.
2. Move these declarations intact from `display.go`:
   - `unsafeDisplayPattern`
   - `safeErrorMessagePattern`
   - `safeDisplay`
   - `safeTokenFragmentDisplay`
   - `safeEndpointDisplay`
   - `safeRefreshFailureDescriptionDisplay`
   - `safeRefreshFailureClass`
3. Keep these formatting helpers in `display.go`:
   - `credentialDisplay`
   - `healthModelDisplay`
   - `requestModelDisplay`
   - `formatTime`
   - `formatPreciseTime`
4. Do not change displayed strings, redaction patterns, truncation lengths,
   safe refresh classes, endpoint allowlist, time formats, TUI layout, key
   handling, management client calls, storage, provider behavior, config, or
   logging.
5. Do not add permanent tests.

## Out of Scope

- Redesigning the sanitizer policy.
- Adding new sanitizer tests.
- Changing the TUI rendering text.
- Changing management routes or DTOs.
- Changing provider, storage, config, or routing code.
- Splitting broader TUI files.

## Implementation Steps

1. Add `display_sanitize.go` with the moved sanitizer declarations.
2. Remove those declarations from `display.go`.
3. Update imports so `display.go` no longer imports `regexp`, `strings`, or
   `unicode` solely for sanitizer logic.
4. Run `gofmt`.
5. Review the diff to confirm this is move-only apart from imports.

## Smoke Checks

Run:

```sh
set -euo pipefail
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
tmp="$(mktemp -d)"
pid=""
cleanup() {
  if [ -n "$pid" ]; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
  rm -rf "$tmp" "$tmpbin"
}
trap cleanup EXIT
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
cfg="$tmp/config.toml"
cat >"$cfg" <<'EOF'
[server]
bind = "127.0.0.1:0"
[providers.codex]
type = "codex"
[providers.deepseek]
type = "deepseek"
[providers.openrouter]
type = "openrouter"
EOF
ILONASIN_HOME="$tmp/home" "$tmpbin/ilonasin" serve --config "$cfg" >"$tmp/serve.log" 2>&1 &
pid="$!"
for _ in $(seq 1 80); do
  sock="$(find "$tmp/home/run" -type s -name 'manage-*.sock' -print 2>/dev/null | head -n 1 || true)"
  if [ -n "$sock" ] &&
    curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/snapshot >/dev/null &&
    curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/subscription-usage >/dev/null; then
    break
  fi
  sleep 0.1
done
if [ -z "${sock:-}" ]; then
  echo "management socket not found"
  cat "$tmp/serve.log"
  exit 1
fi
curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/snapshot >/dev/null
curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/subscription-usage >/dev/null
set +e
printf '\tq' | timeout 3s script -q -e -c \
  "env ILONASIN_HOME='$tmp/home' '$tmpbin/ilonasin' manage --config '$cfg'" \
  "$tmp/manage.typescript" >/dev/null
manage_status="$?"
set -e
if [ "$manage_status" -ne 0 ] && [ "$manage_status" -ne 124 ]; then
  cat "$tmp/manage.typescript" 2>/dev/null || true
  exit "$manage_status"
fi
git diff --check
rg -n "unsafeDisplayPattern|safeErrorMessagePattern|func safeDisplay|func safeTokenFragmentDisplay|func safeEndpointDisplay|func safeRefreshFailureDescriptionDisplay|func safeRefreshFailureClass" internal/tui/display_sanitize.go
rg -n "func credentialDisplay|func healthModelDisplay|func requestModelDisplay|func formatTime|func formatPreciseTime" internal/tui/display.go
if rg -n "unsafeDisplayPattern|safeErrorMessagePattern|func safeDisplay|func safeTokenFragmentDisplay|func safeEndpointDisplay|func safeRefreshFailureDescriptionDisplay|func safeRefreshFailureClass" internal/tui/display.go; then
  echo "sanitizer helpers remain in display.go"
  exit 1
fi
```

## Acceptance

- TUI sanitizer helpers compile from `display_sanitize.go`.
- `display.go` retains only formatting helpers.
- Sanitizer patterns and behavior are unchanged.
- Direct compile, vet, serve, management route, and manage PTY smokes pass.
- The unrelated dirty `AGENTS.md` file is not staged or committed.
