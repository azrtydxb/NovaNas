import { z } from 'zod';
import { ConditionSchema } from '../common/condition';
import { ApiVersionSchema } from '../common/enums';
import { ObjectMetaSchema } from '../common/metadata';
import { VolumeSourceRefSchema } from '../common/references';

export const SnapshotSpecSchema = z.object({
  source: VolumeSourceRefSchema,
  locked: z.boolean().optional(),
  retainUntil: z.string().datetime({ offset: true }).optional(),
  labels: z.record(z.string(), z.string()).optional(),
});
export type SnapshotSpec = z.infer<typeof SnapshotSpecSchema>;

export const SnapshotStatusSchema = z
  .object({
    phase: z.enum(['Pending', 'Ready', 'Failed', 'Deleted']),
    sizeBytes: z.number().int().nonnegative(),
    createdAt: z.string().datetime({ offset: true }),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type SnapshotStatus = z.infer<typeof SnapshotStatusSchema>;

export const SnapshotSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('Snapshot'),
  metadata: ObjectMetaSchema,
  spec: SnapshotSpecSchema,
  status: SnapshotStatusSchema.optional(),
});
export type Snapshot = z.infer<typeof SnapshotSchema>;
