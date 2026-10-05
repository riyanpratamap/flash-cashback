# UI Wireframe

The screens of the Flash Cashback app, drafted before the build. Data comes from the endpoints in
[api-contract.md](api-contract.md).

Final after the decision session. Choices are recorded in [DECISIONS.md](DECISIONS.md) and cited by ID.

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

States covered inside those screens: loading, load error, campaign ended, awards paused, redemptions paused, a Rp0
result, a partial award, empty history, zero balance.

There is no payment detail sheet; the history row itself shows the reason (D08).

## 1. Home

```
Flash Cashback
[DEMO] (User A) ( User B ) ( User C )

+--------------------------------------------------+
| Campaign active                                   |
| Cashback up to 5% on payments of Rp20.000 or      |
| more, max Rp50.000 per day, while cashback lasts. |
| How it works                                      |
+--------------------------------------------------+
| Cashback balance   Rp15.000           [ Redeem ]  |
+--------------------------------------------------+
| Earned today               Rp47.000 / Rp50.000    |
| [=============================================  ] |
| Rp3.000 left to earn today. Resets at 00:00 WIB.  |
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
  - `PAUSED`: "Cashback is temporarily unavailable. Payments still work as usual." Neutral, not red. The Earned today
    card stays.
  - `ENDED`: "Flash Cashback has ended. All cashback has been claimed. Payments still work as usual, and you can
    still redeem your balance." The Earned today card is removed and Make a payment becomes a secondary button.
- **Balance card:** Redeem is disabled at balance 0. When `redemption_status` is `PAUSED`, Redeem is disabled and the
  card says "Redemption is on hold and your balance is safe." (D05). If the campaign has also ended, the `ENDED`
  banner drops ", and you can still redeem your balance".
- **Earned today:** earned, cap, a progress bar, and what is left, then "Resets at 00:00 WIB." (D04). At 0 left:
  "You've reached today's limit. Resets at 00:00 WIB."
- **Recent activity:** the two newest items. Empty: "No activity yet."
- **Refresh:** on open, on return from another screen, and on pull to refresh.
- **Load error:** "Couldn't load your cashback. Your balance is safe. Check your connection and try again." with Try
  again. If only recent activity fails, the rest renders and that section says so.
- There is no "ending soon" banner (D08).

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
  - campaign ended: "This payment won't earn cashback. Flash Cashback has ended."
  - awards paused: "This payment won't earn cashback. Cashback is temporarily unavailable."
  - nothing left today: "This payment won't earn cashback. You've reached today's limit."
  - 5% is more than what is left today: the estimate is what is left, and the line reads "Earn up to Rp3.000
    cashback, the rest of today's Rp50.000 limit." (D02)
  - otherwise: "Earn up to Rp{estimate} cashback. Final amount is confirmed after payment."
  - The estimate never knows the budget, so a `PARTIAL_BUDGET` result can be lower; "up to" covers it.
- **Pay button:** on press it disables at once, creates one idempotency key for this attempt, and sends the request.
- **Outcomes:** success opens screen 3. A timeout or server error opens screen 4. A rejected amount shows the inline
  error. Any other rejection shows "Something went wrong. Please try again."

## 3. Payment result

```
              Payment successful

              Cashback earned
                 +Rp5.000
        5% cashback added to your balance.

+--------------------------------------------------+
| Amount                                 Rp100.000 |
| Reference                    PAY-20261003-000042 |
| Time                            3 Oct, 14:32 WIB |
+--------------------------------------------------+

[                    Done                          ]
[             Make another payment                 ]
```

The payment always shows as successful. The cashback leads, as the one large number, then the reason; the payment
itself is a quiet two-column list of Amount, Reference and Time. No balance is shown here; Done returns to Home, which
refetches.

| Reason              | Amount     | Chip                | Text                                                          |
| ------------------- | ---------- | ------------------- | ------------------------------------------------------------- |
| `AWARDED`           | +Rp{award} | none                | "5% cashback added to your balance."                          |
| `PARTIAL_DAILY_CAP` | +Rp{award} | Daily limit reached | "You've reached today's Rp50.000 cashback limit."             |
| `PARTIAL_BUDGET`    | +Rp{award} | Last of the cashback | "This was the last of the campaign cashback. Flash Cashback has now ended." |
| `DAILY_CAP_REACHED` | Rp0        | Daily limit reached | "You've already reached today's Rp50.000 cashback limit."     |
| `BELOW_MINIMUM`     | Rp0        | Below minimum       | "Payments under Rp20.000 don't earn cashback."                |
| `CAMPAIGN_ENDED`    | Rp0        | Campaign ended      | "Flash Cashback has ended. All cashback has been claimed."    |
| `CAMPAIGN_PAUSED`   | Rp0        | Unavailable         | "Cashback is temporarily unavailable. Payments still work as usual." |
| unknown code        | as sent    | none                | "See How it works for the cashback rules."                    |

"How it works" is offered on every variant except `AWARDED`. Cashback is credited at once; there is no pending state
(D07, trust condition 18).

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

- Shown when a payment or redemption times out, loses the connection, or gets a server error (including 503
  `SERVICE_BUSY`). The outcome is unknown: the request may have gone through.
- The app resends the same request with the **same idempotency key**, three times about two seconds apart. Then the
  spinner stops and Check again sends it once more, as often as the user presses.
- The word "failed" never appears for an unknown outcome.
- Leaving the screen is blocked while checking, so the key is not lost and the user is not led to pay again.
- For a redemption: "Checking your redemption..." and "Please don't redeem again."
- **Killed while checking (D48):** before sending, the app saves the attempt (user ID, payment or redemption, amount,
  key, time) and clears it on a definite answer. On the next launch, if one is saved:
  - **Recent:** Checking reopens and resends it with the same key and the saved user ID.
  - **Old:** Home shows a card instead of resending: "A payment of Rp100.000 from 3 Oct, 14:32 wasn't confirmed."
    with **Check now** (opens Checking, same key) and **Dismiss** ("Check your history before paying again."). For a
    redemption the card says "A redemption of …".
  - The attempt is always resent as the user it was made by, whichever demo user is selected.
  - Assumption: "recent" means under 10 minutes old.

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
- Works after the campaign has ended, unless redemptions are paused. The budget never refuses a redemption (D03).
- **Paused** (`redemption_status` is `PAUSED`, D05): the button is disabled and the screen says "Redemption is
  temporarily unavailable. Your balance is safe."
- **`REDEMPTION_PAUSED`** (the switch went off after the screen loaded): the campaign is refetched, and the screen
  shows the paused state above.

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
- Shows the newest 20 and stops; no loading more on scroll (D08).

## 7. How Flash Cashback works

```
<  How Flash Cashback works

[ Pay Rp100.000                       earn Rp5.000 ]

Up to 5% cashback
On every payment of Rp20.000 or more. Payments under Rp20.000 don't earn cashback.

Up to Rp50.000 per day
The limit resets at 00:00 WIB. A payment that reaches the limit earns what is left of it, so it can earn less than
5%.

While cashback lasts
The campaign ends when all cashback has been claimed. The last payment may earn less than 5%. Payments after that
still work, without cashback.

Redeem anytime
Your balance stays redeemable after the campaign ends. Redeemed cashback goes to your main account.

Rounded down
Cashback is rounded down to the nearest rupiah.
```

The numbers come from the campaign rules. In production this page would be server-driven content.

## Assumptions

- Redemption success is a confirmation followed by a return to Home; there is no separate result screen.
- The demo user switcher would not exist in production.
