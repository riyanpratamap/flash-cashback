---
name: grill-me
description: Resolve a material open decision by presenting 2–4 mutually exclusive options with one recommendation, waiting for the owner, and recording the answer in docs/DECISIONS.md. Use only for questions that survive the ask-last filter in AGENTS.md rule 1.
---

# Grill Me

A decision protocol for choices that belong to the owner. Its purpose is a visible, owned, recorded decision, not a
long questionnaire.

## Filter First

A question reaches the owner only if all of these hold:

- the brief, `docs/DECISIONS.md`, and the repo do not already answer it;
- a safe probe (reading docs, running a command) cannot answer it;
- it is **material**: a product rule, a stack or library choice, the data model, a correctness or concurrency
  rule, security, or something cross-cutting or hard to reverse.

Everything else becomes a stated assumption (`Assumption: …`) in the spec, report, or commit body. Copy, labels, error
wording, and local implementation details are never grill-me questions. Stay within the decision budget in `AGENTS.md`
rule 2.

## Open Decisions and Batches

`AGENTS.md` names which decisions are open and which choices are confirmed in batches.

- **Open decisions** go through "Asking" below, one per message, in the order `AGENTS.md` gives. The owner's choice and
  the owner's reason are recorded as given. Do not polish, lengthen, or replace the reason; if it is unclear, ask one
  follow-up question. If the owner asks what an option would cost, answer; do not push the recommendation again.
- **Batches** are shown as one table each (proposed ID · decision · chosen · reason) and confirmed once: confirm all,
  or name the rows to change or discuss. A named row goes through "Asking".
- **Conflicts** between approved inputs are shown as a table (where they disagree · proposed fix) and confirmed the
  same way.

If `docs/DECISIONS.md` does not exist, create it with a one-paragraph header and a summary table (ID · decision ·
chosen), then one entry per decision. IDs start at `D01` and are never reused. Open decisions are recorded first.

## Asking

- First show the whole list of questions that survived the filter, ordered so that dependencies come first.
- **One decision per message.** Wait for the answer.
- **2–4 mutually exclusive options**, each with its trade-off in one line.
- **Exactly one recommendation**, first, with why it fits this project's brief and constraints.
- Always allow "something else" and "let's discuss".

```
D07 — Session storage: where does the signed-in session live?
| Option | Trade-off |
| A. Server-side session with an httpOnly cookie (recommended) | Revocable at once; needs a session store |
| B. Signed token in an httpOnly cookie | No store; cannot be revoked before it expires |
| C. Token in client storage | Simplest client code; readable by any injected script |
Recommendation: A — revocation matters more here than avoiding one table.
Or: something else / let's discuss.
```

## Recording

Append to `docs/DECISIONS.md` and add a row to its summary table:

```
### D07 — Session storage
- **Options:** A. server-side session · B. signed token in a cookie · C. token in client storage
- **Recommended:** A
- **Chosen:** A (add "against recommendation" when it differs)
- **Rationale:** the owner's reason, in their words
- **Cost accepted:** what this choice gives up, as the owner stated it
- **Would revisit if:** the condition under which another option becomes right
```

Before recording a rule that depends on engine or library behaviour, check that behaviour. A recommendation is never a
resolution: only the owner's explicit answer is.
