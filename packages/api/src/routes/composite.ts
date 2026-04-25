import type { CustomObjectsApi } from '@kubernetes/client-node';
import {
  type AppInstance,
  AppInstanceSchema,
  type Dataset,
  DatasetSchema,
  type Disk,
  DiskSchema,
  type Share,
  ShareSchema,
  type Vm,
  VmSchema,
} from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { AuthzRole, canWrite, ownNamespace } from '../auth/authz.js';
import { requireAuth } from '../auth/decorators.js';
import { hasRole } from '../auth/rbac.js';
import { buildAppInstanceResource } from '../resources/apps.js';
import { buildDatasetResource } from '../resources/datasets.js';
import { buildDiskResource } from '../resources/disks.js';
import { buildShareResource } from '../resources/shares.js';
import { buildVmResource } from '../resources/vms.js';
import { writeAudit } from '../services/audit.js';
import { type CompositeStep, runComposite } from '../services/composite.js';
import {
  CrdApiError,
  CrdConflictError,
  CrdInvalidError,
  CrdNotFoundError,
} from '../services/crd.js';
import type { DbClient } from '../services/db.js';
import type { JobsService } from '../services/jobs.js';
import type { AuthenticatedUser } from '../types.js';

interface CompositeDeps {
  kubeCustom?: CustomObjectsApi;
  db?: DbClient | null;
  jobs?: JobsService | null;
}

function errorStatus(err: unknown): number {
  if (err instanceof CrdNotFoundError) return 404;
  if (err instanceof CrdConflictError) return 409;
  if (err instanceof CrdInvalidError) return 422;
  if (err instanceof CrdApiError) return err.statusCode || 500;
  return 500;
}

function errorBody(err: unknown): { error: string; message: string } {
  if (err instanceof CrdApiError) return { error: err.name, message: err.message };
  const msg = (err as { message?: string })?.message ?? 'internal error';
  return { error: 'internal_error', message: msg };
}

export async function compositeRoutes(app: FastifyInstance, deps: CompositeDeps): Promise<void> {
  const { kubeCustom, db, jobs } = deps;
  const security = [{ sessionCookie: [] }];

  if (!kubeCustom) {
    // stub when kubeCustom unavailable (test / missing kubeconfig)
    for (const url of [
      '/api/v1/composite/dataset-with-share',
      '/api/v1/composite/install-app',
      '/api/v1/composite/create-vm',
    ]) {
      app.post(url, { schema: { tags: ['composite'], security } }, async (_req, reply) => {
        return reply
          .code(503)
          .send({ error: 'unavailable', message: 'kubernetes client not configured' });
      });
    }
    return;
  }

  const datasets = buildDatasetResource(kubeCustom);
  const shares = buildShareResource(kubeCustom);
  const vms = buildVmResource(kubeCustom);
  const disks = buildDiskResource(kubeCustom);
  const apps = buildAppInstanceResource(kubeCustom);

  // --------------------------------------------------------------------------
  // POST /api/v1/composite/dataset-with-share
  app.route<{
    Body: { dataset: Dataset; shares: Share[] };
  }>({
    method: 'POST',
    url: '/api/v1/composite/dataset-with-share',
    preHandler: requireAuth,
    schema: {
      summary: 'Create a Dataset and one or more Shares atomically',
      tags: ['composite'],
      security,
      body: { type: 'object' },
    },
    handler: async (req, reply) => {
      const user = req.user as AuthenticatedUser;
      if (!canWrite(user, 'Dataset') || !canWrite(user, 'Share')) {
        return reply.code(403).send({ error: 'forbidden', message: 'insufficient role' });
      }
      const body = (req.body ?? {}) as { dataset?: unknown; shares?: unknown };
      const ds = DatasetSchema.safeParse(body.dataset);
      if (!ds.success) {
        return reply.code(400).send({ error: 'invalid_body', message: ds.error.message });
      }
      if (!Array.isArray(body.shares) || body.shares.length === 0) {
        return reply
          .code(400)
          .send({ error: 'invalid_body', message: 'shares[] required (non-empty)' });
      }
      const parsedShares: Share[] = [];
      for (const s of body.shares) {
        const r = ShareSchema.safeParse(s);
        if (!r.success) {
          return reply
            .code(400)
            .send({ error: 'invalid_body', message: `share: ${r.error.message}` });
        }
        parsedShares.push(r.data);
      }

      const jobRow = jobs
        ? await jobs
            .create({
              kind: 'composite:dataset-with-share',
              params: { dataset: ds.data.metadata.name, shares: parsedShares.length },
              ownerId: user.sub || null,
            })
            .catch(() => null)
        : null;

      const steps: CompositeStep<{ created: { dataset?: Dataset; shares: Share[] } }>[] = [
        {
          name: 'create-dataset',
          exec: async (ctx) => {
            const created = await datasets.create(ds.data);
            ctx.created.dataset = created;
            return created;
          },
          rollback: async (_ctx, created) => {
            await datasets.delete((created as Dataset).metadata.name).catch(() => {});
          },
        },
        ...parsedShares.map<CompositeStep<{ created: { dataset?: Dataset; shares: Share[] } }>>(
          (shareBody, idx) => ({
            name: `create-share-${idx}`,
            exec: async (ctx) => {
              const created = await shares.create(shareBody);
              ctx.created.shares.push(created);
              return created;
            },
            rollback: async (_ctx, created) => {
              await shares.delete((created as Share).metadata.name).catch(() => {});
            },
          })
        ),
      ];

      const ctx = { created: { dataset: undefined as Dataset | undefined, shares: [] as Share[] } };
      const result = await runComposite({ ctx, steps, logger: req.log });

      if (!result.success) {
        await writeAudit(db ?? null, req.log, {
          actor: user.username,
          action: 'composite.dataset-with-share',
          kind: 'Dataset',
          resourceId: ds.data.metadata.name,
          outcome: 'failure',
          ip: req.ip,
          details: { failedStep: result.failedStep, message: result.error.message },
        });
        if (jobs && jobRow) {
          await jobs
            .update(jobRow.id, {
              state: 'failed',
              error: result.error.message,
              finishedAt: new Date(),
            })
            .catch(() => {});
        }
        return reply.code(errorStatus(result.error)).send(errorBody(result.error));
      }

      await writeAudit(db ?? null, req.log, {
        actor: user.username,
        action: 'composite.dataset-with-share',
        kind: 'Dataset',
        resourceId: ds.data.metadata.name,
        outcome: 'success',
        ip: req.ip,
        details: { shares: ctx.created.shares.map((s) => s.metadata.name) },
      });
      if (jobs && jobRow) {
        await jobs
          .update(jobRow.id, {
            state: 'succeeded',
            progressPercent: 100,
            finishedAt: new Date(),
          })
          .catch(() => {});
      }

      return reply.code(201).send({
        dataset: ctx.created.dataset,
        shares: ctx.created.shares,
        jobId: jobRow?.id,
      });
    },
  });

  // --------------------------------------------------------------------------
  // POST /api/v1/composite/install-app
  app.route<{
    Body: {
      app: string;
      version: string;
      namespace: string;
      values?: Record<string, unknown>;
      autoDataset?: { name: string; size: string; pool: string };
    };
  }>({
    method: 'POST',
    url: '/api/v1/composite/install-app',
    preHandler: requireAuth,
    schema: {
      summary: 'Install an app, optionally provisioning a config dataset',
      tags: ['composite'],
      security,
      body: { type: 'object' },
    },
    handler: async (req, reply) => {
      const user = req.user as AuthenticatedUser;
      const body = req.body;
      if (!body || typeof body !== 'object') {
        return reply.code(400).send({ error: 'invalid_body', message: 'body required' });
      }
      if (!body.app || !body.version || !body.namespace) {
        return reply.code(400).send({
          error: 'invalid_body',
          message: 'app, version, namespace required',
        });
      }
      const namespace = body.namespace;
      if (!canWrite(user, 'AppInstance', namespace)) {
        return reply.code(403).send({ error: 'forbidden', message: 'insufficient role' });
      }
      if (body.autoDataset && !canWrite(user, 'Dataset')) {
        return reply.code(403).send({ error: 'forbidden', message: 'cannot create dataset' });
      }
      // Best-effort; operators construct user namespace as `user-<username>`.
      if (user && namespace !== ownNamespace(user) && !hasRole(user, AuthzRole.Admin)) {
        return reply.code(403).send({ error: 'forbidden', message: 'namespace not owned by user' });
      }

      const jobRow = jobs
        ? await jobs
            .create({
              kind: 'composite:install-app',
              params: { app: body.app, version: body.version, namespace },
              ownerId: user.sub || null,
            })
            .catch(() => null)
        : null;

      const ctx = {
        created: {
          dataset: undefined as Dataset | undefined,
          app: undefined as AppInstance | undefined,
        },
      };
      const steps: CompositeStep<typeof ctx>[] = [];

      if (body.autoDataset) {
        const dsSpec: Dataset = {
          apiVersion: 'novanas.io/v1alpha1',
          kind: 'Dataset',
          metadata: { name: body.autoDataset.name },
          spec: {
            pool: body.autoDataset.pool,
            size: body.autoDataset.size,
            filesystem: 'xfs',
          },
        };
        steps.push({
          name: 'create-dataset',
          exec: async (c) => {
            const created = await datasets.create(dsSpec);
            c.created.dataset = created;
            return created;
          },
          rollback: async (_c, created) => {
            await datasets.delete((created as Dataset).metadata.name).catch(() => {});
          },
        });
      }

      const appSpec: AppInstance = {
        apiVersion: 'novanas.io/v1alpha1',
        kind: 'AppInstance',
        metadata: { name: body.app, namespace },
        spec: {
          app: body.app,
          version: body.version,
          values: body.values,
          storage: body.autoDataset
            ? [
                {
                  name: 'config',
                  dataset: body.autoDataset.name,
                  size: body.autoDataset.size,
                },
              ]
            : undefined,
        },
      };
      steps.push({
        name: 'create-app-instance',
        exec: async (c) => {
          const created = await apps.create(appSpec, namespace);
          c.created.app = created;
          return created;
        },
        rollback: async (_c, created) => {
          await apps.delete((created as AppInstance).metadata.name, namespace).catch(() => {});
        },
      });

      const result = await runComposite({ ctx, steps, logger: req.log });

      if (!result.success) {
        await writeAudit(db ?? null, req.log, {
          actor: user.username,
          action: 'composite.install-app',
          kind: 'AppInstance',
          resourceId: body.app,
          namespace,
          outcome: 'failure',
          ip: req.ip,
          details: { failedStep: result.failedStep, message: result.error.message },
        });
        if (jobs && jobRow) {
          await jobs
            .update(jobRow.id, {
              state: 'failed',
              error: result.error.message,
              finishedAt: new Date(),
            })
            .catch(() => {});
        }
        return reply.code(errorStatus(result.error)).send(errorBody(result.error));
      }

      await writeAudit(db ?? null, req.log, {
        actor: user.username,
        action: 'composite.install-app',
        kind: 'AppInstance',
        resourceId: body.app,
        namespace,
        outcome: 'success',
        ip: req.ip,
      });
      if (jobs && jobRow) {
        await jobs
          .update(jobRow.id, {
            state: 'succeeded',
            progressPercent: 100,
            finishedAt: new Date(),
          })
          .catch(() => {});
      }

      return reply.code(201).send({
        appInstance: ctx.created.app,
        dataset: ctx.created.dataset,
        jobId: jobRow?.id,
      });
    },
  });

  // --------------------------------------------------------------------------
  // POST /api/v1/composite/create-vm
  app.route<{
    Body: {
      vm: Vm;
      disks: Array<{
        name: string;
        size: string;
        pool: string;
        source?: { type: string; [k: string]: unknown };
      }>;
    };
  }>({
    method: 'POST',
    url: '/api/v1/composite/create-vm',
    preHandler: requireAuth,
    schema: {
      summary: 'Create a VM with its block volumes',
      tags: ['composite'],
      security,
      body: { type: 'object' },
    },
    handler: async (req, reply) => {
      const user = req.user as AuthenticatedUser;
      const body = req.body ?? ({} as typeof req.body);
      const vmParse = VmSchema.safeParse(body.vm);
      if (!vmParse.success) {
        return reply
          .code(400)
          .send({ error: 'invalid_body', message: `vm: ${vmParse.error.message}` });
      }
      if (!Array.isArray(body.disks)) {
        return reply.code(400).send({ error: 'invalid_body', message: 'disks[] required' });
      }
      const namespace = vmParse.data.metadata.namespace ?? ownNamespace(user);
      if (!canWrite(user, 'Vm', namespace)) {
        return reply.code(403).send({ error: 'forbidden', message: 'insufficient role' });
      }

      const jobRow = jobs
        ? await jobs
            .create({
              kind: 'composite:create-vm',
              params: { vm: vmParse.data.metadata.name, disks: body.disks.length },
              ownerId: user.sub || null,
            })
            .catch(() => null)
        : null;

      const ctx = { created: { disks: [] as Disk[], vm: undefined as Vm | undefined } };
      const steps: CompositeStep<typeof ctx>[] = [];

      for (const [idx, d] of body.disks.entries()) {
        const diskBody: Disk = {
          apiVersion: 'novanas.io/v1alpha1',
          kind: 'Disk',
          metadata: { name: `${vmParse.data.metadata.name}-${d.name}` },
          spec: { pool: d.pool, role: 'data' },
        };
        const parsed = DiskSchema.safeParse(diskBody);
        if (!parsed.success) {
          return reply
            .code(400)
            .send({ error: 'invalid_body', message: `disk[${idx}]: ${parsed.error.message}` });
        }
        steps.push({
          name: `create-disk-${idx}`,
          exec: async (c) => {
            const created = await disks.create(parsed.data);
            c.created.disks.push(created);
            return created;
          },
          rollback: async (_c, created) => {
            await disks.delete((created as Disk).metadata.name).catch(() => {});
          },
        });
      }

      steps.push({
        name: 'create-vm',
        exec: async (c) => {
          const created = await vms.create(vmParse.data, namespace);
          c.created.vm = created;
          return created;
        },
        rollback: async (_c, created) => {
          await vms.delete((created as Vm).metadata.name, namespace).catch(() => {});
        },
      });

      const result = await runComposite({ ctx, steps, logger: req.log });

      if (!result.success) {
        await writeAudit(db ?? null, req.log, {
          actor: user.username,
          action: 'composite.create-vm',
          kind: 'Vm',
          resourceId: vmParse.data.metadata.name,
          namespace,
          outcome: 'failure',
          ip: req.ip,
          details: { failedStep: result.failedStep, message: result.error.message },
        });
        if (jobs && jobRow) {
          await jobs
            .update(jobRow.id, {
              state: 'failed',
              error: result.error.message,
              finishedAt: new Date(),
            })
            .catch(() => {});
        }
        return reply.code(errorStatus(result.error)).send(errorBody(result.error));
      }

      await writeAudit(db ?? null, req.log, {
        actor: user.username,
        action: 'composite.create-vm',
        kind: 'Vm',
        resourceId: vmParse.data.metadata.name,
        namespace,
        outcome: 'success',
        ip: req.ip,
        details: { disks: ctx.created.disks.map((d) => d.metadata.name) },
      });
      if (jobs && jobRow) {
        await jobs
          .update(jobRow.id, {
            state: 'succeeded',
            progressPercent: 100,
            finishedAt: new Date(),
          })
          .catch(() => {});
      }

      return reply.code(201).send({
        vm: ctx.created.vm,
        disks: ctx.created.disks,
        jobId: jobRow?.id,
      });
    },
  });
}
