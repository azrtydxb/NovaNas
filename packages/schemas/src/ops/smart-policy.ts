import { z } from 'zod';
import { ConditionSchema } from '../common/condition.js';
import { ApiVersionSchema } from '../common/enums.js';
import { ObjectMetaSchema } from '../common/metadata.js';
import { CronSchema } from '../common/quantity.js';

export const SmartAppliesToSchema = z
  .object({
    all: z.boolean(),
    pools: z.array(z.string()),
    disks: z.array(z.string()),
    classes: z.array(z.enum(['nvme', 'ssd', 'hdd'])),
  })
  .partial();
export type SmartAppliesTo = z.infer<typeof SmartAppliesToSchema>;

export const SmartThresholdSchema = z.object({
  warning: z.number(),
  critical: z.number(),
});
export type SmartThreshold = z.infer<typeof SmartThresholdSchema>;

export const SmartActionSchema = z.enum(['alert', 'alertAndMarkDegraded', 'markDegraded', 'none']);
export type SmartAction = z.infer<typeof SmartActionSchema>;

export const SmartPolicySpecSchema = z.object({
  appliesTo: SmartAppliesToSchema,
  shortTest: z.object({ cron: CronSchema }).optional(),
  longTest: z.object({ cron: CronSchema }).optional(),
  thresholds: z
    .object({
      reallocatedSectors: SmartThresholdSchema,
      pendingSectors: SmartThresholdSchema,
      temperature: SmartThresholdSchema,
      powerOnHours: SmartThresholdSchema,
    })
    .partial()
    .optional(),
  actions: z
    .object({
      onWarning: SmartActionSchema,
      onCritical: SmartActionSchema,
    })
    .partial()
    .optional(),
});
export type SmartPolicySpec = z.infer<typeof SmartPolicySpecSchema>;

export const SmartPolicyStatusSchema = z
  .object({
    phase: z.enum(['Active', 'Failed']),
    diskCount: z.number().int().nonnegative(),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type SmartPolicyStatus = z.infer<typeof SmartPolicyStatusSchema>;

export const SmartPolicySchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('SmartPolicy'),
  metadata: ObjectMetaSchema,
  spec: SmartPolicySpecSchema,
  status: SmartPolicyStatusSchema.optional(),
});
export type SmartPolicy = z.infer<typeof SmartPolicySchema>;
