import { z } from 'zod';
import { ApiVersionSchema } from '../common/enums.js';
import { ObjectMetaSchema } from '../common/metadata.js';

export const AppChartRefSchema = z.object({
  ociRef: z.string().optional(),
  helmRepo: z.string().optional(),
  name: z.string().optional(),
  version: z.string().optional(),
  digest: z.string().optional(),
});
export type AppChartRef = z.infer<typeof AppChartRefSchema>;

export const AppRequirementsSchema = z.object({
  minRamMB: z.number().int().nonnegative().optional(),
  minCpu: z.number().nonnegative().optional(),
  requiresGpu: z.boolean().optional(),
  ports: z.array(z.number().int().min(1).max(65535)).optional(),
  privileged: z.boolean().optional(),
});
export type AppRequirements = z.infer<typeof AppRequirementsSchema>;

export const AppSpecSchema = z.object({
  displayName: z.string(),
  version: z.string(),
  icon: z.string().optional(),
  description: z.string().optional(),
  schema: z.unknown().optional(),
  chart: AppChartRefSchema,
  requirements: AppRequirementsSchema.optional(),
  category: z.string().optional(),
  homepage: z.string().optional(),
  maintainers: z.array(z.string()).optional(),
});
export type AppSpec = z.infer<typeof AppSpecSchema>;

export const AppSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('App'),
  metadata: ObjectMetaSchema,
  spec: AppSpecSchema,
});
export type App = z.infer<typeof AppSchema>;
