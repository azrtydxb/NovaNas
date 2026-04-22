import { z } from 'zod';
import { ConditionSchema } from '../common/condition.js';
import { ApiVersionSchema } from '../common/enums.js';
import { ObjectMetaSchema } from '../common/metadata.js';
import { BandwidthSchema } from '../common/quantity.js';
import { SecretReferenceSchema } from '../common/references.js';

export const ReplicationTransportSchema = z.object({
  compression: z.enum(['none', 'zstd', 'lz4']).optional(),
  encryption: z.boolean().optional(),
  bandwidth: z
    .object({
      limit: BandwidthSchema.optional(),
      schedule: z.string().optional(),
    })
    .optional(),
});
export type ReplicationTransport = z.infer<typeof ReplicationTransportSchema>;

export const ReplicationTargetSpecSchema = z.object({
  endpoint: z.string().url(),
  auth: z.object({
    secretRef: z.string().optional(),
    secret: SecretReferenceSchema.optional(),
  }),
  transport: ReplicationTransportSchema.optional(),
  tlsVerify: z.boolean().optional(),
});
export type ReplicationTargetSpec = z.infer<typeof ReplicationTargetSpecSchema>;

export const ReplicationTargetStatusSchema = z
  .object({
    phase: z.enum(['Pending', 'Connected', 'Failed']),
    remoteVersion: z.string(),
    lastHandshake: z.string().datetime({ offset: true }),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type ReplicationTargetStatus = z.infer<typeof ReplicationTargetStatusSchema>;

export const ReplicationTargetSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('ReplicationTarget'),
  metadata: ObjectMetaSchema,
  spec: ReplicationTargetSpecSchema,
  status: ReplicationTargetStatusSchema.optional(),
});
export type ReplicationTarget = z.infer<typeof ReplicationTargetSchema>;
