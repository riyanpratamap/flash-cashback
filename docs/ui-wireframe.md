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
| 1   | Home                     | app start                          | `GET /campaign`, `GET /me/cashback`, `GET /me/history` (five newest) |
| 2   | Make a payment           | Home, Payment result               | campaign rules and today's remaining, already loaded |
| 3   | Payment result           | Make a payment, Checking           | the `POST /payments` response                       |
| 4   | Checking                 | a payment or redemption with an unknown outcome | retries of the same request          |
| 5   | Redeem cashback          | Home                               | `GET /me/cashback`, `POST /redemptions`             |
| 6   | Transaction history      | Home "See all"                     | `GET /me/history`, `GET /me/cashback`               |
| 7   | How Flash Cashback works | "How it works" links               | campaign rules                                      |

States covered inside those screens: loading, load error, campaign ended, awards paused, redemptions paused, a Rp0
result, a partial award, empty history, zero balance.

There is no payment detail sheet; the history row itself shows the reason (D08).

## 1. Home

```
Flash Cashback
[DEMO] (User A) ( User B ) ( User C )

Cashback balance
Rp15.000                                  [ Redeem ]
[               Make a payment                     ]

+--------------------------------------------------+
| Campaign active                                   |
| Cashback up to 5% on payments of Rp20.000 or      |
| more, max Rp50.000 per day, while cashback lasts. |
| How it works                                      |
+--------------------------------------------------+
| Earned today               Rp47.000 / Rp50.000    |
| [=============================================  ] |
| Rp3.000 left to earn today. Resets at 00:00 WIB.  |
+--------------------------------------------------+

Recent activity                            See all
Cashback redeemed                        +Rp42.000
To main account
Payment                                 −Rp500.000
Earned Rp25.000 cashback
```

- **User switcher:** marked `DEMO`, stands in for login. It sets `X-User-ID`, reloads the screen, and the choice is
  remembered across launches.
- **Layout:** the balance hero and the Make a payment button come first; the balance is the only display-size text on
  Home. The campaign status strip sits below them, above Earned today.
- **Banner:** a compact tinted strip (info tint when `ACTIVE`, warning otherwise): its title in subhead semibold, its
  text in caption, padding 8 vertical / 12 horizontal, and the "How it works" link in caption size with its 44 pt
  target. Its content comes from `status`. The numbers come from `rules`, never from hard-coded text.
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
- **Recent activity:** the five newest items (D54), in the History row format (screen 6) without the time; a Rp0 payment
  has no subtitle line. Empty: "No activity yet."
- **Refresh:** on open, on return from another screen, and on pull to refresh.
- **Load error:** "Couldn't load your cashback. Your balance is safe. Check your connection and try again." with Try
  again. If only recent activity fails, the rest renders and that section says so.
- There is no "ending soon" banner (D08).

## 2. Make a payment

```
<  Make a payment

Amount (IDR)
[ 100.000                                          ]
(i) Earn up to Rp5.000 cashback. You'll see the exact
    amount after you pay.
( Rp20.000 ) ( Rp50.000 ) ( Rp100.000 )

[                Pay Rp100.000                     ]
```

With no amount typed, the slot under the field reads "Payments under Rp20.000 earn no cashback."

- **Amount:** digits only, formatted as typed. Chips fill it. Above Rp10.000.000: "Enter an amount up to
  Rp10.000.000."
- **One line slot** under the amount field, never two lines: the hint "Payments under Rp20.000 earn no cashback." (its
  numbers from the rules) while no amount is typed, and the info line once an amount is typed. An error
  (`INVALID_AMOUNT` or a rejection) takes the slot as the alert line. The footer holds only the Pay button.
- **Info line,** the only client-side estimate, always worded "up to":
  - below the minimum: "This payment won't earn cashback. The minimum is Rp20.000."
  - campaign ended: "This payment won't earn cashback. Flash Cashback has ended."
  - awards paused: "This payment won't earn cashback. Cashback is temporarily unavailable."
  - nothing left today: "This payment won't earn cashback. You've reached today's limit."
  - 5% is more than what is left today: the estimate is what is left, and the line reads "Earn up to Rp3.000
    cashback, the rest of today's Rp50.000 limit." (D02)
  - otherwise: "Earn up to Rp{estimate} cashback. You'll see the exact amount after you pay."
  - The estimate never knows the budget, so a `PARTIAL_BUDGET` result can be lower; "up to" covers it.
- **Pay button:** on press it disables at once, creates one idempotency key for this attempt, and sends the request.
- **Outcomes:** success opens screen 3. A timeout or server error opens screen 4. A rejected amount shows the inline
  error. Any other rejection shows "Something went wrong. Please try again."

## 3. Payment result

```
                      ( ✓ )
              Payment successful
                  Rp100.000
               3 Oct, 14:32 WIB
              PAY-20261003-000042

               ( +Rp5.000 cashback )

[                    Done                          ]
[             Make another payment                 ]
```

Top to bottom:

- the success mark: a 72 pt circle in the positive tint with a check, drawn without an image or icon; accessibility
  role image, label "Success";
- "Payment successful";
- the payment amount in display size, text colour;
- a muted caption with the time and zone, then the reference;
- the cashback pill, centred, caption size: above Rp0 positive text on the positive tint, "+Rp{award} cashback",
  followed by " · {chip}" when the reason has one; at Rp0 muted text on the track colour, "No cashback · {chip}";
  under it a centred "How it works" link on a partial award only;
- the footer: Done (primary) and Make another payment (secondary).

The content above the footer is centred vertically.

The payment always shows as successful. The payment leads, as the one large number; the cashback is the bonus on top,
in one small pill below it (D17). There is no card, no "Cashback earned" label, and no reason sentence. No balance is
shown here; Done returns to Home, which refetches. When the body cannot be parsed: the mark, the title, and the
attempt's amount, with no pill. An unknown code, or rules not loaded yet, shows the amount alone in the pill, with
no chip.

| Reason              | Amount     | Pill                                    |
| ------------------- | ---------- | --------------------------------------- |
| `AWARDED`           | +Rp{award} | "+Rp{award} cashback"                   |
| `PARTIAL_DAILY_CAP` | +Rp{award} | "+Rp{award} cashback · Daily limit reached" |
| `PARTIAL_BUDGET`    | +Rp{award} | "+Rp{award} cashback · Last of the cashback" |
| `DAILY_CAP_REACHED` | Rp0        | "No cashback · Daily limit reached"     |
| `BELOW_MINIMUM`     | Rp0        | "No cashback · Below minimum"           |
| `CAMPAIGN_ENDED`    | Rp0        | "No cashback · Campaign ended"          |
| `CAMPAIGN_PAUSED`   | Rp0        | "No cashback · Unavailable"             |
| unknown code        | as sent    | the amount alone ("+Rp{award} cashback" or "No cashback") |

"How it works" is offered only when some cashback was earned but less than the full rate (`PARTIAL_DAILY_CAP`,
`PARTIAL_BUDGET`); never on `AWARDED` and never at Rp0, an unknown code included. Cashback is credited at once; there is no pending state
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
- **Success:**
  - the success mark centred;
  - "Redemption successful";
  - the amount in display size, tabular;
  - one details card on a white surface, rows split by a hairline separator, label muted on the left, value on the
    right: Sent to · Main account; Reference; and Cashback balance · Rp{balance_after} only when the answer was a
    fresh 201 (a replay shows no balance); no separate "Sent to your main account" line;
  - when the body cannot be parsed: the mark, "Your redemption went through.", and "Check your balance on the home
    screen.";
  - the footer: Done only, returning Home.
  - The content is centred vertically above the footer, as on screen 3.

  ```
                        ( ✓ )
                Redemption successful
                      Rp18.000

  +--------------------------------------------------+
  | Sent to                             Main account |
  |--------------------------------------------------|
  | Reference                    RDM-20261003-000003 |
  |--------------------------------------------------|
  | Cashback balance                             Rp0 |
  +--------------------------------------------------+

  [                    Done                          ]
  ```
- **`INSUFFICIENT_BALANCE`:** the balance is refetched first, then "You can redeem up to Rp{balance}."
- **Unknown outcome:** screen 4.
- Works after the campaign has ended, unless redemptions are paused. The budget never refuses a redemption (D03).
- **Paused** (`redemption_status` is `PAUSED`, D05): the button is disabled and the screen says "Redemption is
  temporarily unavailable. Your balance is safe."
- **`REDEMPTION_PAUSED`** (the switch went off after the screen loaded): the campaign is refetched, and the screen
  shows the paused state above.

## 6. Transaction history

```
<  Transaction history

TODAY, 3 OCT
Payment                                 −Rp100.000
14:32 · Earned Rp5.000 cashback
Payment                                  −Rp15.000
13:05
Cashback redeemed                        +Rp42.000
11:20 · To main account

YESTERDAY, 2 OCT
Payment                                 −Rp200.000
19:45 · Earned Rp10.000 cashback
```

- Every payment (Rp0 ones included) and every redemption: a transaction list, not only cashback.
- No balance header: the list only; the balance lives on Home and Redeem.
- The right column is the money the transaction moved: a payment is `−` its amount, a redemption is `+` its amount
  (sent to the main account) in the positive colour.
- Title: "Payment" or "Cashback redeemed". Subtitle of a payment: the time, then "Earned Rp{awarded} cashback" (no
  sign), then the reason chip when the award was above Rp0 but less than the full 5%; a Rp0 payment shows the time
  only. Subtitle of a redemption: the time, then "To main account".
- Grouped by day, from the date in `created_at` as sent by the API, never converted to the device's time zone.
- Empty: "No activity yet. Make a payment to start earning cashback." Error: "Couldn't load your history." with Try
  again.
- **Paging (D54):** the first 20 load on open. Nearing the end of the list loads the next 20 and appends them, with
  a spinner in the list footer while it loads. A day that spans two pages keeps one header. If the next page fails,
  the rows already loaded stay and the footer reads "Couldn't load more." with Try again. Once the API answers
  `next_cursor: null`, nothing more is requested and the footer is empty.

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
