import { z } from 'zod';
import { ConditionSchema } from '../common/condition.js';
import { ApiVersionSchema } from '../common/enums.js';
import { ObjectMetaSchema } from '../common/metadata.js';

export const IngressRuleSchema = z.object({
  host: z.string(),
  backend: z.string(),
  path: z.string().optional(),
});
export type IngressRule = z.infer<typeof IngressRuleSchema>;

export const IngressTlsSchema = z.object({
  certificate: z.string(),
});
export type IngressTls = z.infer<typeof IngressTlsSchema>;

export const IngressSpecSchema = z.object({
  hostname: z.string(),
  tls: IngressTlsSchema.optional(),
  rules: z.array(IngressRuleSchema),
});
export type IngressSpec = z.infer<typeof IngressSpecSchema>;

export const IngressStatusSchema = z
  .object({
    phase: z.enum(['Pending', 'Active', 'Failed']),
    vip: z.string(),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type IngressStatus = z.infer<typeof IngressStatusSchema>;

export const IngressSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('Ingress'),
  metadata: ObjectMetaSchema,
  spec: IngressSpecSchema,
  status: IngressStatusSchema.optional(),
});
export type Ingress = z.infer<typeof IngressSchema>;
