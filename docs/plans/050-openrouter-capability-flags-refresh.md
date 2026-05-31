# 050 OpenRouter Capability Flags Refresh

## Context

Plan 041 intentionally filtered OpenRouter `models` and `cache_control` out of
model-cache capability flags because those request fields were unsupported at
that time. The current code now supports them through explicit namespaced
escape hatches:

- `provider_options.openrouter.models`
- `provider_options.openrouter.cache_control`

The architecture says model discovery metadata should be useful but sanitized.
It must not persist raw provider payloads, raw supported-parameter lists, route
objects, prompts, completions, or provider-private metadata.

## Scope

1. Update OpenRouter model-cache capability flags to include implemented support
   for namespaced OpenRouter model fallbacks and cache control.
2. Continue deriving flags only from `/models` `supported_parameters`.
3. Map:
   - `models` to `model_fallbacks`,
   - `cache_control` to `cache_control`.
4. Keep unsupported OpenRouter fields filtered out, including `route`,
   `plugins`, `modalities`, `image_config`, `stop_server_tools_when`, `trace`,
   `debug`, and unknown future values.
5. Keep deterministic sorted comma-separated flags.
6. Do not change request validation, request forwarding, routing, fallback
   policy, credential selection, storage schema, or TUI behavior.
7. Do not store raw `supported_parameters`, pricing, descriptions, provider
   payloads, request bodies, response bodies, or raw provider metadata.

## Implementation

1. Update `internal/provider/http_chat.go`.
   - Extend `openRouterCapabilityFlags` with `models -> model_fallbacks`.
   - Extend `openRouterCapabilityFlags` with `cache_control -> cache_control`.
   - Leave all other unsupported OpenRouter-only fields ignored.

2. Update `internal/app/app.go` smoke harness.
   - Keep the fake OpenRouter `/models` fixture containing implemented,
     unsupported, and marker-bearing `supported_parameters`.
   - Update the OpenRouter model-cache expected capability string.
   - Remove `models` and `cache_control` from the forbidden capability names.
   - Add `model_fallbacks` and `cache_control` to the expected SQLite
     model-cache metadata checks only.
   - Keep `/v1/models` output OpenAI-compatible and free of capability
     metadata.
   - Keep leak checks for `route`, `plugins`, `modalities`, `image_config`,
     `stop_server_tools_when`, `trace`, `debug`, raw marker strings, pricing,
     descriptions, and raw provider payloads.

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
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check
rm -rf "$tmp" "$tmpbin"
```

`go test ./...` is only a compile/package check. No permanent test files will
be added.

## Review Questions

1. Are `model_fallbacks` and `cache_control` the right stable local flag names
   for these now-implemented OpenRouter features?
2. Does this keep capability flags as sanitized metadata rather than routing
   policy?
3. Are the remaining unsupported OpenRouter fields still filtered strongly
   enough?
