import { z } from 'zod';
import { ConditionSchema } from '../common/condition.js';
import { ApiVersionSchema } from '../common/enums.js';
import { ObjectMetaSchema } from '../common/metadata.js';
import { BytesQuantitySchema } from '../common/quantity.js';

export const VmOsTypeSchema = z.enum(['linux', 'windows', 'other']);
export type VmOsType = z.infer<typeof VmOsTypeSchema>;

export const VmOsSchema = z.object({
  type: VmOsTypeSchema,
  variant: z.string().optional(),
});
export type VmOs = z.infer<typeof VmOsSchema>;

export const VmResourcesSchema = z.object({
  cpu: z.number().int().positive(),
  memoryMiB: z.number().int().positive(),
  sockets: z.number().int().positive().optional(),
  cores: z.number().int().positive().optional(),
  threads: z.number().int().positive().optional(),
});
export type VmResources = z.infer<typeof VmResourcesSchema>;

export const VmDiskBusSchema = z.enum(['virtio', 'scsi', 'sata', 'ide']);
export type VmDiskBus = z.infer<typeof VmDiskBusSchema>;

export const VmDiskSourceSchema = z.discriminatedUnion('type', [
  z.object({
    type: z.literal('dataset'),
    dataset: z.string(),
    size: BytesQuantitySchema.optional(),
  }),
  z.object({
    type: z.literal('blockVolume'),
    blockVolume: z.string(),
  }),
  z.object({
    type: z.literal('iso'),
    isoLibrary: z.string(),
  }),
  z.object({
    type: z.literal('clone'),
    sourceVm: z.string(),
    sourceDisk: z.string(),
  }),
]);
export type VmDiskSource = z.infer<typeof VmDiskSourceSchema>;

export const VmDiskSchema = z.object({
  name: z.string(),
  source: VmDiskSourceSchema,
  bus: VmDiskBusSchema.optional(),
  boot: z.number().int().positive().optional(),
  readOnly: z.boolean().optional(),
});
export type VmDisk = z.infer<typeof VmDiskSchema>;

export const VmCdromSchema = z.object({
  name: z.string(),
  source: z.object({
    type: z.literal('iso'),
    isoLibrary: z.string(),
  }),
});
export type VmCdrom = z.infer<typeof VmCdromSchema>;

export const VmNetworkSchema = z.object({
  type: z.enum(['bridge', 'pod', 'masquerade']),
  bridge: z.string().optional(),
  mac: z.string().optional(),
  model: z.enum(['virtio', 'e1000', 'rtl8139']).optional(),
});
export type VmNetwork = z.infer<typeof VmNetworkSchema>;

export const VmGpuPassthroughEntrySchema = z.object({
  vendor: z.string().optional(),
  device: z.string(),
  deviceName: z.string().optional(),
});
export type VmGpuPassthroughEntry = z.infer<typeof VmGpuPassthroughEntrySchema>;

export const VmGpuSchema = z.object({
  passthrough: z.array(VmGpuPassthroughEntrySchema).optional(),
});
export type VmGpu = z.infer<typeof VmGpuSchema>;

export const VmGraphicsSchema = z.object({
  enabled: z.boolean(),
  type: z.enum(['spice', 'vnc']).optional(),
});
export type VmGraphics = z.infer<typeof VmGraphicsSchema>;

export const VmAutostartSchema = z.enum(['never', 'onBoot', 'always']);
export type VmAutostart = z.infer<typeof VmAutostartSchema>;

export const VmPowerStateSchema = z.enum(['Running', 'Stopped', 'Paused']);
export type VmPowerState = z.infer<typeof VmPowerStateSchema>;

export const VmSpecSchema = z.object({
  owner: z.string().optional(),
  os: VmOsSchema,
  resources: VmResourcesSchema,
  disks: z.array(VmDiskSchema).optional(),
  cdrom: z.array(VmCdromSchema).optional(),
  network: z.array(VmNetworkSchema).optional(),
  gpu: VmGpuSchema.optional(),
  graphics: VmGraphicsSchema.optional(),
  autostart: VmAutostartSchema.optional(),
  powerState: VmPowerStateSchema.optional(),
});
export type VmSpec = z.infer<typeof VmSpecSchema>;

export const VmStatusSchema = z
  .object({
    phase: z.enum(['Pending', 'Running', 'Stopped', 'Paused', 'Failed']),
    consoleUrl: z.string(),
    ip: z.string(),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type VmStatus = z.infer<typeof VmStatusSchema>;

export const VmSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('Vm'),
  metadata: ObjectMetaSchema,
  spec: VmSpecSchema,
  status: VmStatusSchema.optional(),
});
export type Vm = z.infer<typeof VmSchema>;
