# C5 code check — focus refetch skips fresh queries (HEAD 41cfae6)

Verdict: **APPROVE_WITH_FINDINGS**. Gate `make mobile-check` exit 0 (27 suites, 294 tests). Reviewed after commit at
the owner's request; fixes go in a separate commit.

No finding on: focus timing (ref effect and `useFocusEffect` in the same component run in declaration order; the
focus event comes from the parent after the child's passive effects); `...queries` identity; `refetchAll` under the
React Compiler; pull and retry refetch all; first focus still skipped; AC-58 holds through the invalidation; moving the
clock in the two old tests follows the amended spec; screens changed only in removed memo blocks.

| ID | Severity | Confidence | File:line | Finding | Suggested fix |
| -- | -------- | ---------- | --------- | ------- | ------------- |
| F1 | minor | MEDIUM | `mobile/src/api/hooks.ts:38-41,50-53` | Focus reads the last render's snapshot. `useQuery` tracks read fields; the screens never read `isFetching`/`dataUpdatedAt` in render, so their changes do not re-render and the ref keeps `isFetching: true` after a fetch that ends with equal data; a later focus skips a needed refetch. The commit `Assumption:` covers only the extra-refetch direction. | Read live state at focus: `queryClient.getQueryState(key)` or `refetchQueries({ predicate })`; keep `refetchAll`. |
| F2 | minor | MEDIUM | `mobile/src/api/hooks.ts:52` | Reading `isFetching`/`dataUpdatedAt` through the tracked proxy in the focus callback adds them to the tracked set for good: after the first focus every fetch start/end re-renders the screen, partly undoing C4. The C4 render test never fires a focus. | F1's fix removes it; or a render-count test with a focus before the press. |
| F3 | minor | HIGH | `index.test.tsx:181-190`, `history.test.tsx:148-152` | `Date.now` spy leaks if an assertion fails before `mockRestore` (no `restoreMocks` in jest config). | `try/finally`, `afterEach(jest.restoreAllMocks)`, or fake timers as in `index.focus.test.tsx`. |
| F4 | minor | HIGH | `hooks.ts:52`, `index.focus.test.tsx` | No test proves "skip while fetching": removing `!q.isFetching` keeps all green. | Test: old data, cashback GET held pending, focus → no new GET; prove by the mutation. |
| F5 | minor | MEDIUM | `index.focus.test.tsx:62-69` | `afterRefetch` recorded but not asserted, so the 3 → 2 claim is not pinned. | `expect(afterRefetch).toBe(2)` before the focus. |
| F6 | minor | MEDIUM | `hooks.ts:34` | Comment: `dataUpdatedAt` is 0 only for a query that never succeeded. | Reword. |
| F7 | minor | MEDIUM | `plans/learnings.md` | No C5 row; F1/F2 justify one. | Add a row. |
