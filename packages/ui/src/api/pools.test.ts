import { act, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { useCreatePool, useDeletePool, usePool, usePools } from './pools';
import { installMockFetch, renderHookWithClient } from './test-utils';

describe('pools api hooks', () => {
  let fetchMock: ReturnType<typeof installMockFetch>;

  beforeEach(() => {
    fetchMock = installMockFetch();
  });
  afterEach(() => fetchMock.restore());

  it('usePools returns a list and normalizes { items: [] }', async () => {
    fetchMock.enqueue({
      status: 200,
      body: {
        items: [
          {
            apiVersion: 'novanas.io/v1alpha1',
            kind: 'StoragePool',
            metadata: { name: 'fast' },
            spec: { tier: 'hot' },
          },
        ],
      },
    });
    const { result } = renderHookWithClient(() => usePools());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.length).toBe(1);
    expect(result.current.data?.[0]?.metadata.name).toBe('fast');
    expect(fetchMock.calls[0]?.url).toMatch(/\/pools$/);
  });

  it('usePools surfaces 500 errors', async () => {
    fetchMock.enqueue({ status: 500, body: { message: 'boom' } });
    const { result } = renderHookWithClient(() => usePools());
    await waitFor(() => expect(result.current.isError).toBe(true), { timeout: 2000 });
  });

  it('usePool is disabled without a name', async () => {
    const { result } = renderHookWithClient(() => usePool(undefined));
    expect(result.current.fetchStatus).toBe('idle');
  });

  it('useCreatePool POSTs the body', async () => {
    fetchMock.enqueue({
      status: 200,
      body: {
        apiVersion: 'novanas.io/v1alpha1',
        kind: 'StoragePool',
        metadata: { name: 'fast' },
        spec: { tier: 'hot' },
      },
    });
    const { result } = renderHookWithClient(() => useCreatePool());
    await act(async () => {
      await result.current.mutateAsync({
        metadata: { name: 'fast' },
        spec: { tier: 'hot' },
      });
    });
    expect(fetchMock.calls[0]?.init?.method).toBe('POST');
  });

  it('useDeletePool issues DELETE', async () => {
    fetchMock.enqueue({ status: 200, body: {} });
    const { result } = renderHookWithClient(() => useDeletePool());
    await act(async () => {
      await result.current.mutateAsync('fast');
    });
    expect(fetchMock.calls[0]?.init?.method).toBe('DELETE');
    expect(fetchMock.calls[0]?.url).toMatch(/\/pools\/fast$/);
  });
});
