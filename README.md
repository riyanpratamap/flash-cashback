# Flash Cashback

A cashback campaign for payments: 5% back, at least Rp20.000 per payment, at most Rp50.000 a day per user, from a
Rp10.000.000 budget. Go API, PostgreSQL, Redis, React Native app. Money is integer IDR and the rules are decided in
PostgreSQL inside the payment transaction.

How it is made safe for money, the rules as interpreted, and its limits are in
[docs/design-overview.md](docs/design-overview.md).

## How to run

Prerequisites: Docker with Compose v2; Node LTS for the app. No `.env` is needed.

```sh
docker compose up -d --build --wait
curl -fsS localhost:8080/v1/healthz
```

Expect `{"status":"ok","dependencies":{"postgres":"ok","redis":"ok"}}`. Only port 8080 is published. To change it, edit
the `ports` mapping in `docker-compose.yml` (for example `"18080:8080"`), use that port in every URL below, and set
`EXPO_PUBLIC_API_URL` to match.

### Demo state

`demo-reset` refuses unless `FC_DEMO=1` (exit 2, nothing changed). It truncates the money tables, so use it only on a
demo database.

```sh
docker compose exec -e FC_DEMO=1 api /app/admin demo-reset
```

Result (AC-57): `user_a` has balance 15000 and has earned 47000 today; `user_b` has no rows; `user_c` has earned
50000 today (the daily cap). The demo users are `user_a`, `user_b`, `user_c`.

### Try the API with curl

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

### Operations

Operations are command-line tools, not HTTP endpoints (D23). `--by` names the operator.

```sh
docker compose exec api /app/admin pause-awards --by alice        # awards earn Rp0, CAMPAIGN_PAUSED
docker compose exec api /app/admin resume-awards --by alice
docker compose exec api /app/admin pause-redemptions --by alice   # new redemptions are 409 REDEMPTION_PAUSED
docker compose exec api /app/admin resume-redemptions --by alice
docker compose exec api /app/reconcile                            # checks the invariants; exit 0 = books balance
```

Reconcile only reports. It also prints the outstanding cashback liability.

### The app

```sh
cd mobile && npm ci && npx expo start
```

The app reads `EXPO_PUBLIC_API_URL`, default `http://localhost:8080/v1` (D09).

- **iOS simulator:** works with the default.
- **Expo Go on Android or iPhone:** the phone must be on the same network as your computer. Start with your
  computer's LAN address: `EXPO_PUBLIC_API_URL=http://<your-computer-LAN-IP>:8080/v1 npx expo start`.

Screen recording: TODO(owner): screen recording

## More detail

- [docs/design-overview.md](docs/design-overview.md): what has to be true before real money, rules as interpreted,
  decisions, rejected options, out of scope, where it breaks
- [docs/challenge-brief.md](docs/challenge-brief.md): the requirements
- [docs/DECISIONS.md](docs/DECISIONS.md): every decision with the owner's reason
- [docs/api-contract.md](docs/api-contract.md): endpoints, fields, codes
- [docs/ui-wireframe.md](docs/ui-wireframe.md): screens
- [docs/prd.md](docs/prd.md): acceptance criteria and invariants
- [docs/tech-spec.md](docs/tech-spec.md): design, lock order, where it breaks
