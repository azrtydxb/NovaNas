import { installMockFetch, makeQueryClient, wrapper } from '@/api/test-utils';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { Route as PoolsRoute } from './_auth.storage.pools';

const PoolsPage = PoolsRoute.options.component!;

describe('PoolsPage', () => {
  let fetchMock: ReturnType<typeof installMockFetch>;

  beforeEach(() => {
    fetchMock = installMockFetch();
  });
  afterEach(() => fetchMock.restore());

  const renderPage = () => {
    const qc = makeQueryClient();
    const Wrapper = wrapper(qc);
    return render(<PoolsPage />, { wrapper: Wrapper });
  };

  it('shows a loading skeleton then a list row', async () => {
    fetchMock.enqueue({
      status: 200,
      body: {
        items: [
          {
            apiVersion: 'novanas.io/v1alpha1',
            kind: 'StoragePool',
            metadata: { name: 'fast' },
            spec: { tier: 'hot' },
            status: { phase: 'Active', diskCount: 3 },
          },
        ],
      },
    });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText('fast')).toBeInTheDocument();
    });
    // The Tier badge renders "Tier <value>"; match the inner span only.
    const tierMatches = screen.getAllByText(
      (_content, el) => el?.tagName === 'SPAN' && el.textContent?.trim() === 'Tier hot'
    );
    expect(tierMatches.length).toBeGreaterThan(0);
  });

  it('shows empty state when no pools', async () => {
    fetchMock.enqueue({ status: 200, body: { items: [] } });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText(/no pools yet/i)).toBeInTheDocument();
    });
  });

  it('opens the create dialog and validates required fields', async () => {
    fetchMock.enqueue({ status: 200, body: { items: [] } });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText(/no pools yet/i)).toBeInTheDocument();
    });
    // Click the first "Create pool" CTA (header button).
    const buttons = screen.getAllByRole('button', { name: /create pool/i });
    fireEvent.click(buttons[0]!);
    // Dialog renders a second "Create pool" button (submit) — disabled initially.
    await waitFor(() => {
      const allCreate = screen.getAllByRole('button', { name: /create pool/i });
      const submit = allCreate[allCreate.length - 1]!;
      expect(submit).toBeDisabled();
    });
  });

  it('shows an error state on failure', async () => {
    fetchMock.enqueue({ status: 500, body: { message: 'boom' } });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText(/unable to load pools/i)).toBeInTheDocument();
    });
  });
});
