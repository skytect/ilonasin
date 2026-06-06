# 495 Codex Cyber Use Verification Research

## Context

The user wants to understand whether ilonasin can route Codex OAuth traffic to
accounts that have cyber-use verification, and how to determine that safely.
This requires source-grounded research before implementation because the
relevant signals may be account state, access-token claims, rate-limit payloads,
model availability, error classes, or interactive verification endpoints.

The installed Codex CLI is `codex-cli 0.137.0`, and a matching source snapshot
is available at `/tmp/codex-src-0.137.0/codex`.

## Goal

Document how Codex exposes or implies cyber-use verification state and propose a
clean ilonasin implementation approach for routing requirements without
inventing unsupported account state.

## Research Questions

1. What Codex source paths mention cyber safety, cyber use, verification,
   account verification, usage limits, or account capability gates?
2. What stages appear in Codex source or official Codex documentation:
   unverified, verification required, pending, verified, rejected, expired, or
   unknown?
3. What signs can ilonasin safely observe:
   - access-token or ID-token claims;
   - account/profile payloads;
   - subscription usage responses;
   - model list or capability differences;
   - 401/403/429 error classes and response fields;
   - explicit Codex settings or verification endpoints;
   - absence of evidence.
4. Does determining the state require persisting metadata, and if so which
   metadata is safe under the architecture?
5. What is the cleanest ilonasin design for a future routing feature:
   configuration shape, credential metadata, management/TUI display, routing
   requirement semantics, failure mode, and refresh behavior?

## Scope

1. Inspect local Codex source under `/tmp/codex-src-0.137.0/codex`, especially
   `codex-rs/codex-api`, auth/session code, backend OpenAPI models, account or
   usage clients, rate-limit handling, and any cyber-safety modules.
2. Inspect existing ilonasin docs and Codex research docs for relevant current
   architecture constraints.
3. Use local Codex source first. Use only official OpenAI/Codex documentation
   as fallback or supporting context. Do not use forums, unofficial reverse
   engineering, private account probing, or undocumented live endpoints as
   evidence in this slice.
4. Add a research document under `docs/`, likely
   `docs/codex-cyber-use-verification.md`.
5. Update `docs/ilonasin-architecture.md` only if the research establishes a
   durable privacy or routing constraint that should govern future
   implementation. Do not update architecture for product speculation.
6. Do not implement routing, storage schema, config, TUI, provider behavior, or
   live account probing in this slice.
7. Do not inspect local user credential files, account-specific tokens, cookies,
   browser state, or private account data.
8. Do not add permanent tests.

## Verification

Run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Check the research artifact for privacy leaks. It must not contain local
credential paths, token values, cookies, full account IDs, account emails, full
request IDs, raw provider payloads, or private account state.

Run direct CLI smokes with a temporary binary:

1. Start `ilonasin serve` with isolated home and a valid config.
2. Verify management health and snapshot over the Unix socket.
3. Run bounded `ilonasin manage` through a PTY at narrow and wide widths.
4. Clean up all temporary files and processes.

## Acceptance

- Research cites concrete Codex source paths and line references.
- Any official OpenAI/Codex documentation used as supporting context is cited
  with URL and access date.
- Research distinguishes proven signals from inference and unknowns.
- Research directly answers whether ilonasin can safely route by cyber-use
  verification now, what evidence supports that answer, what remains unknown,
  and what must not be implemented without stronger proof.
- Unknown verification state is not treated as verified, and absence of
  evidence is not treated as routing eligibility.
- Research proposes a future implementation boundary that preserves provider,
  credential, routing, management, TUI, logging, and SQLite separation.
- No runtime behavior changes are made in this slice.
