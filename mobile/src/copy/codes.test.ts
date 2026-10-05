import { bannerCopy, errorCopy, formatPercent, reasonCopy, zoneLabel } from './codes';

const rules = { rate_bps: 500, min_payment: 20000, daily_cap: 50000, timezone: 'Asia/Jakarta' };
const otherRules = { rate_bps: 1000, min_payment: 30000, daily_cap: 70000, timezone: 'Asia/Jakarta' };

describe('reasonCopy (screen 3)', () => {
  it.each([
    ['AWARDED', null, '5% cashback added to your balance.', false],
    ['PARTIAL_DAILY_CAP', 'Daily limit reached', "You've reached today's Rp50.000 cashback limit.", true],
    [
      'PARTIAL_BUDGET',
      'Last of the cashback',
      'This was the last of the campaign cashback. Flash Cashback has now ended.',
      true,
    ],
    ['DAILY_CAP_REACHED', 'Daily limit reached', "You've already reached today's Rp50.000 cashback limit.", true],
    ['BELOW_MINIMUM', 'Below minimum', "Payments under Rp20.000 don't earn cashback.", true],
    ['CAMPAIGN_ENDED', 'Campaign ended', 'Flash Cashback has ended. All cashback has been claimed.', true],
    [
      'CAMPAIGN_PAUSED',
      'Unavailable',
      'Cashback is temporarily unavailable. Payments still work as usual.',
      true,
    ],
  ])('%s', (reason, chip, text, howItWorks) => {
    expect(reasonCopy(reason, rules)).toEqual({ chip, text, howItWorks });
  });

  it('takes its numbers from the rules, not from the text', () => {
    expect(reasonCopy('AWARDED', otherRules).text).toBe('10% cashback added to your balance.');
    expect(reasonCopy('BELOW_MINIMUM', otherRules).text).toBe("Payments under Rp30.000 don't earn cashback.");
    expect(reasonCopy('DAILY_CAP_REACHED', otherRules).text).toBe(
      "You've already reached today's Rp70.000 cashback limit.",
    );
  });

  it('an unknown reason shows the generic text and never throws (D20)', () => {
    expect(reasonCopy('SOMETHING_NEW', rules)).toEqual({
      chip: null,
      text: 'See How it works for the cashback rules.',
      howItWorks: true,
    });
  });
});

describe('bannerCopy (screen 1)', () => {
  it('ACTIVE states the rules from the campaign', () => {
    expect(bannerCopy('ACTIVE', 'AVAILABLE', rules)).toEqual({
      title: 'Campaign active',
      text: 'Cashback up to 5% on payments of Rp20.000 or more, max Rp50.000 per day, while cashback lasts.',
    });
    expect(bannerCopy('ACTIVE', 'AVAILABLE', otherRules).text).toContain('up to 10% on payments of Rp30.000 or more, max Rp70.000');
  });

  it('PAUSED is neutral text', () => {
    expect(bannerCopy('PAUSED', 'AVAILABLE', rules)).toEqual({
      title: null,
      text: 'Cashback is temporarily unavailable. Payments still work as usual.',
    });
  });

  it('ENDED offers redeeming unless redemptions are paused', () => {
    expect(bannerCopy('ENDED', 'AVAILABLE', rules).text).toBe(
      'Flash Cashback has ended. All cashback has been claimed. Payments still work as usual, and you can still redeem your balance.',
    );
    expect(bannerCopy('ENDED', 'PAUSED', rules).text).toBe(
      'Flash Cashback has ended. All cashback has been claimed. Payments still work as usual.',
    );
  });

  it('an unknown status shows the generic text', () => {
    expect(bannerCopy('NEW_STATE', 'AVAILABLE', rules)).toEqual({
      title: null,
      text: 'See How it works for the cashback rules.',
    });
  });
});

describe('errorCopy (screens 2 and 5)', () => {
  it('INSUFFICIENT_BALANCE names the refetched balance', () => {
    expect(errorCopy('INSUFFICIENT_BALANCE', { balance: 18000 })).toBe('You can redeem up to Rp18.000.');
  });

  it('INSUFFICIENT_BALANCE without a balance is generic', () => {
    expect(errorCopy('INSUFFICIENT_BALANCE', {})).toBe('Something went wrong. Please try again.');
  });

  it('INVALID_AMOUNT shows the limit', () => {
    expect(errorCopy('INVALID_AMOUNT', {})).toBe('Enter an amount up to Rp10.000.000.');
  });

  it('REDEMPTION_PAUSED shows the paused state', () => {
    expect(errorCopy('REDEMPTION_PAUSED', {})).toBe('Redemption is temporarily unavailable. Your balance is safe.');
  });

  it.each(['MALFORMED_REQUEST', 'IDEMPOTENCY_KEY_REUSED', 'UNREADABLE_ERROR', 'FROM_THE_FUTURE'])(
    '%s is generic',
    (code) => {
      expect(errorCopy(code, {})).toBe('Something went wrong. Please try again.');
    },
  );
});

describe('formatPercent and zoneLabel', () => {
  it.each([
    [500, '5%'],
    [1000, '10%'],
    [250, '2.5%'],
    [5, '0.05%'],
  ])('%d bps -> %s', (bps, want) => {
    expect(formatPercent(bps)).toBe(want);
  });

  it('labels Asia/Jakarta as WIB and keeps another zone as is', () => {
    expect(zoneLabel('Asia/Jakarta')).toBe('WIB');
    expect(zoneLabel('Asia/Makassar')).toBe('Asia/Makassar');
  });
});
