import { act, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { useBuckets, useCreateBucket, useDeleteBucket } from './buckets';
import { installMockFetch, renderHookWithClient } from './test-utils';

describe('buckets api hooks', () => {
  let fetchMock: ReturnType<typeof installMockFetch>;
  beforeEach(() => {
    fetchMock = installMockFetch();
  });
  afterEach(() => fetchMock.restore());

  it('useBuckets returns a list', async () => {
    fetchMock.enqueue({
      status: 200,
      body: {
        items: [
          {
            apiVersion: 'novanas.io/v1alpha1',
            kind: 'Bucket',
            metadata: { name: 'media' },
            spec: { store: 's3-main' },
          },
        ],
      },
    });
    const { result } = renderHookWithClient(() => useBuckets());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.[0]?.metadata.name).toBe('media');
  });

  it('useCreateBucket POSTs body', async () => {
    fetchMock.enqueue({
      status: 200,
      body: {
        apiVersion: 'novanas.io/v1alpha1',
        kind: 'Bucket',
        metadata: { name: 'media' },
        spec: { store: 's3-main' },
      },
    });
    const { result } = renderHookWithClient(() => useCreateBucket());
    await act(async () => {
      await result.current.mutateAsync({
        metadata: { name: 'media' },
        spec: { store: 's3-main' },
      });
    });
    expect(fetchMock.calls[0]?.init?.method).toBe('POST');
  });

  it('useDeleteBucket issues DELETE', async () => {
    fetchMock.enqueue({ status: 200, body: {} });
    const { result } = renderHookWithClient(() => useDeleteBucket());
    await act(async () => {
      await result.current.mutateAsync('media');
    });
    expect(fetchMock.calls[0]?.init?.method).toBe('DELETE');
    expect(fetchMock.calls[0]?.url).toMatch(/\/buckets\/media$/);
  });
});
