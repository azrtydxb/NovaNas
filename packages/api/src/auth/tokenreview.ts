import type { AuthenticationV1Api } from '@kubernetes/client-node';
import type { FastifyReply, FastifyRequest } from 'fastify';
import type { AuthenticatedUser } from '../types.js';

/**
 * Service-to-service authentication via Kubernetes TokenReview.
 *
 * NovaNas in-cluster components (disk-agent, storage-meta, storage-
 * agent, etc.) authenticate to the API by presenting their pod's
 * ServiceAccount projected JWT in the `Authorization: Bearer …`
 * header. We forward the token to the kube-apiserver's TokenReview
 * endpoint, which validates it and returns the SA identity.
 *
 * The mapping `system:serviceaccount:<ns>:<sa>` → internal principal
 * lives in `serviceAccountRoleMap`. Each SA gets its own minimal
 * NovaNas role:
 *
 *   novanas-disk-agent     → internal:disk-agent       (writes Disk/*)
 *   novanas-storage-meta   → internal:storage-meta     (reads pools/disks)
 *   novanas-storage-agent  → internal:storage-agent    (reads pools/disks)
 *   novanas-operators      → internal:operator         (reads stay-set)
 *
 * Anything outside the map is rejected. No ambient SA access.
 */

export interface TokenReviewMiddleware {
  /** Fastify preHandler that authenticates Bearer-token requests. */
  authenticate: (req: FastifyRequest, reply: FastifyReply) => Promise<void>;
}

export interface TokenReviewOptions {
  api: AuthenticationV1Api;
  /**
   * Map of `system:serviceaccount:<ns>:<sa>` username (as TokenReview
   * returns it) to an internal NovaNas principal. SAs not in the map
   * are rejected with 403.
   */
  serviceAccountRoleMap?: Record<string, ServicePrincipal>;
}

export interface ServicePrincipal {
  /** Logical principal name surfaced in audit. */
  name: string;
  /** Roles granted to this SA. The authz layer reads `roles`. */
  roles: string[];
  /** Groups for hasGroup() checks (mirrors keycloak `groups` claim). */
  groups: string[];
}

const DEFAULT_SA_MAP: Record<string, ServicePrincipal> = {
  'system:serviceaccount:novanas-system:novanas-disk-agent': {
    name: 'novanas-disk-agent',
    roles: ['internal:disk-agent'],
    groups: [],
  },
  'system:serviceaccount:novanas-system:novanas-storage': {
    name: 'novanas-storage',
    roles: ['internal:storage'],
    groups: [],
  },
  'system:serviceaccount:novanas-system:novanas-storage-meta': {
    name: 'novanas-storage-meta',
    roles: ['internal:storage'],
    groups: [],
  },
  'system:serviceaccount:novanas-system:novanas-storage-agent': {
    name: 'novanas-storage-agent',
    roles: ['internal:storage'],
    groups: [],
  },
  'system:serviceaccount:novanas-system:novanas-operators': {
    name: 'novanas-operators',
    roles: ['internal:operator'],
    groups: [],
  },
};

export function buildTokenReviewMiddleware(opts: TokenReviewOptions): TokenReviewMiddleware {
  const map = { ...DEFAULT_SA_MAP, ...(opts.serviceAccountRoleMap ?? {}) };

  async function authenticate(req: FastifyRequest, reply: FastifyReply): Promise<void> {
    const authz = req.headers.authorization ?? '';
    if (!authz.toLowerCase().startsWith('bearer ')) {
      reply.code(401).send({ error: 'unauthorized', message: 'bearer token required' });
      return;
    }
    const token = authz.slice(7).trim();
    if (!token) {
      reply.code(401).send({ error: 'unauthorized', message: 'empty bearer token' });
      return;
    }

    let username: string | undefined;
    try {
      const res = await opts.api.createTokenReview({
        apiVersion: 'authentication.k8s.io/v1',
        kind: 'TokenReview',
        metadata: {},
        spec: { token },
      } as Parameters<typeof opts.api.createTokenReview>[0]);
      const body = (res as { body?: unknown; status?: unknown }).body ?? res;
      const status = (body as { status?: { authenticated?: boolean; user?: { username?: string } } })
        .status;
      if (!status?.authenticated || !status.user?.username) {
        reply.code(401).send({ error: 'unauthorized', message: 'token not valid' });
        return;
      }
      username = status.user.username;
    } catch (err) {
      req.log.warn({ err }, 'TokenReview failed');
      reply.code(401).send({ error: 'unauthorized', message: 'token verification failed' });
      return;
    }

    const principal = map[username];
    if (!principal) {
      reply.code(403).send({
        error: 'forbidden',
        message: `service account ${username} not authorised for NovaNas API`,
      });
      return;
    }

    const user: AuthenticatedUser = {
      sub: username,
      username: principal.name,
      groups: principal.groups,
      roles: principal.roles,
      tenant: 'default',
      claims: { service_account: true, kube_username: username },
    };
    (req as FastifyRequest & { user: AuthenticatedUser }).user = user;
  }

  return { authenticate };
}

/** Roles that grant cross-cutting "read all" access for service accounts. */
export const SERVICE_READ_ROLES = new Set([
  'internal:disk-agent',
  'internal:storage',
  'internal:operator',
]);

/** Roles that grant write access to specific kinds for the disk-agent. */
export const DISK_AGENT_WRITE_KINDS = new Set(['Disk']);
