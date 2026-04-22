import { z } from 'zod';
import { ConditionSchema } from '../common/condition';
import { ApiVersionSchema } from '../common/enums';
import { ObjectMetaSchema } from '../common/metadata';

export const SshKeySpecSchema = z.object({
  owner: z.string(),
  publicKey: z.string(),
  comment: z.string().optional(),
  expiresAt: z.string().datetime({ offset: true }).optional(),
});
export type SshKeySpec = z.infer<typeof SshKeySpecSchema>;

export const SshKeyStatusSchema = z
  .object({
    phase: z.enum(['Active', 'Expired', 'Revoked']),
    fingerprint: z.string(),
    keyType: z.string(),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type SshKeyStatus = z.infer<typeof SshKeyStatusSchema>;

export const SshKeySchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('SshKey'),
  metadata: ObjectMetaSchema,
  spec: SshKeySpecSchema,
  status: SshKeyStatusSchema.optional(),
});
export type SshKey = z.infer<typeof SshKeySchema>;
