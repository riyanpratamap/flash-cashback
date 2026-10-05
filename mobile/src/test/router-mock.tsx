import { useEffect } from 'react';

// A stand-in for expo-router in screen tests: navigation is recorded, focus fires once on mount, and tests can fire it again.
export const push = jest.fn<void, [string]>();

type FocusEffect = () => void | (() => void);
const focusEffects = new Set<FocusEffect>();

export function useRouter() {
  return { push };
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
