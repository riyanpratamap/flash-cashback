# Flash Cashback — Delivery Plan

Order of work for [prd.md](../docs/prd.md) and [tech-spec.md](../docs/tech-spec.md) ("§" = tech-spec section). Phases
follow the AGENTS.md outline as cut by D08 (read caches and CI kept), with the load test restored by D51 and dropped again by D52. Each task lands as one commit
carrying its code, its tests, its ticked box here, and its `plans/learnings.md` row (AGENTS rule 9). **29 tasks**,
above the ~25 guide by owner decision: smaller reviewable tasks; merging would create two oversized tasks;
P4.5 added by D51; its tooling removed by D52. P4.6 added by D53.

**Naming (D49):** repository `github.com/riyanpratamap/flash-cashback` (the existing `origin`, cloned by the
clean-clone check); Go module `github.com/riyanpratamap/flash-cashback/backend`, `go.mod` in `backend/`. Tools on the
host: Docker with Compose v2, Go (version pinned in P0.1), Node LTS (P5); `gh` CLI optional (else check CI runs in the
browser on the GitHub Actions page).

**Conventions for every task.** Skills: `go` = programming-go + developing-backend; `ts` = programming-typescript +
developing-mobile-ui. Read the known-pitfalls sections for the tools touched. **Task gate** (default): `make gate`
exits 0 and the stop check passes; concurrency tasks also run `make test-race` (`-race -count=20`, tests named
`TestRace*`); mobile tasks run `make mobile-check`. Every concurrency test starts on a barrier, rolls back lock
holders in cleanup, asserts errors and totals separately, then asserts reconcile (INV-01–09). Each mutation is applied
to the code or the live test schema (never the migration file), shown red, reverted, and reported. Integration state
is built through the services; a test may set only a switch flag or the reseeded budget by SQL. **Critical** tasks get
`code-checker`. Evidence is the exit code, read directly (e.g. `cmd; echo exit=$?`), never through a pipe's tail.

## P0 — Tooling, compose skeleton, migrations, health check

- [x] **P0.1** Go module, config, Makefile, test compose, CI gate job — TC20 (base) · `go` · not critical
  - `backend/go.mod` (module path per D49, Go toolchain pinned), `internal/config` (§8 table, defaults, integer timeouts
    validated), `.env.example`, `.gitignore` (`.env`), `docker-compose.test.yml` (pinned majors, host ports 55432 /
    56379), `internal/integration/doc.go` (`//go:build integration`, package only), `Makefile`:
    `fmt fmt-check vet lint test test-integration test-race gate mobile-check`. `test` = `go test ./...` (no tags, no
    database; also what pre-push runs). `test-integration` = `docker compose -f docker-compose.test.yml up -d --wait`,
    then `cd backend && go test -tags integration -race -p 1 -count=1 ./internal/integration/...`. `test-race` = same
    bring-up, then `go test -tags integration -race -p 1 -count=20 -run '^TestRace' ./internal/integration/...`.
    `mobile-check` prints "mobile/ not created" and exits 0 until P5.1. `.github/workflows/ci.yml` job `gate`
    (`make gate`, `make test-race`), action versions pinned.
  - Install: `cd backend && go mod init github.com/riyanpratamap/flash-cashback/backend &&
    go get -tool honnef.co/go/tools/cmd/staticcheck@<pinned>`.
  - Done when: `make gate` exits 0 with config unit tests (defaults; `LOCK_TIMEOUT=abc` rejected); `make
    test-integration` and `make test-race` each exit 0 with no tests yet ("no test files").
  - Result: `make gate`, `test-integration`, `test-race` exit 0; staticcheck v0.8.1 pinned; postgres:18 and redis:8 healthy with tmpfs.
- [x] **P0.2** Migration, boot, integration harness — AC-53 · INV-02–07 (constraints), INV-06 (triggers) · TC10 · `go`
  · **critical** (migration)
  - `migrations/00001_init.sql` exactly as §2 + `embed.go`; `internal/boot` (connect with 30 s retry, goose up with
    the Postgres session locker, seed `ON CONFLICT DO NOTHING`); `internal/integration` harness (truncate + reseed
    with a budget, `fc_now()` replace and restore) and `freshDB(t)`: `CREATE DATABASE fc_boot_<random>` on the test
    server, cleanup `DROP DATABASE … WITH (FORCE)`.
  - Install: `go get github.com/jackc/pgx/v5@<pinned> github.com/pressly/goose/v3@<pinned>`.
  - Tests: boot on `freshDB` → one campaign row, spent 0, switches off; every §2 constraint name exists in the
    catalogue; UPDATE/DELETE on ledger, payments, redemptions raise. `TestRaceBootTwice`: two boots at once on one
    `freshDB` per run → one migration, one row. **Mutation:** boot without the session locker, run against `freshDB` → red.
  - Done when: `make test-integration` and `make test-race` exit 0; no `fc_boot_*` database left after; mutation reported.
  - Result: `make gate`, `make test-race` exit 0, no `fc_boot_*` left; pgx v5.11.0, goose v3.28.0 pinned; no-locker mutation red (`pg_proc` duplicate key), constraint and trigger drops red; `TestRaceBootTwiceEmpty` (empty DB, outcome-only) added; re-boot keeps budget and spent (TC10), reset-on-conflict mutation red.
- [x] **P0.3** API server, healthz, image, compose — AC-50, AC-56 · TC8 (health), TC20 · `go` · not critical
  - `cmd/api` (§8 timeouts, 8 s drain on SIGTERM, `healthcheck` subcommand), chi router, `GET /v1/healthz` (PG 1 s,
    Redis 50 ms, go-redis options of §6); `cmd/admin` and `cmd/reconcile` as stubs (exit 2, "not built yet") so the
    image and the Commands table exist from P0; `backend/Dockerfile` (multi-stage, `CGO_ENABLED=0`, distroless,
    `/app/api|admin|reconcile`), `.dockerignore`, root `docker-compose.yml` (TCP `pg_isready`, `service_healthy`,
    only 8080 published, every variable defaulted).
  - Install: `go get github.com/go-chi/chi/v5@<pinned> github.com/redis/go-redis/v9@<pinned> github.com/google/uuid@<pinned>`.
  - Done when: integration tests give 200 `status` ok, redis ok / 200 `status` ok, redis `degraded` (Redis at a closed
    port) / 503 `status` `unavailable`, postgres `down` (PG at a closed port, C16 body); `docker compose up -d --build
    --wait` exits 0; `docker compose ps` shows only 8080.
  - Result: `make gate`, `make test-race` exit 0; `docker compose up -d --build --wait` exit 0, `ps` publishes only 8080, healthz `ok`/`ok`/`ok`; chi v5.3.2, go-redis v9.22.0 pinned (google/uuid deferred to P1.2, first use); mutations red: PG failure mapped to 200 (unit and integration), Redis failure not degraded.

**P0 gate** (in order):
1. Add CI job `smoke` to `ci.yml` (`up -d --build --wait`, `curl -fsS` healthz, only 8080 published, `down -v`); own
   commit `ci(infra): add compose smoke job`.
2. `git config core.hooksPath .githooks` · `make gate` → exit 0 · `make test-race` → exit 0.
3. `docker compose up -d --build --wait` → exit 0 · `curl -fsS localhost:8080/v1/healthz` → `"status":"ok"` ·
   `docker compose down`.
4. **Stop check proof:** add `backend/internal/zz_stop.go` containing `package internal;func  x(){}`, then
   `printf '{}' | scripts/agent-stop-check.sh; echo "exit=$?"` → gofmt message and `exit=2`; with the file still
   there, a subagent asked to finish is blocked by the hook (its transcript shows the stop-check message).
5. **Pre-commit proof:** `git add backend/internal/zz_stop.go && git commit -m "chore(repo): probe"; echo exit=$?` →
   gofmt rejection, nonzero; `git reset backend/internal/zz_stop.go`, delete the file, rerun the stop check → `exit=0`.
6. `git commit --allow-empty -m "Bad message"; echo exit=$?` → rejected by `commit-msg`, nonzero.
7. `git push` (pre-push runs `make test` → passes) · CI `gate` and `smoke` green (`gh run watch --exit-status` → exit
   0, or green in the browser).
8. The AGENTS.md clean-clone check → healthz succeeds, then `down -v` in the clone.

**P0 gate result (2026-10-05):** steps 1–3 and 5–8 pass; CI run 37252821294 `gate` and `smoke` green. Step 4: the
script exits 2 and blocks the main session; the subagent block is not proven (probe transcript shows no message).
The owner accepted this evidence; the subagent proof moves to P6.2.

## P1 — Pure rules and read endpoints

- [x] **P1.1** Domain rules — AC-01–12 and AC-71 (rule level), AC-13 (reference) · INV-07 · TC1, TC2 · `go` · **critical**
  - `internal/domain`: `Award`, `CampaignStatus`, `TodayRemaining`, `Reference`, WIB formatting (fixed `+07:00`).
  - Done when: `make test` exits 0 with the §3 award table, every reason, the AC-41 status table, `000042` /
    `1234567` references; **mutation:** check cap before budget for the reason → the tie row (D45) red.
  - Result: `make test`, `make gate` exit 0; `internal/domain` stdlib only, §3 award table (+ nonzero-spent, AC-71 rows), AC-41 table, `000042`/`1234567`, WIB day table; mutations red: cap before budget (tie row), budget/cap dropped from `min`, round up (INV-07 grid); cap above minimum (AC-07 row); min-bound check removed (OutOfRange rows).
- [x] **P1.2** Validators and HTTP edge — AC-17, AC-18, AC-54 · TC20 · `go` · **critical** (key parse, request hash)
  - Pure validators in `internal/domain` (§1, §7, §11: user ID, key length + 8-4-4-4-12 + `uuid.Parse`, raw-token
    amount, `limit`), table-tested there; handlers call them. Request ID echo/generate, access log, recover; D21 error
    mapper incl. chi 404/405; `request_hash`.
  - Done when: `make test` exits 0 with domain tables for every AC-17/18 value, and `httptest` through the real router
    with service fakes for the user → key → body order, panic → fixed 500 with no stack, `request_id` = `X-Request-ID`.
  - Result: `make test` and `make gate` exit 0; domain tables cover every AC-17/18 value, `httptest` through `NewRouter` covers check order, 404/405, panic → fixed 500, request ID echo, access log; google/uuid v1.6.0 pinned; mutations red: key before user, no 36-char/canonical check, no `recoverer`, amount decoded into an `int64` struct field.
- [x] **P1.3** Store reads, `GET /campaign`, `GET /me/cashback` — AC-16, AC-41 (flag rows), AC-48, AC-49 (these two) ·
  INV-10 · TC22 · `go` · not critical
  - `internal/store` pool, `campaigns.Get`, `balances.Today` (one query, day from `fc_campaign_day(fc_now())`);
    `service.Reads`; handlers.
  - Done when: integration tests: user_new gives 0 / 0 / 50000, WIB date, next 00:00+07:00; AC-41 rows 1, 2, 5; no
    response key matches `budget|spent`.
  - Result: `make gate` exit 0; `store.GetCampaign`/`store.Today` (one query, `fc_campaign_day(fc_now())`), `service.Reads`, `GET /v1/campaign` and `GET /v1/me/cashback`, views in `internal/domain`; integration: user_new 0/0/50000 with WIB date and next 00:00+07:00 at 16:59:59Z and 17:00:00Z, AC-41 rows 1/2/5, no `budget|spent` key (active, paused), `/campaign` body equal for budgets 2000 and 9000000; mutations red: redemption flag ignored, `now()` instead of `fc_now()`, `Budget` field added to the view. Owner: no state seeded by SQL beyond flags and budget; spent rows, ended status, cross-user scoping (AC-48) and its mutations moved to P2.2 / P3.3.

**P1 gate:** `make gate` → exit 0 · `docker compose up -d --build --wait` · `curl -fsS -H 'X-User-ID: user_a'
localhost:8080/v1/me/cashback` → balance 0 · `curl -s localhost:8080/v1/campaign` (no user) → 400 `MISSING_USER` ·
`docker compose down`.

**P1 gate result (2026-10-05):** all steps pass. `make gate` exit 0; `up -d --build --wait` exit 0; `GET
/me/cashback` as user_a → balance 0, earned 0, remaining 50000, WIB date and next 00:00+07:00; `GET /campaign` with
no user → 400 `MISSING_USER`; `down` exit 0.

## P2 — Payment award, idempotency, reconciliation, concurrency

- [x] **P2.1** Money transaction helper — AC-27, AC-55 (mapping) · TC3 · `go` · **critical** (locking)
  - `store.InTx` (§4, §4.4): `context.WithoutCancel` + 5 s, `SET LOCAL` timeouts from validated ints, deferred
    rollback ignoring only "tx closed", SQLSTATE → busy (`55P03`, `57014`, `40P01`, deadline before COMMIT) /
    invariant (`23514`, `23505`, append-only raise) / unknown (COMMIT error); mapper: busy → 503 `SERVICE_BUSY`, else 500.
  - Done when: integration tests force each SQLSTATE and show a cancelled parent context does not cancel the tx.
  - Result: `store.TxRunner.InTx` plus `SERVICE_BUSY` mapping; integration tests force 55P03, 57014, 40P01, cap
    deadline, 23514, 23505, append-only and a COMMIT error; cancelled parent and PG-down proved; 4 mutations red: dropped `WithoutCancel` (cancelled-parent test), dropped `SET LOCAL lock_timeout`
    (lock-timeout test, red only after its "gave up within 2 s" assertion), COMMIT errors classified by SQLSTATE
    (commit-error test), `ErrBusy` mapped to 500 (mapper busy/deadlock test). The deadlock test is named
    `TestRaceTx…`, so it runs under `make test-race`.
- [x] **P2.2** `Payments.Pay` and `POST /payments` — AC-01–12, AC-15, AC-71, AC-41 (spent rows), AC-17/18 (no row) ·
  INV-01–04, 06–08 · TC1, TC2, TC12 · `go` · **critical**
  - §4.1 steps 1, 3–4, 6–9 and COMMIT (cache deletes come in P4.3); lock order D44; rule snapshot; money log line
    (§7). AC-12 sets `awards_paused` by SQL until P4.1.
  - Done when: integration tests for each AC and the boundary list (Rp19.999 / 20.000 / 20.001, exact cap, last of
    budget, both short incl. the tie, no rows, ended); **mutation (AC-71):** `Award` uses the campaign cap → 500.
  - Carried from P1.3: `GET /campaign` `ENDED` (incl. ended wins over paused) and no `budget|spent` key with spent
    partial and equal, built by payments; `GET /me/cashback` for user_a after payments vs user_new (AC-48 reads);
    **mutations:** `spent` ignored in `Status`; balance subquery and user-day join in `store.Today` unscoped;
    `TodayRemaining` without the user-day row.
  - Result: `store.Pay` (§4.1 steps 4, 6-9, D44 order, user-day cap, one `fc_now()`), `service.Payments.Pay` with the §7 money log line (incl. `request_id`, carried in `domain.MoneyCommand`) after COMMIT, `POST /v1/payments` 201 with the contract body, wired in `cmd/api`; integration tests for AC-01-12, 15, 17, 18, 71, AC-41 spent rows, AC-48 reads, a two-user payment, the log line and the boundary list, clock pinned; books asserted after each. Mutations red: `Award` with the campaign cap (AC-71 → 500), `spent` ignored in `Status`, `Today` balance subquery unscoped, user-day join unscoped, `TodayRemaining` without the row, ledger insert skipped, `earned` not added, `user_id` dropped from the user-day UPDATE and from its FOR UPDATE select, literal rate/min in the payment insert, `campaignCap` snapshotted instead of `userCap`, money log line demoted and its `request_id` removed. Replay is not built (P2.3): a duplicate key surfaces as 23505 → 500.
- [x] **P2.3** Payment idempotency — AC-19–22 (payment parts) · INV-05 · TC3 · `go` · **critical**
  - Fast replay, in-lock lookup (step 5), `ON CONFLICT … DO NOTHING RETURNING` no row → rollback + replay, 409 on
    hash mismatch, `Idempotent-Replayed: true`, body rebuilt from the stored row.
  - Done when: integration tests incl. replay after pause and after end; **mutation:** skip the hash compare → AC-20 red.
  - Result: `store.FindPayment` (pool or tx), step-5 lookup under the campaign lock and `ON CONFLICT ON CONSTRAINT payments_user_key_unique DO NOTHING RETURNING id` with no row, both ending in `store.ErrReplay` so `InTx` rolls back; `service.Payments.Pay` fast path before the tx and one `replay` helper (hash compare → `ErrIdempotencyKeyReused`, else body rebuilt from the row, money log `replayed=true`); POST /payments 200 + `Idempotent-Replayed: true` or 409 `IDEMPOTENCY_KEY_REUSED`. Tests: AC-19 (byte-equal body, no new rows, after pause and after end), AC-20 (awarded and zero-award originals), AC-21 payment part, AC-22, step-5 direct (asserts the attempt wrote nothing before replaying), fast path under a held campaign lock, replay money log line, httptest mapping. Mutations red: M1 hash compare skipped → AC-20; M2a fast path and step 5 dropped together stays green via the conflict path (step 9); M2b then also `ON CONFLICT` dropped → AC-19/20 red (500); F1 fast-path block deleted → `TestPayFastReplayTakesNoCampaignLock` red (503 SERVICE_BUSY after the 2s lock timeout); F2 step-5 return deleted → `TestPayStepFiveLookupFindsCommittedPayment` red (in-tx spent 10000, want 5000); F3 replay logged with `replayed=false` → `TestPayReplayMoneyLog` red; F4a replayed answer always 201 → `TestReplayedPaymentIs200WithHeader` and `TestPayReplayAC19` red; F4b `user_id` neutralised in the `FindPayment` WHERE → `TestPayKeyIsPerUserAC21` red (user_b gets user_a's payment, 200 replayed); F4c AC-22: validation skipped → `TestPayRejectedKeyIsNotStoredAC22` red (500 on the first assertion), which proves only that the 422 exists: "the rejected key is not stored" is structural (validation runs before any database access), so no intact-code mutation targets it. The step-9 conflict path is a last guard that no intact-code test reaches; it is proven only by M2b, and its concurrent proof is P2.6.
- [x] **P2.4** Reconcile — AC-51 · INV-01–09 · TC5, TC19, TC20 · `go` · **critical** (reconciliation)
  - `internal/reconcile` (§9 checks 1–10, one `REPEATABLE READ READ ONLY` tx, JSON lines, exit 0/1/2), real
    `cmd/reconcile`, test helper `assertReconciled(t)`; CI `smoke` adds one curl payment and
    `docker compose exec -T api /app/reconcile`.
  - Done when: clean data → exit 0 with liability; for each check a break in the live test schema (guard dropped where
    needed) → exit 1 naming that check; row counts unchanged after every run.
  - Result: `internal/reconcile` (`Run` in one `REPEATABLE READ READ ONLY` tx, always rolled back; nine checks as a table of `{name, inv, sql}` returning violation counts, then a `liability` line and a summary; `Command` maps exit 0/1/2, stderr without SQL or credentials) and the real `cmd/reconcile` (config, `boot.Connect` with a 5 s wait, no migrate or seed). `assertReconciled` replaces `assertBooks` in all integration tests. Tests: clean books (every line, liability 10000 of 10000000), empty books, 25 breaks in the live test schema covering checks 1-9 including the redemption side by SQL-inserted rows (guards dropped and re-added, ledger trigger disabled, never the migration), row counts and every row's values (balances, earnings, ledger, payments, redemptions, campaign) unchanged after every run (the read-only transaction mode itself is untested), Command exit 0 / 1 / 2 (no secret in stderr; a query failing inside the snapshot, `fc_campaign_day` renamed, gives exit 2, check name and SQLSTATE 42883, no SQL, nothing on stdout). Check 6 also counts ledger entries with neither payment nor redemption. Mutations red (this run): each `OR` branch alone: delete `c.spent > c.budget OR` → `2 budget below spent`; delete `OR u.earned > u.daily_cap` → `3 cap below earned`; delete the payment award-match clause of check 6 → `6 award entry of another user`; `FULL OUTER JOIN` → `LEFT JOIN` in check 1 → `1 balance without ledger rows`; delete the orphan count → `6 entry with neither payment nor redemption`. `SELECT 0` for checks 1, 2, 3, 4, 5, 6, 8 each → red in their own break tests (check 1 and 8 also in `TestReconcileCommand`, 8 in the query-failure case). Earlier run: check 7 and check 9 `SELECT 0`, liability +1 → clean test, exit 1 mapped to 0 → Command test. CI `smoke` posts one payment and runs `reconcile`.
- [x] **P2.5** Race: budget and cap — AC-25, AC-26 · INV-02, INV-03 · TC1, TC2, TC14 · `go` · **critical**
  - **Mutations:** AC-25: campaign read without `FOR NO KEY UPDATE`; then also drop `campaigns_spent_within_budget`.
    AC-26: remove the campaign and user-day locks; then also drop `ude_earned_within_cap`.
  - Done when: `make test-race` exits 0; each mutation red and reported.
  - Result: test-only; `store.Pay` unchanged. `TestRacePayBudgetDrainAC25` (50 users x 100000 at once, budget 12000: 2 AWARDED 5000, 1 PARTIAL_BUDGET 2000, 47 CAMPAIGN_ENDED; spent 12000) and `TestRacePayDailyCapAC26` (user_a, 30 x 120000: 8 AWARDED 6000, 1 PARTIAL_DAILY_CAP 2000, 21 DAILY_CAP_REACHED; earned, balance, spent 50000). Each runs on a dedicated 60-connection pool, warmed, with one goroutine per request released by a closed channel; goroutines only record, the test asserts: non-201 replies are reported with `t.Errorf` (count plus first three bodies), then the exact tally of the 201 replies, DB totals, `assertReconciled` always run. `payRouterOn(pool, w)` and `doPayment` (no `t`) were split out of `payments_test.go`. Mutations red (this run): AC-25 campaign read without `FOR NO KEY UPDATE` -> 48 of 50 500s, and the tally and totals lines also print; plus `campaigns_spent_within_budget` dropped -> 50 x AWARDED, spent 250000, reconcile INV-02; AC-26 both locks removed -> 500s; plus `ude_earned_within_cap` dropped -> 30 x AWARDED, earned 180000, reconcile INV-03. `make test-race` (-count=20) exit 0.
- [x] **P2.6** Race: same key, lock timeout, disconnect — AC-23, AC-24, AC-27, AC-70 · INV-05, INV-06 · TC3 · `go` ·
  **critical**
  - **Mutations:** AC-23/24: drop `payments_user_key_unique` and the step-5 lookup. AC-27: remove `SET LOCAL
    lock_timeout`. AC-70: request context instead of `WithoutCancel` (resend is 201, not 200). AC-70 waits on
    `pg_stat_activity` `wait_event_type = 'Lock'` before cancelling.
  - Done when: `make test-race` exits 0; each mutation red and reported.
  - Result: test-only; `store.Pay`, `InTx` and the service unchanged. `TestRacePaySameKeyAC23` (20 x one key and body at once: one 201, nineteen 200 with `Idempotent-Replayed: true` and a byte-equal body; payments 1, ledger 1, balance 5000), `TestRacePaySameKeyTwoBodiesAC24` (10 x 100000 and 10 x 50000 interleaved: one 201, same-body 200 replays, other-body 409 `IDEMPOTENCY_KEY_REUSED`, one row), `TestRacePayLockTimeoutAC27` (campaign row held, `LockTimeoutMS` 300: 503 `SERVICE_BUSY` under 1 s, nothing written, the same key then 201), `TestRacePayClientDisconnectAC70` (real server and client; waits on `pg_stat_activity` `wait_event_type = 'Lock'`, cancels, releases: one payment, resend 200 replayed with the stored id). Each ends in `assertReconciled`. Helpers: `payRouterTx`, `raceReq.key`, `holdCampaignLock`. Mutations, runs red / 20: step-5 block and `payments_user_key_unique` dropped (500s) 20; plus plain `ON CONFLICT DO NOTHING` (20 x 201, 20 payments, reconcile INV-05) 20; step-5 block alone stays green 20/20 (the P2.3 carry-over: step 9 replays and 409s concurrently); step-5 block gone and step-9 `ErrNoRows` branch returning an error 20; no `SET LOCAL lock_timeout` (201 after 2 s) 20; `WithTimeout(ctx, limit)` for `WithoutCancel` in `InTx` (no payment row) 20. Review: AC-70 poll keyed on `pg_blocking_pids` of the holder pid; count and resend checks use `t.Errorf` so `assertReconciled` always runs; `countInt` returns its error. M4 re-proved red.
- [x] **P2.7** Campaign day and durability — AC-13, AC-14, AC-52, AC-55 · INV-08 · TC6, TC10 · `go` · **critical**
  - Clock via `fc_now()` replacement (23:59:59 / 00:00:00 WIB); `TestRaceMidnight` (AC-14, held campaign row);
    restart = `boot.Run` again with `CAMPAIGN_BUDGET=1`; PG at a closed port → POST 500, uncached GET 500.
  - **Mutations:** AC-14: `created_at` from `clock_timestamp()`. AC-52: seed `ON CONFLICT DO UPDATE SET budget`.
  - Done when: `make test-race` exits 0; mutations reported.
  - Result: test-only; `store.Pay`, boot and the router unchanged. Helper `setClockFrom(t, at)` (`fc_now()` = `now()` plus an offset, so it is transaction start on a clock that moves from `at`) and `waitBlockedBy(holderPid)` (extracted from AC-70). `TestPayCampaignDayAC13` (user_c: 50000 AWARDED at 23:59:59 WIB, DAILY_CAP_REACHED `PAY-20261003-`, then at 00:00:00 WIB 5000 AWARDED `PAY-20261004-`; GET shows 2026-10-04, earned 5000, resets_at 2026-10-05T00:00:00+07:00, balance 55000), `TestRaceMidnightAC14` (payment started 23:59:59.4 WIB waits on the held campaign row past midnight and 1 s: 201, created_at before release and before midnight, campaign_day 2026-10-03, `PAY-20261003-`, earned 5000 on the old day, GET now 2026-10-04 earned 0), `TestPayRestartKeepsBudgetAC52` (`boot.Run` with budget 1: budget, spent, one row unchanged), `TestPostgresDownAC55` (dead pool: POST, GET /me/cashback, GET /campaign all 500 INTERNAL_ERROR, no address or SQL in the body; the log buffer has no SQL or `goroutine `; DB counts unchanged by construction, since the handler only touches the dead pool). Each ends in `assertReconciled`. Mutations red: `fc_campaign_day` with UTC -> AC-13 (cap still reached at 00:00 WIB) and AC-14; `created_at` from `fc_now() + (clock_timestamp() - now())` before the payment insert -> AC-14 created_at, day check and reconcile `campaign_day_matches_created_at`; seed `ON CONFLICT DO UPDATE SET budget` -> AC-52 (`campaigns_spent_within_budget` rejects the reseed, `boot.Run` errors); unknown errors mapped to 503 -> AC-55 all three (status check); a log line with SQL text added in `httpapi/errors.go` -> AC-55 log check. The counts check is not mutation-proved. The log never contained `127.0.0.1`.

**P2 gate:** `make gate` → exit 0 · `make test-race` → exit 0 · `docker compose up -d --build --wait` · curl `POST
/v1/payments` 100000 as user_a with a new key → 201, 5000 `AWARDED`; same key again → 200 + `Idempotent-Replayed:
true` · `docker compose exec api /app/reconcile; echo exit=$?` → exit=0 · `docker compose down` · CI green.

**P2 gate result (2026-10-05):** all steps pass. `make gate` exit 0; `make test-race` exit 0 (178 s); `up -d --build
--wait` exit 0; POST 100000 as user_a → 201, 5000 `AWARDED`; same key → 200, `Idempotent-Replayed: true`, same body;
reconcile exit 0 (INV-01–09 ok); `down` exit 0; CI run 37265718476 `gate` and `smoke` green on `fd6e862`.

## P3 — Redemption and history

- [x] **P3.1** `Redemptions.Redeem` and `POST /redemptions` — AC-29–32, AC-34, AC-35, AC-16 (redeem), AC-21 (redeem
  with K), AC-48 (redeem), AC-17/18 (no row) · INV-01, 04–06, 09 · TC3, TC4, TC13 (stub) · `go` · **critical**
  - §4.2 in the D46 order; replay incl. stored `balance_after`; payout stub; money log line (§7, `op` redeem). AC-32
    sets the flag by SQL until P4.1.
  - Done when: integration tests + reconcile; **mutation:** balance check before the paused check → AC-32 red.
  - Result: `store.Redeem` (§4.2 steps 4-10, D46 order: balance `FOR UPDATE`, step-5 key lookup, plain campaign read, paused before balance, debit, insert `ON CONFLICT DO NOTHING`, ledger `REDEMPTION`), `store.FindRedemption`, `service.Redemptions.Redeem` (fast replay while paused too, stored `balance_after` on replay, payout stub per TC13, money log `op` redeem after COMMIT), `POST /v1/redemptions` 201 / 200 + `Idempotent-Replayed: true` / 422 `INSUFFICIENT_BALANCE` / 409 `REDEMPTION_PAUSED`, wired in `cmd/api`; `ErrReplay` is shared by both operations. Integration tests for AC-16, 17/18, 21, 29-32, 34, 35, 48, a direct step-5 `store.Redeem` in `InTx`, and the log line, state built by payments (SQL only for the flag and the reseeded budget), each ending in `assertReconciled`; `seedBooks` now holds one real redemption (user_d). No cache delete (P4.3), no race tests (P3.2). Mutations red: balance check before paused check (AC-32, 422 instead of 409), paused check removed (AC-32, a 201 and a changed balance), hash compare skipped (AC-35, 200 instead of 409), replay with the current balance (AC-35, balance_after 5000 not 0), step-5 lookup removed (direct test, `ErrInsufficientBalance`); httpapi 409 to 422 and dropped replay header, red.
- [x] **P3.2** Race: redemptions — AC-33, AC-69, AC-28 · INV-01, 04, 05 · TC3, TC4, TC5 · `go` · **critical**
  - **Mutations:** AC-33: balance read without `FOR UPDATE`; then also drop `balances_nonneg`. AC-69: drop
    `redemptions_user_key_unique` and the step-5 lookup. AC-28: award locks the campaign `FOR UPDATE` → `40P01`.
  - Done when: `make test-race` exits 0; each mutation red and reported.
  - Result: test-only; `store.Redeem`, `store.Pay` and the services unchanged. `TestRaceRedeemOneBalanceAC33` (a test transaction holds user_a's balance row; two redeems of 18000 queue behind it, confirmed by `pg_blocking_pids`; one 201 with `balance_after` 0, one 422 `INSUFFICIENT_BALANCE`, balance 0, redemptions 1, ledger 2), `TestRaceRedeemSameKeyAC69` (10 x 1000, one key, at once: one 201, nine 200 `Idempotent-Replayed: true` with byte-equal bodies and `balance_after` 17000; redemptions 1, `REDEMPTION` ledger 1, balance 17000), `TestRaceAwardsAndRedemptionsAC28` (funded on 2026-10-02, raced on 2026-10-03: 20 payments of 100000 and 10 redeems of 1000 interleaved; no non-2xx, 10 x 5000 `AWARDED`, 10 x 0 `DAILY_CAP_REACHED`, 10 redemptions, balance 50000, earned 50000). Each ends in `assertReconciled`. Helpers: `allAtOnce`, `redeemAllAtOnce`, `holdRowLock`, `waitBlockedByN` (counts waiters queued behind a waiter, since the second row-lock waiter waits on the first, not the holder). Mutations, runs red / 20: AC-33 balance read without `FOR UPDATE` (the UPDATE still queues behind the holder; second answer 500 on a CHECK) 20; plus `balances_nonneg` dropped, still 500 via `ledger_balance_after_nonneg` 20; plus that dropped, still 500 via `redemptions_balance_after_nonneg` 20; all three dropped (two 201, balance -18000, reconcile INV-04) 20; AC-69 step-5 block and `redemptions_user_key_unique` dropped (500s, no 201) 20; plus plain `ON CONFLICT DO NOTHING` (10 x 201, 10 redemptions, balance 8000, reconcile INV-05) 20; step-5 block alone dropped stays green 20/20 (step 10 replays); step-5 block gone and step-10 `ErrNoRows` branch returning an error 20; AC-28 `Pay` campaign read `FOR UPDATE` (503 `SERVICE_BUSY`; Postgres log shows both deadlock and lock-timeout cancels) 20.
- [x] **P3.3** `GET /me/history` — AC-47, AC-02 / AC-15 (history), AC-16, AC-48, AC-49 (all endpoints, every state) ·
  INV-10 · TC12, TC22 · `go` · not critical
  - `history.Newest`: `UNION ALL`, `LIMIT` per branch, `created_at DESC, id DESC`.
  - Done when: integration tests; AC-49 compares `GET /campaign` at budget left 2000 and 9000000 byte for byte.
  - Carried from P1.3: AC-48 cross-user for `GET /me/cashback` after a redemption (user_b sees none of user_a's).
  - Result: `store.Newest` (one statement: `UNION ALL` of two branches, each `ORDER BY created_at DESC, id DESC LIMIT $2` on its `(user_id, created_at DESC, id DESC)` index, then the same order and limit outside; columns of the other table are `NULL` and scan into pointers), `Reads.History` (items always non-nil, so empty is `{"items":[]}`), `HistoryView` / `HistoryItem` in `internal/domain`, `HistoryReader` now returns the view, `GET /v1/me/history` wired in `cmd/api` with the same `Reads` value. Integration: AC-47 exact item shapes in order (cashback on payments only, destination on redemptions only, WIB `created_at`, `PAY-` / `RDM-` references) and 30 + 30 interleaved rows give the 20 newest by default, 50 at `limit=50`, 1 at `limit=1`; five payments at one instant list id 5 to 1; AC-02 the 19999 payment listed with 0 `BELOW_MINIMUM`; AC-15 history keeps 47000 `AWARDED` after the rule change; AC-16 body exactly `{"items":[]}`; AC-48 user_b sees nothing in `/me/history` and `/me/cashback` after user_a pays and redeems; AC-49 no `budget|spent` key in campaign, cashback, history, payment and redemption replies for active, both paused, partially spent and ended (budget reseeded to `spent + 2000` / `spent`; the byte compare at 2000 and 9000000 is `TestCampaignBodyIndependentOfBudget`). `noBudgetKey` now walks arrays. Mutations red: outer order ASC (AC-47 order and limit), `user_id` removed from the redemptions branch (AC-48), items left nil (AC-16 and AC-48), `omitempty` dropped on `cashback` (shape). Not behaviour-observable: per-branch `LIMIT` removed stays green (performance only); `id DESC` dropped from the outer order, and from all three orders, stays green because the index scan already returns the larger id first; with all three dropped and `payments_user_newest` also dropped on the live test schema (`docker compose -f docker-compose.test.yml exec postgres psql`), the tie test is red 5/5 (`ids = [1 2 3 4 5]`), and the intact code without the index is green 5/5, so the order comes from the `id DESC` sort; index re-created.

**P3 gate:** P2 gate commands, plus curl `POST /v1/redemptions` 1000 as user_a → 201 with `balance_after` · `GET
/v1/me/history` lists both, newest first · reconcile → exit=0.

**P3 gate result (2026-10-05):** all steps pass. `make gate` exit 0; `make test-race` exit 0 (230 s); `up -d --build
--wait` exit 0; POST 100000 as user_a → 201, 5000 `AWARDED`; same key → 200, `Idempotent-Replayed: true`, same body;
POST /redemptions 1000 → 201, `balance_after` 14000; `GET /me/history` lists the redemption, then the payments,
newest first; reconcile exit 0 (INV-01–09 ok, liability 14000); `down` exit 0; CI run 37272712560 `gate` and `smoke`
green on `cebfe64`.

## P4 — Operations tools, then Redis invalidation, then read caches

- [x] **P4.1** Switch commands — AC-36, AC-37, AC-40, AC-12 / AC-32 / AC-41 via the command · TC7 (base) · `go` ·
  **critical** (campaign-row lock)
  - `cmd/admin` four commands per §4.3 (`--by` required → exit 2; lock wait 5 s → exit 1 "busy, retry"; one JSON line
    after commit); `service.Switches`.
  - Done when: integration tests run the command entry point and read its stdout; `docker compose exec api /app/admin
    pause-awards --by owner` prints the line with `changed` true, then false on a second run.
  - Result: `store.SetSwitch` (`SELECT <flag>, now() ... FOR NO KEY UPDATE`, then `UPDATE` only when the value differs; the column comes from the closed `domain.Switch` enum, two fixed statements), `service.Switches.Set` (through `TxRunner.InTx`, returns after COMMIT), and `internal/admin.Command` (arg parsing before any connection, lock timeout 5 s by default with statement timeout and cap 10 s above it, one JSON line after commit; exit 0 / 1 / 2, "busy, retry" on `ErrBusy`) behind the real `cmd/admin`. Tests: unit `TestParse` and usage-before-connect; integration `TestSwitchCommandLineAC40` (all four commands, exact fields, second run `changed` false with `updated_at` kept), `TestSwitchCommandUsageChangesNothingAC40`, `TestPauseAwardsAC36`, `TestPauseRedemptionsAC37`, `TestSwitchCommandBusy`; AC-12, AC-32 / AC-35 and AC-41 rows 2, 4 and 5 (`TestCampaignStatusAfterPayments`, `TestCampaignStatusRows`) now set the flag through the command; both end in `assertReconciled`. Review fixes (`plans/reviews/P4.1-code-check.md`): F1 `TestRaceSwitchWaitsForOpenChange` (an open change holds the switch; the command waits, then prints old true / changed false and keeps the holder's `updated_at`; runs in `make test-race`); F2 all AC-41 paused rows go through the command; F3 reconcile added to `TestCampaignStatusRows`; F4 a config failure in `cmd/admin` exits 1, not 2; F5 unit tests for the connect-failure exit (exit 1, empty stdout, no URL or password on stderr) and `failureMessage` (busy / outcome unknown / fixed text without driver text). No cache delete yet (P4.3). Mutations red: awards column swapped (AC-36 and AC-40 first line), `changed` always true (AC-40 second line), `--by` checked after the transaction (flags changed on a usage error), lock timeout ignored (busy test: exit 0, 3 s, flag changed), unchanged switch still updated (`updated_at` rewritten), `--by` check removed (unit parse red); F1 `FOR NO KEY UPDATE` removed from both switch SELECTs (race test red 20 of 20 runs: `old:false, changed:true`); F5 connect failure exiting 2 and printing the URL (connect test red), driver text passed through and wrong unknown-outcome text (`TestFailureMessage` red). Compose: `docker compose exec api /app/admin pause-awards --by owner` → `changed` true; again → `changed` false; `resume-awards` → `changed` true; no `--by` → exit 2; reconcile exit 0. The line after commit is structural: it is printed only after `Set` returns.
- [x] **P4.2** Race: pause in flight — AC-38, AC-39 (held, waiting, and 40-user cases) · TC7 · `go` · **critical**
  - Test hooks nil in production (§11); "began after" = `created_at` later than DB `clock_timestamp()` read after exit.
  - **Mutations:** switch SELECT `FOR UPDATE` instead of `FOR NO KEY UPDATE` (an open payment or redemption insert
    holding `FOR KEY SHARE` must not block `pause-*`). AC-38: `Pay` campaign read without `FOR NO KEY UPDATE`. AC-39: remove the paused check in
    `Redeem`; separately `FOR SHARE` on the step-6 read; separately the flag read before the balance lock (D44 order).
  - Done when: `make test-race` exits 0; each mutation red and reported.
  - Result: `store.Hooks` (`AfterCampaignRead`, `RedeemHeld`; zero value in `cmd/api` and `cmd/admin`, `grep Hooks backend/cmd` empty) on `TxRunner`, copied by `service.Payments` / `service.Redemptions` into `PayInput.AfterCampaignRead` / `RedeemInput.Held`; `Pay` calls its hook after the step-5 lookup, `Redeem` after the ledger insert, before COMMIT. `race_switches_test.go`: `TestRacePauseAwardsAmongPaymentsAC38` (20 payments, the first held after its locked campaign read; `pause-awards` is seen waiting behind it through the lock-wait chain, which ends at a backend idle in its transaction, for 150 ms; the gate then opens, 20 more follow; paid rows by id are awarded then only paused, all later ones paused; the 5 s timer is only the fallback), `TestRacePauseRedemptionsHeldAC39` (`pause-redemptions` exits 0 while a redemption is open; the next is 409; the open one is 201 with `balance_after` 4000), `TestRacePauseRedemptionsWaitingAC39` (a redemption waiting on the balance lock is 409 after the pause), `TestRacePauseRedemptionsAmongUsersAC39` (40 users: 20 held after their writes, 20 waiting on their balance lock; the pause runs from the test goroutine between them; exactly 20 x 201 with balance 4000 and one row, 20 x 409 `REDEMPTION_PAUSED` with balance 5000 and no row, no 201 created after the command returned); all end in `assertReconciled`. `racePool` split into `warmedPool`. Mutations red, each 20 of 20 runs: `Pay` campaign read without `FOR NO KEY UPDATE` (AC-38: the pause never waits, and an AWARDED row follows the paused ones); paused check removed in `Redeem` (Held, Waiting and AmongUsers); the flag read before the balance lock (Waiting: 201, balance 4000; AmongUsers); `FOR SHARE` on the step-6 read and the redemption switch SELECT `FOR UPDATE` (Held 20 of 20; AmongUsers red in all 18 runs that finished inside the 110 s test limit, each run waits for the 5 s lock timeout; the hook after the writes holds `FOR KEY SHARE`). `make gate` exit 0; `make test-race` exit 0 (306 s).
- [x] **P4.3** Invalidation after commit; `demo-reset`; harness Redis reset — AC-57 · TC9 (deletes), TC10 (demo
  exception) · `go` · **critical** (Pay/Redeem after-commit path; truncates money tables)
  - §6 keys deleted on a fresh 50 ms context after commit in Pay (cashback; campaign when spent = budget), Redeem,
    switches; `admin demo-reset` refuses without `FC_DEMO=1` (exit 2), builds state through the services, deletes keys.
    Harness: reset runs `FLUSHDB` on the test Redis; the SQL flag and budget helpers delete `fc:v1:campaign`. No reads
    use the cache yet, so no test can see a stale value.
  - Done when: integration tests set each key in Redis, run the write, and see the key gone; Redis at a closed port →
    writes still 201; reconcile → exit 0 after demo-reset.
  - Result: `internal/cache` `Invalidator` (delete-only, INV-11; `Delete` on `WithoutCancel` plus the Redis timeout, a failure is a warn log with the keys and no address; `DeleteAll` SCAN `fc:v1:*` + DEL, 2 s cap, demo-reset only), `CampaignKey`, `CashbackKey`. `store.PayOutcome.Exhausted` (`awarded > 0 && spent + awarded == budget` from the locked read); `Payments` deletes the user's cashback key after COMMIT and the campaign key when exhausted, `Redemptions` the cashback key; replay, 409, 422, paused and busy delete nothing; an unknown COMMIT outcome (`store.ErrUnknownOutcome`) deletes the cashback key only in Pay (the campaign key not: `Exhausted` is unknown) and in Redeem (§6). Switch commands delete the campaign key after the printed line, also when `changed` is false. `admin demo-reset` (no flags; without `FC_DEMO=1` exit 2 before any connection): `store.DemoTruncate` (five tables `RESTART IDENTITY`, spent 0, both switches off, budget kept, one transaction), `service.Demo.Reset` (user_a 940000 and redeem 32000, user_c 1000000, through the services; anything else exit 1), then `DeleteAll` on every path once the truncate has committed, also when a step fails (Redis failure: a warning, exit 0), one `demo_reset` JSON line; the failure text says to run demo-reset again, a deadlock (40P01) reaches it as `ErrBusy` and prints "busy, retry" (unit `TestDemoFailureMessage`). Harness: `reset` runs `FLUSHDB`; `setCampaignSQL` (SQL plus delete of the campaign key) at the flag and budget state-setting sites; routers and `runAdmin` use an invalidator on the test Redis, `deadInvalidator` a closed port. Tests: `cache_invalidation_test.go` (user key only, last of budget, no award, replay and 409, request context cancelled mid-transaction, redeem 201 / 422 / replay / 409 / paused, every switch command, Redis at a closed port, warn without the address), `demo_test.go` (AC-57 refusal changes nothing, demo state from a dirty one twice with ids restarting and no keys left, Redis down), unit `TestParseDemoReset`, `TestDemoResetRefusesBeforeConnecting`. Mutations red: Pay delete removed, `Exhausted` always true / never true, request context instead of `WithoutCancel`, Redeem delete removed, Redeem delete whatever the outcome (422 and paused), Pay delete before the replay check, switch delete removed, demo check removed (integration and unit), `DeleteAll` skipped, no `RESTART IDENTITY`, spent not reset (also reconcile INV-02), Redis-down warning dropped, log carrying the driver error, `Delete` panicking on error. Compose: `demo-reset` exit 2 without `FC_DEMO`, exit 0 with it, `/me/cashback` user_a balance 15000 earned 47000, reconcile exit 0. Review round (F1-F10): harness `rdb` is built with `cache.NewClient` (500 ms timeout), the rule-changing SQL in `payments_test.go` goes through `setCampaignSQL`; new tests `TestDemoResetStateNotReachedAC57` (budget 50000: exit 1, fixed text, empty stdout, no key left), the demo-state test seeds budget 7654321 and expects it kept, `TestPayDeletesCashbackKeyWhenCommitOutcomeUnknown` and `TestRedeemDeletesCashbackKeyWhenCommitOutcomeUnknown` (a deferred constraint trigger on `payments` / `redemptions` makes COMMIT fail through the service). Mutations red: `DemoTruncate` resets the budget; `pay` accepts any award; `DeleteAll` only on success; the ErrUnknownOutcome branch removed in Pay and in Redeem. Mutation green and left so: "Pay delete after `InTx` whatever the outcome" (replay and 409 return at the step-2 fast path before the transaction, and an extra delete is harmless; this is not proved by a test). Assumption: demo-reset runs with the app idle and is demo only; a deadlock against live traffic aborts one side. Deferred to P4.4 (F8): go-redis's own logger still writes failed dials, with the Redis address, to stderr. `make gate` exit 0; `make test-race` exit 0.
- [x] **P4.4** Read caches — AC-42, AC-43, AC-44, AC-45, AC-46, AC-50 (Redis states) · INV-11 · TC8, TC9 · `go` · not
  critical (no money path; write services hold no cache client)
  - `internal/cache` (§6: 50 ms, `MaxRetries: -1`, `ContextTimeoutEnabled`, bad value = miss, warn once per 10 s);
    `Reads` use it; cashback TTL capped at `resets_at` − `fc_now()`, not cached under 1 s.
  - Done when: integration tests for AC-42 (warm cache, pay, redeem, pause), Redis at a closed port, `CLIENT PAUSE 500
    ALL` (read ≤ ~100 ms), invalid JSON, balance 999999 cached then redeem 20000 → 422, clock at 23:59:30 → TTL ≤ 30 s;
    **mutation:** remove the P4.3 delete in Pay → AC-42 red.
  - Also (D51): `CACHE_READS` in `internal/config` (`on`/`off`, else rejected) and `.env.example`; `Reads` skips the
    cache when `off` (AC-76). Test: with `off`, both GETs answer correctly and `KEYS fc:v1:*` stays empty; a Pay still
    deletes a key set by hand; **mutation:** ignore the flag → AC-76 red. Carried from P4.3 (F8): silence go-redis's
    own logger so a failed dial never prints the Redis address.
  - Result: `internal/cache` `ReadCache` (read side, a separate type from `Invalidator`; GET on a 50 ms context, SET on `WithoutCancel` plus the same deadline; any Redis error, `redis.Nil` or invalid value is a miss; a hit is decoded with `DisallowUnknownFields`, one value only, then checked (status enums, amounts >= 0, `date`, `resets_at`); warn at most once per 10 s per op `get`, `set`, `decode` with the key and no address, injectable clock). `NewReadCache` returns nil when `CACHE_READS=off`, so `service.NewReads(pool, nil)` is the off mode (AC-76) and `main.go` needs no branch. `cashbackTTL` = min(`CACHE_CASHBACK_TTL`, `ResetsAt - Now`), not stored under 1 s; `store.Today` returns `fc_now()` as `TodayRow.Now` in the same statement. `NewClient` silences go-redis's logger once (F8 closed: a child-process test dials a closed port and finds no address on stderr). `config` `CACHE_READS` (`on` default, `off`, else `config CACHE_READS: "x" is not one of on, off`), `.env.example`. Tests: `cache_reads_test.go` (planted body served, fill with TTLs, AC-42, AC-43, AC-44 twice, AC-45 table of eight bad values plus 999999 then redeem 20000 -> 422, AC-46 at 23:59:30 and 23:59:59.5, AC-76, throttled warnings without the address), unit `TestThrottle...`, `TestDecodeCampaign`, `TestDecodeCashback`, `TestCashbackTTL`, `TestNewClientSilencesDriverLogger`, config tests. Existing routers (`readsRouter`, `payRouterOn`) pass a nil cache. Mutations red: P4.3 cashback delete removed in Pay (AC-42), flag ignored (AC-76), no `DisallowUnknownFields`, no validation, lax decode, TTL not capped (AC-46), `ContextTimeoutEnabled: false` (second AC-44 test), timeouts stretched (first AC-44 test), `SetLogger` removed, throttle removed (unit and integration), cache never read, a panic on a failed SET (AC-43). Not provable: `MaxRetries: 0` stays green, the context deadline already bounds the retries. Compose: second read of user_a leaves `fc:v1:cashback:user_a`, a payment then a read shows 18000, with Redis stopped reads stay 200 in about 104 ms before the review fixes and about 53 ms after (the SET is skipped after a failed GET) and the api log has no `redis:6379`, reconcile exit 0.  Review fixes (plans/reviews/P4.4-code-check.md): F1 `ReadCache.Campaign/Cashback` return `Lookup` (`Hit`, `Miss`, `Unavailable`) and `service.Reads` SETs only after a `Miss`, so a read pays one Redis deadline; AC-44 bounds 90 ms (not 120: two deadlines measure ~105 ms, 120 would not catch them), throttle test `set` warnings 0; F2 `SetCashback` takes the time before `store.Today` and `cashbackTTL` subtracts the elapsed time; F3 `TestReadErrorsAreNeverCached` (campaign row gone, both GETs non-200, no keys) replaces the invalid-header check; F4 decode through pointer mirror structs, a missing field is a miss (unit rows, AC-45 rows); F5 a GET failing with `context.Canceled` while the request context is done is a silent `Unavailable` (`TestCancelledRequestIsASilentMissNoWarn`). Mutations red: always SET (both AC-44 tests and the throttle test), `SetCampaign` / `SetCashback` before the error check, cancel check removed, `earned` / `remaining` nil checks dropped (unit, as a nil panic), elapsed ignored in `cashbackTTL`. `make gate` exit 0; `make test-race` exit 0.
- [x] **P4.5** Load test — AC-77 · D51 · TC8 · `go` (Makefile, compose), k6 JS · not critical (no app code)
  - `loadtest/campaign.js`, `cashback.js`, `mixed.js`, a shared `lib.js` (§9: `constant-arrival-rate`, thresholds on
    5xx, 503 counted); compose service `k6` (`grafana/k6:<pinned>`, profile `loadtest`, `./loadtest` read-only);
    root `docker-compose.yml` passes `CACHE_READS: ${CACHE_READS:-on}`; `make load-test` per §9 (both modes,
    demo-reset before, reconcile after, one printed table); `loadtest/results/` in `.gitignore`.
  - Install: none on the host; `docker pull grafana/k6:<pinned>` happens on first run.
  - Done when: `make load-test; echo exit=$?` → exit=0 with the table printed for both modes; reconcile exit 0 after
    each; `docker compose up -d --wait` alone does not start `k6`; `docker compose stop api` during a run makes the
    target exit nonzero (threshold breach shown, then `start api`).
    The numbers are recorded in this task's Result; the README copy is P6.1.
  - Result: compose `k6` (`grafana/k6:2.3.0`, profile `loadtest`, no `depends_on`, `./loadtest` read-only, `./loadtest/results` rw, `user` from `LOADTEST_UID/GID`), api `CACHE_READS: ${CACHE_READS:-on}`; `loadtest/lib.js` (rates, `constant-arrival-rate`, counters `read_fail` / `pay_fail` / `pay_503`, thresholds `count==0`, `handleSummary` writes `/results/<mode>-<scenario>.json` and one table row), `campaign.js`, `cashback.js`, `mixed.js` (payments of 100000 from `lt_p001`..`lt_p200`, v4 UUID from `crypto.getRandomValues`), `run.sh` (both modes, `demo-reset` before, reconcile after, nonzero k6 or reconcile fails and prints the table so far, api restored to `on`), `make load-test`, `loadtest/results/` ignored. `make gate` exit 0; `up -d --build --wait` starts no `k6`; `make load-test` exit 0, reconcile exit 0 after each mode; `docker compose stop api` during `on campaign` -> `read_fail` threshold crossed, target exit 2, then `start`, reconcile exit 0. Machine: Apple M2, 16 GiB, Docker 29.1.3 (Docker Desktop VM 8 CPUs, 7.7 GiB); k6, api, PostgreSQL and Redis share it. Rates: campaign 4000/s, cashback 4000/s, mixed 2000/s each read + 20 pays/s, 30 s each (200/s then 2000/s did not make `off` busy; 4000/s drops some iterations, so the rate is not fully reached; see warnings). Last run (ms, p50 / p95 / p99):
    ```
    scenario      mode   p50 ms   p95 ms   p99 ms    req/s  errors   503s
    campaign      on        0.3      0.6      3.9   3998.2       0      -
    cashback      on        0.3      1.0      8.6   3984.0       0      -
    mixed-read    on        0.2      1.4     21.2   3935.3       0      -
    mixed-pay     on        2.3     18.3    272.9     20.0       0      0
    campaign      off       0.2      1.0     12.2   3945.0       0      -
    cashback      off       0.3      0.7      5.1   3997.3       0      -
    mixed-read    off       0.4      0.8      2.3   4000.1       0      -
    mixed-pay     off       3.1      5.9     13.8     20.0       0      0
    ```
    Plainly: on this machine `on` shows no reliable gain. The reads are single-row queries (sub-millisecond either way), the load generator and the stack compete for the same 8 CPUs, and run-to-run noise (an earlier run had mixed-pay p95 5.0 `on` vs 14.3 `off`, this one the reverse) is larger than the difference. No payment saw a 503 in either mode, so D06's `SERVICE_BUSY` claim is not shown by this load. Assumptions: transport errors (status 0) fail the run like a 5xx; `dropped_iterations` is a warning only; api is left in `CACHE_READS=on`.
  - Removed (D52, 2026-10-05): the owner dropped the load test ("the numbers don't prove anything"); `loadtest/`,
    the compose `k6` service and `make load-test` are deleted, `CACHE_READS` stays. This Result is kept as the one-off
    measurement.
- [x] **P4.6** Campaign cache only — AC-42, AC-45, AC-76 (amended), AC-46 (removed) · INV-11 · TC8, TC9 · D53 · `go` ·
  **critical** (Pay/Redeem after-commit path)
  - Remove the cashback read cache: `ReadCache.Cashback` / `SetCashback`, `cashbackTTL`, `TodayRow.Now` if nothing else
    reads it, `CashbackKey`, `CACHE_CASHBACK_TTL` (`internal/config`, `.env.example`, `docker-compose.yml`); `Reads`
    serves `GET /me/cashback` from PostgreSQL only. Pay deletes only `fc:v1:campaign` when exhausted (nothing on an
    unknown COMMIT outcome); Redeem deletes nothing and holds no `Invalidator`. Switch commands and `demo-reset` unchanged.
  - Tests first: AC-42 amended (cashback reads fresh after pay and redeem with no `fc:v1:cashback:*` key written;
    campaign `PAUSED` after `pause-awards`, `ENDED` after the last-of-budget award, cache warm); AC-45 stale key
    999999 planted for user_a → `GET /me/cashback` 18000 and redeem 20000 → 422; AC-76 campaign only. Delete the
    AC-46 tests and the cashback-key invalidation tests, and say which tests were removed in the Result.
  - **Mutations:** `Reads` reads `fc:v1:cashback:{user}` again → AC-45 red; Pay campaign delete removed → AC-42
    `ENDED` red; flag ignored → AC-76 red.
  - Done when: `make gate` and `make test-race` exit 0; mutations reported; compose: after a read of user_a, `KEYS
    fc:v1:*` shows only `fc:v1:campaign`; reconcile exit 0.
  - Result: removed `ReadCache.Cashback` / `SetCashback`, `cashbackTTL`, `minCashbackTTL`, the cashback mirror and
    decoder, `cache.CashbackKey`, `TodayRow.Now` (nothing else read it; the `now` column is gone from the `Today` CTE, and the day still comes from `fc_campaign_day(fc_now())`),
    `CACHE_CASHBACK_TTL` (`config`, `.env.example`, `docker-compose.yml`). `Reads.Cashback` is PostgreSQL only. Pay
    deletes `fc:v1:campaign` after COMMIT when `Exhausted` and nothing else; an unknown COMMIT outcome deletes nothing.
    `Redemptions` has no `Invalidator` (`NewRedemptions(tx, log)`) and deletes nothing. Switch commands, `demo-reset`
    and `CACHE_READS` (campaign only) unchanged. Tests added or amended: `TestWarmCacheFollowsWritesAC42` (no key
    after cashback reads, `PAUSED`), `TestWarmCampaignShowsEndedAfterTheLastOfTheBudgetAC42`,
    `TestCampaignMissFillsTheKeyWithItsTTL`, `TestInvalidCachedCampaignIsAMissAC45`,
    `TestStaleCashbackKeyIsNeverReadAC45` (999999 planted, 18000 shown, redeem 20000 -> 422),
    `TestCacheReadsOffAC76` (campaign only), `TestPayDeletesNoKeyWhileTheBudgetLasts`,
    `TestPayThatTakesTheLastOfTheBudgetDeletesOnlyTheCampaignKey`, `TestPayWithNoAwardDeletesNoKey`,
    `TestRedeemDeletesNoKey`, `TestPayDeletesNothingWhenCommitOutcomeUnknown`,
    `TestRedeemDeletesNothingWhenCommitOutcomeUnknown`; `TestPayDeleteSurvivesCancelledRequestContext` now uses the
    exhausted campaign key. Tests removed: `TestCashbackTTLEndsWithTheCampaignDayAC46` (AC-46),
    `TestReadsServeAValidCachedBody` and `TestReadMissFillsTheKeyWithItsTTL` (replaced by campaign-only versions),
    `TestInvalidCachedValueIsAMissAC45` and `TestStaleCachedBalanceNeverDecidesARedemptionAC45` (replaced),
    `TestPayDeletesOnlyThatUsersCashbackKey`, `TestPayWithNoAwardStillDeletesCashbackKeyOnly`,
    `TestRedeemDeletesCashbackKeyOnlyWhenItCommits`, `TestPayDeletesCashbackKeyWhenCommitOutcomeUnknown`,
    `TestRedeemDeletesCashbackKeyWhenCommitOutcomeUnknown`, unit `TestDecodeCashback`, `TestCashbackTTL`, and the
    `CACHE_CASHBACK_TTL` config rows. Mutations red: `Reads` reads `fc:v1:cashback:{user}` (AC-45 stale key test, 999999
    vs 18000); Pay campaign delete removed (AC-42 `ENDED`, last-of-budget delete, cancelled-context, AC-76 delete);
    flag ignored (AC-76, key written with the cache off). Compose: `/me/cashback` user_a 15000, `/campaign` ACTIVE,
    `KEYS fc:v1:*` only `fc:v1:campaign`, reconcile exit 0. `make gate` exit 0; `make test-race` exit 0.

**P4 gate:** `make gate`, `make test-race` → exit 0 · stack up · `docker compose exec -e FC_DEMO=1 api /app/admin
demo-reset` → exit 0 · `docker compose stop redis`; healthz → 200, `status` ok, redis `degraded`; `GET /me/cashback`
as user_a → 15000; `docker compose start redis` · pause/resume both switches with `--by` · reconcile → exit=0 ·
`down` · CI green.

**P4 gate result (2026-10-05):** all steps pass. `make gate` exit 0; `make test-race` exit 0; `up -d --build --wait`
exit 0; `demo-reset` exit 0; Redis stopped → healthz 200, `status` ok, redis `degraded`, `GET /me/cashback` user_a
balance 15000; Redis started → healthz redis ok; pause/resume awards and redemptions with `--by owner` each exit 0,
`changed` true; reconcile exit 0 (INV-01–09 ok, liability 65000); `down` exit 0; CI run 37300465581 `gate` and
`smoke` green on `200cd71`. The load-test step was removed by D52.

## P5 — Mobile app

- [x] **P5.1** Expo scaffold, API client, formatting, user — AC-67, AC-68 · `ts` · not critical
  - Install: `npx create-expo-app@<pinned> mobile` (TypeScript, Expo Router), delete its `AGENTS.md`, `CLAUDE.md`,
    `.claude/`, reset script; `npx expo install @tanstack/react-query @react-native-async-storage/async-storage
    expo-crypto`; `npm i -D jest-expo @testing-library/react-native @types/jest eslint-config-expo` (lockfile
    committed). Scripts `lint`, `typecheck` (`tsc --noEmit`, `"types": ["jest"]`), `test`; `make mobile-check` runs
    them; CI job `mobile`. `src/api/client.ts` (10 s abort = unknown), `queries.ts`, `money/format.ts`,
    `copy/codes.ts`, `user/`.
  - Done when: `make mobile-check` exits 0 (formatter, classification, base URL, user kept after remount); a
    deliberate lint error makes the stop check exit 2.
  - Result: `make mobile-check` (74 tests), `make gate` exit 0; expo-doctor 21/21; stop check exit 2 on a lint error, 0 after; Expo SDK 57 (expo 57.0.26, create-expo-app 5.0.0), react-native 0.86.3, TypeScript 6.0.3, jest-expo 57.0.5, RNTL 14.0.1, @types/jest 29.5.14 (expo's pin), eslint 9.39.5; CI job `mobile` added; the template's `src/app` kept as the router root.
- [x] **P5.2** Read screens: Home, History, How it works — AC-64, AC-66, AC-67 (picker), AC-75 · US-6 · `ts` · not
  critical
  - Done when: RNTL tests (awaited) cover every AC-64 state; AC-66 runs with `TZ=UTC`; AC-75 serves `rules` as 1000
    bps / 30000 / 70000 and screen 7 shows 10%, Rp30.000, Rp70.000.
  - Result: Home, History, and How it works built with RNTL tests (AC-64 states, AC-66 in UTC via a Jest global setup,
    AC-67 picker, AC-75 at 1000 bps / Rp30.000 / Rp70.000); seven mutations red and restored.
- [x] **P5.3** Money attempt machine and saved attempts — AC-59, AC-60, AC-61, AC-72, AC-73, AC-74 (logic) · TC21 ·
  `ts` · **critical** (client idempotency)
  - `src/attempts/` per §10: ref guard, key from expo-crypto per press, save to `fc:attempts` before send, remove on
    2xx/4xx only, resend x3 ~2 s then wait, launch check (< 10 min resend with saved user; older → card data).
  - Done when: Jest (fake timers) green; **mutations:** remove the ref guard → AC-59 red; new key per send → AC-60 red;
    send before save → AC-72 red; selected user instead of saved → AC-73 red.
  - Result: `src/attempts/` (types, store, launch, machine, AttemptProvider) wired into `_layout.tsx`; 43 new Jest
    tests (fake timers); four mutations red and restored (ref guard, new key per send, send before save, selected user
    instead of saved). Review fixes: after a 4xx whose removal fails twice, a `fc:attempts:resolved` marker keeps the
    launch check from resending the key; dismiss only removes a card; a save error removes the key best-effort; ten
    more mutations red. AC-73 "Checking opens" proved at provider level, the screen opening is asserted in P5.4.
    Assumption (owner-accepted deviation from §10): a launch resend that ends in `waiting` stops the launch queue; the
    remaining recent attempts become unconfirmed cards, with their keys kept.
- [x] **P5.4** Pay, Payment result, Checking, unconfirmed card — AC-58, AC-60, AC-61 (UI), AC-62, AC-63, AC-74 (UI) ·
  TC21 · `ts` · **critical** (Checking blocks back; "failed" never shown)
  - Done when: RNTL tests: every reason + fallback, every info-line variant, estimate Rp3.000, a 4xx shows the inline
    or generic error copy and the next press sends a new key (AC-61), back blocked, Dismiss sends nothing and shows
    the history hint; with two recent attempts at launch, each one's result (done or rejected) is shown before the
    next is resent (P5.3 review F5).
  - Result: `pay`, `payment-result`, `checking` routes, `AttemptNavigator` (state drives screens, incl. the launch
    resend opening Checking), `UnconfirmedCard` on Home, `payInfoLine`/`estimateCashback`, and
    `acknowledge()` in AttemptProvider (F5: a launch resend waits for Done or leaving the rejection). 86 new or
    changed Jest tests; mutations red and restored: back block removed, key reused after a 4xx, acknowledge wait
    skipped, "failed" copy, waiting not blocking, Dismiss that also sends, Pay not acknowledging on leave.
    Review fixes (F1, F2, F3, F5): the result acknowledges on unmount; a press while the launch queue waits turns the
    rest into cards and runs with a new key; Pay fills in a rejection that arrives while it is open; layout options,
    disabled Pay and a non-UTC device stamp are now asserted. 6 more tests, each mutation red and restored.
    Assumption: redemption answers only return Home until P5.5 adds its screens.
- [x] **P5.5** Redeem and confirmation — AC-65, AC-60 (redemption) · `ts` · not critical
  - Done when: RNTL tests: `INSUFFICIENT_BALANCE` refetches then shows the limit; `REDEMPTION_PAUSED` refetches the
    campaign; success shows `balance_after`; a launch-resent redemption result (done or rejected) is shown and
    acknowledged on leave, and the P5.4 interim `dismissTo('/')` + `acknowledge()` in `AttemptNavigator` is removed
    (P5.4 review F4).
  - Result: `parseRedemptionResult`, Redeem screen (form and confirmation in one route), navigator interim branch
    removed (Redeem shown after Checking: `dismissTo` when pressed there, `replace` after a launch resend). 24 new
    or changed Jest tests; mutations red and restored: balance refetch not awaited (stale limit), interim
    navigator branch kept, unmount acknowledge removed, button not disabled when paused.
    Assumption: the confirmation is on the Redeem route, not an eighth route.

**P5 gate:** `make mobile-check`, `make gate` → exit 0 · stack up + demo-reset · `cd mobile && npm ci && npx expo
start`; the owner walks screens 1–7 as user_a (pay, result, redeem, history) on a simulator or Expo Go · reconcile →
exit=0 · CI `gate`, `smoke`, `mobile` green.

**P5 gate result (2026-10-06):** all steps pass. `make mobile-check` exit 0 (274 tests); `make gate` exit 0; `up -d
--build --wait` exit 0; `demo-reset` exit 0; the app runs on the iOS simulator (Expo Go) and the owner walked screens
1–7 (pay, result, redeem, history). The first run found four bugs, each fixed with a test: route tests under
`src/app` were bundled as routes (moved to `src/app-tests`, `Href` typing); headerless screens under the status bar
(`SafeAreaView`); back buttons carried the previous title (`headerBackButtonDisplayMode: 'minimal'`); a screen opened
by deep link had no back (`initialRouteName: 'index'`). Reconcile exit 0 (INV-01–09 ok, liability 52600); CI run
37313144327 `gate`, `smoke`, `mobile` green on `e7a39c4`.

## P6 — Hardening, README, submission

- [x] **P6.1** README — all trust conditions · `go`, `ts` · not critical
  - Sections in order: how to run (stack, curl examples, app on Expo Go Android / iOS simulator with
    `EXPO_PUBLIC_API_URL`, changing port 8080, a placeholder link `TODO(owner): screen recording` for the owner to
    fill); trust conditions (copied from DECISIONS.md, not rewritten); rules as interpreted; decisions that matter;
    rejected options; out of scope; where it breaks (§12). The load test (D51, dropped by D52) goes under rejected
    options. Every rule line checked against its AC.
  - Done when: the owner reads it start to finish and each curl example runs as written.
  - Result: `README.md` written in the AGENTS.md section order; the D07 table is copied verbatim (22 rows, diffed
    against DECISIONS.md). Every README command run as written on the stack after `demo-reset`: healthz, reads 200,
    payment 201 then replay 200 (`Idempotent-Replayed: true`, same body), redeem 201, four switch commands, reconcile,
    each exit 0. The run found `GET /campaign` needs `X-User-ID` (400 `MISSING_USER`); the example now sends it.
    The screen-recording link stays `TODO(owner)` for P6.2.
- [ ] **P6.2** Submission checks — AC-56, AC-57 · owner-run · not critical
  - `docker compose down -v`; `docker builder prune -af`; the clean-clone check with `docker compose build --no-cache`
    before `up -d --wait`; walkthrough from the README only on Expo Go on an Android phone (C11); reconcile after it →
    exit=0; `! docker compose logs api | grep -qEi 'select |insert |goroutine '; echo exit=$?` → exit=0; the owner's
    recording link is filled in; CI green on the commit.
  - Carried from the P0 gate step 4: rerun the subagent stop-hook proof (a subagent that writes the unformatted
    `zz_stop.go` itself, then tries to finish; its transcript must show the stop-check block).

**P6 gate:** every P6.2 step exits 0 / passes as stated, recorded in the P6.2 commit body.

## Changes — mobile readability and performance

From `/change` (2026-10-06): no AC changes. C1–C4 change no behaviour; C5 follows the amended tech-spec focus rule.

- [x] **C1** Shared guards, one month table, one import style · `ts` · not critical
  - `src/api/guards.ts` (`isRecord`, `isStr`, `isInt`) used by `client.ts`, `payments.ts`, `redemptions.ts`,
    `queries.ts`, `attempts/store.ts`; `MONTHS` exported once from `copy/stamp.ts`; every import in `src/` uses `@/`.
  - Done when: `make mobile-check` exits 0 with no test changed.
  - Result: `make mobile-check` exit 0, 24 suites / 274 tests, no test changed beyond import lines; `jest.mock` and `requireActual` paths moved to `@/` too (Jest resolves it).
- [x] **C2** Theme tokens and UI primitives · `ts` · not critical
  - `src/ui/theme.ts` (colours, spacing, radius), `src/ui/Card.tsx`, `src/ui/AmountInput.tsx`, used by every screen
    and `UnconfirmedCard`, `Button`, `ActivityRow`, `LoadError`.
  - Done when: `make mobile-check` exits 0 with no test changed.
  - Result: `make mobile-check` exit 0, 24 suites / 274 tests, no test changed; `theme.ts`, `Card`, `AmountInput` added and used by all screens and the four `ui` components; formatting stays in the screens.
- [x] **C3** `useAmountForm(kind)` for Pay and Redeem · `ts` · critical (press path)
  - Owns amount text, launch-rejection prefill, acknowledge on leave, `inFlight`, the rejection matching the amount.
  - Done when: hook tests (prefill, no overwrite after typing, acknowledge on unmount) red then green; mutation
    (drop the amount match) red; `pay`/`redeem` tests unchanged and green; code-checker report.
  - Result: `make mobile-check` exit 0, 25 suites / 281 tests (7 new in `useAmountForm.test.tsx`; `pay` and `redeem` tests unchanged); `useAmountForm` owns text, prefill, `inFlight`, matching rejection and acknowledge on leave; `Fragment` replaced by `<>`. Mutations red: amount match dropped (rejection test), kind check dropped on the prefill (other-kind test); review: 2 minor test gaps fixed, mutations red (kind check dropped on `rejection` -> new other-kind test red; `inFlight` reduced to `sending` -> `saving` row red).
- [x] **C4** Split the attempt context · `ts` · critical (AttemptProvider)
  - Three contexts: attempt `state`; `unconfirmed` + `launchChecked`; actions (stable for the provider's life,
    `checkNow`/`dismiss` read `unconfirmed` through a ref). Guard, keys, launch queue, `refreshAfter` unchanged.
  - Done when: a Profiler test counts Home commits during a payment press, before and after (numbers in the result
    and learnings); provider and navigator tests unchanged and green; code-checker report.
  - Result: `make mobile-check` exit 0, 26 suites / 288 tests (2 new in `index.renders.test.tsx`). Three contexts (state, launch, actions) from the one provider; actions stable (`unconfirmed` and the user read through refs, the cards changed only via `updateUnconfirmed`, which writes the ref and the state together); `useAttempts` removed, callers on the narrowest hooks. Home renders (calls of `useCampaign`, a Profiler only reports the mount under this renderer) during a payment press: before 2, after 1 (the refetch after `done`); saving to sending: before 2, after 0. Mutation red: state and `unconfirmed` merged into the actions value (both tests red, 2 and 2 renders). Guard, keys, `refreshAfter`, launch queue logic untouched (comments only). review: F1 fixed (user synced in layout effect; switch-then-press test, mutation red).
- [ ] **C5** Focus refetch skips fresh queries; screens drop manual memo · `ts` · not critical
  - `useRefreshOnFocus(...queries)` returns `refetchAll` for pull and retry; on focus refetches only queries not
    fetching and older than 2 s. Home and History drop the destructured `refetch` + `useCallback` blocks.
  - Done when: a test red then green: after a `done` payment, returning Home sends no second `/me/cashback` GET
    within 2 s, and a focus after 2 s does; AC-58 test green.

**Changes gate:** `make mobile-check` exit 0; walkthrough Home → Pay → result → Done → History → Redeem
on Expo Go.

## Slip rule

If the work must shrink, the only planned cut is mobile polish beyond the wireframe's states and copy. Any other cut
(screen 7, any cache, anything else) is not planned: it needs a new DECISIONS entry proposed by the owner first. Never
cut: any concurrency test or its mutation, reconcile, tests of the critical write paths, D48 saved attempts, the README.
