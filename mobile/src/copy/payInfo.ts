import { formatRp } from '../money/format';
import type { Rules } from './codes';

const NO_CASHBACK = "This payment won't earn cashback.";

/** The one client estimate: the rate of the rules rounded down, cut to what is left of today's cap. Never knows the budget. */
export function estimateCashback(amount: number, rules: Rules, remainingToday: number): number {
  return Math.min(fullAward(amount, rules), remainingToday);
}

function fullAward(amount: number, rules: Rules): number {
  return Math.floor((amount * rules.rate_bps) / 10_000);
}

/** The Pay screen info line (wireframe screen 2). Always worded "up to": the server decides the award. */
export function payInfoLine(amount: number, status: string, rules: Rules, remainingToday: number): string {
  if (amount < rules.min_payment) return `${NO_CASHBACK} Payments under ${formatRp(rules.min_payment)} earn no cashback.`;
  if (status === 'ENDED') return `${NO_CASHBACK} Flash Cashback has ended.`;
  if (status === 'PAUSED') return `${NO_CASHBACK} Cashback is temporarily unavailable.`;
  if (remainingToday <= 0) return `${NO_CASHBACK} You've reached today's limit.`;
  const estimate = estimateCashback(amount, rules, remainingToday);
  if (fullAward(amount, rules) > remainingToday) {
    return `Earn up to ${formatRp(estimate)} cashback, the rest of today's ${formatRp(rules.daily_cap)} limit.`;
  }
  return `Earn up to ${formatRp(estimate)} cashback. Final amount is confirmed after payment.`;
}
