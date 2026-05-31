# Plan 093: Codex Compatibility Audit

Status: executed.

## Goal

Decide, with current evidence, how close `ilonasin serve` is to being a safe
backend for normal `codex exec` use before switching Codex workflows over to
it.

This slice produces a compatibility report and only fixes defects that are
small, local, and directly proven by the audit. Larger gaps become the next
numbered plans.

## Ground Truth

- `docs/ilonasin-architecture.md` requires strict local request validation,
  provider-specific adapters, explicit routing, credential-domain separation,
  and metadata-only observability.
- `docs/codex-client-red-team.md` recorded that Codex CLI uses Responses API
  traffic for custom model providers, and that earlier failures were endpoint
  shape mismatches.
- Plan `092` added local `/responses` and `/v1/responses` support for text
  turns, plus `GET /models` for root-base provider configs.
- Plan `090` added direct Chat Completions support for multimodal content,
  Codex reasoning options, service tier, and function-tool translation. That
  does not prove real Codex CLI compatibility through local Responses.
- `docs/codex-endpoints.md` shows Codex can use `/responses`, `/models`,
  `/files`, `/responses/compact`, `/memories/trace_summarize`, realtime, and
  other endpoint families. This audit is scoped to `codex exec` model-provider
  behavior, not the entire Codex app backend.
- Current Codex auth docs require keeping provider env-key auth, Codex OAuth
  account state, local ilonasin client tokens, logs, request metadata, and
  upstream credentials as separate boundaries.

## In Scope

1. Identify the current Codex CLI version and provider request behavior.
   - Use `codex --version`, `codex exec --help`, and source inspection if the
     local source snapshot exists.
   - Record only version, option names, endpoint families, and safe structural
     behavior.

2. Build a live probe matrix for `codex exec` against `ilonasin`.
   - Use the real provider credentials already configured in `~/.ilonasin`.
     Real credential smoke is required before claiming compatibility.
   - Use a temporary `CODEX_HOME` with mode `0700`.
   - Start a worktree-built `ilonasin serve` in the background with a
     temporary config that points at the real `~/.ilonasin` database, but uses
     temporary log, cache, runtime, and data directories where possible.
   - Use a fresh env-key local client token created through the management API,
     and disable it after the smoke.
   - Expect the real database to retain metadata-only audit rows for the
     disabled local token, request metadata, and health events. Do not retain
     raw prompts, completions, request bodies, response bodies, provider
     payloads, image bytes, tool arguments, tool results, bearer tokens,
     account IDs, or request IDs.
   - Run `codex exec` with `--ephemeral --ignore-user-config`.
   - Unset unrelated Codex/OpenAI auth environment variables for the Codex
     process. The only provider auth value visible to Codex should be
     `ILONASIN_CLIENT_TOKEN`.
   - Never pass a literal bearer token through `-c`, shell arguments, config
     files, logs, captures, or the report.
   - Do not copy `~/.ilonasin`, logs, cache, SQLite, WAL/SHM files, Codex
     `auth.json`, Codex `.credentials.json`, request metadata, OAuth token
     state, or account state into another audit home. The live smoke may point
     at the existing real SQLite database in place.
   - Use fake upstreams only for deterministic negative probes that would be
     unsafe, flaky, or impossible to force against live providers.
   - Exercise both provider base URL variants:
     - `http://127.0.0.1:<port>`
     - `http://127.0.0.1:<port>/v1`

3. Probe the compatibility areas needed before switching normal Codex use:
   - root-base and `/v1` model discovery,
   - simple text prompt,
   - developer/system instruction behavior,
   - reasoning effort configuration,
   - fast or priority service tier configuration,
   - image attachment behavior via `codex exec --image`,
   - complex tool-call behavior against a throwaway workspace, split into
     shell execution, `apply_patch`, and at least one non-shell function or
     custom tool follow-up shape when the current Codex CLI can express it,
   - explicit handling of `function_call_output`, custom tool output, MCP
     output, and tool-search-like output shapes when the current CLI emits
     them,
   - request cancellation or timeout behavior,
   - retry behavior for upstream `401`, retryable `5xx`, and `Retry-After`
     when this can be exercised safely through fake upstreams,
   - privacy behavior, especially outbound `store:false` and no local
     persistence of prompts, completions, images, tool arguments, tool results,
     raw request bodies, raw response bodies, full bearer tokens, full request
     IDs, or account IDs.

   Image rejection is a switching blocker for normal Codex use if the current
   CLI emits image inputs through local Responses and ilonasin cannot process
   them. The audit may record the blocker without implementing image support in
   this slice.

4. Classify each probe result.
   - `pass`: Codex CLI completes, route behavior matches the requested feature,
     and ilonasin records only allowed metadata.
   - `partial`: Codex CLI completes but a feature is not actually exercised.
   - `local_missing`: ilonasin rejects or drops a feature it should support.
   - `codex_config_limit`: the CLI cannot express the intended probe.
   - `upstream_limit`: the routed provider rejects or lacks the feature.
   - `unsafe`: forbidden data is logged, stored, rendered, persisted, sent to
     the wrong provider, or sent outside the explicitly selected provider route.

   Normal dispatch of supported prompts, images, and tool data to the explicitly
   selected upstream provider is not a privacy failure by itself. The privacy
   boundary is local storage, logging, rendering, metadata, wrong-route
   forwarding, and unsupported forwarding.

5. Assert routing and metadata invariants per probe.
   - requested provider instance,
   - requested model,
   - resolved provider instance,
   - resolved model,
   - selected local credential ID,
   - HTTP status and normalized error class,
   - retry count and fallback count,
   - same-provider and same-model fallback only,
   - health event class and retry-after behavior when applicable.

6. Write a compatibility report.
   - Add or update `docs/codex-compatibility-audit.md`.
   - Include the current Codex CLI version, safe probe matrix, result classes,
     known blockers, and recommended next numbered plans.
   - Use metadata-only columns: feature, base URL variant, provider instance,
     requested model, resolved model, endpoint family, reached local server,
     local layer reached, status, error class, metadata assertion, privacy
     assertion, result class, switching gate impact, and next fix.
   - Do not include raw prompts, raw completions, JSONL traces, HTTP request
     bodies, response bodies, provider payloads, image bytes, image data URLs,
     tool arguments, tool results, bearer tokens, account IDs, request IDs,
     full paths with query strings, or copied credential state.

7. Define a no-switch gate.
   - Both bare and `/v1` base URL variants must pass model discovery and simple
     text.
   - Developer/system instruction behavior must pass.
   - Reasoning effort and fast or priority service-tier behavior must pass or
     be classified as Codex CLI configuration limits.
   - Tool-loop probes must pass before switching normal coding-agent use.
   - Image probes must pass before switching multimodal use.
   - Privacy scans must pass with zero forbidden marker leaks.
   - `partial` results do not count as compatibility.

8. Apply narrow fixes only when directly proven.
   - Examples: a missing route alias, a strict decoder rejection for a
     non-stateful field Codex always sends, or a smoke harness gap.
   - Do not implement full local Responses tool loops, local Responses
     multimodal support, file upload support, compaction, memories, realtime,
     quota tracking, or account-pooling changes in this slice.

## Out of Scope

- Quota tracking and quota pooling.
- Any work in the separate quota-tracking worktree.
- Broad route refactors not required by probe evidence.
- Switching the user's real Codex config to use ilonasin.
- Persisting raw Codex transcripts, JSONL, request bodies, response bodies,
  image files, image bytes, prompts, completions, tool arguments, or tool
  results.
- Implementing `/files`, `/responses/compact`, memories, realtime, or hosted
  tool endpoints.
- Implementing local Responses tool-loop or multimodal support unless the audit
  finds a one-line compatibility defect rather than a feature gap.

## Implementation Steps

1. Get three senior plan reviews:
   - Codex CLI behavior and probe matrix.
   - Ilonasin architecture and provider boundaries.
   - Privacy, logging, and isolated smoke safety.
2. Run CLI and source discovery.
3. Build a worktree binary and temporary `CODEX_HOME`.
4. Start the daemon with a `trap` that kills and waits for it, disables the
   audit local client token, and removes temp homes, binaries, command
   summaries, workspaces, and generated images on every exit path.
5. Define sentinel markers before running probes:
   - prompt marker,
   - completion marker,
   - image filename and content marker,
   - tool argument marker,
   - tool result marker,
   - local bearer prefix marker,
   - fake request ID marker,
   - fake account ID marker,
   - raw body marker,
   - raw provider payload marker.
6. Generate a non-sensitive local image for image probes. Do not use a user
   image.
7. Run the real-credential probe matrix first, then fake-upstream negative
   probes as needed. Immediately reduce stdout, stderr, JSON events, and
   recorder observations to allowlisted fields. Do not save raw terminal
   transcripts, raw JSONL, raw request bodies, raw response bodies, image bytes,
   prompts, completions, tool arguments, or tool results.
8. Audit temp logs, SQLite metadata, temp `CODEX_HOME`, Codex auth files,
   Codex credential files, session/history/plugin/MCP state, temp workspaces,
   command summaries, and local output for forbidden sentinel leakage.
9. Account for durable metadata left in the real SQLite database after live
   smoke. Disabled audit token rows, request metadata, and health events are
   allowed; raw content, secrets, account IDs, and request IDs are not.
10. Apply any narrow fixes justified by direct evidence.
11. Re-run the relevant probes after fixes.
12. Run standard smoke checks:

```sh
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp" "$tmpbin"' EXIT
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check
```

13. Get three senior reviews of the findings and any code changes.
14. Commit the plan, report, and any narrow fixes with a `Co-Authored-By` line.

## Review Questions for Subagents

1. Does this matrix cover the real risks before using Codex through ilonasin,
   especially multimodal, tool-loop, reasoning, service-tier, route, and
   privacy behavior?
2. Does the plan avoid overclaiming compatibility from direct HTTP support or
   a single text-only smoke?
3. Are the allowed fixes narrow enough to avoid mixing an audit slice with a
   feature implementation slice?
