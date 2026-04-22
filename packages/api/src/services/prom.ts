import type { Env } from '../env.js';

/**
 * Prometheus query client. Only a URL placeholder for now — Wave 3 will
 * implement `queryRange`, `queryInstant`, etc.
 */
export interface PromClient {
  readonly url: string | undefined;
  queryInstant(expr: string): Promise<unknown>;
  queryRange(expr: string, start: Date, end: Date, stepSeconds: number): Promise<unknown>;
}

export function createPromClient(env: Env): PromClient {
  return {
    url: env.PROMETHEUS_URL,
    async queryInstant(_expr: string) {
      throw new Error('not implemented');
    },
    async queryRange(_expr, _start, _end, _step) {
      throw new Error('not implemented');
    },
  };
}
