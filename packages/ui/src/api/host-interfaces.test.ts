import { act, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { useCreateHostInterface, useHostInterfaces } from './host-interfaces';
import { installMockFetch, renderHookWithClient } from './test-utils';

describe('host-interfaces api hooks', () => {
  let fetchMock: ReturnType<typeof installMockFetch>;
  beforeEach(() => {
    fetchMock = installMockFetch();
  });
  afterEach(() => fetchMock.restore());

  it('useHostInterfaces returns a list', async () => {
    fetchMock.enqueue({
      status: 200,
      body: {
        items: [
          {
            apiVersion: 'novanas.io/v1alpha1',
            kind: 'HostInterface',
            metadata: { name: 'mgmt0' },
            spec: { backing: 'eth0', usage: ['management'] },
          },
        ],
      },
    });
    const { result } = renderHookWithClient(() => useHostInterfaces());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.[0]?.metadata.name).toBe('mgmt0');
  });

  it('useCreateHostInterface POSTs body', async () => {
    fetchMock.enqueue({
      status: 200,
      body: {
        apiVersion: 'novanas.io/v1alpha1',
        kind: 'HostInterface',
        metadata: { name: 'mgmt0' },
        spec: { backing: 'eth0', usage: ['management'] },
      },
    });
    const { result } = renderHookWithClient(() => useCreateHostInterface());
    await act(async () => {
      await result.current.mutateAsync({
        metadata: { name: 'mgmt0' },
        spec: { backing: 'eth0', usage: ['management'] },
      });
    });
    expect(fetchMock.calls[0]?.init?.method).toBe('POST');
  });
});
