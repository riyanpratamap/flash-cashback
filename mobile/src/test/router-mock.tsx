import { useEffect } from 'react';

// A stand-in for expo-router in screen tests: navigation is recorded, focus fires once on mount, and tests can fire it again.
export const push = jest.fn<void, [string]>();

type FocusEffect = () => void | (() => void);
const focusEffects = new Set<FocusEffect>();

export const replace = jest.fn<void, [string]>();
export const back = jest.fn<void, []>();
export const dismissTo = jest.fn<void, [string]>();

const router = { push, replace, back, dismissTo };

/** Clears every recorded navigation. */
export function resetRouter() {
  [push, replace, back, dismissTo].forEach((fn) => fn.mockReset());
}

export function useRouter() {
  return router;
}

export function useFocusEffect(effect: FocusEffect) {
  useEffect(() => {
    focusEffects.add(effect);
    const cleanup = effect();
    return () => {
      focusEffects.delete(effect);
      if (typeof cleanup === 'function') cleanup();
    };
  }, [effect]);
}

/** The screen regains focus (the user returns from another screen). */
export function regainFocus() {
  focusEffects.forEach((effect) => effect());
}
