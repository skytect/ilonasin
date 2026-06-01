# 228 TUI Section Density

## Context

The TUI already has the correct top-level sections: API, providers, usage, and logs. The next issue is information shape inside each section. The current panes still leave too much explanatory text and many list-style rows, while the user wants denser screen-sized panes with internal scrolling, more visual summaries, clearer separation of downstream API key management from upstream provider auth, and compact human-readable times in the system timezone.

The worktree currently contains unrelated uncommitted server changes in:

- `internal/server/chat_nonstream.go`;
- `internal/server/chat_stream.go`;
- `internal/server/credentials.go`.

This slice must not modify or stage those files.

## Goal

Make the existing four TUI sections more useful and compact without changing management APIs, storage, provider routing, server behavior, or the top-level tab model.

## Scope

1. Keep the top-level sections as API, providers, usage, and logs.
2. Keep the existing pane-scrolling mechanism and screen-sized dashboard layout.
3. Rework the API section so it foregrounds:
   - the three exposed local API surfaces: Chat Completions, Responses, and Anthropic Messages;
   - bind address;
   - local downstream token count and enabled/disabled split;
   - a compact local token list with created/disabled time chips.
4. Rework provider panes so they foreground:
   - upstream provider instances and auth capabilities;
   - upstream API keys;
   - OAuth accounts and provider account identities, especially visible email labels when captured;
   - fallback credential groups.
5. Rework usage panes so they stay visual and compact:
   - token totals remain grouped by provider;
   - token breakdown includes input, output, reasoning, cache hit, cache miss, and cache write counts;
   - cache/reasoning rates render as bars or compact visual chips rather than prose;
   - subscription pool rows remain summative only, not average/min language.
6. Rework logs panes so metadata and IO policy become compact visual status lines:
   - request metadata remains cards because each request is a repeated item;
   - retention/IO policy becomes compact status cards or chips rather than paragraph text;
   - fallback rows remain compact event cards.
7. Preserve local-time formatting in the TUI by using existing client-side time
   helpers such as `formatRelativeLocalTime`, `timeChip`, and
   `optionalTimeChip`. Do not add server-side time or DTO changes.
8. Add small TUI helper functions only where they reduce repeated formatting.
9. Preserve redaction/privacy constraints: no prompts, completions, request bodies, response bodies, raw streams, tool arguments/results, provider payloads, full tokens, full account IDs, or provider request IDs.
10. Do not add management DTO fields, schema fields, config mutation, route changes, provider changes, or permanent tests.
11. Do not modify or stage unrelated dirty server files.

## Non-Goals

- No new Bubble Tea navigation model.
- No new third-party TUI dependency.
- No server-side usage calculation changes.
- No subscription keepalive behavior changes.
- No Anthropic API work.

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
cat >"$tmp/config.toml" <<EOF2
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
snapshot="$(curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/snapshot)"
printf '%s' "$snapshot" | jq -e '.providers | length >= 2' >/dev/null
for cols in 76 120 160; do
  timeout 4s script -q -e -c "stty cols $cols rows 32; exec env ILONASIN_HOME=$tmp/home $tmpbin/ilonasin manage --config $tmp/config.toml" /dev/null >"$tmp/manage-$cols.out" || true
  rg "api|providers|usage|logs|Chat Completions|Responses|Anthropic|local tokens|token mix|cache|retention" "$tmp/manage-$cols.out" >/dev/null
  if rg "average|min left|Guidance|tab / shift\\+tab switch sections" "$tmp/manage-$cols.out"; then
    echo "unwanted old copy or privacy prose remains in visible smoke output" >&2
    exit 1
  fi
done
```

After implementation, review the rendered output files manually enough to verify:

- narrow output remains legible;
- wide output takes horizontal space through panes and card grids;
- API surfaces are visible in the API pane;
- provider panes show email/display identity when metadata has it;
- usage pool language is summative;
- used/remaining quota is one bar per window;
- logs explain metadata/IO policy compactly.

Also run a temporary focused TUI render smoke, then remove it before commit. It
should construct a `Model` directly with seeded snapshot rows, render API,
providers, usage, and logs at narrow and wide widths, and assert:

- local token output includes created and disabled time chips;
- local token output includes only safe token fragments, not a full token value;
- OAuth and provider account rows render safe email/display labels visibly;
- all four top-level sections render their intended pane content, not just the
  tab bar;
- unsafe account-like labels, full local tokens, provider request IDs,
  request/response body markers, raw payload markers, prompts, completions, and
  tool data markers are absent or redacted in rendered output;
- subscription pool text contains summative used/left/capacity values and does
  not contain average/min-left language.
- local time labels come from existing TUI local-time helpers and do not require
  management DTO or server changes.

## Acceptance

- The four top-level TUI sections remain API, providers, usage, and logs.
- Each section uses compact screen-sized panes with pane-local scrolling.
- API distinguishes local downstream tokens from upstream provider credentials.
- Provider OAuth/account panes keep email/display labels visible when present.
- Usage visuals include token breakdowns, cache/reasoning rates, subscription account bars, and summative subscription pool bars.
- Logs are compact metadata/IO status surfaces, not prose-heavy help text.
- Compile, vet, serve smoke, manage wide/narrow smoke, whitespace checks, and implementation review pass.
