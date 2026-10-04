---
name: developing-backend
description: Judgement for API, domain, database, and cache code — where code lives, how writes stay correct under concurrency and retries, what happens to each error, what a log line carries, where input is validated, and how each layer is tested. Use with the language skill for any backend change.
---

# Developing the Backend

## Place Code Before Writing It

Ask of each piece whether it **decides** or **does**.

| Layer      | Holds                                                               | Never holds                         |
| ---------- | ------------------------------------------------------------------- | ----------------------------------- |
| domain     | pure rules, given their inputs                                      | clock, database, cache, HTTP        |
| service    | one use case: open the transaction, load, call domain, save, return | HTTP status codes, SQL strings      |
| repository | parameterised queries and locks, always scoped by the caller        | business rules                      |
| handler    | validate input, call the service, map the result to HTTP            | business rules, SQL                 |
| cache      | get, set, delete, counters; fails open                              | anything that decides a stored fact |

Misplacement shows early: a domain function that wants the clock, a handler that decides a rule, a repository that
picks a status code, a cache read inside a decision that is then written to the database.

## Critical Writes

These rules hold for every code path that changes a value people rely on being exact: a balance, a quota, a stock
count, a budget, a counter with a limit. The project's `AGENTS.md` names which paths those are.

- **One transaction.** Every write of one operation commits together or not at all. Defer the rollback immediately
  after beginning.
- **Check and change under the same lock.** A limit is read with its row locked, or checked and changed in one
  conditional statement. A plain read followed by a write is a race.
- **One lock order,** the one the project's decisions record. A code path that takes the same locks in another order is
  a deadlock waiting for load.
- **Idempotent by key.** When a request can be retried, the idempotent insert comes first in the transaction; a retry
  returns the stored result and changes nothing. A reused key with a different body is rejected.
- **Constraints are the last line of defence.** `CHECK` and `UNIQUE` constraints state every invariant the code relies
  on; a violation is a bug surfaced as a 500, never caught and ignored.
- **Audit records are append-only.** Ledger or history rows are never updated or deleted; a correction is a new entry.
- **Short transactions.** No cache call, HTTP call, or slow work between begin and commit. A lock timeout is set so a
  queue fails fast and retryable instead of hanging.
- **Cache after commit.** Caches are invalidated (deleted, not rewritten) after the commit succeeds. A cache failure is
  logged and never fails or changes the operation.
- **An unknown outcome is not a failure.** A timeout or a 5xx means the client may retry with the same key; the API
  makes that safe.

## Give Every Error One Fate

| Fate         | When                                                      |
| ------------ | --------------------------------------------------------- |
| handle it    | this code can do something meaningful (retry, fall back)  |
| propagate it | the caller decides; add context where the meaning changes |
| fail loudly  | a broken invariant or programming error                   |

Discarding is never a fate. Driver error types stop at the repository, which maps the ones that carry meaning (unique
violation, lock timeout, check violation). A failure becomes an HTTP response exactly once, in one error mapper, in the
project's error envelope, with a fixed message for 500s.

## A Log Line Is Evidence

Structured fields, not prose: request ID, caller, operation, the record's ID, outcome, duration. Log a failure once,
where it is finally handled. Every critical write logs one line. Never log a full request body, a secret, SQL text with
values, or a whole driver error object.

## Validate Once, Where Trust Ends

Validate every header, body, and query string at the handler and turn it into a typed value; trust that type inward.
Domain invariants are still enforced by the domain and by constraints. Every query that touches a caller's rows is
filtered by the caller's identity from the request; a client never names another caller's data.

## Database

- Every query is parameterised; never build SQL from input text.
- Migrations are versioned files that run automatically, after the database is healthy.
- Timestamps carry a time zone; a calendar day is a date computed in one stated time zone, in one place.
- Read paths take no locks.

## Test at the Narrowest Layer

| What             | Test                                                                           |
| ---------------- | ------------------------------------------------------------------------------ |
| a domain rule    | unit test on the pure function, table of worked examples at every boundary     |
| a repository     | integration test against the real database                                     |
| a critical write | integration test of the service: result, rows written, invariants              |
| a race           | many concurrent callers against the service, invariants asserted, proved by a mutation |
| a handler        | HTTP test through the router: status, headers, body, error envelope            |
| a retry          | same key twice, in sequence and at once: one effect, same response             |
| cache absent     | the same request succeeds with the cache stopped                               |
| scoping          | two callers; B never sees or changes A's data                                  |

Every error path gets its own test. A green-first test is proved by a mutation.

## Before Calling It Ready

- Every entry point validates its input; every query is scoped by the caller.
- Every critical write: one transaction, locks in the recorded order, idempotent where retried, constraints in place,
  invariants tested.
- Every error path is tested and ends handled, propagated with context, or failing loudly.
- Nothing sensitive in a log line, fixture, or response.
- Every new dependency is named in the task, pinned, and used.
