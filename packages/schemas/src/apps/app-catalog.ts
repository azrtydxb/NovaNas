import { z } from 'zod';
import { ConditionSchema } from '../common/condition';
import { ApiVersionSchema } from '../common/enums';
import { ObjectMetaSchema } from '../common/metadata';

export const AppCatalogSourceTypeSchema = z.enum(['git', 'helm', 'oci', 'custom']);
export type AppCatalogSourceType = z.infer<typeof AppCatalogSourceTypeSchema>;

export const AppCatalogSourceSchema = z.object({
  type: AppCatalogSourceTypeSchema,
  url: z.string(),
  branch: z.string().optional(),
  path: z.string().optional(),
  refreshInterval: z.string().optional(),
});
export type AppCatalogSource = z.infer<typeof AppCatalogSourceSchema>;

export const AppCatalogTrustSchema = z.object({
  signedBy: z.string().optional(),
  required: z.boolean().optional(),
});
export type AppCatalogTrust = z.infer<typeof AppCatalogTrustSchema>;

export const AppCatalogSpecSchema = z.object({
  source: AppCatalogSourceSchema,
  trust: AppCatalogTrustSchema.optional(),
});
export type AppCatalogSpec = z.infer<typeof AppCatalogSpecSchema>;

export const AppCatalogStatusSchema = z
  .object({
    phase: z.enum(['Pending', 'Synced', 'Failed']),
    lastSync: z.string().datetime({ offset: true }),
    appCount: z.number().int().nonnegative(),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type AppCatalogStatus = z.infer<typeof AppCatalogStatusSchema>;

export const AppCatalogSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('AppCatalog'),
  metadata: ObjectMetaSchema,
  spec: AppCatalogSpecSchema,
  status: AppCatalogStatusSchema.optional(),
});
export type AppCatalog = z.infer<typeof AppCatalogSchema>;
