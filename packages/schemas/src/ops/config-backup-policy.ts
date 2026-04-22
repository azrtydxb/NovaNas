import { z } from 'zod';
import { ConditionSchema } from '../common/condition';
import { ApiVersionSchema } from '../common/enums';
import { ObjectMetaSchema } from '../common/metadata';
import { CronSchema } from '../common/quantity';
import { SecretReferenceSchema } from '../common/references';

export const ConfigBackupDestinationSchema = z.object({
  name: z.string(),
  type: z.enum(['bucket', 's3', 'cloudBackupTarget', 'localPath']),
  bucket: z.string().optional(),
  cloudBackupTarget: z.string().optional(),
  path: z.string().optional(),
  s3: z
    .object({
      endpoint: z.string().optional(),
      bucket: z.string(),
      region: z.string().optional(),
      prefix: z.string().optional(),
      credentialsSecret: SecretReferenceSchema,
    })
    .optional(),
});
export type ConfigBackupDestination = z.infer<typeof ConfigBackupDestinationSchema>;

export const ConfigBackupPolicySpecSchema = z.object({
  cron: CronSchema,
  destinations: z.array(ConfigBackupDestinationSchema),
  include: z
    .object({
      crds: z.boolean().optional(),
      keycloak: z.boolean().optional(),
      openbao: z.boolean().optional(),
      postgres: z.boolean().optional(),
    })
    .optional(),
  encryption: z
    .object({
      enabled: z.boolean(),
      passphraseSecret: SecretReferenceSchema.optional(),
    })
    .optional(),
  retention: z
    .object({
      keepLast: z.number().int().nonnegative(),
    })
    .optional(),
});
export type ConfigBackupPolicySpec = z.infer<typeof ConfigBackupPolicySpecSchema>;

export const ConfigBackupPolicyStatusSchema = z
  .object({
    phase: z.enum(['Active', 'Running', 'Failed']),
    lastRun: z.string().datetime({ offset: true }),
    nextRun: z.string().datetime({ offset: true }),
    lastArchive: z.string(),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type ConfigBackupPolicyStatus = z.infer<typeof ConfigBackupPolicyStatusSchema>;

export const ConfigBackupPolicySchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('ConfigBackupPolicy'),
  metadata: ObjectMetaSchema,
  spec: ConfigBackupPolicySpecSchema,
  status: ConfigBackupPolicyStatusSchema.optional(),
});
export type ConfigBackupPolicy = z.infer<typeof ConfigBackupPolicySchema>;
