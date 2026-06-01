# 186 Remove Codex Account Pooling Config

## Context

`docs/ilonasin-architecture.md` now defines credential pooling as default
same-provider-instance and same-model routing behavior. Plan 097 explicitly
stopped requiring `codex_account_pooling = true` in `config.toml`.

The code still exposes `codex_account_pooling` in `config.ProviderConfig` and
copies it into `provider.Instance.CodexAccountPooling`, but no serving,
management, TUI, storage, or provider code reads that instance field for
behavior. This leaves stale config surface from the old opt-in account pooling
design.

## Goal

Remove the unused Codex account-pooling config and provider instance fields so
the config surface matches the current architecture.

After this slice:

- `config.ProviderConfig` no longer has `CodexAccountPooling`,
- `provider.Instance` no longer has `CodexAccountPooling`,
- `provider.NewRegistry` no longer copies `codex_account_pooling`,
- `codex_account_pooling` has no code references outside historical docs/plans,
- default API-key and Codex OAuth credential pooling behavior is unchanged,
- no storage, management route, TUI, provider adapter, request routing, or
  fallback metadata behavior changes.

## Scope

1. Update `internal/config/config.go`.
   - Remove `CodexAccountPooling bool` with the
     `toml:"codex_account_pooling"` tag from `ProviderConfig`.
2. Update `internal/provider/provider.go`.
   - Remove `CodexAccountPooling` from `Instance`.
   - Remove assignment in `NewRegistry`.
3. Run source guards that reject live-code references to
   `CodexAccountPooling` and `codex_account_pooling`.
4. Do not edit historical plans or external research docs.
   - Those documents are records of earlier slices and provider research, and
     some mention the old opt-in gate as historical context.
5. Do not add dependencies or permanent tests.

## Out of Scope

- Removing fallback-policy management routes or TUI controls.
- Renaming credential group metadata.
- Changing default pooling behavior.
- Changing TOML unknown-key handling globally.
- Editing architecture docs, because they already describe default pooling.

## Implementation Steps

1. Remove the unused config/provider fields.
2. Run `gofmt`.
3. Review the diff before smoke checks.
4. Run compile, vet, direct `serve` and `manage` smokes, source guards, and
   whitespace checks.

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
set +e
printf 'q' | timeout 3s script -q -e -c \
  "sh -c 'stty cols 100 rows 40; exec env ILONASIN_HOME=\"$tmp/home\" \"$tmpbin/ilonasin\" manage --config \"$cfg\"'" \
  "$tmp/manage.typescript" >/dev/null
manage_status="$?"
set -e
if [ "$manage_status" -ne 0 ] && [ "$manage_status" -ne 124 ]; then
  cat "$tmp/manage.typescript"
  exit "$manage_status"
fi
rg -q 'overview' "$tmp/manage.typescript"
if rg -n 'CodexAccountPooling|codex_account_pooling' internal cmd; then
  exit 1
fi
git diff --check
```

## Acceptance

- Live code no longer exposes the stale `codex_account_pooling` config surface.
- Provider instances no longer carry an unused `CodexAccountPooling` field.
- Default pooling behavior remains unchanged because the removed field had no
  live behavioral readers.
- Compile, vet, direct `serve`, direct `manage`, source guards, and whitespace
  checks pass.
- Existing unrelated files are not staged or committed.
