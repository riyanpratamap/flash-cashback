import { formatDeviceStamp, formatStamp } from '@/copy/stamp';

describe('formatStamp', () => {
  it('reads the date and time out of the text as sent, whatever the device zone', () => {
    expect(formatStamp('2026-10-03T14:32:00+07:00')).toBe('3 Oct, 14:32');
    expect(formatStamp('2026-01-31T00:05:00+07:00')).toBe('31 Jan, 00:05');
  });
});

describe('formatDeviceStamp', () => {
  it('shows a moment the device itself recorded, in the device zone (tests run in UTC)', () => {
    expect(formatDeviceStamp('2026-10-03T14:32:00.000Z')).toBe('3 Oct, 14:32');
    expect(formatDeviceStamp('2026-01-31T00:05:09.000Z')).toBe('31 Jan, 00:05');
  });

  it('moves with the device zone, across a day boundary', () => {
    // The test runtime stays in UTC; a device in Jakarta (UTC+7) reports an offset of -420 minutes.
    const offset = jest.spyOn(Date.prototype, 'getTimezoneOffset').mockReturnValue(-420);
    try {
      expect(formatDeviceStamp('2026-10-03T14:32:00.000Z')).toBe('3 Oct, 21:32');
      expect(formatDeviceStamp('2026-01-31T20:05:09.000Z')).toBe('1 Feb, 03:05');
    } finally {
      offset.mockRestore();
    }
  });

  it('is empty for a time it cannot read', () => {
    expect(formatDeviceStamp('not a time')).toBe('');
  });
});
