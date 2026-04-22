import { z } from 'zod';
import { ConditionSchema } from '../common/condition.js';
import { ApiVersionSchema } from '../common/enums.js';
import { ObjectMetaSchema } from '../common/metadata.js';

export const ServiceNameSchema = z.enum([
  'ssh',
  'smb',
  'nfs',
  'iscsi',
  'nvmeof',
  's3',
  'api',
  'ui',
  'grafana',
  'prometheus',
  'loki',
  'keycloak',
  'openbao',
]);
export type ServiceName = z.infer<typeof ServiceNameSchema>;

export const ServiceToggleSchema = z.object({
  name: ServiceNameSchema,
  enabled: z.boolean(),
  bindInterface: z.string().optional(),
  port: z.number().int().min(1).max(65535).optional(),
});
export type ServiceToggle = z.infer<typeof ServiceToggleSchema>;

export const ServicePolicySpecSchema = z.object({
  services: z.array(ServiceToggleSchema),
});
export type ServicePolicySpec = z.infer<typeof ServicePolicySpecSchema>;

export const ServicePolicyStatusSchema = z
  .object({
    phase: z.enum(['Applied', 'Failed']),
    appliedAt: z.string().datetime({ offset: true }),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type ServicePolicyStatus = z.infer<typeof ServicePolicyStatusSchema>;

export const ServicePolicySchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('ServicePolicy'),
  metadata: ObjectMetaSchema,
  spec: ServicePolicySpecSchema,
  status: ServicePolicyStatusSchema.optional(),
});
export type ServicePolicy = z.infer<typeof ServicePolicySchema>;
