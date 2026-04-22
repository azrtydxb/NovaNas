import { ApiError, api } from '@/lib/api';
import { getUserManager } from '@/lib/auth';
import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { useEffect, useState } from 'react';

export const Route = createFileRoute('/auth/callback')({
  component: AuthCallback,
});

function AuthCallback() {
  const navigate = useNavigate();
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        // Finish the OIDC redirect — extract code/state from the URL.
        const userManager = getUserManager();
        const oidcUser = await userManager.signinRedirectCallback();
        // Exchange the code with the NovaNas API so it can set the session cookie.
        await api.post('/auth/callback', {
          code: oidcUser.state,
          access_token: oidcUser.access_token,
          id_token: oidcUser.id_token,
        });
        if (!cancelled) {
          await navigate({ to: '/dashboard', replace: true });
        }
      } catch (e) {
        if (cancelled) return;
        if (e instanceof ApiError) {
          setError(e.message);
        } else if (e instanceof Error) {
          setError(e.message);
        } else {
          setError('Unknown sign-in error');
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [navigate]);

  return (
    <div className='min-h-screen grid place-items-center bg-surface'>
      <div className='flex flex-col items-center gap-3'>
        {error ? (
          <>
            <div className='text-danger font-medium'>Sign-in failed</div>
            <div className='text-sm text-foreground-muted'>{error}</div>
            <a href='/login' className='text-accent underline underline-offset-2 text-sm'>
              Back to login
            </a>
          </>
        ) : (
          <div className='text-foreground-muted text-sm'>Completing sign-in…</div>
        )}
      </div>
    </div>
  );
}
