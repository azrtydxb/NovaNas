import { z } from 'zod';
import { ConditionSchema } from '../common/condition.js';
import { ApiVersionSchema } from '../common/enums.js';
import { ObjectMetaSchema } from '../common/metadata.js';
import { ResourceReferenceSchema } from '../common/references.js';

export const GpuDeviceSpecSchema = z.object({
  passthrough: z.boolean().optional(),
});
export type GpuDeviceSpec = z.infer<typeof GpuDeviceSpecSchema>;

export const GpuDeviceStatusSchema = z
  .object({
    vendor: z.string(),
    model: z.string(),
    pciAddress: z.string(),
    deviceId: z.string(),
    driver: z.string(),
    vfioBound: z.boolean(),
    iommuGroup: z.number().int().nonnegative(),
    assignedTo: ResourceReferenceSchema,
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type GpuDeviceStatus = z.infer<typeof GpuDeviceStatusSchema>;

export const GpuDeviceSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('GpuDevice'),
  metadata: ObjectMetaSchema,
  spec: GpuDeviceSpecSchema,
  status: GpuDeviceStatusSchema.optional(),
});
export type GpuDevice = z.infer<typeof GpuDeviceSchema>;
