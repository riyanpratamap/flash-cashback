---
name: developing-mobile-ui
description: Standards and procedure for React Native screens, state, effects, and server data — function components, the narrowest home for each value, safe handling of requests that change server state (one idempotency key per attempt, unknown outcomes), accessibility, and user-level tests. Use before building or changing any screen, hook, or data flow in a mobile app.
---

# Developing Mobile UI

## Components

- Function components with a named props type; exclusive variants are a discriminated union.
- Rendering is pure: output comes from props and state; derived values are computed during render, not stored.
- Hooks are called unconditionally at the top level, named `use…`, with complete dependency lists.
- List keys are stable IDs, never array positions.
- Code is grouped by feature. Rules live in plain functions, adapted by a hook; the component only presents.
- Copy, layout, and states follow the project's screen specification. A screen shows nothing that specification does
  not describe.

## The Server Owns What It Computes

- The app never recomputes a value the server is the authority for (a balance, a total, a remaining quota) and then
  presents it as fact. It shows what the API returned. An estimate is allowed only where the screen specification says
  so, and it is labelled as an estimate.
- Amounts are integers from the API, formatted only at render.
- Days and times are shown in the time zone the API states, not silently converted to the device's.
- A load failure shows the error state, never a zero or an empty value that looks real.

## Requests That Change State

A request that creates or changes something on the server has four outcomes, and the UI models all four: accepted,
rejected (an answer the API defines as final), throttled, and **unknown** (a timeout, a dropped connection, or a server
error).

- **One idempotency key per attempt,** created when the user confirms, kept for the life of that attempt (a ref, not
  render-time state), reused on every retry, and discarded only on a definite answer.
- **Unknown is never shown as failed.** The app keeps checking with the same key.
- **Double submission is blocked synchronously** (a guard set in the handler before the request starts), in addition to
  the disabled button.
- Every request has a timeout. An aborted request may still have succeeded on the server; that is the unknown outcome.
- After a definite answer, the data that changed is refetched; nothing is patched by hand.

## Find Each Value's Home

Ask in order and stop at the first yes:

| Question                                                  | Home                        |
| --------------------------------------------------------- | --------------------------- |
| can it be computed from props, state, or cached data?     | computed during render      |
| does the server own it?                                   | the server-data cache       |
| does only one component use it?                           | that component's state      |
| do a parent and near children share it?                   | the parent, passed as props |
| does a distant part of the tree need it, rarely changing? | context                     |

Server data is never copied into component state, where it goes stale beside the cache.

## Question Every Effect

Before writing an effect, check: a derived value is computed in render; a response to a press belongs in the event
handler; server data comes from the data layer; resetting state on identity change uses a `key`. An effect that
survives synchronises with one external system (a timer, app focus) and cleans up what it starts.

## Accessibility

Every control has an accessible label and role; touch targets are at least 44 points; state is never shown by colour
alone; errors are announced, not only coloured.

## Tests

- Render the screen and act as a user: find by role, label, or visible text; press and type; assert on what appears.
  Never assert on internal state.
- Stub the network at the fetch boundary; test success, each rejection the screen handles, the unknown outcome, and
  loading.
- The retry path has a test proving the same key is sent again.

## Procedure

1. From the screen specification and the acceptance criteria, list each state and interaction (default, loading, error,
   empty, success, disabled, checking) as an increment.
2. Red, green, refactor per increment, starting from a failing user-level test.
3. When a component starts branching on a rule, move the rule into a pure function with its own test.

## Before Handing Off

- Typecheck, lint, and component tests pass.
- Each value sits in the narrowest home; no server data copied into state; nothing the server owns is recomputed.
- The changed screen has been seen once on a simulator against the running backend, including its error state.
