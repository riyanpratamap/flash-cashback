export type AttemptKind = 'payment' | 'redemption';

/** One money attempt as saved in AsyncStorage under `fc:attempts` (D48). `created_at` is the device clock, ISO 8601. */
export type SavedAttempt = {
  readonly user_id: string;
  readonly kind: AttemptKind;
  readonly amount: number;
  readonly key: string;
  readonly created_at: string;
};
