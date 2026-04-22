import type { Redis } from 'ioredis';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { type BuiltApp, buildApp } from './app.js';
import type { Env } from './env.js';
import type { KeycloakClient } from './services/keycloak.js';

// Minimal in-memory Redis stub good enough for /health + /api/version.
function fakeRedis(): Redis {
  const store = new Map<string, string>();
  const api = {
    async get(k: string) {
      return store.get(k) ?? null;
    },
    async set(k: string, v: string) {
      store.set(k, v);
      return 'OK';
    },
    async setex(k: string, _ttl: number, v: string) {
      store.set(k, v);
      return 'OK';
    },
    async del(k: string) {
      return store.delete(k) ? 1 : 0;
    },
    async expire() {
      return 1;
    },
    async publish() {
      return 0;
    },
    async psubscribe() {
      return 1;
    },
    async punsubscribe() {
      return 1;
    },
    on() {
      return api;
    },
    disconnect() {
      /* no-op */
    },
  };
  return api as unknown as Redis;
}

function fakeKeycloak(): KeycloakClient {
  return {
    config: {} as never,
    issuerUrl: 'http://localhost/realms/test',
    clientId: 'test',
    buildAuthUrl() {
      return {
        url: new URL('http://localhost/authorize'),
        state: 's',
        nonce: 'n',
        codeVerifier: 'v',
      };
    },
    async exchangeCode() {
      throw new Error('not used in this test');
    },
    async logout() {
      /* no-op */
    },
  };
}

const testEnv: Env = Object.freeze({
  NODE_ENV: 'test',
  PORT: 0,
  LOG_LEVEL: 'silent' as unknown as Env['LOG_LEVEL'],
  DATABASE_URL: 'postgres://localhost/test',
  REDIS_URL: 'redis://localhost:6379',
  KEYCLOAK_ISSUER_URL: 'http://localhost/realms/test',
  KEYCLOAK_CLIENT_ID: 'test',
  KEYCLOAK_CLIENT_SECRET: 'test-secret',
  SESSION_COOKIE_NAME: 'novanas_session',
  SESSION_SECRET: 'test-secret-0123456789abcdef',
  API_VERSION: '0.0.0-test',
  API_PUBLIC_URL: 'http://localhost:8080',
  KUBECONFIG_PATH: undefined,
  OPENBAO_ADDR: undefined,
  OPENBAO_TOKEN: undefined,
  PROMETHEUS_URL: undefined,
});

describe('NovaNas API app', () => {
  let built: BuiltApp;

  beforeAll(async () => {
    built = await buildApp({
      env: testEnv,
      logger: { level: 'silent' } as never,
      redis: fakeRedis(),
      keycloak: fakeKeycloak(),
      disableSwagger: true,
      disablePubSub: true,
    });
  });

  afterAll(async () => {
    await built.app.close();
  });

  it('GET /health returns 200', async () => {
    const res = await built.app.inject({ method: 'GET', url: '/health' });
    expect(res.statusCode).toBe(200);
    const body = res.json() as { status: string; uptime: number };
    expect(body.status).toBe('ok');
    expect(typeof body.uptime).toBe('number');
  });

  it('GET /api/version returns the configured version', async () => {
    const res = await built.app.inject({ method: 'GET', url: '/api/version' });
    expect(res.statusCode).toBe(200);
    const body = res.json() as { version: string; apiVersion: string };
    expect(body.version).toBe('0.0.0-test');
    expect(body.apiVersion).toBe('v1');
  });

  it('GET /api/v1/pools without session returns 401', async () => {
    const res = await built.app.inject({ method: 'GET', url: '/api/v1/pools' });
    expect(res.statusCode).toBe(401);
  });
});
