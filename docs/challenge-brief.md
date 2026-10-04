# Challenge Brief

The take-home brief as received, transcribed word for word. This file is never edited; interpretations are recorded in
`DECISIONS.md`.

---

# Take-Home — "Flash Cashback"

## Objective

This exercise is about judgment, not typing speed. AI can turn the rules below into a working feature in an hour. What
it won't do on its own is decide what has to be true before that feature is allowed near real money.

We want to see what you build when nobody tells you what "done" looks like.

## The problem

We're launching a flash cashback campaign.

The rules:

- Users earn **5% cashback** on every payment they make.
- Payments under **20,000 IDR** earn nothing.
- Each user can earn at most **50,000 IDR of cashback per day**.
- The campaign has a total budget of **10,000,000 IDR**. When it's gone, the campaign is over.
- Users can **redeem** their cashback balance.

## Scope

Build the Flash Cashback use case only.

**In scope** — payments, which earn cashback or don't according to the rules above, and what a user needs to see and do
around their cashback.

**Out of scope** — refunds and clawback of cashback already awarded. Authentication; assume you know who the user is.
Products, catalogue, stock; a payment is just an amount.

## What we expect

1. **Use AI to build this.** We do, and we expect you to.
2. **An MVP, but production grade.** Keep the scope small. Whatever you do ship should be ready to run in production —
   you decide what that means for a feature that moves real money.
3. **Work out what's missing.** The rules above describe the happy path. Think about what else has to be true before
   this can be trusted with real money, and decide what makes the cut.
4. **Stack: Go, PostgreSQL, Redis, React Native.**
5. **A working demo** we can run.
6. **Push it to your own GitHub account** and share the link with us.

## The interview

We'll use what you build as the basis for the technical interview: you'll demo it, then we'll go through your decisions
together — what you chose, what you rejected, and where you think it would break.
