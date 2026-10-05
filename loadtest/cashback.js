import { scenario, options as opts, summary, get, readUser, iteration, CASHBACK_RATE } from './lib.js';

export const options = opts({ cashback: scenario('run', CASHBACK_RATE, 200, 2000) }, false);

export function run() {
  get('/v1/me/cashback', readUser(iteration()));
}

export const handleSummary = summary('cashback', ['read']);
