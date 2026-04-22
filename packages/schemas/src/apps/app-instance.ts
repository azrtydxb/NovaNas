import { z } from 'zod';
import { ConditionSchema } from '../common/condition.js';
import { ApiVersionSchema, ExposureModeSchema } from '../common/enums.js';
import { ObjectMetaSchema } from '../common/metadata.js';
import { BytesQuantitySchema } from '../common/quantity.js';

export const AppInstanceStorageModeSchema = z.enum(['ReadWrite', 'ReadOnly']);
export type AppInstanceStorageMode = z.infer<typeof AppInstanceStorageModeSchema>;

export const AppInstanceStorageSchema = z.object({
  name: z.string(),
  dataset: z.string().optional(),
  blockVolume: z.string().optional(),
  bucket: z.string().optional(),
  size: BytesQuantitySchema.optional(),
  mode: AppInstanceStorageModeSchema.optional(),
  mountPath: z.string().optional(),
});
export type AppInstanceStorage = z.infer<typeof AppInstanceStorageSchema>;

export const AppInstanceExposeSchema = z.object({
  port: z.number().int().min(1).max(65535),
  protocol: z.enum(['TCP', 'UDP']).optional(),
  advertise: ExposureModeSchema.optional(),
  tls: z
    .object({
      certificate: z.string(),
    })
    .optional(),
  hostname: z.string().optional(),
});
export type AppInstanceExpose = z.infer<typeof AppInstanceExposeSchema>;

export const AppInstanceNetworkSchema = z.object({
  expose: z.array(AppInstanceExposeSchema).optional(),
});
export type AppInstanceNetwork = z.infer<typeof AppInstanceNetworkSchema>;

export const AppInstanceUpdatesSchema = z.object({
  autoUpdate: z.boolean().optional(),
  channel: z.string().optional(),
});
export type AppInstanceUpdates = z.infer<typeof AppInstanceUpdatesSchema>;

export const AppInstanceSpecSchema = z.object({
  app: z.string(),
  version: z.string(),
  values: z.record(z.string(), z.unknown()).optional(),
  storage: z.array(AppInstanceStorageSchema).optional(),
  network: AppInstanceNetworkSchema.optional(),
  updates: AppInstanceUpdatesSchema.optional(),
});
export type AppInstanceSpec = z.infer<typeof AppInstanceSpecSchema>;

export const AppInstanceStatusSchema = z
  .object({
    phase: z.enum(['Pending', 'Running', 'Stopped', 'Failed', 'Updating']),
    healthy: z.boolean(),
    revision: z.number().int().nonnegative(),
    exposedAt: z.string(),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type AppInstanceStatus = z.infer<typeof AppInstanceStatusSchema>;

export const AppInstanceSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('AppInstance'),
  metadata: ObjectMetaSchema,
  spec: AppInstanceSpecSchema,
  status: AppInstanceStatusSchema.optional(),
});
export type AppInstance = z.infer<typeof AppInstanceSchema>;
