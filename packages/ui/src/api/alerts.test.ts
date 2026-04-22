import { waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { useActiveAlerts } from './alerts';
import { installMockFetch, renderHookWithClient } from './test-utils';

describe('alerts api hooks', () => {
  let fetchMock: ReturnType<typeof installMockFetch>;
  beforeEach(() => {
    fetchMock = installMockFetch();
  });
  afterEach(() => fetchMock.restore());

  it('useActiveAlerts calls /system/alerts with state=active', async () => {
    fetchMock.enqueue({ status: 200, body: [] });
    const { result } = renderHookWithClient(() => useActiveAlerts());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetchMock.calls[0]?.url).toMatch(/\/system\/alerts/);
    expect(fetchMock.calls[0]?.url).toMatch(/state=active/);
  });

  it('surfaces 403', async () => {
    fetchMock.enqueue({ status: 403, body: { message: 'forbidden' } });
    const { result } = renderHookWithClient(() => useActiveAlerts());
    await waitFor(() => expect(result.current.isError).toBe(true));
  });
});
