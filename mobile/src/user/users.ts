export const DEMO_USERS = ['user_a', 'user_b', 'user_c'] as const;
export type DemoUser = (typeof DEMO_USERS)[number];
export const DEFAULT_USER: DemoUser = 'user_a';
