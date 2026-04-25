import { Trans } from '@lingui/react';
import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { useEffect } from 'react';

/**
 * Vestigial route from the old client-side OIDC flow. The api now owns
 * /api/v1/auth/callback server-side and 302's the browser to the
 * original `redirectTo` after the code exchange — so the SPA route at
 * /auth/callback should never be hit. Keep it as a soft fallback that
 * just bounces to / so users who somehow land here aren't stranded.
 */
export const Route = createFileRoute('/auth/callback')({
  component: AuthCallback,
});

function AuthCallback() {
  const navigate = useNavigate();
  useEffect(() => {
    void navigate({ to: '/', replace: true });
  }, [navigate]);
  return (
    <div className='min-h-screen grid place-items-center bg-surface'>
      <div className='text-foreground-muted text-sm'>
        <Trans id='Completing sign-in…' />
      </div>
    </div>
  );
}
