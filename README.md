# Flash Cashback

A cashback campaign for payments: 5% back, at least Rp20.000 per payment, at most Rp50.000 a day per user, from a
Rp10.000.000 budget. Go API, PostgreSQL, Redis, React Native app. Money is integer IDR and the rules are decided in
PostgreSQL inside the payment transaction.

How it is made safe for money, the rules as interpreted, and its limits are in
[docs/design-overview.md](docs/design-overview.md).

## Demo

Recorded on the app against the [demo state](#demo-users). Reason codes are in [docs/api-contract.md](docs/api-contract.md).

<table>
  <tr>
    <td width="50%" valign="top">
      <b>Full cashback</b><br>
      <video src="https://github.com/user-attachments/assets/ddde82c8-5707-4f29-b48a-4591b318ef4f" controls width="100%"></video>
    </td>
    <td width="50%" valign="top">
      <b>Partial cashback</b><br>
      <video src="https://github.com/user-attachments/assets/6ccca6f6-b7a2-4bad-a120-4db07e517694" controls width="100%"></video>
    </td>
  </tr>
  <tr>
    <td width="50%" valign="top">
      <b>No cashback</b><br>
      <video src="https://github.com/user-attachments/assets/8504b1ae-69eb-4040-b06e-e655898c7f8f" controls width="100%"></video>
    </td>
    <td width="50%" valign="top">
      <b>Redeem</b><br>
      <video src="https://github.com/user-attachments/assets/c0f457c7-c7a9-40ac-902e-2cfc8d61aa4a" controls width="100%"></video>
    </td>
  </tr>
</table>

## Prerequisites

| Need                          | For                         | Notes                                                       |
| ----------------------------- | --------------------------- | ----------------------------------------------------------- |
| Docker with Compose v2        | API, PostgreSQL, Redis      | Port 8080 free on the host. No `.env` is needed.            |
| Node 20.19+ or 22.13+, npm    | the mobile app              | Required by React Native 0.86                               |
| Expo Go (SDK 57) on a phone   | the mobile app, any OS      | Or the iOS simulator on a Mac (Xcode)                       |
| Go 1.26                       | running the tests only      | Not needed to run the demo                                  |

## Quick start

**1. Start the stack.** Migrations and the campaign seed run on first boot.

```sh
docker compose up -d --build --wait
curl -fsS localhost:8080/v1/healthz
```

Expect `{"status":"ok","dependencies":{"postgres":"ok","redis":"ok"}}`.

**2. Load the demo state.** `demo-reset` refuses unless `FC_DEMO=1` (exit 2, nothing changed). It truncates the money
tables, so use it only on a demo database.

```sh
docker compose exec -e FC_DEMO=1 api /app/admin demo-reset
```

Expect exit 0 and one `demo_reset` JSON line.

**3. Open the app.**

```sh
cd mobile && npm ci && npx expo start
```

Press `i` for the iOS simulator, or scan the QR code with Expo Go. On a phone, see [The app](#the-app) for the API URL.

## Demo users

Pick the user with the `DEMO` switcher at the top of Home, or send it as `X-User-ID`. State after `demo-reset` (AC-57):

| User     | Starting state                          | Try                                                             |
| -------- | --------------------------------------- | --------------------------------------------------------------- |
| `user_a` | balance 15000, earned 47000 today       | pay Rp100.000: earns 3000, `PARTIAL_DAILY_CAP`; then redeem     |
| `user_b` | no rows                                 | pay Rp19.999: Rp0, `BELOW_MINIMUM`; pay Rp100.000: 5000 `AWARDED` |
| `user_c` | earned 50000 today (the daily cap)      | pay any amount from Rp20.000: Rp0, `DAILY_CAP_REACHED`          |

## Try the API with curl

Every route except health needs `X-User-ID`; every POST needs an `Idempotency-Key` (a UUID). Shapes and codes are in
[docs/api-contract.md](docs/api-contract.md).

```sh
curl -s localhost:8080/v1/campaign -H 'X-User-ID: user_a'
curl -s localhost:8080/v1/me/cashback -H 'X-User-ID: user_a'

# user_a has 3000 of today's cap left, so Rp100.000 earns 3000 with reason PARTIAL_DAILY_CAP
curl -si -X POST localhost:8080/v1/payments -H 'X-User-ID: user_a' \
  -H 'Idempotency-Key: 6f1c2a9e-4b7d-4e2a-9c3f-1a2b3c4d5e6f' \
  -H 'Content-Type: application/json' -d '{"amount":100000}'

# the same request again: 200, header Idempotent-Replayed: true, the same body, no second payment
curl -si -X POST localhost:8080/v1/payments -H 'X-User-ID: user_a' \
  -H 'Idempotency-Key: 6f1c2a9e-4b7d-4e2a-9c3f-1a2b3c4d5e6f' \
  -H 'Content-Type: application/json' -d '{"amount":100000}'

# redeem part of the balance (15000 + 3000 = 18000 after the payment above)
curl -si -X POST localhost:8080/v1/redemptions -H 'X-User-ID: user_a' \
  -H 'Idempotency-Key: 0b8e7d6c-5a4f-4e3d-8c2b-9a8f7e6d5c4b' \
  -H 'Content-Type: application/json' -d '{"amount":5000}'

curl -s 'localhost:8080/v1/me/history?limit=5' -H 'X-User-ID: user_a'
```

The examples run in order after `demo-reset`. Rerunning them without a reset replays the stored answers, because the
keys are the same.

## Operations

Operations are command-line tools, not HTTP endpoints (D23). `--by` names the operator.

```sh
docker compose exec api /app/admin pause-awards --by alice        # awards earn Rp0, CAMPAIGN_PAUSED
docker compose exec api /app/admin resume-awards --by alice
docker compose exec api /app/admin pause-redemptions --by alice   # new redemptions are 409 REDEMPTION_PAUSED
docker compose exec api /app/admin resume-redemptions --by alice
docker compose exec api /app/reconcile                            # checks the invariants; exit 0 = books balance
```

Reconcile only reports. It also prints the outstanding cashback liability.

## The app

The app reads `EXPO_PUBLIC_API_URL`, default `http://localhost:8080/v1` (D09).

- **iOS simulator:** works with the default.
- **Expo Go on Android or iPhone:** the phone must be on the same network as your computer. Start with your
  computer's LAN address: `EXPO_PUBLIC_API_URL=http://<your-computer-LAN-IP>:8080/v1 npx expo start`.

To use another host port, edit the `ports` mapping in `docker-compose.yml` (for example `"18080:8080"`), use that port
in every URL above, and set `EXPO_PUBLIC_API_URL` to match.

## Run the tests

Run from the repo root. The integration tests start their own PostgreSQL and Redis from `docker-compose.test.yml`, on
ports apart from the demo stack.

```sh
make gate          # gofmt check, vet, staticcheck, unit tests, integration tests with -race
make test-race     # the concurrency tests, -race -count=20
make cover         # unit and integration coverage of the backend, jest coverage of the app
make mobile-check  # lint, typecheck, and tests of the app (run npm ci in mobile/ first)
```

## Test results

| Area    | Kind                                | Tests | Command                 |
| ------- | ----------------------------------- | ----- | ----------------------- |
| Backend | unit (no database)                  | 53    | `make test`             |
| Backend | integration                         | 113   | `make test-integration` |
| Backend | concurrency (subset of integration) | 18    | `make test-race`        |
| Mobile  | Jest, 30 test files                 | 323   | `make mobile-check`     |

| Coverage                                   | Statements |
| ------------------------------------------ | ---------- |
| Backend total (unit + integration)         | 84.8%      |
| `internal/domain` (award and status rules) | 97.3%      |
| `internal/service` (payment, redemption)   | 94.0%      |
| `internal/store` (SQL, locks)              | 77.5%      |
| `internal/httpapi`                         | 96.7%      |
| `internal/reconcile`                       | 86.7%      |

| Mobile     | Covered           |
| ---------- | ----------------- |
| Statements | 97.66% (712/729)  |
| Branches   | 94.17% (501/532)  |
| Functions  | 98.11% (208/212)  |
| Lines      | 99.51% (617/620)  |

Measured at commit `8673bce` on 2026-10-06. `make cover` regenerates the figures; it runs without `-race` and prints
every package, including `cmd/*`, whose `main` functions only the compose smoke test runs. Mobile figures leave
out the test helpers in `src/test/`.

## Stop and clean up

```sh
docker compose down      # stop; the database is kept
docker compose down -v   # stop and drop the database volume; the next start seeds a fresh campaign
```

## More detail

- [docs/design-overview.md](docs/design-overview.md): what has to be true before real money, rules as interpreted,
  decisions, rejected options, out of scope, where it breaks
- [docs/challenge-brief.md](docs/challenge-brief.md): the requirements
- [docs/DECISIONS.md](docs/DECISIONS.md): every decision with the owner's reason
- [docs/api-contract.md](docs/api-contract.md): endpoints, fields, codes
- [docs/ui-wireframe.md](docs/ui-wireframe.md): screens
- [docs/prd.md](docs/prd.md): acceptance criteria and invariants
- [docs/tech-spec.md](docs/tech-spec.md): design, lock order, where it breaks
