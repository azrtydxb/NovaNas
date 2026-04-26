import { type Dataset, DatasetSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import type { OpenBaoAdmin } from '../services/openbao-admin.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildDatasetResource(db: DbClient): PgResource<Dataset> {
  return new PgResource<Dataset>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'Dataset',
    schema: DatasetSchema,
    namespaced: false,
  });
}

export function register(
  app: FastifyInstance,
  db: DbClient,
  openbao?: OpenBaoAdmin | null
): void {
  const resource = buildDatasetResource(db);
  registerCrudRoutes<Dataset>({
    app,
    basePath: '/api/v1/datasets',
    tag: 'datasets',
    kind: 'Dataset',
    resource,
    schema: DatasetSchema,
    // Per-dataset encryption envelope (#51). Mirrors the deleted
    // operator's KeyProvisioner.ProvisionVolume: when encryption is
    // enabled, generate a wrapped DK against the configured KMS key
    // (or fall back to the dataset's own name as the key) and persist
    // the ciphertext on status. The plaintext DK is never seen by the
    // api — agents recover it at mount time via /transit/decrypt.
    afterCreate: openbao
      ? async (ds, req) => {
          if (!ds.spec.encryption?.enabled) return;
          if (ds.status?.encryption?.provisioned) return; // idempotent
          const keyName = ds.spec.encryption.kmsKey ?? ds.metadata.name;
          // Make sure the key exists before asking for a datakey.
          await openbao.ensureTransitKey(keyName);
          const { ciphertext, keyVersion } = await openbao.generateDataKey(keyName);
          await resource.patch(ds.metadata.name, {
            status: {
              encryption: {
                provisioned: true,
                wrappedDK: ciphertext,
                keyVersion,
                provisionedAt: new Date().toISOString(),
              },
            },
          });
          req.log.debug(
            { kind: 'Dataset', name: ds.metadata.name, keyVersion },
            'wrapped DK provisioned'
          );
        }
      : undefined,
  });
}
