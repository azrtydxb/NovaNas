import { z } from 'zod';
import { ConditionSchema } from '../common/condition';
import { ApiVersionSchema, ObjectLockModeSchema, VersioningSchema } from '../common/enums';
import { ObjectMetaSchema } from '../common/metadata';
import { BytesQuantitySchema, DurationSchema } from '../common/quantity';
import { ProtectionPolicySchema } from './protection';

export const BucketEncryptionSchema = z.object({
  enabled: z.boolean(),
  kmsKey: z.string().optional(),
});
export type BucketEncryption = z.infer<typeof BucketEncryptionSchema>;

export const BucketTieringSchema = z.object({
  primary: z.string(),
  demoteTo: z.string().optional(),
  demoteAfter: DurationSchema.optional(),
});
export type BucketTiering = z.infer<typeof BucketTieringSchema>;

export const BucketObjectLockSchema = z.object({
  enabled: z.boolean(),
  mode: ObjectLockModeSchema.optional(),
  defaultRetention: z
    .object({
      period: DurationSchema,
    })
    .optional(),
});
export type BucketObjectLock = z.infer<typeof BucketObjectLockSchema>;

export const BucketQuotaSchema = z.object({
  hardBytes: BytesQuantitySchema.optional(),
  hardObjects: z.number().int().nonnegative().optional(),
});
export type BucketQuota = z.infer<typeof BucketQuotaSchema>;

export const BucketLifecycleRuleSchema = z.object({
  prefix: z.string().optional(),
  expireAfter: DurationSchema.optional(),
  transitionAfter: DurationSchema.optional(),
  transitionTo: z.string().optional(),
  abortIncompleteMultipartAfter: DurationSchema.optional(),
});
export type BucketLifecycleRule = z.infer<typeof BucketLifecycleRuleSchema>;

export const BucketSpecSchema = z.object({
  store: z.string(),
  protection: ProtectionPolicySchema.optional(),
  tiering: BucketTieringSchema.optional(),
  encryption: BucketEncryptionSchema.optional(),
  versioning: VersioningSchema.optional(),
  objectLock: BucketObjectLockSchema.optional(),
  quota: BucketQuotaSchema.optional(),
  lifecycle: z.array(BucketLifecycleRuleSchema).optional(),
});
export type BucketSpec = z.infer<typeof BucketSpecSchema>;

export const BucketStatusSchema = z
  .object({
    phase: z.enum(['Pending', 'Active', 'Failed']),
    objectCount: z.number().int().nonnegative(),
    usedBytes: z.number().int().nonnegative(),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type BucketStatus = z.infer<typeof BucketStatusSchema>;

export const BucketSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('Bucket'),
  metadata: ObjectMetaSchema,
  spec: BucketSpecSchema,
  status: BucketStatusSchema.optional(),
});
export type Bucket = z.infer<typeof BucketSchema>;
