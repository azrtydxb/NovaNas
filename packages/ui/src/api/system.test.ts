import { waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { useSystemHealth, useSystemInfo } from './system';
import { installMockFetch, renderHookWithClient } from './test-utils';

describe('system api hooks', () => {
  let fetchMock: ReturnType<typeof installMockFetch>;
  beforeEach(() => {
    fetchMock = installMockFetch();
  });
  afterEach(() => fetchMock.restore());

  it('useSystemHealth returns health', async () => {
    fetchMock.enqueue({
      status: 200,
      body: {
        status: 'ok',
        capacity: { usedBytes: 1, totalBytes: 2 },
        pools: { online: 1, total: 1 },
        disks: { active: 2, total: 2 },
        apps: { running: 0, installed: 0 },
        vms: { running: 0, defined: 0 },
        services: [],
      },
    });
    const { result } = renderHookWithClient(() => useSystemHealth());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.status).toBe('ok');
  });

  it('useSystemInfo returns 500 as error', async () => {
    fetchMock.enqueue({ status: 500, body: { message: 'err' } });
    const { result } = renderHookWithClient(() => useSystemInfo());
    await waitFor(() => expect(result.current.isError).toBe(true), { timeout: 2000 });
  });
});
