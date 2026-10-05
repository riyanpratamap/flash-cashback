import { QueryClientProvider } from '@tanstack/react-query';
import { Stack } from 'expo-router';
import { useState } from 'react';

import { createQueryClient } from '@/api/query-client';
import { AttemptNavigator } from '@/attempts/AttemptNavigator';
import { AttemptProvider } from '@/attempts/AttemptProvider';
import { CHECKING_SCREEN_OPTIONS } from '@/attempts/blockBack';
import { UserProvider } from '@/user/UserProvider';

export default function RootLayout() {
  const [queryClient] = useState(createQueryClient);
  return (
    <QueryClientProvider client={queryClient}>
      <UserProvider>
        <AttemptProvider>
          <AttemptNavigator />
          <Stack screenOptions={{ headerBackButtonDisplayMode: 'minimal' }}>
            <Stack.Screen name="index" options={{ title: 'Flash Cashback', headerShown: false }} />
            <Stack.Screen name="pay" options={{ title: 'Make a payment' }} />
            <Stack.Screen name="payment-result" options={{ headerShown: false, gestureEnabled: false }} />
            <Stack.Screen name="checking" options={CHECKING_SCREEN_OPTIONS} />
            <Stack.Screen name="redeem" options={{ title: 'Redeem cashback' }} />
            <Stack.Screen name="history" options={{ title: 'Cashback history' }} />
            <Stack.Screen name="how-it-works" options={{ title: 'How Flash Cashback works' }} />
          </Stack>
        </AttemptProvider>
      </UserProvider>
    </QueryClientProvider>
  );
}
