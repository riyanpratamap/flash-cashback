import { isRecord } from '@/api/guards';

export type PaymentResult = {
  reference: string;
  amount: number;
  createdAt: string;
  awarded: number;
  reason: string;
};

/** The fields of a `POST /payments` answer that the result shows; null when the body is not that shape. */
export function parsePaymentResult(body: unknown): PaymentResult | null {
  if (!isRecord(body) || !isRecord(body.payment) || !isRecord(body.cashback)) return null;
  const { payment, cashback } = body;
  if (
    typeof payment.reference !== 'string' ||
    typeof payment.created_at !== 'string' ||
    typeof payment.amount !== 'number' ||
    typeof cashback.awarded !== 'number' ||
    typeof cashback.reason !== 'string'
  ) {
    return null;
  }
  return {
    reference: payment.reference,
    amount: payment.amount,
    createdAt: payment.created_at,
    awarded: cashback.awarded,
    reason: cashback.reason,
  };
}
