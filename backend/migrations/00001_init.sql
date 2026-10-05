-- +goose Up
-- The one campaign-day function (D04). fc_now() is the only clock; tests may replace it in the test DB.
-- +goose StatementBegin
CREATE FUNCTION fc_now() RETURNS timestamptz LANGUAGE sql STABLE AS $$ SELECT now() $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION fc_campaign_day(ts timestamptz) RETURNS date LANGUAGE sql STABLE
  AS $$ SELECT (ts AT TIME ZONE 'Asia/Jakarta')::date $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION fc_day_resets_at(d date) RETURNS timestamptz LANGUAGE sql STABLE
  AS $$ SELECT (d + 1)::timestamp AT TIME ZONE 'Asia/Jakarta' $$;
-- +goose StatementEnd

CREATE TABLE campaigns (
  id                 text        PRIMARY KEY,
  name               text        NOT NULL,
  rate_bps           integer     NOT NULL CONSTRAINT campaigns_rate_range     CHECK (rate_bps BETWEEN 1 AND 10000),
  min_payment        bigint      NOT NULL CONSTRAINT campaigns_min_positive   CHECK (min_payment > 0),
  daily_cap          bigint      NOT NULL CONSTRAINT campaigns_cap_positive   CHECK (daily_cap > 0),
  budget             bigint      NOT NULL CONSTRAINT campaigns_budget_positive CHECK (budget > 0),
  spent              bigint      NOT NULL DEFAULT 0 CONSTRAINT campaigns_spent_nonneg CHECK (spent >= 0),
  awards_paused      boolean     NOT NULL DEFAULT false,
  redemptions_paused boolean     NOT NULL DEFAULT false,
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT campaigns_spent_within_budget CHECK (spent <= budget),               -- INV-02, TC1
  CONSTRAINT campaigns_min_earns CHECK (min_payment * rate_bps >= 10000)          -- a payment at the minimum earns ≥ Rp1
);

CREATE TABLE user_daily_earnings (
  campaign_id text   NOT NULL REFERENCES campaigns(id),
  user_id     text   NOT NULL,
  day         date   NOT NULL,
  earned      bigint NOT NULL DEFAULT 0 CONSTRAINT ude_earned_nonneg CHECK (earned >= 0),
  daily_cap   bigint NOT NULL CONSTRAINT ude_cap_positive CHECK (daily_cap > 0),  -- the day's cap, copied from the campaign row at creation
  PRIMARY KEY (campaign_id, user_id, day),
  CONSTRAINT ude_earned_within_cap CHECK (earned <= daily_cap)                    -- INV-03, TC2
);

CREATE TABLE payments (
  id               bigserial   PRIMARY KEY,
  campaign_id      text        NOT NULL REFERENCES campaigns(id),
  user_id          text        NOT NULL CONSTRAINT payments_user_format CHECK (user_id ~ '^[a-z0-9_-]{1,64}$'),
  idempotency_key  uuid        NOT NULL,
  request_hash     bytea       NOT NULL CONSTRAINT payments_hash_len CHECK (octet_length(request_hash) = 32),
  amount           bigint      NOT NULL CONSTRAINT payments_amount_range CHECK (amount BETWEEN 1 AND 10000000),
  status           text        NOT NULL CONSTRAINT payments_status CHECK (status = 'SUCCEEDED'),
  cashback_awarded bigint      NOT NULL CONSTRAINT payments_award_nonneg CHECK (cashback_awarded >= 0),
  cashback_reason  text        NOT NULL CONSTRAINT payments_reason_known CHECK (cashback_reason IN
                     ('AWARDED','PARTIAL_DAILY_CAP','PARTIAL_BUDGET','BELOW_MINIMUM','CAMPAIGN_ENDED',
                      'CAMPAIGN_PAUSED','DAILY_CAP_REACHED')),
  rate_bps         integer     NOT NULL,   -- rule snapshot (TC12)
  min_payment      bigint      NOT NULL,
  daily_cap        bigint      NOT NULL,
  campaign_day     date        NOT NULL,
  created_at       timestamptz NOT NULL,
  CONSTRAINT payments_user_key_unique UNIQUE (user_id, idempotency_key),          -- INV-05, TC3
  CONSTRAINT payments_award_within_rate CHECK (cashback_awarded * 10000 <= amount * rate_bps),
  CONSTRAINT payments_award_within_cap CHECK (cashback_awarded <= daily_cap),
  CONSTRAINT payments_reason_matches_award CHECK ((cashback_awarded > 0) =
                     (cashback_reason IN ('AWARDED','PARTIAL_DAILY_CAP','PARTIAL_BUDGET')))
);
CREATE INDEX payments_user_newest ON payments (user_id, created_at DESC, id DESC);

CREATE TABLE redemptions (
  id              bigserial   PRIMARY KEY,
  campaign_id     text        NOT NULL REFERENCES campaigns(id),
  user_id         text        NOT NULL CONSTRAINT redemptions_user_format CHECK (user_id ~ '^[a-z0-9_-]{1,64}$'),
  idempotency_key uuid        NOT NULL,
  request_hash    bytea       NOT NULL CONSTRAINT redemptions_hash_len CHECK (octet_length(request_hash) = 32),
  amount          bigint      NOT NULL CONSTRAINT redemptions_amount_range CHECK (amount BETWEEN 1 AND 10000000),
  status          text        NOT NULL CONSTRAINT redemptions_status CHECK (status IN ('PENDING','COMPLETED','FAILED')),
  destination     text        NOT NULL CONSTRAINT redemptions_destination CHECK (destination = 'MAIN_ACCOUNT'),
  balance_after   bigint      NOT NULL CONSTRAINT redemptions_balance_after_nonneg CHECK (balance_after >= 0),
  campaign_day    date        NOT NULL,
  created_at      timestamptz NOT NULL,
  CONSTRAINT redemptions_user_key_unique UNIQUE (user_id, idempotency_key)        -- INV-05, TC3
);
CREATE INDEX redemptions_user_newest ON redemptions (user_id, created_at DESC, id DESC);

CREATE TABLE cashback_balances (
  user_id    text        PRIMARY KEY CONSTRAINT balances_user_format CHECK (user_id ~ '^[a-z0-9_-]{1,64}$'),
  balance    bigint      NOT NULL CONSTRAINT balances_nonneg CHECK (balance >= 0),  -- INV-04, TC4
  updated_at timestamptz NOT NULL
);

CREATE TABLE ledger_entries (
  id            bigserial   PRIMARY KEY,
  user_id       text        NOT NULL,
  kind          text        NOT NULL,
  amount        bigint      NOT NULL,
  payment_id    bigint      CONSTRAINT ledger_payment_once UNIQUE REFERENCES payments(id),
  redemption_id bigint      CONSTRAINT ledger_redemption_once UNIQUE REFERENCES redemptions(id),
  balance_after bigint      NOT NULL CONSTRAINT ledger_balance_after_nonneg CHECK (balance_after >= 0),
  created_at    timestamptz NOT NULL,
  CONSTRAINT ledger_kind_shape CHECK (                                            -- IS NOT NULL in each branch (KP)
    (kind = 'AWARD' AND amount > 0 AND payment_id IS NOT NULL AND redemption_id IS NULL) OR
    (kind = 'REDEMPTION' AND amount < 0 AND redemption_id IS NOT NULL AND payment_id IS NULL))
);
CREATE INDEX ledger_user ON ledger_entries (user_id, id);

-- Append-only (D40, INV-06): row-level UPDATE/DELETE raise. TRUNCATE (tests, demo-reset) is not a row event.
-- +goose StatementBegin
CREATE FUNCTION fc_append_only() RETURNS trigger LANGUAGE plpgsql
  AS $$ BEGIN RAISE EXCEPTION '% is append-only', TG_TABLE_NAME; END $$;
-- +goose StatementEnd
CREATE TRIGGER ledger_append_only BEFORE UPDATE OR DELETE ON ledger_entries FOR EACH ROW EXECUTE FUNCTION fc_append_only();
CREATE TRIGGER payments_append_only BEFORE UPDATE OR DELETE ON payments FOR EACH ROW EXECUTE FUNCTION fc_append_only();
CREATE TRIGGER redemptions_append_only BEFORE UPDATE OR DELETE ON redemptions FOR EACH ROW EXECUTE FUNCTION fc_append_only();
