import { z } from 'zod';
import { ConditionSchema } from '../common/condition';
import { ApiVersionSchema } from '../common/enums';
import { ObjectMetaSchema } from '../common/metadata';

export const OverlayTypeSchema = z.enum(['geneve', 'vxlan', 'none']);
export type OverlayType = z.infer<typeof OverlayTypeSchema>;

export const ClusterNetworkSpecSchema = z.object({
  podCidr: z.string(),
  serviceCidr: z.string(),
  overlay: z
    .object({
      type: OverlayTypeSchema,
      egressInterface: z.string().optional(),
    })
    .optional(),
  policy: z
    .object({
      defaultDeny: z.boolean().optional(),
    })
    .optional(),
  mtu: z.union([z.number().int().positive(), z.literal('auto')]).optional(),
});
export type ClusterNetworkSpec = z.infer<typeof ClusterNetworkSpecSchema>;

export const ClusterNetworkStatusSchema = z
  .object({
    phase: z.enum(['Pending', 'Active', 'Failed']),
    effectiveMtu: z.number().int().positive(),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type ClusterNetworkStatus = z.infer<typeof ClusterNetworkStatusSchema>;

export const ClusterNetworkSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('ClusterNetwork'),
  metadata: ObjectMetaSchema,
  spec: ClusterNetworkSpecSchema,
  status: ClusterNetworkStatusSchema.optional(),
});
export type ClusterNetwork = z.infer<typeof ClusterNetworkSchema>;
