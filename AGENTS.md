# Flash Cashback — Agent Guide

Flash Cashback is a cashback campaign feature (Go API, PostgreSQL, Redis, React Native app), built from
[docs/challenge-brief.md](docs/challenge-brief.md). `CLAUDE.md` only imports this file.

The owner makes every material decision. Agents propose, execute, and verify; they never settle an owner's decision
silently and never grade their own work as done.

This feature moves money. When speed and correctness pull apart, correctness wins, and the cut comes from scope.

## Workflow

| Stage   | Who                                                    | Output                                  | Budget        |
| ------- | ------------------------------------------------------ | --------------------------------------- | ------------- |
| 1 Spec  | `spec-maker` (+ grill-me) → `spec-checker`             | `docs/prd.md`, `docs/tech-spec.md`      | 1 check round |
| 2 Plan  | `plan-maker` → `plan-checker`                          | `plans/delivery.md`                     | 1 check round |
| 3 Build | per task: `code-maker` → `code-checker` → `code-fixer` | code, tests, ticked task, learnings row | 1 fix round   |
| 4 Ship  | owner                                                  | clean-clone gate, README, submission    | —             |

A red gate that is not a behaviour gap goes to `code-fixer` in red-gate mode. Makers repair their own drafts from
checker reports: a finding is evidence to validate, not an instruction to obey. Loops stop at their budget, never at
"zero findings".

**Plan size:** the plan stays compact, at most about 25 tasks. `code-checker` runs on every **critical
task** (defined in the Project Profile below); for other tasks the owner's diff read is the review unless the owner
asks for the checker.

The agents, commands, and skills under `.claude/` name no product. Everything specific to this project is in this file,
mostly in the Project Profile, and in `docs/`.

## Sources of Truth

| What                                 | Where                       |
| ------------------------------------ | --------------------------- |
| Requirements (never edited)          | `docs/challenge-brief.md`   |
| Decisions and their reasons          | `docs/DECISIONS.md`         |
| HTTP API (approved before the build) | `docs/api-contract.md`      |
| Screens (approved before the build)  | `docs/ui-wireframe.md`      |
| What and why (ACs)                   | `docs/prd.md`               |
| How (design)                         | `docs/tech-spec.md`         |
| Order of work and gates              | `plans/delivery.md`         |
| What surprised us                    | `plans/learnings.md`        |
| Review reports                       | `plans/reviews/`            |
| Pitfalls of the chosen tools         | `docs/known-pitfalls.md`    |
| Reviewer summary (copies only)       | `docs/design-overview.md`   |

Precedence: brief → DECISIONS → api-contract and ui-wireframe → prd → tech-spec → delivery. `DECISIONS.md` is created
by the decision session at the start of stage 1. Each fact lives in one place: the spec cites decision IDs (`D07`) and
links the API contract instead of restating them. A conflict is raised to the owner, never resolved silently.

## Working Rules

1. **Ask last.** Before asking the owner anything: read what the repo records, run a safe read-only probe, and reuse what
   is already known. If the choice is reversible, low-risk, and unlikely to surprise the owner, take it as a **stated
   assumption** (write `Assumption:` in the report and the commit body) and continue. Batch the questions that remain.
2. **Decision budget.** Use the [grill-me skill](.claude/skills/grill-me/SKILL.md) only for product rules, stack and
   library choices, the data model, money and concurrency rules, security, and anything cross-cutting or hard to
   reverse. The Project Profile names the open decisions, which are asked one at a time, and the batches, which are
   confirmed together. Beyond those, add few: copy, labels, and local implementation details are assumptions, not
   decisions.
3. **No application code** before `docs/prd.md`, `docs/tech-spec.md`, and `plans/delivery.md` are approved and committed.
4. **One task at a time.** Name the `delivery.md` task ID first; change only what that task needs.
5. **Work, then right, then fast.** First the most direct code that passes; then clean names, structure, and tests; speed
   work only with a measurement. No abstraction before it has earned its place. Money correctness is never traded for
   speed.
6. **Test-first.** One failing test per behaviour, failing on an assertion (a compile error or crash is not red). A test
   that is green on its first run is proved by a mutation that turns it red, and the mutation is reported.
   - **A concurrency test can pass by luck.** Every test of a race (daily cap, budget, double redeem, same idempotency
     key) is proved by a mutation: remove the lock or the constraint it protects, show the test go red, restore. It runs
     with `-race -count=20` in the task gate.
   - **Invariants are asserted after every concurrency test:** the ones listed in the Project Profile.
7. **Separate roles.** Maker writes, checker reads, fixer applies confirmed findings.
8. **Checks are enforced by tools, not promises.** Anything a tool can check is enforced at one of the checkpoints
   below; agents and reviewers spend judgement only on what no tool can see. Never use `--no-verify`, skip a test,
   loosen an assertion, add an ignore, or disable a rule to pass.
   - **Evidence is command output with its exit code.** A summary such as "all green" is not evidence; read exit codes
     directly, never through a piped `tail`.
   - **A failure is reported, not buried.** If the task gate or the stop check still fails, the report starts with
     `FAILED`, includes the output, and proposes no commit.
9. **Thematic commits.** One purpose per commit, complete: the code, its tests, the ticked task in `delivery.md`, and its
   `learnings.md` row travel together. A grill-me decision, the spec, and the plan get their own `docs` commits.
10. **The owner commits.** Agents propose a Conventional Commit message; they commit only when the owner says so. No
    AI attribution in commits or PRs: no `Co-Authored-By` trailer naming a model or tool and no "Generated with" line.
11. **Smallest responsible change.** No unrequested features, refactors, or dependencies. A task that needs a library
    names the install step; dependencies are pinned (`go.mod`, a committed lockfile).
12. **Read the pitfalls.** Before a task, read the sections of `docs/known-pitfalls.md` for the tools it touches.

## Checkpoints

| #   | Checkpoint       | Fires when                  | Runs                                                                        | Kind      |
| --- | ---------------- | --------------------------- | --------------------------------------------------------------------------- | --------- |
| 1   | Agent stop check | an agent or subagent stops  | `gofmt` + `go vet` if Go changed; lint + typecheck if TypeScript changed    | tool      |
| 2   | pre-commit       | every commit                | `gofmt` on staged Go files; commit message format and no AI attribution     | tool      |
| 3   | pre-push         | every push                  | unit tests (no database)                                                    | tool      |
| 4   | CI               | every push to GitHub        | full gate with PostgreSQL and Redis + `docker compose up` smoke             | tool      |
| –   | Task gate        | before a commit is proposed | format check, vet, lint, unit tests, integration tests with `-race`         | agent-run |
| –   | Review           | after each task             | code-checker (critical tasks), then the owner's diff read                   | judgement |

Slow checks (database tests, image builds) never run on every commit: they run in the task gate and in CI. Checkpoint 1
is `scripts/agent-stop-check.sh`, wired in `.claude/settings.json`; each half stays inactive until its project exists
(`backend/go.mod`; `lint` and `typecheck` scripts in `mobile/package.json`). Checkpoints 2 and 3 are the scripts in
`.githooks/`, enabled once per clone with `git config core.hooksPath .githooks`. The task gate is fast feedback; CI is
the guarantee.

## Project Profile

What the generic agents and skills need to know about this project.

**Approved inputs** (drafted before the build; a change is the owner's decision):

| Input                  | Role                                                                       |
| ---------------------- | -------------------------------------------------------------------------- |
| `docs/api-contract.md` | The contract: every endpoint, field, code, and header                      |
| `docs/ui-wireframe.md` | The screen specification: copy, states, and which request feeds each value |

Both are drafts with `OPEN — decision N` markers. They become final when the decision session ends (see below).

**Open decisions.** These eight belong to the owner and are asked one at a time, in this order. For each, the agent
lays out the options with what each costs and names one recommendation; the owner chooses and gives the reason in
their own words, and that reason is what `DECISIONS.md` records. An agent never settles one of these, never treats
its own recommendation as the answer, and never rewrites the owner's reason.

1. Where the daily cap and the budget are enforced (in PostgreSQL inside the payment transaction, or a Redis counter
   with later settlement in PostgreSQL).
2. What a payment earns when its full 5% does not fit the daily cap or the remaining budget (a partial award, or
   nothing).
3. When the budget is spent (when cashback is awarded, or when it is redeemed).
4. What "per day" means (which clock, which time zone, calendar day or rolling 24 hours).
5. Whether there is a kill switch, and what it stops (awards only, awards and redemptions, or two switches).
6. What Redis is for, given the brief requires it (rate limiting, read caches, counters, or a mix), and what happens
   when it is down.
7. What else has to be true before this is trusted with real money, and what makes the cut. At least: abuse by one
   user and by many accounts; refunds of a payment whose cashback is already redeemed; cashback on authorised versus
   settled payments; balances that are never redeemed; what is monitored and who is alerted; a trail of operator
   actions; a rule changed while the campaign runs; a budget that a restart or redeploy must not reset; a rate limit
   keyed on a header anyone can set. The outcome is recorded as one table, the **trust conditions**: condition that
   must hold · how it is guaranteed · how it is proved · status (built, stated only, or out) with the reason. The
   conditions the other decisions create (budget never overspent, daily cap never exceeded, no double payment on a
   retry, no negative balance) are rows of the same table. The rows marked built are this project's definition of
   production ready.
8. The cut line. Candidates to keep or drop: read caches, an `ENDING_SOON` status, cursor paging in history, the
   payment detail sheet, a load-test command, CI.

**Confirmed in batches, not debated:** the "Settled defaults" and "Library picks" tables in `docs/api-contract.md`,
and the engineering defaults below. The owner confirms each batch once and can pull any row into the session.

**Engineering defaults:** every money write is one database transaction; a request that can be retried is idempotent
by a key stored with the row it creates, in that same transaction; an append-only ledger records every credit and
debit, and a balance row holds the running total; `CHECK` and `UNIQUE` constraints state every invariant the code
relies on; a reconcile command proves the invariants and only reports.

**After the session:** the main session replaces every `OPEN` marker in the two approved inputs with the chosen
variant, shows the owner the diff, and proposes `docs(spec): finalise contract and wireframe`. The spec is written
only after that commit.

**Critical write paths:** the payment award and the redemption, and the idempotent insert in front of each. The exact
rows they touch follow from decisions 1 to 3 and are named in the tech spec.

**Critical tasks:** any task that touches a critical write path, locking, idempotency, the award rules,
reconciliation, or a migration.

**Invariants** (asserted after every concurrency test and by the reconcile command; the spec adds any that the
decisions create):

- per user, the sum of ledger entries equals the balance;
- total cashback earned never exceeds the budget;
- no user earns more than the daily cap in one campaign day;
- no balance is negative;
- per user and per operation, one idempotency key produces at most one payment and at most one redemption.

**Boundary cases every check covers:** Rp19.999, Rp20.000, and Rp20.001; the award that exactly reaches the daily cap;
the award that takes the last of the budget; the cap and the budget both short; a user with no rows yet; an ended
campaign; a payment in flight across the day boundary; a redemption equal to and one above the balance.

**Adversarial scenarios for the spec check,** beyond the general ones: many users draining the last of the budget at
once; two redeems of the same balance; Redis down, slow, or stale; and, if a kill switch exists, a pause that lands
while an award or a redemption is in flight.

**Phase outline:** P0 tooling, compose skeleton, migrations, health check · P1 pure rules and read endpoints ·
P2 payment award, idempotency, concurrency tests, reconciliation · P3 redemption and history · P4 Redis and
operations tools, as decisions 5, 6 and 8 define them · P5 mobile app · P6 hardening, README, submission.

**Slip priorities,** first to be cut first: mobile polish, anything decision 8 marked optional. Never cut:
concurrency tests, reconciliation, the README.

**Final checks before submission:** the clean-clone check without build caches; a full walkthrough of the app as a
reviewer would run it, on a path that does not need a Mac; reconcile after the walkthrough.

**README sections:** the project description and how to run, then links to the docs. The README links
`docs/design-overview.md`, which holds, in this order: what has to be true before this touches real money (the trust
conditions table from decision 7, copied from `DECISIONS.md`); rules as interpreted; decisions that matter; rejected
options; out of scope; where it breaks (copied from the tech spec).

**Naming:** the Go module path and the repository name are given by the owner before P0.

## Changing the App

Once the first delivery is done, every change starts from one question: **does it change what the app is supposed to
do?** No means the code is wrong and the spec is right. Yes means the spec changes first, so docs and code never drift.

| Command              | Use when                                    | Spec changes? | Commits                                  |
| -------------------- | ------------------------------------------- | ------------- | ---------------------------------------- |
| `/bugfix <report>`   | the app contradicts an existing AC          | no            | one `fix`                                |
| `/feature <request>` | new behaviour the spec does not describe    | yes, new ACs  | `docs(spec)`, `docs(plan)`, then `feat`s |
| `/change <request>`  | unsure, or a tweak to an existing behaviour | amended AC    | routes to one of the above               |

- **Bug:** reproduce first with a failing `AC-xx:` regression test, confirmed by the owner, then the smallest fix. A
  bug with no AC behind it is a spec gap: add the AC first.
- **Feature:** a small version of the whole workflow: questions, new ACs and design, one check round each, an appended
  delivery phase, then the build loop.
- **Enhancement:** amend the AC (`AC-12` becomes `AC-12a` when split), then tasks in a `Changes` phase.

## Skills

| Load                                                                     | When                                                   |
| ------------------------------------------------------------------------ | ------------------------------------------------------ |
| [grill-me](.claude/skills/grill-me/SKILL.md)                             | an open decision survives rule 1                       |
| [programming-go](.claude/skills/programming-go/SKILL.md)                 | any Go change                                          |
| [developing-backend](.claude/skills/developing-backend/SKILL.md)         | API, domain, database, cache, or money-movement code   |
| [programming-typescript](.claude/skills/programming-typescript/SKILL.md) | any TypeScript change                                  |
| [developing-mobile-ui](.claude/skills/developing-mobile-ui/SKILL.md)     | React Native screens, state, effects, data fetching    |

## Stack

The brief mandates the first four bullets. Every other choice is recorded in `docs/DECISIONS.md`. Do not introduce a
tool that is not recorded.

- **Backend:** Go
- **Database:** PostgreSQL
- **Redis:** required by the brief; its role is open decision 6
- **Mobile app:** React Native
- **Runtime:** the API, PostgreSQL, and Redis start with one `docker compose up` from the repo root, with no `.env`
  required; migrations and the campaign seed run automatically on first boot. The mobile app runs from the host.

Library choices are proposed in `docs/api-contract.md` ("Library picks") and become binding when `docs/DECISIONS.md`
records them; the summary table there is the list agents follow.

## Layout

```
backend/     Go module: cmd/ (api, admin, reconcile), internal/, migrations/
mobile/      React Native app (TypeScript)
docs/        brief, decisions, contract, wireframe, spec
plans/       delivery plan, learnings, review reports
scripts/     agent stop check
```

## Conventions

- **Money:** integer IDR in `int64` / `BIGINT`, never a float, never a string. Rates are basis points. Rounding is
  always down.
- **Time:** timestamps are `timestamptz`. What a campaign "day" is, and which clock decides it, is open decision 4;
  once decided, one function computes it and nothing else does.
- **Domain rules** (award amount and reason, campaign status) are pure functions with no I/O, tested with tables of
  worked examples.
- **Money writes** happen in one database transaction, take locks in the order `docs/DECISIONS.md` records, and are
  idempotent by key. Which store decides an amount is open decision 1.
- **Errors:** JSON `{ "error": { "code": "...", "message": "...", "request_id": "..." } }` with the right HTTP status;
  no stack traces or SQL in responses or logs.
- **Security:** every query is scoped by the user from `X-User-ID`; budget figures never leave the server.
- **Config:** everything comes from the environment with a working default; `.env` is never committed; `.env.example`
  lists every variable.

## Commits

Conventional Commits, enforced by `.githooks/commit-msg`: `type(scope): subject`, imperative, lower case, no trailing
period. Scopes: `repo`, `agents`, `decisions`, `spec`, `plan`, `api`, `mobile`, `db`, `infra`.

Message shape, so the hook passes the first time:

- Subject at most 72 characters; a blank line, then the body; a blank line, then the footer.
- Every body and footer line is at most 72 characters. Wrap by hand.
- `Assumption:` and `Refs:` lines are footer lines. Keep each assumption short; if it needs more words, continue on the
  next line, indented by two spaces.
- Count the lines before proposing, for example with `awk 'length > 72' msg.txt`, which must print nothing.

## Commands

Run from the repo root. The `make` targets are created in P0; until then the table is the target list P0 must deliver.

| Purpose           | Command                                                                  |
| ----------------- | ------------------------------------------------------------------------ |
| Enable hooks      | `git config core.hooksPath .githooks` (once per clone)                   |
| Stack up (build)  | `docker compose up -d --build --wait`                                    |
| Stack up          | `docker compose up -d --wait` (API on `localhost:8080`)                  |
| Stack down        | `docker compose down` (`down -v` also drops the database volume)         |
| Task gate         | `make gate` (`fmt-check`, `vet`, `lint`, `test`, `test-integration`)     |
| Unit tests        | `make test` (no database)                                                |
| Integration tests | `make test-integration` (real PostgreSQL and Redis, `-race`)             |
| Concurrency proof | `make test-race` (`-race -count=20` on the concurrency tests)            |
| Format / check    | `make fmt` / `make fmt-check`                                            |
| Mobile checks     | `make mobile-check` (lint, typecheck, tests in `mobile/`)                |
| Mobile app        | `cd mobile && npm ci && npx expo start` (or the command DECISIONS records) |
| Reconcile         | `docker compose exec api /app/reconcile`                                 |

Clean-clone check (P0 gate and again before submission; run `docker compose down` first):

```sh
rm -rf /tmp/fc-clean-1 && git clone "$(git remote get-url origin)" /tmp/fc-clean-1 &&
  cd /tmp/fc-clean-1 && test ! -e .env && docker compose up -d --build --wait
curl -fsS localhost:8080/v1/healthz
docker compose down -v # in the clone only
```
