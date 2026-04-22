import { act, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { useCreateDataset, useDatasets } from './datasets';
import { installMockFetch, renderHookWithClient } from './test-utils';

describe('datasets api hooks', () => {
  let fetchMock: ReturnType<typeof installMockFetch>;
  beforeEach(() => {
    fetchMock = installMockFetch();
  });
  afterEach(() => fetchMock.restore());

  it('useDatasets returns a bare array', async () => {
    fetchMock.enqueue({
      status: 200,
      body: [
        {
          apiVersion: 'novanas.io/v1alpha1',
          kind: 'Dataset',
          metadata: { name: 'photos' },
          spec: { pool: 'bulk', size: '100Gi', filesystem: 'xfs' },
        },
      ],
    });
    const { result } = renderHookWithClient(() => useDatasets());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.[0]?.metadata.name).toBe('photos');
  });

  it('useCreateDataset POSTs and surfaces 409 errors', async () => {
    fetchMock.enqueue({ status: 409, body: { message: 'already exists' } });
    const { result } = renderHookWithClient(() => useCreateDataset());
    let caught: Error | undefined;
    await act(async () => {
      try {
        await result.current.mutateAsync({
          metadata: { name: 'photos' },
          spec: { pool: 'bulk', size: '100Gi', filesystem: 'xfs' },
        });
      } catch (e) {
        caught = e as Error;
      }
    });
    expect(caught?.message).toMatch(/already exists/);
    expect(fetchMock.calls[0]?.init?.method).toBe('POST');
  });
});
