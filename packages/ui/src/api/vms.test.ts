import { act, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { installMockFetch, renderHookWithClient } from './test-utils';
import { useCreateVm, useDeleteVm, useVmAction, useVms } from './vms';

describe('vms api hooks', () => {
  let fetchMock: ReturnType<typeof installMockFetch>;
  beforeEach(() => {
    fetchMock = installMockFetch();
  });
  afterEach(() => fetchMock.restore());

  it('useVms returns a list', async () => {
    fetchMock.enqueue({
      status: 200,
      body: {
        items: [
          {
            apiVersion: 'novanas.io/v1alpha1',
            kind: 'Vm',
            metadata: { name: 'vm01' },
            spec: { os: { type: 'linux' }, resources: { cpu: 2, memoryMiB: 2048 } },
          },
        ],
      },
    });
    const { result } = renderHookWithClient(() => useVms());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.[0]?.metadata.name).toBe('vm01');
  });

  it('useCreateVm POSTs', async () => {
    fetchMock.enqueue({ status: 200, body: {} });
    const { result } = renderHookWithClient(() => useCreateVm());
    await act(async () => {
      await result.current.mutateAsync({
        metadata: { name: 'vm01' },
        spec: { os: { type: 'linux' }, resources: { cpu: 2, memoryMiB: 2048 } },
      });
    });
    expect(fetchMock.calls[0]?.init?.method).toBe('POST');
  });

  it('useDeleteVm passes deleteDisks flag', async () => {
    fetchMock.enqueue({ status: 200, body: {} });
    const { result } = renderHookWithClient(() => useDeleteVm());
    await act(async () => {
      await result.current.mutateAsync({ name: 'vm01', deleteDisks: true });
    });
    expect(fetchMock.calls[0]?.url).toMatch(/deleteDisks=true/);
  });

  it('useVmAction hits action endpoint', async () => {
    fetchMock.enqueue({ status: 200, body: {} });
    const { result } = renderHookWithClient(() => useVmAction('vm01'));
    await act(async () => {
      await result.current.mutateAsync('start');
    });
    expect(fetchMock.calls[0]?.url).toMatch(/\/vms\/vm01\/start$/);
  });
});
