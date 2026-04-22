import { z } from 'zod';
import { ConditionSchema } from '../common/condition.js';
import { ApiVersionSchema } from '../common/enums.js';
import { ObjectMetaSchema } from '../common/metadata.js';

export const VlanSpecSchema = z.object({
  parent: z.string(),
  vlanId: z.number().int().min(1).max(4094),
  mtu: z.number().int().positive().optional(),
});
export type VlanSpec = z.infer<typeof VlanSpecSchema>;

export const VlanStatusSchema = z
  .object({
    phase: z.enum(['Pending', 'Active', 'Failed']),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type VlanStatus = z.infer<typeof VlanStatusSchema>;

export const VlanSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('Vlan'),
  metadata: ObjectMetaSchema,
  spec: VlanSpecSchema,
  status: VlanStatusSchema.optional(),
});
export type Vlan = z.infer<typeof VlanSchema>;
