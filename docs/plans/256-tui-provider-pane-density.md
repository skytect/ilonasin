# 256 TUI Provider Pane Density

## Goal

Make the Providers section of `ilonasin manage` match the current
screen-sized dashboard direction without changing daemon behavior.

The Providers section already owns the right concerns: provider instances,
upstream API keys, OAuth/provider accounts, model cache, and fallback groups.
The remaining issue is presentation. Upstream keys, OAuth accounts, provider
accounts, and fallback groups still render as repeated cards, which makes the
section less compact than API and Logs and increases pane-local scrolling.

## Scope

1. Keep the current top-level sections and pane IDs unchanged.
2. Keep existing management DTOs, actions, selection state, and sanitizers.
3. Replace repeated provider credential cards with compact row blocks:
   - upstream API keys show status, ID/label, provider, kind, fallback group,
     safe key fragment, and created/disabled times;
   - OAuth rows show selected cursor, safe email/display label, provider,
     credential ID, plan, expiry, refresh state, and safe refresh failure text;
   - provider account rows show safe email/display label, provider, credential
     ID, and plan;
   - fallback group rows show status, provider/group, credential kind,
     credential count, and explicit/default policy.
4. Preserve empty-state cards because they are compact explanatory states.
5. Keep safe email-like labels visible where already exposed by management DTOs.
6. Do not render raw upstream account IDs, raw API keys, OAuth tokens, bearer
   tokens, full account IDs, request IDs, prompt/completion bodies, or unsafe
   identity markers.
7. Ensure OAuth refresh failure descriptions go through the same unsafe-marker
   redaction expectations as other displayed provider text before rendering.
8. Add only tiny TUI helpers if they reduce duplicated row formatting.

## Boundaries

- No management API, DTO, storage, schema, provider, server route, Anthropic,
  logging, config, or action-routing changes.
- No direct SQLite or `config.toml` mutation from TUI.
- No permanent tests.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
```

Run a temporary focused in-package render smoke, then remove it before commit:

- seed upstream API keys, OAuth credentials, provider accounts, and fallback
  groups;
- include safe email-like labels and field-specific unsafe marker strings,
  including OAuth `RefreshFailureDescription`;
- render the Providers tab at 80, 120, 160, and 220 columns;
- assert safe email-like labels render;
- assert unsafe marker strings remain redacted;
- assert provider credential, OAuth, provider account, and fallback populated
  paths no longer call the card-grid renderer or `renderMetricAccentCard`;
- assert pane IDs and pane order are unchanged;
- assert output lines fit the requested widths after ANSI stripping.

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO capture
   disabled, keepalive disabled, and at least two provider instances.
3. Verify the management health endpoint over the management socket.
4. Run `manage` under a short timeout and verify API/providers/usage/logs chrome
   renders.
5. Remove all temporary artifacts.

## Acceptance

- Providers pane repeated rows are compact row blocks instead of card grids.
- Empty states remain clear and compact.
- Safe email-like account labels remain visible.
- Secret-like or unsafe identity markers remain redacted.
- Pane identity, pane order, key routing, and daemon-backed boundaries are
  unchanged.
- Compile, vet, focused render smoke, serve smoke, manage smoke, and
  implementation review pass.
