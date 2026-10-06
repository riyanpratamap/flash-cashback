# C22 code check — internal/store coverage

Verdict: **APPROVE_WITH_FINDINGS** (no critical, no major, two minor).

Checker's gate: `make gate` exit=0; `make cover` exit=0 (backend 88.5%, `internal/store` 96.8%, 181/187). The 14 new
tests under `-race -count=10`: exit 0, 5.1 s. Counts: 127 integration functions (113 + 14) excluding `TestMain`, 18
`TestRace*`.

Verified:

- No production code changed: only `README.md`, `plans/delivery.md`, `plans/learnings.md` modified, plus the new
  `backend/internal/integration/fault_test.go`.
- Conflict tests cannot deadlock or pass by luck. The winner, committed from the pool inside the hook, needs only the
  FK `FOR KEY SHARE` on `campaigns(id)`, which does not conflict with the loser's `FOR NO KEY UPDATE` (Pay) or the
  `FOR UPDATE` on the balance (Redeem). The loser has not inserted yet, so the unique index has nothing to wait on.
  Under READ COMMITTED, the `INSERT … ON CONFLICT DO NOTHING` and the re-read see the committed winner.
- `TestTxLostConnectionIsUnknownOutcome` cannot pass by luck. A connection that is never closed yields a plain error,
  which `classifyBeforeCommit` passes through, and the `ErrUnknownOutcome` assertion fails.
- Isolation: each test starts with `reset`; `mustSetNow` restores the clock in cleanup; no `t.Parallel`.
- `assertReconciled` runs in both insert-conflict tests and in every Pay and Redeem driver-error subtest.
- The six uncovered statements match the gaps the Result line lists.

Not verified: mutations M1–M10 were not re-run, because that means editing production code.

| ID | Severity | Confidence | File:line | Finding | Suggested fix |
| --- | --- | --- | --- | --- | --- |
| F1 | minor | MEDIUM | `fault_test.go:157-171`, `:301-317` | The two re-read-error tests commit a winner but never call `assertReconciled`. The Pay test has no rollback check, so leaked loser writes would pass. The Redeem test checks only the balance. | Pay: `readCounts` before, expect `payments+1`, then `assertReconciled`. Redeem: `readRedeemShape` with `redemptions+1`, fill in the winner's books, then `assertReconciled`. Prove it with a mutation: InTx commits when fn returns `ErrReplay`. |
| F2 | minor | MEDIUM | `fault_test.go:416`, `:439`, `:193` | In SetSwitch update, DemoTruncate TRUNCATE and Pay "find payment", the fault fires before any write, so the "rolled back" checks cannot go red, and the comment "paused after a failed change" claims more than the test proves. | Drop those rollback claims or reword the comment ("nothing paused"). |
