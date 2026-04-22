import { z } from 'zod';
import { ConditionSchema } from '../common/condition.js';
import { ApiVersionSchema } from '../common/enums.js';
import { ObjectMetaSchema } from '../common/metadata.js';
import { CronSchema } from '../common/quantity.js';
import { VolumeSourceRefSchema } from '../common/references.js';

export const RetentionPolicySchema = z
  .object({
    hourly: z.number().int().nonnegative(),
    daily: z.number().int().nonnegative(),
    weekly: z.number().int().nonnegative(),
    monthly: z.number().int().nonnegative(),
    yearly: z.number().int().nonnegative(),
    keepLast: z.number().int().nonnegative(),
  })
  .partial();
export type RetentionPolicy = z.infer<typeof RetentionPolicySchema>;

export const SnapshotScheduleSpecSchema = z.object({
  source: VolumeSourceRefSchema,
  cron: CronSchema,
  retention: RetentionPolicySchema.optional(),
  namingFormat: z.string().optional(),
  locked: z.boolean().optional(),
  suspended: z.boolean().optional(),
});
export type SnapshotScheduleSpec = z.infer<typeof SnapshotScheduleSpecSchema>;

export const SnapshotScheduleStatusSchema = z
  .object({
    phase: z.enum(['Active', 'Suspended', 'Failed']),
    lastRun: z.string().datetime({ offset: true }),
    nextRun: z.string().datetime({ offset: true }),
    snapshotsCreated: z.number().int().nonnegative(),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type SnapshotScheduleStatus = z.infer<typeof SnapshotScheduleStatusSchema>;

export const SnapshotScheduleSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('SnapshotSchedule'),
  metadata: ObjectMetaSchema,
  spec: SnapshotScheduleSpecSchema,
  status: SnapshotScheduleStatusSchema.optional(),
});
export type SnapshotSchedule = z.infer<typeof SnapshotScheduleSchema>;
