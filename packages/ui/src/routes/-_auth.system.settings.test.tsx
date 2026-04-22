import { installMockFetch, makeQueryClient, wrapper } from '@/api/test-utils';
import { render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { Route as SettingsRoute } from './_auth.system.settings';

const SettingsPage = SettingsRoute.options.component!;

describe('system SettingsPage', () => {
  let fetchMock: ReturnType<typeof installMockFetch>;
  beforeEach(() => {
    fetchMock = installMockFetch();
  });
  afterEach(() => fetchMock.restore());

  const renderPage = () => {
    const qc = makeQueryClient();
    const Wrapper = wrapper(qc);
    return render(<SettingsPage />, { wrapper: Wrapper });
  };

  it('renders the hostname field from the loaded spec', async () => {
    fetchMock.enqueue({
      match: '/system/settings',
      status: 200,
      body: {
        apiVersion: 'novanas.io/v1alpha1',
        kind: 'SystemSettings',
        metadata: { name: 'system' },
        spec: { hostname: 'nova01', timezone: 'Europe/Brussels' },
      },
    });
    renderPage();
    await waitFor(() => expect(screen.getByDisplayValue('nova01')).toBeInTheDocument());
    expect(screen.getByDisplayValue('Europe/Brussels')).toBeInTheDocument();
  });

  it('renders error state on failure', async () => {
    fetchMock.enqueue({ match: '/system/settings', status: 500, body: { error: 'boom' } });
    renderPage();
    await waitFor(() => expect(screen.getByText(/unable to load settings/i)).toBeInTheDocument());
  });
});
