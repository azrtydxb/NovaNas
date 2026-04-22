import { waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { useRecentAudit } from './audit';
import { installMockFetch, renderHookWithClient } from './test-utils';

describe('audit api hooks', () => {
  let fetchMock: ReturnType<typeof installMockFetch>;
  beforeEach(() => {
    fetchMock = installMockFetch();
  });
  afterEach(() => fetchMock.restore());

  it('useRecentAudit passes the limit', async () => {
    fetchMock.enqueue({ status: 200, body: [] });
    const { result } = renderHookWithClient(() => useRecentAudit(5));
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetchMock.calls[0]?.url).toMatch(/limit=5/);
  });

  it('surfaces 500', async () => {
    fetchMock.enqueue({ status: 500, body: { message: 'err' } });
    const { result } = renderHookWithClient(() => useRecentAudit(5));
    await waitFor(() => expect(result.current.isError).toBe(true), { timeout: 2000 });
  });
});
