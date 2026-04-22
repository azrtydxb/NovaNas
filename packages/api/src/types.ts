import type { FastifyBaseLogger } from 'fastify';
import type { Redis } from 'ioredis';
import type { Env } from './env.js';

/**
 * Shared user identity resolved from the session / JWT claims.
 * See docs/10-identity-and-secrets.md for the canonical claim set.
 */
export interface AuthenticatedUser {
  sub: string;
  username: string;
  email?: string;
  name?: string;
  groups: string[];
  roles: string[];
  /** Tenant scope — always 'default' in single-node NovaNas (see docs/04). */
  tenant: string;
  /** Raw ID token claims for downstream RBAC resolution. */
  claims: Record<string, unknown>;
}

export interface SessionRecord {
  userId: string;
  username: string;
  createdAt: number;
  expiresAt: number;
  idToken: string;
  accessToken: string;
  refreshToken?: string;
  claims: Record<string, unknown>;
}

export interface AppDependencies {
  env: Env;
  logger: FastifyBaseLogger;
  redis: Redis;
  // Placeholder: real Drizzle client once @novanas/db lands
  db: unknown;
  kube: unknown;
  keycloak: unknown;
  prom: unknown;
}

export interface ErrorBody {
  error: string;
  message?: string;
  code?: string;
  details?: unknown;
}

export interface NotImplementedBody {
  error: 'not implemented';
  wave: number;
}

declare module 'fastify' {
  interface FastifyRequest {
    user?: AuthenticatedUser;
    sessionId?: string;
  }
  interface FastifyInstance {
    deps: AppDependencies;
  }
}
