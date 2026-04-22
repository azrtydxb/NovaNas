import { z } from 'zod';
import { ConditionSchema } from '../common/condition.js';
import { ApiVersionSchema } from '../common/enums.js';
import { ObjectMetaSchema } from '../common/metadata.js';

export const UpdateChannelSchema = z.enum(['stable', 'beta', 'edge', 'manual']);
export type UpdateChannel = z.infer<typeof UpdateChannelSchema>;

export const UpdatePolicySpecSchema = z.object({
  channel: UpdateChannelSchema,
  autoUpdate: z.boolean().optional(),
  autoReboot: z.boolean().optional(),
  maintenanceWindow: z
    .object({
      cron: z.string(),
      durationMinutes: z.number().int().positive(),
    })
    .optional(),
  skipVersions: z.array(z.string()).optional(),
});
export type UpdatePolicySpec = z.infer<typeof UpdatePolicySpecSchema>;

export const UpdatePolicyStatusSchema = z
  .object({
    phase: z.enum(['Idle', 'Checking', 'Downloading', 'Installing', 'PendingReboot', 'Failed']),
    currentVersion: z.string(),
    availableVersion: z.string(),
    lastCheck: z.string().datetime({ offset: true }),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type UpdatePolicyStatus = z.infer<typeof UpdatePolicyStatusSchema>;

export const UpdatePolicySchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('UpdatePolicy'),
  metadata: ObjectMetaSchema,
  spec: UpdatePolicySpecSchema,
  status: UpdatePolicyStatusSchema.optional(),
});
export type UpdatePolicy = z.infer<typeof UpdatePolicySchema>;
