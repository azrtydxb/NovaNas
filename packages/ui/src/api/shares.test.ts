import { act, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { useCreateShare, useDeleteShare, useShares } from './shares';
import { installMockFetch, renderHookWithClient } from './test-utils';

describe('shares api hooks', () => {
  let fetchMock: ReturnType<typeof installMockFetch>;
  beforeEach(() => {
    fetchMock = installMockFetch();
  });
  afterEach(() => fetchMock.restore());

  it('useShares returns a list', async () => {
    fetchMock.enqueue({
      status: 200,
      body: {
        items: [
          {
            apiVersion: 'novanas.io/v1alpha1',
            kind: 'Share',
            metadata: { name: 'media' },
            spec: { dataset: 'ds', path: '/', protocols: {} },
          },
        ],
      },
    });
    const { result } = renderHookWithClient(() => useShares());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.length).toBe(1);
  });

  it('useCreateShare POSTs', async () => {
    fetchMock.enqueue({ status: 200, body: {} });
    const { result } = renderHookWithClient(() => useCreateShare());
    await act(async () => {
      await result.current.mutateAsync({
        metadata: { name: 'media' },
        spec: { dataset: 'ds', path: '/', protocols: { smb: { server: 'smb0' } } },
      });
    });
    expect(fetchMock.calls[0]?.init?.method).toBe('POST');
  });

  it('useDeleteShare DELETEs', async () => {
    fetchMock.enqueue({ status: 200, body: {} });
    const { result } = renderHookWithClient(() => useDeleteShare());
    await act(async () => {
      await result.current.mutateAsync('media');
    });
    expect(fetchMock.calls[0]?.init?.method).toBe('DELETE');
  });
});
