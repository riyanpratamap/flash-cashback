import { parsePaymentResult } from '@/api/payments';

const body = {
  payment: { id: 42, reference: 'PAY-20261003-000042', amount: 100000, status: 'SUCCEEDED', created_at: '2026-10-03T14:32:00+07:00' },
  cashback: { awarded: 5000, reason: 'AWARDED' },
};

describe('parsePaymentResult', () => {
  it('reads the fields the result shows', () => {
    expect(parsePaymentResult(body)).toEqual({
      reference: 'PAY-20261003-000042',
      amount: 100000,
      createdAt: '2026-10-03T14:32:00+07:00',
      awarded: 5000,
      reason: 'AWARDED',
    });
  });

  it('keeps an unknown reason as sent', () => {
    expect(parsePaymentResult({ ...body, cashback: { awarded: 0, reason: 'NEW_ONE' } })?.reason).toBe('NEW_ONE');
  });

  it.each([null, 'x', {}, { payment: {}, cashback: {} }, { ...body, cashback: { awarded: '5000', reason: 'AWARDED' } }])(
    'is null for a body it cannot read: %j',
    (bad) => {
      expect(parsePaymentResult(bad)).toBeNull();
    },
  );
});
