import { z } from 'zod';
import { ConditionSchema } from '../common/condition.js';
import { ApiVersionSchema } from '../common/enums.js';
import { ObjectMetaSchema } from '../common/metadata.js';
import { ResourceReferenceSchema } from '../common/references.js';

export const CustomDomainTlsProviderSchema = z.enum(['letsencrypt', 'internal', 'upload']);
export type CustomDomainTlsProvider = z.infer<typeof CustomDomainTlsProviderSchema>;

export const CustomDomainSpecSchema = z.object({
  hostname: z.string(),
  target: ResourceReferenceSchema,
  tls: z.object({
    provider: CustomDomainTlsProviderSchema,
    certificate: z.string().optional(),
  }),
});
export type CustomDomainSpec = z.infer<typeof CustomDomainSpecSchema>;

export const CustomDomainStatusSchema = z
  .object({
    phase: z.enum(['Pending', 'Active', 'Failed']),
    certificateStatus: z.enum(['Pending', 'Issued', 'Expired', 'Failed']),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type CustomDomainStatus = z.infer<typeof CustomDomainStatusSchema>;

export const CustomDomainSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('CustomDomain'),
  metadata: ObjectMetaSchema,
  spec: CustomDomainSpecSchema,
  status: CustomDomainStatusSchema.optional(),
});
export type CustomDomain = z.infer<typeof CustomDomainSchema>;
