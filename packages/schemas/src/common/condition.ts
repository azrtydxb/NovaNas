import { z } from 'zod';

/**
 * Kubernetes-style status Condition.
 */
export const ConditionStatusSchema = z.enum(['True', 'False', 'Unknown']);
export type ConditionStatus = z.infer<typeof ConditionStatusSchema>;

export const ConditionSchema = z.object({
  type: z.string(),
  status: ConditionStatusSchema,
  reason: z.string().optional(),
  message: z.string().optional(),
  lastTransitionTime: z.string().datetime({ offset: true }).optional(),
  observedGeneration: z.number().int().optional(),
});
export type Condition = z.infer<typeof ConditionSchema>;
