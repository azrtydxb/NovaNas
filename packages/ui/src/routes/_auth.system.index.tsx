import { createFileRoute, redirect } from '@tanstack/react-router';

export const Route = createFileRoute('/_auth/system/')({
  beforeLoad: () => {
    throw redirect({ to: '/system/settings' });
  },
});
