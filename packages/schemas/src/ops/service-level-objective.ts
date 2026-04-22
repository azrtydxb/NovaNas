import { z } from 'zod';
import { ConditionSchema } from '../common/condition';
import { ApiVersionSchema } from '../common/enums';
import { ObjectMetaSchema } from '../common/metadata';
import { DurationSchema } from '../common/quantity';

export const SloIndicatorSchema = z.object({
  goodQuery: z.string(),
  totalQuery: z.string(),
});
export type SloIndicator = z.infer<typeof SloIndicatorSchema>;

export const ServiceLevelObjectiveSpecSchema = z.object({
  description: z.string().optional(),
  target: z.number().min(0).max(100),
  window: DurationSchema,
  indicator: SloIndicatorSchema,
  alertOnBurnRate: z.boolean().optional(),
  alertChannels: z.array(z.string()).optional(),
});
export type ServiceLevelObjectiveSpec = z.infer<typeof ServiceLevelObjectiveSpecSchema>;

export const ServiceLevelObjectiveStatusSchema = z
  .object({
    phase: z.enum(['Active', 'Failed']),
    currentObjective: z.number(),
    errorBudgetRemaining: z.number(),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type ServiceLevelObjectiveStatus = z.infer<typeof ServiceLevelObjectiveStatusSchema>;

export const ServiceLevelObjectiveSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('ServiceLevelObjective'),
  metadata: ObjectMetaSchema,
  spec: ServiceLevelObjectiveSpecSchema,
  status: ServiceLevelObjectiveStatusSchema.optional(),
});
export type ServiceLevelObjective = z.infer<typeof ServiceLevelObjectiveSchema>;
