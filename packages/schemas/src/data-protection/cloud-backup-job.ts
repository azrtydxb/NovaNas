import { z } from 'zod';
import { ConditionSchema } from '../common/condition.js';
import { ApiVersionSchema } from '../common/enums.js';
import { ObjectMetaSchema } from '../common/metadata.js';
import { CronSchema } from '../common/quantity.js';
import { VolumeSourceRefSchema } from '../common/references.js';
import { RetentionPolicySchema } from './snapshot-schedule.js';

export const CloudBackupJobSpecSchema = z.object({
  source: VolumeSourceRefSchema,
  target: z.string(),
  cron: CronSchema.optional(),
  retention: RetentionPolicySchema.optional(),
  excludes: z.array(z.string()).optional(),
  suspended: z.boolean().optional(),
});
export type CloudBackupJobSpec = z.infer<typeof CloudBackupJobSpecSchema>;

export const CloudBackupJobStatusSchema = z
  .object({
    phase: z.enum(['Pending', 'Running', 'Succeeded', 'Failed', 'Suspended']),
    lastRun: z.string().datetime({ offset: true }),
    nextRun: z.string().datetime({ offset: true }),
    bytesUploaded: z.number().int().nonnegative(),
    snapshotId: z.string(),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type CloudBackupJobStatus = z.infer<typeof CloudBackupJobStatusSchema>;

export const CloudBackupJobSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('CloudBackupJob'),
  metadata: ObjectMetaSchema,
  spec: CloudBackupJobSpecSchema,
  status: CloudBackupJobStatusSchema.optional(),
});
export type CloudBackupJob = z.infer<typeof CloudBackupJobSchema>;
