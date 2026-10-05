import { parseRedemptionResult } from './redemptions';

const body = {
  redemption: { id: 3, reference: 'RDM-20261003-000003', amount: 18000, status: 'COMPLETED', destination: 'MAIN_ACCOUNT' },
  balance_after: 7000,
};

describe('parseRedemptionResult', () => {
  it('reads the fields the confirmation shows', () => {
    expect(parseRedemptionResult(body)).toEqual({ reference: 'RDM-20261003-000003', amount: 18000, balanceAfter: 7000 });
  });

  it('is null when the body is not that shape', () => {
    expect(parseRedemptionResult(null)).toBeNull();
    expect(parseRedemptionResult({ balance_after: 1 })).toBeNull();
    expect(parseRedemptionResult({ ...body, balance_after: '7000' })).toBeNull();
    expect(parseRedemptionResult({ ...body, redemption: { ...body.redemption, amount: 'x' } })).toBeNull();
    expect(parseRedemptionResult({ ...body, redemption: { ...body.redemption, reference: 1 } })).toBeNull();
  });
});
