import { installMockFetch, makeQueryClient, wrapper } from '@/api/test-utils';
import { render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { Route as S3Route } from './_auth.sharing.s3';

const S3Page = S3Route.options.component!;

describe('S3Page (buckets)', () => {
  let fetchMock: ReturnType<typeof installMockFetch>;
  beforeEach(() => {
    fetchMock = installMockFetch();
  });
  afterEach(() => fetchMock.restore());

  const renderPage = () => {
    const qc = makeQueryClient();
    const Wrapper = wrapper(qc);
    return render(<S3Page />, { wrapper: Wrapper });
  };

  it('shows a bucket row', async () => {
    fetchMock.enqueue({ match: '/object-stores', status: 200, body: { items: [] } });
    fetchMock.enqueue({ match: '/bucket-users', status: 200, body: { items: [] } });
    fetchMock.enqueue({
      match: '/buckets',
      status: 200,
      body: {
        items: [
          {
            apiVersion: 'novanas.io/v1alpha1',
            kind: 'Bucket',
            metadata: { name: 'media' },
            spec: { store: 's3-main', versioning: 'enabled' },
            status: { phase: 'Active' },
          },
        ],
      },
    });
    renderPage();
    await waitFor(() => expect(screen.getByText('media')).toBeInTheDocument());
  });

  it('shows empty state', async () => {
    fetchMock.enqueue({ match: '/object-stores', status: 200, body: { items: [] } });
    fetchMock.enqueue({ match: '/bucket-users', status: 200, body: { items: [] } });
    fetchMock.enqueue({ match: '/buckets', status: 200, body: { items: [] } });
    renderPage();
    await waitFor(() => expect(screen.getByText(/no buckets yet/i)).toBeInTheDocument());
  });

  it('renders an action button for creating a bucket', async () => {
    fetchMock.enqueue({ match: '/object-stores', status: 200, body: { items: [] } });
    fetchMock.enqueue({ match: '/bucket-users', status: 200, body: { items: [] } });
    fetchMock.enqueue({ match: '/buckets', status: 200, body: { items: [] } });
    renderPage();
    await waitFor(() => expect(screen.getByText(/no buckets yet/i)).toBeInTheDocument());
    const buttons = screen.getAllByRole('button', { name: /create bucket/i });
    expect(buttons.length).toBeGreaterThan(0);
  });
});
