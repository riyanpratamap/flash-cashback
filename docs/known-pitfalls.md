# Known Pitfalls

Behaviour of the tools in this stack that is easy to get wrong. Each applies only when its tool is chosen in
`docs/DECISIONS.md`. Agents read the sections for the tools a task touches before starting it. An item here is a claim to check, not a fact to trust: when one decides a
design, confirm it with a test. What the build teaches goes into `plans/learnings.md`; anything general enough is
promoted here.

## Process

- A rule written in a decision can contradict engine behaviour. Check the behaviour before recording the rule.
- Copy a decision's scope exactly into config; widening it silently changes behaviour elsewhere.
- Read exit codes, not the tail of piped output: `| tail` hides a non-zero exit.
- Never undo a probe with a path-wide `git checkout -- .`; it reverts the task's own uncommitted edits. Back up the
  probed file instead.
- Scripted search-and-replace edits hit every match; anchor them and re-read the file afterwards.
- A task that needs a library must name the install step.
- Keep submission checks (fresh clone, no `.env`, `--no-cache`) in the plan and run them; they are the reviewer's first
  step.
- A concurrency test that passes proves little by itself. Remove the lock it guards and watch it fail before trusting it.
- A fixture that sets one column of an invariant by hand (a remaining budget, an earned total) breaks the reconcile
  checks. Build test state through the real code path.
- A mutation made by editing an applied migration file changes nothing: the test database is already migrated.
  Mutate the live test schema, and restore it the same way.
- A summary of a rule drifts from the rule. Check every rule line in the README against its acceptance criterion.

## Go

- `defer tx.Rollback(ctx)` right after `Begin`. After a successful `Commit` the deferred rollback returns a "tx closed"
  error; that one error is safe to ignore, and only that one.
- An early `return` between `Begin` and `Commit` rolls back silently. Every path that should persist must reach
  `Commit` and check its error.
- `encoding/json` decodes numbers into `float64` when the target is `any`. Decode into a struct with `int64` fields, and
  test that `1.5`, `"100"`, `1e5`, a negative, `null`, and a missing field are each rejected or handled on purpose.
- `amount × rate_bps` in `int64` is safe only because the amount has an upper bound. Keep the bound check before the
  multiplication.
- A cancelled request context cancels in-flight queries. Good for reads; for work after a commit (cache invalidation)
  use a context detached from cancellation with its own timeout, or a client disconnect leaves a stale cache.
- A client that disconnects during the transaction causes a rollback, or not, depending on where the cancel lands. The
  client can't tell; that is why it retries with the same key.
- `http.Server` has no timeouts by default. Set read-header, read, write, and idle timeouts, and shut down gracefully so
  in-flight transactions finish.
- `go test -race` needs cgo and a C toolchain; a minimal build image may not have one. Decide where race tests run.
- `go test` caches results. Tests that depend on a database need `-count=1` (or a higher count) to actually run.
- Packages are tested in parallel processes by default. Integration tests in several packages sharing one database will
  trample each other: use `-p 1`, one integration package, or a database per package.
- `t.Fatal` from a goroutine other than the test's own does not stop the test correctly. Collect errors and assert in
  the test goroutine.
- A test that starts 100 goroutines against a pool of 10 connections measures the pool queue. Make sure the test still
  overlaps transactions (a start barrier helps), and size the pool on purpose.
- `uuid.Parse` also accepts braced, `urn:uuid:` and dash-less forms. Check the length as well when the contract
  asks for the canonical form.
- Decoding a JSON body into a struct matches field names without regard to case, and a number field can accept
  forms the contract forbids. Decode the raw value when the contract is strict about a field.
- A test that holds a lock in a second transaction and then fails an assertion can leave the pool unable to close,
  so the run hangs. Roll the holder back in the test's cleanup.
- An HTTP test client with the default idle-connection limit opens a new socket for most concurrent calls and can
  exhaust the host's ports. Raise the limit in load and race tests.

## PostgreSQL

- `INSERT ... ON CONFLICT DO NOTHING RETURNING` returns **no row** on a conflict. With a single-row query that is a
  "no rows" error; it is the signal for a replay, not a failure.
- Under READ COMMITTED, a second insert with the same unique key **waits** for the first transaction to finish, then
  does nothing if it committed. The stored row is visible to the next statement in the waiting transaction.
- Any error inside a transaction aborts it; every later statement fails until rollback. Don't catch a unique violation
  and carry on in the same transaction. Use `ON CONFLICT`, or a savepoint.
- `now()` is the transaction's start time and does not advance inside it. A transaction that begins at 23:59:59 and
  waits on a lock past midnight still belongs to the old day. Use the same expression for the day and for
  `created_at` so they can't disagree. `clock_timestamp()` is the wall clock.
- `SELECT ... FOR UPDATE` on a row that does not exist locks nothing. Insert the row first (with `ON CONFLICT DO
  NOTHING`), then lock it.
- Inserting a row with a foreign key takes `FOR KEY SHARE` on the referenced row and holds it until the transaction
  ends. A later `SELECT ... FOR UPDATE` on that parent row conflicts with it, so two such transactions deadlock
  (`40P01`). Lock a parent you update without changing its key with `FOR NO KEY UPDATE`, which is what a plain
  `UPDATE` of a non-key column takes anyway.
- A plain `SELECT` before `FOR UPDATE` can read a value that is stale by the time the lock is held. Re-read every
  value used in the decision from the locked row.
- `SET LOCAL lock_timeout` works only inside a transaction and does not take bind parameters. `lock_timeout` also
  applies to the wait on a unique-index conflict.
- SQLSTATE codes worth mapping: `55P03` lock not available (lock timeout), `40P01` deadlock, `23505` unique violation,
  `23514` check violation, `57014` query cancelled. Match on the code, never on message text.
- `(now() AT TIME ZONE 'Asia/Jakarta')::date` gives the WIB date. `timestamptz` stores an instant; the offset in a
  response is formatting done on the way out, not something the column remembers.
- Scanning a `NULL` into a non-pointer Go value fails. `COALESCE` in the query or scan into a pointer; a missing row is
  a different case from a `NULL`.
- Sequences are not transactional. A rolled-back insert, and an `ON CONFLICT DO NOTHING` that hits a conflict, still
  consume an ID. Payment and redemption IDs will have gaps; nothing may assume they are contiguous.
- A row comparison `(a, b, c) < ($1, $2, $3)` matches an `ORDER BY a DESC, b DESC, c DESC` only when every column
  sorts in the same direction. Check that the index is actually used with `EXPLAIN`.
- A `UNION ALL` of two tables with an outer `ORDER BY ... LIMIT` may sort everything. Limit each branch as well.
- `TRUNCATE ... RESTART IDENTITY CASCADE` is the fast reset between integration tests, and it takes an exclusive lock:
  never while another test holds a transaction open.
- DDL is transactional in PostgreSQL, so a migration can be all or nothing if the tool wraps it in a transaction.
- A unique constraint or check violation that the code "can't" hit is a bug report. Return a 500 and log it; don't
  turn it into a 4xx.
- An insert into a table with a foreign key takes `FOR KEY SHARE` on the referenced row, held to the end of the
  transaction. `FOR UPDATE` on that row conflicts with it: two transactions that each insert and then lock the parent
  deadlock. `FOR NO KEY UPDATE` does not conflict and is what an `UPDATE` of a non-key column takes anyway.
- The deadlock detector fires after 1 second by default, before a 2-second `lock_timeout`. A deadlock surfaces as
  SQLSTATE `40P01`, not as a lock timeout.
- A `CHECK` whose expression evaluates to NULL passes. A rule over a nullable column needs `IS NOT NULL` written
  into each branch.
- A replay must return the original response. Any value in the response that changes later (a balance after a
  redemption) has to be stored with the row.
- With a lock removed, a race usually ends in a constraint violation, not in a wrong total. A race test asserts on
  errors and on totals separately, so either failure is red.
- A parameter used both as a column value and in arithmetic can get inconsistent deduced types (SQLSTATE `42P08`).
  Cast it.
- A constraint over two columns is named `<table>_check`, not after one column. Read the catalogue before dropping
  one in a test.

## Redis

- `INCR` followed by `EXPIRE` is two commands. If the second never runs, the counter never expires and the user is
  limited forever. Make it atomic (a script, or a transaction) or set the expiry in a way that can't be skipped.
- A fixed one-minute window allows up to twice the limit across a window boundary. State that, or use a sliding window.
- Set short dial, read, and write timeouts. A slow Redis must cost milliseconds, not hang a payment.
- Failing open means every Redis error path returns "allowed" or "cache miss", including timeouts and parse errors,
  and logs once without flooding.
- Invalidate by deleting after the commit, never by writing a new value, and never inside the transaction.
- Between a commit and the delete, a reader can repopulate the cache with the old value. The TTL bounds the damage;
  keep TTLs short and never cache anything a money decision reads.
- Never cache an error response or a partial result.
- A cached JSON value is a boundary: validate it when read, and treat a value that fails to parse as a miss.
- `go-redis` ignores a context deadline unless `ContextTimeoutEnabled` is set; only its option timeouts apply.
- A rate limit that remembers an accepted key must remember it for longer than a user can keep retrying, or a late
  retry is counted as new and can be refused.
- A rate limiter placed before the idempotency-key check sees no key and misbehaves. Fix the middleware order in
  one place that production and tests share.

## Docker and Compose

- An API server must listen on `0.0.0.0` inside a container, not `localhost`.
- Every compose variable needs a default so a plain `docker compose up` works with no `.env`.
- `depends_on` waits for health only with `condition: service_healthy`, and only if the dependency has a healthcheck.
- The official PostgreSQL image starts a temporary server during first-time initialisation. A healthcheck over the
  local socket can pass too early; check over TCP (`pg_isready -h 127.0.0.1`) with the real user and database.
- A scratch or distroless image has no shell, `curl`, or `wget` for a healthcheck. Give the binary a health
  subcommand, or use a base image that has a client.
- Build static binaries (`CGO_ENABLED=0`) for a minimal runtime image; that image then can't run `-race` tests.
- Publishing 5432 or 6379 on the host collides with a reviewer's local PostgreSQL or Redis. Publish only the API port,
  or use uncommon host ports.
- `docker compose down -v` deletes the data volume. A seed that runs on boot must be idempotent, and must not reset
  the remaining budget on a restart.
- `docker compose up --wait` fails if any service is unhealthy; a one-shot service (a migration job) must exit 0.
- Image build context: keep `.git`, `node_modules`, and the mobile app out of the API image's context.
- A fixed compose project `name` makes every clone the same project, so a "clean" clone reuses the old volume.
  Drop volumes before a clean-clone check.
- A reviewer's own service on the published port blocks `docker compose up`. Say in the README how to change it.
- Two API processes starting together both try to migrate. Use the migration tool's session lock.

## React Native and Expo

- `localhost` means the device itself. The iOS simulator can reach the host's `localhost`; the Android emulator needs
  `10.0.2.2`; a physical phone needs the host's LAN address. Make the API base URL configurable and document it.
- Release builds block plain `http` by default on both platforms. Development builds are more lenient; state which one
  the reviewer is expected to run.
- `fetch` has no timeout. Use an `AbortController`. An aborted request may still have committed on the server: abort
  means unknown, never failed.
- `crypto.randomUUID()` is not guaranteed in the React Native runtime. Use a library for UUID v4 and name it in the
  task.
- A state update is not synchronous. Disabling a button with `setState` does not stop a second press in the same
  frame; guard with a ref inside the handler.
- An idempotency key created during render changes on every render. Create it in the press handler and hold it in a ref
  until a definite answer arrives.
- `Intl.NumberFormat` and locale data are not identical across the JavaScript engines and platforms. A hand-written
  formatter for `Rp100.000` is a few lines and is testable.
- `new Date(isoString)` converts to the device's time zone. Group and label days from the date part of the API's
  string, or a user travelling outside WIB sees payments under the wrong day.
- JavaScript numbers are exact integers up to 2^53 − 1, far above any IDR amount here; never parse an amount with
  `parseFloat` of a formatted string.
- A screen that refetches on focus can fire while a money request is in flight; make sure the refetch can't clear the
  in-flight attempt's key or state.
- The mobile app does not run inside `docker compose`. The README must give the exact commands to start it and point it
  at the API.
- `create-expo-app` scaffolds its own agent files (`AGENTS.md`, `CLAUDE.md`, `.claude/`) and a reset script inside
  the app folder. Remove them so the repository has one agent guide.
- TypeScript 6 no longer loads `@types/*` automatically; Jest globals need `"types": ["jest"]`.
- Testing Library's `render` and `fireEvent` are async in current versions. Without `await`, an assertion can see
  the previous frame and pass for the wrong reason.
- A day label built through `Date` is wrong only when the device zone differs from the server's. Tests run in UTC,
  so the fixture needs a time just after midnight in the server's zone.
- State kept only in memory resets on every launch. Anything the demo relies on across launches (the selected
  user) needs device storage, which is a dependency to name.

