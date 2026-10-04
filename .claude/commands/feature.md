---
description: Add a new feature — spec it first (grill-me, new ACs, design), append a delivery phase, then build it task by task.
argument-hint: <the feature and why it is wanted>
---

Feature request from the owner: $ARGUMENTS

Follow the new feature flow in `AGENTS.md` ("Changing the App"). Stop at each checkpoint for the owner.

1. **Scope:** read the brief, `docs/DECISIONS.md`, `docs/prd.md`, and `docs/tech-spec.md`. Restate the feature in two
   lines and list what it touches (data model, API, UI, runtime). Confirm it is new behaviour, not a bug or a tweak;
   otherwise suggest `/bugfix` or `/change`.
2. **Questions:** use the spec-maker agent in amend mode to list the material open questions. Ask them here with the
   grill-me skill and record the answers in `docs/DECISIONS.md`. Everything else is a stated assumption.
3. **Spec:** use spec-maker to add new ACs (continuing the numbering) and the design changes to `docs/tech-spec.md`,
   then spec-checker for one round. Propose `docs(spec): add <feature>`. **Checkpoint: owner approves and commits.**
4. **Plan:** use plan-maker to append a new phase to `plans/delivery.md` with tasks, AC IDs, done-when, and a gate,
   then plan-checker for one round. Propose `docs(plan): add <feature> phase`. **Checkpoint: owner approves and
   commits.**
5. **Build:** for each task, the normal loop: code-maker, code-checker, code-fixer, then the owner reviews and commits.
6. **Close:** run the phase gate; if the feature touched a data store or the container setup, run the clean-clone check
   and any consistency check the project defines.
