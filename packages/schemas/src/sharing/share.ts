import { z } from 'zod';
import { ConditionSchema } from '../common/condition.js';
import { AccessModeSchema, ApiVersionSchema } from '../common/enums.js';
import { ObjectMetaSchema } from '../common/metadata.js';

export const SmbShareConfigSchema = z.object({
  server: z.string(),
  shadowCopies: z.boolean().optional(),
  caseSensitive: z.boolean().optional(),
  browseable: z.boolean().optional(),
  guestOk: z.boolean().optional(),
  readOnly: z.boolean().optional(),
});
export type SmbShareConfig = z.infer<typeof SmbShareConfigSchema>;

export const NfsSquashSchema = z.enum(['noRootSquash', 'rootSquash', 'allSquash']);
export type NfsSquash = z.infer<typeof NfsSquashSchema>;

export const NfsShareConfigSchema = z.object({
  server: z.string(),
  squash: NfsSquashSchema.optional(),
  allowedNetworks: z.array(z.string()).optional(),
  readOnly: z.boolean().optional(),
  sync: z.boolean().optional(),
});
export type NfsShareConfig = z.infer<typeof NfsShareConfigSchema>;

export const SharePrincipalSchema = z.object({
  user: z.string().optional(),
  group: z.string().optional(),
});
export type SharePrincipal = z.infer<typeof SharePrincipalSchema>;

export const ShareAccessEntrySchema = z.object({
  principal: SharePrincipalSchema,
  mode: AccessModeSchema,
});
export type ShareAccessEntry = z.infer<typeof ShareAccessEntrySchema>;

export const ShareSpecSchema = z.object({
  dataset: z.string(),
  path: z.string(),
  protocols: z.object({
    smb: SmbShareConfigSchema.optional(),
    nfs: NfsShareConfigSchema.optional(),
  }),
  access: z.array(ShareAccessEntrySchema).optional(),
});
export type ShareSpec = z.infer<typeof ShareSpecSchema>;

export const ShareStatusSchema = z
  .object({
    phase: z.enum(['Pending', 'Active', 'Failed']),
    exportedAt: z.string().datetime({ offset: true }),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type ShareStatus = z.infer<typeof ShareStatusSchema>;

export const ShareSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('Share'),
  metadata: ObjectMetaSchema,
  spec: ShareSpecSchema,
  status: ShareStatusSchema.optional(),
});
export type Share = z.infer<typeof ShareSchema>;
