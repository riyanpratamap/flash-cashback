# C3 code check — `useAmountForm(kind)` for Pay and Redeem

Verdict: **APPROVE_WITH_FINDINGS**. Gate `make mobile-check` exit 0 (25 suites, 281 tests).

Behavioural equivalence with HEAD checked item by item: initial prefill, adjust-during-render keyed on the state
object, rejection match on phase, kind and amount, `inFlight`, unmount acknowledge (payment `'rejected'`; redemption
`'done'` then `'rejected'`), `setText` formatting for input, chips and Redeem all, Pay's `MAX_AMOUNT` and info line,
Redeem's paused and balance rules and confirmation. Hook order safe: `useAmountForm` runs before Redeem's early
returns. No copy changed. No package, lockfile, or config changed.

Test integrity: RNTL 14 async `renderHook`/`rerender`/`unmount` awaited; the no-overwrite test exercises the
`seen !== state` guard; both reported mutations plausible; unmount tests assert the exact call list.

| ID   | Severity | Confidence | File:line                                                 | Finding                                                                                                                                            | Suggested fix                                                                                                    |
| ---- | -------- | ---------- | --------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| C3-1 | minor    | HIGH       | `mobile/src/attempts/useAmountForm.ts:36`, hook test      | No test covers the kind check in `rejection`: in the other-kind test the text is empty, so `rejection` is null whatever the kind. Dropping the check stays green. | Redemption rejection, payment hook, type the same amount, assert `rejection` null; prove with the mutation.      |
| C3-2 | minor    | MEDIUM     | `mobile/src/attempts/useAmountForm.test.tsx` (`inFlight`) | Test named "saving or sending" checks only `sending` → true; `inFlight = true` or `=== 'sending'` survive.                                           | Table: `saving`, `sending` → true; `idle`, `rejected` → false.                                                   |
