import { z } from 'zod';
import { ConditionSchema } from '../common/condition.js';
import { ApiVersionSchema } from '../common/enums.js';
import { ObjectMetaSchema } from '../common/metadata.js';

export const HostInterfaceUsageSchema = z.enum([
  'management',
  'storage',
  'cluster',
  'vmBridge',
  'appIngress',
]);
export type HostInterfaceUsage = z.infer<typeof HostInterfaceUsageSchema>;

export const HostInterfaceAddressSchema = z.object({
  cidr: z.string(),
  type: z.enum(['static', 'dhcp', 'slaac']),
});
export type HostInterfaceAddress = z.infer<typeof HostInterfaceAddressSchema>;

export const HostInterfaceSpecSchema = z.object({
  backing: z.string(),
  addresses: z.array(HostInterfaceAddressSchema).optional(),
  gateway: z.string().optional(),
  dns: z.array(z.string()).optional(),
  mtu: z.number().int().positive().optional(),
  usage: z.array(HostInterfaceUsageSchema),
});
export type HostInterfaceSpec = z.infer<typeof HostInterfaceSpecSchema>;

export const HostInterfaceStatusSchema = z
  .object({
    phase: z.enum(['Pending', 'Active', 'Failed']),
    effectiveAddresses: z.array(z.string()),
    link: z.enum(['up', 'down']),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type HostInterfaceStatus = z.infer<typeof HostInterfaceStatusSchema>;

export const HostInterfaceSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('HostInterface'),
  metadata: ObjectMetaSchema,
  spec: HostInterfaceSpecSchema,
  status: HostInterfaceStatusSchema.optional(),
});
export type HostInterface = z.infer<typeof HostInterfaceSchema>;
