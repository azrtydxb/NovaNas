import { z } from 'zod';
import { ConditionSchema } from '../common/condition';
import { ApiVersionSchema } from '../common/enums';
import { ObjectMetaSchema } from '../common/metadata';

export const NvmeofTransportSchema = z.enum(['tcp', 'rdma']);
export type NvmeofTransport = z.infer<typeof NvmeofTransportSchema>;

export const NvmeofTargetSpecSchema = z.object({
  blockVolume: z.string(),
  subsystemNqn: z.string().optional(),
  transport: NvmeofTransportSchema.optional(),
  listen: z.object({
    hostInterface: z.string(),
    port: z.number().int().min(1).max(65535).optional(),
  }),
  allowedHostNqns: z.array(z.string()).optional(),
});
export type NvmeofTargetSpec = z.infer<typeof NvmeofTargetSpecSchema>;

export const NvmeofTargetStatusSchema = z
  .object({
    phase: z.enum(['Pending', 'Active', 'Failed']),
    subsystemNqn: z.string(),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type NvmeofTargetStatus = z.infer<typeof NvmeofTargetStatusSchema>;

export const NvmeofTargetSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('NvmeofTarget'),
  metadata: ObjectMetaSchema,
  spec: NvmeofTargetSpecSchema,
  status: NvmeofTargetStatusSchema.optional(),
});
export type NvmeofTarget = z.infer<typeof NvmeofTargetSchema>;
