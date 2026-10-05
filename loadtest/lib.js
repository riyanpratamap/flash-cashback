// Shared pieces of the load test (D51, AC-77). Rates and durations are set
// here so the `on` and `off` runs use the same load.
import http from 'k6/http';
import exec from 'k6/execution';
import { Trend, Counter } from 'k6/metrics';

export const BASE_URL = __ENV.BASE_URL || 'http://api:8080';
export const MODE = __ENV.MODE || 'on';

export const DURATION = '30s';
export const CAMPAIGN_RATE = 4000; // GET /v1/campaign, per second
export const CASHBACK_RATE = 4000; // GET /v1/me/cashback, per second
export const MIXED_READ_RATE = 2000; // each of the two reads in "mixed"
export const MIXED_PAY_RATE = 20; // POST /v1/payments in "mixed"
export const READ_USERS = 200; // lt_0001 .. lt_0200
export const PAY_USERS = 200; // lt_p001 .. lt_p200

const readMs = new Trend('read_ms', true);
const payMs = new Trend('pay_ms', true);
const readReq = new Counter('read_req');
const readErr = new Counter('read_err'); // non-2xx and transport errors
const readFail = new Counter('read_fail'); // status 0 or 5xx
const payReq = new Counter('pay_req');
const payErr = new Counter('pay_err');
const payFail = new Counter('pay_fail'); // status 0 or 5xx other than 503
const pay503 = new Counter('pay_503');

export function scenario(exe, rate, preAllocatedVUs, maxVUs) {
  return {
    executor: 'constant-arrival-rate',
    exec: exe,
    rate,
    timeUnit: '1s',
    duration: DURATION,
    preAllocatedVUs,
    maxVUs,
  };
}

export function options(scenarios, withPay) {
  const thresholds = { read_fail: ['count==0'] };
  if (withPay) thresholds.pay_fail = ['count==0'];
  return {
    scenarios,
    thresholds,
    summaryTrendStats: ['med', 'p(95)', 'p(99)'],
  };
}

function pad(n, w) {
  return String(n).padStart(w, '0');
}

export function readUser(i) {
  return `lt_${pad((i % READ_USERS) + 1, 4)}`;
}

export function payUser(i) {
  return `lt_p${pad((i % PAY_USERS) + 1, 3)}`;
}

export function get(path, user) {
  const res = http.get(`${BASE_URL}${path}`, { headers: { 'X-User-ID': user } });
  readMs.add(res.timings.duration);
  readReq.add(1);
  if (res.status < 200 || res.status > 299) readErr.add(1);
  if (res.status === 0 || res.status >= 500) readFail.add(1);
  return res;
}

function uuidV4() {
  const b = new Uint8Array(16);
  crypto.getRandomValues(b);
  b[6] = (b[6] & 0x0f) | 0x40;
  b[8] = (b[8] & 0x3f) | 0x80;
  const h = Array.from(b, (x) => pad(x.toString(16), 2).slice(-2)).join('');
  return `${h.slice(0, 8)}-${h.slice(8, 12)}-${h.slice(12, 16)}-${h.slice(16, 20)}-${h.slice(20)}`;
}

export function pay(user) {
  const res = http.post(`${BASE_URL}/v1/payments`, JSON.stringify({ amount: 100000 }), {
    headers: {
      'X-User-ID': user,
      'Idempotency-Key': uuidV4(),
      'Content-Type': 'application/json',
    },
  });
  payMs.add(res.timings.duration);
  payReq.add(1);
  if (res.status < 200 || res.status > 299) payErr.add(1);
  if (res.status === 503) pay503.add(1);
  else if (res.status === 0 || res.status >= 500) payFail.add(1);
  return res;
}

export function iteration() {
  return exec.scenario.iterationInTest;
}

function val(data, metric, key) {
  const m = data.metrics[metric];
  return m && m.values && m.values[key] !== undefined ? m.values[key] : 0;
}

function row(data, name, trend, req, err, busy) {
  const secs = data.state.testRunDurationMs / 1000;
  const ms = (k) => val(data, trend, k).toFixed(1);
  const cols = [
    name.padEnd(13),
    MODE.padEnd(4),
    ms('med').padStart(8),
    ms('p(95)').padStart(8),
    ms('p(99)').padStart(8),
    (val(data, req, 'count') / secs).toFixed(1).padStart(8),
    String(val(data, err, 'count')).padStart(7),
    (busy ? String(val(data, busy, 'count')) : '-').padStart(6),
  ];
  return cols.join(' ');
}

// kinds: which rows to print, e.g. ['read'] or ['read', 'pay'].
export function summary(name, kinds) {
  return (data) => {
    if (val(data, 'dropped_iterations', 'count') > 0) {
      console.warn(
        `${name}: ${val(data, 'dropped_iterations', 'count')} dropped iterations; the arrival rate was not reached`,
      );
    }
    const lines = [];
    if (kinds.includes('read')) {
      const n = kinds.includes('pay') ? `${name}-read` : name;
      lines.push(row(data, n, 'read_ms', 'read_req', 'read_err', null));
    }
    if (kinds.includes('pay')) {
      lines.push(row(data, `${name}-pay`, 'pay_ms', 'pay_req', 'pay_err', 'pay_503'));
    }
    return {
      [`/results/${MODE}-${name}.json`]: JSON.stringify(data, null, 2),
      stdout: `${lines.join('\n')}\n`,
    };
  };
}
