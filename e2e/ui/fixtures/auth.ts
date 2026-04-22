import { type APIRequestContext, type Page, test as base } from "@playwright/test";
import { ApiClient } from "../lib/api-client";

/**
 * Auth fixture — pre-authenticates against the NovaNas API using either:
 *   - a static API token (E2E_API_TOKEN), preferred for CI speed, OR
 *   - an OIDC password-grant against the bundled Keycloak dev realm.
 *
 * The resulting bearer token is injected into a storageState file reused
 * across tests and also exposed as an ApiClient instance.
 */

export interface AuthFixtures {
  api: ApiClient;
  authedPage: Page;
}

const BASE_URL = process.env.NOVANAS_BASE_URL ?? "https://localhost:8443";
const E2E_USERNAME = process.env.E2E_USERNAME ?? "e2e-admin";
const E2E_PASSWORD = process.env.E2E_PASSWORD ?? "e2e-admin-password";
const E2E_API_TOKEN = process.env.E2E_API_TOKEN;

export async function acquireToken(request: APIRequestContext): Promise<string> {
  if (E2E_API_TOKEN) return E2E_API_TOKEN;

  // Fallback: OIDC password grant against the dev Keycloak realm.
  const discovery = await request.get(
    `${BASE_URL}/realms/novanas/.well-known/openid-configuration`,
    { ignoreHTTPSErrors: true },
  );
  if (!discovery.ok()) {
    throw new Error(
      `OIDC discovery failed (${discovery.status()}); set E2E_API_TOKEN to skip.`,
    );
  }
  const { token_endpoint } = (await discovery.json()) as { token_endpoint: string };
  const tokenRes = await request.post(token_endpoint, {
    form: {
      grant_type: "password",
      client_id: "novanas-ui",
      username: E2E_USERNAME,
      password: E2E_PASSWORD,
      scope: "openid profile",
    },
    ignoreHTTPSErrors: true,
  });
  if (!tokenRes.ok()) {
    throw new Error(`OIDC token request failed: ${tokenRes.status()}`);
  }
  const { access_token } = (await tokenRes.json()) as { access_token: string };
  return access_token;
}

export const test = base.extend<AuthFixtures>({
  api: async ({ request }, use) => {
    const token = await acquireToken(request);
    await use(new ApiClient({ baseURL: BASE_URL, token }));
  },
  authedPage: async ({ page, request }, use) => {
    const token = await acquireToken(request);
    await page.addInitScript((t) => {
      window.localStorage.setItem("novanas.token", t as string);
    }, token);
    await use(page);
  },
});

export { expect } from "@playwright/test";
