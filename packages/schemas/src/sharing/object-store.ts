import { z } from 'zod';
import { ConditionSchema } from '../common/condition.js';
import { ApiVersionSchema } from '../common/enums.js';
import { ObjectMetaSchema } from '../common/metadata.js';

export const ObjectStoreTlsSchema = z.object({
  enabled: z.boolean(),
  certificate: z.string().optional(),
});
export type ObjectStoreTls = z.infer<typeof ObjectStoreTlsSchema>;

export const ObjectStoreFeaturesSchema = z
  .object({
    versioning: z.boolean(),
    objectLock: z.boolean(),
    replication: z.boolean(),
    website: z.boolean(),
    select: z.boolean(),
    notifications: z.boolean(),
  })
  .partial();
export type ObjectStoreFeatures = z.infer<typeof ObjectStoreFeaturesSchema>;

export const ObjectStoreSpecSchema = z.object({
  bindInterface: z.string().optional(),
  port: z.number().int().min(1).max(65535).optional(),
  tls: ObjectStoreTlsSchema.optional(),
  region: z.string().optional(),
  features: ObjectStoreFeaturesSchema.optional(),
});
export type ObjectStoreSpec = z.infer<typeof ObjectStoreSpecSchema>;

export const ObjectStoreStatusSchema = z
  .object({
    phase: z.enum(['Pending', 'Running', 'Failed']),
    endpoint: z.string(),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type ObjectStoreStatus = z.infer<typeof ObjectStoreStatusSchema>;

export const ObjectStoreSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('ObjectStore'),
  metadata: ObjectMetaSchema,
  spec: ObjectStoreSpecSchema,
  status: ObjectStoreStatusSchema.optional(),
});
export type ObjectStore = z.infer<typeof ObjectStoreSchema>;
