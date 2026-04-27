import { z } from 'zod';
import { ConditionSchema } from '../common/condition.js';
import { ApiVersionSchema } from '../common/enums.js';
import { ObjectMetaSchema } from '../common/metadata.js';
import { DeviceFilterSchema } from './storage-pool.js';

// File backend (filesystem-backed pool) reuses the same shape as
// StoragePool.spec.fileBackend in the operators API. Kept inline here
// to avoid pulling in the operators schema as a dependency.
export const FileBackendSpecSchema = z.object({
  path: z.string(),
  sizeBytes: z.number().int().positive().optional(),
});
export type FileBackendSpec = z.infer<typeof FileBackendSpecSchema>;

export const BackendAssignmentSpecSchema = z.object({
  // Pool this assignment belongs to.
  poolRef: z.string(),
  // Node this assignment targets.
  nodeName: z.string(),
  // Backend type, copied from the StoragePool spec.
  backendType: z.enum(['file', 'lvm', 'raw']),
  // Device selector, copied from StoragePool. raw/lvm only.
  deviceFilter: DeviceFilterSchema.optional(),
  // File backend config, copied from StoragePool. file only.
  fileBackend: FileBackendSpecSchema.optional(),
});
export type BackendAssignmentSpec = z.infer<typeof BackendAssignmentSpecSchema>;

export const BackendAssignmentStatusSchema = z
  .object({
    phase: z.enum(['Pending', 'Provisioning', 'Ready', 'Failed']),
    device: z.string(),
    pcieAddr: z.string(),
    bdevName: z.string(),
    capacity: z.number().int().nonnegative(),
    message: z.string(),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type BackendAssignmentStatus = z.infer<typeof BackendAssignmentStatusSchema>;

export const BackendAssignmentSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('BackendAssignment'),
  metadata: ObjectMetaSchema,
  spec: BackendAssignmentSpecSchema,
  status: BackendAssignmentStatusSchema.optional(),
});
export type BackendAssignment = z.infer<typeof BackendAssignmentSchema>;
