# Flash Cashback — Technical Specification

How the [PRD](prd.md) is built. Decisions are cited by ID from [DECISIONS.md](DECISIONS.md); request and response
shapes are in [api-contract.md](api-contract.md) and screens in [ui-wireframe.md](ui-wireframe.md), and are not
restated. Engine behaviour this design relies on is checked against [known-pitfalls.md](known-pitfalls.md) ("KP").

## 1. Architecture

```
 Expo app (host / phone) --HTTP--> api (Go, chi) --pgx--> PostgreSQL   (truth: every money read and write)
                                       |  \------go-redis--> Redis      (campaign read cache only, D06, D53)
 operator: docker compose exec api /app/admin | /app/reconcile  --> PostgreSQL (+ Redis key delete)
```

Package layout (`backend/`, one Go module):

| Path                          | Holds                                                                                   |
| ----------------------------- | --------------------------------------------------------------------------------------- |
| `cmd/api`                     | server; `api healthcheck` subcommand for the container healthcheck (KP: distroless)     |
| `cmd/admin`                   | `pause-awards`, `resume-awards`, `pause-redemptions`, `resume-redemptions` (stdout line, D47), `demo-reset` |
| `cmd/reconcile`               | runs `internal/reconcile`, prints, sets the exit code                                   |
| `internal/config`             | environment → typed config with defaults (§12)                                          |
| `internal/domain`             | pure: `Award`, `CampaignStatus`, `TodayRemaining`, `Reference`, amount and ID validation |
| `internal/store`              | pgx pool, `InTx` helper, repositories, SQLSTATE → typed errors                          |
| `internal/cache`              | read-cache get/set/delete, 50 ms deadline, fail open                                    |
| `internal/service`            | `Payments`, `Redemptions`, `Reads`, `Switches`, `Demo`                                  |
| `internal/httpapi`            | router, middleware (request ID, access log, recover), handlers, error mapper            |
| `internal/reconcile`          | the checks (§9), used by the command and by every concurrency test                      |
| `internal/boot`               | goose migrate (embedded, session locker, D26) then seed (C2)                            |
| `migrations/`                 | SQL files + `embed.go`                                                                  |
| `internal/integration`        | every DB/Redis test, one package, build tag `integration` (KP: `-p 1`)                  |

Endpoint → handler → service → repository (shapes: [contract](api-contract.md)):

| Endpoint              | Handler           | Service                   | Repository                                       |
| --------------------- | ----------------- | ------------------------- | ------------------------------------------------ |
| `GET /v1/campaign`    | `getCampaign`     | `Reads.Campaign`          | `campaigns.Get`                                  |
| `GET /v1/me/cashback` | `getCashback`     | `Reads.Cashback`          | `balances.Today` (one query)                     |
| `POST /v1/payments`   | `postPayment`     | `Payments.Pay`            | `campaigns`, `daily`, `payments`, `balances`, `ledger` |
| `POST /v1/redemptions`| `postRedemption`  | `Redemptions.Redeem`      | `campaigns`, `balances`, `redemptions`, `ledger` |
| `GET /v1/me/history`  | `getHistory`      | `Reads.History`           | `history.Page` (keyset, §3, D54)                 |
| `GET /v1/healthz`     | `getHealth`       | — (pings)                 | pool ping, Redis ping                            |

## 2. Data model

One migration `00001_init.sql`. All constraints are named, so a mutation test can drop one by name (KP: two-column
checks are otherwise `<table>_check`). There is one campaign row, id `flash-cashback`.

```sql
-- The one campaign-day function (D04). fc_now() is the only clock; tests may replace it in the test DB.
CREATE FUNCTION fc_now() RETURNS timestamptz LANGUAGE sql STABLE AS $$ SELECT now() $$;
CREATE FUNCTION fc_campaign_day(ts timestamptz) RETURNS date LANGUAGE sql STABLE
  AS $$ SELECT (ts AT TIME ZONE 'Asia/Jakarta')::date $$;
CREATE FUNCTION fc_day_resets_at(d date) RETURNS timestamptz LANGUAGE sql STABLE
  AS $$ SELECT (d + 1)::timestamp AT TIME ZONE 'Asia/Jakarta' $$;

CREATE TABLE campaigns (
  id                 text        PRIMARY KEY,
  name               text        NOT NULL,
  rate_bps           integer     NOT NULL CONSTRAINT campaigns_rate_range     CHECK (rate_bps BETWEEN 1 AND 10000),
  min_payment        bigint      NOT NULL CONSTRAINT campaigns_min_positive   CHECK (min_payment > 0),
  daily_cap          bigint      NOT NULL CONSTRAINT campaigns_cap_positive   CHECK (daily_cap > 0),
  budget             bigint      NOT NULL CONSTRAINT campaigns_budget_positive CHECK (budget > 0),
  spent              bigint      NOT NULL DEFAULT 0 CONSTRAINT campaigns_spent_nonneg CHECK (spent >= 0),
  awards_paused      boolean     NOT NULL DEFAULT false,
  redemptions_paused boolean     NOT NULL DEFAULT false,
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT campaigns_spent_within_budget CHECK (spent <= budget),               -- INV-02, TC1
  CONSTRAINT campaigns_min_earns CHECK (min_payment * rate_bps >= 10000)          -- a payment at the minimum earns ≥ Rp1
);

CREATE TABLE user_daily_earnings (
  campaign_id text   NOT NULL REFERENCES campaigns(id),
  user_id     text   NOT NULL,
  day         date   NOT NULL,
  earned      bigint NOT NULL DEFAULT 0 CONSTRAINT ude_earned_nonneg CHECK (earned >= 0),
  daily_cap   bigint NOT NULL CONSTRAINT ude_cap_positive CHECK (daily_cap > 0),  -- the day's cap, copied from the campaign row at creation
  PRIMARY KEY (campaign_id, user_id, day),
  CONSTRAINT ude_earned_within_cap CHECK (earned <= daily_cap)                    -- INV-03, TC2
);

CREATE TABLE payments (
  id               bigserial   PRIMARY KEY,
  campaign_id      text        NOT NULL REFERENCES campaigns(id),
  user_id          text        NOT NULL CONSTRAINT payments_user_format CHECK (user_id ~ '^[a-z0-9_-]{1,64}$'),
  idempotency_key  uuid        NOT NULL,
  request_hash     bytea       NOT NULL CONSTRAINT payments_hash_len CHECK (octet_length(request_hash) = 32),
  amount           bigint      NOT NULL CONSTRAINT payments_amount_range CHECK (amount BETWEEN 1 AND 10000000),
  status           text        NOT NULL CONSTRAINT payments_status CHECK (status = 'SUCCEEDED'),
  cashback_awarded bigint      NOT NULL CONSTRAINT payments_award_nonneg CHECK (cashback_awarded >= 0),
  cashback_reason  text        NOT NULL CONSTRAINT payments_reason_known CHECK (cashback_reason IN
                     ('AWARDED','PARTIAL_DAILY_CAP','PARTIAL_BUDGET','BELOW_MINIMUM','CAMPAIGN_ENDED',
                      'CAMPAIGN_PAUSED','DAILY_CAP_REACHED')),
  rate_bps         integer     NOT NULL,   -- rule snapshot (TC12)
  min_payment      bigint      NOT NULL,
  daily_cap        bigint      NOT NULL,
  campaign_day     date        NOT NULL,
  created_at       timestamptz NOT NULL,
  CONSTRAINT payments_user_key_unique UNIQUE (user_id, idempotency_key),          -- INV-05, TC3
  CONSTRAINT payments_award_within_rate CHECK (cashback_awarded * 10000 <= amount * rate_bps),
  CONSTRAINT payments_award_within_cap CHECK (cashback_awarded <= daily_cap),
  CONSTRAINT payments_reason_matches_award CHECK ((cashback_awarded > 0) =
                     (cashback_reason IN ('AWARDED','PARTIAL_DAILY_CAP','PARTIAL_BUDGET')))
);
CREATE INDEX payments_user_newest ON payments (user_id, created_at DESC, id DESC);

CREATE TABLE redemptions (
  id              bigserial   PRIMARY KEY,
  campaign_id     text        NOT NULL REFERENCES campaigns(id),
  user_id         text        NOT NULL CONSTRAINT redemptions_user_format CHECK (user_id ~ '^[a-z0-9_-]{1,64}$'),
  idempotency_key uuid        NOT NULL,
  request_hash    bytea       NOT NULL CONSTRAINT redemptions_hash_len CHECK (octet_length(request_hash) = 32),
  amount          bigint      NOT NULL CONSTRAINT redemptions_amount_range CHECK (amount BETWEEN 1 AND 10000000),
  status          text        NOT NULL CONSTRAINT redemptions_status CHECK (status IN ('PENDING','COMPLETED','FAILED')),
  destination     text        NOT NULL CONSTRAINT redemptions_destination CHECK (destination = 'MAIN_ACCOUNT'),
  balance_after   bigint      NOT NULL CONSTRAINT redemptions_balance_after_nonneg CHECK (balance_after >= 0),
  campaign_day    date        NOT NULL,
  created_at      timestamptz NOT NULL,
  CONSTRAINT redemptions_user_key_unique UNIQUE (user_id, idempotency_key)        -- INV-05, TC3
);
CREATE INDEX redemptions_user_newest ON redemptions (user_id, created_at DESC, id DESC);

CREATE TABLE cashback_balances (
  user_id    text        PRIMARY KEY CONSTRAINT balances_user_format CHECK (user_id ~ '^[a-z0-9_-]{1,64}$'),
  balance    bigint      NOT NULL CONSTRAINT balances_nonneg CHECK (balance >= 0),  -- INV-04, TC4
  updated_at timestamptz NOT NULL
);

CREATE TABLE ledger_entries (
  id            bigserial   PRIMARY KEY,
  user_id       text        NOT NULL,
  kind          text        NOT NULL,
  amount        bigint      NOT NULL,
  payment_id    bigint      CONSTRAINT ledger_payment_once UNIQUE REFERENCES payments(id),
  redemption_id bigint      CONSTRAINT ledger_redemption_once UNIQUE REFERENCES redemptions(id),
  balance_after bigint      NOT NULL CONSTRAINT ledger_balance_after_nonneg CHECK (balance_after >= 0),
  created_at    timestamptz NOT NULL,
  CONSTRAINT ledger_kind_shape CHECK (                                            -- IS NOT NULL in each branch (KP)
    (kind = 'AWARD' AND amount > 0 AND payment_id IS NOT NULL AND redemption_id IS NULL) OR
    (kind = 'REDEMPTION' AND amount < 0 AND redemption_id IS NOT NULL AND payment_id IS NULL))
);
CREATE INDEX ledger_user ON ledger_entries (user_id, id);

-- Append-only (D40, INV-06): row-level UPDATE/DELETE raise. TRUNCATE (tests, demo-reset) is not a row event.
CREATE FUNCTION fc_append_only() RETURNS trigger LANGUAGE plpgsql
  AS $$ BEGIN RAISE EXCEPTION '% is append-only', TG_TABLE_NAME; END $$;
CREATE TRIGGER ledger_append_only BEFORE UPDATE OR DELETE ON ledger_entries FOR EACH ROW EXECUTE FUNCTION fc_append_only();
CREATE TRIGGER payments_append_only BEFORE UPDATE OR DELETE ON payments FOR EACH ROW EXECUTE FUNCTION fc_append_only();
CREATE TRIGGER redemptions_append_only BEFORE UPDATE OR DELETE ON redemptions FOR EACH ROW EXECUTE FUNCTION fc_append_only();
```

The balance row holds the running total; the ledger is the history (D40). `campaigns.spent` is the running total of
awards (D03); the budget is never in a response (D18).

## 3. Domain rules (pure, `internal/domain`)

**Campaign day (D04).** Only `fc_campaign_day(fc_now())` in SQL decides a day; Go never derives a day from a clock. Go
formats instants in a fixed `+07:00` zone (WIB has no DST) for D19, with RFC 3339 at second precision.

| `fc_now()` (UTC)       | WIB                       | Campaign day | `resets_at`                 |
| ---------------------- | ------------------------- | ------------ | --------------------------- |
| 2026-10-03T16:59:59Z   | 2026-10-03T23:59:59+07:00 | 2026-10-03   | 2026-10-04T00:00:00+07:00   |
| 2026-10-03T17:00:00Z   | 2026-10-04T00:00:00+07:00 | 2026-10-04   | 2026-10-05T00:00:00+07:00   |
| 2026-10-03T00:00:00Z   | 2026-10-03T07:00:00+07:00 | 2026-10-03   | 2026-10-04T00:00:00+07:00   |

**Award (D02, D03, D12, D13).** `Award(amount, rule{rate, min, cap}, earnedToday, budget, spent, awardsPaused) →
(awarded, reason)`. `rate` and `min` come from the locked campaign row; `cap` comes from the user-day row, which copied
it from the campaign row when the row was created, so a cap changed by SQL mid-day applies from the next campaign day
and can never make `ude_earned_within_cap` fail (Assumption, within D07's "no live rule change"; AC-71). The payment
snapshots this same `cap`. Rp0 reasons in order: `BELOW_MINIMUM` (amount < min), `CAMPAIGN_ENDED` (spent ≥ budget),
`CAMPAIGN_PAUSED`, `DAILY_CAP_REACHED` (cap − earned ≤ 0). Otherwise `full = amount × rate / 10000` (integer, the amount
bound is checked first, KP), `award = min(full, cap − earned, budget − spent)`; reason `AWARDED` if `award = full`,
else `PARTIAL_BUDGET` if `award = budget − spent` (the campaign ends; this also decides the case where both limits cut,
including a tie with `cap − earned`, D45, contract reason table per C15), else `PARTIAL_DAILY_CAP`.

| Amount   | Earned | Budget left | Paused | Award | Reason              |
| -------- | ------ | ----------- | ------ | ----- | ------------------- |
| 19999    | 0      | 10000000    | no     | 0     | `BELOW_MINIMUM`     |
| 20000    | 0      | 10000000    | no     | 1000  | `AWARDED`           |
| 20001    | 0      | 10000000    | no     | 1000  | `AWARDED`           |
| 100000   | 45000  | 10000000    | no     | 5000  | `AWARDED`           |
| 100000   | 47000  | 10000000    | no     | 3000  | `PARTIAL_DAILY_CAP` |
| 100000   | 50000  | 10000000    | no     | 0     | `DAILY_CAP_REACHED` |
| 100000   | 0      | 5000        | no     | 5000  | `AWARDED` (ends)    |
| 100000   | 0      | 2000        | no     | 2000  | `PARTIAL_BUDGET`    |
| 100000   | 47000  | 2000        | no     | 2000  | `PARTIAL_BUDGET`    |
| 100000   | 48000  | 2000        | no     | 2000  | `PARTIAL_BUDGET` (tie) |
| 100000   | 48000  | 3000        | no     | 2000  | `PARTIAL_DAILY_CAP` |
| 100000   | 0      | 0           | yes    | 0     | `CAMPAIGN_ENDED`    |
| 100000   | 50000  | 10000000    | yes    | 0     | `CAMPAIGN_PAUSED`   |
| 19999    | 0      | 0           | yes    | 0     | `BELOW_MINIMUM`     |
| 10000000 | 0      | 10000000    | no     | 50000 | `PARTIAL_DAILY_CAP` |

**Status.** `CampaignStatus(spent, budget, awardsPaused)`: `ENDED` if spent ≥ budget, else `PAUSED` if paused, else
`ACTIVE`. `redemption_status` is `PAUSED` iff `redemptions_paused`. `TodayRemaining = max(cap − earned, 0)`, where
`cap` is today's user-day row cap if the row exists, else the campaign row's.

**Reference (C9).** `Reference("PAY"|"RDM", campaignDay, id)` → `PAY-20261003-000042`; the ID is zero-padded to 6
digits and grows past that (`1234567` → `PAY-20261003-1234567`). IDs have gaps (KP: sequences).

**History order and cursor (D54).** Order key `(created_at DESC, type_rank DESC, id DESC)`, `PAYMENT` = 1,
`REDEMPTION` = 0: a total order, since a payment and a redemption can share an `id`. The cursor is the key of the last
row of a page: `EncodeCursor(t, type, id)` = base64url without padding (`RawURLEncoding`) of `v1|<t>|<P|R>|<id>`, `t`
in UTC formatted `RFC3339Nano`. `timestamptz` holds microseconds and pgx scans them exactly, so the cursor round-trips
exactly; the response's `created_at` is second precision (D19) and can never serve as a cursor. `DecodeCursor` fails
(→ 400 `MALFORMED_REQUEST`) unless: the base64url decodes, there are four `|` fields, the version is `v1`, the type is
`P` or `R`, `id` is a positive int64, `t` parses, `t` has no sub-microsecond digits, and `EncodeCursor` of the parsed
values gives back the input exactly (one canonical form per position). Both are pure, in `internal/domain`.

The page query: two `UNION ALL` branches (payments with `1 AS type_rank`, redemptions with `0`), each `WHERE user_id =
$user AND (created_at, id) < ($t, $bound) ORDER BY created_at DESC, id DESC LIMIT $limit + 1` (KP: limit each branch;
both columns `DESC`, so the row comparison matches the `*_user_newest` index), then `ORDER BY created_at DESC,
type_rank DESC, id DESC LIMIT $limit + 1`. Without a cursor the branches have no `(created_at, id)` filter (a second
static query text). Go picks `$bound` per branch from the branch rank and the cursor rank:

| Cursor at (t, rank, id) | Branch rank | `$bound`                | Rows of this branch at `created_at = t`  |
| ----------------------- | ----------- | ----------------------- | ---------------------------------------- |
| (t, P=1, 42)            | P=1 (equal) | 42                      | `id < 42`: older payments at t           |
| (t, P=1, 42)            | R=0 (lower) | 9223372036854775807     | all of them (they sort after any payment) |
| (t, R=0, 3)             | R=0 (equal) | 3                       | `id < 3`                                 |
| (t, R=0, 3)             | P=1 (higher)| 0                       | none (they sorted before the cursor; ids are ≥ 1) |

Rows with `created_at < t` pass in every case. The `max int64` bound misses only a row whose id is `max int64`, which
a `bigserial` never reaches in practice. `limit + 1` rows back means a next page exists: the extra row is dropped and
`next_cursor` encodes the last kept row; fewer means `next_cursor` is null. Rows are always scoped by `$user` from
`X-User-ID`; the cursor is only a position (AC-47a).

## 4. Critical write paths

Isolation READ COMMITTED. Every money transaction runs on a context detached from the client (`context.WithoutCancel`)
with a 5 s cap, so a client disconnect does not decide the outcome (KP; AC-70); it starts with `SET LOCAL lock_timeout = '<n>ms'` and
`SET LOCAL statement_timeout = '<n>ms'`, formatted from validated integer config (no bind parameters, KP).

**Lock order (D44, D46).** Award (D44): `campaigns` row `FOR NO KEY UPDATE` → `user_daily_earnings` row →
`cashback_balances` row → new rows (`payments`, `ledger_entries`). Redemption (D46, superseding the D44 redemption
order): `cashback_balances` row `FOR UPDATE` → plain read of `campaigns`, no lock (D43) → new rows (`redemptions`,
`ledger_entries`); a user with no balance row takes no lock and reads the flag at once. Switch command: `campaigns`
only. The campaign row is never locked `FOR UPDATE`: the `redemptions` and `payments` inserts take `FOR KEY SHARE` on
it through the foreign key, which `FOR NO KEY UPDATE` and a non-key `UPDATE` do not block, but `FOR UPDATE` would (KP:
deadlock). So the only campaign-row lock a redemption takes is that `FOR KEY SHARE`, after its balance lock, and no
other path holds a mode that conflicts with it: a redemption never waits on the campaign row, and the award's
campaign → balance order and the redemption's balance → (key share) campaign order cannot deadlock.

### 4.1 `Payments.Pay` (D01, D02, D03, D04, D16, D39)

1. Handler validates headers and body (§7), computes `request_hash = sha256("amount=" + decimal amount)`.
2. Fast replay path, no transaction: select the payment by `(user_id, idempotency_key)`. Found: same hash → 200 replay
   built from the stored row; other hash → 409 `IDEMPOTENCY_KEY_REUSED`.
3. `BEGIN`; set timeouts.
4. `SELECT rate_bps, min_payment, daily_cap, budget, spent, awards_paused, fc_now(), fc_campaign_day(fc_now()) FROM
   campaigns WHERE id = $1 FOR NO KEY UPDATE`. All payments serialise here (D01 cost). Every value used in the decision
   comes from this locked read (KP).
5. Select the payment by key again. A concurrent duplicate that committed while this one waited is visible now (READ
   COMMITTED): `ROLLBACK`, then replay or 409 as in step 2.
6. `INSERT INTO user_daily_earnings (...) VALUES ($c, $u, $day, 0, $campaignCap) ON CONFLICT DO NOTHING`, then
   `SELECT earned, daily_cap ... FOR UPDATE` (KP: lock a row only after it exists). This row's `daily_cap` is the
   one `Award` uses (§3).
7. `domain.Award(...)`.
8. If `awarded > 0`, in the D44 order: `UPDATE campaigns SET spent = spent + $a, updated_at = $now RETURNING spent,
   budget`; `UPDATE user_daily_earnings SET earned = earned + $a`; `INSERT INTO cashback_balances ... ON CONFLICT
   (user_id) DO UPDATE SET balance = cashback_balances.balance + EXCLUDED.balance RETURNING balance` (the balance row
   lock is taken here, after the user-day row).
9. New rows: `INSERT INTO payments (... created_at = $now, campaign_day = $day ...) ON CONFLICT ON CONSTRAINT
   payments_user_key_unique DO NOTHING RETURNING id`. No row (KP: a conflict returns no row) → `ROLLBACK` (which also
   undoes step 8), replay. If `awarded > 0`: `INSERT INTO ledger_entries (AWARD, +a, payment_id, balance_after)`.
10. `COMMIT`; its error is checked (KP). After commit, on a fresh 50 ms context: if `spent = budget` after this award,
    delete `fc:v1:campaign`. One log line (§7). 201.

`created_at` and `campaign_day` come from the same `fc_now()` (transaction start), so a transaction that starts at
23:59:59 WIB and commits after midnight counts once, for the old day (TC6, KP).

### 4.2 `Redemptions.Redeem` (D03, D05, D16, D39, D43, D46)

1. Handler validates; amount range first (422), per the contract's check order.
2. Fast replay path as 4.1 step 2 (a replay is answered even while paused).
3. `BEGIN`; set timeouts.
4. `SELECT balance FROM cashback_balances WHERE user_id = $1 FOR UPDATE`; no row means balance 0 and no lock.
5. Select the redemption by key: found → `ROLLBACK`, replay or 409. This runs under the balance lock, so a concurrent
   duplicate is seen here instead of failing the balance check.
6. `SELECT redemptions_paused, fc_now(), fc_campaign_day(fc_now()) FROM campaigns WHERE id = $1`, a plain read with
   no lock (D43), after the balance lock (D46). Under READ COMMITTED it sees the flag as last committed when this
   statement starts, so any wait on the balance row is over before the check and a pause committed during that wait is
   seen. A pause that commits after this read does not stop this redemption, which completes (TC7 as amended);
   `pause-redemptions` does not wait for it (§4.3). `fc_now()` is still the transaction start (`created_at`, §4.1).
7. Paused → `ROLLBACK`, 409 `REDEMPTION_PAUSED` (before the balance check, per the contract's check order).
8. `amount > balance` → `ROLLBACK`, 422 `INSUFFICIENT_BALANCE`.
9. `UPDATE cashback_balances SET balance = balance - $a ... RETURNING balance` (= `balance_after`).
10. `INSERT INTO redemptions (status 'COMPLETED', destination 'MAIN_ACCOUNT', balance_after, ...) ON CONFLICT ...
    DO NOTHING RETURNING id`; no row → `ROLLBACK`, replay. `INSERT INTO ledger_entries (REDEMPTION, −a, ...)`.
11. The main-account payout is a stub that completes at once and makes no call (TC13, stated only).
12. `COMMIT`; log; 201. No cache key is deleted (D53).

### 4.3 Switch commands (D05, D23, D43, D44, D46, D47, TC7)

`admin pause-awards --by <operator>` (and the three others): `BEGIN; SET LOCAL lock_timeout = '5s'; SELECT
awards_paused ... FOR NO KEY UPDATE; UPDATE campaigns SET awards_paused = true, updated_at = now(); COMMIT`. The
`UPDATE` waits for in-flight awards (they hold `FOR NO KEY UPDATE`), and every award that locks the row after the
commit reads the new flag, so a pause never lands inside an award. It does not wait for redemptions, which hold no lock
on the row (D43): a redemption whose flag read (§4.2 step 6) ran before the commit completes; one whose read runs after
it gets 409, including one that was waiting on its balance row when the pause committed (D46). Accepted cost (D43,
D46): when `pause-redemptions` returns, a redemption that already read the flag may still commit. The window runs from
that flag read to its commit: a few milliseconds, with no lock wait left in it (the balance lock is already held and the
foreign-key `FOR KEY SHARE` conflicts with nothing the switch holds), so `LOCK_TIMEOUT` does not apply; the hard bound
is the 5 s transaction context (§4), which rolls the redemption back if it has not committed by then. The redemption
has its own API log line and is in reconcile. After commit the command prints one JSON line to its stdout
`{"event":"operator_action","switch":"awards","action":"pause","operator":…,"old":false,"new":true,"changed":true,
"at":…}`, then deletes `fc:v1:campaign`. The line reaches only the operator's terminal and is not stored: there is no
operator trail (D47, trust condition 11 stated only). Exit 0 done, 1 failure (including lock timeout: "busy, retry"),
2 usage (missing `--by`).

### 4.4 Failures inside a money transaction

| Failure                                                | Result                                 |
| ------------------------------------------------------ | -------------------------------------- |
| `55P03` lock timeout (the contract's case)             | rollback; 503 `SERVICE_BUSY`           |
| `57014` statement timeout, or the 5 s transaction context's deadline (`context.DeadlineExceeded`) on any statement before `COMMIT` | rollback; 503 `SERVICE_BUSY` (Assumption: same meaning as the contract's code, nothing committed, retry with the same key) |
| `40P01` deadlock (should not happen by the D44/D46 order) | rollback; 503 `SERVICE_BUSY`, logged `error` (same Assumption) |
| `23514` check, unexpected `23505`, append-only raise   | rollback; 500 `INTERNAL_ERROR`, logged `error` as `invariant_violation` (KP) |
| connection lost, any error or deadline at `COMMIT`     | 500 `INTERNAL_ERROR`; outcome may be committed, so never 503; the app retries with the same key |
| Redis error after commit                               | logged `warn`; response unchanged      |

## 5. Idempotency (D16, D39, TC3)

- **Life:** the app creates one key per attempt (§10). The server stores it on the `payments` or `redemptions` row it
  creates, in the same transaction; it never expires. A 4xx creates no row, so the key stays unused.
- **Scope:** per table, so per (user, operation): the same UUID may be a payment for user_a, a payment for user_b, and a
  redemption for user_a.
- **Replay:** the stored row rebuilds the response exactly, including `balance_after` (KP); status 200 with
  `Idempotent-Replayed: true`.
- **Mismatch:** same key, different hash → 409, nothing written.
- **Concurrent duplicates:** payments serialise on the campaign row and see the winner at step 5; redemptions serialise
  on the balance row and see it at step 5 (a user with no balance row has nothing to redeem: every duplicate is 422 or
  409 and writes nothing). The unique constraint with `ON CONFLICT DO NOTHING` is the last guard. A
  duplicate that waits past `lock_timeout` gets 503 and retries.

## 6. Caches (D06, D53, TC8, TC9)

| Key                        | Value                     | TTL                                         | Deleted after                                    |
| -------------------------- | ------------------------- | ------------------------------------------- | ------------------------------------------------ |
| `fc:v1:campaign`           | `GET /campaign` body      | `CACHE_CAMPAIGN_TTL` 5 s                    | a switch command; an award that makes spent = budget; `demo-reset` |

- Read: GET with a 50 ms deadline → hit: parse and validate, a bad value is a miss (KP) → miss: read PostgreSQL,
  respond, then SET with a 50 ms deadline. Errors and partial results are never cached. `GET /me/cashback` and
  `GET /me/history` are not cached; they always read PostgreSQL (D53).
- Client: `ContextTimeoutEnabled: true` (KP), dial/read/write/pool timeouts 50 ms, `MaxRetries: -1` (in go-redis v9,
  0 means the default of 3 retries; -1 disables them), so one slow call costs at most one 50 ms deadline. Any Redis error or
  timeout is a miss; failures log `warn` at most once per 10 s per operation.
- Deletes happen after commit only, on a detached context (KP). If a delete fails, the TTL bounds the staleness. A
  COMMIT whose outcome is unknown (§4.4) deletes nothing: whether that award exhausted the budget is unknown, and the
  5 s TTL bounds the campaign key.
- No money path reads the cache; the write services do not hold a cache client for reads (INV-11).
- `CACHE_READS` (D51, kept by D52): `on` by default; `off` makes `Reads` skip the campaign cache GET and SET and
  answer from PostgreSQL (AC-76). Deletes after commit run in both modes. It lets an operator serve reads without the cache while
  Redis stays up; it is not a Redis health switch (a down Redis is already a miss).
- Known window: a reader that read PostgreSQL before a commit can SET the old value after the delete; it lives until
  the TTL (KP). Accepted by D06.

## 7. Errors and logging (D20, D21, D22, D28)

- Validation, at the handler, in this order: `X-User-ID` (`^[a-z0-9_-]{1,64}$`); `Idempotency-Key` on POST (36
  characters, 8-4-4-4-12 hex, then `uuid.Parse`, KP); body ≤ 1 KiB, a JSON object with exactly the key `amount`
  (case-sensitive, no duplicates, no other keys) whose raw token is a number (else 400 `MALFORMED_REQUEST`); the token
  must be plain digits with value 1..10000000 (else 422 `INVALID_AMOUNT`). The raw token is inspected, not decoded into
  `any` (KP). `limit` on history: digits, 1..50; `cursor` on history: absent, or `DecodeCursor` succeeds (§3; an empty value fails), else
  400 `MALFORMED_REQUEST`.
- One error mapper turns typed errors into the D21 envelope and the contract's codes; 500 always has a fixed message.
  chi's 404/405 handlers use the same mapper. A recover middleware turns a panic into 500.
- Request ID: `X-Request-ID` echoed if it matches `^[A-Za-z0-9._-]{1,128}$`, else a new UUID; on every response and
  log line.
- Log lines (slog JSON): one access line per request (`request_id`, method, route pattern, status, `duration_ms`,
  `user_id`); one line per money write (`op`, `user_id`, `payment_id`/`redemption_id`, `amount`, `awarded`, `reason`,
  `replayed`). The admin command prints its own line to the operator's terminal (§4.3); it is not part of the API's
  logs and is not kept (D47). Never bodies, SQL, or whole driver errors.

## 8. Server and runtime

- `http.Server` on `HTTP_ADDR` (`0.0.0.0:8080` in compose, KP) with read-header 5 s, read 10 s, write 15 s, idle 60 s;
  on SIGTERM it stops accepting and drains for up to 8 s so in-flight transactions finish (KP). 8 s covers the 5 s
  transaction cap and stays under Docker's default 10 s stop grace period, so compose needs no `stop_grace_period`.
- Start: connect PostgreSQL (retry up to 30 s), goose up with the Postgres session locker (D26), seed with `INSERT INTO
  campaigns (...) VALUES ('flash-cashback', 'Flash Cashback', 500, 20000, 50000, $CAMPAIGN_BUDGET) ON CONFLICT (id) DO
  NOTHING` (C2, TC10; the rules come from the brief, only the budget is configurable), create the Redis client (Redis
  being down is not fatal), serve.
- `GET /v1/healthz` (contract "GET /healthz" gives the 200 and 503 bodies, C14, C16): PostgreSQL `SELECT 1` (1 s) and
  Redis `PING` (50 ms). PostgreSQL failing → 503; Redis down or slow → `degraded`, never 503 (D06). The container
  healthcheck passes on 200.
- Compose: `postgres` (pinned major, TCP healthcheck `pg_isready -h 127.0.0.1` with the real user and database, KP),
  `redis` (pinned major, `redis-cli ping`), `api` (build `backend/`, multi-stage `CGO_ENABLED=0`, distroless runtime with
  `/app/api`, `/app/admin`, `/app/reconcile`; healthcheck `/app/api healthcheck`; `depends_on` with `service_healthy`).
  Only 8080 is published (KP). Every variable has a default; no `.env` needed.

| Variable              | Default                                                       | Used by            |
| --------------------- | ------------------------------------------------------------- | ------------------ |
| `HTTP_ADDR`           | `:8080`                                                       | api                |
| `DATABASE_URL`        | `postgres://flash:flash@postgres:5432/flash?sslmode=disable`  | all                |
| `DB_MAX_CONNS`        | `20`                                                          | api                |
| `REDIS_ADDR`          | `redis:6379`                                                  | api, admin         |
| `REDIS_TIMEOUT`       | `50ms`                                                        | api, admin         |
| `CACHE_CAMPAIGN_TTL`  | `5s`                                                          | api                |
| `CACHE_READS`         | `on` (`on` or `off`; anything else rejected at start)         | api                |
| `LOCK_TIMEOUT`        | `2s`                                                          | api                |
| `STATEMENT_TIMEOUT`   | `5s`                                                          | api                |
| `CAMPAIGN_BUDGET`     | `10000000` (read at the first seed only)                      | api                |
| `LOG_LEVEL`           | `info`                                                        | all                |
| `FC_DEMO`             | unset (`demo-reset` refuses unless `1`)                       | admin              |

## 9. Operations

- **Switches:** §4.3; run as `docker compose exec api /app/admin pause-awards --by riyan`.
- **Reconcile (D42, TC5, TC19, TC20):** read-only, one `REPEATABLE READ READ ONLY` transaction so every check sees one
  snapshot. One JSON line per check, then a summary; exit 0 all pass, 1 any fail, 2 cannot run. Checks:
  1. per user, `sum(ledger.amount) = balance` (users with ledger rows but no balance row count as 0) — INV-01;
  2. `spent ≤ budget` and `spent = sum(payments.cashback_awarded) = sum(AWARD ledger)` — INV-02;
  3. per (user, day), `earned = sum(awarded that day)` and `earned ≤ daily_cap` — INV-03;
  4. no negative balance or `balance_after` — INV-04;
  5. no duplicate `(user_id, idempotency_key)` in either table — INV-05;
  6. every awarded payment and every redemption has exactly one ledger entry of the matching amount; no orphan entry —
     INV-06;
  7. every payment's award ≤ floor(amount × rate / 10000) and reason/amount agree — INV-07;
  8. `campaign_day = fc_campaign_day(created_at)` on both tables — INV-08;
  9. per user, ledger `balance_after` equals the running sum by `id`, and each redemption's `balance_after` equals its
     ledger entry's — INV-09;
  10. report only: outstanding liability = sum of balances; budget and spent (operator output, not a response).
- **Demo reset (contract "Demo state command"):** refuses unless `FC_DEMO=1` (exit 2). Truncates the five money tables
  `RESTART IDENTITY`, sets `spent = 0` and both switches off, keeps the budget, then builds state through the services:
  user_a pays 940000 (47000) and redeems 32000; user_c pays 1000000 (50000); user_b nothing. Deletes the cache keys.
  This is the one, demo-only exception to INV-06 (append-only; `TRUNCATE` fires no row trigger) and to trust condition
  10 (spent goes back to 0): it refuses without `FC_DEMO=1`, which compose does not set, and is never run in
  production (Assumption).

## 10. Mobile app (`mobile/`, D32–D37)

| Path                                   | Holds                                                                                     |
| -------------------------------------- | ----------------------------------------------------------------------------------------- |
| `app/_layout.tsx`                      | `QueryClientProvider`, `UserProvider`, `AttemptProvider`, stack                           |
| `app/index.tsx` … `app/how-it-works.tsx` | screens 1–7: `index`, `pay`, `payment-result`, `checking`, `redeem`, `history`, `how-it-works` |
| `src/api/client.ts`                    | base URL `EXPO_PUBLIC_API_URL` (default `http://localhost:8080/v1`, D09), headers, 10 s `AbortController` timeout (KP), outcome classification |
| `src/api/queries.ts`                   | query keys `['campaign']`, `['cashback', user]`, Home `['history', user, 5]`, History `['history', user, 'pages']` (D54) |
| `src/money/format.ts`                  | hand-written `Rp100.000` formatter and digit parser (KP)                                   |
| `src/copy/payInfo.ts`                  | the one client estimate and info-line variant (pure)                                      |
| `src/copy/codes.ts`                    | reason, status, and error code → copy, with the fallback (D20)                            |
| `src/user/`                            | demo user in AsyncStorage (D36)                                                           |
| `src/attempts/`                        | the money-attempt state machine below; the saved-attempt store and the launch check (D48) |

Value homes: campaign, balance, today, and history live only in the TanStack Query cache (D33), refetched on focus and
pull (a focus refetch skips a query that is fetching or was updated in the last 2 s: after a money answer the
invalidation has just refetched it); the amount input lives in the screen; the current attempt (kind, user, amount,
key, last response) lives in `AttemptProvider`, held in a ref so a refetch or re-render cannot touch it (KP), and is
mirrored in AsyncStorage under `fc:attempts` as a list of `{user_id, kind, amount, key, created_at}` keyed by `key`
(D48), so a kill loses nothing.
Each request of an attempt sends the attempt's saved `user_id` as `X-User-ID`, never the selected demo user. Day labels
come from the date part of `created_at`, never `new Date` (KP).

Money attempt (payment or redemption), one per press:

```
idle --press (ref guard, new key from expo-crypto)--> saving
saving --stored in fc:attempts--> sending      saving --storage error--> rejected (generic error; nothing sent)
sending --2xx--> done: show result / confirmation; invalidate campaign, cashback, ['history', user]; remove saved attempt
sending --4xx--> rejected: show its copy (INSUFFICIENT_BALANCE: refetch cashback first;
                 REDEMPTION_PAUSED: refetch campaign); remove saved attempt; next press is a new attempt
sending --timeout | network | 5xx--> checking (screen 4, back blocked): resend same key x3, ~2 s apart
checking --2xx--> done      checking --4xx--> rejected (back on the originating screen)
checking --3 unknowns--> waiting: "Check again" --> checking (same key, as often as pressed)
```

`done` carries `replayed` (from the `Idempotent-Replayed` header) next to the body; the Redeem confirmation shows
`balance_after` only when `replayed` is false (AC-65a/b).

Launch check (D48, wireframe screen 4), before Home renders its data, per saved attempt, oldest first:

```
age < 10 min  --> checking (same key, saved user, saved amount), as above
age >= 10 min --> unconfirmed card on Home: "Check now" --> checking (same key, saved user)
                                            "Dismiss"   --> remove saved attempt; show the history hint; send nothing
```

The age is the device clock minus the saved `created_at`; a negative age (clock moved back) counts as recent
(Assumption). Apart from Dismiss, only a definite answer removes an attempt: 2xx or 4xx; a timeout, network error, or
5xx keeps it.

GET queries retry once; the attempt machine is the only retry for POSTs.

History paging (D54, AC-66c, AC-66d): Home reads `limit=5` under `['history', user, 5]`. The History screen is a
`SectionList` fed by `useInfiniteQuery` under `['history', user, 'pages']`: `initialPageParam` none, `limit=20`,
`getNextPageParam` returns `next_cursor`, `undefined` when null. The pages are flattened, then grouped by day (from
`created_at`, as above), so a day spanning two pages gets one header. `onEndReached` calls `fetchNextPage` only when
`hasNextPage && !isFetchingNextPage && !isFetchNextPageError`; the footer shows a spinner while `isFetchingNextPage`,
and "Couldn't load more." with Try again (`fetchNextPage`, same cursor) on `isFetchNextPageError`, leaving loaded pages
in place. A money write invalidates the prefix `['history', user]`, so both keys refetch; the infinite query refetches
its loaded pages in order from the first, with fresh cursors.

## 11. Testing strategy

| Layer        | Tests                                                                                                       |
| ------------ | ----------------------------------------------------------------------------------------------------------- |
| domain       | table tests: §3 award table, every reason, status, remaining, reference, amount and key validation; history cursor round-trip (µs `t`, both types, large id) and every rejected form, and the §3 bound table (D54) |
| handler      | `httptest` through the real router with service fakes: every 4xx code, envelope, request ID, header order, raw-token amount cases, no budget keys (AC-49) |
| integration  | real PostgreSQL and Redis from the test compose file on uncommon host ports (D30); `-tags integration -p 1 -count=1`; tables truncated between tests and the campaign row reseeded, with `budget` set to the remaining budget the test needs and `spent` 0 (so reconcile's `spent = sum(awards)` holds); earned, balances, and every other state built only through the services (KP) |
| clock        | tests replace `fc_now()` in the test database to a fixed instant and restore it in cleanup (AC-13)          |
| cache        | Redis stopped (client at a closed port), Redis stalled (`CLIENT PAUSE 500 ALL`), a bad cached value, freshness after writes |
| mobile       | Jest + RNTL (D34): fetch and AsyncStorage stubbed; every screen state; the retry path asserts the same key is resent; the double press; day grouping with `TZ=UTC`; D48: the attempt is stored before fetch is called and removed on 2xx/4xx only (AC-72), "killed during checking → relaunch → same key" by remounting the app over the saved store with another user selected and asserting key and `X-User-ID` (AC-73), an 11-minute-old attempt sends nothing until Check now (AC-74) |
| system       | CI compose smoke: `up --wait`, healthz, one payment by curl, reconcile exits 0                              |

Concurrency tests run under `make test-race` (`-race -count=20`). Each uses a start barrier, a pool larger than the
callers, collects errors and asserts in the test goroutine, rolls back any lock holder in cleanup (KP), asserts errors
and totals separately (KP), and ends with the reconcile checks (INV-01–09). Mutations are applied to the code or the
live test schema (never to the applied migration, KP), shown red, and reverted; each is reported in the task.

| Test                                    | AC     | Mutation that must turn it red                                                            |
| --------------------------------------- | ------ | ----------------------------------------------------------------------------------------- |
| many users drain the budget             | AC-25  | campaign read without `FOR NO KEY UPDATE`; separately, drop `campaigns_spent_within_budget` with the lock removed |
| one user, concurrent payments, cap      | AC-26  | remove the campaign and user-day locks; separately, also drop `ude_earned_within_cap`     |
| same key, concurrent payments           | AC-23, AC-24 | drop `payments_user_key_unique` and the in-lock lookup (step 5)                       |
| same key, concurrent redemptions        | AC-69  | drop `redemptions_user_key_unique` and the in-lock lookup (step 5); balance 18000 ≥ 10 × 1000, so the extra debits succeed and the totals go red |
| client disconnect mid-transaction       | AC-70  | run the transaction on the request context instead of `context.WithoutCancel` (the cancel rolls it back; the resend is 201, not 200) |
| two redemptions of one balance          | AC-33  | balance read without `FOR UPDATE`; separately, also drop `balances_nonneg`, `ledger_balance_after_nonneg` and `redemptions_balance_after_nonneg` (all three guard INV-04; the overspend commits only with all three gone) |
| awards and redemptions mixed            | AC-28  | award locks the campaign row `FOR UPDATE` instead of `FOR NO KEY UPDATE` (the redemption's foreign-key `FOR KEY SHARE` then waits on it while holding the balance; expect `40P01`) |
| pause during awards (TC7, award case)   | AC-38  | campaign read in `Pay` without `FOR NO KEY UPDATE` (an award that read `false` commits after the pause, so the id order breaks) |
| pause during redemptions (TC7, redemption case) | AC-39 | remove the paused check in `Redeem` (a redemption begun after the command returns is 201); separately, add `FOR SHARE` to the step-6 read (the held case: the command no longer returns while the redemption is open); separately, move the flag read back before the balance lock, the D44 order (the waiting case: the redemption that waited on its balance row through the pause is 201) |
| lock timeout                            | AC-27  | remove `SET LOCAL lock_timeout` (the request waits for the 2 s hold and then gets 201, or a later 503 from another timeout; either misses "503 within 1 s") |
| cap changed mid-day                     | AC-71  | `Award` uses the campaign row's cap (earned passes 50000, the CHECK fails, 500)          |
| in flight across midnight               | AC-14  | write `created_at` from `clock_timestamp()`                                               |

Test hooks (nil in production): AC-38 widens the gap between the campaign read and the payment insert; AC-39 holds a
redemption open between its flag read and its write. The AC-39 waiting case needs no hook: a test transaction holds the
user's balance row `FOR UPDATE`, the redemption is started and the test polls `pg_stat_activity` until that backend
shows `wait_event_type = 'Lock'`, then runs the pause, then rolls the holder back (test `LOCK_TIMEOUT` above the hold).
The poll makes it deterministic: under the mutation the flag was read before the wait, so it is 201 every run. AC-70
uses the same poll: once the payment's backend waits on the campaign lock, the test cancels the HTTP client's context,
then rolls the holder back after 500 ms and waits for the row or the 5 s cap.
"Began after the command returned" means a row whose `created_at`
(transaction start, §4) is later than `clock_timestamp()` read from the database right after the command exits.
Reconcile (INV-01–09) runs after both.

History paging (AC-47a) is an integration test, not a race. The fixture is built through the services, stepping
`fc_now()` one second per item, with the AC's two equal-time pairs pinned at the page boundaries: items 20/21 (payment
then redemption, same instant, same `id`, aligned by `setval` on the two sequences, test-only SQL; page 1's cursor is
on the payment, so the redemption branch takes the `max int64` bound) and items 39/40 (same instant, payment `id`
lower; page 2's cursor is on the redemption, so the payment branch takes bound 0). Before the mid-walk payment the test
sets `fc_now()` later than every item. Mutations that must turn it red: pass the cursor id as `$bound` in both branches
(item 21 is skipped; item 39 repeats on page 3); drop `type_rank` from the outer `ORDER BY` (items 39/40 swap, since
the redemption's `id` is higher); drop `user_id` from one branch (the cross-user case). The test also runs `EXPLAIN`
on the cursor query, with `SET LOCAL enable_seqscan = off` because 45 rows would otherwise plan as a sequential scan,
and asserts each branch is an Index Scan on its `*_user_newest` index with no Sort node below the
branch `LIMIT` (KP).

## 12. Where it breaks

| Limit                                                                 | At higher scale                                                       |
| --------------------------------------------------------------------- | --------------------------------------------------------------------- |
| Every payment, including Rp0 ones, serialises on one campaign row (D01) | Redis counter as a fast gate or budget buckets (D01 revisit); skip the lock for `BELOW_MINIMUM` |
| After `pause-redemptions` returns, a redemption that already read the flag can still commit, in the milliseconds from its flag read to its commit, at most the 5 s transaction cap (D43, D46); no hard freeze | Redemption flag on its own row, locked by each redemption (D43 option C) |
| No operator trail: a pause or resume leaves no lasting record; its line reaches only the operator's terminal (D47, TC11 stated only) | An `operator_actions` row written in the same transaction as the flag change |
| `demo-reset` truncates the money tables and resets spent; only `FC_DEMO=1` guards it (INV-06, TC10 exception) | Not shipped in a production image; the command removed from the build |
| A saved attempt lives on one device; a reinstall or a cleared app store loses it (D48) | A server lookup of a payment by key, then the app checks status first (D48 revisit) |
| Waiting transactions hold pool connections; 20 waiters starve reads   | Separate pools for reads and writes; admission limit                  |
| No rate limit; `X-User-ID` is trusted (TC14–16)                       | Verified identity, then per-user rate limits (D06 revisit)            |
| A cache can be stale up to its TTL after a racing read (§6)           | Versioned keys or write-through after commit                          |
| Midnight rush: a full cap before and after 00:00 WIB (D04)            | Rolling window or abuse signals                                       |
| Payout is a stub (TC13); awards on unsettled payments (TC18)          | Outbox, pending status, settlement events                             |
| One campaign, rules fixed at seed (TC12)                              | Campaign versions with effective-from times                           |
| History orders by `created_at`, the transaction start (§4.1): a row that commits late (up to the 5 s cap) can land behind a cursor the app already passed; it shows on the next refresh, never twice (D54) | A sequence taken just before `COMMIT` narrows the window but does not close it; an order assigned after commit (a visibility watermark) closes it |
| A History refetch after a money write reloads every loaded page in sequence (§10, D54) | `maxPages`, or refetch the first page only and merge             |

## Assumptions

- Assumption: every payment, Rp0 ones included, takes the same path and the campaign lock (simplest; one path to test).
- Assumption: tests control the clock by replacing `fc_now()` in the test database; production never replaces it.
- Assumption: the request hash is SHA-256 of the canonical parsed amount, so whitespace or key order never mismatches.
- Assumption: a money transaction runs on a detached context capped at 5 s; `STATEMENT_TIMEOUT` 5 s, `LOCK_TIMEOUT` 2 s.
- Assumption: Go formats WIB with a fixed `+07:00` zone, so the image needs no tzdata.
- Assumption: switch commands take `--by` as the operator name, print it in their stdout line, and wait up to 5 s for
  the lock.
- Assumption: `demo-reset` truncates and resets `spent` to 0 while keeping the budget; it exists only behind `FC_DEMO=1`,
  the one exception to INV-06 and trust condition 10.
- Assumption: `Award` uses the user-day row's cap, so a cap change applies from the next campaign day (§3, AC-71).
- Assumption: 503 `SERVICE_BUSY` also covers `57014`, `40P01`, and the 5 s transaction deadline before `COMMIT`; an
  error at `COMMIT` is 500 (§4.4).
- Assumption: the API drains for 8 s on SIGTERM, under Docker's default 10 s stop grace period.
- Assumption: saved attempts are a list keyed by key; a failed save sends nothing; an attempt under 10 minutes old
  (the wireframe's Assumption), or with a negative age, is resent at launch.
- Assumption: the app's request timeout is 10 s; GET queries retry once; a 4xx during Checking returns to the
  originating screen with its copy.
- Assumption: the D54 cursor is `v1|<created_at>|<P|R>|<id>` in base64url, with one canonical form; a `v1` prefix
  lets a later format be told apart. The order and per-branch `LIMIT` are in §3 (KP).
- Assumption: PostgreSQL and Redis image majors and the Go version are pinned at P0 to the current stable releases.
