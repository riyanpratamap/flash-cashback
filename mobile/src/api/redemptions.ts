export type RedemptionResult = {
  reference: string;
  amount: number;
  balanceAfter: number;
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

/** The fields of a `POST /redemptions` answer that the confirmation shows; null when the body is not that shape. */
export function parseRedemptionResult(body: unknown): RedemptionResult | null {
  if (!isRecord(body) || !isRecord(body.redemption)) return null;
  const { redemption, balance_after: balanceAfter } = body;
  if (
    typeof redemption.reference !== 'string' ||
    typeof redemption.amount !== 'number' ||
    typeof balanceAfter !== 'number'
  ) {
    return null;
  }
  return { reference: redemption.reference, amount: redemption.amount, balanceAfter };
}
