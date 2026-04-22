import { installMockFetch, makeQueryClient, wrapper } from '@/api/test-utils';
import { render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { Route as SharesRoute } from './_auth.sharing.shares';

const SharesPage = SharesRoute.options.component!;

describe('SharesPage', () => {
  let fetchMock: ReturnType<typeof installMockFetch>;
  beforeEach(() => {
    fetchMock = installMockFetch();
  });
  afterEach(() => fetchMock.restore());

  const renderPage = () => {
    const qc = makeQueryClient();
    const Wrapper = wrapper(qc);
    return render(<SharesPage />, { wrapper: Wrapper });
  };

  it('shows a share row with protocol badges', async () => {
    fetchMock.enqueue({ match: '/datasets', status: 200, body: { items: [] } });
    fetchMock.enqueue({ match: '/smb-servers', status: 200, body: { items: [] } });
    fetchMock.enqueue({ match: '/nfs-servers', status: 200, body: { items: [] } });
    fetchMock.enqueue({
      match: '/shares',
      status: 200,
      body: {
        items: [
          {
            apiVersion: 'novanas.io/v1alpha1',
            kind: 'Share',
            metadata: { name: 'media' },
            spec: {
              dataset: 'ds1',
              path: '/',
              protocols: { smb: { server: 'smb0' }, nfs: { server: 'nfs0' } },
            },
          },
        ],
      },
    });
    renderPage();
    await waitFor(() => expect(screen.getByText('media')).toBeInTheDocument());
    expect(screen.getByText('SMB')).toBeInTheDocument();
    expect(screen.getByText('NFS')).toBeInTheDocument();
  });

  it('shows empty state with a new-share action', async () => {
    fetchMock.enqueue({ match: '/datasets', status: 200, body: { items: [] } });
    fetchMock.enqueue({ match: '/smb-servers', status: 200, body: { items: [] } });
    fetchMock.enqueue({ match: '/nfs-servers', status: 200, body: { items: [] } });
    fetchMock.enqueue({ match: '/shares', status: 200, body: { items: [] } });
    renderPage();
    await waitFor(() => expect(screen.getByText(/no shares yet/i)).toBeInTheDocument());
    const buttons = screen.getAllByRole('button', { name: /new share/i });
    expect(buttons.length).toBeGreaterThan(0);
  });
});
