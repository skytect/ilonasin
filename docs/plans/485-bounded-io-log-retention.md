# 485 Bounded IO Log Retention

## Context

The current IO logger is safe behind `[logging].capture_io`, but it writes every
IO record to one append-only `ilonasin-io.log` file. For local debugging this is
useful, but it can produce a huge file during active client traffic and makes
the binary IO logging boundary operationally risky.

The architecture permits IO body persistence only under the explicit
`capture_io` switch. It does not require that all captured IO live forever.

## Goal

Bound IO log growth by rotating `ilonasin-io.log` at the logging boundary and
retaining only a small configured number of local debug files.

## Scope

1. Add config-owned IO log retention fields under `[logging]`:
   - `io_max_bytes`, default `52428800`;
   - `io_max_files`, default `3`.
2. Pass those fields through app bootstrap into `logging.IOOptions`.
3. Update `internal/logging/io.go` so `IOLogger.Record` rotates before writing
   when the active file plus the next JSON line would exceed `io_max_bytes`.
4. Keep current JSONL record shape, `capture_io` gate, secret scrubbing,
   file mode `0600`, log directory mode `0700`, and `ilonasin-io.log` base
   filename.
5. Retain rotated files as `ilonasin-io.log.1`, `.2`, and so on up to
   `io_max_files - 1`; remove older files. `io_max_files = 1` means active file
   only, with no rotated files. Values below `1` fall back to the default.
6. Expose safe retention policy in management `RuntimeStatus`.
7. Render the policy compactly in the TUI IO policy/pruning pane.
8. Update `docs/ilonasin-architecture.md` as the authoritative policy so the IO
   logging exception is explicitly bounded by rotation. Add a note to plan `300`
   that the bounded behavior supersedes that earlier unbounded-file wording.
9. Do not add permanent tests.

## Non-Goals

- No SQLite storage for IO bodies.
- No TUI file browser.
- No remote log shipping.
- No compression.
- No change to normal `ilonasin.log` retention.
- No change to what content is eligible for IO logging.
- No change to provider capture enablement or debug-level upstream capture.

## Implementation Notes

Rotation should happen while holding the IO logger mutex. The logger should
encode each record into a buffer first, use the encoded byte length for the
rotation decision, then write that exact line to the active file. If a single
record exceeds the configured max, rotate first when the active file is nonempty
and then write the oversized record rather than truncating JSON or payloads.
This means the retention bound is practical rather than mathematically hard:
disk use is bounded by `io_max_files * max(io_max_bytes, largest single encoded
record seen in retained files)`.

Rotation failure handling:

- close the active file before rotating;
- remove the oldest retained file first;
- rename rotated files in descending order, for example `.2` to `.3`, then `.1`
  to `.2`, then active to `.1`;
- reopen a new active file with `0600` before writing the pending record;
- apply `home.SecureFile` or equivalent permission tightening to every active
  and rotated IO log path touched by rotation, including pre-existing files;
- if rotation, reopen, chmod, or write fails, keep the current record out of the
  IO log rather than writing a partial JSON object;
- report rotation/write failures through metadata-only structured application
  logging when a logger is available, without bodies, bearer values, local
  tokens, upstream credentials, account IDs, or payload snippets;
- detect short writes and treat them as write failures.

`Record` can keep its no-return call shape, but internally it should avoid
corrupting JSONL: each record is encoded once to a complete line buffer, and the
logger writes only that complete line to the active file.

Configuration should be forgiving for old configs: missing or non-positive
values fall back to defaults. This keeps existing `capture_io = true`
configurations functional but bounded after upgrade.

## Verification

Run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run a direct CLI smoke with a temporary binary and isolated home:

- start `ilonasin serve` with `capture_io = false` and confirm no
  `ilonasin-io.log` exists;
- start `ilonasin serve` with `capture_io = true`, small `io_max_bytes`, and
  `io_max_files = 2`;
- create a temporary local token and send authenticated disposable route
  requests with body markers large enough to force rotation, reaching the
  captured route path even if provider dispatch fails later;
- confirm `ilonasin-io.log` and `ilonasin-io.log.1` exist, `.2` does not exist,
  files are mode `0600` where Unix mode applies, and known local tokens are not
  present;
- check the management snapshot includes the retention policy;
- run bounded `ilonasin manage` through a PTY and confirm the IO policy pane
  shows capture mode plus max size/file count.
- run one old-config compatibility smoke with only `capture_io = true` under
  `[logging]` and confirm the daemon starts with bounded defaults.

## Acceptance

- IO logging remains opt-in and secret-scrubbed.
- IO log disk growth is bounded by `io_max_files * max(io_max_bytes, largest
  single encoded record seen in retained files)`.
- Existing configs keep working with bounded defaults.
- TUI shows the bound through daemon-owned management data.
- No raw IO data is exposed through management snapshots or the TUI.
- Rotation errors and retention metadata never include bodies, bearer values,
  local tokens, upstream credentials, account IDs, or raw payload snippets.
