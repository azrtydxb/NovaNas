import { z } from 'zod';
import { ConditionSchema } from '../common/condition';
import { ApiVersionSchema } from '../common/enums';
import { ObjectMetaSchema } from '../common/metadata';
import { SecretReferenceSchema } from '../common/references';

export const RemoteAccessTunnelTypeSchema = z.enum(['sdwan', 'wireguard', 'tailscale']);
export type RemoteAccessTunnelType = z.infer<typeof RemoteAccessTunnelTypeSchema>;

export const RemoteAccessTunnelExposeSchema = z.object({
  app: z.string().optional(),
  vm: z.string().optional(),
  via: z.enum(['tunnel', 'direct']).optional(),
});
export type RemoteAccessTunnelExpose = z.infer<typeof RemoteAccessTunnelExposeSchema>;

export const RemoteAccessTunnelSpecSchema = z.object({
  type: RemoteAccessTunnelTypeSchema,
  endpoint: z.object({
    hostname: z.string(),
    port: z.number().int().min(1).max(65535).optional(),
  }),
  auth: z.object({
    secretRef: z.string().optional(),
    secret: SecretReferenceSchema.optional(),
  }),
  exposes: z.array(RemoteAccessTunnelExposeSchema).optional(),
});
export type RemoteAccessTunnelSpec = z.infer<typeof RemoteAccessTunnelSpecSchema>;

export const RemoteAccessTunnelStatusSchema = z
  .object({
    phase: z.enum(['Pending', 'Connected', 'Disconnected', 'Failed']),
    connectedAt: z.string().datetime({ offset: true }),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type RemoteAccessTunnelStatus = z.infer<typeof RemoteAccessTunnelStatusSchema>;

export const RemoteAccessTunnelSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('RemoteAccessTunnel'),
  metadata: ObjectMetaSchema,
  spec: RemoteAccessTunnelSpecSchema,
  status: RemoteAccessTunnelStatusSchema.optional(),
});
export type RemoteAccessTunnel = z.infer<typeof RemoteAccessTunnelSchema>;
