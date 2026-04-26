/**
 * Common interface for `CrdResource` and `PgResource`.
 *
 * The route layer (`resources/_register.ts`) uses this so a single
 * `registerCrudRoutes` call works against either backend. During the
 * CRD → Postgres migration we flip resources one at a time by
 * swapping which class instantiates the resource handle; the routes,
 * Zod schemas, and validation hooks don't change.
 */

export interface ListResourceOptions {
  namespace?: string;
  limit?: number;
}

export interface ListResourceResult<T> {
  items: T[];
}

export interface Resource<T> {
  readonly namespaced: boolean;
  list(opts?: ListResourceOptions): Promise<ListResourceResult<T>>;
  get(name: string, namespace?: string): Promise<T>;
  create(body: T, namespace?: string): Promise<T>;
  patch(name: string, patch: object, namespace?: string): Promise<T>;
  delete(name: string, namespace?: string): Promise<void>;
}

/**
 * Errors raised by Resource implementations. Both CrdApiError and
 * PgApiError extend this so error mapping in the route layer is
 * uniform regardless of backend.
 */
export interface ResourceErrorLike {
  statusCode: number;
  message: string;
  name: string;
}

export function isNotFound(err: unknown): boolean {
  const n = (err as { name?: string })?.name ?? '';
  return n === 'CrdNotFoundError' || n === 'PgNotFoundError';
}

export function isConflict(err: unknown): boolean {
  const n = (err as { name?: string })?.name ?? '';
  return n === 'CrdConflictError' || n === 'PgConflictError';
}

export function isInvalid(err: unknown): boolean {
  const n = (err as { name?: string })?.name ?? '';
  return n === 'CrdInvalidError' || n === 'PgInvalidError';
}

export function isResourceApiError(err: unknown): err is ResourceErrorLike {
  const n = (err as { name?: string })?.name ?? '';
  return n === 'CrdApiError' || n === 'PgApiError' || isNotFound(err) || isConflict(err) || isInvalid(err);
}
