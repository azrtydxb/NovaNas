import { installMockFetch, makeQueryClient, wrapper } from '@/api/test-utils';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { Route as VmsRoute } from './_auth.vms';

const VmsPage = VmsRoute.options.component!;

describe('VmsPage', () => {
  let fetchMock: ReturnType<typeof installMockFetch>;
  beforeEach(() => {
    fetchMock = installMockFetch();
  });
  afterEach(() => fetchMock.restore());

  const renderPage = () => {
    const qc = makeQueryClient();
    const Wrapper = wrapper(qc);
    return render(<VmsPage />, { wrapper: Wrapper });
  };

  it('shows a VM row', async () => {
    // Datasets and ISO libraries are fetched by the (hidden) create-wizard dialog.
    fetchMock.enqueue({ match: '/datasets', status: 200, body: { items: [] } });
    fetchMock.enqueue({ match: '/iso-libraries', status: 200, body: { items: [] } });
    fetchMock.enqueue({
      match: '/vms',
      status: 200,
      body: {
        items: [
          {
            apiVersion: 'novanas.io/v1alpha1',
            kind: 'Vm',
            metadata: { name: 'vm01' },
            spec: {
              os: { type: 'linux' },
              resources: { cpu: 2, memoryMiB: 2048 },
            },
            status: { phase: 'Running', ip: '10.0.0.5' },
          },
        ],
      },
    });
    renderPage();
    await waitFor(() => expect(screen.getByText('vm01')).toBeInTheDocument(), {
      timeout: 3000,
    });
    expect(screen.getByText('Running')).toBeInTheDocument();
  });

  it('shows empty state and opens create wizard', async () => {
    fetchMock.enqueue({ match: '/datasets', status: 200, body: { items: [] } });
    fetchMock.enqueue({ match: '/iso-libraries', status: 200, body: { items: [] } });
    fetchMock.enqueue({ match: '/vms', status: 200, body: { items: [] } });
    renderPage();
    await waitFor(() => expect(screen.getByText(/no vms yet/i)).toBeInTheDocument());
    const buttons = screen.getAllByRole('button', { name: /new vm/i });
    fireEvent.click(buttons[0]!);
    await waitFor(() => {
      expect(screen.getByText(/configure os, resources/i)).toBeInTheDocument();
    });
  });
});
