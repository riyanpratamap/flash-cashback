# UI Wireframe

The screens of the Flash Cashback app, drafted before the build. Data comes from the endpoints in
[api-contract.md](api-contract.md).

Copy that depends on one of the open decisions (listed in `AGENTS.md`) is marked:

> **OPEN — decision N.** The variants.

After the decision session the markers are replaced by the chosen copy and the owner approves the diff.

Worked example used throughout: **User A**, balance Rp15.000, earned today Rp47.000 of Rp50.000.

## Conventions

- **Amounts:** `Rp` and the integer with `.` as the thousands separator: `Rp100.000`. Earned cashback is shown with
  `+`, redemptions with `−`, a zero award as `Rp0` with no sign.
- **Colour is never the only signal.** A Rp0 result is not an error and is never red.
- **The server owns the money.** The app shows what the API returned. The one number it computes is the "Earn up to"
  estimate on the Pay screen, and it is labelled as an estimate.
- **Codes to copy:** the app maps reason, status, and error codes to the copy in this file. An unknown code shows
  generic text and never crashes.
- **No budget figures anywhere.**
- **A value that failed to load is never shown as zero.**

## Screen map

| #   | Screen                   | Reached from                       | Data                                                |
| --- | ------------------------ | ---------------------------------- | --------------------------------------------------- |
| 1   | Home                     | app start                          | `GET /campaign`, `GET /me/cashback`, `GET /me/history` (two newest) |
| 2   | Make a payment           | Home, Payment result               | campaign rules and today's remaining, already loaded |
| 3   | Payment result           | Make a payment, Checking           | the `POST /payments` response                       |
| 4   | Checking                 | a payment or redemption with an unknown outcome | retries of the same request          |
| 5   | Redeem cashback          | Home                               | `GET /me/cashback`, `POST /redemptions`             |
| 6   | Cashback history         | Home "See all"                     | `GET /me/history`, `GET /me/cashback`               |
| 7   | How Flash Cashback works | "How it works" links               | campaign rules                                      |

States covered inside those screens: loading, load error, campaign ended, a Rp0 result, empty history, zero balance.

> **OPEN — decision 8 (scope).** A payment detail sheet (tap a history row to see the reference, date, and the reason
> in the past tense) is a candidate. Without it, the history row itself shows the reason.

## 1. Home

```
Flash Cashback
[DEMO] (User A) ( User B ) ( User C )

+--------------------------------------------------+
| Campaign active                                   |
| 5% cashback on payments of Rp20.000 or more, up   |
| to Rp50.000 per day, while quota lasts.           |
| How it works                                      |
+--------------------------------------------------+
| Cashback balance   Rp15.000           [ Redeem ]  |
+--------------------------------------------------+
| Earned today               Rp47.000 / Rp50.000    |
| [=============================================  ] |
| Rp3.000 left to earn today.                       |
+--------------------------------------------------+
[               Make a payment                     ]

Recent activity                            See all
Redeemed to main account                 −Rp42.000
Payment Rp500.000                        +Rp25.000
```

- **User switcher:** marked `DEMO`, stands in for login. It sets `X-User-ID`, reloads the screen, and the choice is
  remembered across launches.
- **Banner:** from `status`. The numbers come from `rules`, never from hard-coded text.
  - `ACTIVE`: "Campaign active" and the rule line above.
  - `ENDED`: "Flash Cashback has ended. All cashback has been claimed. Payments still work as usual, and you can
    still redeem your balance." The Earned today card is removed and Make a payment becomes a secondary button.
- **Balance card:** Redeem is disabled at balance 0.
- **Earned today:** earned, cap, a progress bar, and what is left. At 0 left: "You've reached today's limit."
- **Recent activity:** the two newest items. Empty: "No activity yet."
- **Refresh:** on open, on return from another screen, and on pull to refresh.
- **Load error:** "Couldn't load your cashback. Your balance is safe. Check your connection and try again." with Try
  again. If only recent activity fails, the rest renders and that section says so.

> **OPEN — decision 4 (per day).** With a calendar day in one time zone, the earned line ends "Resets at 00:00 WIB."
> With a rolling 24 hours there is no reset time to show and the line needs other wording.

> **OPEN — decision 5 (kill switch).** If a switch exists, a `PAUSED` banner: "Cashback is temporarily unavailable.
> Payments still work as usual." If the switch also stops redemptions, add "Redemption is on hold and your balance is
> safe." and disable Redeem.

> **OPEN — decision 8 (scope).** If `ENDING_SOON` is kept: an amber banner "Ending soon. Most of the cashback has been
> claimed. It may run out before your next payment."

## 2. Make a payment

```
<  Make a payment

Amount (IDR)
[ 100.000                                          ]
Payments under Rp20.000 earn no cashback.
( Rp20.000 ) ( Rp50.000 ) ( Rp100.000 )

(i) Earn up to Rp5.000 cashback. Final amount is
    confirmed after payment.

[                Pay Rp100.000                     ]
```

- **Amount:** digits only, formatted as typed. Chips fill it. Above Rp10.000.000: "Enter an amount up to
  Rp10.000.000."
- **Info line,** the only client-side estimate, always worded "up to":
  - below the minimum: "This payment won't earn cashback. Payments under Rp20.000 earn no cashback."
  - nothing left today: "This payment won't earn cashback. You've reached today's limit."
  - campaign ended: "This payment won't earn cashback. Flash Cashback has ended."
  - otherwise: "Earn up to Rp{estimate} cashback. Final amount is confirmed after payment."
- **Pay button:** on press it disables at once, creates one idempotency key for this attempt, and sends the request.
- **Outcomes:** success opens screen 3. A timeout or server error opens screen 4. A rejected amount shows the inline
  error. Any other rejection shows "Something went wrong. Please try again."

> **OPEN — decision 2 (award at a limit).**
> A. Partial award: when the 5% is more than what is left today, the estimate is what is left and the line reads
> "Earn up to Rp3.000 cashback, the rest of today's Rp50.000 limit."
> B. All or nothing: when the 5% is more than what is left today, the line reads "This payment won't earn cashback.
> It would go over today's Rp50.000 limit."

## 3. Payment result

```
                     ( ✓ )
              Payment successful
                 Rp100.000
   Ref. PAY-20261003-000042 · 3 Oct, 14:32 WIB

+--------------------------------------------------+
| Cashback earned                         +Rp5.000  |
| 5% cashback added to your balance.                |
+--------------------------------------------------+

[                    Done                          ]
[             Make another payment                 ]
```

The payment always shows as successful. Cashback is a separate card that leads with what the user got, then the
reason. No balance is shown here; Done returns to Home, which refetches.

| Reason              | Amount     | Chip                | Text                                                          |
| ------------------- | ---------- | ------------------- | ------------------------------------------------------------- |
| `AWARDED`           | +Rp{award} | none                | "5% cashback added to your balance."                          |
| `DAILY_CAP_REACHED` | Rp0        | Daily limit reached | "You've already reached today's Rp50.000 cashback limit."     |
| `BELOW_MINIMUM`     | Rp0        | Below minimum       | "Payments under Rp20.000 don't earn cashback."                |
| `CAMPAIGN_ENDED`    | Rp0        | Campaign ended      | "Flash Cashback has ended. All cashback has been claimed."    |
| unknown code        | as sent    | none                | "See How it works for the cashback rules."                    |

"How it works" is offered on every variant except `AWARDED`.

> **OPEN — decision 2 (award at a limit).** Variant A adds two rows: `PARTIAL_DAILY_CAP` ("+Rp3.000", chip "Daily
> limit reached", "You've reached today's Rp50.000 cashback limit.") and `PARTIAL_BUDGET` ("+Rp1.200", chip "Last of
> the cashback", "This was the last of the campaign cashback. Flash Cashback has now ended.").

> **OPEN — decision 5 (kill switch).** If a switch exists: `CAMPAIGN_PAUSED`, Rp0, chip "Unavailable", "Cashback is
> temporarily unavailable. Payments still work as usual."

> **OPEN — decision 7 (real-money risks).** If cashback is granted only after settlement, this card shows a pending
> amount and says when it becomes redeemable.

## 4. Checking

```
                     ( ◌ )
           Checking your payment...
                 Rp100.000
   This is taking longer than usual. Please don't
   pay again. This screen updates as soon as we
               have the result.

[                 Check again                      ]
```

- Shown when a payment or redemption times out, loses the connection, or gets a server error. The outcome is
  unknown: the request may have gone through.
- The app resends the same request with the **same idempotency key**, three times about two seconds apart. Then the
  spinner stops and Check again sends it once more, as often as the user presses.
- The word "failed" never appears for an unknown outcome.
- Leaving the screen is blocked while checking, so the key is not lost and the user is not led to pay again.
- For a redemption: "Checking your redemption..." and "Please don't redeem again."

## 5. Redeem cashback

```
<  Redeem cashback

| Available to redeem                    Rp18.000   |
Amount to redeem (IDR)
[ 18.000                                           ]
Up to Rp18.000                        ( Redeem all )
| Sent to                             Main account  |

[               Redeem Rp18.000                    ]
```

- The balance is refetched when the screen opens. Redeem all fills it. Above the balance: "You can redeem up to
  Rp{balance}." No minimum.
- The button is disabled at balance 0, or an empty or invalid amount. On press it disables at once and creates one
  idempotency key.
- **Success:** "Rp{amount} sent to your main account. Your balance is now Rp{balance_after}." then Done returns to
  Home.
- **`INSUFFICIENT_BALANCE`:** the balance is refetched first, then "You can redeem up to Rp{balance}."
- **Unknown outcome:** screen 4.
- Works after the campaign has ended.

> **OPEN — decision 5 (kill switch).** If the switch stops redemptions: while paused the button is disabled and the
> screen says "Redemption is temporarily unavailable. Your balance is safe."

> **OPEN — decision 3 (when the budget is spent).** If the budget is spent at redemption, a redemption can be refused
> because the budget is gone, and this screen needs copy for it.

## 6. Cashback history

```
<  Cashback history

[ Current balance                        Rp18.000 ]

TODAY, 3 OCT
Payment Rp100.000                         +Rp5.000
14:32
Payment Rp15.000                               Rp0
13:05 · Below minimum
Redeemed to main account                 −Rp42.000
11:20

YESTERDAY, 2 OCT
Payment Rp200.000                        +Rp10.000
19:45
```

- The header is the current balance, not a running balance per row.
- Rows show the title, the amount, and the time. A payment that earned less than the full 5% also shows its reason.
  Rp0 payments are listed. Redemptions are neutral with `−`.
- Grouped by day, from the date in `created_at` as sent by the API, never converted to the device's time zone.
- Empty: "No activity yet. Make a payment to start earning cashback." Error: "Couldn't load your history." with Try
  again.

> **OPEN — decision 8 (scope).** Variant A shows the newest 20 and stops. Variant B loads more on scroll with a cursor.

## 7. How Flash Cashback works

```
<  How Flash Cashback works

[ Pay Rp100.000                       earn Rp5.000 ]

5% cashback
On every payment of Rp20.000 or more. Payments under Rp20.000 don't earn cashback.

Up to Rp50.000 per day
…

While cashback lasts
The campaign ends when all cashback has been claimed. Payments after that still work, without cashback.

Redeem anytime
Your balance stays redeemable after the campaign ends. Redeemed cashback goes to your main account.

Rounded down
Cashback is rounded down to the nearest rupiah.
```

The numbers come from the campaign rules. In production this page would be server-driven content.

> **OPEN — decision 4 (per day).** The "Up to Rp50.000 per day" paragraph states when the limit resets.

> **OPEN — decision 2 (award at a limit).** The same paragraph states what a payment that reaches the limit earns:
> the remaining amount (A) or nothing (B).

## Assumptions

- Redemption success is a confirmation followed by a return to Home; there is no separate result screen.
- The demo user switcher would not exist in production.
