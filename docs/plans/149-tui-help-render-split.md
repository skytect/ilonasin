# 149 TUI Help Render Split

## Context

Plans 103 and 106 through 148 split TUI rendering, model state, lifecycle,
shared helpers, key dispatch, account actions, OAuth actions, viewport
mechanics, observability render sections, and pruning rendering.

`internal/tui/overview.go` still owns both overview rendering and help
rendering. The architecture treats `ilonasin manage` as a first-class local
management UI with predictable, auditable controls. Keeping static help text in
its own render file makes the overview renderer easier to audit around provider
instances, model cache, and metadata-only summaries, while keeping key-binding
documentation separate.

## Goal

Move help rendering out of `overview.go` into a focused same-package helper
file without changing behavior.

After this slice:

- `overview.go` owns overview rendering and model-cache summaries.
- `help.go` owns help tab rendering.

## Scope

1. Add `internal/tui/help.go`.
2. Move the existing `writeHelp` method from `overview.go` into the new file
   unchanged.
3. Keep all help text, ordering, privacy statement wording, and method
   receiver unchanged.
4. Keep layout tab dispatch unchanged: `layout.go` still calls
   `m.writeHelp(&b)` for the help tab.
5. Do not change key handling, actions, management clients, snapshot loading,
   provider adapters, config, routing, storage, or TUI behavior.
6. Do not add permanent tests.
7. Do not push.

## Non-Goals

- No visual redesign.
- No key-binding changes.
- No management route, provider, config, storage, routing, or action changes.
- No model-cache summary split in this slice.
- No reintroduction of `serve --check` or `manage --check`.

## Implementation

1. Create `internal/tui/help.go` with `package tui`.
2. Move `writeHelp` unchanged from `overview.go`.
3. Remove any now-unused imports from `overview.go`.
4. Keep imports minimal in the new file.
5. Run `gofmt`.
6. Review the diff to confirm this is relocation only plus import cleanup.
7. Review the PTY smoke transcript to confirm the help tab still renders
   `Keys` and the privacy statement.

## Smoke Checks

Run:

```sh
set -euo pipefail
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
tmp="$(mktemp -d)"
cleanup() {
  if [ -n "${pid:-}" ]; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
  rm -rf "$tmp" "$tmpbin"
}
trap cleanup EXIT
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
cfg="$tmp/config.toml"
cat >"$cfg" <<EOF
[server]
bind = "127.0.0.1:0"
[providers.codex]
type = "codex"
[providers.deepseek]
type = "deepseek"
[providers.openrouter]
type = "openrouter"
EOF
ILONASIN_HOME="$tmp/home" "$tmpbin/ilonasin" serve --config "$cfg" \
  >"$tmp/serve.log" 2>&1 &
pid="$!"
for _ in $(seq 1 80); do
  sock="$(find "$tmp/home/run" -type s -name 'manage-*.sock' -print 2>/dev/null | head -n 1 || true)"
  if [ -n "$sock" ] && curl --silent --fail --unix-socket "$sock" \
    http://ilonasin/_ilonasin/manage/snapshot >/dev/null && \
    curl --silent --fail --unix-socket "$sock" \
    http://ilonasin/_ilonasin/manage/subscription-usage >/dev/null; then
    break
  fi
  sleep 0.1
done
if [ -z "${sock:-}" ]; then
  echo "management socket not found"
  cat "$tmp/serve.log"
  exit 1
fi
curl --silent --fail --unix-socket "$sock" \
  http://ilonasin/_ilonasin/manage/snapshot >/dev/null
curl --silent --fail --unix-socket "$sock" \
  http://ilonasin/_ilonasin/manage/subscription-usage >/dev/null
set +e
printf '\t\t\tq' | timeout 3s script -q -e -c \
  "env ILONASIN_HOME='$tmp/home' '$tmpbin/ilonasin' manage --config '$cfg'" \
  "$tmp/manage-help.typescript" >/dev/null
manage_status="$?"
set -e
if [ "$manage_status" -ne 0 ] && [ "$manage_status" -ne 124 ]; then
  cat "$tmp/manage-help.typescript" 2>/dev/null || true
  exit "$manage_status"
fi
if ! grep -q "Keys" "$tmp/manage-help.typescript" ||
  ! grep -q "Privacy" "$tmp/manage-help.typescript" ||
  ! grep -q "The TUI renders snapshot metadata" "$tmp/manage-help.typescript"; then
  echo "help render smoke failed"
  cat "$tmp/manage-help.typescript"
  exit 1
fi
git diff --check
```

Acceptance:

- Compile/package check passes.
- Vet passes.
- Existing permanent test-file inventory is reviewed.
- Fresh binary builds.
- Direct `serve` smoke starts the daemon and exposes snapshot and subscription
  usage management routes.
- Direct `manage` smoke runs in a pseudo-terminal, navigates to the help tab,
  renders key help and the privacy statement, and exits cleanly or times out
  with status 124. Any other status fails the smoke.
- `git diff --check` passes.
- Moved-code diff review confirms `writeHelp` is unchanged except for the new
  file location and imports.

## Review Questions

1. Is help rendering the right next extraction from `overview.go`?
2. Should model-cache summary helpers remain with `writeOverview` for this
   slice because they are only used there?
3. Is the help-tab PTY smoke sufficient for this relocation-only split?
