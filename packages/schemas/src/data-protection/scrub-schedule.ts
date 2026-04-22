import { z } from 'zod';
import { ConditionSchema } from '../common/condition';
import { ApiVersionSchema } from '../common/enums';
import { ObjectMetaSchema } from '../common/metadata';
import { CronSchema } from '../common/quantity';

export const ScrubScheduleSpecSchema = z.object({
  pool: z.string(),
  cron: CronSchema,
  priority: z.enum(['low', 'normal', 'high']).optional(),
  repair: z.boolean().optional(),
  suspended: z.boolean().optional(),
});
export type ScrubScheduleSpec = z.infer<typeof ScrubScheduleSpecSchema>;

export const ScrubScheduleStatusSchema = z
  .object({
    phase: z.enum(['Active', 'Running', 'Suspended', 'Failed']),
    lastRun: z.string().datetime({ offset: true }),
    nextRun: z.string().datetime({ offset: true }),
    chunksScrubbed: z.number().int().nonnegative(),
    errorsRepaired: z.number().int().nonnegative(),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type ScrubScheduleStatus = z.infer<typeof ScrubScheduleStatusSchema>;

export const ScrubScheduleSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('ScrubSchedule'),
  metadata: ObjectMetaSchema,
  spec: ScrubScheduleSpecSchema,
  status: ScrubScheduleStatusSchema.optional(),
});
export type ScrubSchedule = z.infer<typeof ScrubScheduleSchema>;
