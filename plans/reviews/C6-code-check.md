# C6 code check — mobile restyle

**Verdict:** APPROVE_WITH_FINDINGS

**Gate:** `make mobile-check` exit 0 (ESLint `--max-warnings 0`; Jest 27/27
suites, 297/297 tests; one pre-existing `act(...)` console.error in
`index.test.tsx`). Raw `fontSize`/hex grep and `<Text[ >]` grep in
`src/app` print nothing. Changes only in `src/ui/**`, `src/app/*.tsx`,
`payment-result.test.tsx` (one hunk), wireframe screen 3, delivery.md,
learnings.md.

**Verified correct:** press-path guards and `disabled` values unchanged;
conditional renders, roles, labels, states and `checking-spinner` testID
kept; `AppText` passes every prop through (caller `style` last);
`ActivityRow` `last` correct on Home and per History day group;
payment-result renders with `result` null (attempt amount only) and with
`rules` undefined (no zone suffix, no crash); every contrast pair in the
`theme.ts` comment recomputed and exact; `FormScreen` has no measurement
loop and the iOS offset is correct.

| ID | Severity | Confidence | File:line | Finding | Suggested fix |
| --- | --- | --- | --- | --- | --- |
| F1 | major | MEDIUM | mobile/src/ui/FormScreen.tsx:22 | Android `behavior` is `undefined`; with edge-to-edge on SDK 57 the window may not resize, so the pinned Pay/Redeem footer can sit under the number pad (before C6 it was reachable by scroll). | Run on Expo Go Android with the keyboard open; use `behavior="padding"` (or `height`) on Android too. |
| F2 | minor | MEDIUM | mobile/src/ui/Button.tsx:57-58 | Fixed `height` 50/36 replaces `minHeight`/`minWidth` 44; large text or a wrapped label is clipped. | `minHeight: 50` / `36`, keep `minWidth: 44`. |
| F3 | minor | MEDIUM | mobile/src/ui/Button.tsx:24 | `hitSlop` 8 on all sides with an 8 pt row gap: adjacent pill touch areas overlap (switcher can pick the neighbour user). | Vertical slop only (`{ top: 8, bottom: 8 }`). |
| F4 | minor | HIGH | mobile/src/ui/theme.ts:25 | primaryStrong on primaryTintPressed #CFE5FA is 4.35:1 (< 4.5), missing from the comment. | Lighten to ~#D6E9FB or drop the pressed tint; add the pair to the comment. |
| F5 | minor | MEDIUM | mobile/src/app/index.tsx:327, history.tsx:159, payment-result.tsx:636 | `Card` `gap: 8` plus row padding gives 12 above / 20 below each hairline; Redeem details differ. | `gap: 0` on the list/details card styles. |
| F6 | minor | MEDIUM | mobile/src/app/index.tsx:327-350 | Empty/loading/error activity text sits in the list Card with 4 pt vertical padding, cramped. | Own vertical padding (12) or render outside the list Card. |
| F7 | minor | MEDIUM | mobile/src/ui/FormScreen.tsx:29 | Footer keeps the bottom safe-area inset while the keyboard is up: ~34 pt extra gap on home-indicator iPhones. | Accept, or drop the inset while the keyboard is shown. |
| F8 | minor | MEDIUM | mobile/src/app/redeem.tsx:753-787, pay.tsx:503-507 | Reading order changed (Redeem: balance rows below the input; Pay: info line in the footer) without an `Assumption:` line. | Add an `Assumption:` footer naming the reorder. |
| F9 | minor | MEDIUM | mobile/src/ui/Button.tsx:47 | `LinkText` has no underline, so links are distinguished by colour only (WCAG 1.4.1). | Keep the underline. |

## Triage (main session)

- F1–F7: sent to code-fixer.
- F8: handled in the commit message.
- F9: not applied — the owner's brief says "LinkText: primary colour,
  no underline". Raised to the owner.
