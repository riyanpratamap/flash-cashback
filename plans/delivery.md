# Flash Cashback — Delivery Plan

Order of work for [prd.md](../docs/prd.md) and [tech-spec.md](../docs/tech-spec.md) ("§" = tech-spec section). Phases
follow the AGENTS.md outline as cut by D08 (read caches and CI kept, no load test). Each task lands as one commit
carrying its code, its tests, its ticked box here, and its `plans/learnings.md` row (AGENTS rule 9). **27 tasks**,
above the ~25 guide by owner decision: smaller reviewable tasks; merging would create two oversized tasks.

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
- [ ] **P0.2** Migration, boot, integration harness — AC-53 · INV-02–07 (constraints), INV-06 (triggers) · TC10 · `go`
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
- [ ] **P0.3** API server, healthz, image, compose — AC-50, AC-56 · TC8 (health), TC20 · `go` · not critical
  - `cmd/api` (§8 timeouts, 8 s drain on SIGTERM, `healthcheck` subcommand), chi router, `GET /v1/healthz` (PG 1 s,
    Redis 50 ms, go-redis options of §6); `cmd/admin` and `cmd/reconcile` as stubs (exit 2, "not built yet") so the
    image and the Commands table exist from P0; `backend/Dockerfile` (multi-stage, `CGO_ENABLED=0`, distroless,
    `/app/api|admin|reconcile`), `.dockerignore`, root `docker-compose.yml` (TCP `pg_isready`, `service_healthy`,
    only 8080 published, every variable defaulted).
  - Install: `go get github.com/go-chi/chi/v5@<pinned> github.com/redis/go-redis/v9@<pinned> github.com/google/uuid@<pinned>`.
  - Done when: integration tests give 200 `status` ok, redis ok / 200 `status` ok, redis `degraded` (Redis at a closed
    port) / 503 `status` `unavailable`, postgres `down` (PG at a closed port, C16 body); `docker compose up -d --build
    --wait` exits 0; `docker compose ps` shows only 8080.

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

## P1 — Pure rules and read endpoints

- [ ] **P1.1** Domain rules — AC-01–12 and AC-71 (rule level), AC-13 (reference) · INV-07 · TC1, TC2 · `go` · **critical**
  - `internal/domain`: `Award`, `CampaignStatus`, `TodayRemaining`, `Reference`, WIB formatting (fixed `+07:00`).
  - Done when: `make test` exits 0 with the §3 award table, every reason, the AC-41 status table, `000042` /
    `1234567` references; **mutation:** check cap before budget for the reason → the tie row (D45) red.
- [ ] **P1.2** Validators and HTTP edge — AC-17, AC-18, AC-54 · TC20 · `go` · **critical** (key parse, request hash)
  - Pure validators in `internal/domain` (§1, §7, §11: user ID, key length + 8-4-4-4-12 + `uuid.Parse`, raw-token
    amount, `limit`), table-tested there; handlers call them. Request ID echo/generate, access log, recover; D21 error
    mapper incl. chi 404/405; `request_hash`.
  - Done when: `make test` exits 0 with domain tables for every AC-17/18 value, and `httptest` through the real router
    with service fakes for the user → key → body order, panic → fixed 500 with no stack, `request_id` = `X-Request-ID`.
- [ ] **P1.3** Store reads, `GET /campaign`, `GET /me/cashback` — AC-16, AC-41 (flag rows), AC-48, AC-49 (these two) ·
  INV-10 · TC22 · `go` · not critical
  - `internal/store` pool, `campaigns.Get`, `balances.Today` (one query, day from `fc_campaign_day(fc_now())`);
    `service.Reads`; handlers.
  - Done when: integration tests: user_new gives 0 / 0 / 50000, WIB date, next 00:00+07:00; AC-41 rows 1, 2, 5; no
    response key matches `budget|spent`.

**P1 gate:** `make gate` → exit 0 · `docker compose up -d --build --wait` · `curl -fsS -H 'X-User-ID: user_a'
localhost:8080/v1/me/cashback` → balance 0 · `curl -s localhost:8080/v1/campaign` (no user) → 400 `MISSING_USER` ·
`docker compose down`.

## P2 — Payment award, idempotency, reconciliation, concurrency

- [ ] **P2.1** Money transaction helper — AC-27, AC-55 (mapping) · TC3 · `go` · **critical** (locking)
  - `store.InTx` (§4, §4.4): `context.WithoutCancel` + 5 s, `SET LOCAL` timeouts from validated ints, deferred
    rollback ignoring only "tx closed", SQLSTATE → busy (`55P03`, `57014`, `40P01`, deadline before COMMIT) /
    invariant (`23514`, `23505`, append-only raise) / unknown (COMMIT error); mapper: busy → 503 `SERVICE_BUSY`, else 500.
  - Done when: integration tests force each SQLSTATE and show a cancelled parent context does not cancel the tx.
- [ ] **P2.2** `Payments.Pay` and `POST /payments` — AC-01–12, AC-15, AC-71, AC-41 (spent rows), AC-17/18 (no row) ·
  INV-01–04, 06–08 · TC1, TC2, TC12 · `go` · **critical**
  - §4.1 steps 1, 3–4, 6–9 and COMMIT (cache deletes come in P4.3); lock order D44; rule snapshot; money log line
    (§7). AC-12 sets `awards_paused` by SQL until P4.1.
  - Done when: integration tests for each AC and the boundary list (Rp19.999 / 20.000 / 20.001, exact cap, last of
    budget, both short incl. the tie, no rows, ended); **mutation (AC-71):** `Award` uses the campaign cap → 500.
- [ ] **P2.3** Payment idempotency — AC-19–22 (payment parts) · INV-05 · TC3 · `go` · **critical**
  - Fast replay, in-lock lookup (step 5), `ON CONFLICT … DO NOTHING RETURNING` no row → rollback + replay, 409 on
    hash mismatch, `Idempotent-Replayed: true`, body rebuilt from the stored row.
  - Done when: integration tests incl. replay after pause and after end; **mutation:** skip the hash compare → AC-20 red.
- [ ] **P2.4** Reconcile — AC-51 · INV-01–09 · TC5, TC19, TC20 · `go` · **critical** (reconciliation)
  - `internal/reconcile` (§9 checks 1–10, one `REPEATABLE READ READ ONLY` tx, JSON lines, exit 0/1/2), real
    `cmd/reconcile`, test helper `assertReconciled(t)`; CI `smoke` adds one curl payment and
    `docker compose exec -T api /app/reconcile`.
  - Done when: clean data → exit 0 with liability; for each check a break in the live test schema (guard dropped where
    needed) → exit 1 naming that check; row counts unchanged after every run.
- [ ] **P2.5** Race: budget and cap — AC-25, AC-26 · INV-02, INV-03 · TC1, TC2, TC14 · `go` · **critical**
  - **Mutations:** AC-25: campaign read without `FOR NO KEY UPDATE`; then also drop `campaigns_spent_within_budget`.
    AC-26: remove the campaign and user-day locks; then also drop `ude_earned_within_cap`.
  - Done when: `make test-race` exits 0; each mutation red and reported.
- [ ] **P2.6** Race: same key, lock timeout, disconnect — AC-23, AC-24, AC-27, AC-70 · INV-05, INV-06 · TC3 · `go` ·
  **critical**
  - **Mutations:** AC-23/24: drop `payments_user_key_unique` and the step-5 lookup. AC-27: remove `SET LOCAL
    lock_timeout`. AC-70: request context instead of `WithoutCancel` (resend is 201, not 200). AC-70 waits on
    `pg_stat_activity` `wait_event_type = 'Lock'` before cancelling.
  - Done when: `make test-race` exits 0; each mutation red and reported.
- [ ] **P2.7** Campaign day and durability — AC-13, AC-14, AC-52, AC-55 · INV-08 · TC6, TC10 · `go` · **critical**
  - Clock via `fc_now()` replacement (23:59:59 / 00:00:00 WIB); `TestRaceMidnight` (AC-14, held campaign row);
    restart = `boot.Run` again with `CAMPAIGN_BUDGET=1`; PG at a closed port → POST 500, uncached GET 500.
  - **Mutations:** AC-14: `created_at` from `clock_timestamp()`. AC-52: seed `ON CONFLICT DO UPDATE SET budget`.
  - Done when: `make test-race` exits 0; mutations reported.

**P2 gate:** `make gate` → exit 0 · `make test-race` → exit 0 · `docker compose up -d --build --wait` · curl `POST
/v1/payments` 100000 as user_a with a new key → 201, 5000 `AWARDED`; same key again → 200 + `Idempotent-Replayed:
true` · `docker compose exec api /app/reconcile; echo exit=$?` → exit=0 · `docker compose down` · CI green.

## P3 — Redemption and history

- [ ] **P3.1** `Redemptions.Redeem` and `POST /redemptions` — AC-29–32, AC-34, AC-35, AC-16 (redeem), AC-21 (redeem
  with K), AC-48 (redeem), AC-17/18 (no row) · INV-01, 04–06, 09 · TC3, TC4, TC13 (stub) · `go` · **critical**
  - §4.2 in the D46 order; replay incl. stored `balance_after`; payout stub; money log line (§7, `op` redeem). AC-32
    sets the flag by SQL until P4.1.
  - Done when: integration tests + reconcile; **mutation:** balance check before the paused check → AC-32 red.
- [ ] **P3.2** Race: redemptions — AC-33, AC-69, AC-28 · INV-01, 04, 05 · TC3, TC4, TC5 · `go` · **critical**
  - **Mutations:** AC-33: balance read without `FOR UPDATE`; then also drop `balances_nonneg`. AC-69: drop
    `redemptions_user_key_unique` and the step-5 lookup. AC-28: award locks the campaign `FOR UPDATE` → `40P01`.
  - Done when: `make test-race` exits 0; each mutation red and reported.
- [ ] **P3.3** `GET /me/history` — AC-47, AC-02 / AC-15 (history), AC-16, AC-48, AC-49 (all endpoints, every state) ·
  INV-10 · TC12, TC22 · `go` · not critical
  - `history.Newest`: `UNION ALL`, `LIMIT` per branch, `created_at DESC, id DESC`.
  - Done when: integration tests; AC-49 compares `GET /campaign` at budget left 2000 and 9000000 byte for byte.

**P3 gate:** P2 gate commands, plus curl `POST /v1/redemptions` 1000 as user_a → 201 with `balance_after` · `GET
/v1/me/history` lists both, newest first · reconcile → exit=0.

## P4 — Operations tools, then Redis invalidation, then read caches

- [ ] **P4.1** Switch commands — AC-36, AC-37, AC-40, AC-12 / AC-32 / AC-41 via the command · TC7 (base) · `go` ·
  **critical** (campaign-row lock)
  - `cmd/admin` four commands per §4.3 (`--by` required → exit 2; lock wait 5 s → exit 1 "busy, retry"; one JSON line
    after commit); `service.Switches`.
  - Done when: integration tests run the command entry point and read its stdout; `docker compose exec api /app/admin
    pause-awards --by owner` prints the line with `changed` true, then false on a second run.
- [ ] **P4.2** Race: pause in flight — AC-38, AC-39 (held, waiting, and 40-user cases) · TC7 · `go` · **critical**
  - Test hooks nil in production (§11); "began after" = `created_at` later than DB `clock_timestamp()` read after exit.
  - **Mutations:** AC-38: `Pay` campaign read without `FOR NO KEY UPDATE`. AC-39: remove the paused check in
    `Redeem`; separately `FOR SHARE` on the step-6 read; separately the flag read before the balance lock (D44 order).
  - Done when: `make test-race` exits 0; each mutation red and reported.
- [ ] **P4.3** Invalidation after commit; `demo-reset`; harness Redis reset — AC-57 · TC9 (deletes), TC10 (demo
  exception) · `go` · **critical** (Pay/Redeem after-commit path; truncates money tables)
  - §6 keys deleted on a fresh 50 ms context after commit in Pay (cashback; campaign when spent = budget), Redeem,
    switches; `admin demo-reset` refuses without `FC_DEMO=1` (exit 2), builds state through the services, deletes keys.
    Harness: reset runs `FLUSHDB` on the test Redis; the SQL flag and budget helpers delete `fc:v1:campaign`. No reads
    use the cache yet, so no test can see a stale value.
  - Done when: integration tests set each key in Redis, run the write, and see the key gone; Redis at a closed port →
    writes still 201; reconcile → exit 0 after demo-reset.
- [ ] **P4.4** Read caches — AC-42, AC-43, AC-44, AC-45, AC-46, AC-50 (Redis states) · INV-11 · TC8, TC9 · `go` · not
  critical (no money path; write services hold no cache client)
  - `internal/cache` (§6: 50 ms, `MaxRetries: -1`, `ContextTimeoutEnabled`, bad value = miss, warn once per 10 s);
    `Reads` use it; cashback TTL capped at `resets_at` − `fc_now()`, not cached under 1 s.
  - Done when: integration tests for AC-42 (warm cache, pay, redeem, pause), Redis at a closed port, `CLIENT PAUSE 500
    ALL` (read ≤ ~100 ms), invalid JSON, balance 999999 cached then redeem 20000 → 422, clock at 23:59:30 → TTL ≤ 30 s;
    **mutation:** remove the P4.3 delete in Pay → AC-42 red.

**P4 gate:** `make gate`, `make test-race` → exit 0 · stack up · `docker compose exec -e FC_DEMO=1 api /app/admin
demo-reset` → exit 0 · `docker compose stop redis`; healthz → 200, `status` ok, redis `degraded`; `GET /me/cashback`
as user_a → 15000; `docker compose start redis` · pause/resume both switches with `--by` · reconcile → exit=0 · `down`
· CI green.

## P5 — Mobile app

- [ ] **P5.1** Expo scaffold, API client, formatting, user — AC-67, AC-68 · `ts` · not critical
  - Install: `npx create-expo-app@<pinned> mobile` (TypeScript, Expo Router), delete its `AGENTS.md`, `CLAUDE.md`,
    `.claude/`, reset script; `npx expo install @tanstack/react-query @react-native-async-storage/async-storage
    expo-crypto`; `npm i -D jest-expo @testing-library/react-native @types/jest eslint-config-expo` (lockfile
    committed). Scripts `lint`, `typecheck` (`tsc --noEmit`, `"types": ["jest"]`), `test`; `make mobile-check` runs
    them; CI job `mobile`. `src/api/client.ts` (10 s abort = unknown), `queries.ts`, `money/format.ts`,
    `copy/codes.ts`, `user/`.
  - Done when: `make mobile-check` exits 0 (formatter, classification, base URL, user kept after remount); a
    deliberate lint error makes the stop check exit 2.
- [ ] **P5.2** Read screens: Home, History, How it works — AC-64, AC-66, AC-67 (picker), AC-75 · US-6 · `ts` · not
  critical
  - Done when: RNTL tests (awaited) cover every AC-64 state; AC-66 runs with `TZ=UTC`; AC-75 serves `rules` as 1000
    bps / 30000 / 70000 and screen 7 shows 10%, Rp30.000, Rp70.000.
- [ ] **P5.3** Money attempt machine and saved attempts — AC-59, AC-60, AC-61, AC-72, AC-73, AC-74 (logic) · TC21 ·
  `ts` · **critical** (client idempotency)
  - `src/attempts/` per §10: ref guard, key from expo-crypto per press, save to `fc:attempts` before send, remove on
    2xx/4xx only, resend x3 ~2 s then wait, launch check (< 10 min resend with saved user; older → card data).
  - Done when: Jest (fake timers) green; **mutations:** remove the ref guard → AC-59 red; new key per send → AC-60 red;
    send before save → AC-72 red; selected user instead of saved → AC-73 red.
- [ ] **P5.4** Pay, Payment result, Checking, unconfirmed card — AC-58, AC-60, AC-61 (UI), AC-62, AC-63, AC-74 (UI) ·
  TC21 · `ts` · **critical** (Checking blocks back; "failed" never shown)
  - Done when: RNTL tests: every reason + fallback, every info-line variant, estimate Rp3.000, a 4xx shows the inline
    or generic error copy and the next press sends a new key (AC-61), back blocked, Dismiss sends nothing and shows
    the history hint.
- [ ] **P5.5** Redeem and confirmation — AC-65, AC-60 (redemption) · `ts` · not critical
  - Done when: RNTL tests: `INSUFFICIENT_BALANCE` refetches then shows the limit; `REDEMPTION_PAUSED` refetches the
    campaign; success shows `balance_after`.

**P5 gate:** `make mobile-check`, `make gate` → exit 0 · stack up + demo-reset · `cd mobile && npm ci && npx expo
start`; the owner walks screens 1–7 as user_a (pay, result, redeem, history) on a simulator or Expo Go · reconcile →
exit=0 · CI `gate`, `smoke`, `mobile` green.

## P6 — Hardening, README, submission

- [ ] **P6.1** README — all trust conditions · `go`, `ts` · not critical
  - Sections in order: how to run (stack, curl examples, app on Expo Go Android / iOS simulator with
    `EXPO_PUBLIC_API_URL`, changing port 8080, a placeholder link `TODO(owner): screen recording` for the owner to
    fill); trust conditions (copied from DECISIONS.md, not rewritten); rules as interpreted; decisions that matter;
    rejected options; out of scope; where it breaks (§12). Every rule line checked against its AC.
  - Done when: the owner reads it start to finish and each curl example runs as written.
- [ ] **P6.2** Submission checks — AC-56, AC-57 · owner-run · not critical
  - `docker compose down -v`; `docker builder prune -af`; the clean-clone check with `docker compose build --no-cache`
    before `up -d --wait`; walkthrough from the README only on Expo Go on an Android phone (C11); reconcile after it →
    exit=0; `! docker compose logs api | grep -qEi 'select |insert |goroutine '; echo exit=$?` → exit=0; the owner's
    recording link is filled in; CI green on the commit.

**P6 gate:** every P6.2 step exits 0 / passes as stated, recorded in the P6.2 commit body.

## Slip rule

If the work must shrink, the only planned cut is mobile polish beyond the wireframe's states and copy. Any other cut
(screen 7, any cache, anything else) is not planned: it needs a new DECISIONS entry proposed by the owner first. Never
cut: any concurrency test or its mutation, reconcile, tests of the critical write paths, D48 saved attempts, the README.
