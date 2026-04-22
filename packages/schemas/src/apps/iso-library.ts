import { z } from 'zod';
import { ConditionSchema } from '../common/condition';
import { ApiVersionSchema } from '../common/enums';
import { ObjectMetaSchema } from '../common/metadata';

export const IsoSourceSchema = z.object({
  url: z.string().url(),
  sha256: z.string().optional(),
  name: z.string().optional(),
});
export type IsoSource = z.infer<typeof IsoSourceSchema>;

export const IsoLibrarySpecSchema = z.object({
  dataset: z.string(),
  sources: z.array(IsoSourceSchema).optional(),
});
export type IsoLibrarySpec = z.infer<typeof IsoLibrarySpecSchema>;

export const IsoLibraryEntrySchema = z.object({
  name: z.string(),
  sizeBytes: z.number().int().nonnegative(),
  sha256: z.string(),
  downloadedAt: z.string().datetime({ offset: true }),
});
export type IsoLibraryEntry = z.infer<typeof IsoLibraryEntrySchema>;

export const IsoLibraryStatusSchema = z
  .object({
    phase: z.enum(['Pending', 'Ready', 'Failed']),
    entries: z.array(IsoLibraryEntrySchema),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type IsoLibraryStatus = z.infer<typeof IsoLibraryStatusSchema>;

export const IsoLibrarySchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('IsoLibrary'),
  metadata: ObjectMetaSchema,
  spec: IsoLibrarySpecSchema,
  status: IsoLibraryStatusSchema.optional(),
});
export type IsoLibrary = z.infer<typeof IsoLibrarySchema>;
