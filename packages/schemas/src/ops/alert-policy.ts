import { z } from 'zod';
import { ConditionSchema } from '../common/condition';
import { ApiVersionSchema } from '../common/enums';
import { ObjectMetaSchema } from '../common/metadata';
import { DurationSchema } from '../common/quantity';

export const AlertSeveritySchema = z.enum(['info', 'warning', 'critical']);
export type AlertSeverity = z.infer<typeof AlertSeveritySchema>;

export const AlertConditionSchema = z.object({
  query: z.string(),
  operator: z.enum(['>', '<', '>=', '<=', '==', '!=']),
  threshold: z.number(),
  for: DurationSchema.optional(),
});
export type AlertCondition = z.infer<typeof AlertConditionSchema>;

export const AlertPolicySpecSchema = z.object({
  description: z.string().optional(),
  severity: AlertSeveritySchema,
  condition: AlertConditionSchema,
  channels: z.array(z.string()),
  labels: z.record(z.string(), z.string()).optional(),
  annotations: z.record(z.string(), z.string()).optional(),
  suspended: z.boolean().optional(),
});
export type AlertPolicySpec = z.infer<typeof AlertPolicySpecSchema>;

export const AlertPolicyStatusSchema = z
  .object({
    phase: z.enum(['Active', 'Firing', 'Suspended', 'Failed']),
    lastFired: z.string().datetime({ offset: true }),
    fireCount: z.number().int().nonnegative(),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type AlertPolicyStatus = z.infer<typeof AlertPolicyStatusSchema>;

export const AlertPolicySchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('AlertPolicy'),
  metadata: ObjectMetaSchema,
  spec: AlertPolicySpecSchema,
  status: AlertPolicyStatusSchema.optional(),
});
export type AlertPolicy = z.infer<typeof AlertPolicySchema>;
