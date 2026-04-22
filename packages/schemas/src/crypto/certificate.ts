import { z } from 'zod';
import { ConditionSchema } from '../common/condition.js';
import { ApiVersionSchema } from '../common/enums.js';
import { ObjectMetaSchema } from '../common/metadata.js';
import { SecretReferenceSchema } from '../common/references.js';

export const CertificateProviderSchema = z.enum(['acme', 'internalPki', 'upload']);
export type CertificateProvider = z.infer<typeof CertificateProviderSchema>;

export const CertificateSpecSchema = z.object({
  provider: CertificateProviderSchema,
  commonName: z.string(),
  dnsNames: z.array(z.string()).optional(),
  ipAddresses: z.array(z.string()).optional(),
  acme: z
    .object({
      issuer: z.enum(['letsencrypt', 'letsencrypt-staging', 'zerossl', 'custom']),
      email: z.string().email().optional(),
      directoryUrl: z.string().url().optional(),
    })
    .optional(),
  upload: z
    .object({
      certSecret: SecretReferenceSchema,
      keySecret: SecretReferenceSchema,
    })
    .optional(),
  renewBeforeDays: z.number().int().positive().optional(),
});
export type CertificateSpec = z.infer<typeof CertificateSpecSchema>;

export const CertificateStatusSchema = z
  .object({
    phase: z.enum(['Pending', 'Issued', 'Renewing', 'Expired', 'Failed']),
    serialNumber: z.string(),
    issuer: z.string(),
    notBefore: z.string().datetime({ offset: true }),
    notAfter: z.string().datetime({ offset: true }),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type CertificateStatus = z.infer<typeof CertificateStatusSchema>;

export const CertificateSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('Certificate'),
  metadata: ObjectMetaSchema,
  spec: CertificateSpecSchema,
  status: CertificateStatusSchema.optional(),
});
export type Certificate = z.infer<typeof CertificateSchema>;
