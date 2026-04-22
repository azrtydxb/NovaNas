import { z } from 'zod';
import { ApiVersionSchema } from '../common/enums.js';
import { ObjectMetaSchema } from '../common/metadata.js';

export const PhysicalInterfaceStatusSchema = z
  .object({
    macAddress: z.string(),
    speedMbps: z.number().int().nonnegative(),
    duplex: z.enum(['full', 'half', 'unknown']),
    link: z.enum(['up', 'down']),
    driver: z.string(),
    pcieSlot: z.string(),
    capabilities: z.array(z.string()),
    usedBy: z.string(),
  })
  .partial();
export type PhysicalInterfaceStatus = z.infer<typeof PhysicalInterfaceStatusSchema>;

export const PhysicalInterfaceSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('PhysicalInterface'),
  metadata: ObjectMetaSchema,
  // observed resource — no spec fields in design
  spec: z.object({}).optional(),
  status: PhysicalInterfaceStatusSchema.optional(),
});
export type PhysicalInterface = z.infer<typeof PhysicalInterfaceSchema>;
