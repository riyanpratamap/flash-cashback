# Learnings

| Date | Task | What happened | How caught | What changed |
| ---- | ---- | ------------- | ---------- | ------------ |
| 2026-10-05 | P0.1 | `go mod edit -go=1.26` is stored as `go 1.26.0`; setup-go reads it from go.mod | go.mod diff | Accepted: the toolchain line pins go1.26.3 |
| 2026-10-05 | P0.2 | Without the session locker `TestRaceBootTwice` stayed green: goose retries a concurrent create of its version table after 1 s, so the second boot finds the migration applied | mutation run was green | Test pre-creates the version table (`HasPending`) so the race is on the migration; mutation now red with `23505` on `pg_proc` |
| 2026-10-05 | P0.2 | goose session locker polls every 5 s by default, so a losing boot waits 5 s and `-count=20` took 100 s | race test timing | `lock.WithLockTimeout(1, 60)`: poll every 1 s, give up after 60 s |
| 2026-10-05 | P0.2 | `NewPostgresSessionLocker` returns `(locker, error)` in goose v3.28.0; `setNow` on an unmigrated DB creates `fc_now` and breaks the first migration | compile error; red run before the migration existed | Locker error checked; reset the test compose volume after a red run that never migrated |
| 2026-10-05 | P0.2 | `Connect` retry: the last ping runs at the deadline and fails with `DeadlineExceeded`, hiding the real cause (connection refused) | new error-cause test red | Keep the earlier error when a ping fails only on our own deadline |
