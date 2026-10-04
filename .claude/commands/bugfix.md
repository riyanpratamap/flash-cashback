---
description: Fix a bug — reproduce it with a failing regression test tied to the AC it breaks, then fix, review, and propose one fix commit.
argument-hint: <what is wrong, and how to see it>
---

Bug report from the owner: $ARGUMENTS

Follow the bug fix flow in `AGENTS.md` ("Changing the App"):

1. **Locate:** read `docs/prd.md` and find the AC this behaviour breaks. Read the related code, tests, and the
   `docs/known-pitfalls.md` sections for the tools involved.
   - If no AC covers the behaviour, stop: the spec has a gap, so this is an enhancement. Say so and suggest `/change`.
2. **Reproduce:** write one regression test, named `AC-xx: …`, that fails because of the bug. Run it and show the
   failure. Do not fix anything yet. If it will not reproduce, stop and report what you tried.
3. **Wait** for the owner to confirm the test fails for the reason they saw.
4. **Fix:** use the code-maker agent for the smallest change that turns the test green, with the full task gate.
5. **Review:** use the code-checker agent; confirm the regression test would fail on the old code. Apply confirmed
   findings with code-fixer.
6. **Close:** add a `plans/learnings.md` row (what broke, how it was caught, what changed) and propose one commit:
   `fix(<scope>): <subject>` with the test, the fix, and the learnings row. Do not commit.
