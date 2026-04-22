# @novanas/db

Drizzle ORM schema, migrations, and a typed Postgres client for NovaNas.

This package owns everything that lives in the NovaNas control-plane Postgres:

- `users`, `groups`, `user_groups` — local projection of Keycloak identity
- `sessions` — persisted session records (hot path is Redis; this is for audit)
- `audit_log` — append-only audit trail (see `docs/12-observability.md`)
- `jobs` — long-running background work (backups, scrubs, etc.)
- `notifications` — user-facing alerts
- `user_preferences` — per-user UI state
- `app_catalog_cache` — cached Helm / app catalog entries
- `api_tokens` — long-lived CLI / automation tokens
- `metric_rollups` — downsampled metrics for cheap dashboard paint

Postgres is the source of truth for these tables; Keycloak remains the
source of truth for credentials and group membership (see
`docs/10-identity-and-secrets.md`).

## Usage

```ts
import { createDb, migrate } from '@novanas/db';

const db = createDb(process.env.DATABASE_URL!);
await migrate(db);

// `db` is a fully typed Drizzle client:
const rows = await db.query.users.findMany();
```

## Configuration

Point the package at a Postgres instance via `DATABASE_URL`:

```
export DATABASE_URL=postgres://novanas:novanas@localhost:5432/novanas
```

## Generating migrations

After editing any file under `src/schema/`:

```
pnpm --filter @novanas/db db:generate
```

This writes SQL into `migrations/`. Commit both the schema change and the
generated SQL in the same PR.

## Applying migrations

In production the API server calls `migrate(db)` at startup. For local
development you can also do:

```
pnpm --filter @novanas/db db:push
```

which syncs the schema directly without generating migration files.

## Notes

- All primary keys are UUIDs (`uuid().defaultRandom()`).
- All tables carry `created_at` (and `updated_at` where mutations are expected).
- Foreign keys are declared explicitly; cascading deletes are used only where
  the child row has no independent meaning (e.g. `user_groups`, `sessions`).
- `jsonb` payload columns use `.$type<T>()` to surface concrete shapes to
  TypeScript consumers.
