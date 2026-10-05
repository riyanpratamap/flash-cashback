import { createQueryClient } from './query-client';

describe('createQueryClient', () => {
  it('retries a failed GET once (tech-spec §10)', () => {
    expect(createQueryClient().getDefaultOptions().queries?.retry).toBe(1);
  });
});
