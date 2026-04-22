import { z } from 'zod';
import { ConditionSchema } from '../common/condition.js';
import { ApiVersionSchema } from '../common/enums.js';
import { ObjectMetaSchema } from '../common/metadata.js';
import { BytesQuantitySchema } from '../common/quantity.js';
import { ProtectionPolicySchema } from './protection.js';

export const BlockVolumeEncryptionSchema = z.object({
  enabled: z.boolean(),
  kmsKey: z.string().optional(),
});
export type BlockVolumeEncryption = z.infer<typeof BlockVolumeEncryptionSchema>;

export const BlockVolumeTieringSchema = z.object({
  primary: z.string(),
  demoteTo: z.string().optional(),
  demoteAfter: z.string().optional(),
  promoteAfterAccesses: z.number().int().positive().optional(),
});
export type BlockVolumeTiering = z.infer<typeof BlockVolumeTieringSchema>;

export const BlockVolumeSpecSchema = z.object({
  pool: z.string(),
  size: BytesQuantitySchema,
  protection: ProtectionPolicySchema.optional(),
  encryption: BlockVolumeEncryptionSchema.optional(),
  tiering: BlockVolumeTieringSchema.optional(),
});
export type BlockVolumeSpec = z.infer<typeof BlockVolumeSpecSchema>;

export const BlockVolumeStatusSchema = z
  .object({
    phase: z.enum(['Pending', 'Bound', 'Available', 'Failed']),
    usedBytes: z.number().int().nonnegative(),
    device: z.string(),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type BlockVolumeStatus = z.infer<typeof BlockVolumeStatusSchema>;

export const BlockVolumeSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('BlockVolume'),
  metadata: ObjectMetaSchema,
  spec: BlockVolumeSpecSchema,
  status: BlockVolumeStatusSchema.optional(),
});
export type BlockVolume = z.infer<typeof BlockVolumeSchema>;
