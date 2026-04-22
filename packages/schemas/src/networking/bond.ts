import { z } from 'zod';
import { ConditionSchema } from '../common/condition';
import { ApiVersionSchema } from '../common/enums';
import { ObjectMetaSchema } from '../common/metadata';

export const BondModeSchema = z.enum([
  'active-backup',
  'balance-alb',
  'balance-tlb',
  '802.3ad',
  'balance-rr',
  'balance-xor',
  'broadcast',
]);
export type BondMode = z.infer<typeof BondModeSchema>;

export const BondLacpSchema = z.object({
  rate: z.enum(['slow', 'fast']).optional(),
  aggregatorSelect: z.enum(['stable', 'bandwidth', 'count']).optional(),
});
export type BondLacp = z.infer<typeof BondLacpSchema>;

export const BondSpecSchema = z.object({
  interfaces: z.array(z.string()).min(1),
  mode: BondModeSchema,
  lacp: BondLacpSchema.optional(),
  xmitHashPolicy: z.enum(['layer2', 'layer2+3', 'layer3+4', 'encap2+3', 'encap3+4']).optional(),
  mtu: z.number().int().positive().optional(),
  miimon: z.number().int().nonnegative().optional(),
});
export type BondSpec = z.infer<typeof BondSpecSchema>;

export const BondStatusSchema = z
  .object({
    phase: z.enum(['Pending', 'Active', 'Degraded', 'Failed']),
    activeSlaves: z.array(z.string()),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type BondStatus = z.infer<typeof BondStatusSchema>;

export const BondSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('Bond'),
  metadata: ObjectMetaSchema,
  spec: BondSpecSchema,
  status: BondStatusSchema.optional(),
});
export type Bond = z.infer<typeof BondSchema>;
