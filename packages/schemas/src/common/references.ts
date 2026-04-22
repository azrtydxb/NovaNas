import { z } from 'zod';

/**
 * Reference to a Kubernetes Secret (or OpenBao path).
 * Either a classic secretName + key, or an OpenBao URI like
 * "openbao://novanas/replication/offsite-token".
 */
export const SecretReferenceSchema = z.union([
  z.object({
    secretName: z.string(),
    key: z.string().optional(),
    namespace: z.string().optional(),
  }),
  z.object({
    secretRef: z.string().regex(/^(openbao|vault):\/\/.+/, 'invalid secret URI'),
  }),
]);
export type SecretReference = z.infer<typeof SecretReferenceSchema>;

/**
 * Generic cross-kind resource reference.
 */
export const ResourceReferenceSchema = z.object({
  apiVersion: z.string().optional(),
  kind: z.string(),
  name: z.string(),
  namespace: z.string().optional(),
});
export type ResourceReference = z.infer<typeof ResourceReferenceSchema>;

/**
 * Discriminated reference to anything that can be a volume-like source of
 * snapshots, replication, or backup.
 */
export const VolumeSourceRefSchema = z.discriminatedUnion('kind', [
  z.object({
    kind: z.literal('BlockVolume'),
    name: z.string(),
    namespace: z.string().optional(),
  }),
  z.object({
    kind: z.literal('Dataset'),
    name: z.string(),
    namespace: z.string().optional(),
  }),
  z.object({
    kind: z.literal('Bucket'),
    name: z.string(),
    namespace: z.string().optional(),
  }),
  z.object({
    kind: z.literal('AppInstance'),
    name: z.string(),
    namespace: z.string(),
  }),
  z.object({
    kind: z.literal('Vm'),
    name: z.string(),
    namespace: z.string(),
  }),
]);
export type VolumeSourceRef = z.infer<typeof VolumeSourceRefSchema>;
