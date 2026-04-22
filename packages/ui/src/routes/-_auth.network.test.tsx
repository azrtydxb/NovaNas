import { installMockFetch, makeQueryClient, wrapper } from '@/api/test-utils';
import { render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { Route as NetworkRoute } from './_auth.network';

const NetworkPage = NetworkRoute.options.component!;

describe('NetworkPage', () => {
  let fetchMock: ReturnType<typeof installMockFetch>;
  beforeEach(() => {
    fetchMock = installMockFetch();
  });
  afterEach(() => fetchMock.restore());

  const renderPage = () => {
    const qc = makeQueryClient();
    const Wrapper = wrapper(qc);
    return render(<NetworkPage />, { wrapper: Wrapper });
  };

  it('renders the Interfaces tab by default', async () => {
    // Enqueue empty lists for every request the default tab issues.
    for (let i = 0; i < 8; i++) fetchMock.enqueue({ status: 200, body: { items: [] } });
    renderPage();
    await waitFor(() => expect(screen.getByText('Physical interfaces')).toBeInTheDocument());
    expect(screen.getByText('Bonds')).toBeInTheDocument();
    expect(screen.getByText('VLANs')).toBeInTheDocument();
  });

  it('shows a bond row when one is returned', async () => {
    fetchMock.enqueue({ match: '/physical-interfaces', status: 200, body: { items: [] } });
    fetchMock.enqueue({
      match: '/bonds',
      status: 200,
      body: {
        items: [
          {
            apiVersion: 'novanas.io/v1alpha1',
            kind: 'Bond',
            metadata: { name: 'bond0' },
            spec: { interfaces: ['eth0', 'eth1'], mode: '802.3ad' },
          },
        ],
      },
    });
    fetchMock.enqueue({ match: '/vlans', status: 200, body: { items: [] } });
    fetchMock.enqueue({ match: '/host-interfaces', status: 200, body: { items: [] } });
    for (let i = 0; i < 8; i++) fetchMock.enqueue({ status: 200, body: { items: [] } });
    renderPage();
    await waitFor(() => expect(screen.getByText('bond0')).toBeInTheDocument());
  });
});
