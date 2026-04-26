import { index, jsonb, pgTable, text, timestamp, uniqueIndex, varchar } from 'drizzle-orm/pg-core';

/**
 * NovaNas business-object storage. Each row stores one resource of one
 * `kind` keyed by `(kind, name)` for cluster-scoped resources or
 * `(kind, namespace, name)` for namespaced ones.
 *
 * The schema mirrors the Kubernetes CRD envelope (apiVersion / kind /
 * metadata / spec / status) so the existing `@novanas/schemas` Zod
 * types and the SPA's data shapes work unchanged. The pivot is purely
 * about *where* the source of truth lives — we keep the wire format.
 *
 * Why a single polymorphic table instead of one per kind:
 *   - All NovaNas resources share the same envelope and the same set
 *     of CRUD operations; the only kind-specific logic lives in the
 *     route's `validate` hook and the Zod schema. A polymorphic table
 *     keeps `PgResource` thin and lets new kinds register without a
 *     migration.
 *   - Cross-resource invariants (the validate hooks) span kinds: a
 *     disk attach checks the pool's class. Joining within one table
 *     is straightforward.
 *   - Postgres handles >100M jsonb rows fine; sharding by kind would
 *     be premature.
 *
 * Indexes:
 *   - `(kind, name)` unique for cluster-scoped fast-path GET.
 *   - `(kind, namespace, name)` unique for namespaced GET.
 *   - `kind` for LIST.
 *   - GIN on labels would let us label-select but isn't a 1.0 need.
 */
export const resources = pgTable(
  'resources',
  {
    /** Resource kind (e.g. "Disk", "StoragePool"). */
    kind: varchar('kind', { length: 64 }).notNull(),
    /** Cluster-scoped or namespace-relative name. RFC 1123, max 253. */
    name: varchar('name', { length: 253 }).notNull(),
    /** Empty string for cluster-scoped resources. */
    namespace: varchar('namespace', { length: 253 }).notNull().default(''),
    /** Free-form labels mirroring `metadata.labels`. */
    labels: jsonb('labels').notNull().$type<Record<string, string>>().default({}),
    /** Free-form annotations mirroring `metadata.annotations`. */
    annotations: jsonb('annotations').notNull().$type<Record<string, string>>().default({}),
    /** User-supplied desired state. Schema validated per-kind by the route. */
    spec: jsonb('spec').notNull().$type<Record<string, unknown>>().default({}),
    /** Operator-/agent-supplied observed state. */
    status: jsonb('status').notNull().$type<Record<string, unknown>>().default({}),
    /**
     * Monotonically increasing per-row revision. Bumped on every write.
     * The HTTP layer surfaces this as `metadata.resourceVersion` for
     * optimistic-concurrency clients.
     */
    revision: text('revision').notNull().default('1'),
    createdAt: timestamp('created_at', { withTimezone: true }).notNull().defaultNow(),
    updatedAt: timestamp('updated_at', { withTimezone: true }).notNull().defaultNow(),
    deletedAt: timestamp('deleted_at', { withTimezone: true }),
  },
  (table) => ({
    pk: uniqueIndex('resources_kind_namespace_name_idx').on(table.kind, table.namespace, table.name),
    kindIdx: index('resources_kind_idx').on(table.kind),
    updatedIdx: index('resources_updated_idx').on(table.updatedAt),
  })
);

export type ResourceRow = typeof resources.$inferSelect;
export type NewResourceRow = typeof resources.$inferInsert;
