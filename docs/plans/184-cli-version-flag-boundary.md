# 184 CLI Version Flag Boundary

## Context

`docs/ilonasin-architecture.md` locks the product shape to one binary with two
subcommands:

- `ilonasin serve`
- `ilonasin manage`

The current CLI also accepts `ilonasin version`. Build identity output is useful,
but representing it as a third command conflicts with the architecture and with
the usage text. This slice keeps version identity as a global flag while
restoring the two-command shape.

## Goal

Keep `ilonasin --version` and `ilonasin -v`, but remove `version` as a
subcommand and update usage text so the CLI presents only `serve` and `manage`
as commands.

After this slice:

- `ilonasin --version` prints version, commit hash, commit subject when
  available, and dirty state,
- `ilonasin -v` behaves the same,
- `ilonasin version` is rejected like any other unknown command,
- usage text lists only `serve` and `manage`,
- no app, server, TUI, provider, storage, config, or management behavior
  changes.

## Scope

1. Update `internal/cli/cli.go`.
   - Keep `-v` and `--version` in the top-level dispatch.
   - Remove `version` from the top-level dispatch.
   - Update usage text to `usage: ilonasin <serve|manage> [--config path]`.
2. Leave `internal/cli/version.go` responsible for formatting build identity.
3. Leave `nix/package.nix` version stamping intact.
4. Do not add dependencies, routes, config fields, storage, TUI behavior, or
   permanent tests.

## Out of Scope

- Changing the version string format again.
- Adding subcommand-specific `--version` flags.
- Adding `--check` commands or old check behavior.
- Changing serve/manage runtime behavior.

## Implementation Steps

1. Remove `version` from the CLI dispatch case.
2. Restore usage text to the two-command shape.
3. Run `gofmt`.
4. Run compile, vet, direct version checks, serve route smoke, manage PTY smoke,
   and whitespace checks.
5. Review the diff before committing.

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
"$tmpbin/ilonasin" --version | rg '^ilonasin 0\.1\.0 \([0-9a-f]{12}, .+\)$'
"$tmpbin/ilonasin" -v | rg '^ilonasin 0\.1\.0 \([0-9a-f]{12}, .+\)$'
"$tmpbin/ilonasin" --help | rg -q 'usage: ilonasin <serve\|manage> \[--config path\]'
if "$tmpbin/ilonasin" --help | rg -q 'version'; then
  "$tmpbin/ilonasin" --help
  exit 1
fi
if "$tmpbin/ilonasin" version >"$tmp/version.out" 2>"$tmp/version.err"; then
  cat "$tmp/version.out" "$tmp/version.err"
  exit 1
fi
rg -q 'unknown command "version"' "$tmp/version.err"
rg -q 'usage: ilonasin <serve\|manage> \[--config path\]' "$tmp/version.err"
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
git diff --check
```

## Acceptance

- CLI command surface presents only `serve` and `manage` as subcommands.
- `--version` and `-v` still print useful build identity.
- `version` is rejected as an unknown command.
- Compile, vet, serve route smoke, manage PTY smoke, and whitespace checks pass.
- Existing unrelated files are not staged or committed.
