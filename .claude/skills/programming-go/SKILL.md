---
name: programming-go
description: Standards and procedure for writing or changing Go — toolchain checks, error handling, context, typing of money and identifiers, concurrency, and tests. Use before the first edit of any Go change.
---

# Programming Go

## Toolchain

`gofmt` and `go vet` are mandatory; a further linter runs only if the project's decisions record one. The Go version is
pinned in `go.mod` and matched by the build image. A check is never silenced (`//nolint`, a build tag, an excluded
path) to make code pass; the code is fixed instead.

## Typing Rules

- **Money is an integer** in the currency's smallest unit (`int64`). No `float32`/`float64` anywhere near an amount, a
  rate, or a comparison of amounts. Rates are integers too (basis points). Multiply before dividing, and bound the
  inputs so the product cannot overflow.
- **Distinct values stay distinct:** identifiers and codes of different kinds are named types or validated values, not
  bare strings passed around. Codes with a fixed set of values are typed constants, handled exhaustively in a `switch`
  with a `default` that fails loudly.
- **Validate, never assume.** Data crossing a boundary (request body, header, query string, environment, a row from the
  database, a cached value) is parsed into a typed value before the code relies on it.
- **Zero values are decisions.** A missing row meaning "zero" is written explicitly at the repository, not left to an
  unchecked zero value.
- **Business logic is functions over values.** Effects (clock, database, cache, logging) are passed in as parameters or
  small interfaces defined where they are used; pure rule functions import none of them.

## Errors

- A failure that is part of normal operation (a rule not met, a duplicate, not found) is a **returned value** the
  caller must handle: a typed result or a sentinel/typed error checked with `errors.Is` / `errors.As`.
- Wrap with context where the meaning changes: `fmt.Errorf("load order %d: %w", id, err)`. Never compare error strings.
- No discarded error. `_ = f()` needs a comment saying why discarding is correct (the one standing case is the deferred
  rollback after a commit).
- `panic` is for programmer errors and broken invariants only; a recovery middleware turns it into a 500.

## Context

- `ctx context.Context` is the first parameter of every function that does I/O, and it is passed down, never stored in
  a struct.
- Every database and cache call takes the request context, so a timeout or disconnect cancels the work.
- Work that must finish after the response is decided (for example cache invalidation after a commit) uses a context
  detached from the request's cancellation, with its own short timeout.

## Concurrency

- Correctness between requests comes from the database (locks, constraints), never from a Go mutex: there can be more
  than one process.
- Every goroutine has an owner that waits for it (`sync.WaitGroup`, `errgroup`) and a way to stop.
- No shared mutable state without a stated guard. `go test -race` is part of the gate.

## Tests

- Table-driven tests for pure rules, one row per worked example, named by the case.
- Integration tests use the real database and cache, never a mock of them, and reset state between tests.
- From a goroutine inside a test, report with `t.Errorf` (or collect and assert afterwards), never `t.Fatal`.
- A test that needs "now" or "today" gets it from a parameter or the database, not `time.Now()`.

## Procedure

1. Read the tests that cover the code before changing it.
2. List the increments, simplest first, including failure paths.
3. Red, green, refactor per increment at the narrowest test target (`go test ./internal/<pkg> -run TestName`);
   `go vet` on every green, not only at the end.
4. When the compiler or `vet` rejects a change, fix the design, not the checker.

| While writing, you meet                          | Apply                                       |
| ------------------------------------------------ | ------------------------------------------- |
| an amount, a rate, a limit                       | `int64`, integer math, a stated rounding    |
| data from a request, header, env, or cache       | parse into a typed value at the edge        |
| a call that can fail in normal use               | a returned result the caller must check     |
| an error from a driver                           | map it at the repository; wrap with context |
| a decision that needs the date or the clock      | take it as a parameter or ask the database  |
| a business decision inside an effectful function | extract a pure function                     |
| two requests that could interleave               | a database lock or constraint, plus a test  |

## Before Handing Off

- `gofmt -l` prints nothing; `go vet ./...` and the tests pass with `-race`; exit codes read directly.
- The diff has been read once for what no tool sees: a float near money, an unchecked error, a context dropped, a rule
  computed outside its pure function.
- No check, test, or assertion was relaxed.
