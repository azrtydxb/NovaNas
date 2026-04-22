import { z } from 'zod';
import { ConditionSchema } from '../common/condition.js';
import { ApiVersionSchema } from '../common/enums.js';
import { ObjectMetaSchema } from '../common/metadata.js';
import { SecretReferenceSchema } from '../common/references.js';

export const UpsIntegrationSchema = z.enum(['nut', 'apcupsd']);
export type UpsIntegration = z.infer<typeof UpsIntegrationSchema>;

export const UpsActionSchema = z.enum(['shutdown', 'alertOnly', 'stopVms', 'stopApps']);
export type UpsAction = z.infer<typeof UpsActionSchema>;

export const UpsPolicySpecSchema = z.object({
  integration: UpsIntegrationSchema,
  host: z.string().optional(),
  port: z.number().int().min(1).max(65535).optional(),
  deviceName: z.string().optional(),
  authSecret: SecretReferenceSchema.optional(),
  thresholds: z
    .object({
      batteryPercent: z.number().min(0).max(100).optional(),
      runtimeSeconds: z.number().int().nonnegative().optional(),
    })
    .optional(),
  onBattery: z.array(UpsActionSchema).optional(),
  onLowBattery: z.array(UpsActionSchema).optional(),
});
export type UpsPolicySpec = z.infer<typeof UpsPolicySpecSchema>;

export const UpsPolicyStatusSchema = z
  .object({
    phase: z.enum(['Active', 'Disconnected', 'Failed']),
    batteryPercent: z.number(),
    runtimeSeconds: z.number().int().nonnegative(),
    onBattery: z.boolean(),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type UpsPolicyStatus = z.infer<typeof UpsPolicyStatusSchema>;

export const UpsPolicySchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('UpsPolicy'),
  metadata: ObjectMetaSchema,
  spec: UpsPolicySpecSchema,
  status: UpsPolicyStatusSchema.optional(),
});
export type UpsPolicy = z.infer<typeof UpsPolicySchema>;
