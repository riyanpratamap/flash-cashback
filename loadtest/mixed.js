import {
  scenario, options as opts, summary, get, pay, readUser, payUser, iteration,
  MIXED_READ_RATE, MIXED_PAY_RATE,
} from './lib.js';

export const options = opts(
  {
    campaign_reads: scenario('campaign', MIXED_READ_RATE, 200, 2000),
    cashback_reads: scenario('cashback', MIXED_READ_RATE, 200, 2000),
    payments: scenario('payment', MIXED_PAY_RATE, 20, 200),
  },
  true,
);

export function campaign() {
  get('/v1/campaign', 'user_a');
}

export function cashback() {
  get('/v1/me/cashback', readUser(iteration()));
}

export function payment() {
  pay(payUser(iteration()));
}

export const handleSummary = summary('mixed', ['read', 'pay']);
