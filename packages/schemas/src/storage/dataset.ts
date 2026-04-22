import { z } from 'zod';
import { ConditionSchema } from '../common/condition.js';
import {
  AclModeSchema,
  ApiVersionSchema,
  CompressionSchema,
  FilesystemTypeSchema,
} from '../common/enums.js';
import { ObjectMetaSchema } from '../common/metadata.js';
import { BytesQuantitySchema, DurationSchema } from '../common/quantity.js';
import { ProtectionPolicySchema } from './protection.js';

export const DatasetEncryptionSchema = z.object({
  enabled: z.boolean(),
  kmsKey: z.string().optional(),
});
export type DatasetEncryption = z.infer<typeof DatasetEncryptionSchema>;

export const DatasetTieringSchema = z.object({
  primary: z.string(),
  demoteTo: z.string().optional(),
  demoteAfter: DurationSchema.optional(),
  promoteAfterAccesses: z.number().int().positive().optional(),
});
export type DatasetTiering = z.infer<typeof DatasetTieringSchema>;

export const DatasetQuotaSchema = z.object({
  hard: BytesQuantitySchema.optional(),
  soft: BytesQuantitySchema.optional(),
});
export type DatasetQuota = z.infer<typeof DatasetQuotaSchema>;

export const DatasetDefaultsSchema = z.object({
  owner: z.string().optional(),
  group: z.string().optional(),
  mode: z
    .string()
    .regex(/^0?[0-7]{3,4}$/, 'invalid POSIX mode')
    .optional(),
});
export type DatasetDefaults = z.infer<typeof DatasetDefaultsSchema>;

export const DatasetSpecSchema = z.object({
  pool: z.string(),
  size: BytesQuantitySchema,
  filesystem: FilesystemTypeSchema,
  protection: ProtectionPolicySchema.optional(),
  aclMode: AclModeSchema.optional(),
  tiering: DatasetTieringSchema.optional(),
  encryption: DatasetEncryptionSchema.optional(),
  compression: CompressionSchema.optional(),
  quota: DatasetQuotaSchema.optional(),
  defaults: DatasetDefaultsSchema.optional(),
});
export type DatasetSpec = z.infer<typeof DatasetSpecSchema>;

export const DatasetStatusSchema = z
  .object({
    phase: z.enum(['Pending', 'Mounted', 'Degraded', 'Failed']),
    mountPoint: z.string(),
    usedBytes: z.number().int().nonnegative(),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type DatasetStatus = z.infer<typeof DatasetStatusSchema>;

export const DatasetSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('Dataset'),
  metadata: ObjectMetaSchema,
  spec: DatasetSpecSchema,
  status: DatasetStatusSchema.optional(),
});
export type Dataset = z.infer<typeof DatasetSchema>;
