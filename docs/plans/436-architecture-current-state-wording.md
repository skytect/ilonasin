# 436 Architecture Current State Wording

## Context

Plan 426 identified three stale wording issues in `docs/ilonasin-architecture.md`:

- the document labels itself as a draft architecture plan even though current
  slices use it as the active target architecture;
- subscription account provider-terms policy appears both in Deferred Research
  and as a separate Open Question;
- the SQLite table section says exact schema is deferred even though live
  migrations now define the concrete schema.

These are documentation alignment issues, not runtime bugs.

## Goal

Update architecture wording to match the current project state without claiming
the whole codebase has reached final zero-debt completion.

## Scope

1. Update `docs/ilonasin-architecture.md`.
2. Change the status line to describe the document as the active target
   architecture for current implementation work.
3. Reword the SQLite section so it distinguishes:
   - conceptual table boundaries in the architecture document;
   - concrete schema details owned by live SQLite migrations.
4. Consolidate the duplicated subscription provider-terms question into the
   Deferred Research list and remove the separate Open Questions section if it
   becomes empty.
5. Rename or reframe `MVP Target` as the current implemented product surface,
   preserving the existing route/provider/TUI list.
6. Do not change runtime code, schemas, migrations, config, management API, TUI,
   logging, provider behavior, or request/response behavior.
7. Do not add permanent tests.

## Verification

Run:

```sh
rg -n "Status:|draft architecture|MVP Target|Current Implemented|Open Questions|Exact schema is deferred|Conceptual SQLite Tables|subscription account fallback|provider-term" docs/ilonasin-architecture.md
git diff --check
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health and
snapshot over the Unix socket, running bounded `ilonasin manage` at narrow and
wide terminal widths, and cleaning up all temporary files and processes.

## Acceptance

- The architecture document reads as the active target architecture.
- Current SQLite wording points concrete schema details to migrations.
- Subscription provider-terms research appears once.
- The implemented surface list is no longer framed as a future MVP target.
- No runtime behavior changes.
