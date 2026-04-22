import { createFileRoute, redirect } from '@tanstack/react-router';

export const Route = createFileRoute('/_auth/data-protection/')({
  beforeLoad: () => {
    throw redirect({ to: '/data-protection/schedules' });
  },
});
