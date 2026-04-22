import { installMockFetch, makeQueryClient, wrapper } from '@/api/test-utils';
import { render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { Route as AppsRoute } from './_auth.apps';

const AppsPage = AppsRoute.options.component!;

describe('AppsPage', () => {
  let fetchMock: ReturnType<typeof installMockFetch>;
  beforeEach(() => {
    fetchMock = installMockFetch();
  });
  afterEach(() => fetchMock.restore());

  const renderPage = () => {
    const qc = makeQueryClient();
    const Wrapper = wrapper(qc);
    return render(<AppsPage />, { wrapper: Wrapper });
  };

  it('shows a catalog card and an installed-app row', async () => {
    fetchMock.enqueue({ match: '/datasets', status: 200, body: { items: [] } });
    // Catalog
    fetchMock.enqueue({
      match: '/apps-available',
      status: 200,
      body: {
        items: [
          {
            apiVersion: 'novanas.io/v1alpha1',
            kind: 'App',
            metadata: { name: 'jellyfin' },
            spec: {
              displayName: 'Jellyfin',
              version: '10.8.0',
              description: 'Media server',
              category: 'Media',
              chart: {},
            },
          },
        ],
      },
    });
    // Installed
    fetchMock.enqueue({
      match: '/apps',
      status: 200,
      body: {
        items: [
          {
            apiVersion: 'novanas.io/v1alpha1',
            kind: 'AppInstance',
            metadata: { name: 'jf-1' },
            spec: { app: 'jellyfin', version: '10.8.0' },
            status: { phase: 'Running' },
          },
        ],
      },
    });
    renderPage();
    await waitFor(() => expect(screen.getByText('Jellyfin')).toBeInTheDocument());
    await waitFor(() => expect(screen.getByText('jf-1')).toBeInTheDocument());
  });
});
