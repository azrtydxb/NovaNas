import { z } from 'zod';
import { ConditionSchema } from '../common/condition';
import { ApiVersionSchema } from '../common/enums';
import { ObjectMetaSchema } from '../common/metadata';
import { SecretReferenceSchema } from '../common/references';

export const SmbAdJoinSchema = z.object({
  realm: z.string(),
  workgroup: z.string().optional(),
  organizationalUnit: z.string().optional(),
  authSecret: SecretReferenceSchema,
});
export type SmbAdJoin = z.infer<typeof SmbAdJoinSchema>;

export const SmbServerSpecSchema = z.object({
  bindInterface: z.string().optional(),
  minProtocol: z.enum(['SMB2', 'SMB3']).optional(),
  maxProtocol: z.enum(['SMB2', 'SMB3']).optional(),
  signing: z.enum(['auto', 'required', 'disabled']).optional(),
  encryption: z.enum(['auto', 'required', 'disabled']).optional(),
  adJoin: SmbAdJoinSchema.optional(),
  ldap: z
    .object({
      uri: z.string(),
      baseDn: z.string(),
      bindSecret: SecretReferenceSchema.optional(),
    })
    .optional(),
});
export type SmbServerSpec = z.infer<typeof SmbServerSpecSchema>;

export const SmbServerStatusSchema = z
  .object({
    phase: z.enum(['Pending', 'Running', 'Failed']),
    joinedRealm: z.string(),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type SmbServerStatus = z.infer<typeof SmbServerStatusSchema>;

export const SmbServerSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('SmbServer'),
  metadata: ObjectMetaSchema,
  spec: SmbServerSpecSchema,
  status: SmbServerStatusSchema.optional(),
});
export type SmbServer = z.infer<typeof SmbServerSchema>;
