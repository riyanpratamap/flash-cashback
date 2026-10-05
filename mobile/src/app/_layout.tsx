import { QueryClientProvider } from '@tanstack/react-query';
import { Stack } from 'expo-router';
import { useState } from 'react';

import { createQueryClient } from '@/api/query-client';
import { AttemptProvider } from '@/attempts/AttemptProvider';
import { UserProvider } from '@/user/UserProvider';

export default function RootLayout() {
  const [queryClient] = useState(createQueryClient);
  return (
    <QueryClientProvider client={queryClient}>
      <UserProvider>
        <AttemptProvider>
          <Stack>
            <Stack.Screen name="index" options={{ title: 'Flash Cashback', headerShown: false }} />
            <Stack.Screen name="history" options={{ title: 'Cashback history' }} />
            <Stack.Screen name="how-it-works" options={{ title: 'How Flash Cashback works' }} />
          </Stack>
        </AttemptProvider>
      </UserProvider>
    </QueryClientProvider>
  );
}
