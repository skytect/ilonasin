# 333 Chat Metadata Affinity

## Context

Credential pooling now avoids resolver-order dominance through:

- local API token, provider instance, and provider model hashing;
- request affinity from Chat `session_id`, then Chat `user`;
- request affinity from Responses `prompt_cache_key`;
- request affinity from Anthropic metadata-derived `session_id`;
- deterministic local-token/provider/model ordering plus least-in-flight
  reservation when no request affinity exists.

OpenAI Chat already accepts a bounded string `metadata` object and forwards it
where providers support it. Some OpenAI-compatible clients use metadata fields
for session identity instead of first-class `session_id` or `user`. Today those
requests fall back to no-affinity balancing even when a safe session-like
metadata value is present.

The architecture allows routing to use metadata-only request characteristics,
but it forbids storing prompts, request bodies, full account IDs, raw provider
payloads, and hidden cross-provider or cross-model routing behavior.

## Scope

1. Extend local-only Chat affinity extraction in `internal/openai`.
   - Keep `session_id` as the first preference.
   - Keep `user` as the second preference.
   - Add a third preference from `metadata` string pairs.
2. Use only conservative metadata keys:
   - `session_id`
   - `thread_id`
   - `conversation_id`
   - `prompt_cache_key`
3. Trim all affinity candidates and require non-empty values up to 256 runes.
   Ignore blank or overlong values for affinity instead of rejecting the
   already-valid Chat request.
4. Ignore unsafe marker-shaped values before using them for affinity.
   - Ignore values that look like JSON objects.
   - Ignore values that look like JWTs.
   - Ignore values containing account or device markers such as `account`,
     `acct_`, `account_uuid`, `device`, `device_id`, `bearer`, `token`,
     `secret`, `authorization`, `oauth`, or `sk-`.
5. Keep the affinity key local-only.
   - Do not store it in request metadata.
   - Do not log it.
   - Do not render it in management or TUI views.
   - Do not expose it in errors.
   - Do not use prompts, messages, tool payloads, request bodies, response
     bodies, bearer tokens, upstream account IDs, device IDs, IP addresses, or
     user-agent strings.
6. Preserve provider payload behavior.
   - Chat `metadata` continues to be forwarded or rejected exactly as current
     provider validation and marshaling dictate.
   - The new affinity extraction must not mutate `Metadata`.
7. Do not change routing, credential storage, quota storage, management APIs,
   TUI, provider adapters, IO logging, or permanent tests.

## Out Of Scope

- Parsing inbound request headers for affinity.
- Persisted session-to-credential maps.
- Subscription remaining-quota weighting.
- Cross-provider or cross-model fallback.
- Deriving affinity from message content.
- Adding new metadata fields to request logs.

## Implementation Steps

1. Add a small helper near `chatAffinityKey` to derive affinity from allowed
   metadata keys in deterministic priority order.
2. Update `chatAffinityKey` to fall back to that helper after `session_id` and
   `user`.
3. Keep the helper unexported and free of server/provider imports.
4. Review the diff before running checks.

## Verification

Use a temporary focused check, then remove it before commit, covering:

- `session_id` still wins over `user` and metadata;
- `user` still wins over metadata;
- metadata `session_id`, `thread_id`, `conversation_id`, and
  `prompt_cache_key` can each become the affinity key;
- metadata priority is deterministic when multiple allowed keys are present;
- blank and overlong `session_id`, `user`, and metadata values are ignored for
  affinity;
- JSON-object-looking values, JWT-looking values, and
  account/device/secret marker-shaped values in `session_id`, `user`, and
  metadata are ignored for affinity;
- decoded `Metadata` is unchanged;
- `MarshalUpstreamChatRequest` still includes the original `metadata` object
  unchanged when `metadata` is present;
- marshaled Chat requests do not include `AffinityKey`;
- request metadata helpers do not persist affinity values;
- server credential planning treats metadata-derived affinity as a sticky
  request affinity;
- requests with blank, overlong, or unlisted metadata leave `AffinityKey` empty
  so the existing no-affinity deterministic ordering plus least-in-flight
  reservation path remains active.

Then run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/openai
go test ./internal/server
go test ./...
go vet ./...
```

Build `cmd/ilonasin`, start `ilonasin serve` with temporary `ILONASIN_HOME` and
`[server] bind = "127.0.0.1:0"`, check `/_ilonasin/manage/health` over the
management socket, run a short `ilonasin manage` TUI smoke, then clean up.

## Acceptance

- Chat clients that put safe session/cache identity in metadata can get sticky
  same-provider, same-model credential affinity when existing provider
  validation allows Chat metadata.
- Chat clients without usable metadata still use the existing no-affinity
  deterministic local-token/provider/model ordering plus least-in-flight
  reservation behavior.
- Affinity remains local-only, metadata-only, and invisible to logs,
  management, TUI, and request metadata.
- Existing provider validation, provider marshaling, quota filtering, fallback
  recording, and routing constraints remain unchanged.
