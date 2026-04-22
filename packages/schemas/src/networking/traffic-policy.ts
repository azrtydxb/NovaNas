import { z } from 'zod';
import { ConditionSchema } from '../common/condition.js';
import { ApiVersionSchema } from '../common/enums.js';
import { ObjectMetaSchema } from '../common/metadata.js';
import { BandwidthSchema, CronSchema } from '../common/quantity.js';

export const TrafficPolicyScopeKindSchema = z.enum([
  'HostInterface',
  'Namespace',
  'App',
  'Vm',
  'ReplicationJob',
  'ObjectStore',
]);
export type TrafficPolicyScopeKind = z.infer<typeof TrafficPolicyScopeKindSchema>;

export const TrafficLimitsSchema = z
  .object({
    egress: z
      .object({
        max: BandwidthSchema.optional(),
        burst: BandwidthSchema.optional(),
      })
      .optional(),
    ingress: z
      .object({
        max: BandwidthSchema.optional(),
        burst: BandwidthSchema.optional(),
      })
      .optional(),
  })
  .partial();
export type TrafficLimits = z.infer<typeof TrafficLimitsSchema>;

export const TrafficSchedulingWindowSchema = z.object({
  cron: CronSchema,
  durationMinutes: z.number().int().positive(),
  overrideEgress: z
    .object({
      max: BandwidthSchema.optional(),
      burst: BandwidthSchema.optional(),
    })
    .optional(),
  overrideIngress: z
    .object({
      max: BandwidthSchema.optional(),
      burst: BandwidthSchema.optional(),
    })
    .optional(),
});
export type TrafficSchedulingWindow = z.infer<typeof TrafficSchedulingWindowSchema>;

export const TrafficPolicySpecSchema = z.object({
  scope: z.object({
    kind: TrafficPolicyScopeKindSchema,
    name: z.string(),
    namespace: z.string().optional(),
  }),
  limits: TrafficLimitsSchema.optional(),
  scheduling: z.record(z.string(), TrafficSchedulingWindowSchema).optional(),
  priority: z.number().int().optional(),
});
export type TrafficPolicySpec = z.infer<typeof TrafficPolicySpecSchema>;

export const TrafficPolicyStatusSchema = z
  .object({
    phase: z.enum(['Active', 'Failed']),
    appliedAt: z.string().datetime({ offset: true }),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type TrafficPolicyStatus = z.infer<typeof TrafficPolicyStatusSchema>;

export const TrafficPolicySchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('TrafficPolicy'),
  metadata: ObjectMetaSchema,
  spec: TrafficPolicySpecSchema,
  status: TrafficPolicyStatusSchema.optional(),
});
export type TrafficPolicy = z.infer<typeof TrafficPolicySchema>;
