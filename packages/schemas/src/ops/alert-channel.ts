import { z } from 'zod';
import { ConditionSchema } from '../common/condition';
import { ApiVersionSchema } from '../common/enums';
import { ObjectMetaSchema } from '../common/metadata';
import { SecretReferenceSchema } from '../common/references';

export const AlertChannelTypeSchema = z.enum([
  'email',
  'webhook',
  'ntfy',
  'pushover',
  'slack',
  'discord',
  'telegram',
  'browserPush',
]);
export type AlertChannelType = z.infer<typeof AlertChannelTypeSchema>;

export const AlertChannelSpecSchema = z.object({
  type: AlertChannelTypeSchema,
  email: z
    .object({
      to: z.array(z.string().email()),
      from: z.string().email().optional(),
    })
    .optional(),
  webhook: z
    .object({
      url: z.string().url(),
      secret: SecretReferenceSchema.optional(),
      headers: z.record(z.string(), z.string()).optional(),
    })
    .optional(),
  ntfy: z
    .object({
      server: z.string().url().optional(),
      topic: z.string(),
      authSecret: SecretReferenceSchema.optional(),
    })
    .optional(),
  pushover: z
    .object({
      userKey: SecretReferenceSchema,
      token: SecretReferenceSchema,
    })
    .optional(),
  minSeverity: z.enum(['info', 'warning', 'critical']).optional(),
});
export type AlertChannelSpec = z.infer<typeof AlertChannelSpecSchema>;

export const AlertChannelStatusSchema = z
  .object({
    phase: z.enum(['Active', 'Failed']),
    lastDeliveryAt: z.string().datetime({ offset: true }),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type AlertChannelStatus = z.infer<typeof AlertChannelStatusSchema>;

export const AlertChannelSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('AlertChannel'),
  metadata: ObjectMetaSchema,
  spec: AlertChannelSpecSchema,
  status: AlertChannelStatusSchema.optional(),
});
export type AlertChannel = z.infer<typeof AlertChannelSchema>;
