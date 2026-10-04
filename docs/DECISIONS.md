# Decisions

Every material decision for Flash Cashback, with the owner's reason in the owner's own words. The open decisions from
the AGENTS.md Project Profile come first (D01–D08), then the extra question raised by the spec pass (D09), then the
batches confirmed together (D10–D42), then the conflicts between the approved inputs and how they were resolved. IDs
are never reused. A later change adds a new entry that supersedes an old one; it does not edit the old one.

## Summary

| ID  | Decision                                       | Chosen                                                                                 |
| --- | ---------------------------------------------- | -------------------------------------------------------------------------------------- |
| D01 | Where the daily cap and budget are enforced    | PostgreSQL, inside the payment transaction                                             |
| D02 | Award when 5% does not fully fit               | Partial award                                                                          |
| D03 | When the budget is spent                       | At award                                                                               |
| D04 | What "per day" means                           | Calendar day in Asia/Jakarta (WIB), decided by the database clock                      |
| D05 | Kill switch                                    | Two independent switches: awards, redemptions                                          |
| D06 | What Redis is for                              | Read caches for `GET /campaign` and `GET /me/cashback`; no rate limiting               |
| D07 | Trust conditions                               | The trust conditions table below                                                       |
| D08 | Cut line                                       | Keep read caches, CI; drop `ENDING_SOON`, cursor paging, detail sheet, load test       |
| D09 | Demo path without a Mac                        | Expo Go on a phone; iOS simulator documented; README recording and curl examples      |
| D10 | Base URL                                       | `http://localhost:8080/v1`                                                             |
| D11 | Money                                          | Integer IDR, `int64` / `BIGINT`                                                        |
| D12 | Rounding                                       | Integer math, rounded down                                                             |
| D13 | Minimum                                        | Exactly Rp20.000 earns                                                                 |
| D14 | Amount range                                   | 1..10.000.000 for payments and redemptions                                             |
| D15 | User identity                                  | `X-User-ID` matching `^[a-z0-9_-]{1,64}$`; demo users user_a, user_b, user_c           |
| D16 | Retries                                        | `Idempotency-Key` UUID on every POST, scoped per (user, operation)                     |
| D17 | Payment vs cashback                            | A valid payment always succeeds; cashback can be Rp0                                   |
| D18 | Budget figures                                 | Never in a response                                                                    |
| D19 | Time format                                    | RFC 3339 with offset                                                                   |
| D20 | Codes                                          | UPPER_SNAKE_CASE; the app has a fallback                                               |
| D21 | Errors                                         | `{"error":{"code","message","request_id"}}`; message not shown to users                |
| D22 | Request ID                                     | `X-Request-ID` echoed or generated                                                     |
| D23 | Operations                                     | CLI tools, not HTTP                                                                    |
| D24 | HTTP router                                    | chi                                                                                    |
| D25 | PostgreSQL access                              | pgx v5, hand-written SQL                                                               |
| D26 | Migrations                                     | goose, embedded, run at API start, Postgres session locker                             |
| D27 | Redis client                                   | go-redis v9                                                                            |
| D28 | Logging                                        | `log/slog` JSON                                                                        |
| D29 | Go checks                                      | gofmt, go vet, staticcheck                                                             |
| D30 | Integration tests                              | Real PostgreSQL and Redis from a test compose file                                     |
| D31 | CI                                             | GitHub Actions                                                                         |
| D32 | Mobile framework                               | Expo + Expo Router, TypeScript                                                         |
| D33 | Server data in the app                         | TanStack Query                                                                         |
| D34 | Mobile tests                                   | Jest + React Native Testing Library                                                    |
| D35 | Key generation                                 | expo-crypto on the phone, google/uuid in Go                                            |
| D36 | Remembered demo user                           | `@react-native-async-storage/async-storage`                                            |
| D37 | Mobile lint and typecheck                      | ESLint with `eslint-config-expo`, `tsc --noEmit`                                       |
| D38 | Money writes                                   | Every money write is one database transaction                                          |
| D39 | Idempotency storage                            | Key stored with the row it creates, in the same transaction                            |
| D40 | Ledger                                         | Append-only ledger; a balance row holds the running total                              |
| D41 | Constraints                                    | `CHECK` and `UNIQUE` state every invariant the code relies on                          |
| D42 | Reconcile                                      | A reconcile command proves the invariants and only reports                             |
| D43 | How a redemption reads the redemption switch   | Plain read of the flag; no lock on the campaign row                                    |
| D44 | Lock order                                     | `campaigns` → `user_daily_earnings` → `cashback_balances` → new rows                   |
| D45 | Reason when both limits cut an award           | `PARTIAL_BUDGET` only when the award equals the budget left; else `PARTIAL_DAILY_CAP`  |
| D46 | When a redemption reads the switch flag        | After the balance lock; supersedes the D44 redemption order                            |
| D47 | Operator action trail (spec check F1)          | Not built; trust condition 11 becomes stated only                                      |
| D48 | App killed while checking                      | Persist the in-flight attempt; resume with the same key after relaunch                 |

## Open decisions

### D01 — Where the daily cap and the budget are enforced

- **Options:** A. PostgreSQL, inside the payment transaction · B. Redis counter reserves, PostgreSQL settles later ·
  C. PostgreSQL with the budget split into N bucket rows
- **Recommended:** A
- **Chosen:** A
- **Rationale:** "from the calculation, the campaign is still small i believe. with 10m budget and the smallest full
  award at 1000, then it's roughly 10000 awards in total, so speed is not my problem here. the main thing is, i should
  be calculated right."
- **Cost accepted:** "every award waits for the same campaign row, so during a burst they run one at a time. Some users
  may get a 503 and the app retries with the same key. Throughput is limited by that one row."
- **Would revisit if:** "the budget or traffic grows a lot (around 100x), or I see many 503s from lock timeouts in
  testing. Then I'd put a Redis counter in front as a fast gate, with the database still the final record, or split
  the budget into buckets."
- **Note:** D08 drops the load-test command, so "in testing" means the concurrency tests (as the owner states in D08).
  A lock timeout returns 503 `SERVICE_BUSY`, retryable with the same key.

### D02 — What a payment earns when its full 5% does not fit

- **Options:** A. Partial award: min(5% rounded down, cap left today, budget left) · B. All or nothing
- **Recommended:** A
- **Chosen:** A
- **Rationale:** "it will affect the life time of a campaign. whenever there are small amount of money, we need to give
  it to user. the B option will create never-ending effect where the budget never reach 0 and that will make user will
  still see the campaign banner but not getting any cashback. it will affecting their trust."
- **Cost accepted:** the owner accepted the stated cost: some users get less than 5% and need copy that explains why;
  two extra reason codes (`PARTIAL_DAILY_CAP`, `PARTIAL_BUDGET`); a tie-break rule when the cap and the budget both cut
  the award.
- **Would revisit if:** "after i get new requirement from the product team."
- **Copy:** "let's use copy like "cashback UP TO xxx %"" — the promise reads "Cashback up to 5%" (see C10).
- **Assumptions:** when the cap and the budget both cut the award, the reason is `PARTIAL_BUDGET`. Rp0 reasons are
  checked in the order `BELOW_MINIMUM`, `CAMPAIGN_ENDED`, `CAMPAIGN_PAUSED`, `DAILY_CAP_REACHED`.

### D03 — When the budget is spent

- **Options:** A. At award · B. At redemption
- **Recommended:** A
- **Chosen:** A
- **Rationale:** "there are two balances. The cashback balance is ours: it goes up when cashback is awarded and down when
  it is redeemed. The main account balance belongs to another system and only receives the money on redemption. "Gone"
  means awarded into the cashback balance. Redemption is a separate flow that moves money the user already owns, so the
  budget does not track it."
- **Cost accepted:** the owner accepted the stated cost: cashback that is never redeemed still counts against the budget
  forever; it is promised but never paid out.
- **Would revisit if:** "finance wants cashback expiry"
- **Note:** redemption works after the campaign has ended, unless redemptions are paused (D05). The hand-off to the main
  account is trust condition 13.

### D04 — What "per day" means

- **Options:** A. Calendar day in Asia/Jakarta (WIB) · B. Calendar day in UTC · C. Rolling 24 hours · D. The device's
  time zone. Clock: (1) PostgreSQL `now()` in the payment transaction · (2) the API server's clock
- **Recommended:** A with clock (1)
- **Chosen:** A with clock (1): "Calendar day in WIB, decided by the database clock."
- **Rationale:** "matches users' expectation and drafted copy"
- **Cost accepted:** "it will reset at 00, so WITA and WIT will be like 01.00 or 02.00. it's acceptable i guess." and
  "accept midnight rush" (a user can earn the full cap just before and again just after 00:00 WIB).
- **Would revisit if:** "abuse shows up around midnight"
- **Note:** `now()` is the transaction start time, so a payment whose transaction starts before 00:00 WIB counts for the
  old day even if it commits after. One function computes the campaign day; nothing else does.

### D05 — Kill switch

- **Options:** A. No switch · B. One switch, awards only · C. One switch, awards and redemptions · D. Two independent
  switches
- **Recommended:** D
- **Chosen:** D
- **Rationale:** "it will allow us (as company) to have more control about the running campaign."
- **Cost accepted:** "one more flag, one more command and one more refusal code than a single switch, plus tests for
  both. A redemption that already passed the check finishes even after the pause; the switch stops new redemptions, it
  does not undo one in progress."
- **Would revisit if:** "in practice the two switches are always pressed together. Then one switch is simpler to operate
  and I would merge them."
- **Note:** both flags live on the PostgreSQL campaign row, never in Redis. A paused award earns Rp0 with
  `CAMPAIGN_PAUSED`; a paused redemption returns 409 `REDEMPTION_PAUSED`. Each pause and resume writes a structured log
  line (trust condition 11).

### D06 — What Redis is for

- **Options:** A. Rate limiting only · B. Read caches only (`GET /campaign`, `GET /me/cashback`) · C. Rate limiting plus
  a short-TTL cache of `GET /campaign` · D. Counters for cap and budget (ruled out by D01)
- **Recommended:** A, fail open
- **Chosen:** B (against recommendation)
- **Rationale:** "these two reads are called every time the home screen opens or refreshes, so they are the most
  frequent requests. Caching them takes that load off the database and leaves its connections for the payment and
  redemption transactions. The campaign data is the same for every user, so one cached copy serves everyone."
  On rejecting rate limiting: "I don't choose rate limiting because the user ID comes from a header that anyone can
  change, so a per-user limit is easy to avoid, and a per-IP limit can block honest users who share an IP on mobile
  networks. The daily cap already limits what one account can earn. I also don't want a helper that can refuse a real
  payment by mistake."
- **When Redis is down:** "read from PostgreSQL. Slower, but never wrong, and payments are not affected."
- **Cost accepted:** "a cached value can be out of date for a short time, so the cache must be deleted after every
  payment and redemption commits, and that needs tests. There is no rate limit, so a script can flood the payment
  endpoint; the damage is limited by the daily cap and the lock timeout."
- **Would revisit if:** "I see abuse or bursts on the payment endpoint. Then I add rate limiting, behind real
  authentication so the limit is tied to a verified user."
- **Assumptions:** `GET /campaign` is one shared key with a TTL of about 5 s, deleted after a pause or resume and when
  the budget reaches 0. `GET /me/cashback` is one key per user, deleted after each award or redemption commits for that
  user, with a TTL of about 60 s as a backstop. Each Redis call has a deadline of about 50 ms; a slow Redis counts as
  down. No money decision reads the cache.

### D07 — Trust conditions

- **Recommended:** the statuses as proposed
- **Chosen:** the proposed statuses, with these owner changes: row 11 is built as a structured log line, and a durable
  `operator_actions` table is stated only; row 13 is stated only; rows 21 and 22 added.
- **Rationale:** "I split the list by one question: can I guarantee it with what exists in this project? If the
  condition only needs my database, my code and my app, I build it and prove it with a test, because that is what
  "production ready" means for the part I own. If it needs a system that is not here, like real authentication, the
  main account or payment settlement, I don't fake that system. I write down what production needs so nothing is
  hidden."
  On row 13: "The main account system does not exist here, so a fake one would add scope without proving anything
  real."
- **Cost accepted:** "the conditions marked stated only are real gaps. This cannot go live with real money until
  identity, the main account transfer and settlement are connected. A reviewer can see exactly which ones."
- **Would revisit if:** "any of those systems becomes available. The first one I would build is the main account
  transfer with an outbox and a pending status, because that is where money leaves our system."

#### Trust conditions

The rows marked **built** are this project's definition of production ready. The README copies this table.

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

### D08 — Cut line

- **Options:** keep or drop each of: read caches, `ENDING_SOON`, cursor paging in history, payment detail sheet,
  load-test command, CI
- **Recommended:** keep read caches, load test, CI; drop `ENDING_SOON`, cursor paging, payment detail sheet
- **Chosen:** keep read caches, CI; drop `ENDING_SOON`, cursor paging, payment detail sheet, load-test command (the
  load-test drop is against recommendation)
- **Rationale:** "I keep what proves a trust condition or is already decided, and drop what the brief does not ask for.
  ENDING_SOON hints at how much budget is left, which goes against keeping budget figures on the server. A user capped
  at Rp50.000 a day makes only a few payments, so 20 history rows is enough and the row already shows the reason. CI is
  one file and shows a reviewer the tests pass. The load test is the biggest tool to build and explain, and the
  concurrency tests already prove the limits, so I leave it out to keep the scope small."
- **Cost accepted:** "the limits are proved by tests inside the code, not through real HTTP traffic. I have no tool that
  measures lock timeouts under load, so my D01 revisit condition is checked by the concurrency tests only. Users see at
  most 20 history items and no detail view."
- **Would revisit if:** "I need to measure real throughput before a launch. Then I add the load test first. Paging and
  the detail sheet come back if users can make many payments per day."

## Further questions

### D09 — Demo path without a Mac

- **Options:** A. Expo web as the guaranteed path, Expo Go documented · B. Expo Go on a phone (Android or iPhone) ·
  C. Android emulator only
- **Recommended:** A
- **Chosen:** B (against recommendation), with two additions: "The documented paths are the iOS simulator and Expo Go on
  a phone (Android or iPhone), with the API base URL set by an environment variable. The README also has a short screen
  recording of the app, and curl examples so the backend can be checked without the app at all."
- **Rationale:** "the brief asks for React Native and says I will demo it myself, so the main demo is native on a
  simulator. I don't want to add a web build and CORS handling on the server just for the demo, because that is code the
  product does not need. A recording and curl examples let a reviewer see it working without any setup."
- **Cost accepted:** "a reviewer without a Mac who wants to run the app needs a phone on the same network and has to set
  the API address. That setup can fail."
- **Would revisit if:** "reviewers cannot run it. Then I add the web build as the easiest path."
- **Assumption:** the base URL comes from `EXPO_PUBLIC_API_URL`, default `http://localhost:8080/v1`.

### D43 — How a redemption reads the redemption switch

Raised by the spec pass: the draft locked the campaign row `FOR SHARE` in every redemption, so redemptions queued
behind in-flight awards.

- **Options:** A. Plain read of the flag, no lock on the campaign row · B. `FOR SHARE` lock on the campaign row ·
  C. The redemption flag on its own row, locked
- **Recommended:** A
- **Chosen:** A
- **Rationale:** "in D03 I decided redemption is a separate flow that moves money the user already owns. It should not
  wait behind other people's payments. The lock on the campaign row is the cost I accepted for awards in D01, not for
  redemptions. And in D05 I already accepted that a redemption which passed the check finishes even after a pause, so
  the lock does not buy anything I asked for."
- **Cost accepted:** "when the pause command returns, one redemption that already read the flag may still commit a few
  milliseconds later. The operator cannot say "nothing is in flight" at that instant. It is visible in the log and in
  reconcile."
- **Would revisit if:** "operations needs a hard guarantee that nothing completes after a pause, for example for a fraud
  freeze. Then I move the redemption flag to its own row and lock that (option C)."
- **Note:** trust condition 7 is amended to match (C13).

### D46 — When a redemption reads the redemption switch

Raised by the spec repair: in the D44 order the flag was read before the balance lock, so a redemption waiting on its
balance row could hold an old "not paused" answer for up to the lock timeout. Supersedes the redemption order in D44.

- **Options:** A. Read the flag after taking the balance lock (amend the D44 redemption order) · B. Keep the D44 order
  and widen the D43 cost to the lock timeout
- **Recommended:** A
- **Chosen:** A
- **Rationale:** "the check should happen as late as possible. If the redemption reads the flag first and then waits for
  the balance row, it can hold an old "not paused" answer for up to the lock timeout and still commit after the pause.
  Locking the balance first means the waiting is done before the check, so the check sees any pause that already
  committed. The read still takes no lock, so redemptions do not queue behind awards."
- **Cost accepted:** "the redemption order in D44 changes, and the spec steps change with it. There is still a small
  window between the flag read and the commit, a few milliseconds, which is the cost I accepted in D43."
- **Would revisit if:** "same as D43. If operations needs a hard guarantee that nothing completes after a pause, I move
  the redemption flag to its own row and lock it."
- **Redemption order now:** `cashback_balances` (`FOR UPDATE`) → plain read of the `campaigns` flag → new rows. A user
  with no balance row takes no lock; the flag is read at once.

### D47 — Operator action trail

Raised by the spec check (F1): the admin command runs through `docker compose exec`, so its log line reaches only the
operator's terminal and is not kept. Amends trust condition 11.

- **Options:** A. Admin writes the line into the API's log stream · B. Durable `operator_actions` table written in the
  same transaction · C. Not built; trust condition 11 becomes stated only
- **Recommended:** A
- **Chosen:** C (against recommendation)
- **Rationale:** "the brief does not ask for an operator audit trail and I want to keep the scope small. The log line
  from D07 only reaches the operator's terminal, so I will not call it built."
- **Cost accepted:** "a pause or resume leaves no lasting record."
- **Would revisit if:** "this goes to production. Then I add a row written in the same transaction as the flag change."

### D48 — App killed while checking

Raised by the spec check (F11): the attempt's idempotency key lived only in memory, so a kill during Checking could
lead to a second payment with a new key. Amends trust condition 21.

- **Options:** A. Persist the in-flight attempt · B. Memory only; narrow trust condition 21 to "while the app stays
  running"
- **Recommended:** A
- **Chosen:** A, with two owner details: "save the user ID with the attempt, so it is never resent as another demo
  user. And save a timestamp, so an old attempt is not resent automatically; show it to the user and let them decide."
- **Rationale:** "this is the one case where the app itself can cause a double payment. If the app is killed while
  checking, the key is lost, and the user can pay again with a new key while the first payment may have gone through.
  The server cannot detect that, because two keys look like two valid payments. Saving the attempt before sending and
  clearing it on a definite answer means the app always continues with the same key after a relaunch."
- **Cost accepted:** "one more storage path, a check at launch, and tests for "killed during checking, relaunch, same
  key"."
- **Would revisit if:** "the server offers a way to look up a payment by key without creating one. Then on relaunch the
  app can check the status first instead of resending."

## Further batch

Raised by the spec pass and confirmed by the owner as proposed ("confirm all").

| ID  | Decision                             | Chosen                                                                                                                                                                                                                                         | Reason                                                                                                                                                                                         |
| --- | ------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| D44 | Lock order                           | Award: `campaigns` (`FOR NO KEY UPDATE`) → `user_daily_earnings` → `cashback_balances` → new rows. Redemption: plain read of `campaigns` (D43) → `cashback_balances` (`FOR UPDATE`) → new rows. Switch command: `campaigns` only             | One order on every path, so no deadlock; `FOR NO KEY UPDATE` because inserts with a foreign key take `FOR KEY SHARE` on the campaign row (known pitfalls)                                     |
| D45 | Reason when both limits cut an award | `PARTIAL_BUDGET` only when the award equals what is left of the budget (the campaign ends); otherwise `PARTIAL_DAILY_CAP`. Supersedes the D02 assumption "both cut → `PARTIAL_BUDGET`"                                                          | When the cap is the tighter limit the campaign does not end, so "Flash Cashback has now ended" would be false                                                                                  |

## Batches

Confirmed by the owner as proposed ("confirm all").

### Settled defaults (from `docs/api-contract.md`)

| ID  | Decision            | Chosen                                                                                                                                                          | Reason                       |
| --- | ------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------- |
| D10 | Base URL            | `http://localhost:8080/v1`                                                                                                                                      | Version prefix               |
| D11 | Money               | Integer IDR, `int64` / `BIGINT`                                                                                                                                 | Exact                        |
| D12 | Rounding            | Integer math, rounded down                                                                                                                                      | Never over 5%                |
| D13 | Minimum             | Exactly Rp20.000 earns                                                                                                                                          | The brief says "under"       |
| D14 | Amount range        | 1..10.000.000 for payments and redemptions                                                                                                                      | Sanity bound                 |
| D15 | User identity       | `X-User-ID` matching `^[a-z0-9_-]{1,64}$`; demo users user_a, user_b, user_c                                                                                    | Authentication out of scope  |
| D16 | Retries             | `Idempotency-Key` canonical UUID on every POST, scoped per (user, operation). Same key and same body → 200 with `Idempotent-Replayed: true`; other body → 409 | Safe retry                   |
| D17 | Payment vs cashback | A valid payment always succeeds; cashback can be Rp0                                                                                                            | Bonus on top                 |
| D18 | Budget figures      | Never in a response                                                                                                                                             | Sensitive                    |
| D19 | Time                | RFC 3339 with offset (`+07:00`, per D04)                                                                                                                        | Unambiguous                  |
| D20 | Codes               | UPPER_SNAKE_CASE; the app has a fallback                                                                                                                        | Forward-compatible           |
| D21 | Errors              | `{"error":{"code","message","request_id"}}`; the message is not shown to users                                                                                  | One shape                    |
| D22 | Request ID          | `X-Request-ID` echoed or generated                                                                                                                              | Traceability                 |
| D23 | Operations          | CLI tools, not HTTP                                                                                                                                             | No authentication needed     |

### Library picks (from `docs/api-contract.md`, plus D36 and D37)

| ID  | Area                      | Chosen                                                                                       | Reason                                                     |
| --- | ------------------------- | -------------------------------------------------------------------------------------------- | ---------------------------------------------------------- |
| D24 | HTTP router               | chi                                                                                          | As proposed in the contract                                |
| D25 | PostgreSQL access         | pgx v5, hand-written SQL                                                                     | As proposed in the contract                                |
| D26 | Migrations                | goose, embedded, run at API start, with the Postgres session locker                          | Two instances must not migrate at once (known pitfalls)    |
| D27 | Redis client              | go-redis v9                                                                                  | As proposed in the contract                                |
| D28 | Logging                   | `log/slog` JSON                                                                              | As proposed in the contract                                |
| D29 | Go checks                 | gofmt, go vet, staticcheck                                                                   | As proposed in the contract                                |
| D30 | Integration tests         | Real PostgreSQL and Redis from a test compose file                                           | As proposed in the contract                                |
| D31 | CI                        | GitHub Actions                                                                               | Kept by D08                                                |
| D32 | Mobile                    | Expo + Expo Router, TypeScript                                                               | As proposed in the contract                                |
| D33 | Server data in the app    | TanStack Query                                                                               | As proposed in the contract                                |
| D34 | Mobile tests              | Jest + React Native Testing Library                                                          | As proposed in the contract                                |
| D35 | Key generation            | expo-crypto on the phone, google/uuid in Go                                                  | As proposed in the contract                                |
| D36 | Remembered demo user      | `@react-native-async-storage/async-storage`                                                  | The wireframe remembers the user across launches           |
| D37 | Mobile lint and typecheck | ESLint with `eslint-config-expo`; `tsc --noEmit`                                             | Checkpoint 1 needs `lint` and `typecheck` scripts          |

### Engineering defaults (from AGENTS.md)

| ID  | Decision            | Chosen                                                                                                                                                                                     | Reason                      |
| --- | ------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | --------------------------- |
| D38 | Money writes        | Every money write is one database transaction                                                                                                                                              | Engineering default         |
| D39 | Idempotency storage | A retryable request is idempotent by a key stored with the row it creates, in the same transaction. A 4xx creates no row, so its key is not stored; the app never retries a 4xx          | Engineering default         |
| D40 | Ledger              | An append-only ledger records every credit and debit; a balance row holds the running total                                                                                                | Engineering default         |
| D41 | Constraints         | `CHECK` and `UNIQUE` constraints state every invariant the code relies on                                                                                                                  | Engineering default         |
| D42 | Reconcile           | A reconcile command proves the invariants and only reports                                                                                                                                 | Engineering default         |

## Conflicts

Confirmed by the owner as proposed ("confirm all"). The contract and wireframe fixes are applied in the finalisation
step that follows this file.

| #   | Where they disagree                                                                                                       | Resolution                                                                                                              |
| --- | ------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------- |
| C1  | `api-contract.md` scopes idempotency keys "per user"; the AGENTS.md invariant says "per user and per operation"          | Scope by (user, operation) (D16); amend the contract                                                                    |
| C2  | `api-contract.md` says the budget comes from configuration; trust condition 10 says a restart must not reset it           | Configuration seeds the campaign row on first boot only; afterwards the row is the truth; amend the contract           |
| C3  | AGENTS.md checkpoint 4 assumes CI, which was a cut candidate                                                              | CI kept (D08); no change                                                                                                |
| C4  | The AGENTS.md layout lists `cmd/loadtest`; the contract offers a load-test command                                        | Load test dropped (D08); remove it from the AGENTS.md layout and drop the contract marker                               |
| C5  | Wireframe `DAILY_CAP_REACHED` copy vs the all-or-nothing variant                                                          | Moot: D02 chose partial awards                                                                                          |
| C6  | Wireframe "All cashback has been claimed" vs a budget that never reaches zero                                             | Moot: D02 chose partial awards                                                                                          |
| C7  | Wireframe "you can still redeem… works after the campaign has ended" vs the redemption switch (D05)                       | Keep the copy and add "unless redemptions are paused"                                                                   |
| C8  | Invariant "total cashback earned never exceeds the budget" vs budget spent at redemption                                  | Moot: D03 chose at award                                                                                                |
| C9  | The payment reference `PAY-20261003-000042` has a date with no time zone                                                  | The date comes from the D04 day function; zero-padded ID; gaps allowed                                                  |
| C10 | Wireframe promise copy "5% cashback" vs the D02 copy "Cashback up to 5%"                                                  | Amend the wireframe promise lines; the rule itself stays 5%                                                             |
| C11 | AGENTS.md final check runs the walkthrough "on a path that does not need a Mac"; D09 demos on the iOS simulator           | The final check runs on Expo Go on an Android phone; the owner's own demo can use the simulator; no AGENTS.md change    |
| C12 | Wireframe copy "while quota lasts" vs "While cashback lasts"                                                              | Unify to "while cashback lasts"                                                                                         |
| C13 | Trust condition 7 claimed a pause cannot land mid-redemption; D43 reads the redemption flag without a lock               | Amend row 7: a redemption either sees the pause or passed the check before it and completes                             |
| C14 | `api-contract.md` writes `GET /healthz`; the real path is `/v1/healthz`, and the body shape is not named                 | Write `/v1/healthz` and name the body `{"status","dependencies":{"postgres","redis"}}`                                  |
| C15 | `api-contract.md` `PARTIAL_BUDGET` row says "only if the budget was the tighter one", leaving a tie undefined           | Match D45: the award equals what was left of the budget, so the campaign has ended, including a tie with the cap       |
| C16 | The `/v1/healthz` 503 body values were not in the contract                                                                | Add 503 `{"status":"unavailable","dependencies":{"postgres":"down","redis":"ok" or "degraded"}}` to the contract     |
