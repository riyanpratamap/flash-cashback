import { act, renderHook } from '@testing-library/react-native';

import { useAttempts } from '@/attempts/AttemptProvider';
import type { AttemptState } from '@/attempts/machine';
import { useAmountForm } from '@/attempts/useAmountForm';
import type { AttemptKind } from '@/attempts/types';

jest.mock('@/attempts/AttemptProvider', () => ({ useAttempts: jest.fn() }));

const acknowledge = jest.fn();
let current: AttemptState = { phase: 'idle' };
const mockedUseAttempts = jest.mocked(useAttempts);

const attempt = (kind: AttemptKind, amount: number) => ({
  user_id: 'user_a',
  kind,
  amount,
  key: 'k1',
  created_at: '2026-10-03T14:32:00+07:00',
});
const rejected = (kind: AttemptKind, amount: number): AttemptState => ({
  phase: 'rejected',
  attempt: attempt(kind, amount),
  code: 'INVALID_AMOUNT',
  message: 'm',
  requestId: 'r',
});

beforeEach(() => {
  acknowledge.mockReset();
  current = { phase: 'idle' };
  mockedUseAttempts.mockImplementation(
    () => ({ state: current, acknowledge }) as unknown as ReturnType<typeof useAttempts>,
  );
});

describe('useAmountForm', () => {
  it('prefills from a rejected state of the same kind at mount', async () => {
    current = rejected('payment', 1500000);
    const { result } = await renderHook(() => useAmountForm('payment'));
    expect(result.current.text).toBe('1.500.000');
    expect(result.current.amount).toBe(1500000);
  });

  it('does not prefill from a rejection of the other kind', async () => {
    current = rejected('redemption', 1500000);
    const { result } = await renderHook(() => useAmountForm('payment'));
    expect(result.current.text).toBe('');
    expect(result.current.rejection).toBeNull();
  });

  it('fills the text once when a rejection arrives later, and never overwrites later typing', async () => {
    const { result, rerender } = await renderHook(() => useAmountForm('redemption'));
    expect(result.current.text).toBe('');
    current = rejected('redemption', 7000);
    await rerender({});
    expect(result.current.text).toBe('7.000');
    await act(async () => result.current.setText('12345'));
    expect(result.current.text).toBe('12.345');
    await rerender({});
    expect(result.current.text).toBe('12.345');
  });

  it('reports the rejection only while the typed amount is the rejected amount', async () => {
    current = rejected('payment', 7000);
    const { result } = await renderHook(() => useAmountForm('payment'));
    expect(result.current.rejection?.code).toBe('INVALID_AMOUNT');
    await act(async () => result.current.setText('7001'));
    expect(result.current.rejection).toBeNull();
  });

  it('does not report a rejection of the other kind for the same amount', async () => {
    current = rejected('redemption', 7000);
    const { result } = await renderHook(() => useAmountForm('payment'));
    await act(async () => result.current.setText('7000'));
    expect(result.current.amount).toBe(7000);
    expect(result.current.rejection).toBeNull();
  });

  it.each([
    ['saving', { phase: 'saving', attempt: attempt('payment', 1) }, true],
    ['sending', { phase: 'sending', attempt: attempt('payment', 1) }, true],
    ['checking', { phase: 'checking', attempt: attempt('payment', 1) }, false],
    ['idle', { phase: 'idle' }, false],
    ['rejected', rejected('payment', 1), false],
  ] as [string, AttemptState, boolean][])('tells inFlight in the %s phase', async (_phase, state, expected) => {
    current = state;
    const { result } = await renderHook(() => useAmountForm('payment'));
    expect(result.current.inFlight).toBe(expected);
  });

  it('acknowledges a rejection on leaving Pay', async () => {
    const { unmount } = await renderHook(() => useAmountForm('payment'));
    expect(acknowledge).not.toHaveBeenCalled();
    await unmount();
    expect(acknowledge.mock.calls).toEqual([['rejected']]);
  });

  it('acknowledges done, then the rejection, on leaving Redeem', async () => {
    const { unmount } = await renderHook(() => useAmountForm('redemption'));
    await unmount();
    expect(acknowledge.mock.calls).toEqual([['done'], ['rejected']]);
  });
});
