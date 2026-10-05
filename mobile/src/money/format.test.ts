import { formatAsTyped, formatRp, formatSigned, parseDigits } from '@/money/format';

describe('formatRp', () => {
  it.each([
    [0, 'Rp0'],
    [19999, 'Rp19.999'],
    [20000, 'Rp20.000'],
    [100000, 'Rp100.000'],
    [10000000, 'Rp10.000.000'],
  ])('%d -> %s', (amount, want) => {
    expect(formatRp(amount)).toBe(want);
  });
});

describe('formatSigned', () => {
  it.each([
    [5000, 'earned', '+Rp5.000'],
    [42000, 'redeemed', '−Rp42.000'],
    [0, 'earned', 'Rp0'],
    [0, 'redeemed', 'Rp0'],
  ] as const)('%d %s -> %s', (amount, kind, want) => {
    expect(formatSigned(amount, kind)).toBe(want);
  });
});

describe('parseDigits', () => {
  it.each([
    ['100.000', 100000],
    ['abc', null],
    ['', null],
    ['007', 7],
    ['1a2', 12],
  ])('%j -> %j', (text, want) => {
    expect(parseDigits(text)).toBe(want);
  });
});

describe('formatAsTyped', () => {
  it.each([
    ['', ''],
    ['abc', ''],
    ['1000', '1.000'],
    ['100.000', '100.000'],
    ['0012', '12'],
    ['0', '0'],
  ])('%j -> %j', (text, want) => {
    expect(formatAsTyped(text)).toBe(want);
  });
});
