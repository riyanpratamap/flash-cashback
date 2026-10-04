# API Contract

The HTTP API of Flash Cashback, drafted before the build. Screens are in [ui-wireframe.md](ui-wireframe.md).

This draft is deliberately incomplete. Eight decisions are still open (listed in `AGENTS.md`), and every part of this
contract that depends on one is marked:

> **OPEN — decision N.** The variants, and what each changes here.

After the decision session the markers are replaced by the chosen variant, the owner approves the diff, and from then
on `prd.md` and `tech-spec.md` link here and never restate a request or response shape.

## Settled defaults

Small choices taken without debate. Each has one line of reason; the owner confirms them as a batch and can pull any
of them into the session.

| Topic | Default | Reason |
| --- | --- | --- |
| Base URL | `http://localhost:8080/v1` | One version prefix leaves room for a breaking change |
| Money | Integer IDR in JSON, `int64` and `BIGINT` behind it. `100000` means Rp100.000 | Rupiah has no usable minor unit; integers are exact |
| Rounding | The 5% is computed in integer math and rounded down | Never pays more than 5% of a fixed budget |
| Minimum | A payment of exactly Rp20.000 earns | The brief says "under 20,000" earns nothing |
| Amount range | A whole number from 1 to 10,000,000, for payments and redemptions | A sanity bound; rejects obviously wrong input |
| User identity | `X-User-ID` header matching `^[a-z0-9_-]{1,64}$`, on every route except health. Demo users `user_a`, `user_b`, `user_c` | The brief puts authentication out of scope; a header sits where a verified token would |
| Retries | `Idempotency-Key` header (canonical UUID) on every POST, scoped per user. The same key and body returns the stored result with 200 and `Idempotent-Replayed: true`; the same key with another body is 409 | A timed-out request may have committed; the client must be able to ask again safely |
| Payment and cashback | A valid payment always succeeds; cashback is a separate result and can be Rp0 | The campaign is a bonus on top of paying |
| Budget figures | Never in any response | Business-sensitive, and stale the moment it is shown |
| Time | RFC 3339 with an explicit offset | Unambiguous on any device |
| Codes | `UPPER_SNAKE_CASE`; the app maps codes to copy and has a fallback for a code it does not know | The server can add a code without breaking an older app |
| Errors | `{"error": {"code", "message", "request_id"}}`; `message` is for logs, never shown to users | One shape for every failure |
| Request ID | `X-Request-ID` echoed or generated, on every response and log line | A complaint maps to exact log lines |
| Operations | Command-line tools, not HTTP endpoints | With no authentication, an admin endpoint would be open to anyone |

## Endpoints at a glance

| Endpoint | Purpose | Screens |
| --- | --- | --- |
| `GET /campaign` | Campaign state and rules | Home banner, Pay info line, How it works |
| `GET /me/cashback` | Balance and today's progress | Home, Pay info line, Redeem, History header |
| `POST /payments` | Make a payment; cashback is decided here | Pay, Checking, Payment result |
| `POST /redemptions` | Redeem cashback | Redeem |
| `GET /me/history` | Payments and redemptions, newest first | History, Home recent activity |
| `GET /healthz` | Dependencies reachable | None; Docker Compose and monitoring |

## GET /campaign

```json
{
  "id": "flash-cashback",
  "name": "Flash Cashback",
  "status": "ACTIVE",
  "rules": { "rate_bps": 500, "min_payment": 20000, "daily_cap": 50000 }
}
```

`status` is `ACTIVE` or `ENDED` at least. The rules are served, not hard-coded in the app, so a different rate or cap
needs no app release.

> **OPEN — decision 5 (kill switch).** If a switch exists, add `PAUSED` to `status`. What `PAUSED` stops (awards only,
> awards and redemptions, or two separate flags) decides the copy on Home and Redeem and whether `POST /redemptions`
> gains an error code.

> **OPEN — decision 8 (scope).** An `ENDING_SOON` status (budget below a threshold) is a candidate. It adds a threshold
> setting and one banner; the brief does not ask for it.

> **OPEN — decision 4 (per day).** If the day is a calendar day in one time zone, add `rules.timezone`.

## GET /me/cashback

A user with no activity gets zeros, never a 404.

```json
{
  "balance": 15000,
  "today": { "earned": 47000, "remaining": 3000 }
}
```

> **OPEN — decision 4 (per day).** A calendar day adds `today.date` and `today.resets_at`. A rolling 24 hours has no
> single reset time and needs a different field (for example the time the oldest counted award expires).

## POST /payments

```http
POST /v1/payments
X-User-ID: user_a
Idempotency-Key: 6f1c2a9e-4b7d-4e2a-9c3f-1a2b3c4d5e6f
Content-Type: application/json

{ "amount": 100000 }
```

**201 Created** (or 200 with `Idempotent-Replayed: true` for a retry)

```json
{
  "payment": {
    "id": 42,
    "reference": "PAY-20261003-000042",
    "amount": 100000,
    "status": "SUCCEEDED",
    "created_at": "2026-10-03T14:32:00+07:00"
  },
  "cashback": { "awarded": 5000, "reason": "AWARDED" }
}
```

`cashback.reason` says why the award is what it is. Settled codes: `AWARDED` (the full 5%), `BELOW_MINIMUM`,
`DAILY_CAP_REACHED`, `CAMPAIGN_ENDED`. A Rp0 result has exactly one reason; the order in which the Rp0 reasons are
checked is fixed in the spec.

> **OPEN — decision 2 (award at a limit).**
> A. Partial award: a payment that crosses the daily cap or the end of the budget earns what still fits. Adds
> `PARTIAL_DAILY_CAP` and `PARTIAL_BUDGET`, and a rule for when both limits cut the award.
> B. All or nothing: a payment whose full 5% does not fit earns Rp0 with `DAILY_CAP_REACHED` or `CAMPAIGN_ENDED`. No
> new codes; a user can be stranded below the cap and the budget may never reach zero.

> **OPEN — decision 1 (where the cap and budget are enforced).**
> A. In PostgreSQL, inside the payment's transaction. The response above is final when it returns. A long wait on a
> lock needs a retryable 503 `SERVICE_BUSY`.
> B. A Redis counter reserves the cashback and PostgreSQL records it afterwards. The response may need a pending
> state (`cashback.status`), and a reconciliation between the two stores.

> **OPEN — decision 3 (when the budget is spent).** At award, the budget ends the moment the last cashback is granted.
> At redemption, awards can continue past the budget and `POST /redemptions` needs a way to refuse.

> **OPEN — decision 5 (kill switch).** If a switch exists, add `CAMPAIGN_PAUSED` to the Rp0 reasons.

> **OPEN — decision 7 (real-money risks).** If cashback is granted only once a payment is settled, `cashback` needs a
> pending state. If it stays instant, the simplification is stated in the README.

**Validation.** A body that is not JSON, a missing `amount`, or an `amount` that is not a bare number is 400
`MALFORMED_REQUEST`. A number outside 1 to 10,000,000 or with a fraction is 422 `INVALID_AMOUNT`.

## POST /redemptions

Moves cashback from the balance to the user's main account. The payout is simulated and completes at once; `PENDING`
and `FAILED` are reserved for a real payout.

```http
POST /v1/redemptions
X-User-ID: user_a
Idempotency-Key: 0b8e7d6c-5a4f-4e3d-8c2b-9a8f7e6d5c4b
Content-Type: application/json

{ "amount": 18000 }
```

```json
{
  "redemption": {
    "id": 3,
    "reference": "RDM-20261003-000003",
    "amount": 18000,
    "status": "COMPLETED",
    "destination": "MAIN_ACCOUNT",
    "created_at": "2026-10-03T15:10:00+07:00"
  },
  "balance_after": 0
}
```

Any whole amount from 1 up to the balance; no minimum. An amount above the balance, including two redeems racing, is
422 `INSUFFICIENT_BALANCE` and nothing changes. The amount range is checked before the balance. `balance_after` is the
balance right after this redemption, and a replay returns the same value as the original response.

> **OPEN — decision 5 (kill switch).** If the switch also stops redemptions, add 409 `REDEMPTION_PAUSED`.

## GET /me/history

Payments (Rp0 ones included) and redemptions in one list, newest first. Each item carries what the list row needs.

```json
{
  "items": [
    { "type": "PAYMENT", "id": 42, "reference": "PAY-20261003-000042", "amount": 100000,
      "status": "SUCCEEDED", "created_at": "2026-10-03T14:32:00+07:00",
      "cashback": { "awarded": 5000, "reason": "AWARDED" } },
    { "type": "REDEMPTION", "id": 3, "reference": "RDM-20261003-000003", "amount": 42000,
      "status": "COMPLETED", "destination": "MAIN_ACCOUNT", "created_at": "2026-10-03T11:20:00+07:00" }
  ]
}
```

Amounts are always positive; `type` decides the sign the app shows.

> **OPEN — decision 8 (scope).**
> A. The newest N items only (`limit`, default 20, maximum 50), no paging. Enough for a campaign in which a user makes
> a handful of payments.
> B. Cursor paging (`next_cursor`), stable while new payments arrive. More code and more tests.

## GET /healthz

200 when the service can do its job, 503 when it cannot. The body names each dependency.

> **OPEN — decision 6 (what Redis is for).** If Redis never decides money, Redis down is "degraded" with 200. If Redis
> holds the counters (decision 1 B), Redis down is 503.

## Errors

A 4xx is a definite answer and the app stops. A 5xx or a timeout is an unknown outcome and the app retries with the
same idempotency key; it never resends with a new key by itself.

| Code | HTTP | When |
| --- | --- | --- |
| `MISSING_USER`, `INVALID_USER` | 400 | No `X-User-ID`, or it does not match the pattern |
| `MISSING_IDEMPOTENCY_KEY`, `INVALID_IDEMPOTENCY_KEY` | 400 | POST without the header, or not a canonical UUID |
| `MALFORMED_REQUEST` | 400 | Body or query that cannot be read |
| `INVALID_AMOUNT` | 422 | Not a whole number from 1 to 10,000,000 |
| `INSUFFICIENT_BALANCE` | 422 | Redeem above the balance |
| `IDEMPOTENCY_KEY_REUSED` | 409 | Same key, different body |
| `NOT_FOUND`, `METHOD_NOT_ALLOWED` | 404, 405 | No such route, or the wrong method |
| `INTERNAL_ERROR` | 500 | Anything unexpected |

> **OPEN — decision 6 (what Redis is for).** A rate limit adds 429 `RATE_LIMITED` with `Retry-After`. If there is one,
> a retry of a key already accepted must never be refused, or an unknown outcome could be shown as a failure.

## Operations

| Tool | What it does |
| --- | --- |
| Reconcile command | Checks the money invariants and exits non-zero on any mismatch. It reports; it never repairs |
| Demo state command | Puts the database into a known state for the three demo users. Refuses to run unless a demo flag is set |
| Budget settings | The budget comes from configuration, defaulting to Rp10.000.000, so the ended state can be reached with a small budget |

> **OPEN — decision 5 (kill switch).** If chosen: a pause and resume command.

> **OPEN — decision 8 (scope).** A load-test command (many generated users paying until the budget is gone, then
> reconcile) is a candidate. The concurrency tests already prove the limits; the load test proves them through HTTP.

## Library picks

Confirmed as one batch in the session; any row can be pulled out.

| Area | Pick | Reason |
| --- | --- | --- |
| HTTP router | `chi` | Standard `net/http` handlers, little to learn |
| PostgreSQL access | `pgx` v5, hand-written SQL | The locking statements are the design and should be visible |
| Migrations | `goose`, embedded, run when the API starts | One `docker compose up`, no separate step |
| Redis client | `go-redis` v9 | The common client |
| Logging | Standard library `log/slog`, JSON | No dependency |
| Go checks | `gofmt`, `go vet`, `staticcheck` | Real mistakes, almost no configuration |
| Integration tests | Real PostgreSQL and Redis from a test compose file | The tests depend on real locking |
| CI | GitHub Actions | The repository is on GitHub |
| Mobile | Expo with Expo Router, TypeScript | A reviewer runs it with one command |
| Server data in the app | TanStack Query | Loading, error, and refetch states without hand-written flags |
| Mobile tests | Jest with React Native Testing Library | Tests act as a user would |
| Key generation | `expo-crypto` on the phone, `google/uuid` in Go | `crypto.randomUUID` is not guaranteed in React Native |
