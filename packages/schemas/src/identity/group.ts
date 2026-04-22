import { z } from 'zod';
import { ConditionSchema } from '../common/condition';
import { ApiVersionSchema } from '../common/enums';
import { ObjectMetaSchema } from '../common/metadata';

export const GroupSpecSchema = z.object({
  name: z.string(),
  displayName: z.string().optional(),
  members: z.array(z.string()).optional(),
  realm: z.string().optional(),
  federated: z.boolean().optional(),
  gid: z.number().int().nonnegative().optional(),
});
export type GroupSpec = z.infer<typeof GroupSpecSchema>;

export const GroupStatusSchema = z
  .object({
    phase: z.enum(['Pending', 'Active', 'Failed']),
    keycloakId: z.string(),
    memberCount: z.number().int().nonnegative(),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type GroupStatus = z.infer<typeof GroupStatusSchema>;

export const GroupSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('Group'),
  metadata: ObjectMetaSchema,
  spec: GroupSpecSchema,
  status: GroupStatusSchema.optional(),
});
export type Group = z.infer<typeof GroupSchema>;
