import { type Disk, DiskSchema, type StoragePool, StoragePoolSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { RegisterValidationError, registerCrudRoutes } from './_register.js';

export function buildDiskResource(db: DbClient): PgResource<Disk> {
  return new PgResource<Disk>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'Disk',
    schema: DiskSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  const disks = buildDiskResource(db);
  // Sibling pool resource for cross-kind validation (deviceFilter
  // compatibility). Same db handle, no extra route registrations.
  const pools = new PgResource<StoragePool>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'StoragePool',
    schema: StoragePoolSchema,
    namespaced: false,
  });

  registerCrudRoutes<Disk>({
    app,
    basePath: '/api/v1/disks',
    tag: 'disks',
    kind: 'Disk',
    resource: disks,
    schema: DiskSchema,
    validate: async (action, body, req) => {
      if (action !== 'patch') return;
      // We only enforce on attach: a PATCH that sets spec.pool.
      const patchBody = body as { spec?: { pool?: string; role?: string } };
      const targetPool = patchBody?.spec?.pool;
      if (!targetPool) return;

      // Identify which Disk we're patching — name comes from the
      // path. Look it up to read deviceClass + system label.
      const params = (req.params ?? {}) as { name?: string };
      if (!params.name) return;
      const disk = await disks.get(params.name).catch(() => null);
      if (!disk) {
        throw new RegisterValidationError(`Disk '${params.name}' not found.`);
      }

      // 1. System disks (OS / mounted partitions) are never poolable.
      // Now actually enforced — there's no kube-apiserver backdoor.
      const labels = disk.metadata?.labels ?? {};
      if (labels['novanas.io/system'] === 'true') {
        throw new RegisterValidationError(
          `Disk '${disk.metadata.name}' hosts the operating system and ` +
            `cannot be added to a pool.`
        );
      }

      // 2. Pool's deviceFilter must match the disk's class. A pool
      //    created with preferredClass=hdd refuses ssd/nvme members.
      const pool = await pools.get(targetPool).catch(() => null);
      if (!pool) {
        throw new RegisterValidationError(`Pool '${targetPool}' not found.`);
      }
      const want = pool.spec?.deviceFilter?.preferredClass;
      const have = disk.status?.deviceClass;
      if (want && have && want !== have) {
        throw new RegisterValidationError(
          `Pool '${targetPool}' is restricted to ${want} devices; disk ` +
            `'${disk.metadata.name}' is ${have}.`
        );
      }
    },
  });
}
