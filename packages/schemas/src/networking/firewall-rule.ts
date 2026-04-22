import { z } from 'zod';
import { ConditionSchema } from '../common/condition.js';
import { ApiVersionSchema } from '../common/enums.js';
import { ObjectMetaSchema } from '../common/metadata.js';

export const FirewallScopeSchema = z.enum(['host', 'pod']);
export type FirewallScope = z.infer<typeof FirewallScopeSchema>;

export const FirewallDirectionSchema = z.enum(['inbound', 'outbound']);
export type FirewallDirection = z.infer<typeof FirewallDirectionSchema>;

export const FirewallActionSchema = z.enum(['allow', 'deny', 'reject', 'log']);
export type FirewallAction = z.infer<typeof FirewallActionSchema>;

export const FirewallProtocolSchema = z.enum(['tcp', 'udp', 'icmp', 'any']);
export type FirewallProtocol = z.infer<typeof FirewallProtocolSchema>;

export const FirewallEndpointSchema = z.object({
  cidrs: z.array(z.string()).optional(),
  labels: z.record(z.string(), z.string()).optional(),
  ports: z.array(z.number().int().min(1).max(65535)).optional(),
  protocol: FirewallProtocolSchema.optional(),
});
export type FirewallEndpoint = z.infer<typeof FirewallEndpointSchema>;

export const FirewallRuleSpecSchema = z.object({
  scope: FirewallScopeSchema,
  direction: FirewallDirectionSchema,
  action: FirewallActionSchema,
  interface: z.string().optional(),
  source: FirewallEndpointSchema.optional(),
  destination: FirewallEndpointSchema.optional(),
  priority: z.number().int().optional(),
});
export type FirewallRuleSpec = z.infer<typeof FirewallRuleSpecSchema>;

export const FirewallRuleStatusSchema = z
  .object({
    phase: z.enum(['Active', 'Failed']),
    installedAt: z.string().datetime({ offset: true }),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type FirewallRuleStatus = z.infer<typeof FirewallRuleStatusSchema>;

export const FirewallRuleSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('FirewallRule'),
  metadata: ObjectMetaSchema,
  spec: FirewallRuleSpecSchema,
  status: FirewallRuleStatusSchema.optional(),
});
export type FirewallRule = z.infer<typeof FirewallRuleSchema>;
