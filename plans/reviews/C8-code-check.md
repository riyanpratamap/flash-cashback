# C8 code check — redemption success screen, balance only on a fresh 201

**Verdict:** APPROVE_WITH_FINDINGS

**Gate:** `make mobile-check` exit 0 (lint, typecheck, Jest 28 suites /
302 tests). Working tree unchanged by the review (`git diff | shasum`
equal before and after).

**Mutation:** reasoned, not run (reviewer is read-only; a scratchpad copy
was blocked). Balance row forced on -> replay test red; machine drops the
flag -> `true` row and replay screen test red; machine hard-codes `true`
-> `false` row and fresh-201 test red. The fixer re-runs the first one.

**Verified:** `replayed` is set only at `src/api/client.ts:53` and `done`
is built only in `finish()` (`machine.ts:48`); press, Checking resend,
Check again and launch resend all pass through it. Balance row and its
separator follow `state.replayed` (`redeem.tsx:73,77`). Unparsed body:
mark, "Your redemption went through.", "Check your balance on the home
screen.", no list, no balance. Done unchanged. SuccessMark: Views only,
role image, label "Success", 72 pt. Fixture header matches the client
comparison. No change to src/api, src/copy, backend or dependencies.

| ID | Severity | Confidence | File:line | Finding | Suggested fix |
| --- | --- | --- | --- | --- | --- |
| F1 | minor | HIGH | mobile/src/app-tests/redeem.test.tsx:311-312 | The AC-60 launch-resend test no longer checks the balance (it used to assert "Your balance is now Rp0."). | Assert `Cashback balance` and `Rp0` (fresh 201), or switch to `moneyReplayed` and assert no balance. |
| F2 | minor | MEDIUM | mobile/src/attempts/machine.test.ts:23 | Only `runPress` is tested; no replayed answer goes through `check()`, the realistic replay path. | Add a `check(attempt, deps(replayed), true)` row expecting `done.replayed === true`. |
| F3 | minor | MEDIUM | plans/delivery.md (C8 Result) | Result says "Code-checker report pending". | Update after this report is saved. |
