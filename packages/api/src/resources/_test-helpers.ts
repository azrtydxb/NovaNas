import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { Redis } from 'ioredis';
import { type BuiltApp, buildApp } from '../app.js';
import type { Env } from '../env.js';
import type { KeycloakClient } from '../services/keycloak.js';
import type { AuthenticatedUser } from '../types.js';

/** RFC 7396 JSON merge-patch — good enough for our fake. */
function mergePatch(
  target: Record<string, unknown>,
  patch: Record<string, unknown>
): Record<string, unknown> {
  const out: Record<string, unknown> = { ...target };
  for (const [k, v] of Object.entries(patch)) {
    if (v === null) {
      delete out[k];
    } else if (
      typeof v === 'object' &&
      !Array.isArray(v) &&
      typeof out[k] === 'object' &&
      out[k] !== null &&
      !Array.isArray(out[k])
    ) {
      out[k] = mergePatch(out[k] as Record<string, unknown>, v as Record<string, unknown>);
    } else {
      out[k] = v;
    }
  }
  return out;
}

/** An in-memory fake of the subset of `CustomObjectsApi` the API server uses. */
export class FakeCustomObjectsApi {
  private cluster = new Map<string, Map<string, Record<string, unknown>>>();
  private nsStore = new Map<string, Map<string, Map<string, Record<string, unknown>>>>();

  private clusterBucket(plural: string): Map<string, Record<string, unknown>> {
    let b = this.cluster.get(plural);
    if (!b) {
      b = new Map();
      this.cluster.set(plural, b);
    }
    return b;
  }
  private nsBucket(ns: string, plural: string): Map<string, Record<string, unknown>> {
    let nsMap = this.nsStore.get(ns);
    if (!nsMap) {
      nsMap = new Map();
      this.nsStore.set(ns, nsMap);
    }
    let b = nsMap.get(plural);
    if (!b) {
      b = new Map();
      nsMap.set(plural, b);
    }
    return b;
  }

  seed(plural: string, obj: Record<string, unknown>, namespace?: string): void {
    const name = (obj.metadata as { name?: string } | undefined)?.name ?? '';
    const bucket = namespace ? this.nsBucket(namespace, plural) : this.clusterBucket(plural);
    bucket.set(name, obj);
  }

  // cluster-scoped
  async listClusterCustomObject(
    _g: string,
    _v: string,
    plural: string
  ): Promise<{ response: unknown; body: unknown }> {
    return {
      response: {},
      body: { items: Array.from(this.clusterBucket(plural).values()) },
    };
  }
  async getClusterCustomObject(
    _g: string,
    _v: string,
    plural: string,
    name: string
  ): Promise<{ response: unknown; body: unknown }> {
    const obj = this.clusterBucket(plural).get(name);
    if (!obj) throw Object.assign(new Error('not found'), { statusCode: 404 });
    return { response: {}, body: obj };
  }
  async createClusterCustomObject(
    _g: string,
    _v: string,
    plural: string,
    body: object
  ): Promise<{ response: unknown; body: unknown }> {
    const obj = body as Record<string, unknown>;
    const name = (obj.metadata as { name?: string } | undefined)?.name ?? '';
    const bucket = this.clusterBucket(plural);
    if (bucket.has(name)) throw Object.assign(new Error('conflict'), { statusCode: 409 });
    bucket.set(name, obj);
    return { response: {}, body: obj };
  }
  async patchClusterCustomObject(
    _g: string,
    _v: string,
    plural: string,
    name: string,
    patch: object
  ): Promise<{ response: unknown; body: unknown }> {
    const bucket = this.clusterBucket(plural);
    const cur = bucket.get(name);
    if (!cur) throw Object.assign(new Error('not found'), { statusCode: 404 });
    const merged = mergePatch(cur, patch as Record<string, unknown>);
    bucket.set(name, merged);
    return { response: {}, body: merged };
  }
  async deleteClusterCustomObject(
    _g: string,
    _v: string,
    plural: string,
    name: string
  ): Promise<{ response: unknown; body: unknown }> {
    const bucket = this.clusterBucket(plural);
    if (!bucket.has(name)) throw Object.assign(new Error('not found'), { statusCode: 404 });
    bucket.delete(name);
    return { response: {}, body: {} };
  }

  // namespaced
  async listNamespacedCustomObject(
    _g: string,
    _v: string,
    ns: string,
    plural: string
  ): Promise<{ response: unknown; body: unknown }> {
    return {
      response: {},
      body: { items: Array.from(this.nsBucket(ns, plural).values()) },
    };
  }
  async getNamespacedCustomObject(
    _g: string,
    _v: string,
    ns: string,
    plural: string,
    name: string
  ): Promise<{ response: unknown; body: unknown }> {
    const obj = this.nsBucket(ns, plural).get(name);
    if (!obj) throw Object.assign(new Error('not found'), { statusCode: 404 });
    return { response: {}, body: obj };
  }
  async createNamespacedCustomObject(
    _g: string,
    _v: string,
    ns: string,
    plural: string,
    body: object
  ): Promise<{ response: unknown; body: unknown }> {
    const obj = body as Record<string, unknown>;
    const name = (obj.metadata as { name?: string } | undefined)?.name ?? '';
    const bucket = this.nsBucket(ns, plural);
    if (bucket.has(name)) throw Object.assign(new Error('conflict'), { statusCode: 409 });
    bucket.set(name, obj);
    return { response: {}, body: obj };
  }
  async patchNamespacedCustomObject(
    _g: string,
    _v: string,
    ns: string,
    plural: string,
    name: string,
    patch: object
  ): Promise<{ response: unknown; body: unknown }> {
    const bucket = this.nsBucket(ns, plural);
    const cur = bucket.get(name);
    if (!cur) throw Object.assign(new Error('not found'), { statusCode: 404 });
    const merged = mergePatch(cur, patch as Record<string, unknown>);
    bucket.set(name, merged);
    return { response: {}, body: merged };
  }
  async deleteNamespacedCustomObject(
    _g: string,
    _v: string,
    ns: string,
    plural: string,
    name: string
  ): Promise<{ response: unknown; body: unknown }> {
    const bucket = this.nsBucket(ns, plural);
    if (!bucket.has(name)) throw Object.assign(new Error('not found'), { statusCode: 404 });
    bucket.delete(name);
    return { response: {}, body: {} };
  }
}

export function fakeRedis(): Redis {
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

export function fakeKeycloak(): KeycloakClient {
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

export const testEnv: Env = Object.freeze({
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

export interface TestAppHandle {
  built: BuiltApp;
  kube: FakeCustomObjectsApi;
  /** Inject a user into every subsequent request by stashing their id under a test cookie. */
  authAs(user: Partial<AuthenticatedUser> & { username: string; roles: string[] }): Promise<string>;
}

/**
 * Build a Fastify app wired to a FakeCustomObjectsApi and return helpers for
 * creating authenticated test sessions.
 */
export async function buildTestApp(): Promise<TestAppHandle> {
  const kube = new FakeCustomObjectsApi();
  const redis = fakeRedis();
  const built = await buildApp({
    env: testEnv,
    logger: { level: 'silent' } as never,
    redis,
    keycloak: fakeKeycloak(),
    kubeCustom: kube as unknown as CustomObjectsApi,
    disableSwagger: true,
    disablePubSub: true,
  });

  async function authAs(user: Partial<AuthenticatedUser> & { username: string; roles: string[] }) {
    const claims: Record<string, unknown> = {
      sub: user.sub ?? user.username,
      preferred_username: user.username,
      email: user.email,
      name: user.name,
      realm_access: { roles: user.roles },
      groups: user.groups ?? [],
    };
    const sid = await built.sessions.create({
      userId: user.sub ?? user.username,
      username: user.username,
      createdAt: Date.now(),
      expiresAt: Date.now() + 3600_000,
      idToken: 'test',
      accessToken: 'test',
      claims,
    });
    return sid;
  }

  return { built, kube, authAs };
}

/** Build a cookie header for the given session id, using the fastify signer. */
export function cookieFor(built: BuiltApp, sid: string): string {
  const signed = built.app.signCookie(sid);
  return `${testEnv.SESSION_COOKIE_NAME}=${encodeURIComponent(signed)}`;
}
