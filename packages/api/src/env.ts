import { z } from 'zod';

/**
 * Environment variable schema for the NovaNas API server.
 * Parsed once at bootstrap; all other modules import the frozen `env` object.
 */
export const EnvSchema = z.object({
  NODE_ENV: z.enum(['development', 'test', 'production']).default('development'),
  PORT: z.coerce.number().int().positive().default(8080),
  LOG_LEVEL: z.enum(['trace', 'debug', 'info', 'warn', 'error', 'fatal']).default('info'),

  DATABASE_URL: z.string().url(),
  REDIS_URL: z.string().url(),

  KEYCLOAK_ISSUER_URL: z.string().url(),
  // Optional in-cluster Service URL for discovery. When set, we fetch
  // the well-known doc over plain HTTP to avoid Node/undici's flaky
  // NODE_EXTRA_CA_CERTS handling for ingress-served HTTPS (#49). Public
  // KEYCLOAK_ISSUER_URL is still used as the canonical `iss` value the
  // SPA sees in tokens.
  KEYCLOAK_INTERNAL_ISSUER_URL: z.string().url().optional(),
  KEYCLOAK_CLIENT_ID: z.string().min(1),
  KEYCLOAK_CLIENT_SECRET: z.string().min(1),

  SESSION_COOKIE_NAME: z.string().default('novanas_session'),
  SESSION_SECRET: z.string().min(16),

  KUBECONFIG_PATH: z.string().optional(),
  OPENBAO_ADDR: z.string().url().optional(),
  OPENBAO_TOKEN: z.string().optional(),
  PROMETHEUS_URL: z.string().url().optional(),

  API_VERSION: z.string().default('0.0.0'),
  API_PUBLIC_URL: z.string().url().default('http://localhost:8080'),
});

export type Env = z.infer<typeof EnvSchema>;

let cached: Env | undefined;

export function loadEnv(source: NodeJS.ProcessEnv = process.env): Env {
  if (cached) return cached;
  const parsed = EnvSchema.safeParse(source);
  if (!parsed.success) {
    // Don't leak secret values in the error output.
    const issues = parsed.error.issues
      .map((i) => `  - ${i.path.join('.')}: ${i.message}`)
      .join('\n');
    throw new Error(`Invalid environment configuration:\n${issues}`);
  }
  cached = Object.freeze(parsed.data);
  return cached;
}

/** Reset cache (for tests). */
export function resetEnvCache(): void {
  cached = undefined;
}
