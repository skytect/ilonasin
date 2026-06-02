# 244 Server Route Preflight

## Context

`docs/ilonasin-architecture.md` describes the request pipeline as parser,
strict validator, model address resolver, routing policy, credential resolver,
provider adapter, and upstream provider. The three local generation surfaces
currently repeat the same provider capability and adapter preflight checks:

- `POST /v1/chat/completions`;
- `POST /responses` and `POST /v1/responses`;
- `POST /v1/messages`.

Each route has to keep its own response envelope and early metadata recording,
but the provider-side preflight decision is duplicated: configured provider
must support chat, provider must have a usable auth lane, adapters must be
available, an adapter must exist for the provider type, and the adapter must
accept the normalized chat request.

This slice should make that provider/adapter boundary explicit without changing
route behavior.

## Goal

Centralize generation-route provider preflight into a small server helper while
preserving all current OpenAI-compatible, Responses, and Anthropic response
shapes, metadata rows, status codes, log event names, and routing behavior.

## Scope

1. Add server helpers, likely in a new `internal/server/route_preflight.go`.
2. Define a small result type that can carry:
   - resolved `provider.ChatAdapter`;
   - status code;
   - error class;
   - safe client-facing message.
3. Split preflight into two phases so current route ordering is preserved:
   - provider capability and adapter lookup;
   - adapter request validation after the route has a normalized chat request.
4. Move only these shared checks into the first helper:
   - provider instance supports chat;
   - provider instance has API-key or OAuth auth available;
   - `s.adapters` is non-nil;
   - `s.adapters.ForProvider(instance.Type)` succeeds.
5. Move only this shared check into the second helper:
   - `adapter.ValidateChatRequest(instance, chatReq)` succeeds.
6. Preserve the distinction between:
   - unsupported provider capability:
     `providerUnsupportedCapabilityMessage`, status `501`, class
     `provider_unimplemented`;
   - missing adapter:
     `providerUnavailableMessage`, status `501`, class
     `provider_unimplemented`;
   - adapter request validation failure:
     adapter error message, status `400`, class `unsupported_request`.
7. Update `chat_route.go`, `responses_route.go`, and `anthropic_route.go` to
   call the helper and keep route-specific early metadata recording and response
   writing at the route boundary.
8. Preserve current Responses ordering:
   - resolve model and instance;
   - run provider capability and adapter lookup;
   - only then run `responsesReq.ToChatCompletionRequest(instance.Type)`;
   - then run local chat validation and adapter request validation.
9. Preserve current Anthropic ordering:
   - resolve model and instance;
   - run `req.ToChatCompletion(instance.Type)` and local chat validation;
   - then run provider capability and adapter lookup;
   - then run adapter request validation.
10. Do not change credential resolution, retry/fallback behavior, request
   metadata contents, response writers, provider adapters, storage, management
   DTOs, config, TUI, IO logging, Anthropic count-tokens, models route, or
   public endpoints.
11. Do not add permanent tests.

## Non-Goals

- No route behavior change.
- No shared OpenAI/Anthropic error envelope abstraction.
- No credential-resolution refactor.
- No retry/fallback changes.
- No Responses or Anthropic compatibility change.
- No management API or TUI change.

## Verification

Run a temporary focused in-package smoke test, then remove it before commit. It
should exercise the helper or the three route call paths enough to prove:

- unsupported provider capability still yields status `501`,
  `provider_unimplemented`, and the unsupported-capability message;
- nil adapters still yields status `501`, `provider_unimplemented`, and the
  provider-unavailable message;
- adapter lookup miss still yields status `501`, `provider_unimplemented`, and
  the provider-unavailable message;
- adapter validation error still yields status `400`, `unsupported_request`,
  and the adapter error text;
- successful preflight returns the adapter.
- a route-level Responses case preserves ordering by returning
  `501 provider_unimplemented` for an unsupported or no-adapter provider before
  a request-to-chat translation failure could surface as `400`.

Then run:

```sh
if rg 'provider adapter is not implemented' internal/server -n; then
  echo 'old provider adapter message remains in live server code' >&2
  exit 1
fi
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
timeout 4s script -q -e -c "stty cols 140 rows 32; exec env ILONASIN_HOME=$tmp/home $tmpbin/ilonasin manage --config $tmp/config.toml" /dev/null >"$tmp/manage.out" || true
rg "api|providers|usage|logs" "$tmp/manage.out" >/dev/null
```

During diff review, explicitly verify:

- early metadata recording remains route-specific and unchanged;
- OpenAI-compatible routes keep `writeError`;
- Anthropic route keeps `writeAnthropicError`;
- log event labels stay `chat_route`, `responses_route`, and
  `anthropic_route`;
- credential resolution remains after successful adapter validation;
- no provider adapter, storage, management, config, TUI, models, or
  count-tokens files changed.

## Acceptance

- Shared provider/adapter preflight logic exists in one server helper.
- Chat, Responses, and Anthropic routes keep their existing wire behavior.
- The route pipeline better matches the architecture without changing public
  behavior.
- Focused smoke, compile, vet, serve smoke, manage smoke, whitespace checks,
  and three implementation reviews pass.
