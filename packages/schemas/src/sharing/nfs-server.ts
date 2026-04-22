import { z } from 'zod';
import { ConditionSchema } from '../common/condition';
import { ApiVersionSchema } from '../common/enums';
import { ObjectMetaSchema } from '../common/metadata';

export const NfsServerSpecSchema = z.object({
  bindInterface: z.string().optional(),
  versions: z.array(z.enum(['3', '4', '4.1', '4.2'])).optional(),
  grace: z.number().int().positive().optional(),
  threads: z.number().int().positive().optional(),
  krb5: z
    .object({
      enabled: z.boolean(),
      realm: z.string().optional(),
      keytabSecret: z.object({ secretName: z.string(), key: z.string().optional() }).optional(),
    })
    .optional(),
});
export type NfsServerSpec = z.infer<typeof NfsServerSpecSchema>;

export const NfsServerStatusSchema = z
  .object({
    phase: z.enum(['Pending', 'Running', 'Failed']),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type NfsServerStatus = z.infer<typeof NfsServerStatusSchema>;

export const NfsServerSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('NfsServer'),
  metadata: ObjectMetaSchema,
  spec: NfsServerSpecSchema,
  status: NfsServerStatusSchema.optional(),
});
export type NfsServer = z.infer<typeof NfsServerSchema>;
