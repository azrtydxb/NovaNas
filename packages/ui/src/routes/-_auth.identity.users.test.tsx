import { installMockFetch, makeQueryClient, wrapper } from '@/api/test-utils';
import { render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { Route as UsersRoute } from './_auth.identity.users';

const UsersPage = UsersRoute.options.component!;

describe('UsersPage', () => {
  let fetchMock: ReturnType<typeof installMockFetch>;
  beforeEach(() => {
    fetchMock = installMockFetch();
  });
  afterEach(() => fetchMock.restore());

  const renderPage = () => {
    const qc = makeQueryClient();
    const Wrapper = wrapper(qc);
    return render(<UsersPage />, { wrapper: Wrapper });
  };

  it('shows a user row', async () => {
    fetchMock.enqueue({ match: '/groups', status: 200, body: { items: [] } });
    fetchMock.enqueue({
      match: '/users',
      status: 200,
      body: {
        items: [
          {
            apiVersion: 'novanas.io/v1alpha1',
            kind: 'User',
            metadata: { name: 'alice' },
            spec: { username: 'alice', email: 'a@b.co' },
          },
        ],
      },
    });
    renderPage();
    await waitFor(() => expect(screen.getByText('alice')).toBeInTheDocument());
    expect(screen.getByText('a@b.co')).toBeInTheDocument();
  });

  it('shows the empty state when no users exist', async () => {
    fetchMock.enqueue({ match: '/groups', status: 200, body: { items: [] } });
    fetchMock.enqueue({ match: '/users', status: 200, body: { items: [] } });
    renderPage();
    await waitFor(() => expect(screen.getByText(/no users yet/i)).toBeInTheDocument());
  });
});
