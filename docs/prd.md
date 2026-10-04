# Flash Cashback — Product Requirements

What the feature must do and why. Requirements come from [challenge-brief.md](challenge-brief.md); every choice is a
decision in [DECISIONS.md](DECISIONS.md) and is cited by ID (`D02`, trust condition `TC7`). Request and response shapes
are in [api-contract.md](api-contract.md); screens and copy are in [ui-wireframe.md](ui-wireframe.md). Neither is
restated here. The design is in [tech-spec.md](tech-spec.md).

## Problem

A flash cashback campaign pays 5% back on payments of Rp20.000 or more, at most Rp50.000 per user per day, from a fixed
Rp10.000.000 budget, and users can redeem what they earned. The happy path is easy; the work is making sure that, under
retries, concurrency, partial failures, and operator action, the money adds up exactly and nothing is paid twice.

## Users

- **Payer:** a user of the app who pays, earns cashback, sees balance and progress, and redeems. Identified by
  `X-User-ID` (D15); authentication is out of scope (brief).
- **Operator:** runs the switch, reconcile, and demo commands on the server (D23).
- **Reviewer:** runs the stack with one command and walks through the app (D09).

## Goals

1. Every payment gets the right cashback by the brief's rules, as interpreted by D02, D03, D04, D12, D13.
2. The budget is never overspent, no user exceeds the daily cap, no balance goes negative, and no retry pays or redeems
   twice, under any concurrency (TC1–TC5).
3. The user always sees a truthful state: never a failed load shown as zero, never an unknown outcome shown as failed
   (TC21).
4. Operators can stop awards and redemptions independently and prove the books balance (D05, D42).
5. Redis can fail without affecting money (D06, TC8).

## Out of scope and non-goals

| Item                                                     | Reason                                                     |
| -------------------------------------------------------- | ---------------------------------------------------------- |
| Refunds and clawback (TC17)                              | Out of scope in the brief                                  |
| Authentication; verified identity (TC16); many-account abuse and KYC (TC15) | Out of scope in the brief; stated only     |
| Products, catalogue, stock                               | Out of scope in the brief; a payment is just an amount     |
| Real main-account transfer, outbox, pending payout (TC13) | Stated only: the main account system does not exist here   |
| Awarding on settlement with a pending state (TC18)       | Stated only: payments are simulated and settle at once     |
| Rate limiting (TC14)                                     | Out: header identity makes it easy to avoid (D06)          |
| Operator action trail (TC11)                             | Stated only (D47): the admin command prints a line to the operator's terminal; nothing is kept |
| Live rule-change command (TC12)                          | Out: rules are snapshotted per payment; no live change     |
| Cashback expiry (TC19)                                   | Out: liability is reported, not expired (D03)              |
| Alert routing (TC20)                                     | Stated only: no paging system here                         |
| `ENDING_SOON`, history paging, payment detail sheet, load-test command | Dropped by the cut line (D08)              |
| Web build of the app, CORS                               | Not needed for the chosen demo path (D09)                  |

## User stories

- **US-1** As a payer, I pay an amount and immediately see the payment succeeded and what cashback it earned and why.
- **US-2** As a payer, I see my cashback balance, what I earned today, what is left, and when the day resets.
- **US-3** As a payer, I redeem any part of my balance to my main account, even after the campaign has ended.
- **US-4** As a payer whose request timed out, I am never charged twice and never told it failed when it may not have,
  even if the app is killed while it is checking.
- **US-5** As a payer, I see my recent payments (including Rp0 ones) and redemptions, newest first.
- **US-6** As a payer, I can read how the campaign works, with the numbers the server serves.
- **US-7** As an operator, I pause and resume awards and redemptions separately, and see on my terminal what each command changed.
- **US-8** As an operator, I run one command that proves the books balance and reports the outstanding liability.

## Acceptance criteria

Defaults unless stated: rules `rate_bps` 500, `min_payment` 20000, `daily_cap` 50000 (brief); campaign ACTIVE,
redemptions AVAILABLE; each POST carries a new key. "Budget left N" means the campaign row is reseeded before the test
with budget N and spent 0; "earned" and "balance" states are built through real payments (and redemptions).

### Award rules (D02, D03, D12, D13, D17, D45)

- **AC-01** Given user_a has no rows and budget left ≥ 5000, when user_a pays 100000, then 201, payment `SUCCEEDED`, cashback 5000 `AWARDED`; `GET /me/cashback` shows balance 5000, earned 5000, remaining 45000; one `AWARD` ledger entry of +5000; spent rises by 5000.
- **AC-02** Given user_a, when paying 19999, then 201 with cashback 0 `BELOW_MINIMUM`; balance, earned, spent unchanged; no ledger entry; the payment appears in history.
- **AC-03** When paying exactly 20000, then cashback 1000 `AWARDED`.
- **AC-04** When paying 20001, then 1000 (1000.05 rounded down); 39999 gives 1999; 10000000 on a fresh day gives 50000 `PARTIAL_DAILY_CAP` (5% is 500000).
- **AC-05** Given earned today 45000, when paying 100000, then 5000 `AWARDED`, earned 50000, remaining 0.
- **AC-06** Given earned today 47000, when paying 100000, then 3000 `PARTIAL_DAILY_CAP`, earned 50000.
- **AC-07** Given earned today 50000, when paying 100000, then 0 `DAILY_CAP_REACHED`; paying 19999 gives `BELOW_MINIMUM`.
- **AC-08** Given budget left 5000 and cap left 50000, when paying 100000, then 5000 `AWARDED`, spent equals budget, and `GET /campaign` shows `ENDED`.
- **AC-09** Given budget left 2000 and cap left 50000, when paying 100000, then 2000 `PARTIAL_BUDGET` and the campaign is `ENDED`.
- **AC-10** Both short, paying 100000: cap left 3000 and budget left 2000 gives 2000 `PARTIAL_BUDGET`; cap left 2000 and budget left 2000 (a tie) gives 2000 `PARTIAL_BUDGET` and the campaign is `ENDED`; cap left 2000 and budget left 3000 gives 2000 `PARTIAL_DAILY_CAP` and the campaign stays `ACTIVE` (D45, contract reason table per C15: `PARTIAL_BUDGET` only when the award equals the budget left, ties included).
- **AC-11** Given spent equals budget, when paying 100000, then 201 with 0 `CAMPAIGN_ENDED`; paying 19999 gives `BELOW_MINIMUM`.
- **AC-12** Given awards paused and budget left, when paying 100000, then 0 `CAMPAIGN_PAUSED`; paused with earned 50000 gives `CAMPAIGN_PAUSED`; paused with spent equal to budget gives `CAMPAIGN_ENDED` (D02 order).

### Campaign day (D04, TC6)

- **AC-13** Given the database clock at 2026-10-03T23:59:59+07:00 and user_c earned 50000 that day, when user_c pays 100000, then `DAILY_CAP_REACHED` and reference `PAY-20261003-…`; at 2026-10-04T00:00:00+07:00 the same payment earns 5000 `AWARDED`, and `GET /me/cashback` shows date 2026-10-04 and `resets_at` 2026-10-05T00:00:00+07:00.
- **AC-14** Given another transaction holds the campaign row, when a payment's transaction starts, waits 1 s, then commits, then its `created_at` equals its transaction start (before the release), and its campaign day and reference date are the day of that `created_at`.
- **AC-15** Given a payment made under the seed rules, when the rules on the campaign row are changed (test-only SQL), then that payment's stored rate, minimum, cap, award, and reason are unchanged in history (TC12).
- **AC-71** Given user_a earned 47000 today under cap 50000, when the campaign cap is changed to 60000 (test-only SQL),
  then paying 100000 is 201 with 3000 `PARTIAL_DAILY_CAP` (no 500) and `GET /me/cashback` shows remaining 0; after the
  clock moves to the next campaign day, paying 100000 earns 5000 and remaining is 55000 (Assumption: a cap change
  applies from the next campaign day).
- **AC-16** Given user_new has no rows, when reading `GET /me/cashback` and `GET /me/history`, then 200 with balance 0, earned 0, remaining 50000, today's WIB date and next 00:00+07:00; history has no items; redeeming 1 is 422 `INSUFFICIENT_BALANCE`.

### Validation (D14, D15, D16)

- **AC-17** For both POSTs: amounts 0, -1, 10000001, 1.5, 100000.0, 1e5 are 422 `INVALID_AMOUNT`; `"100000"`, `null`, `true`, `{}`, a missing `amount`, an unknown field, a non-JSON or empty body are 400 `MALFORMED_REQUEST`; no row is written.
- **AC-18** Missing `X-User-ID` is 400 `MISSING_USER`; `User_A`, an empty value, 65 characters, `a b` are 400 `INVALID_USER`; a POST without `Idempotency-Key` is 400 `MISSING_IDEMPOTENCY_KEY`; `abc`, a braced UUID, `urn:uuid:…`, 32 hex digits without dashes are 400 `INVALID_IDEMPOTENCY_KEY`. User is checked before key, key before body.

### Idempotency (D16, D39, TC3)

- **AC-19** Given a payment of 100000 returned 201 for key K, when the same user resends K with 100000 (also after awards were paused or the campaign ended), then 200 with `Idempotent-Replayed: true`, a body equal to the original, and no new rows.
- **AC-20** Given K was used for 100000, when resent with 50000, then 409 `IDEMPOTENCY_KEY_REUSED` and nothing changes.
- **AC-21** Given user_a used K for a payment, when user_b pays with K, or user_a redeems with K, then each is a new 201.
- **AC-22** Given K got 422 for amount 0, when K is sent with 100000, then 201 (a 4xx stores no key).
- **AC-23** When 20 requests for 100000 with the same key are sent at once, then exactly one 201 and nineteen 200 replays with equal bodies; one payment row; one ledger entry; balance up by 5000.
- **AC-24** When 10 requests with key K for 100000 and 10 with K for 50000 are sent at once, then one 201, the same-body requests are 200 replays, the others 409; one row.
- **AC-69** Given user_a has balance 18000, when 10 redemptions of 1000 with the same key are sent at once, then exactly
  one 201 and nine 200 replays, all with equal bodies and `balance_after` 17000; one redemption row; one `REDEMPTION`
  ledger entry; balance 17000.

### Concurrency (D01, TC1, TC2)

- **AC-25** Given budget left 12000, when 50 different users each pay 100000 at once, then exactly two 5000 `AWARDED`, one 2000 `PARTIAL_BUDGET`, 47 `CAMPAIGN_ENDED`; spent equals budget; no 5xx.
- **AC-26** Given user_a earned 0 today, when user_a sends 30 payments of 120000 at once, then eight 6000 `AWARDED`, one 2000 `PARTIAL_DAILY_CAP`, 21 `DAILY_CAP_REACHED`; earned 50000.
- **AC-27** Given the lock timeout is 300 ms and another transaction holds the campaign row for 2 s, when user_a pays,
  then 503 `SERVICE_BUSY` arrives within 1 s of the request, no payment row, balance and spent unchanged; after release,
  the same key gives 201 (not a replay).
- **AC-70** Given another transaction holds the campaign row for 500 ms (lock timeout above that), when user_a's payment
  of 100000 is waiting on it and the client cancels the request, then after the release the payment commits (one
  payment row, balance +5000); resending the same key gives 200 replayed with that payment's body; one row.
- **AC-28** When one user sends 20 payments of 100000 and 10 redemptions of 1000 at once, then no deadlock (`40P01`), no 5xx, and every invariant holds.

### Redemption (D03, D05, TC4)

- **AC-29** Given balance 18000, when redeeming 18000, then 201 `COMPLETED` to `MAIN_ACCOUNT`, `balance_after` 0, one `REDEMPTION` ledger entry of −18000, balance 0, spent unchanged.
- **AC-30** Given balance 18000, when redeeming 1, then 201 with `balance_after` 17999.
- **AC-31** Given balance 18000, when redeeming 18001, then 422 `INSUFFICIENT_BALANCE` and nothing changes.
- **AC-32** Given redemptions paused, redeeming 0 is 422 `INVALID_AMOUNT`; with balance 0, redeeming 1 is 409 `REDEMPTION_PAUSED` (range, then switch, then balance).
- **AC-33** Given balance 18000, when two redemptions of 18000 run at once, then one 201 and one 422 `INSUFFICIENT_BALANCE`; balance 0, never negative.
- **AC-34** Given the campaign is `ENDED` and balance 18000, when redeeming 18000, then 201.
- **AC-35** Given a redemption of 18000 returned `balance_after` 0 and the user then earned 5000, when the key is replayed, then 200 replayed with `balance_after` 0; a replay while redemptions are paused also returns the stored 200; a different amount is 409.

### Switches (D05, D47, TC7)

- **AC-36** When `pause-awards` runs, then the next `GET /campaign` shows `PAUSED`, payments earn 0 `CAMPAIGN_PAUSED`, redemptions are unaffected; after `resume-awards`, `ACTIVE`.
- **AC-37** When `pause-redemptions` runs, then `redemption_status` is `PAUSED`, a new redemption is 409 `REDEMPTION_PAUSED` with nothing changed, payments still award; after `resume-redemptions`, a redemption is 201.
- **AC-38** When 40 payments of 100000 by different users run and `pause-awards` lands among them, then, ordered by payment id, the reasons are a run with no `CAMPAIGN_PAUSED` followed by a run of only `CAMPAIGN_PAUSED`; every payment whose transaction began after the command returned is `CAMPAIGN_PAUSED`; invariants hold (TC7, award case).
- **AC-39** Given a redemption of 1000 has read `redemptions_paused` false and is held open, when `pause-redemptions` runs, then the command returns while that redemption is still open (D43); the held redemption then commits 201; a redemption started after the command returned is 409 `REDEMPTION_PAUSED`. Given another transaction holds user_a's balance row (balance 5000) and a redemption of 1000 by user_a is waiting on that lock, when `pause-redemptions` runs and then the holder rolls back, then the waiting redemption is 409 `REDEMPTION_PAUSED` and user_a's balance stays 5000 (D46: the flag is read after the balance lock). When 40 users with balance 5000 each redeem 1000 at once and `pause-redemptions` lands among them, then each response is 201 or 409 `REDEMPTION_PAUSED`, every 409 changed nothing, no 201 redemption began its transaction after the command returned, and invariants hold (TC7, redemption case).
- **AC-40** Each switch command prints exactly one JSON line to its stdout after its commit with the switch, the action,
  the operator from `--by`, the old and new value, and the time; a pause of a paused switch exits 0 and prints `changed`
  false; without `--by` it exits 2, changes nothing, and prints no action line. The line is not stored anywhere (D47:
  no operator trail is claimed).

### Reads and caches (D06, D08, D18, TC8, TC9, TC22)

- **AC-41** Given the seeded row, `GET /campaign` serves rate 500 bps, minimum 20000, and daily cap 50000 from the
  campaign row, and its two statuses follow this table (spent built by real payments against a reseeded budget):

  | spent vs budget | awards paused | redemptions paused | status   | redemption status |
  | --------------- | ------------- | ------------------ | -------- | ----------------- |
  | below           | no            | no                 | `ACTIVE` | `AVAILABLE`       |
  | below           | yes           | no                 | `PAUSED` | `AVAILABLE`       |
  | equal           | no            | no                 | `ENDED`  | `AVAILABLE`       |
  | equal           | yes           | yes                | `ENDED`  | `PAUSED`          |
  | below           | no            | yes                | `ACTIVE` | `PAUSED`          |
- **AC-42** Given `GET /me/cashback` was just served (cache warm) with balance 0, when user_a pays 100000 then reads again, then balance 5000; after redeeming 1000, 4000. After `pause-awards`, the next `GET /campaign` shows `PAUSED`.
- **AC-43** Given Redis is stopped, then both reads return correct values from PostgreSQL, payments and redemptions return the same results as with Redis up, and `GET /v1/healthz` is 200 with Redis `degraded`.
- **AC-44** Given Redis stalls for 500 ms, then a read waits at most about 50 ms on Redis and answers from PostgreSQL; payments are unaffected.
- **AC-45** Given the cache holds invalid JSON for a key, then the read treats it as a miss and answers correctly; given it holds balance 999999 for user_a whose real balance is 18000, a redemption of 20000 is 422.
- **AC-46** Given the database clock at 23:59:30 WIB, when `GET /me/cashback` populates the cache, then that entry expires no later than 00:00:00 WIB.
- **AC-47** `GET /me/history` lists payments (Rp0 included) and redemptions newest first, 20 by default, up to `limit` 50; `limit` 0, 51, or `abc` is 400 `MALFORMED_REQUEST`.
- **AC-48** Given user_a has payments and a balance, then user_b's balance, today, and history show none of it, and user_b cannot redeem user_a's balance.
- **AC-49** No response of any endpoint, in any campaign state, contains a budget or spent figure; `GET /campaign` with budget left 2000 equals the one with 9000000 left.

### Operations and runtime (D23, D26, D42, C2, TC10, TC19, TC20)

- **AC-50** `GET /v1/healthz` (body per the contract, "GET /healthz") needs no `X-User-ID`. Both up: 200, `status` ok,
  postgres ok, redis ok. Redis stopped: 200, `status` ok, redis `degraded`. PostgreSQL stopped: 503, `status`
  `unavailable`, postgres `down`.
- **AC-51** Given data built by real requests, reconcile exits 0 and prints each check and the outstanding liability (sum of balances); given a balance changed by hand in SQL, it exits 1 and names the failing check; it never changes data.
- **AC-52** Given a payment awarded 5000, when the API restarts with `CAMPAIGN_BUDGET=1`, then budget and spent are unchanged.
- **AC-53** Given an empty database, when the API starts, then migrations run and one campaign row exists with budget `CAMPAIGN_BUDGET` (default 10000000), spent 0, both switches off; two API processes starting together migrate once and seed one row.
- **AC-54** An unknown route is 404 `NOT_FOUND`, a wrong method 405 `METHOD_NOT_ALLOWED`, a panic 500 `INTERNAL_ERROR` with a fixed message; every error uses the D21 envelope, `request_id` equals the `X-Request-ID` response header, a sent `X-Request-ID` is echoed, and no SQL or stack trace appears.
- **AC-55** Given PostgreSQL is down, then `POST /payments` is 500 `INTERNAL_ERROR` with nothing committed and a `GET` that misses the cache is 500.
- **AC-56** From a clean clone with no `.env`, `docker compose up -d --build --wait` succeeds, only port 8080 is published, and `curl -fsS localhost:8080/v1/healthz` succeeds.
- **AC-57** The demo command refuses without the demo flag (exit 2, nothing changed); with it, user_a has balance 15000 and earned today 47000, user_b has no rows, user_c has earned 50000; reconcile then exits 0.

### Mobile app (D09, D32–D37, TC21; copy and states per the wireframe)

- **AC-58** Given user_a on Home, when paying Rp100.000, then the Payment result shows +Rp5.000 with the `AWARDED` text; Done returns to Home, which refetches balance and activity.
- **AC-59** When Pay is pressed twice in the same frame, then exactly one request is sent.
- **AC-60** Given the payment request times out, then Checking opens, resends the same key three times about 2 s apart, then Check again resends the same key; a 2xx opens the result; "failed" never appears; leaving the screen is blocked. Same for a redemption.
- **AC-61** Given a 4xx on Pay, then the inline or generic error shows and the next press uses a new key.
- **AC-72** When Pay Rp100.000 is pressed as user_a, then the attempt (user_a, payment, 100000, key K, created time) is
  in AsyncStorage before the request is sent (the stubbed fetch finds it at call time); a 2xx or a 4xx removes it; a
  timeout, network error, or 5xx keeps it. Same for a redemption (D48).
- **AC-73** Given a saved payment attempt by user_a with key K, 9 minutes old, and user_b selected, when the app
  launches, then Checking opens without a press and resends with key K, amount 100000, and `X-User-ID` user_a; a 2xx
  shows the result, clears the attempt, and leaves user_b selected (D48, the "killed during checking → relaunch → same
  key" test).
- **AC-74** Given a saved attempt 10 minutes old or older (11 minutes in the test), when the app launches, then no
  request is sent and Home shows the unconfirmed-attempt card with the amount and the attempt's time per the wireframe
  (screen 4); Check now opens Checking and resends with the saved key and user; Dismiss removes the attempt, shows
  "Check your history before paying again.", and sends nothing.
- **AC-62** The Payment result shows the amount, chip, and text of each reason in the wireframe table, and the fallback for an unknown code.
- **AC-63** The Pay info line shows each wireframe variant; with today's remaining 3000 and amount Rp100.000, the estimate is Rp3.000.
- **AC-64** Home shows loading, a load error with no zero values, the `ACTIVE`, `PAUSED`, and `ENDED` banners, the redemption-paused card, empty activity, and Redeem disabled at balance 0.
- **AC-65** On Redeem, `INSUFFICIENT_BALANCE` refetches the balance then shows the limit; `REDEMPTION_PAUSED` refetches the campaign and shows the paused state; success shows the confirmation with `balance_after`, then Done returns Home.
- **AC-66** With the device in UTC and an item at 2026-10-04T00:30:00+07:00, History groups it under 4 Oct; it shows at most 20 rows and the reason on partial and Rp0 payments.
- **AC-67** The chosen demo user sets `X-User-ID` on every request and is remembered after the app restarts.
- **AC-68** The app calls `EXPO_PUBLIC_API_URL` when set, else `http://localhost:8080/v1`.

## Invariants

Asserted by reconcile (D42) after every concurrency test, and by the reconcile command.

| ID     | Statement                                                                                    | Guarded by (constraint · lock)                                                    | Proved by                    |
| ------ | -------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------- | ---------------------------- |
| INV-01 | Per user, the sum of ledger amounts equals the balance                                       | Ledger written in the money transaction · balance row lock                        | AC-28, AC-33, reconcile      |
| INV-02 | Spent never exceeds the budget, and equals the sum of awarded cashback                       | `campaigns_spent_within_budget` · campaign row `FOR NO KEY UPDATE`                | AC-25, AC-08–10              |
| INV-03 | Per user and campaign day, earned ≤ that day's cap, and equals that day's awarded sum        | `ude_earned_within_cap` · user-day row `FOR UPDATE`                               | AC-26, AC-05–07, AC-71       |
| INV-04 | No balance and no ledger `balance_after` is negative                                         | `balances_nonneg`, `ledger_balance_after_nonneg` · balance row `FOR UPDATE`       | AC-33, AC-31                 |
| INV-05 | Per user and operation, one key produces at most one payment and at most one redemption      | `payments_user_key_unique`, `redemptions_user_key_unique`                          | AC-23, AC-24, AC-69, AC-70   |
| INV-06 | Each award and each redemption has exactly one ledger entry; ledger, payment, and redemption rows are never updated or deleted (one exception: `demo-reset`, behind `FC_DEMO=1`, truncates them; never in production, AC-57) | Unique `payment_id`, `redemption_id` · append-only triggers | AC-23, AC-69, reconcile |
| INV-07 | A payment's award is ≤ floor(amount × rate_bps / 10000) under its stored rules, and is 0 exactly for the Rp0 reasons | `payments_award_within_rate`, `payments_reason_matches_award` | AC-01–12, AC-15 |
| INV-08 | Every payment and redemption's campaign day is the WIB day of its `created_at`               | Both written from one `fc_now()` value                                            | AC-13, AC-14                 |
| INV-09 | A redemption's stored `balance_after` equals its ledger entry's                              | Written in the same transaction from one `RETURNING` value                        | AC-35, reconcile             |
| INV-10 | No response contains a budget figure                                                         | Response types have no such field (D18)                                           | AC-49                        |
| INV-11 | No money decision reads Redis                                                                | Write services have no cache dependency (D06)                                     | AC-43, AC-45                 |

## Trust conditions marked built → ACs

| TC | ACs | TC | ACs |
| -- | --- | -- | --- |
| 1 budget | AC-08–10, AC-25, INV-02 | 10 restart (demo reset excepted, AC-57) | AC-52, AC-53 |
| 2 daily cap | AC-05–07, AC-26, AC-71, INV-03 | 12 rules snapshot | AC-15, AC-71, INV-07 |
| 3 no double pay | AC-19–24, AC-35, AC-69, AC-70, INV-05 | 14 one-user abuse (cap) | AC-26 |
| 4 no negative | AC-31, AC-33, INV-04 | 19 liability report | AC-51 |
| 5 ledger = balance | AC-28, AC-51, INV-01 | 20 problems seen | AC-50, AC-51, AC-54 |
| 6 midnight | AC-13, AC-14, INV-08 | 21 app never double pays, even if killed (D48) | AC-59–61, AC-72–74 |
| 7 pause vs in-flight award / redemption (D43, D46) | AC-38, AC-39 | 22 no budget figures | AC-49, INV-10 |
| 8 Redis down/slow | AC-43, AC-44 | | |
| 9 stale cache | AC-42, AC-45, AC-46, INV-11 | | |

Trust condition 11 (operator trail) is stated only (D47) and has no AC; AC-40 covers only the command's own output.

## Traceability

| Brief line                                                       | AC / INV                                        |
| ---------------------------------------------------------------- | ----------------------------------------------- |
| Objective: decide what has to be true before real money          | Trust conditions table above; INV-01–11         |
| 5% cashback on every payment                                     | AC-01, AC-03, AC-04, INV-07                     |
| Payments under 20,000 IDR earn nothing                           | AC-02, AC-03, AC-07, AC-11                      |
| At most 50,000 IDR of cashback per day                           | AC-05–07, AC-10, AC-13, AC-14, AC-26, INV-03    |
| Budget 10,000,000 IDR; when gone, the campaign is over           | AC-08–11, AC-25, AC-41, AC-52, AC-53, INV-02    |
| Users can redeem their cashback balance                          | AC-29–35, AC-37, AC-39, AC-65, AC-69            |
| In scope: payments that earn or don't                            | AC-01–28, AC-70, AC-71                          |
| In scope: what a user needs to see and do                        | AC-41–49, AC-58–68, AC-72–74                    |
| Out of scope: refunds and clawback                               | Non-goals (TC17)                                |
| Out of scope: authentication                                     | AC-18, AC-48; non-goals (TC15, TC16)            |
| Out of scope: products; a payment is just an amount              | AC-17                                           |
| Use AI to build this                                             | Process (AGENTS.md workflow); no AC             |
| MVP, production grade                                            | AC-27, AC-43–46, AC-50–56, AC-70, AC-72–74, INV-01–11 |
| Work out what's missing; decide what makes the cut               | Trust conditions; non-goals; D08                |
| Stack: Go, PostgreSQL, Redis, React Native                       | AC-43, AC-53, AC-56, AC-58                      |
| A working demo we can run                                        | AC-56, AC-57, AC-67, AC-68                      |
| Push to GitHub                                                   | Ship stage; CI kept (D08, D31); no AC           |
| Interview: decisions, rejected options, where it breaks          | README sections; tech-spec "Where it breaks"    |

## Assumptions

- Assumption: an amount in decimal or exponent form (`100000.0`, `1e5`) is 422 `INVALID_AMOUNT`, and an unknown body
  field is 400 `MALFORMED_REQUEST` (AC-17).
- Assumption: header checks run in the order user, key, body (AC-18); the key may use upper- or lower-case hex.
- Assumption: PostgreSQL unreachable is 500 `INTERNAL_ERROR`; the contract reserves `SERVICE_BUSY` for lock waits
  (AC-55).
- Assumption: `SERVICE_BUSY` (503) also covers the other ways a money transaction gives up before `COMMIT` with
  nothing committed: statement timeout (`57014`), deadlock (`40P01`), and the 5 s transaction deadline. An error at or
  after `COMMIT` is 500, because the outcome is then unknown (tech-spec §4.4).
- Assumption: a change of the daily cap on the campaign row applies from the next campaign day; the current day keeps
  the cap copied into the user-day row (AC-71). D07 builds no live rule-change command, so this arises only by SQL.
- Assumption: `demo-reset` is the one path that truncates ledger, payment, and redemption rows and resets spent to 0;
  it refuses unless `FC_DEMO=1` and is never run in production (INV-06, TC10, AC-57).
- Assumption: the app saves attempts as a list keyed by idempotency key, so a new attempt never overwrites an unresolved
  one; Home shows one card per old attempt. If saving fails, nothing is sent and the generic error shows (AC-72–74).
- Assumption: the demo state is user_a balance 15000 / earned 47000 (the wireframe example), user_b empty, user_c at
  the cap (AC-57).
- Assumption: idempotency keys never expire; they live as long as the row they created.

The reason when both limits cut an award is D45, not an assumption (AC-10).
