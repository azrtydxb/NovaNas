import { z } from 'zod';
import { ConditionSchema } from '../common/condition.js';
import { ApiVersionSchema } from '../common/enums.js';
import { ObjectMetaSchema } from '../common/metadata.js';
import { DurationSchema } from '../common/quantity.js';
import { SecretReferenceSchema } from '../common/references.js';

export const AuditSinkTypeSchema = z.enum(['loki', 's3', 'syslog', 'file', 'webhook']);
export type AuditSinkType = z.infer<typeof AuditSinkTypeSchema>;

export const AuditSinkSchema = z.object({
  name: z.string(),
  type: AuditSinkTypeSchema,
  loki: z
    .object({ url: z.string().url(), authSecret: SecretReferenceSchema.optional() })
    .optional(),
  s3: z
    .object({
      endpoint: z.string().optional(),
      bucket: z.string(),
      prefix: z.string().optional(),
      credentialsSecret: SecretReferenceSchema,
    })
    .optional(),
  syslog: z
    .object({
      host: z.string(),
      port: z.number().int().min(1).max(65535),
      protocol: z.enum(['udp', 'tcp', 'tls']),
    })
    .optional(),
  file: z.object({ path: z.string() }).optional(),
  webhook: z
    .object({ url: z.string().url(), authSecret: SecretReferenceSchema.optional() })
    .optional(),
});
export type AuditSink = z.infer<typeof AuditSinkSchema>;

export const AuditPolicySpecSchema = z.object({
  events: z.array(z.string()).optional(),
  severity: z.enum(['info', 'warning', 'critical']).optional(),
  sinks: z.array(AuditSinkSchema),
  retention: DurationSchema.optional(),
});
export type AuditPolicySpec = z.infer<typeof AuditPolicySpecSchema>;

export const AuditPolicyStatusSchema = z
  .object({
    phase: z.enum(['Active', 'Failed']),
    eventsEmitted: z.number().int().nonnegative(),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type AuditPolicyStatus = z.infer<typeof AuditPolicyStatusSchema>;

export const AuditPolicySchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('AuditPolicy'),
  metadata: ObjectMetaSchema,
  spec: AuditPolicySpecSchema,
  status: AuditPolicyStatusSchema.optional(),
});
export type AuditPolicy = z.infer<typeof AuditPolicySchema>;
