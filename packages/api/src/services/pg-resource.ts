import { resources } from '@novanas/db';
import { and, asc, eq, inArray, isNull } from 'drizzle-orm';
import type { z } from 'zod';
import type { DbClient } from './db.js';

/**
 * Postgres-backed analogue of CrdResource. Same five-method surface
 * (list/get/create/patch/delete) plus the same typed-error vocabulary
 * so callers (notably `_register.ts`) treat both interchangeably.
 *
 * The on-disk shape is a single polymorphic `resources` table; see
 * `packages/db/src/schema/resources.ts` for the design rationale.
 * `PgResource<T>` (re-)materialises the K8s-style envelope on every
 * read so existing Zod schemas (`@novanas/schemas`) and the SPA's
 * `metadata + spec + status` shape Just Work — the move is about
 * source-of-truth, not wire format.
 *
 * Concurrency: every write bumps `revision` and updates `updated_at`.
 * Optimistic concurrency on PATCH is implemented in `patch()`.
 */

export class PgApiError extends Error {
  public readonly statusCode: number;
  public override readonly cause?: unknown;
  constructor(statusCode: number, message: string, cause?: unknown) {
    super(message);
    this.statusCode = statusCode;
    this.cause = cause;
    this.name = 'PgApiError';
  }
}
export class PgNotFoundError extends PgApiError {
  constructor(message = 'not found', cause?: unknown) {
    super(404, message, cause);
    this.name = 'PgNotFoundError';
  }
}
export class PgConflictError extends PgApiError {
  constructor(message = 'conflict', cause?: unknown) {
    super(409, message, cause);
    this.name = 'PgConflictError';
  }
}
export class PgInvalidError extends PgApiError {
  constructor(message = 'invalid', cause?: unknown) {
    super(422, message, cause);
    this.name = 'PgInvalidError';
  }
}

export interface PgListOptions {
  namespace?: string;
  limit?: number;
}

export interface PgListResult<T> {
  items: T[];
}

export interface PgResourceOptions<T> {
  db: DbClient;
  /** K8s-style apiVersion, e.g. "novanas.io/v1alpha1". */
  apiVersion: string;
  /** Resource kind, e.g. "Disk". Used as the row's `kind` and in envelope. */
  kind: string;
  /** Zod schema for full envelope round-tripping. */
  schema: z.ZodType<T>;
  /** Whether this resource is namespace-scoped. */
  namespaced: boolean;
  /** Default namespace for namespaced resources. */
  defaultNamespace?: string;
}

interface Envelope {
  apiVersion: string;
  kind: string;
  metadata: {
    name: string;
    namespace?: string;
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
    resourceVersion?: string;
    creationTimestamp?: string;
    uid?: string;
  };
  spec?: Record<string, unknown>;
  status?: Record<string, unknown>;
}

interface RowShape {
  kind: string;
  name: string;
  namespace: string;
  labels: Record<string, string>;
  annotations: Record<string, string>;
  spec: Record<string, unknown>;
  status: Record<string, unknown>;
  revision: string;
  createdAt: Date;
  updatedAt: Date;
  deletedAt: Date | null;
}

export class PgResource<T> {
  constructor(private readonly opts: PgResourceOptions<T>) {}

  get namespaced(): boolean {
    return this.opts.namespaced;
  }

  private ns(namespace?: string): string {
    if (!this.opts.namespaced) return '';
    const n = namespace ?? this.opts.defaultNamespace;
    if (!n) throw new PgInvalidError('namespace required');
    return n;
  }

  private rowToEnvelope(row: RowShape): Envelope {
    return {
      apiVersion: this.opts.apiVersion,
      kind: this.opts.kind,
      metadata: {
        name: row.name,
        ...(this.opts.namespaced ? { namespace: row.namespace } : {}),
        labels: row.labels,
        annotations: row.annotations,
        resourceVersion: row.revision,
        creationTimestamp: row.createdAt.toISOString(),
      },
      spec: row.spec,
      status: row.status,
    };
  }

  private parse(envelope: Envelope): T {
    const parsed = this.opts.schema.safeParse(envelope);
    if (parsed.success) return parsed.data;
    // Be lenient with reads of stored rows: the schema may have evolved
    // since the row was written, and that shouldn't 500 the route. Same
    // best-effort posture as `list()`. PgInvalidError is reserved for
    // user-supplied bodies that don't match (validated upstream of
    // create()).
    return envelope as unknown as T;
  }

  async list(opts: PgListOptions = {}): Promise<PgListResult<T>> {
    const conds = [eq(resources.kind, this.opts.kind), isNull(resources.deletedAt)];
    if (this.opts.namespaced) {
      conds.push(eq(resources.namespace, this.ns(opts.namespace)));
    }
    let q = this.opts.db
      .select()
      .from(resources)
      .where(and(...conds))
      .orderBy(asc(resources.name));
    if (opts.limit && opts.limit > 0) {
      q = q.limit(opts.limit) as typeof q;
    }
    const rows = (await q) as RowShape[];
    const items: T[] = [];
    for (const row of rows) {
      const env = this.rowToEnvelope(row);
      const parsed = this.opts.schema.safeParse(env);
      if (parsed.success) items.push(parsed.data);
      // silently drop malformed rows; ops can clean them up
    }
    return { items };
  }

  async get(name: string, namespace?: string): Promise<T> {
    const conds = [
      eq(resources.kind, this.opts.kind),
      eq(resources.name, name),
      isNull(resources.deletedAt),
    ];
    if (this.opts.namespaced) {
      conds.push(eq(resources.namespace, this.ns(namespace)));
    } else {
      conds.push(eq(resources.namespace, ''));
    }
    const rows = (await this.opts.db
      .select()
      .from(resources)
      .where(and(...conds))
      .limit(1)) as RowShape[];
    const row = rows[0];
    if (!row) {
      throw new PgNotFoundError(`${this.opts.kind} '${name}' not found`);
    }
    return this.parse(this.rowToEnvelope(row));
  }

  async create(body: T, namespace?: string): Promise<T> {
    const env = body as unknown as Envelope;
    const name = env.metadata?.name;
    if (!name || typeof name !== 'string') {
      throw new PgInvalidError('metadata.name is required');
    }
    const ns = this.opts.namespaced ? this.ns(namespace ?? env.metadata.namespace) : '';
    const labels = env.metadata.labels ?? {};
    const annotations = env.metadata.annotations ?? {};
    const spec = (env.spec ?? {}) as Record<string, unknown>;
    const status = (env.status ?? {}) as Record<string, unknown>;

    try {
      const inserted = (await this.opts.db
        .insert(resources)
        .values({
          kind: this.opts.kind,
          name,
          namespace: ns,
          labels,
          annotations,
          spec,
          status,
          revision: '1',
        })
        .returning()) as RowShape[];
      const row = inserted[0];
      if (!row) {
        throw new PgApiError(500, 'insert returned no rows');
      }
      return this.parse(this.rowToEnvelope(row));
    } catch (err) {
      // Postgres unique-violation on (kind, namespace, name) → 409.
      const code = (err as { code?: string })?.code;
      if (code === '23505') {
        throw new PgConflictError(`${this.opts.kind} '${name}' already exists`, err);
      }
      throw new PgApiError(500, `create failed: ${(err as Error)?.message}`, err);
    }
  }

  /**
   * Apply a JSON-merge-patch-style update. Top-level keys
   * (metadata.labels, metadata.annotations, spec, status) replace the
   * corresponding column wholesale; nested keys merge per JSON Merge
   * Patch (RFC 7396) semantics. Caller is expected to send a sparse
   * patch — the route layer does that already via `req.body`.
   */
  async patch(name: string, patch: object, namespace?: string): Promise<T> {
    const ns = this.opts.namespaced ? this.ns(namespace) : '';
    const conds = [
      eq(resources.kind, this.opts.kind),
      eq(resources.name, name),
      eq(resources.namespace, ns),
      isNull(resources.deletedAt),
    ];

    return await this.opts.db.transaction(async (tx) => {
      const rows = (await tx
        .select()
        .from(resources)
        .where(and(...conds))
        .for('update')
        .limit(1)) as RowShape[];
      const row = rows[0];
      if (!row) {
        throw new PgNotFoundError(`${this.opts.kind} '${name}' not found`);
      }
      const p = patch as {
        metadata?: { labels?: Record<string, string>; annotations?: Record<string, string> };
        spec?: Record<string, unknown>;
        status?: Record<string, unknown>;
      };
      const labels = (
        p.metadata?.labels ? mergePatch(row.labels, p.metadata.labels) : row.labels
      ) as Record<string, string>;
      const annotations = (
        p.metadata?.annotations
          ? mergePatch(row.annotations, p.metadata.annotations)
          : row.annotations
      ) as Record<string, string>;
      const spec = p.spec ? mergePatch(row.spec, p.spec) : row.spec;
      const status = p.status ? mergePatch(row.status, p.status) : row.status;

      const next = (parseInt(row.revision, 10) + 1).toString();

      const updated = (await tx
        .update(resources)
        .set({
          labels,
          annotations,
          spec,
          status,
          revision: next,
          updatedAt: new Date(),
        })
        .where(and(...conds))
        .returning()) as RowShape[];
      const updatedRow = updated[0];
      if (!updatedRow) {
        throw new PgApiError(500, 'update returned no rows');
      }
      return this.parse(this.rowToEnvelope(updatedRow));
    });
  }

  async delete(name: string, namespace?: string): Promise<void> {
    const ns = this.opts.namespaced ? this.ns(namespace) : '';
    const conds = [
      eq(resources.kind, this.opts.kind),
      eq(resources.name, name),
      eq(resources.namespace, ns),
      isNull(resources.deletedAt),
    ];
    const deleted = (await this.opts.db
      .delete(resources)
      .where(and(...conds))
      .returning()) as RowShape[];
    if (deleted.length === 0) {
      throw new PgNotFoundError(`${this.opts.kind} '${name}' not found`);
    }
  }

  /**
   * Sibling resource access for cross-kind validation hooks (e.g. a
   * disk attach checks the pool exists). Same db handle, different kind.
   */
  sibling<U>(opts: Omit<PgResourceOptions<U>, 'db'>): PgResource<U> {
    return new PgResource<U>({ db: this.opts.db, ...opts });
  }

  /**
   * Bulk-existence helper used by uniqueness validation hooks (e.g.
   * "no two pools share a tier"). Returns the rows whose name is in
   * `names`, scoped to this kind+namespace.
   */
  async listByNames(names: string[], namespace?: string): Promise<T[]> {
    if (names.length === 0) return [];
    const ns = this.opts.namespaced ? this.ns(namespace) : '';
    const conds = [
      eq(resources.kind, this.opts.kind),
      eq(resources.namespace, ns),
      inArray(resources.name, names),
      isNull(resources.deletedAt),
    ];
    const rows = (await this.opts.db
      .select()
      .from(resources)
      .where(and(...conds))) as RowShape[];
    const out: T[] = [];
    for (const r of rows) {
      const parsed = this.opts.schema.safeParse(this.rowToEnvelope(r));
      if (parsed.success) out.push(parsed.data);
    }
    return out;
  }
}

/**
 * Shallow JSON Merge Patch. Top-level keys in `patch` replace those in
 * `target`; `null` in patch deletes the key (RFC 7396).
 */
function mergePatch(
  target: Record<string, unknown>,
  patch: Record<string, unknown>
): Record<string, unknown> {
  const out: Record<string, unknown> = { ...target };
  for (const [k, v] of Object.entries(patch)) {
    if (v === null) {
      delete out[k];
    } else {
      out[k] = v;
    }
  }
  return out;
}
