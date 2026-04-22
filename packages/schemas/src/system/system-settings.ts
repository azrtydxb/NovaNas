import { z } from 'zod';
import { ConditionSchema } from '../common/condition';
import { ApiVersionSchema } from '../common/enums';
import { ObjectMetaSchema } from '../common/metadata';
import { SecretReferenceSchema } from '../common/references';

export const SmtpSettingsSchema = z.object({
  host: z.string(),
  port: z.number().int().min(1).max(65535),
  encryption: z.enum(['none', 'starttls', 'tls']).optional(),
  from: z.string().email(),
  authSecret: SecretReferenceSchema.optional(),
});
export type SmtpSettings = z.infer<typeof SmtpSettingsSchema>;

export const SystemSettingsSpecSchema = z.object({
  hostname: z.string().optional(),
  timezone: z.string().optional(),
  locale: z.string().optional(),
  ntp: z
    .object({
      servers: z.array(z.string()),
      enabled: z.boolean().optional(),
    })
    .optional(),
  smtp: SmtpSettingsSchema.optional(),
  motd: z.string().optional(),
  supportContact: z.string().optional(),
});
export type SystemSettingsSpec = z.infer<typeof SystemSettingsSpecSchema>;

export const SystemSettingsStatusSchema = z
  .object({
    phase: z.enum(['Applied', 'Failed']),
    appliedAt: z.string().datetime({ offset: true }),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type SystemSettingsStatus = z.infer<typeof SystemSettingsStatusSchema>;

export const SystemSettingsSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('SystemSettings'),
  metadata: ObjectMetaSchema,
  spec: SystemSettingsSpecSchema,
  status: SystemSettingsStatusSchema.optional(),
});
export type SystemSettings = z.infer<typeof SystemSettingsSchema>;
