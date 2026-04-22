import { z } from 'zod';
import { ConditionSchema } from '../common/condition.js';
import { ApiVersionSchema } from '../common/enums.js';
import { ObjectMetaSchema } from '../common/metadata.js';

export const VipPoolAnnounceSchema = z.enum(['arp', 'bgp', 'ndp']);
export type VipPoolAnnounce = z.infer<typeof VipPoolAnnounceSchema>;

export const VipPoolSpecSchema = z.object({
  range: z.string(),
  interface: z.string(),
  announce: VipPoolAnnounceSchema.optional(),
});
export type VipPoolSpec = z.infer<typeof VipPoolSpecSchema>;

export const VipPoolStatusSchema = z
  .object({
    phase: z.enum(['Active', 'Failed']),
    allocated: z.number().int().nonnegative(),
    available: z.number().int().nonnegative(),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type VipPoolStatus = z.infer<typeof VipPoolStatusSchema>;

export const VipPoolSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('VipPool'),
  metadata: ObjectMetaSchema,
  spec: VipPoolSpecSchema,
  status: VipPoolStatusSchema.optional(),
});
export type VipPool = z.infer<typeof VipPoolSchema>;
