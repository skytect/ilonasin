# 047 OpenRouter Distillable Text Provider Preference

## Context

The current provider option path supports OpenRouter routing, privacy,
filtering, sort, and performance preferences under the explicit namespace:

```json
{
  "provider_options": {
    "openrouter": {
      "provider": {}
    }
  }
}
```

The live OpenRouter OpenAPI document accessed on 2026-05-31 lists
`provider.enforce_distillable_text` as a nullable boolean:

- `true` restricts routing to models where the author allows text distillation.
- `false` is accepted by the upstream schema, but does not add a restriction.
- `null` is part of the upstream schema, but this codebase has been using a
  stricter local subset that rejects nulls inside provider options.

The architecture requires explicit provider-specific escape hatches,
strict unsupported-field rejection, no silent cross-provider behavior, and no
storage of request bodies or raw provider payloads.

## Scope

1. Add OpenRouter-only support for
   `provider_options.openrouter.provider.enforce_distillable_text`.
2. Accept only JSON booleans for this field, including both `true` and `false`.
   Reject `null`, strings, numbers, objects, and arrays.
3. Forward the accepted value unchanged to the upstream OpenRouter `provider`
   object for both non-streaming and streaming chat completions.
4. Keep the field unsupported for DeepSeek and Codex by namespace validation
   before upstream credential resolution.
5. Keep top-level `provider`, top-level `models`, top-level `route`, and unknown
   OpenRouter provider fields rejected.
6. Do not change model addressing, fallback policy, credential routing,
   telemetry schema, persistence, or TUI behavior.

## Implementation

1. Update `internal/provider/http_chat.go`.
   - Add `enforce_distillable_text` to `validateOpenRouterProvider`.
   - Require the value to be a Go `bool`.
   - Return a field-specific validation error for non-booleans.

2. Update `internal/app/app.go` smoke harness.
   - Add exact upstream validators for:
     - `enforce_distillable_text: true`
     - `enforce_distillable_text: false`
     - a combined provider object with existing supported fields.
   - Add fake upstream route cases for non-stream and stream requests.
   - Add OpenRouter invalid cases for null, string, number, object, and array.
   - Move the old unsupported marker case into typed invalid coverage so the
     marker still verifies no upstream dispatch or metadata leak.
   - Add unsupported-provider cases for DeepSeek and Codex through the existing
     `providerOptionInvalidCases` path.
   - Add Codex no-eligible-cache checks for unsupported and invalid
     OpenRouter distillable-text options.

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

1. Is accepting both boolean `true` and `false` the correct local subset, given
   the live schema says nullable boolean and the architecture favors strict
   explicit forwarding?
2. Does this slice preserve the boundary between local fallback policy and
   OpenRouter's upstream routing preferences?
3. Are the proposed smoke cases enough to prove the field is forwarded only to
   OpenRouter and never persisted as raw request or provider payload data?
