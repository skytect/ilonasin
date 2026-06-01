# 183 TUI Observability Dashboard

## Context

The subscription usage section now uses compact cards and gauges, but the rest
of the observability tab still renders as plain text blocks. The architecture
calls the TUI a first-class management interface with first-class views for
recent metadata-only requests, usage totals, latency/TTFT/TPS summaries,
credential health, retry/fallback events, and quota state.

This slice should improve the existing TUI render layer only. The management
snapshot already exposes the metadata needed for a better presentation, and the
storage/daemon boundaries should not change.

## Goal

Render non-subscription observability data as compact Lipgloss cards and visual
bars instead of dense text dumps.

After this slice:

- recent requests render as compact route cards with status, credential,
  attempts/fallbacks, token/cache summaries, and latency chips,
- usage totals render as provider cards with token breakdowns and cache/reasoning
  bars,
- latency and stream summaries render as scan-friendly cards,
- health and quota rows render as status cards with local-time timestamps,
- fallback events render as route-change cards,
- empty states stay concise,
- no new storage, DTO, provider, route, or config behavior is introduced.

## Scope

1. Update `internal/tui/observability_requests.go`.
   - Replace multiline request text rows with compact cards.
   - Keep only metadata fields already present in `management.RequestSummary`.
2. Update `internal/tui/observability_metrics.go`.
   - Render usage, latency, and stream summaries as cards with metric chips and
     bars where a percentage/rate exists.
3. Update `internal/tui/observability_health.go`.
   - Render health and quota summaries as status cards.
4. Update `internal/tui/observability_fallbacks.go`.
   - Render fallback events as compact cards.
5. Add small visual helpers if they are reusable across these renderers.
6. Preserve privacy boundaries.
   - Do not render bearer tokens, local API tokens, full upstream account IDs,
     prompts, completions, request bodies, response bodies, raw SSE chunks, tool
     data, raw provider payloads, or provider request IDs.
7. Do not change management DTOs, SQLite, provider adapters, routes, config,
   key handling, or Bubble Tea update flow.
8. Do not add dependencies or permanent tests.

## Out of Scope

- Changing subscription usage cards again.
- New tabs, animations, tables, viewport behavior, or key bindings.
- New telemetry fields, migrations, routes, or management API endpoints.
- Querying upstream billing, balances, account settings, or provider quota.

## Implementation Steps

1. Add reusable compact metric helpers to the TUI visual layer.
2. Rework recent request rendering into cards.
3. Rework usage, latency, stream, health, quota, and fallback rendering into
   cards.
4. Run `gofmt`.
5. Review the diff before smoke checks.
6. Run compile, vet, daemon route checks, seeded normal and narrow manage PTY
   smokes, source/import/privacy guards, and whitespace checks.

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
db="$tmp/home/ilonasin.sqlite"
sqlite3 "$db" <<EOF
INSERT INTO provider_credentials(id, provider_instance_id, kind, label, created_at, updated_at)
VALUES
  (201, 'codex', 'oauth', 'codex seeded', datetime('now'), datetime('now')),
  (202, 'deepseek', 'api_key', 'deepseek seeded', datetime('now'), datetime('now')),
  (203, 'codex', 'oauth', 'codex fallback', datetime('now'), datetime('now'));
INSERT INTO request_metadata(
  id, started_at, credential_id, requested_provider_instance, requested_model,
  resolved_provider_instance, resolved_model, http_status, error_class,
  retry_count, fallback_count, fallback_reason, prompt_tokens, completion_tokens,
  total_tokens, reasoning_tokens, cache_hit_tokens, cache_write_tokens,
  cost_microunits, total_latency_ms, time_to_first_token_ms,
  output_tokens_per_second, endpoint, stream, provider_type, message_count,
  tool_count, image_count, requested_service_tier, effective_service_tier,
  reasoning_effort, reasoning_summary, reasoning_max_tokens, reasoning_enabled,
  reasoning_exclude, thinking_type, max_output_tokens, auth_retry_count,
  attempt_count, upstream_latency_ms, output_tokens_per_second_total,
  output_tokens_per_second_after_ttft
) VALUES
  (301, datetime('now', '-3 minutes'), 201, 'codex', 'gpt-5.5',
   'codex', 'gpt-5.5', 200, '', 1, 1, 'quota_pressure',
   1200, 300, 1500, 90, 480, 0, 0, 2400, 700, 18.5,
   'chat_completions', 1, 'codex', 4, 2, 0, 'auto', 'default',
   'high', '', 0, 1, 0, '', 1024, 1, 2, 2100, 21.0, 17.0),
  (302, datetime('now', '-1 minutes'), 202, 'deepseek', 'deepseek-chat',
   'deepseek', 'deepseek-chat', 429, 'rate_limit_exceeded', 0, 0, '',
   900, 0, 900, 0, 0, 0, 0, 850, 0, 0,
   'chat_completions', 0, 'deepseek', 2, 0, 0, '', '',
   '', '', 0, 0, 0, '', 0, 0, 1, 0, 0, 0);
INSERT INTO stream_metrics(request_metadata_id, time_to_first_token_ms, output_tokens_per_second, completion_status, chunk_count)
VALUES (301, 700, 18.5, 'completed', 42);
INSERT INTO health_events(occurred_at, provider_instance_id, credential_id, model_id, event_class, http_status, normalized_error_class, consecutive_failure_count, retry_after)
VALUES
  (datetime('now', '-2 minutes'), 'codex', 201, 'gpt-5.5', 'upstream_success', 200, '', 0, NULL),
  (datetime('now', '-1 minutes'), 'deepseek', 202, 'deepseek-chat', 'upstream_failure', 429, 'rate_limit_exceeded', 3, datetime('now', '+10 minutes'));
INSERT INTO quota_events(request_metadata_id, observed_at, provider_instance_id, credential_id, model_id, source, http_status, error_class, retry_after, reset_at)
VALUES (302, datetime('now', '-1 minutes'), 'deepseek', 202, 'deepseek-chat', 'chat', 429, 'rate_limit_exceeded', datetime('now', '+10 minutes'), datetime('now', '+5 hours'));
INSERT INTO fallback_events(request_metadata_id, occurred_at, provider_instance_id, model_id, from_credential_id, to_credential_id, reason, allowed_by_policy)
VALUES (301, datetime('now', '-3 minutes'), 'codex', 'gpt-5.5', 201, 203, 'quota_pressure', 1);
EOF
set +e
printf '\t\tq' | timeout 3s script -q -e -c \
  "sh -c 'stty cols 120 rows 90; exec env ILONASIN_HOME=\"$tmp/home\" \"$tmpbin/ilonasin\" manage --config \"$cfg\"'" \
  "$tmp/manage-normal.typescript" >/dev/null
normal_status="$?"
printf '\t\tq' | timeout 3s script -q -e -c \
  "sh -c 'stty cols 60 rows 90; exec env ILONASIN_HOME=\"$tmp/home\" \"$tmpbin/ilonasin\" manage --config \"$cfg\"'" \
  "$tmp/manage-narrow.typescript" >/dev/null
narrow_status="$?"
printf '\t\t\033[4~q' | timeout 3s script -q -e -c \
  "sh -c 'stty cols 120 rows 90; exec env ILONASIN_HOME=\"$tmp/home\" \"$tmpbin/ilonasin\" manage --config \"$cfg\"'" \
  "$tmp/manage-bottom.typescript" >/dev/null
bottom_status="$?"
set -e
for status in "$normal_status" "$narrow_status" "$bottom_status"; do
  if [ "$status" -ne 0 ] && [ "$status" -ne 124 ]; then
    cat "$tmp"/manage-*.typescript
    exit "$status"
  fi
done
for capture in "$tmp/manage-normal.typescript" "$tmp/manage-narrow.typescript"; do
  grep -q "Recent requests" "$capture"
  grep -q "Usage totals" "$capture"
  grep -q "Latency" "$capture"
  grep -q "Streams" "$capture"
  grep -q "Health" "$capture"
  grep -q "Quota" "$capture"
  grep -q "codex/gpt-5.5" "$capture"
  grep -q "deepseek/deepseek-chat" "$capture"
  grep -q "█" "$capture"
done
for capture in "$tmp/manage-normal.typescript" "$tmp/manage-bottom.typescript"; do
  grep -q "Fallbacks" "$capture"
  grep -q "quota_pressure" "$capture"
  grep -q "█" "$capture"
done
if rg -n "tokens prompt|cache_hit_rate|avg latency|retry_after|cost_microunits|tps_after_ttft" "$tmp"/manage-*.typescript; then
  cat "$tmp"/manage-*.typescript
  exit 1
fi
if rg -i -n "bearer|sk-|iln_|access_token|refresh_token|id_token|raw|payload|prompt body|completion body|request[_ -]?id|tool argument|tool result|acct_|local token" "$tmp"/manage-*.typescript; then
  cat "$tmp"/manage-*.typescript
  exit 1
fi
! rg -n '"ilonasin/internal/(provider|storage|credentials|config)"|bubbletea|log/slog' \
  internal/tui/observability_requests.go \
  internal/tui/observability_metrics.go \
  internal/tui/observability_health.go \
  internal/tui/observability_fallbacks.go \
  internal/tui/observability_visual.go \
  internal/tui/visual_*.go
git diff --check
```

## Acceptance

- Non-subscription observability sections render as cards with chips and bars.
- Dense snake-case text labels from the old view are gone from the seeded smoke.
- No storage, provider, DTO, route, config, or key handling behavior changes.
- Compile, vet, serve route smoke, seeded manage PTY smokes, privacy grep, and
  whitespace checks pass.
- Existing unrelated files are not staged or committed.
