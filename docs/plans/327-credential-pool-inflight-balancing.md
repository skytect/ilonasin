1. Ground the pooling signal set in observed clients.
   - Codex 0.135 Responses sends `prompt_cache_key` as the thread ID in the
     JSON body, plus session and thread headers.
   - Generic OpenAI Chat and Anthropic Messages clients may send only model and
     message content, so `user`, `session_id`, and metadata are optional
     affinity signals, not out-of-box balancing.

2. Add daemon-local in-flight pressure tracking for credential attempts.
   - Keep the tracker in `internal/server` as an in-memory `Server` helper.
   - Key counts by resolved provider instance ID, resolved provider model ID,
     and credential ID.
   - Do not persist request content, identifiers, affinity keys, or pressure
     state.
   - Do not log or expose affinity keys through management, TUI, metadata, or
     error responses.

3. Apply pressure only when no request affinity exists.
   - Preserve existing deterministic affinity ordering when Responses
     `prompt_cache_key`, Chat `session_id`, or Chat `user` is present.
   - For empty affinity, reorder the eligible same-provider, same-model
     credential pool by current in-flight count, with existing deterministic
     ordering as the tie-breaker.
   - Include Anthropic Messages after its local translation to Chat, since it
     normally has an empty affinity key.
   - Keep quota-block filtering for all requests, before any pressure choice.
   - Select the least-loaded credential and increment its in-flight count under
     the same tracker lock, so concurrent requests cannot all pick the same
     zero-count credential.

4. Track each upstream attempt lifetime.
   - Acquire the selected credential immediately before each non-streaming
     `CompleteChat` call and release after the call returns.
   - Acquire the selected credential immediately before each streaming
     `StreamChat` call and release after the stream call returns.
   - Treat `StreamChat` as synchronous until the upstream stream is fully
     consumed; release with a scoped `defer` around each adapter call.
   - Treat same-credential OAuth refresh retries and fallback attempts as
     separate attempt lifetimes.

5. Preserve boundaries and behavior.
   - Do not change credential storage, quota storage, model routing, provider
     adapters, fallback policy rows, management routes, TUI, or IO logging.
   - Do not introduce cross-provider or cross-model fallback.
   - Keep fallback and health metadata recording unchanged.

6. Verify directly.
   - Use a temporary focused server test file if useful, then remove it before
     commit unless it is an existing permanent test file.
   - Run `git diff --check`, `go test ./internal/server`, `go test ./...`,
     `go vet ./...`, and build `cmd/ilonasin`.
   - Include a temporary concurrent focused check or race-enabled package check
     if needed to prove overlapping no-affinity reservations spread and counts
     return to zero.
   - Smoke `ilonasin serve` with a temporary home and `bind = "127.0.0.1:0"`,
     check `/_ilonasin/manage/health` over the management socket, run a short
     `ilonasin manage` TUI smoke, then clean up temporary files and processes.
