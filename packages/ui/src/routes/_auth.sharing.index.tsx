import { createFileRoute, redirect } from '@tanstack/react-router';

export const Route = createFileRoute('/_auth/sharing/')({
  beforeLoad: () => {
    throw redirect({ to: '/sharing/shares' });
  },
});
