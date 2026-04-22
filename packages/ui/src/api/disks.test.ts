import { act, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { useDisks, useUpdateDisk } from './disks';
import { installMockFetch, renderHookWithClient } from './test-utils';

describe('disks api hooks', () => {
  let fetchMock: ReturnType<typeof installMockFetch>;
  beforeEach(() => {
    fetchMock = installMockFetch();
  });
  afterEach(() => fetchMock.restore());

  it('useDisks returns items', async () => {
    fetchMock.enqueue({
      status: 200,
      body: {
        items: [
          {
            apiVersion: 'novanas.io/v1alpha1',
            kind: 'Disk',
            metadata: { name: 'wwn-0x1' },
            spec: {},
            status: { wwn: 'wwn-0x1', slot: '1', state: 'ACTIVE' },
          },
        ],
      },
    });
    const { result } = renderHookWithClient(() => useDisks());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.[0]?.status?.slot).toBe('1');
  });

  it('useUpdateDisk PATCHes', async () => {
    fetchMock.enqueue({
      status: 200,
      body: {
        apiVersion: 'novanas.io/v1alpha1',
        kind: 'Disk',
        metadata: { name: 'wwn-0x1' },
        spec: { pool: 'fast' },
      },
    });
    const { result } = renderHookWithClient(() => useUpdateDisk('wwn-0x1'));
    await act(async () => {
      await result.current.mutateAsync({ spec: { pool: 'fast' } });
    });
    expect(fetchMock.calls[0]?.init?.method).toBe('PATCH');
  });

  it('surfaces network errors via retry-friendly promise', async () => {
    // Simulate no enqueued mock -> default OK, then override fetch to throw.
    const globalObj = globalThis as unknown as { fetch: typeof fetch };
    const prev = globalObj.fetch;
    globalObj.fetch = (async () => {
      throw new TypeError('Failed to fetch');
    }) as unknown as typeof fetch;
    const { result } = renderHookWithClient(() => useDisks());
    await waitFor(() => expect(result.current.isError).toBe(true), { timeout: 3000 });
    globalObj.fetch = prev;
  });
});
