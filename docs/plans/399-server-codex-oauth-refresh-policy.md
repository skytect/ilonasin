# 399 Server Codex OAuth Refresh Policy

## Context

Fresh whole-codebase reviews flagged scattered Codex OAuth/subscription
eligibility rules. The full issue spans server, app keepalive, management
usage, management visibility, and credentials. A broad cross-package capability
rewrite would touch several local DTO boundaries at once.

This slice takes the smallest behavior-preserving step in the server: Codex
OAuth refresh eligibility is repeated across chat, stream, model-discovery, and
initial bearer resolution paths in `internal/server/credentials.go` and
`internal/server/models.go`.

## Goal

Centralize server-local Codex OAuth refresh eligibility so server retry paths
share one policy helper instead of repeating `instance.Type == "codex"`,
`instance.OAuth`, and refresh availability checks.

## Scope

1. Add small server-local helpers near the credential refresh boundary:
   - one helper for Codex OAuth refresh availability on a provider instance;
   - one helper for bearer credential ID availability where needed.
2. Replace repeated server predicates in:
   - initial OAuth bearer resolution retry;
   - non-streaming chat 401 refresh decision;
   - streaming chat 401 refresh decision;
   - model-discovery chat 401 refresh decision;
   - model-discovery stream 401 refresh decision;
   - model list 401 refresh decision.
3. Preserve exact status/error-class checks and behavior.
4. Do not change app keepalive, management subscription usage, management pool
   visibility, credentials package validation, provider adapters, storage,
   routing, quota, logging, TUI, config, or request/response shapes.

## Verification

Add a temporary focused server package check, then remove it before commit. It
must prove the extracted predicates preserve the current truth table:

- initial bearer refresh remains limited to Codex OAuth, an
  `ErrNoEligibleCredential` result, and an available refresh service;
- chat 401 refresh still requires `upstream_auth_failed`;
- stream 401 refresh still requires `upstream_auth_failed`,
  `PreStreamError`, and not `Started`;
- model-credential chat and stream refresh still require a nonzero credential
  ID and `model_discovery_auth_failed`;
- model-list refresh remains Codex OAuth plus HTTP 401 plus refresh service,
  with no new error-class gate.

Run:

```sh
rg -n 'instance\\.Type == "codex"|instance\\.Type != "codex"|s\\.refresh != nil|shouldRefresh.*401|shouldRefreshOAuthAfterModel401' internal/server
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/server
go test ./...
go vet ./...
```

Run the standard temporary `serve` plus `manage` smoke at narrow and wide
terminal widths.

## Acceptance

- Server refresh retry paths share one Codex OAuth refresh availability helper.
- Existing refresh behavior and error classes are unchanged.
- No cross-package DTO or provider behavior changes are introduced.
- Remaining app/management/credentials capability duplication is left for a
  later, explicitly planned slice.
