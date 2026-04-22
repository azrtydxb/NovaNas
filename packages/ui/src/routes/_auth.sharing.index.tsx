// TODO(i18n-wave-12): strings on this page are still raw English. Migrate to <Trans>/i18n._() once wave 12 is green.
import { createFileRoute, redirect } from '@tanstack/react-router';

export const Route = createFileRoute('/_auth/sharing/')({
  beforeLoad: () => {
    throw redirect({ to: '/sharing/shares' });
  },
});
