# 222 Anthropic Claude Model Aliases

## Context

Plan 221 fixed the first live Claude Code blocker: the patched worktree daemon
no longer rejects `messages[1].role is unsupported`.

The next patched smoke against a worktree-built daemon and real ilonasin
credentials now fails later with:

```text
API Error: 400 model must be addressed as <provider_instance_id>/<provider_model_id>
```

A local capture of the same Claude Code binary shows the request model is:

```text
claude-opus-4-8
```

`internal/server/anthropic_route.go` currently allows only these bare aliases
to route to Codex when exactly one Codex provider instance is configured:

- `claude-haiku-4-6`;
- `claude-opus-4-6`;
- `claude-sonnet-4-6`;
- `haiku`;
- `opus`;
- `sonnet`.

That exact list is already stale for Claude Code 2.1.159. Plan 191 intended
Claude Code model aliases accepted locally to route to `gpt-5.5` when exactly
one Codex provider instance exists, and to fail as ambiguous when multiple
Codex instances exist.

The worktree currently contains unrelated uncommitted auth-retry changes in:

- `internal/server/chat_nonstream.go`;
- `internal/server/chat_stream.go`;
- `internal/server/credentials.go`.

This slice must not modify or stage those files.

## Goal

Make Anthropic Messages model alias fallback robust for Claude-family model
names without introducing hidden cross-provider or cross-model fallback.

## Scope

1. Replace the exact `anthropicCodexFallbackAliases` map with a focused helper
   that accepts:
   - `haiku`;
   - `opus`;
   - `sonnet`;
   - strings beginning with `claude-haiku-`;
   - strings beginning with `claude-opus-`;
   - strings beginning with `claude-sonnet-`.
2. Continue rejecting arbitrary bare model names.
3. Continue rejecting any model string containing `/` through the normal model
   addressing path.
4. Preserve the existing fallback constraints:
   - only Codex provider instances;
   - only instances with chat capability;
   - exactly one matching Codex provider instance;
   - fallback provider model remains `gpt-5.5`;
   - multiple Codex instances remain an ambiguity error.
5. Preserve logging of `anthropic_model_fallback`.
6. Do not change request decoding, message translation, auth, provider
   adapters, storage, management DTOs, TUI, config, IO logging policy, schema,
   or public route names.
7. Do not modify or stage unrelated dirty files.

## Non-Goals

- No complete Anthropic Messages implementation in this slice.
- No `/v1/messages/count_tokens`.
- No native Anthropic upstream provider.
- No true upstream streaming.
- No broad fuzzy model matching.
- No TUI work.
- No permanent tests.

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
snapshot="$(curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/snapshot)"
printf '%s' "$snapshot" | jq -e '.providers | length >= 2' >/dev/null
timeout 3s script -q -e -c "stty cols 140 rows 45; exec env ILONASIN_HOME=$tmp/home $tmpbin/ilonasin manage --config $tmp/config.toml" /dev/null >/dev/null || true
```

Temporary focused smokes:

- prove `claude-opus-4-8`, `claude-sonnet-4-6`, `claude-haiku-4-6`,
  `opus`, `sonnet`, and `haiku` are eligible aliases;
- prove `claude-`, `claude-foo-4-8`, `notclaude-opus-4-8`, `gpt-5.5`,
  empty string, uppercase or mixed-case variants, and arbitrary bare names
  remain ineligible;
- prove names containing `/` remain ineligible for alias fallback.
- prove exactly one Codex chat instance resolves;
- prove zero Codex chat instances keep the normal addressing error;
- prove multiple Codex chat instances still return the ambiguity error.

Remove temporary smoke files before commit.

Live smoke:

Run a worktree-built daemon on a temporary port with:

- existing real SQLite credentials;
- temporary home/log/cache directories;
- `capture_io = false`;
- exactly one Codex provider instance, matching the real `pragnition-codex`
  provider ID.

Then run the underlying Claude Code binary directly with:

```sh
ANTHROPIC_BASE_URL="http://127.0.0.1:<port>" \
ANTHROPIC_AUTH_TOKEN="<ilonasin local token>" \
CLAUDE_CONFIG_DIR="<tmp-claude>" \
/nix/store/p5dqjsfalnc6ws4g3aw2wy3127a6pgri-claude-code-2.1.159/bin/claude \
  --allow-dangerously-skip-permissions --no-session-persistence -p "hi"
```

Acceptance for this slice is that the previous model addressing failure string
is absent and `anthropic_model_fallback` is logged for the sole Codex instance.
A later failure is acceptable only if logs prove it occurs after alias
resolution and is a distinct compatibility gap to address in the next
Anthropic plan.

## Acceptance

- Claude-family bare Anthropic model names can resolve to the sole Codex
  provider instance for `/v1/messages`.
- Arbitrary bare names and addressed names do not use alias fallback.
- Multiple Codex instances remain ambiguous.
- Diff review confirms the slash-containing model check remains before alias
  fallback.
- Temporary focused smokes, compile, vet, serve smoke, manage smoke, and the
  live Claude Code smoke gate pass as defined above.
