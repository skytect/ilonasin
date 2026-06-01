# 231 IO Secret Scrubber Boundary

## Context

`docs/ilonasin-architecture.md` and `docs/plans/300-binary-io-logging-policy.md`
define a binary logging policy:

- normal application logs stay metadata-only;
- `[logging].capture_io = true` enables local IO payload logging for debugging;
- known secrets must never be written to normal logs or IO logs.

The current IO scrubber already redacts exact snake-case JSON secret fields and
plain `Bearer` / `iln_` markers. It does not reliably cover common OAuth and
HTTP field-name variants such as `refreshToken`, `accessToken`, `idToken`,
`clientSecret`, `codeVerifier`, `deviceCode`, or `userCode`. It also does not
scrub URL-encoded form bodies, even though OAuth code exchange uses
`application/x-www-form-urlencoded` style fields such as `code_verifier`.

Since IO logging intentionally preserves payloads when enabled, field-name and
marker scrubbing is one safety boundary. This slice improves that boundary for
known secret carriers without broadly redacting useful non-secret payload fields
such as token counts, account hashes, cache counts, or ordinary prompt/debug
content. It does not claim to complete the full plan-300 configured-secret
exact-value scanner.

The worktree is clean at the start of this slice.

## Goal

Strengthen `logging.ScrubIOBody` so IO logs keep useful payload fidelity while
redacting known secret field-name variants across JSON, form, header-like, and
plain text bodies.

## Scope

1. Improve credential-key normalization in `internal/logging/secrets.go` so it
   recognizes camelCase, PascalCase, kebab-case, snake_case, and spaced variants
   of existing secret carriers.
2. Preserve the existing credential-key allowlist shape. Add narrowly scoped
   aliases only for known secret carriers required by the architecture and docs,
   including OAuth access/refresh/ID tokens, API keys, device/user codes,
   authorization codes, code verifiers, agent assertions, private keys, and
   bearer tokens.
3. Extend `internal/logging/io.go` to scrub URL-encoded form bodies by redacting
   values for credential keys while preserving non-secret keys and parseable
   form structure.
4. Extend plain-text scrubbing to redact simple `key=value` and `key: value`
   secret carriers for known credential keys, while preserving non-secret text.
5. Preserve useful IO payload fields:
   - prompts and completions may remain in IO logs only when `capture_io=true`;
   - `prompt_tokens`, `completion_tokens`, `reasoning_tokens`, `cache_hit`,
     `account_hash`, and similar non-secret operational fields must not be
     redacted just because they contain words such as token or account.
6. Do not change normal application logging, server routes, provider adapters,
   management DTOs, storage, config, TUI, or public APIs.
7. Do not add permanent tests.

## Non-Goals

- No full implementation of plan 300.
- No configured-secret exact-value scanner in this slice. This remains required
  for full plan-300 completion; this slice only improves field-name and marker
  scrubbing.
- No remote log shipping.
- No change to whether IO logging is enabled.
- No attempt to detect arbitrary secrets in user content.

## Verification

Run a temporary focused logging test, then remove it before commit. It must
prove:

- JSON secret-key variants are redacted: `accessToken`, `refreshToken`,
  `idToken`, `clientSecret`, `codeVerifier`, `deviceCode`, `userCode`,
  `authorizationCode`, `apiKey`, `agentAssertion`, and `privateKey`;
- JSON non-secret fields are preserved: `prompt_tokens`, `completion_tokens`,
  `reasoning_tokens`, `cache_hit`, `account_hash`, and normal prompt text;
- non-secret keys containing risky words are preserved, for example
  `token_count`, `account_summary`, and `user_account_label`;
- URL-encoded form fields redact secret carriers such as `code_verifier`,
  `refresh_token`, and `client_secret`, while preserving non-secret fields;
- plain text `Authorization: Bearer ...`, `X-Api-Key: ...`, `code_verifier=...`,
  and `refreshToken: ...` are redacted;
- `Bearer` and `iln_` marker redaction still works.

Then run:

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
cat >"$tmp/config.toml" <<EOF2
[server]
bind = "127.0.0.1:$port"

[paths]
database = "$tmp/home/ilonasin.sqlite"
log_dir = "$tmp/home/logs"
cache_dir = "$tmp/home/cache"

[logging]
capture_io = true

[subscription_keepalive]
enabled = false

[providers.deepseek]
type = "deepseek"

[providers.codex]
type = "codex"
EOF2
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
token="$(curl --silent --fail --unix-socket "$sock" -X POST http://ilonasin/_ilonasin/manage/local-tokens -d '{"label":"io-smoke"}' | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')"
test -n "$token"
curl --silent --output "$tmp/chat.out" --write-out "%{http_code}" \
  -H "Authorization: Bearer $token" \
  -H "Content-Type: application/json" \
  --data '{"model":"missing:model","messages":[{"role":"user","content":"io-prompt-marker"}],"accessToken":"secret-access","prompt_tokens":123}' \
  "http://127.0.0.1:$port/v1/chat/completions" >/dev/null || true
test -s "$tmp/home/logs/ilonasin-io.log"
rg 'io-prompt-marker|prompt_tokens' "$tmp/home/logs/ilonasin-io.log" >/dev/null
if rg "$token|secret-access" "$tmp/home/logs/ilonasin-io.log"; then
  echo "io log leaked known secret marker" >&2
  exit 1
fi
timeout 4s script -q -e -c "stty cols 140 rows 32; exec env ILONASIN_HOME=$tmp/home $tmpbin/ilonasin manage --config $tmp/config.toml" /dev/null >"$tmp/manage.out" || true
rg "api|providers|usage|logs" "$tmp/manage.out" >/dev/null
```

During diff review, explicitly verify:

- only `internal/logging/io.go`, `internal/logging/secrets.go`, and this plan
  changed;
- normal `slog` behavior is not loosened;
- IO scrubbing remains key-based and marker-based, not broad content guessing;
- non-secret metric fields are preserved.

## Acceptance

- IO body scrubbing redacts known secret key variants in JSON, form, and plain
  text payloads.
- IO body scrubbing preserves non-secret operational payload fields.
- Normal application logging behavior is unchanged.
- Focused smoke, compile, vet, serve smoke, manage smoke, whitespace checks,
  and three implementation reviews pass.
