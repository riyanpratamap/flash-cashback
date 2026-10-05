# Design Overview

What has to be true before this touches real money, the rules as interpreted, and the decisions, rejected options,
scope, and limits behind them. The trust conditions table is copied from [DECISIONS.md](DECISIONS.md) (D07); "Where
it breaks" is copied from [tech-spec.md](tech-spec.md) §12. Each decision's full reasoning is in DECISIONS.md.

## What has to be true before this touches real money

The rows marked **built** are this project's definition of production ready. Rows marked **stated only** or **out** are
real gaps (D07): "this cannot go live with real money until identity, the main account transfer and settlement are
connected."

| #   | Condition that must hold                                                | How it is guaranteed                                                                                                                                            | How it is proved                                                         | Status                                                                   |
| --- | ----------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------ | ------------------------------------------------------------------------ |
| 1   | The budget is never overspent                                           | Campaign row lock and `CHECK (spent <= budget)` in the payment transaction (D01)                                                                               | Many users draining the last of the budget at once, a mutation, reconcile | built                                                                    |
| 2   | The daily cap is never exceeded                                         | User-day row lock and `CHECK (earned <= cap)` (D01, D04)                                                                                                       | Concurrent payments by one user, a mutation, reconcile                    | built                                                                    |
| 3   | A retry never pays or redeems twice                                     | `UNIQUE (user, operation, key)` and a stored request hash, in the same transaction (D16, D39)                                                                   | The same key sent concurrently, a mutation                                | built                                                                    |
| 4   | No cashback balance is negative                                         | `CHECK (balance >= 0)` and a balance row lock                                                                                                                   | Two concurrent redemptions of the same balance, a mutation                | built                                                                    |
| 5   | Per user, the sum of ledger entries equals the balance                  | Append-only ledger written in the same transaction (D40)                                                                                                        | Reconcile after every concurrency test                                    | built                                                                    |
| 6   | A payment across midnight counts once, for one day                      | PostgreSQL `now()` at transaction start (D04)                                                                                                                   | Boundary test at 23:59:59 and 00:00:00 WIB                                | built                                                                    |
| 7   | A pause cannot land in the middle of an award. A redemption either sees the pause, or passed the check before it and completes (amended by D43) | Award flag on the locked campaign row (D05); redemption flag read without a lock (D43)                                                                          | A concurrent pause test for each case                                     | built                                                                    |
| 8   | Redis down or slow never affects money                                  | Money reads PostgreSQL only; the cache falls back to PostgreSQL within about 50 ms (D06)                                                                        | Tests with Redis stopped and with Redis delayed                           | built                                                                    |
| 9   | A stale cache never misleads a money decision                           | Cache deleted after commit, short TTL; no write path reads the cache (D06)                                                                                     | Pay, then the balance read is fresh; redemption checks PostgreSQL         | built                                                                    |
| 10  | A restart or redeploy never resets the budget                           | The budget lives in the campaign row; the seed is `ON CONFLICT DO NOTHING`; configuration is read only at the first seed                                       | Pay, restart, budget unchanged                                            | built                                                                    |
| 11  | Every operator action is traceable (amended by D47)                     | Not guaranteed: the admin command prints its line to the operator's terminal only. Production needs a row written in the same transaction as the flag change | —                                                                        | stated only: no lasting record (D47)                                     |
| 12  | A rule changed while the campaign runs does not rewrite history         | Each payment stores the `rate_bps`, `min_payment`, and `daily_cap` it used; no live rule-change command                                                         | History unchanged after a rule change                                     | built (snapshot) / live rule change out                                  |
| 13  | A redemption reaches the main account exactly once                      | The redemption debits the cashback balance and stores its key; the main-account transfer is a stub that completes at once. Production needs a transactional outbox, a pending status, and the key passed to the main-account system | Stub test only                                                            | stated only: the main account system does not exist here                 |
| 14  | Abuse by one user is bounded                                            | The daily cap; no rate limit (D06)                                                                                                                              | Cap tests (row 2)                                                         | built (cap) / rate limit out                                             |
| 15  | Abuse by many accounts is bounded                                       | Needs verified identity and KYC                                                                                                                                 | —                                                                        | stated only: authentication is out of scope (brief)                      |
| 16  | Identity cannot be set by anyone                                        | `X-User-ID` is trusted, as the brief allows; production takes the user from a verified token                                                                    | —                                                                        | stated only                                                              |
| 17  | A refund of a payment whose cashback is already redeemed is handled     | Needs a clawback policy, which conflicts with row 4                                                                                                             | —                                                                        | out: refunds and clawback are out of scope (brief)                       |
| 18  | Cashback is earned only on settled payments                             | Payments here are simulated and settle at once; production awards on settlement, with a pending state                                                           | —                                                                        | stated only                                                              |
| 19  | Balances that are never redeemed are known                              | Reconcile reports the outstanding liability; no expiry (D03)                                                                                                    | Reconcile output                                                          | built (report) / expiry out                                              |
| 20  | Problems are seen                                                       | JSON logs with `request_id`; `/healthz` reports degraded; reconcile exits non-zero on any broken invariant                                                      | Health check and reconcile tests                                          | built / alert routing stated only                                        |
| 21  | The app never turns an unknown outcome into a second payment, even if it is killed while checking (amended by D48) | One idempotency key per attempt, reused on every retry, never shown as failed, back blocked while checking; the attempt (user, kind, amount, key, time) is saved before sending and cleared on a definite answer (D48) | App tests for the retry path, the double tap, and killed during checking → relaunch → same key | built |
| 22  | Budget figures never leave the server                                   | The response types have no budget field (D18)                                                                                                                   | A test on the API responses                                               | built                                                                    |

## Rules as interpreted

| Brief rule | Interpretation | Ref |
| --- | --- | --- |
| 5% cashback | `floor(amount x 500 / 10000)` in integer math, rounded down, never above 5% | D11, D12, AC-04 |
| Under Rp20.000 earns nothing | Exactly Rp20.000 earns (Rp1.000); Rp19.999 earns Rp0 `BELOW_MINIMUM` | D13, AC-02, AC-03 |
| Rp50.000 per day | Award = `min(5%, cap left today, budget left)`; a partial award is paid, not refused | D02, AC-06 |
| What "day" means | Calendar day in Asia/Jakarta (WIB), decided by the database clock at transaction start | D04, AC-13, AC-14 |
| Budget Rp10.000.000, then over | The budget is spent when cashback is awarded, not redeemed; unredeemed cashback still counts | D03, AC-08 |
| Budget tail | The award that takes the last of the budget is paid and ends the campaign (`PARTIAL_BUDGET` if cut) | AC-08, AC-09 |
| Both limits cut | `PARTIAL_BUDGET` only when the award equals what is left of the budget (including a tie); otherwise `PARTIAL_DAILY_CAP` | D45, AC-10 |
| Rp0 reasons | One reason, checked in order: `BELOW_MINIMUM`, `CAMPAIGN_ENDED`, `CAMPAIGN_PAUSED`, `DAILY_CAP_REACHED` | D02, AC-11, AC-12 |
| Payment vs cashback | A valid payment always succeeds; cashback is a bonus and can be Rp0 | D17 |
| Amount range | Whole number 1 to 10.000.000 for payments and redemptions; otherwise 422 `INVALID_AMOUNT` | D14 |
| Redeem | Any whole amount up to the balance, no minimum, also after the campaign ended; above the balance is 422 | D03, AC-30, AC-31, AC-34 |
| Kill switches | Two: awards and redemptions. A paused redemption is 409; one that already passed the check completes | D05, D43, AC-36, AC-37 |
| Retries | `Idempotency-Key` UUID on every POST, per (user, operation); same body replays with 200, other body is 409 | D16, D39 |
| Identity | `X-User-ID` is trusted (no authentication, per the brief) | D15 |
| Budget figures | Never in a response | D18 |

## Decisions that matter

| ID | Decision |
| --- | --- |
| D01 | Cap and budget are enforced in PostgreSQL inside the payment transaction. Cost: awards queue on one campaign row, and a lock timeout is a retryable 503 `SERVICE_BUSY` |
| D02 | Partial award when the full 5% does not fit, so the budget can reach zero and the banner never lies. Copy says "up to 5%" |
| D03 | Budget spent at award: the cashback balance is ours, redemption moves money the user already owns |
| D04 | Calendar day in WIB by the database clock; accepted: a full cap just before and after 00:00 WIB |
| D05 | Two switches, awards and redemptions, stored on the campaign row and operated by CLI |
| D06, D53 | Redis caches `GET /campaign` only (about 5 s); no money decision reads it; when Redis is down or slow, reads fall back to PostgreSQL (fail open). `GET /me/cashback` always reads PostgreSQL |
| D08 | Cut: `ENDING_SOON`, cursor paging, payment detail sheet, load test. Kept: read cache, CI |
| D09 | Demo on the iOS simulator or Expo Go on a phone, with the API address in `EXPO_PUBLIC_API_URL`; no web build, no CORS |
| D16, D39 | A retryable request is idempotent by a key stored with the row it creates, in the same transaction |
| D43, D46 | A redemption reads the redemption flag with no lock, after its balance lock, so it never queues behind awards. Cost: one redemption that already read the flag may commit milliseconds after a pause |
| D44 | One lock order: award `campaigns` -> `user_daily_earnings` -> `cashback_balances` -> new rows; redemption `cashback_balances` -> plain flag read -> new rows (D46) |
| D48 | The app saves an in-flight attempt (user, kind, amount, key, time) before sending and resumes with the same key after a relaunch; an old attempt is shown to the user, not resent silently |

## Rejected options

| Option | Why not |
| --- | --- |
| Redis counter for cap and budget (D01) | The campaign is small (about 10.000 awards at the smallest), so correctness matters more than speed; the database stays the single record |
| Award nothing when 5% does not fit (D02) | The budget would never reach zero and users would see a banner but earn nothing, which hurts trust |
| Spend the budget at redemption (D03) | There are two balances; "gone" means awarded into the cashback balance |
| Rolling 24 hours, UTC day, device time zone, API server clock (D04) | A WIB calendar day by the database clock matches users' expectation and the copy |
| One switch for everything (D05) | Two switches give more control over a running campaign |
| Rate limiting (D06) | The user ID is a header anyone can change, a per-IP limit blocks honest users on shared mobile IPs, and a limiter must never refuse a real payment. The daily cap bounds one account |
| Expo web build (D09) | Needs a web build and CORS on the server just for the demo |
| `operator_actions` table (D47) | Not asked by the brief; trust condition 11 is stated only. Chosen against the recommendation (a log line in the API's log stream) |
| Redemption flag on its own locked row (D43 option C) | Not needed unless operations needs a hard freeze |
| Cache `GET /me/cashback` (D53) | Gain not measurable; the shared campaign cache is the one that helps every user. Chosen against the recommendation |
| Load test (D51, dropped by D52) | Built with k6 in P4.5, then dropped: a tool outside the stack, and "the numbers don't prove anything". The one-off measurement was Apple M2, k6 and the stack sharing 8 CPUs, 4000 reads/s, 30 s per scenario: read p50 about 0.25 ms with the cache on and off, no measurable gain, no payment 503. It cannot be reproduced; the limits are proved by the concurrency tests |

## Out of scope

| Item | Reason |
| --- | --- |
| Refunds and clawback (TC17) | Out of scope in the brief |
| Authentication, verified identity (TC16), many-account abuse and KYC (TC15) | Out of scope in the brief; stated only |
| Products, catalogue, stock | Out of scope in the brief; a payment is just an amount |
| Real main-account transfer, outbox, pending payout (TC13) | Stated only: the main account system does not exist here |
| Awarding on settlement with a pending state (TC18) | Stated only: payments are simulated and settle at once |
| Rate limiting (TC14) | Out: header identity makes it easy to avoid (D06) |
| Operator action trail (TC11) | Stated only (D47): the admin command prints a line to the operator's terminal; nothing is kept |
| Live rule-change command (TC12) | Out: rules are snapshotted per payment; no live change |
| Cashback expiry (TC19) | Out: liability is reported, not expired (D03) |
| Alert routing (TC20) | Stated only: no paging system here |
| `ENDING_SOON`, history paging, payment detail sheet | Dropped by the cut line (D08) |
| Web build of the app, CORS | Not needed for the chosen demo path (D09) |

## Where it breaks

| Limit | At higher scale |
| --- | --- |
| Every payment, including Rp0 ones, serialises on one campaign row (D01) | Redis counter as a fast gate or budget buckets (D01 revisit); skip the lock for `BELOW_MINIMUM` |
| After `pause-redemptions` returns, a redemption that already read the flag can still commit, in the milliseconds from its flag read to its commit, at most the 5 s transaction cap (D43, D46); no hard freeze | Redemption flag on its own row, locked by each redemption (D43 option C) |
| No operator trail: a pause or resume leaves no lasting record; its line reaches only the operator's terminal (D47, TC11 stated only) | An `operator_actions` row written in the same transaction as the flag change |
| `demo-reset` truncates the money tables and resets spent; only `FC_DEMO=1` guards it (INV-06, TC10 exception) | Not shipped in a production image; the command removed from the build |
| A saved attempt lives on one device; a reinstall or a cleared app store loses it (D48) | A server lookup of a payment by key, then the app checks status first (D48 revisit) |
| Waiting transactions hold pool connections; 20 waiters starve reads | Separate pools for reads and writes; admission limit |
| No rate limit; `X-User-ID` is trusted (TC14-16) | Verified identity, then per-user rate limits (D06 revisit) |
| A cache can be stale up to its TTL after a racing read (§6) | Versioned keys or write-through after commit |
| Midnight rush: a full cap before and after 00:00 WIB (D04) | Rolling window or abuse signals |
| Payout is a stub (TC13); awards on unsettled payments (TC18) | Outbox, pending status, settlement events |
| One campaign, rules fixed at seed (TC12) | Campaign versions with effective-from times |
| History shows the newest 50 at most (D08) | Cursor paging |
