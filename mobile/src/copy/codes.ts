import type { Campaign } from '@/api/queries';
import { formatRp } from '@/money/format';

// Server codes become the copy of docs/ui-wireframe.md (screens 1, 3, 5). Every mapping has a fallback for a code this
// version does not know (D20). Numbers in the copy come from the campaign rules, never from the text.

export type Rules = Campaign['rules'];

/** The largest amount the API accepts (docs/api-contract.md, "Amount range"). */
export const MAX_AMOUNT = 10_000_000;

const GENERIC_RULES_TEXT = 'See How it works for the cashback rules.';
const GENERIC_ERROR = 'Something went wrong. Please try again.';

/** 500 -> "5%", 250 -> "2.5%", 5 -> "0.05%": exact, no floating point. */
export function formatPercent(bps: number): string {
  const whole = Math.floor(bps / 100);
  const fraction = String(bps % 100)
    .padStart(2, '0')
    .replace(/0+$/, '');
  return fraction === '' ? `${whole}%` : `${whole}.${fraction}%`;
}

export function zoneLabel(timezone: string): string {
  return timezone === 'Asia/Jakarta' ? 'WIB' : timezone;
}

export type ReasonCopy = { chip: string | null; howItWorks: boolean };

export function reasonCopy(reason: string): ReasonCopy {
  switch (reason) {
    case 'AWARDED':
      return { chip: null, howItWorks: false };
    case 'PARTIAL_DAILY_CAP':
      return { chip: 'Daily limit reached', howItWorks: true };
    case 'PARTIAL_BUDGET':
      return { chip: 'Last of the cashback', howItWorks: true };
    case 'DAILY_CAP_REACHED':
      return { chip: 'Daily limit reached', howItWorks: false };
    case 'BELOW_MINIMUM':
      return { chip: 'Below minimum', howItWorks: false };
    case 'CAMPAIGN_ENDED':
      return { chip: 'Campaign ended', howItWorks: false };
    case 'CAMPAIGN_PAUSED':
      return { chip: 'Unavailable', howItWorks: false };
    default:
      return { chip: null, howItWorks: false };
  }
}

export type BannerCopy = { title: string | null; text: string };

export function bannerCopy(status: string, redemptionStatus: string, rules: Rules): BannerCopy {
  switch (status) {
    case 'ACTIVE':
      return {
        title: 'Campaign active',
        text:
          `Cashback up to ${formatPercent(rules.rate_bps)} on payments of ${formatRp(rules.min_payment)} or more, ` +
          `max ${formatRp(rules.daily_cap)} per day, while cashback lasts.`,
      };
    case 'PAUSED':
      return { title: null, text: 'Cashback is temporarily unavailable. Payments still work as usual.' };
    case 'ENDED': {
      const redeem = redemptionStatus === 'PAUSED' ? '' : ', and you can still redeem your balance';
      return {
        title: null,
        text: `Flash Cashback has ended. All cashback has been claimed. Payments still work as usual${redeem}.`,
      };
    }
    default:
      return { title: null, text: GENERIC_RULES_TEXT };
  }
}

export function errorCopy(code: string, context: { balance?: number }): string {
  switch (code) {
    case 'INSUFFICIENT_BALANCE':
      return context.balance === undefined ? GENERIC_ERROR : `You can redeem up to ${formatRp(context.balance)}.`;
    case 'INVALID_AMOUNT':
      return `Enter an amount up to ${formatRp(MAX_AMOUNT)}.`;
    case 'REDEMPTION_PAUSED':
      return 'Redemption is temporarily unavailable. Your balance is safe.';
    default:
      return GENERIC_ERROR;
  }
}
