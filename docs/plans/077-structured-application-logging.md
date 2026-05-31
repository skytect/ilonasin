# 077 Structured Application Logging

## Context

`ilonasin` currently has `paths.log_dir` and a default `~/.ilonasin/logs/`
layout, but it does not have a real logging boundary. OAuth device login can
fail with safe but opaque classes such as `oauth_login_http_error`, and broader
operations across the daemon, TUI, provider transports, credential services,
metadata recording, and SQLite storage are hard to diagnose.

The architecture requires metadata-only observability and explicitly forbids
storing prompts, completions, request bodies, response bodies, raw provider
payloads, raw SSE chunks, tool arguments, tool results, full bearer tokens,
full provider request IDs, and full account IDs. Codex auth notes also warn
against logging OAuth callback URLs, success URLs, token endpoint bodies,
provider command stdout, or token stores.

This slice adds a proper structured logging setup with levels and outputs,
then wires sanitized logs across the main operational boundaries. It does not
replace the existing SQLite metadata ledger, and it does not introduce raw HTTP
capture.

The next architectural direction is that `manage` should become a TUI client
that talks to the daemon through an internal API instead of mutating SQLite
directly. This logging slice should support that future split by using a
separate logging boundary rather than embedding logging into storage or TUI
internals.

## Scope

1. Use Go stdlib `log/slog`; do not add a logging dependency.
2. Add `[logging]` config:
   - `level`: `debug`, `info`, `warn`, or `error`
   - `format`: `json` or `text`
   - `outputs`: `file`, `stderr`, or both
3. Defaults:
   - `level = "info"`
   - `format = "json"`
   - `outputs = ["file"]`
4. File output writes to `<paths.log_dir>/ilonasin.log` with directory mode
   `0700` and file mode `0600` where practical.
5. Stderr output lets supervised `serve` deployments flow naturally into
   `journalctl`, Docker logs, or shell redirection. Do not add a direct
   journald dependency in this slice.
6. Add a redacting logging handler that redacts sensitive attributes even if a
   caller accidentally passes one. Redaction must recurse through slog groups,
   truncate long string values, and redact keys containing:
   - `auth`
   - `authorization`
   - `bearer`
   - `token`
   - `secret`
   - `key`
   - `cookie`
   - `code`
   - `verifier`
   - `account`
   - `request_id`
   - `generation_id`
   - `url`
   - `uri`
   - `host`
   - `path`
   - `query`
   - `header`
   - `body`
   - `payload`
   - `prompt`
   - `completion`
   - `raw`
   - `stdout`
   - `stderr`
7. Add sanitized structured logs at these boundaries:
   - app bootstrap and command lifecycle,
   - `serve` HTTP request lifecycle,
   - local auth successes and failures,
   - chat and model route outcomes,
   - provider model, chat, stream, OAuth refresh, and OAuth device HTTP
     attempts,
   - credential creation, disable, fallback toggle, OAuth login, and OAuth
     refresh lifecycle events,
   - SQLite open, migration, metadata record, fallback record, health record,
     and telemetry prune events,
   - TUI actions that mutate SQLite or start external provider flows.
8. Include event IDs on warning and error events where the user may need to
   correlate a TUI or CLI failure with a log entry.
9. Surface OAuth device-login event IDs in TUI error text when available.
10. Use explicit allowlisted attributes per boundary. Do not pass arbitrary
    context maps or raw errors to the logger.
11. Exclude request bodies, response bodies, raw provider payloads, raw SSE
    chunks, prompts, completions, tool arguments, tool results, API keys,
    OAuth access tokens, OAuth refresh tokens, ID tokens, auth headers, device
    auth IDs, user codes, authorization codes, code verifiers, full URLs,
    query strings, endpoint hosts, raw paths, full account IDs, and full
    provider request IDs.
12. Do not add permanent tests, provider features, SQLite tables, migrations,
    log viewers, daemon internal APIs, or a manage-through-daemon refactor in
    this slice.

## Implementation

1. Add `internal/logging` for:
   - `Setup`,
   - no-op logging,
   - output parsing,
   - level parsing,
   - JSON/text handler selection,
   - redacting handler,
   - event ID generation.
2. Add `LoggingConfig` to `internal/config`.
3. Extend app runtime bootstrap to open the configured logger after config load
   and close log files during cleanup.
4. Thread `*slog.Logger` through app wiring into server, credential services,
   provider HTTP adapters, and TUI model construction.
5. Keep provider and storage packages dependent only on `log/slog` or small
   local interfaces, not on app or config.
6. For HTTP logging, record only method, endpoint label, status, duration,
   response byte count when already known, provider instance ID, provider type,
   route label, model identifier where already public in local metadata,
   credential ID when available, error class, and event ID.
7. Endpoint labels must be static operation names, for example
   `chat_completions`, `models`, `oauth_device_code`,
   `oauth_device_poll`, `oauth_token`, `oauth_refresh`, and local route labels
   such as `v1_chat_completions`. Do not log raw upstream paths, full URLs,
   hosts, query strings, callback URLs, success URLs, or route parameters.
8. Do not log `err.Error()` directly except for errors already known to be
   local, static, and marker-free. Prefer normalized safe classes.
9. For TUI and credential logs, record action names, provider instance IDs,
   credential IDs, enabled/disabled states, and error classes only.
10. Add smoke assertions that:
    - default JSON file logging writes an event and level,
    - `outputs = ["stderr"]` writes text or JSON logs to stderr and does not
      create a file,
    - `outputs = ["file", "stderr"]` writes both outputs,
    - `format = "text"` is accepted,
    - `level = "error"` filters an info startup event,
    - invalid level, format, and output values fail during config/bootstrap,
    - a forced safe OAuth device-login HTTP error emits an event ID in the TUI
      error and the matching log event. This correlation can be asserted inside
      `manage --check`, because the TUI failure text is not normally emitted
      by a successful check run,
    - generated local bearer tokens such as `iln_...` are absent from logs.
      This must be asserted inside `serve --check` or `manage --check`, because
      shell grep cannot know generated token values after the command exits.
11. Run `gofmt` on touched Go files.
12. Manually review the diff before smoke checks, with special attention to
    redaction and package boundaries.

## Smoke Checks

Run these direct checks before code review:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
tmp="$(mktemp -d)"
unsafe_re='iln_|oauth-login-access-marker|oauth-login-refresh-marker|oauth-access-secret-marker|oauth-refresh-secret-marker|oauth-refresh-old-|oauth-refresh-new-|oauth-refresh-keep-|oauth-refresh-disabled|oauth-refresh-cross|oauth-refresh-stale|oauth-refresh-nonoauth|device-auth-marker|code-verifier-marker|authorization-code-marker|id-token-marker|token_endpoint_body|Bearer sk|Authorization:|Set-Cookie|raw-provider-payload|raw_provider_payload|raw [a-z0-9 _-]*body|raw failed marker|acct_raw|acct_device|acct_prune|sk-serve-check|sk-fallback|sk-observe|sk-prune|prompt marker|completion marker|body marker|req_unsafe|req_old'
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check
test -f "$tmp/logs/ilonasin.log"
grep -q '"event":' "$tmp/logs/ilonasin.log"
grep -q '"level":' "$tmp/logs/ilonasin.log"
grep -q '"event_id":' "$tmp/logs/ilonasin.log"
! grep -E "$unsafe_re" "$tmp/logs/ilonasin.log"

cfgstderr="$tmp/stderr-config.toml"
cat >"$cfgstderr" <<EOF
[server]
bind = "127.0.0.1:0"
[logging]
level = "debug"
format = "json"
outputs = ["stderr"]
[providers.codex]
type = "codex"
[providers.deepseek]
type = "deepseek"
[providers.openrouter]
type = "openrouter"
EOF
stderrlog="$tmp/stderr.log"
ILONASIN_HOME="$tmp/stderr-home" "$tmpbin/ilonasin" manage --config "$cfgstderr" --check 2>"$stderrlog"
grep -q '"event":' "$stderrlog"
! grep -E "$unsafe_re" "$stderrlog"
test ! -f "$tmp/stderr-home/logs/ilonasin.log"

cfgboth="$tmp/both-config.toml"
cat >"$cfgboth" <<EOF
[server]
bind = "127.0.0.1:0"
[logging]
level = "info"
format = "text"
outputs = ["file", "stderr"]
[providers.codex]
type = "codex"
[providers.deepseek]
type = "deepseek"
[providers.openrouter]
type = "openrouter"
EOF
bothstderr="$tmp/both-stderr.log"
ILONASIN_HOME="$tmp/both-home" "$tmpbin/ilonasin" manage --config "$cfgboth" --check 2>"$bothstderr"
grep -q 'event=' "$bothstderr"
grep -q 'event=' "$tmp/both-home/logs/ilonasin.log"
! grep -E "$unsafe_re" "$bothstderr"
! grep -E "$unsafe_re" "$tmp/both-home/logs/ilonasin.log"

cfgerror="$tmp/error-config.toml"
cat >"$cfgerror" <<EOF
[server]
bind = "127.0.0.1:0"
[logging]
level = "error"
format = "json"
outputs = ["file"]
[providers.codex]
type = "codex"
[providers.deepseek]
type = "deepseek"
[providers.openrouter]
type = "openrouter"
EOF
ILONASIN_HOME="$tmp/error-home" "$tmpbin/ilonasin" manage --config "$cfgerror" --check
! grep -q '"level":"INFO"' "$tmp/error-home/logs/ilonasin.log"
grep -q '"level":"ERROR"' "$tmp/error-home/logs/ilonasin.log"

for invalid in level format output empty_output empty_output_list; do
  badcfg="$tmp/bad-$invalid.toml"
  case "$invalid" in
    level) bad='level = "trace"\nformat = "json"\noutputs = ["file"]' ;;
    format) bad='level = "info"\nformat = "xml"\noutputs = ["file"]' ;;
    output) bad='level = "info"\nformat = "json"\noutputs = ["journald"]' ;;
    empty_output) bad='level = "info"\nformat = "json"\noutputs = [""]' ;;
    empty_output_list) bad='level = "info"\nformat = "json"\noutputs = []' ;;
  esac
  printf '[server]\nbind = "127.0.0.1:0"\n[logging]\n%b\n[providers.codex]\ntype = "codex"\n[providers.deepseek]\ntype = "deepseek"\n[providers.openrouter]\ntype = "openrouter"\n' "$bad" >"$badcfg"
  if ILONASIN_HOME="$tmp/bad-$invalid-home" "$tmpbin/ilonasin" manage --config "$badcfg" --check; then
    echo "invalid logging $invalid accepted"
    exit 1
  fi
done
rm -rf "$tmp" "$tmpbin"
```

`go test ./...` is only a compile/package check. No permanent test files will
be added.

## Review Questions

1. Are `slog`, file, and stderr the right logging foundation for both local TUI
   use and supervised daemon use?
2. Are the proposed log boundaries broad enough for this first exhaustive
   logging pass while still respecting the metadata-only architecture?
3. Does keeping direct journald support out of scope preserve portability
   without blocking `journalctl` usage through stderr?
4. Does this setup support the future manage-through-daemon refactor without
   making storage or TUI own logging policy?
