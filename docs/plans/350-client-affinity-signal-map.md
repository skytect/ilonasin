# 350 Client Affinity Signal Map

## Context

Pooling should use fields clients actually send, not idealized fields. The
current credential-pool policy already supports:

- Responses `prompt_cache_key`;
- Responses `client_metadata` fallback for selected safe keys;
- Chat Completions `session_id`, `user`, and selected metadata keys;
- Anthropic Messages metadata-derived affinity;
- selected safe session headers as a lower-priority fallback;
- no-affinity least-in-flight plus round-robin balancing.

Fresh source inspection against `/tmp/codex-src-0.135.0/codex-rs` shows Codex's
normal Responses path builds `prompt_cache_key` from `state.thread_id`, adds
`client_metadata` with `x-codex-installation-id`, and sends `session-id`,
`thread-id`, and `x-client-request-id` headers. The local ilonasin code already
uses the stable body-level `prompt_cache_key` before header fallback.

## Goal

Record a concise, source-backed map of out-of-box affinity-relevant request
fields by client/API so later pooling slices can reason from ground truth.

## Scope

1. Update `docs/codex-compatibility-audit.md` with a short affinity signal
   section covering:
   - Codex CLI Responses fields from current source;
   - Claude Code Anthropic fields already captured in prior plans;
   - generic OpenAI Chat and Responses client behavior as optional fields;
   - why `prompt_cache_key` and session/thread IDs are affinity candidates;
   - why request-id-shaped values are not general affinity candidates.
2. Keep the section metadata-only:
   - no raw captured request bodies;
   - no credential values;
   - no account IDs;
   - no durable request IDs.
3. Do not change server routing, credential selection, storage, management DTOs,
   TUI rendering, config, logging, or provider adapters.
4. Keep this plan as the only new plan file for the slice.

## Verification

Before implementation review:

```sh
git diff --check
go test ./...
go vet ./...
```

Also do a manual diff review for:

- no secret-bearing examples;
- source paths point at existing inspected files;
- no claims that `x-client-request-id` is generally stable for every client;
- no code behavior changes.

## Expected Outcome

- Pooling policy has a local hard-source reference for what Codex and related
  clients actually send.
- Future routing changes can cite concrete request fields instead of relying on
  assumptions.
- No runtime behavior changes.
