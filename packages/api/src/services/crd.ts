import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { z } from 'zod';

/**
 * Generic wrapper around `CustomObjectsApi` that performs Zod-validated
 * list/get/create/patch/delete operations on a single CRD kind.
 *
 * All errors from the Kubernetes API are mapped onto typed errors:
 *  - 404 → {@link CrdNotFoundError}
 *  - 409 → {@link CrdConflictError}
 *  - 422 → {@link CrdInvalidError}
 *  - anything else → {@link CrdApiError}
 */

export interface Gvr {
  group: string;
  version: string;
  plural: string;
}

export interface ListOptions {
  namespace?: string;
  labelSelector?: string;
  fieldSelector?: string;
  limit?: number;
}

export interface ListResult<T> {
  items: T[];
  resourceVersion?: string;
  continueToken?: string;
}

export class CrdApiError extends Error {
  public readonly statusCode: number;
  public override readonly cause?: unknown;
  constructor(statusCode: number, message: string, cause?: unknown) {
    super(message);
    this.statusCode = statusCode;
    this.cause = cause;
    this.name = 'CrdApiError';
  }
}
export class CrdNotFoundError extends CrdApiError {
  constructor(message = 'not found', cause?: unknown) {
    super(404, message, cause);
    this.name = 'CrdNotFoundError';
  }
}
export class CrdConflictError extends CrdApiError {
  constructor(message = 'conflict', cause?: unknown) {
    super(409, message, cause);
    this.name = 'CrdConflictError';
  }
}
export class CrdInvalidError extends CrdApiError {
  constructor(message = 'invalid', cause?: unknown) {
    super(422, message, cause);
    this.name = 'CrdInvalidError';
  }
}

function mapKubeError(err: unknown): CrdApiError {
  const maybe = err as { statusCode?: number; body?: { message?: string }; message?: string };
  const status = maybe?.statusCode ?? 0;
  const msg = maybe?.body?.message ?? maybe?.message ?? 'kube api error';
  if (status === 404) return new CrdNotFoundError(msg, err);
  if (status === 409) return new CrdConflictError(msg, err);
  if (status === 422) return new CrdInvalidError(msg, err);
  return new CrdApiError(status || 500, msg, err);
}

export interface CrdResourceOptions<T> {
  api: CustomObjectsApi;
  gvr: Gvr;
  /** Zod schema for single-object responses. */
  schema: z.ZodType<T>;
  /** Whether this CRD is namespace-scoped. */
  namespaced: boolean;
  /** Default namespace for namespaced CRDs (used when caller omits one). */
  defaultNamespace?: string;
}

export class CrdResource<T> {
  constructor(private readonly opts: CrdResourceOptions<T>) {}

  get namespaced(): boolean {
    return this.opts.namespaced;
  }

  private ns(namespace?: string): string {
    const n = namespace ?? this.opts.defaultNamespace;
    if (!n) throw new CrdInvalidError('namespace required');
    return n;
  }

  async list(opts: ListOptions = {}): Promise<ListResult<T>> {
    const { group, version, plural } = this.opts.gvr;
    try {
      const res = this.opts.namespaced
        ? await this.opts.api.listNamespacedCustomObject(
            group,
            version,
            this.ns(opts.namespace),
            plural,
            undefined,
            undefined,
            undefined,
            opts.fieldSelector,
            opts.labelSelector,
            opts.limit
          )
        : await this.opts.api.listClusterCustomObject(
            group,
            version,
            plural,
            undefined,
            undefined,
            undefined,
            opts.fieldSelector,
            opts.labelSelector,
            opts.limit
          );
      const body = (res as { body: unknown }).body as {
        items?: unknown[];
        metadata?: { resourceVersion?: string; continue?: string };
      };
      const rawItems = Array.isArray(body?.items) ? body.items : [];
      const items: T[] = [];
      for (const raw of rawItems) {
        const parsed = this.opts.schema.safeParse(raw);
        if (parsed.success) items.push(parsed.data);
        // silently drop malformed items; operators own cleanup
      }
      return {
        items,
        resourceVersion: body?.metadata?.resourceVersion,
        continueToken: body?.metadata?.continue,
      };
    } catch (err) {
      throw mapKubeError(err);
    }
  }

  async get(name: string, namespace?: string): Promise<T> {
    const { group, version, plural } = this.opts.gvr;
    try {
      const res = this.opts.namespaced
        ? await this.opts.api.getNamespacedCustomObject(
            group,
            version,
            this.ns(namespace),
            plural,
            name
          )
        : await this.opts.api.getClusterCustomObject(group, version, plural, name);
      return this.parse((res as { body: unknown }).body);
    } catch (err) {
      throw mapKubeError(err);
    }
  }

  async create(body: T, namespace?: string): Promise<T> {
    const { group, version, plural } = this.opts.gvr;
    try {
      const res = this.opts.namespaced
        ? await this.opts.api.createNamespacedCustomObject(
            group,
            version,
            this.ns(namespace),
            plural,
            body as object
          )
        : await this.opts.api.createClusterCustomObject(group, version, plural, body as object);
      return this.parse((res as { body: unknown }).body);
    } catch (err) {
      throw mapKubeError(err);
    }
  }

  async patch(name: string, patch: object, namespace?: string): Promise<T> {
    const { group, version, plural } = this.opts.gvr;
    const options = { headers: { 'Content-Type': 'application/merge-patch+json' } };
    try {
      const res = this.opts.namespaced
        ? await this.opts.api.patchNamespacedCustomObject(
            group,
            version,
            this.ns(namespace),
            plural,
            name,
            patch,
            undefined,
            undefined,
            undefined,
            options
          )
        : await this.opts.api.patchClusterCustomObject(
            group,
            version,
            plural,
            name,
            patch,
            undefined,
            undefined,
            undefined,
            options
          );
      return this.parse((res as { body: unknown }).body);
    } catch (err) {
      throw mapKubeError(err);
    }
  }

  async delete(name: string, namespace?: string): Promise<void> {
    const { group, version, plural } = this.opts.gvr;
    try {
      if (this.opts.namespaced) {
        await this.opts.api.deleteNamespacedCustomObject(
          group,
          version,
          this.ns(namespace),
          plural,
          name
        );
      } else {
        await this.opts.api.deleteClusterCustomObject(group, version, plural, name);
      }
    } catch (err) {
      throw mapKubeError(err);
    }
  }

  private parse(raw: unknown): T {
    const parsed = this.opts.schema.safeParse(raw);
    if (!parsed.success) {
      throw new CrdInvalidError(`response failed validation: ${parsed.error.message}`);
    }
    return parsed.data;
  }
}
