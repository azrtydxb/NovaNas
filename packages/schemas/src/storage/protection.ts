import { z } from 'zod';

/**
 * Replication protection: N copies.
 */
export const ReplicationProtectionSchema = z.object({
  mode: z.literal('replication'),
  replication: z.object({
    copies: z.number().int().min(1).max(8),
  }),
});
export type ReplicationProtection = z.infer<typeof ReplicationProtectionSchema>;

/**
 * Erasure coding protection: k+m shards.
 */
export const ErasureCodingProtectionSchema = z.object({
  mode: z.literal('erasureCoding'),
  erasureCoding: z.object({
    dataShards: z.number().int().min(2),
    parityShards: z.number().int().min(1),
  }),
});
export type ErasureCodingProtection = z.infer<typeof ErasureCodingProtectionSchema>;

/**
 * ProtectionPolicy is a discriminated union on `mode`. Matches the
 * examples in docs/05-crd-reference.md (erasureCoding, replication).
 */
export const ProtectionPolicySchema = z.discriminatedUnion('mode', [
  ReplicationProtectionSchema,
  ErasureCodingProtectionSchema,
]);
export type ProtectionPolicy = z.infer<typeof ProtectionPolicySchema>;
