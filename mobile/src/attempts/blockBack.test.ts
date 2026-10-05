import { blocksLeaving } from '@/attempts/blockBack';

describe('blocksLeaving (AC-60)', () => {
  it.each(['sending', 'checking', 'waiting'] as const)('%s blocks leaving', (phase) => {
    expect(blocksLeaving(phase)).toBe(true);
  });

  it.each(['idle', 'saving', 'done', 'rejected'] as const)('%s lets the user leave', (phase) => {
    expect(blocksLeaving(phase)).toBe(false);
  });
});

// The Checking screen options are asserted where they are wired: src/app/_layout.test.tsx.
