import { z } from 'zod';
import { ConditionSchema } from '../common/condition.js';
import { ApiVersionSchema } from '../common/enums.js';
import { ObjectMetaSchema } from '../common/metadata.js';
import { SecretReferenceSchema } from '../common/references.js';

export const RealmFederationTypeSchema = z.enum(['activeDirectory', 'ldap', 'oidc']);
export type RealmFederationType = z.infer<typeof RealmFederationTypeSchema>;

export const RealmFederationSchema = z.object({
  type: RealmFederationTypeSchema,
  displayName: z.string().optional(),
  connection: z.object({
    url: z.string(),
    baseDn: z.string().optional(),
    usersDn: z.string().optional(),
    groupsDn: z.string().optional(),
    bindDn: z.string().optional(),
    bindSecret: SecretReferenceSchema.optional(),
    startTls: z.boolean().optional(),
  }),
  syncPeriod: z.string().optional(),
});
export type RealmFederation = z.infer<typeof RealmFederationSchema>;

export const KeycloakRealmSpecSchema = z.object({
  displayName: z.string().optional(),
  defaultLocale: z.string().optional(),
  federations: z.array(RealmFederationSchema).optional(),
  mfa: z
    .object({
      totp: z.boolean().optional(),
      webauthn: z.boolean().optional(),
      required: z.boolean().optional(),
    })
    .optional(),
  passwordPolicy: z.string().optional(),
});
export type KeycloakRealmSpec = z.infer<typeof KeycloakRealmSpecSchema>;

export const KeycloakRealmStatusSchema = z
  .object({
    phase: z.enum(['Pending', 'Active', 'Failed']),
    userCount: z.number().int().nonnegative(),
    groupCount: z.number().int().nonnegative(),
    lastSync: z.string().datetime({ offset: true }),
    conditions: z.array(ConditionSchema),
  })
  .partial();
export type KeycloakRealmStatus = z.infer<typeof KeycloakRealmStatusSchema>;

export const KeycloakRealmSchema = z.object({
  apiVersion: ApiVersionSchema,
  kind: z.literal('KeycloakRealm'),
  metadata: ObjectMetaSchema,
  spec: KeycloakRealmSpecSchema,
  status: KeycloakRealmStatusSchema.optional(),
});
export type KeycloakRealm = z.infer<typeof KeycloakRealmSchema>;
