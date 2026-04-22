import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyReply, FastifyRequest } from 'fastify';
import type { Gvr } from './crd.js';
import type { JobsService } from './jobs.js';

/**
 * E1-API-Actions: common helpers for one-shot resource actions.
 *
 * Most actions are implemented via one of four patterns:
 *   1. Spec-field patch (e.g. VM powerState)
 *   2. Annotation trigger (e.g. certificate renew, snapshot restore)
 *   3. Child resource creation (e.g. snapshot restore -> new BlockVolume)
 *   4. Direct job (e.g. system support bundle)
 *
 * The helpers below centralise the mechanical parts so each route handler
 * stays short.
 */

export interface ActionResponse {
  accepted: boolean;
  jobId?: string;
  status: 'pending' | 'running' | 'succeeded' | 'failed';
  message?: string;
  warnings?: string[];
}

const MERGE_PATCH_OPTS = { headers: { 'Content-Type': 'application/merge-patch+json' } };

/** Apply a merge-patch to a cluster- or namespace-scoped CRD. */
export async function patchSpec(
  api: CustomObjectsApi,
  gvr: Gvr,
  name: string,
  patch: Record<string, unknown>,
  namespace?: string
): Promise<void> {
  const { group, version, plural } = gvr;
  if (namespace) {
    await api.patchNamespacedCustomObject(
      group,
      version,
      namespace,
      plural,
      name,
      patch,
      undefined,
      undefined,
      undefined,
      MERGE_PATCH_OPTS
    );
  } else {
    await api.patchClusterCustomObject(
      group,
      version,
      plural,
      name,
      patch,
      undefined,
      undefined,
      undefined,
      MERGE_PATCH_OPTS
    );
  }
}

/**
 * Set an annotation key on a CRD. Operators watch for the annotation +
 * timestamp change to trigger one-shot work (renew, restore, run-now).
 */
export async function setAnnotation(
  api: CustomObjectsApi,
  gvr: Gvr,
  name: string,
  key: string,
  value: string,
  namespace?: string
): Promise<void> {
  await patchSpec(api, gvr, name, { metadata: { annotations: { [key]: value } } }, namespace);
}

/**
 * Check whether the target CRD exists. Returns `true` if found, `false` if
 * the API responded 404, re-throws on anything else.
 */
export async function existsCrd(
  api: CustomObjectsApi,
  gvr: Gvr,
  name: string,
  namespace?: string
): Promise<boolean> {
  const { group, version, plural } = gvr;
  try {
    if (namespace) {
      await api.getNamespacedCustomObject(group, version, namespace, plural, name);
    } else {
      await api.getClusterCustomObject(group, version, plural, name);
    }
    return true;
  } catch (err) {
    const status = (err as { statusCode?: number }).statusCode ?? 0;
    if (status === 404) return false;
    throw err;
  }
}

/** Create a new Job record via the JobsService. */
export async function triggerJob(
  jobs: JobsService,
  kind: string,
  params: Record<string, unknown>,
  ownerId?: string | null
): Promise<string> {
  const row = await jobs.create({ kind, params, ownerId: ownerId ?? null });
  return row.id;
}

/**
 * Destructive-op guard. Returns `true` when the caller has confirmed the
 * destructive op (via `?confirm=true` query param OR
 * `X-Confirm-Destructive: <resource-name>` header). Returns `false` otherwise
 * and writes a 400 to the reply.
 */
export function requireDestructiveConfirm(
  req: FastifyRequest,
  reply: FastifyReply,
  resourceName: string
): boolean {
  const query = (req.query ?? {}) as Record<string, string | undefined>;
  if (query.confirm === 'true') return true;
  const header = req.headers['x-confirm-destructive'];
  const headerValue = Array.isArray(header) ? header[0] : header;
  if (headerValue && headerValue === resourceName) return true;
  reply.code(400).send({
    error: 'confirm_required',
    message:
      'destructive op requires ?confirm=true or X-Confirm-Destructive: <resource-name> header',
  });
  return false;
}

/** Map a kube error status code to a Fastify reply. */
export function kubeErrorReply(reply: FastifyReply, err: unknown): FastifyReply {
  const status = (err as { statusCode?: number }).statusCode ?? 500;
  const msg = (err as { message?: string })?.message ?? 'internal error';
  if (status === 404) return reply.code(404).send({ error: 'not_found', message: msg });
  if (status === 409) return reply.code(409).send({ error: 'conflict', message: msg });
  if (status === 422) return reply.code(422).send({ error: 'invalid', message: msg });
  return reply.code(status || 500).send({ error: 'internal_error', message: msg });
}

/** Utility: generate an ISO timestamp for annotation values. */
export function nowIso(): string {
  return new Date().toISOString();
}

/** Standard action response shape. */
export function accepted(opts: Partial<ActionResponse> = {}): ActionResponse {
  return {
    accepted: true,
    status: opts.status ?? 'running',
    ...opts,
  };
}
