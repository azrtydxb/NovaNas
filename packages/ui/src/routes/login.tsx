import { Brand } from '@/components/chrome/brand';
import { Button } from '@/components/ui/button';
import { Card, CardBody } from '@/components/ui/card';
import { getUserManager } from '@/lib/auth';
import { createFileRoute } from '@tanstack/react-router';
import { LogIn } from 'lucide-react';
import { useState } from 'react';

export const Route = createFileRoute('/login')({
  component: LoginPage,
});

function LoginPage() {
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  async function handleLogin() {
    setBusy(true);
    setErr(null);
    try {
      await getUserManager().signinRedirect();
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Failed to start sign-in');
      setBusy(false);
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
                Sign in to NovaNas
              </h1>
              <p className='text-sm text-foreground-muted'>
                Authenticate with your Keycloak account to manage storage, shares, apps and VMs.
              </p>
            </div>

            <Button variant='primary' size='lg' onClick={handleLogin} disabled={busy}>
              <LogIn size={15} />
              {busy ? 'Redirecting…' : 'Sign in with Keycloak'}
            </Button>

            {err && (
              <div className='text-xs text-danger bg-danger-soft rounded-md px-3 py-2'>{err}</div>
            )}

            <div className='text-xs text-foreground-subtle text-center'>
              Trouble signing in? Contact your NovaNas administrator.
            </div>
          </CardBody>
        </Card>

        <div className='mt-4 text-center text-2xs text-foreground-faint'>
          NovaNas · Kubernetes-native NAS appliance
        </div>
      </div>
    </div>
  );
}
