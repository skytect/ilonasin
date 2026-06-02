# 315 TUI Wrap Log Group Followup

## Context

Recent TUI feedback shows remaining readability issues in the Usage and Logs
sections:

- long safe details should wrap instead of showing ellipses;
- request and fallback logs should read more like compact tables;
- multi-line items need blank separation so adjacent records do not blur
  together;
- GPT 5.5 subscription usage should be grouped above GPT 5.4 Spark usage
  instead of being interwoven by provider.

This is a TUI-only rendering slice. It must not change management DTOs, quota
math, storage, provider behavior, server routes, OAuth refresh, config
mutation, Anthropic compatibility, or logging capture policy.

## Plan

1. Preserve blank lines while pane content is wrapped, so explicit record
   separators survive the final pane renderer.
2. Keep logs table-like with compact summary columns and wrapped continuation
   details. Avoid hiding safe identifying values behind ellipses.
3. Sort subscription account groups and pool groups by limit priority before
   provider, so GPT 5.5 groups render before GPT 5.4 and Spark groups.
4. Keep pooled quota rows summative only, with one combined used/left bar per
   pool window.
5. Review the diff for ANSI width behavior, sanitizer use, accidental non-TUI
   edits, and unintended quota semantics changes.

## Verification

Run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/tui
go test ./...
go vet ./...
```

Run a temporary focused render smoke, then remove it before commit. It should
seed long safe request and fallback details, unsafe marker-shaped values, mixed
GPT 5.5 and GPT 5.4 Spark subscription rows, and pooled summative windows.

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with a temporary `ILONASIN_HOME`, checking management health over the Unix
socket, running `ilonasin manage` under bounded narrow and wide terminals, and
cleaning up the daemon and temporary directory.

## Acceptance

- Safe long usage and log details wrap without literal `...` in body content.
- Logs read as compact table rows with wrapped details beneath each item.
- Multi-line usage and log items have visible blank separation.
- GPT 5.5 subscription usage appears above GPT 5.4 Spark usage.
- Pooled quota rows remain summative only.
- The final diff touches only this plan and TUI render files.
