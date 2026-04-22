import { z } from 'zod';

export const API_VERSION = 'novanas.io/v1alpha1' as const;
export const ApiVersionSchema = z.literal(API_VERSION);

/**
 * Pool tier. A user-defined label used by tiering policies.
 */
export const PoolTierSchema = z.enum(['hot', 'warm', 'cold', 'fast', 'capacity', 'archive']);
export type PoolTier = z.infer<typeof PoolTierSchema>;

/**
 * Device class hint for deviceFilter.
 */
export const DeviceClassSchema = z.enum(['nvme', 'ssd', 'hdd']);
export type DeviceClass = z.infer<typeof DeviceClassSchema>;

export const RecoveryRateSchema = z.enum(['aggressive', 'balanced', 'gentle']);
export type RecoveryRate = z.infer<typeof RecoveryRateSchema>;

export const RebalanceOnAddSchema = z.enum(['manual', 'immediate', 'later']);
export type RebalanceOnAdd = z.infer<typeof RebalanceOnAddSchema>;

export const FilesystemTypeSchema = z.enum(['xfs', 'ext4']);
export type FilesystemType = z.infer<typeof FilesystemTypeSchema>;

export const AclModeSchema = z.enum(['posix', 'nfsv4']);
export type AclMode = z.infer<typeof AclModeSchema>;

export const CompressionSchema = z.enum(['none', 'zstd', 'lz4', 'gzip']);
export type Compression = z.infer<typeof CompressionSchema>;

export const ProtectionModeSchema = z.enum(['replication', 'erasureCoding']);
export type ProtectionMode = z.infer<typeof ProtectionModeSchema>;

export const AccessModeSchema = z.enum(['rw', 'ro', 'none']);
export type AccessMode = z.infer<typeof AccessModeSchema>;

export const ExposureModeSchema = z.enum(['mdns', 'lan', 'reverseProxy', 'internet']);
export type ExposureMode = z.infer<typeof ExposureModeSchema>;

export const VersioningSchema = z.enum(['enabled', 'suspended', 'disabled']);
export type Versioning = z.infer<typeof VersioningSchema>;

export const ObjectLockModeSchema = z.enum(['governance', 'compliance']);
export type ObjectLockMode = z.infer<typeof ObjectLockModeSchema>;

export const SseModeSchema = z.enum(['AES256', 'aws:kms', 'sse-c']);
export type SseMode = z.infer<typeof SseModeSchema>;
