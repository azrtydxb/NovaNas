import { createFileRoute, redirect } from '@tanstack/react-router';

export const Route = createFileRoute('/_auth/storage/')({
  beforeLoad: () => {
    throw redirect({ to: '/storage/pools' });
  },
});
