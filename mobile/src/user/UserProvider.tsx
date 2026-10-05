import AsyncStorage from '@react-native-async-storage/async-storage';
import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';

import { DEFAULT_USER, DEMO_USERS, type DemoUser } from './users';

export const USER_STORAGE_KEY = 'fc:user';

type UserContextValue = { user: DemoUser; setUser: (user: DemoUser) => Promise<void> };

const UserContext = createContext<UserContextValue | null>(null);

function isDemoUser(value: unknown): value is DemoUser {
  return DEMO_USERS.some((u) => u === value);
}

async function readStoredUser(): Promise<DemoUser> {
  try {
    const stored = await AsyncStorage.getItem(USER_STORAGE_KEY);
    return isDemoUser(stored) ? stored : DEFAULT_USER;
  } catch {
    return DEFAULT_USER; // storage unreadable: start as the default demo user
  }
}

/** The demo user (D36). Renders nothing until the stored choice is read, so no request goes out as the wrong user. */
export function UserProvider({ children }: { children: ReactNode }) {
  const [user, setSelected] = useState<DemoUser | null>(null);

  useEffect(() => {
    let cancelled = false;
    void readStoredUser().then((stored) => {
      if (!cancelled) setSelected(stored);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  const setUser = useCallback(async (next: DemoUser) => {
    setSelected(next);
    try {
      await AsyncStorage.setItem(USER_STORAGE_KEY, next);
    } catch {
      // The choice still applies for this launch; it just will not be remembered.
    }
  }, []);

  const value = useMemo(() => (user === null ? null : { user, setUser }), [user, setUser]);
  if (value === null) return null;
  return <UserContext.Provider value={value}>{children}</UserContext.Provider>;
}

export function useUser(): UserContextValue {
  const value = useContext(UserContext);
  if (value === null) throw new Error('useUser must be used inside UserProvider');
  return value;
}
