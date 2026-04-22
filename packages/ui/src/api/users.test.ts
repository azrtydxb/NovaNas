import { act, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { installMockFetch, renderHookWithClient } from './test-utils';
import { useCreateUser, useDeleteUser, useUsers } from './users';

describe('users api hooks', () => {
  let fetchMock: ReturnType<typeof installMockFetch>;
  beforeEach(() => {
    fetchMock = installMockFetch();
  });
  afterEach(() => fetchMock.restore());

  it('useUsers returns a list', async () => {
    fetchMock.enqueue({
      status: 200,
      body: {
        items: [
          {
            apiVersion: 'novanas.io/v1alpha1',
            kind: 'User',
            metadata: { name: 'alice' },
            spec: { username: 'alice' },
          },
        ],
      },
    });
    const { result } = renderHookWithClient(() => useUsers());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.[0]?.spec.username).toBe('alice');
  });

  it('useCreateUser POSTs body', async () => {
    fetchMock.enqueue({
      status: 200,
      body: {
        apiVersion: 'novanas.io/v1alpha1',
        kind: 'User',
        metadata: { name: 'alice' },
        spec: { username: 'alice' },
      },
    });
    const { result } = renderHookWithClient(() => useCreateUser());
    await act(async () => {
      await result.current.mutateAsync({
        metadata: { name: 'alice' },
        spec: { username: 'alice' },
      });
    });
    expect(fetchMock.calls[0]?.init?.method).toBe('POST');
  });

  it('useDeleteUser issues DELETE', async () => {
    fetchMock.enqueue({ status: 200, body: {} });
    const { result } = renderHookWithClient(() => useDeleteUser());
    await act(async () => {
      await result.current.mutateAsync('alice');
    });
    expect(fetchMock.calls[0]?.init?.method).toBe('DELETE');
    expect(fetchMock.calls[0]?.url).toMatch(/\/users\/alice$/);
  });
});
