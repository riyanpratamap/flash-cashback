const MINUS = '−';

function group(digits: string): string {
  return digits.replace(/\B(?=(\d{3})+(?!\d))/g, '.');
}

/** Rp100.000. Amounts are non-negative integers (the API never sends a negative one). */
export function formatRp(amount: number): string {
  return `Rp${group(String(amount))}`;
}

/** +Rp5.000 for earned cashback, −Rp42.000 for a redemption, Rp0 with no sign. */
export function formatSigned(amount: number, kind: 'earned' | 'redeemed'): string {
  if (amount === 0) return formatRp(0);
  return `${kind === 'earned' ? '+' : MINUS}${formatRp(amount)}`;
}

/** The integer in the digits of the text, or null when it has none. Never parseFloat. */
export function parseDigits(text: string): number | null {
  const digits = text.replace(/\D/g, '');
  return digits === '' ? null : Number(digits);
}

/** The text of an amount field while typing: digits only, grouped, no leading zeros. */
export function formatAsTyped(text: string): string {
  const amount = parseDigits(text);
  return amount === null ? '' : group(String(amount));
}
