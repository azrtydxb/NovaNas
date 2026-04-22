import { act, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import {
  useAppInstanceAction,
  useAppInstances,
  useCreateAppInstance,
  useDeleteAppInstance,
} from './app-instances';
import { installMockFetch, renderHookWithClient } from './test-utils';

describe('app-instances api hooks', () => {
  let fetchMock: ReturnType<typeof installMockFetch>;
  beforeEach(() => {
    fetchMock = installMockFetch();
  });
  afterEach(() => fetchMock.restore());

  it('useAppInstances GETs /apps', async () => {
    fetchMock.enqueue({ status: 200, body: { items: [] } });
    const { result } = renderHookWithClient(() => useAppInstances());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetchMock.calls[0]?.url).toMatch(/\/apps$/);
  });

  it('useCreateAppInstance POSTs', async () => {
    fetchMock.enqueue({ status: 200, body: {} });
    const { result } = renderHookWithClient(() => useCreateAppInstance());
    await act(async () => {
      await result.current.mutateAsync({
        metadata: { name: 'jellyfin-1' },
        spec: { app: 'jellyfin', version: '1.0.0' },
      });
    });
    expect(fetchMock.calls[0]?.init?.method).toBe('POST');
  });

  it('useDeleteAppInstance passes deleteData flag', async () => {
    fetchMock.enqueue({ status: 200, body: {} });
    const { result } = renderHookWithClient(() => useDeleteAppInstance());
    await act(async () => {
      await result.current.mutateAsync({ name: 'jf', deleteData: true });
    });
    expect(fetchMock.calls[0]?.init?.method).toBe('DELETE');
    expect(fetchMock.calls[0]?.url).toMatch(/deleteData=true/);
  });

  it('useAppInstanceAction posts to action subpath', async () => {
    fetchMock.enqueue({ status: 200, body: {} });
    const { result } = renderHookWithClient(() => useAppInstanceAction('jf'));
    await act(async () => {
      await result.current.mutateAsync('stop');
    });
    expect(fetchMock.calls[0]?.init?.method).toBe('POST');
    expect(fetchMock.calls[0]?.url).toMatch(/\/apps\/jf\/stop$/);
  });
});
