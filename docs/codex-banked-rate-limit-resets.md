# Codex Banked Rate-Limit Resets

Research date: 2026-07-12 (Singapore time)

## Bottom line

Codex now has **banked rate-limit resets** for eligible accounts. These
are separate from both ordinary 5-hour/weekly quota refreshes and purchased
ChatGPT credits.

A banked reset is an account-bound, non-transferable entitlement that can be
saved and redeemed later to reset an eligible Codex usage-limit window. OpenAI
says banked resets generally expire **30 days after grant**, unless the specific
offer says otherwise.

Ilonasin now collects and exposes a read-only, sanitized banked-reset
inventory. Its original subscription-usage design intentionally ignored fields
named as credits to avoid inferring quota from billing or monetary balances.
Banked resets are the narrow exception: they are non-monetary
quota-entitlement state, not billing credit.

## Do not conflate these four concepts

| Concept | Meaning | Expiration/reset behavior |
| --- | --- | --- |
| 5-hour usage window | Included Codex plan allowance shared by local messages and cloud tasks | Backend reports a `reset_at`; ordinary capacity refreshes without spending a banked reset |
| Weekly usage window | Longer included Codex allowance that may apply in addition to the 5-hour window | Backend reports a separate `reset_at` |
| Banked rate-limit reset | Earned/promotional entitlement that can reset eligible Codex rate limits on demand | Generally must be used within 30 days of grant; does not roll over indefinitely |
| Purchased ChatGPT credits | Paid usage balance used after included limits | Plus/Pro purchased credits are valid for 12 months; separate terms apply to Business/Enterprise/Edu |

A banked reset is not API credit, cash, a transferable balance, or an OAuth
token refresh.

## How banked resets are collected

OpenAI's official June 11, 2026 release notes say eligible Plus and Pro users
received:

- one free banked reset at launch; and
- the ability to earn resets through eligible referral offers.

For a referral reward, sending an invitation is not sufficient. The invitee
must accept through the official flow, satisfy the offer's eligibility rules,
complete the qualifying Codex action (often a first Codex message), and pass
OpenAI's checks. When the offer says so, both parties receive a reset.

Exact availability, caps, cooldowns, plans, workspaces, regions, qualifying
actions, and deadlines vary by offer. The initial public referral window was
June 11–24, 2026 and has ended as of this research date. A current account may
still hold unexpired rewards granted during that window, and future or
account-specific offers may differ.

Self-referrals, duplicate redemptions, aliases used to evade restrictions, and
other abusive activity are ineligible.

## Expiration

Official terms state:

> For offers that grant a banked Codex rate-limit reset, the reset must generally
> be used within 30 days after it is added to the bank, unless the offer says
> otherwise.

The current Codex protocol can expose each banked reset's:

- opaque reset-record ID;
- type;
- status;
- `grantedAt` Unix timestamp;
- nullable `expiresAt` Unix timestamp;
- optional backend title and description.

Therefore Ilonasin should use the returned `expiresAt`, not calculate 30 days
locally. The 30-day statement is a default policy, while the per-record backend
timestamp is authoritative for a particular grant.

Current Ilonasin read-only storage does not persist the upstream reset-record
ID at all. No hash or local handle exists that can be redeemed later.

## How to view and use a reset

Official user-facing flow:

1. Open Codex's profile/usage summary, or use the current Codex TUI `/usage`
   flow when supported.
2. Check the number and expiry of available banked resets.
3. Select **Redeem usage limit reset**.
4. Confirm the reset.
5. Refresh the rate-limit snapshot and verify the intended quota window actually
   changed.

Current Codex protocol evidence has two layers.

### Backend HTTP boundary

The authenticated usage response at `GET /wham/usage` (or
`GET /api/codex/usage` for Codex-API-style bases) can include the summary:

```text
rate_limit_reset_credits.available_count
```

Detailed inventory is a separate authenticated request:

```text
GET /wham/rate-limit-reset-credits
GET /api/codex/rate-limit-reset-credits
```

Its payload contains `available_count` plus `credits[]`. Each detail row can
contain `id`, `reset_type`, `status`, `granted_at`, `expires_at`, `title`, and
`description`. The backend may cap detail rows, so list length need not equal
`available_count`.

The redemption endpoint is:

```text
POST /wham/rate-limit-reset-credits/consume
POST /api/codex/rate-limit-reset-credits/consume
```

Its backend JSON uses `redeem_request_id` and optional `credit_id`.

### Codex app-server boundary

Codex app-server combines the usage summary and optional detail request behind:

```text
account/rateLimits/read
```

and exposes redemption through:

```text
account/rateLimitResetCredit/consume
```

with parameters:

```json
{
  "idempotencyKey": "caller-generated UUID reused for retries",
  "creditId": "optional opaque selected reset-record ID"
}
```

Current documented outcomes:

- `reset` — reset accepted;
- `nothingToReset` — no eligible window currently needs resetting;
- `noCredit` — no available banked reset;
- `alreadyRedeemed` — the same idempotent redemption previously completed.

Ilonasin should use the backend HTTP boundary directly inside its provider
adapter; it should not run or speak to a Codex app-server. After any successful
or idempotent redemption outcome, refetch both usage and inventory. Do not infer
which windows changed from the consume response.

## When a reset is valuable

A reset is most valuable when all of the following are true:

- useful Codex work is blocked or about to be blocked;
- a limiting 5-hour or weekly window is substantially exhausted;
- its natural reset is still far enough away to disrupt the work;
- the banked reset would otherwise expire before a better opportunity; and
- the account is healthy and the backend reports an eligible window.

Prefer the **earliest-expiring** eligible reset when individual details are
available.

Avoid redeeming when:

- normal quota is still ample;
- the natural reset is imminent;
- there is no eligible window (`nothingToReset`);
- the account usage snapshot is stale or contradictory;
- the backend cannot confirm a reset was applied; or
- another account in the legitimate Ilonasin pool can handle the work without
  consuming an expiring entitlement.

A safe recommendation rule is advisory rather than automatic:

```text
Recommend redemption if blocked,
or if a reset expires soon and the limiting window is nearly exhausted.
```

Ilonasin should not automatically redeem banked resets by default. Redemption
has real scarcity and there are public reports of resets being consumed while
usage state failed to update. Require explicit operator confirmation and verify
with a fresh rate-limit read.

## What Ilonasin supports today

Existing design and implementation cover:

- Codex primary/5-hour `used_percent`, `window_minutes`, and `reset_at`;
- secondary/weekly usage and its separate `reset_at`;
- per-account sanitized subscription-usage snapshots;
- summative pool rows and earliest future reset;
- sanitized banked-reset available count, detail availability, detail error
  class, reset type, status, grant time, and expiry;
- stale/error status;
- management-only refresh and TUI rendering;
- keepalive status scaffolding.

Relevant files:

- `docs/plans/104-codex-subscription-usage.md`
- `internal/provider/codex_usage.go`
- `internal/management/subscription_usage_dto.go`
- `internal/management/subscription_usage_response.go`
- `internal/management/subscription_usage_sanitize.go`
- `internal/storage/sqlite/subscription_usage.go`

Plan 104 explicitly ignored balances and fields named as credits because those fields were
originally treated as billing-sensitive and out of scope. That remains correct
for monetary balances, but banked rate-limit resets are now a distinct
quota-control type.

## Recommended Ilonasin design

### Phase 1: read-only collection

The current implementation extends the existing Codex subscription-usage
provider boundary to parse the sanitized banked-reset summary from the
authenticated usage response and fetch optional detail rows from the matching
backend inventory endpoint. It does not spawn or depend on a Codex app-server
process and does not make the management layer speak app-server RPC. The
app-server RPC and schemas remain protocol evidence for the typed fields and
behavior; the provider adapter remains the integration boundary. If the usage
response has no summary, banked-reset inventory is unavailable. If a summary
count exists but the inventory request is unavailable, Ilonasin preserves the
authoritative count and reports that detail rows are unavailable:

```text
available_count
reset_type
status
granted_at
expires_at
observed_at
```

Current read-only code intentionally drops raw upstream reset-record IDs and
does not persist a hash or local handle. No stored value is redeemable. Do not
expose bearer tokens, full account IDs, raw provider payloads, billing balances,
or unrelated credit data.

Future redemption requires a separately reviewed daemon-owned protected mapping
from local selection to upstream reset ID, with lifecycle, retention, and
cleanup rules. That mapping is not implemented.

Expose per-account reset inventory through the existing local Unix management
API and TUI. Aggregate only counts and nearest expiry across the normal Codex
pool.

Suggested display:

```text
Normal Codex pool
Banked resets: 3 available · next expiry in 6d

account-a  2 available · expires Jul 18, Jul 29
account-b  1 available · expires Aug 02
```

Distinguish these states:

- available count is zero;
- account is ineligible;
- backend omitted reset data;
- fetch failed;
- details unavailable but count positive;
- credit expired/redeemed between read and action.

### Phase 2: expiry monitoring

Add warnings without automatic redemption:

- warn at 7 days and 24 hours before expiry;
- deduplicate warnings per credit and threshold;
- surface nearest expiry in the TUI usage pane;
- show an action recommendation only when quota pressure makes redemption useful.

### Phase 3: explicit redemption

If implemented, redemption must:

1. be a management-only mutable operation;
2. require explicit confirmation;
3. select an exact credit, preferably earliest-expiring;
4. use a daemon-owned protected upstream reset-ID mapping that is not exposed
   through management snapshots, TUI, logs, or normal telemetry;
5. use a caller-generated UUID idempotency key and reuse it on retry;
6. serialize concurrent redemptions per account;
7. refetch reset credits and all rate-limit windows after the call;
8. report `reset`, `nothingToReset`, `noCredit`, or `alreadyRedeemed` precisely;
9. never claim success solely because the available count decreased.

Do not add automatic redemption until the backend's reset scope is sufficiently
predictable and explicit operator policy exists.

## Normal unused allowance is not collectible

OpenAI does not state in one explicit Codex sentence that unused included
allowance never rolls over. However, the ordinary 5-hour and weekly limits are
rate-limit windows, not stored balances. The safe operational interpretation is
that remaining allowance is replaced at its displayed reset rather than added
to the next window.

Both windows constrain an account simultaneously. A 5-hour refresh does not
help when the weekly window remains exhausted, and a weekly refresh does not
help when the 5-hour window remains exhausted. Use each account's live backend
timestamps; OpenAI does not publicly guarantee a universal reset anchor.

## Keepalive is a separate idea

`docs/plans/104-codex-subscription-usage.md` also proposed tiny requests at
07:00, 12:00, 17:00, and 22:00 to establish predictable 5-hour window anchors.
That is unrelated to banked reset collection. Public issue reports suggest
window anchoring can depend on first use or even opening a client, but OpenAI's
official documentation does not establish that behavior as a stable contract.
Do not send synthetic keepalives merely to manipulate quota timing without
fresh direct evidence and an explicit operator decision.

## Current Ilonasin behavior and live evidence

Read-only source and earlier management-socket inspection found:

- subscription snapshots preserve per-account `primary` and `secondary`
  `used_percent`, window duration, UTC `reset_at`, and `observed_at`;
- current source also persists sanitized banked-reset count, detail availability,
  detail error class, reset type, status, UTC grant time, and UTC expiry;
- current source persists no upstream reset-record IDs, hashes, or redeemable
  local handles;
- 300-minute primary windows render as `5h`; 10,080-minute secondary windows
  render as `weekly`;
- per-account reset timestamps remain present even when already past;
- pool `earliest_reset_at` includes only strictly future resets;
- subscription snapshots are advisory and are not consulted by routing;
- routing eligibility instead uses model-scoped request quota events and an
  `ActiveUntil` derived from the latest retry/reset evidence;
- blocked credentials become eligible implicitly on the next routing decision
  after `ActiveUntil`; there is no reset wake-up mutation;
- the daemon does not continuously refresh subscription usage solely because a
  displayed reset time passed.

Sanitized live normal-Codex data at `2026-07-12T09:56Z` showed six rows. Five
were observed that day; one was last observed on June 21 but still had
`stale=false` and contributed to pool totals. All primary reset timestamps were
already past, so no primary future pool reset was emitted. This proves the
current `stale_count` and pool totals are not sufficient for a trustworthy
"capacity coming online" schedule.

A future schedule should distinguish:

```text
Routing availability = enabled + resolvable auth + no active quota block
Advisory capacity     = fresh subscription windows + observed reset times
Banked capacity       = available reset credits + individual expiry times
```

Only label a timestamp **capacity coming online** when the row is fresh, the
relevant window is exhausted or actively blocked, its future reset is known,
and no stricter overlapping window remains exhausted beyond that time.
Otherwise call it an **observed reset time**.

## Terms and authorization caveat

OpenAI prohibits sharing personal account credentials and circumventing rate
limits. Multi-account scheduling is safest only where each account is operated
by its authorized owner or accounts are properly provisioned workspace seats.
Ilonasin should not describe personal-account rotation as a supported way to
merge quotas, reset credits, or purchased balances.

## Evidence and caveats

### Official sources

1. OpenAI Help Center, **Codex Referral Promotions**
   https://help.openai.com/en/articles/20001271-codex-referral-promotions
   - Offer-specific eligibility and rewards.
   - Banked resets generally expire 30 days after grant.
   - Resets are not API/cash/transferable credits.

2. OpenAI Help Center, **Using Codex with your ChatGPT plan**
   https://help.openai.com/en/articles/11369540-using-codex-with-your-chatgpt-plan
   - Profile-menu usage summary is the user-facing redemption surface.
   - Options after reaching a limit include available reset, credits, upgrade,
     or waiting.

3. OpenAI ChatGPT release notes, **June 11, 2026 Codex updates**
   https://help.openai.com/en/articles/6825453-chatgpt-release-notes
   - Launch of eligible Plus/Pro reset banking, one launch reset, referrals,
     and 30-day usability.

4. OpenAI Codex pricing
   https://developers.openai.com/codex/pricing
   - Local messages and cloud tasks share a five-hour window; weekly limits may
     also apply.

5. OpenAI Codex app-server documentation and schemas
   https://developers.openai.com/codex/app-server
   https://github.com/openai/codex
   - Read and consume RPCs, idempotency, count/details shape, per-credit expiry,
     and redemption outcomes.

6. OpenAI Help Center, **Using Credits for Flexible Usage**
   https://help.openai.com/en/articles/12642688-using-credits-for-flexible-usage-in-chatgpt-freegopluspro-sora
   - Included usage is consumed first; eligible purchased credits extend usage.
   - Individual purchased credits expire 12 months after purchase and do not
     roll over after expiry.

7. OpenAI account sharing policy and Terms of Use
   https://help.openai.com/en/articles/10471989-openai-account-sharing-policy
   https://openai.com/policies/row-terms-of-use/
   - Personal credentials must not be shared and rate limits must not be
     circumvented.

### Lower-confidence operational evidence

Open GitHub issues report:

- inconsistent reset scope between 5-hour and weekly windows;
- credits consumed without reliably updated usage state;
- disappearing or unavailable launch/referral credits; and
- unclear immediate resets versus banked grants.

These are user reports, not proof of universal behavior. They justify cautious
verification and explicit confirmation, not claims that the feature is always
broken.

## Product implication

The useful Ilonasin feature is broader than showing normal quota reset times:

```text
current quota + natural reset schedule + banked reset inventory + expiry
```

This lets the operator answer:

- Which accounts are about to regain normal capacity?
- Which accounts hold banked emergency capacity?
- Which banked resets will expire unused?
- Is redeeming one now better than waiting or routing to another legitimate
  pooled account?

That should be the basis of the future Ilonasin usage/TUI work.
