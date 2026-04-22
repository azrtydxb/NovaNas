import { z } from 'zod';
import { ConditionSchema } from '../common/condition';
import { ApiVersionSchema } from '../common/enums';
import { ObjectMetaSchema } from '../common/metadata';

export const KmsKeySpecSchema = z.object({
  description: z.string().optional(),
  rotation: z
    .object({
      enabled: z.boolean(),
      period: z.string().optional(),
    })
    .optional(),
  deletionProtection: z.boolean().optional(),
});
export type KmsKeySpec = z.infer<typeof KmsKeySpecSchema>;

export const KmsKeyStatusSchema = z
  .object({
    phase: z.enum(['Pending', 'Active', 'Rotating', 'Disabled', 'Destroyed']),
    keyId: z.string(),
    createdAt: z.string().datetime({ offset: true }),
    lastRotatedAt: z.string().datetime({ offset: true }),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type KmsKeyStatus = z.infer<typeof KmsKeyStatusSchema>;

export const KmsKeySchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('KmsKey'),
  metadata: ObjectMetaSchema,
  spec: KmsKeySpecSchema,
  status: KmsKeyStatusSchema.optional(),
});
export type KmsKey = z.infer<typeof KmsKeySchema>;
