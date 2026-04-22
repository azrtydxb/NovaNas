import { z } from 'zod';
import { ApiVersionSchema } from '../common/enums';
import { ObjectMetaSchema } from '../common/metadata';

export const DiskRoleSchema = z.enum(['data', 'spare']);
export type DiskRole = z.infer<typeof DiskRoleSchema>;

export const DiskStateSchema = z.enum([
  'UNKNOWN',
  'IDENTIFIED',
  'ASSIGNED',
  'ACTIVE',
  'DEGRADED',
  'FAILED',
  'DRAINING',
  'REMOVABLE',
  'QUARANTINED',
  'WIPED',
]);
export type DiskState = z.infer<typeof DiskStateSchema>;

export const DiskSmartStatusSchema = z.object({
  overallHealth: z.enum(['OK', 'WARN', 'FAIL']).optional(),
  temperature: z.number().optional(),
  powerOnHours: z.number().int().nonnegative().optional(),
  reallocatedSectors: z.number().int().nonnegative().optional(),
  pendingSectors: z.number().int().nonnegative().optional(),
  lastShortTest: z.string().datetime({ offset: true }).optional(),
  lastLongTest: z.string().datetime({ offset: true }).optional(),
});
export type DiskSmartStatus = z.infer<typeof DiskSmartStatusSchema>;

export const DiskLifecycleEventSchema = z.object({
  timestamp: z.string().datetime({ offset: true }),
  type: z.string(),
  reason: z.string().optional(),
  message: z.string().optional(),
  fromState: DiskStateSchema.optional(),
  toState: DiskStateSchema.optional(),
  actor: z.string().optional(),
});
export type DiskLifecycleEvent = z.infer<typeof DiskLifecycleEventSchema>;

export const DiskSpecSchema = z.object({
  pool: z.string().optional(),
  role: DiskRoleSchema.optional(),
});
export type DiskSpec = z.infer<typeof DiskSpecSchema>;

export const DiskStatusSchema = z
  .object({
    slot: z.string(),
    model: z.string(),
    serial: z.string(),
    wwn: z.string(),
    sizeBytes: z.number().int().nonnegative(),
    deviceClass: z.enum(['nvme', 'ssd', 'hdd']),
    smart: DiskSmartStatusSchema,
    state: DiskStateSchema,
    recentEvents: z.array(DiskLifecycleEventSchema),
  })
  .partial();
export type DiskStatus = z.infer<typeof DiskStatusSchema>;

export const DiskSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('Disk'),
  metadata: ObjectMetaSchema,
  spec: DiskSpecSchema,
  status: DiskStatusSchema.optional(),
});
export type Disk = z.infer<typeof DiskSchema>;
