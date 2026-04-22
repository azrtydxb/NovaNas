import { z } from 'zod';
import { ConditionSchema } from '../common/condition.js';
import { ApiVersionSchema } from '../common/enums.js';
import { ObjectMetaSchema } from '../common/metadata.js';
import { SecretReferenceSchema } from '../common/references.js';

export const CloudBackupProviderSchema = z.enum(['s3', 'b2', 'azure', 'gcs', 'swift']);
export type CloudBackupProvider = z.infer<typeof CloudBackupProviderSchema>;

export const CloudBackupEngineSchema = z.enum(['restic', 'borg', 'kopia']);
export type CloudBackupEngine = z.infer<typeof CloudBackupEngineSchema>;

export const CloudBackupTargetSpecSchema = z.object({
  provider: CloudBackupProviderSchema,
  endpoint: z.string().optional(),
  bucket: z.string(),
  region: z.string().optional(),
  prefix: z.string().optional(),
  credentialsSecret: SecretReferenceSchema,
  repositoryPasswordSecret: SecretReferenceSchema.optional(),
  engine: CloudBackupEngineSchema.optional(),
});
export type CloudBackupTargetSpec = z.infer<typeof CloudBackupTargetSpecSchema>;

export const CloudBackupTargetStatusSchema = z
  .object({
    phase: z.enum(['Pending', 'Ready', 'Failed']),
    repositoryInitialized: z.boolean(),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type CloudBackupTargetStatus = z.infer<typeof CloudBackupTargetStatusSchema>;

export const CloudBackupTargetSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('CloudBackupTarget'),
  metadata: ObjectMetaSchema,
  spec: CloudBackupTargetSpecSchema,
  status: CloudBackupTargetStatusSchema.optional(),
});
export type CloudBackupTarget = z.infer<typeof CloudBackupTargetSchema>;
