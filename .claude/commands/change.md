---
description: Triage a request — classify it as tiny, bug, enhancement, or feature, propose the route, and run it after the owner agrees.
argument-hint: <what should be different>
---

Request from the owner: $ARGUMENTS

1. **Read** the brief, `docs/DECISIONS.md`, `docs/prd.md`, `docs/tech-spec.md`, and the code this touches.
2. **Classify** it, with one line of reasoning:

| Type        | Test                                                   | Route                                               |
| ----------- | ------------------------------------------------------ | --------------------------------------------------- |
| tiny        | no behaviour change (copy, config, naming)             | code-maker with an `Assumption:`, then code-checker |
| bug         | the app contradicts an existing AC                     | `/bugfix` flow                                      |
| enhancement | an existing behaviour should change (an AC is amended) | enhancement flow below                              |
| feature     | new behaviour the spec does not describe               | `/feature` flow                                     |

3. **Propose** the route and its first step, then **wait** for the owner to agree.
4. **Run it.** The enhancement flow: grill-me only if the change is material; amend the AC in `docs/prd.md` (keep its
   ID, add `a`, `b` for split criteria) and `docs/tech-spec.md`, and propose `docs(spec): …`; append the tasks to a
   `Changes` phase in `plans/delivery.md`; then the normal build loop, one `feat` or `fix` commit per task.
