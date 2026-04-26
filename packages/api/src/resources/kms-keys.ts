import { type KmsKey, KmsKeySchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import type { OpenBaoAdmin } from '../services/openbao-admin.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildKmsKeyResource(db: DbClient): PgResource<KmsKey> {
  return new PgResource<KmsKey>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'KmsKey',
    schema: KmsKeySchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient, openbao?: OpenBaoAdmin | null): void {
  registerCrudRoutes<KmsKey>({
    app,
    basePath: '/api/v1/kms-keys',
    tag: 'kms-keys',
    kind: 'KmsKey',
    resource: buildKmsKeyResource(db),
    schema: KmsKeySchema,
    // OpenBao Transit key sync (#51). The deleted operator's
    // KeyProvisioner did more than this — it also provisioned a
    // wrappedDK envelope used by Dataset for per-volume encryption.
    // That stays in the Dataset hook (separate checkbox); KmsKey only
    // owns the Transit key itself.
    afterCreate: openbao
      ? async (key, req) => {
          await openbao.ensureTransitKey(key.metadata.name, {
            autoRotatePeriod: key.spec.rotation?.enabled ? key.spec.rotation.period : undefined,
            // We never auto-flip deletion_allowed at create time —
            // that's reserved for the explicit afterDelete path
            // below, gated on the deletionProtection flag.
          });
          req.log.debug(
            { kind: 'KmsKey', name: key.metadata.name },
            'transit key ensured in openbao'
          );
        }
      : undefined,
    afterPatch: openbao
      ? async (key, _patch, req) => {
          await openbao.ensureTransitKey(key.metadata.name, {
            autoRotatePeriod: key.spec.rotation?.enabled ? key.spec.rotation.period : undefined,
          });
          req.log.debug(
            { kind: 'KmsKey', name: key.metadata.name },
            'transit key reconfigured in openbao'
          );
        }
      : undefined,
    afterDelete: openbao
      ? async (name, req) => {
          // Spec is gone by afterDelete — including the
          // deletionProtection flag. The api makes the conservative
          // choice: ALWAYS delete when the resource is deleted.
          // Operators who need protection should reject the API
          // delete via authz before it hits Postgres.
          await openbao.deleteTransitKey(name);
          req.log.debug({ kind: 'KmsKey', name }, 'transit key deleted in openbao');
        }
      : undefined,
  });
}
