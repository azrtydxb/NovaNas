import { act, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { useCreateSnapshot, useSnapshots } from './snapshots';
import { installMockFetch, renderHookWithClient } from './test-utils';

describe('snapshots api hooks', () => {
  let fetchMock: ReturnType<typeof installMockFetch>;
  beforeEach(() => {
    fetchMock = installMockFetch();
  });
  afterEach(() => fetchMock.restore());

  it('useSnapshots passes source query params when filtered', async () => {
    fetchMock.enqueue({ status: 200, body: { items: [] } });
    const { result } = renderHookWithClient(() =>
      useSnapshots({ kind: 'Dataset', name: 'photos' })
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetchMock.calls[0]?.url).toMatch(/sourceKind=Dataset/);
    expect(fetchMock.calls[0]?.url).toMatch(/sourceName=photos/);
  });

  it('useCreateSnapshot posts body', async () => {
    fetchMock.enqueue({
      status: 200,
      body: {
        apiVersion: 'novanas.io/v1alpha1',
        kind: 'Snapshot',
        metadata: { name: 'snap1' },
        spec: { source: { kind: 'Dataset', name: 'photos' } },
      },
    });
    const { result } = renderHookWithClient(() => useCreateSnapshot());
    await act(async () => {
      await result.current.mutateAsync({
        metadata: { name: 'snap1' },
        spec: { source: { kind: 'Dataset', name: 'photos' } },
      });
    });
    expect(fetchMock.calls[0]?.init?.method).toBe('POST');
  });
});
