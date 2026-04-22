import { waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { useMetric } from './metrics';
import { installMockFetch, renderHookWithClient } from './test-utils';

describe('metrics api hooks', () => {
  let fetchMock: ReturnType<typeof installMockFetch>;
  beforeEach(() => {
    fetchMock = installMockFetch();
  });
  afterEach(() => fetchMock.restore());

  it('useMetric passes scope, query, range', async () => {
    fetchMock.enqueue({
      status: 200,
      body: { scope: 'pool', query: 'throughput', range: '1h', series: [] },
    });
    const { result } = renderHookWithClient(() => useMetric('pool', 'throughput', '1h'));
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    const url = fetchMock.calls[0]?.url ?? '';
    expect(url).toMatch(/scope=pool/);
    expect(url).toMatch(/query=throughput/);
    expect(url).toMatch(/range=1h/);
  });

  it('surfaces 404', async () => {
    fetchMock.enqueue({ status: 404, body: { message: 'not found' } });
    const { result } = renderHookWithClient(() => useMetric('pool', 'x', '5m'));
    await waitFor(() => expect(result.current.isError).toBe(true));
  });
});
