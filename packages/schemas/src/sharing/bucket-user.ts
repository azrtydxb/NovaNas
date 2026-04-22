import { z } from 'zod';
import { ConditionSchema } from '../common/condition.js';
import { ApiVersionSchema } from '../common/enums.js';
import { ObjectMetaSchema } from '../common/metadata.js';
import { SecretReferenceSchema } from '../common/references.js';

export const BucketUserPolicyActionSchema = z.enum([
  'read',
  'write',
  'delete',
  'list',
  'manage',
  'bypassGovernance',
]);
export type BucketUserPolicyAction = z.infer<typeof BucketUserPolicyActionSchema>;

export const BucketUserPolicySchema = z.object({
  bucket: z.string(),
  prefix: z.string().optional(),
  actions: z.array(BucketUserPolicyActionSchema),
  effect: z.enum(['allow', 'deny']).default('allow').optional(),
});
export type BucketUserPolicy = z.infer<typeof BucketUserPolicySchema>;

export const BucketUserSpecSchema = z.object({
  displayName: z.string().optional(),
  credentials: z.object({
    accessKeySecret: SecretReferenceSchema.optional(),
    secretKeySecret: SecretReferenceSchema.optional(),
  }),
  policies: z.array(BucketUserPolicySchema).optional(),
});
export type BucketUserSpec = z.infer<typeof BucketUserSpecSchema>;

export const BucketUserStatusSchema = z
  .object({
    phase: z.enum(['Pending', 'Active', 'Failed']),
    accessKeyId: z.string(),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type BucketUserStatus = z.infer<typeof BucketUserStatusSchema>;

export const BucketUserSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('BucketUser'),
  metadata: ObjectMetaSchema,
  spec: BucketUserSpecSchema,
  status: BucketUserStatusSchema.optional(),
});
export type BucketUser = z.infer<typeof BucketUserSchema>;
