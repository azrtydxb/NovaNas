import { z } from 'zod';
import { ConditionSchema } from '../common/condition.js';
import { ApiVersionSchema } from '../common/enums.js';
import { ObjectMetaSchema } from '../common/metadata.js';

export const ApiTokenSpecSchema = z.object({
  owner: z.string(),
  scopes: z.array(z.string()),
  expiresAt: z.string().datetime({ offset: true }).optional(),
  description: z.string().optional(),
});
export type ApiTokenSpec = z.infer<typeof ApiTokenSpecSchema>;

export const ApiTokenStatusSchema = z
  .object({
    phase: z.enum(['Pending', 'Active', 'Expired', 'Revoked']),
    tokenId: z.string(),
    createdAt: z.string().datetime({ offset: true }),
    lastUsedAt: z.string().datetime({ offset: true }),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type ApiTokenStatus = z.infer<typeof ApiTokenStatusSchema>;

export const ApiTokenSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('ApiToken'),
  metadata: ObjectMetaSchema,
  spec: ApiTokenSpecSchema,
  status: ApiTokenStatusSchema.optional(),
});
export type ApiToken = z.infer<typeof ApiTokenSchema>;
