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

// LabelSelector matches metav1.LabelSelector exactly; copied here so
// downstream consumers don't need to depend on a Kubernetes types
// package just to construct a Pool.spec.nodeSelector.
export const LabelSelectorSchema = z.object({
  matchLabels: z.record(z.string(), z.string()).optional(),
  matchExpressions: z
    .array(
      z.object({
        key: z.string(),
        operator: z.enum(['In', 'NotIn', 'Exists', 'DoesNotExist']),
        values: z.array(z.string()).optional(),
      })
    )
    .optional(),
});
export type LabelSelector = z.infer<typeof LabelSelectorSchema>;

// FileBackend lets the storage controller materialize a pool as a
// loop-mounted file rather than a raw block device — useful for dev
// boxes with no spare disks. Same shape as BackendAssignment's
// FileBackendSpec; kept here so the Pool spec is self-contained.
export const PoolFileBackendSchema = z.object({
  path: z.string(),
  sizeBytes: z.number().int().positive().optional(),
});
export type PoolFileBackend = z.infer<typeof PoolFileBackendSchema>;

export const StoragePoolSpecSchema = z.object({
  tier: PoolTierSchema,
  deviceFilter: DeviceFilterSchema.optional(),
  recoveryRate: RecoveryRateSchema.optional(),
  rebalanceOnAdd: RebalanceOnAddSchema.optional(),
  disks: z.array(z.string()).optional(),
  // The fields below are read by the storage controller / agent to
  // synthesize BackendAssignments. Optional so existing pools without
  // them still parse — defaults are applied at reconcile time.
  backendType: z.enum(['file', 'lvm', 'raw']).optional(),
  nodeSelector: LabelSelectorSchema.optional(),
  fileBackend: PoolFileBackendSchema.optional(),
});
export type StoragePoolSpec = z.infer<typeof StoragePoolSpecSchema>;

export const StoragePoolStatusSchema = z
  .object({
    // Operator emits Pending/Ready/Active/Degraded/Failed — keep the
    // schema in sync with packages/operators/api/v1alpha1/storagepool_types.go.
    phase: z.enum(['Pending', 'Ready', 'Active', 'Degraded', 'Failed']),
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
