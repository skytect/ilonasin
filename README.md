# Ilonasin

Ilonasin is a local LLM router with OpenAI, Responses, and Anthropic compatible
APIs, provider credentials, and a daemon-backed management TUI.

Run `ilonasin serve` to start the daemon and `ilonasin manage` to manage it.
Configuration defaults to `~/.ilonasin/config.toml`; `ILONASIN_HOME` changes
the home directory. API requests require an Ilonasin local client token,
separate from upstream credentials.

## Model and account selection

Use `<provider-instance>/<model>` for an explicit provider, or a bare model ID
when discovery identifies exactly one matching provider. For example,
`codex/gpt-6-astra` and `gpt-6-astra` select the same model when `codex` is its
only configured provider.

Append `/daybreak-<name>` to select a Codex account cohort independently of
the model:

```text
gpt-5.6-sol/daybreak-blue
gpt-6-astra/daybreak-red
codex/gpt-5.6-sol/daybreak-blue
```

These are syntax examples, not promises that an account is available. The same
credential must advertise both the base model and the upstream cohort marker,
such as `gpt-daybreak-blue-latest`. Names are discovered from those markers;
there is no colour allowlist. Ilonasin sends the base model upstream unchanged.
The suffix filters eligible accounts and persists through retries; unavailable
cohorts fail rather than falling back to an ordinary account or another model.

Model listings and request eligibility share per-credential discovery. Partial
refreshes merge safe routing metadata without deleting previously known models;
complete refreshes replace snapshots and can remove retired models. Cached
addresses do not establish account entitlement.

## Subscription keepalive

Keepalive is opt-in and sends a short request on each resolved Codex OAuth
credential at the configured times:

```toml
[subscription_keepalive]
enabled = true
timezone = "Asia/Singapore"
schedule_times = ["07:00", "12:00", "17:00", "22:00"]
```

`timezone` accepts an IANA timezone or `local`, the default. Invalid timezones
are rejected when loading configuration. The times above are the default
schedule.

Omit `model` to use each account's visible advertised model with the lowest
upstream picker priority, breaking ties by model ID. Daybreak account markers
are excluded. An explicit `model` must be advertised by that account; missing
models or failed discovery skip the request. Reasoning and verbosity use
upstream defaults. No model price or reasoning level is guessed.

The Codex output cap remains unverified, so enabled keepalive reports
`enabled_uncapped`. The short prompt is not a hard token limit.

See [the architecture](docs/ilonasin-architecture.md) for routing, persistence,
credential, and privacy boundaries.
