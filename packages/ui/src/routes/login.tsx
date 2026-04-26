import { Brand } from '@/components/chrome/brand';
import { Button } from '@/components/ui/button';
import { Card, CardBody } from '@/components/ui/card';
import { ApiError, api } from '@/lib/api';
import { i18n } from '@/lib/i18n';
import { Trans } from '@lingui/react';
import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { LogIn } from 'lucide-react';
import { useState } from 'react';

export const Route = createFileRoute('/login')({
  component: LoginPage,
});

function LoginPage() {
  const navigate = useNavigate();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!username || !password || busy) return;
    setBusy(true);
    setErr(null);
    try {
      await api.post('/auth/password-login', { username, password });
      // Hard navigation so React Query re-runs and the new session
      // cookie is picked up everywhere.
      window.location.href = '/';
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        setErr(i18n._('Invalid username or password.'));
      } else if (e instanceof ApiError && e.status === 502) {
        setErr(i18n._('Authentication service is unavailable. Please try again.'));
      } else if (e instanceof Error) {
        setErr(e.message);
      } else {
        setErr(i18n._('Sign-in failed.'));
      }
      setBusy(false);
      // useNavigate is referenced so the import isn't pruned by tree-shaking
      // (and stays handy for future "redirect after login" UX).
      void navigate;
    }
  }

  return (
    <div className='min-h-screen grid place-items-center bg-surface'>
      <div className='w-full max-w-md px-6'>
        <div className='mb-6 flex justify-center'>
          <Brand hostname='nas-01' />
        </div>

        <Card>
          <CardBody className='p-6 flex flex-col gap-4'>
            <div className='flex flex-col gap-1'>
              <h1 className='text-xl font-semibold text-foreground tracking-tight'>
                <Trans id='Sign in to NovaNas' />
              </h1>
              <p className='text-sm text-foreground-muted'>
                <Trans id='Enter your NovaNas account credentials.' />
              </p>
            </div>

            <form className='flex flex-col gap-3' onSubmit={handleSubmit} autoComplete='on'>
              <label className='flex flex-col gap-1 text-sm'>
                <span className='text-foreground-muted'>
                  <Trans id='Username' />
                </span>
                <input
                  className='rounded-md border border-border bg-surface px-3 py-2 text-foreground focus:outline-none focus:ring-2 focus:ring-accent'
                  type='text'
                  name='username'
                  autoComplete='username'
                  // biome-ignore lint/a11y/noAutofocus: login form is the only meaningful input on the page
                  autoFocus
                  required
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  disabled={busy}
                />
              </label>

              <label className='flex flex-col gap-1 text-sm'>
                <span className='text-foreground-muted'>
                  <Trans id='Password' />
                </span>
                <input
                  className='rounded-md border border-border bg-surface px-3 py-2 text-foreground focus:outline-none focus:ring-2 focus:ring-accent'
                  type='password'
                  name='password'
                  autoComplete='current-password'
                  required
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  disabled={busy}
                />
              </label>

              {err && (
                <div className='text-xs text-danger bg-danger-soft rounded-md px-3 py-2'>{err}</div>
              )}

              <Button
                type='submit'
                variant='primary'
                size='lg'
                disabled={busy || !username || !password}
              >
                <LogIn size={15} />
                {busy ? <Trans id='Signing in…' /> : <Trans id='Sign in' />}
              </Button>
            </form>

            <div className='text-xs text-foreground-subtle text-center'>
              <Trans id='Trouble signing in? Contact your NovaNas administrator.' />
            </div>
          </CardBody>
        </Card>

        <div className='mt-4 text-center text-2xs text-foreground-faint'>
          <Trans id='NovaNas · Kubernetes-native NAS appliance' />
        </div>
      </div>
    </div>
  );
}
