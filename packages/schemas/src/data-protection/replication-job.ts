import { z } from 'zod';
import { ConditionSchema } from '../common/condition';
import { ApiVersionSchema } from '../common/enums';
import { ObjectMetaSchema } from '../common/metadata';
import { CronSchema } from '../common/quantity';
import { VolumeSourceRefSchema } from '../common/references';
import { RetentionPolicySchema } from './snapshot-schedule';

export const ReplicationDirectionSchema = z.enum(['push', 'pull']);
export type ReplicationDirection = z.infer<typeof ReplicationDirectionSchema>;

export const ReplicationJobSpecSchema = z.object({
  source: VolumeSourceRefSchema,
  target: z.string(),
  direction: ReplicationDirectionSchema,
  cron: CronSchema.optional(),
  retention: RetentionPolicySchema.optional(),
  remoteName: z.string().optional(),
  suspended: z.boolean().optional(),
});
export type ReplicationJobSpec = z.infer<typeof ReplicationJobSpecSchema>;

export const ReplicationJobStatusSchema = z
  .object({
    phase: z.enum(['Pending', 'Running', 'Succeeded', 'Failed', 'Suspended']),
    lastRun: z.string().datetime({ offset: true }),
    nextRun: z.string().datetime({ offset: true }),
    bytesTransferred: z.number().int().nonnegative(),
    lastSnapshot: z.string(),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type ReplicationJobStatus = z.infer<typeof ReplicationJobStatusSchema>;

export const ReplicationJobSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('ReplicationJob'),
  metadata: ObjectMetaSchema,
  spec: ReplicationJobSpecSchema,
  status: ReplicationJobStatusSchema.optional(),
});
export type ReplicationJob = z.infer<typeof ReplicationJobSchema>;
