import { z } from 'zod';

/**
 * Kubernetes-style bytes quantity string.
 * Examples: "100Gi", "4Ti", "500Mi", "1024".
 * Binary (power-of-two) suffixes only: Ki, Mi, Gi, Ti, Pi, Ei.
 */
export const BytesQuantitySchema = z
  .string()
  .regex(/^\d+(\.\d+)?(Ki|Mi|Gi|Ti|Pi|Ei)?$/, 'invalid bytes quantity');
export type BytesQuantity = z.infer<typeof BytesQuantitySchema>;

/**
 * Kubernetes-style CPU quantity string.
 * Examples: "500m", "1", "2", "0.5".
 */
export const CpuQuantitySchema = z.string().regex(/^\d+(\.\d+)?m?$/, 'invalid cpu quantity');
export type CpuQuantity = z.infer<typeof CpuQuantitySchema>;

/**
 * Duration string. Examples: "30s", "15m", "1h", "7d", "30d".
 */
export const DurationSchema = z
  .string()
  .regex(/^\d+(\.\d+)?(ns|us|ms|s|m|h|d|w)$/, 'invalid duration');
export type Duration = z.infer<typeof DurationSchema>;

/**
 * Cron expression. Five-field POSIX cron.
 */
export const CronSchema = z
  .string()
  .regex(/^(\S+\s+){4}\S+$/, 'invalid cron expression (expected 5 whitespace-separated fields)');
export type Cron = z.infer<typeof CronSchema>;

/**
 * Bandwidth rate string. Examples: "100Mbps", "1Gbps", "500Kbps".
 */
export const BandwidthSchema = z
  .string()
  .regex(/^\d+(\.\d+)?(Kbps|Mbps|Gbps|Tbps|Kb\/s|Mb\/s|Gb\/s)$/, 'invalid bandwidth');
export type Bandwidth = z.infer<typeof BandwidthSchema>;

/**
 * RFC3339 timestamp string.
 */
export const Rfc3339Schema = z.string().datetime({ offset: true });
export type Rfc3339 = z.infer<typeof Rfc3339Schema>;
