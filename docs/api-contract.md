# API Contract

The HTTP API of Flash Cashback, drafted before the build. Screens are in [ui-wireframe.md](ui-wireframe.md).

Final after the decision session. Every choice here is recorded, with its reason, in [DECISIONS.md](DECISIONS.md) and
cited by ID. `prd.md` and `tech-spec.md` link here and never restate a request or response shape.

## Settled defaults

Confirmed as one batch (D10–D23).

| Topic | Default | Reason |
| --- | --- | --- |
| Base URL | `http://localhost:8080/v1` | One version prefix leaves room for a breaking change |
| Money | Integer IDR in JSON, `int64` and `BIGINT` behind it. `100000` means Rp100.000 | Rupiah has no usable minor unit; integers are exact |
| Rounding | The 5% is computed in integer math and rounded down | Never pays more than 5% of a fixed budget |
| Minimum | A payment of exactly Rp20.000 earns | The brief says "under 20,000" earns nothing |
| Amount range | A whole number from 1 to 10,000,000, for payments and redemptions | A sanity bound; rejects obviously wrong input |
| User identity | `X-User-ID` header matching `^[a-z0-9_-]{1,64}$`, on every route except health. Demo users `user_a`, `user_b`, `user_c` | The brief puts authentication out of scope; a header sits where a verified token would |
| Retries | `Idempotency-Key` header (canonical UUID) on every POST, scoped per user and operation (payment or redemption). The same key and body returns the stored result with 200 and `Idempotent-Replayed: true`; the same key with another body is 409 | A timed-out request may have committed; the client must be able to ask again safely |
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
  "redemption_status": "AVAILABLE",
  "rules": { "rate_bps": 500, "min_payment": 20000, "daily_cap": 50000, "timezone": "Asia/Jakarta" }
}
```

The rules are served, not hard-coded in the app, so a different rate or cap needs no app release.

- `status` is about earning: `ACTIVE`, `PAUSED` (the award switch is off, D05), or `ENDED` (the budget is spent, D03).
  `ENDED` wins over `PAUSED`.
- `redemption_status` is about redeeming: `AVAILABLE` or `PAUSED` (the redemption switch is off, D05). It is
  independent of `status`; redemption still works after the campaign has ended.
- The day is a calendar day in `rules.timezone` (D04).
- This response may be served from a cache and be a few seconds old (D06). It is for display only; `POST /payments`
  always decides from the database.

Assumption: the second switch is a separate `redemption_status` field rather than another `status` value, because
the two switches are independent.

## GET /me/cashback

A user with no activity gets zeros, never a 404.

```json
{
  "balance": 15000,
  "today": {
    "date": "2026-10-03",
    "earned": 47000,
    "remaining": 3000,
    "resets_at": "2026-10-04T00:00:00+07:00"
  }
}
```

`today` is the current campaign day: a calendar day in WIB, by the database clock (D04). This response may be served
from a per-user cache that is cleared after each of the user's payments and redemptions commits (D06).

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

The award is decided in PostgreSQL inside the payment's transaction, so the response is final when it returns (D01).
The award is `min(5% rounded down, what is left of today's cap, what is left of the budget)` (D02), and the budget is
spent at that moment (D03). `reference` carries the campaign day in WIB and the zero-padded payment ID; IDs can have
gaps.

`cashback.reason` says why the award is what it is:

| Reason              | Award           | When                                                                 |
| ------------------- | --------------- | -------------------------------------------------------------------- |
| `AWARDED`           | the full 5%     | The full 5% fits today's cap and the budget                          |
| `PARTIAL_DAILY_CAP` | less than 5%    | Today's cap cut the award                                            |
| `PARTIAL_BUDGET`    | less than 5%    | The award equals what was left of the budget, so the campaign has ended, including a tie with what was left of the cap (D45) |
| `BELOW_MINIMUM`     | Rp0             | Amount under `min_payment`                                           |
| `CAMPAIGN_ENDED`    | Rp0             | The budget is spent                                                  |
| `CAMPAIGN_PAUSED`   | Rp0             | The award switch is off (D05)                                        |
| `DAILY_CAP_REACHED` | Rp0             | Nothing is left of today's cap                                       |

A Rp0 result has exactly one reason, checked in this order: `BELOW_MINIMUM`, `CAMPAIGN_ENDED`, `CAMPAIGN_PAUSED`,
`DAILY_CAP_REACHED`.

Cashback is awarded at once, because payments here are simulated and settle at once. A real integration awards on
settlement with a pending state; this simplification is stated in the README (D07, trust condition 18).

If the transaction waits too long for a lock, the response is 503 `SERVICE_BUSY`. Nothing is committed, and the app
retries with the same key (D01).

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

The budget is not involved: it was spent when the cashback was awarded (D03). Redemption works after the campaign has
ended. While the redemption switch is off, a new redemption is 409 `REDEMPTION_PAUSED` and nothing changes; a
redemption that already passed that check completes (D05). A replay of a key that was already accepted returns the
stored result, even while paused.

Assumption: checks run in this order: amount range, then the switch, then the balance.

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

The newest items only: `?limit=`, default 20, maximum 50. No paging (D08). Assumption: a `limit` outside 1 to
50 is 400 `MALFORMED_REQUEST`.

## GET /healthz

The full path is `/v1/healthz`, like every route under the base URL. No `X-User-ID`. 200 when the service can do its
job, 503 when it cannot. The body names each dependency:

```json
{ "status": "ok", "dependencies": { "postgres": "ok", "redis": "degraded" } }
```

PostgreSQL down is 503:

```json
{ "status": "unavailable", "dependencies": { "postgres": "down", "redis": "ok" } }
```

- PostgreSQL down: 503.
- Redis down or slow: 200 with Redis reported as `degraded`. Redis only holds read caches, and reads fall back to
  PostgreSQL (D06).

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
| `REDEMPTION_PAUSED` | 409 | A new redemption while the redemption switch is off |
| `NOT_FOUND`, `METHOD_NOT_ALLOWED` | 404, 405 | No such route, or the wrong method |
| `INTERNAL_ERROR` | 500 | Anything unexpected |
| `SERVICE_BUSY` | 503 | A lock wait timed out; nothing was committed; retry with the same key |

There is no rate limit (D06). A 4xx creates no row, so its idempotency key is not stored (D39).

## Operations

| Tool | What it does |
| --- | --- |
| Reconcile command | Checks the money invariants and exits non-zero on any mismatch. It reports; it never repairs |
| Demo state command | Puts the database into a known state for the three demo users. Refuses to run unless a demo flag is set |
| Budget settings | Configuration seeds the campaign row on first boot only, defaulting to Rp10.000.000, so the ended state can be reached with a small budget on a fresh database. After that the row is the truth: a restart or a changed setting never resets it |
| Switch commands | Pause and resume awards; pause and resume redemptions (D05). Each writes the flag on the campaign row and a structured log line (who, when, which switch) |

A load-test command, `make load-test`, measures the read caches on and off (D51); it is local tooling, not an endpoint.

## Library picks

Confirmed as one batch (D24–D37).

| Area | Pick | Reason |
| --- | --- | --- |
| HTTP router | `chi` | Standard `net/http` handlers, little to learn |
| PostgreSQL access | `pgx` v5, hand-written SQL | The locking statements are the design and should be visible |
| Migrations | `goose`, embedded, run when the API starts, with the Postgres session locker | One `docker compose up`, no separate step; two instances never migrate at once |
| Redis client | `go-redis` v9 | The common client |
| Logging | Standard library `log/slog`, JSON | No dependency |
| Go checks | `gofmt`, `go vet`, `staticcheck` | Real mistakes, almost no configuration |
| Integration tests | Real PostgreSQL and Redis from a test compose file | The tests depend on real locking |
| CI | GitHub Actions | The repository is on GitHub |
| Mobile | Expo with Expo Router, TypeScript | A reviewer runs it with one command |
| Server data in the app | TanStack Query | Loading, error, and refetch states without hand-written flags |
| Mobile tests | Jest with React Native Testing Library | Tests act as a user would |
| Key generation | `expo-crypto` on the phone, `google/uuid` in Go | `crypto.randomUUID` is not guaranteed in React Native |
| Remembered demo user | `@react-native-async-storage/async-storage` | The user switcher is remembered across launches |
| Mobile lint and typecheck | ESLint with `eslint-config-expo`; `tsc --noEmit` | The agent stop check needs `lint` and `typecheck` scripts |
| API base URL in the app | `EXPO_PUBLIC_API_URL`, default `http://localhost:8080/v1` | Expo Go on a phone needs the host's address (D09) |
