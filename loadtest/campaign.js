import { scenario, options as opts, summary, get, CAMPAIGN_RATE } from './lib.js';

export const options = opts({ campaign: scenario('run', CAMPAIGN_RATE, 200, 2000) }, false);

export function run() {
  get('/v1/campaign', 'user_a');
}

export const handleSummary = summary('campaign', ['read']);
