import { act, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { useCertificates, useCreateCertificate, useRenewCertificate } from './certificates';
import { installMockFetch, renderHookWithClient } from './test-utils';

describe('certificates api hooks', () => {
  let fetchMock: ReturnType<typeof installMockFetch>;
  beforeEach(() => {
    fetchMock = installMockFetch();
  });
  afterEach(() => fetchMock.restore());

  it('useCertificates returns a list', async () => {
    fetchMock.enqueue({
      status: 200,
      body: {
        items: [
          {
            apiVersion: 'novanas.io/v1alpha1',
            kind: 'Certificate',
            metadata: { name: 'web' },
            spec: { provider: 'acme', commonName: 'example.com' },
          },
        ],
      },
    });
    const { result } = renderHookWithClient(() => useCertificates());
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.[0]?.metadata.name).toBe('web');
  });

  it('useCreateCertificate POSTs body', async () => {
    fetchMock.enqueue({
      status: 200,
      body: {
        apiVersion: 'novanas.io/v1alpha1',
        kind: 'Certificate',
        metadata: { name: 'web' },
        spec: { provider: 'acme', commonName: 'example.com' },
      },
    });
    const { result } = renderHookWithClient(() => useCreateCertificate());
    await act(async () => {
      await result.current.mutateAsync({
        metadata: { name: 'web' },
        spec: { provider: 'acme', commonName: 'example.com' },
      });
    });
    expect(fetchMock.calls[0]?.init?.method).toBe('POST');
  });

  it('useRenewCertificate POSTs to /renew', async () => {
    fetchMock.enqueue({ status: 200, body: {} });
    const { result } = renderHookWithClient(() => useRenewCertificate());
    await act(async () => {
      await result.current.mutateAsync('web');
    });
    expect(fetchMock.calls[0]?.init?.method).toBe('POST');
    expect(fetchMock.calls[0]?.url).toMatch(/\/certificates\/web\/renew$/);
  });
});
