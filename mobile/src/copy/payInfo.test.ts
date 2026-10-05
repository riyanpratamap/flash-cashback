import { estimateCashback, payInfoLine } from '@/copy/payInfo';

const rules = { rate_bps: 500, min_payment: 20000, daily_cap: 50000, timezone: 'Asia/Jakarta' };

describe('estimateCashback (AC-63)', () => {
  it.each([
    [100000, 50000, 5000],
    [100000, 3000, 3000], // 5% is more than what is left today: the rest of the cap
    [20001, 50000, 1000], // rounded down
    [100000, 0, 0],
  ])('amount %i with %i left today estimates %i', (amount, remaining, expected) => {
    expect(estimateCashback(amount, rules, remaining)).toBe(expected);
  });

  it('uses the rate of the rules, not a fixed 5%', () => {
    expect(estimateCashback(100000, { ...rules, rate_bps: 250 }, 50000)).toBe(2500);
  });
});

describe('payInfoLine (AC-63)', () => {
  const line = (amount: number, status = 'ACTIVE', remaining = 47000) => payInfoLine(amount, status, rules, remaining);

  it('below the minimum', () => {
    expect(line(19999)).toBe("This payment won't earn cashback. The minimum is Rp20.000.");
  });

  it('exactly the minimum earns', () => {
    expect(line(20000)).toBe("Earn up to Rp1.000 cashback. You'll see the exact amount after you pay.");
  });

  it('campaign ended', () => {
    expect(line(100000, 'ENDED')).toBe("This payment won't earn cashback. Flash Cashback has ended.");
  });

  it('awards paused', () => {
    expect(line(100000, 'PAUSED')).toBe("This payment won't earn cashback. Cashback is temporarily unavailable.");
  });

  it('nothing left today', () => {
    expect(line(100000, 'ACTIVE', 0)).toBe("This payment won't earn cashback. You've reached today's limit.");
  });

  it('5% is more than what is left: the estimate is Rp3.000', () => {
    expect(line(100000, 'ACTIVE', 3000)).toBe("Earn up to Rp3.000 cashback, the rest of today's Rp50.000 limit.");
  });

  it('5% exactly equals what is left: the plain line', () => {
    expect(line(100000, 'ACTIVE', 5000)).toBe("Earn up to Rp5.000 cashback. You'll see the exact amount after you pay.");
  });

  it('otherwise the estimate with the plain line', () => {
    expect(line(100000)).toBe("Earn up to Rp5.000 cashback. You'll see the exact amount after you pay.");
  });

  it('an unknown status is treated like an active campaign', () => {
    expect(line(100000, 'SOMETHING_NEW')).toBe("Earn up to Rp5.000 cashback. You'll see the exact amount after you pay.");
  });
});
