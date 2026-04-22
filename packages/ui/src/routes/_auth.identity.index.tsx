import { createFileRoute, redirect } from '@tanstack/react-router';

export const Route = createFileRoute('/_auth/identity/')({
  beforeLoad: () => {
    throw redirect({ to: '/identity/users' });
  },
});
