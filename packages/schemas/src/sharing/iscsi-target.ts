import { z } from 'zod';
import { ConditionSchema } from '../common/condition';
import { ApiVersionSchema } from '../common/enums';
import { ObjectMetaSchema } from '../common/metadata';
import { SecretReferenceSchema } from '../common/references';

export const IscsiAclModeSchema = z.enum(['any', 'whitelist']);
export type IscsiAclMode = z.infer<typeof IscsiAclModeSchema>;

export const IscsiChapAuthSchema = z.object({
  enabled: z.boolean(),
  mutual: z.boolean().optional(),
  userSecret: SecretReferenceSchema.optional(),
  mutualSecret: SecretReferenceSchema.optional(),
});
export type IscsiChapAuth = z.infer<typeof IscsiChapAuthSchema>;

export const IscsiTargetSpecSchema = z.object({
  blockVolume: z.string(),
  portal: z.object({
    hostInterface: z.string(),
    port: z.number().int().min(1).max(65535).optional(),
  }),
  iqn: z.string().optional(),
  aclMode: IscsiAclModeSchema.optional(),
  initiatorAllowList: z.array(z.string()).optional(),
  chap: IscsiChapAuthSchema.optional(),
});
export type IscsiTargetSpec = z.infer<typeof IscsiTargetSpecSchema>;

export const IscsiTargetStatusSchema = z
  .object({
    phase: z.enum(['Pending', 'Active', 'Failed']),
    iqn: z.string(),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type IscsiTargetStatus = z.infer<typeof IscsiTargetStatusSchema>;

export const IscsiTargetSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('IscsiTarget'),
  metadata: ObjectMetaSchema,
  spec: IscsiTargetSpecSchema,
  status: IscsiTargetStatusSchema.optional(),
});
export type IscsiTarget = z.infer<typeof IscsiTargetSchema>;
