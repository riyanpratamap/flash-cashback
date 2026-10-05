# C4 code check — split the attempt context

Verdict: **APPROVE_WITH_FINDINGS**. Gate `make mobile-check` exit 0 (26 suites, 288 tests).

Checked with no finding: every former `setUnconfirmed` path goes through `updateUnconfirmed`, which applies the change
to the ref first, so two updates in one tick chain and none is lost; guard, `settle`, `refreshAfter`, `checkAgain`,
`acknowledge`, and the launch queue (takeover order, early return) match HEAD line by line; the new comments are
accurate; callers use the narrowest hooks and `AttemptNavigator` still acts once per state object; existing test edits
are mechanical (hook names only, no assertion changed); `index.renders.test.tsx` counts renders soundly (`useCampaign`
has one call site in Home, hooks run every render under the React Compiler); the reported mutation is plausible.

| ID | Severity | Confidence | File:line | Finding | Suggested fix |
| -- | -------- | ---------- | --------- | ------- | ------------- |
| F1 | major | HIGH (no test); LOW (live bug) | `mobile/src/attempts/AttemptProvider.tsx:73,89-91,147` | `press` reads `userRef`, synced in a passive `useEffect`; at HEAD it closed over the rendered user. The rule "an attempt carries the user selected at press time" (D48) now rests on React flushing passive effects after a sync commit, and no test switches the user then presses, so deleting the sync would stay green. | Sync in `useLayoutEffect`; add a test: switch user_a → user_b on Home, press pay, assert POST `X-User-ID: user_b` and saved `user_id: 'user_b'`; prove by removing the sync (red). |
