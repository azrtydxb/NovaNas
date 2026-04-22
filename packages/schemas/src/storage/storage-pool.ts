import { z } from 'zod';
import { ConditionSchema } from '../common/condition.js';
import {
  ApiVersionSchema,
  DeviceClassSchema,
  PoolTierSchema,
  RebalanceOnAddSchema,
  RecoveryRateSchema,
} from '../common/enums.js';
import { ObjectMetaSchema } from '../common/metadata.js';
import { BytesQuantitySchema } from '../common/quantity.js';

export const DeviceFilterSchema = z.object({
  preferredClass: DeviceClassSchema.optional(),
  minSize: BytesQuantitySchema.optional(),
  maxSize: BytesQuantitySchema.optional(),
});
export type DeviceFilter = z.infer<typeof DeviceFilterSchema>;

export const StoragePoolSpecSchema = z.object({
  tier: PoolTierSchema,
  deviceFilter: DeviceFilterSchema.optional(),
  recoveryRate: RecoveryRateSchema.optional(),
  rebalanceOnAdd: RebalanceOnAddSchema.optional(),
  disks: z.array(z.string()).optional(),
});
export type StoragePoolSpec = z.infer<typeof StoragePoolSpecSchema>;

export const StoragePoolStatusSchema = z
  .object({
    phase: z.enum(['Pending', 'Active', 'Degraded', 'Failed']),
    capacity: z.object({
      totalBytes: z.number().int().nonnegative(),
      usedBytes: z.number().int().nonnegative(),
      availableBytes: z.number().int().nonnegative(),
    }),
    diskCount: z.number().int().nonnegative(),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type StoragePoolStatus = z.infer<typeof StoragePoolStatusSchema>;

export const StoragePoolSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('StoragePool'),
  metadata: ObjectMetaSchema,
  spec: StoragePoolSpecSchema,
  status: StoragePoolStatusSchema.optional(),
});
export type StoragePool = z.infer<typeof StoragePoolSchema>;
