import { QueryClientProvider } from '@tanstack/react-query';
import { Stack } from 'expo-router';
import { useState } from 'react';

import { createQueryClient } from '@/api/query-client';
import { UserProvider } from '@/user/UserProvider';

export default function RootLayout() {
  const [queryClient] = useState(createQueryClient);
  return (
    <QueryClientProvider client={queryClient}>
      <UserProvider>
        <Stack />
      </UserProvider>
    </QueryClientProvider>
  );
}
