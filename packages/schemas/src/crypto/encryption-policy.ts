import { z } from 'zod';
import { ConditionSchema } from '../common/condition.js';
import { ApiVersionSchema } from '../common/enums.js';
import { ObjectMetaSchema } from '../common/metadata.js';

export const EncryptionCipherSchema = z.enum(['AES256-GCM', 'AES128-GCM', 'XChaCha20-Poly1305']);
export type EncryptionCipher = z.infer<typeof EncryptionCipherSchema>;

export const EncryptionPolicySpecSchema = z.object({
  defaultEnabled: z.boolean().optional(),
  cipher: EncryptionCipherSchema.optional(),
  masterKey: z
    .object({
      sealedBy: z.enum(['tpm', 'passphrase', 'both']).optional(),
    })
    .optional(),
  requireForTiers: z.array(z.string()).optional(),
});
export type EncryptionPolicySpec = z.infer<typeof EncryptionPolicySpecSchema>;

export const EncryptionPolicyStatusSchema = z
  .object({
    phase: z.enum(['Active', 'Failed']),
    masterKeySealed: z.boolean(),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type EncryptionPolicyStatus = z.infer<typeof EncryptionPolicyStatusSchema>;

export const EncryptionPolicySchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('EncryptionPolicy'),
  metadata: ObjectMetaSchema,
  spec: EncryptionPolicySpecSchema,
  status: EncryptionPolicyStatusSchema.optional(),
});
export type EncryptionPolicy = z.infer<typeof EncryptionPolicySchema>;
