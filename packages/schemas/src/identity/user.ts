import { z } from 'zod';
import { ConditionSchema } from '../common/condition';
import { ApiVersionSchema } from '../common/enums';
import { ObjectMetaSchema } from '../common/metadata';

export const UserSpecSchema = z.object({
  username: z.string(),
  email: z.string().email().optional(),
  displayName: z.string().optional(),
  groups: z.array(z.string()).optional(),
  admin: z.boolean().optional(),
  enabled: z.boolean().optional(),
  realm: z.string().optional(),
  federated: z.boolean().optional(),
  uid: z.number().int().nonnegative().optional(),
  primaryGid: z.number().int().nonnegative().optional(),
  homeDataset: z.string().optional(),
  shell: z.string().optional(),
});
export type UserSpec = z.infer<typeof UserSpecSchema>;

export const UserStatusSchema = z
  .object({
    phase: z.enum(['Pending', 'Active', 'Disabled', 'Failed']),
    keycloakId: z.string(),
    lastLogin: z.string().datetime({ offset: true }),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type UserStatus = z.infer<typeof UserStatusSchema>;

export const UserSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('User'),
  metadata: ObjectMetaSchema,
  spec: UserSpecSchema,
  status: UserStatusSchema.optional(),
});
export type User = z.infer<typeof UserSchema>;
