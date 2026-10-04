---
name: programming-typescript
description: Standards and procedure for writing or changing TypeScript — strict compiler settings, typing rules, expected failures as values, promise handling, and the gates. Use before the first edit of any TypeScript change.
---

# Programming TypeScript

## Compiler

`tsconfig` enables `strict`, `noUncheckedIndexedAccess`, `noImplicitReturns`, `noFallthroughCasesInSwitch`,
`noUnusedLocals`, and `noUnusedParameters`. TypeScript is a pinned, direct dev dependency. An option is never relaxed
to make code compile; the code is fixed instead.

## Typing Rules

- **No `any`.** A value of unknown shape is `unknown`, narrowed before use.
- **Validate, never assert.** Data crossing a boundary (API response, storage, environment, parsed text) passes a
  schema or type guard before the code relies on its type. `as` and `!` do not silence a type error; a justified `!`
  carries an `eslint-disable` comment saying why.
- **Exclusive states are discriminated unions** with a literal tag, handled exhaustively. Never a cluster of booleans
  or optional fields that must not coexist. A request is `idle | sending | checking | done | failed`, not three flags.
- **Readonly by default** for data callers must not change; lookup tables are `as const`.
- **Server codes are unions with a fallback.** Codes received from an API are typed unions, and every mapping from
  code to copy has a default for a code this version does not know.
- **Money is an integer `number`** in the currency's smallest unit. No arithmetic on formatted strings; format only at
  render.
- **Business logic is functions over readonly data.** Effects (network, storage, clock) are passed in or stay in the
  shell.

## Failures

- A failure that is part of normal operation (validation error, rule not met, rate limited) is **returned** as
  a discriminated result, e.g. `{ kind: 'ok', value } | { kind: 'rejected', code } | { kind: 'unknown' }`. The caller
  must check it.
- Exceptions are for programmer errors and broken invariants.
- No swallowed error: handle it, wrap it with context, or propagate it.
- Every promise is awaited, returned, or given a rejection handler.

## Procedure

1. Read the tests that cover the code before changing it.
2. List the increments, simplest first, including failure paths.
3. Red, green, refactor per increment at the narrowest test target; **typecheck on every green**, not only at the end.
4. When the compiler rejects a change, fix the design, not the checker.

| While writing, you meet                          | Apply                       |
| ------------------------------------------------ | --------------------------- |
| data from the API, storage, or env               | validate at the edge        |
| a value whose shape is unknown                   | `unknown` and narrow        |
| two flags that must never be set together        | a discriminated union       |
| a call that can fail in normal use               | a returned result           |
| a promise started and not awaited                | await, return, or handle it |
| a business decision inside an effectful function | extract a pure function     |

## Before Handing Off

- Format check, lint (type-aware rules), typecheck, and tests pass; exit codes read directly.
- The diff has been read once for what no linter sees: a thrown error that callers should expect, an unvalidated
  boundary, an `as` hiding a real mismatch.
- No compiler option, lint rule, or coverage exclusion was relaxed.
