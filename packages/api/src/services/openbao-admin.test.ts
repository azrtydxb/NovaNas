import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Env } from '../env.js';
import { createOpenBaoAdmin } from './openbao-admin.js';

const env: Env = {
  NODE_ENV: 'test',
  PORT: 8080,
  LOG_LEVEL: 'info',
  DATABASE_URL: 'postgres://x:y@localhost/z',
  REDIS_URL: 'memory://',
  KEYCLOAK_ISSUER_URL: 'https://kc.example.com/realms/novanas',
  KEYCLOAK_CLIENT_ID: 'novanas-api',
  KEYCLOAK_CLIENT_SECRET: 'secret',
  SESSION_COOKIE_NAME: 'novanas_session',
  SESSION_SECRET: 'a-very-long-test-secret',
  API_VERSION: '0.0.0',
  API_PUBLIC_URL: 'https://kc.example.com',
  OPENBAO_ADDR: 'http://bao.example.com:8200',
  OPENBAO_TOKEN: 't0k3n',
};

describe('openbao-admin', () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('returns null when env is incomplete', () => {
    const partial = { ...env, OPENBAO_TOKEN: undefined } as unknown as Env;
    expect(createOpenBaoAdmin(partial)).toBeNull();
  });

  it('ensureTransitKey creates when missing, returns true', async () => {
    fetchMock
      .mockResolvedValueOnce(new Response('not found', { status: 404 })) // exists?
      .mockResolvedValueOnce(new Response(null, { status: 204 })); // create
    const admin = createOpenBaoAdmin(env)!;
    const created = await admin.ensureTransitKey('vol-1');
    expect(created).toBe(true);
    const createCall = fetchMock.mock.calls[1];
    expect(createCall?.[0]).toBe('http://bao.example.com:8200/v1/transit/keys/vol-1');
    const body = JSON.parse((createCall?.[1] as RequestInit).body as string);
    expect(body).toMatchObject({ type: 'aes256-gcm96' });
  });

  it('ensureTransitKey patches config on existing key, returns false', async () => {
    fetchMock
      .mockResolvedValueOnce(new Response(JSON.stringify({ data: {} }), { status: 200 })) // exists
      .mockResolvedValueOnce(new Response(null, { status: 204 })); // /config
    const admin = createOpenBaoAdmin(env)!;
    const created = await admin.ensureTransitKey('vol-1', { autoRotatePeriod: '720h' });
    expect(created).toBe(false);
    const cfgCall = fetchMock.mock.calls[1];
    expect(cfgCall?.[0]).toBe('http://bao.example.com:8200/v1/transit/keys/vol-1/config');
  });

  it('deleteTransitKey toggles deletion_allowed before delete', async () => {
    fetchMock
      .mockResolvedValueOnce(new Response(JSON.stringify({ data: {} }), { status: 200 })) // exists
      .mockResolvedValueOnce(new Response(null, { status: 204 })) // flip
      .mockResolvedValueOnce(new Response(null, { status: 204 })); // delete
    const admin = createOpenBaoAdmin(env)!;
    await admin.deleteTransitKey('vol-1');
    expect(fetchMock).toHaveBeenCalledTimes(3);
    expect(fetchMock.mock.calls[1]?.[0]).toContain('/config');
    expect((fetchMock.mock.calls[2]?.[1] as RequestInit).method).toBe('DELETE');
  });

  it('deleteTransitKey is a no-op when key is missing', async () => {
    fetchMock.mockResolvedValueOnce(new Response('not found', { status: 404 }));
    const admin = createOpenBaoAdmin(env)!;
    await admin.deleteTransitKey('missing');
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});
