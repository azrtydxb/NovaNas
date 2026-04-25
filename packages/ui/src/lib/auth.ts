/**
 * OIDC client wiring via oidc-client-ts for Keycloak.
 *
 * Flow:
 *   1. /login → signinRedirect() → Keycloak authorize endpoint
 *   2. Keycloak redirects to /auth/callback with ?code=...
 *   3. callback page hands the code to the NovaNas API
 *      (POST /api/v1/auth/callback) which exchanges it for tokens,
 *      sets the session cookie and returns the user.
 *   4. API also handles logout: POST /api/v1/auth/logout then redirect to
 *      the Keycloak end_session_endpoint.
 */

import { UserManager, type UserManagerSettings, WebStorageStateStore } from 'oidc-client-ts';

export interface OidcConfig {
  issuer: string;
  clientId: string;
  redirectUri: string;
  postLogoutRedirectUri: string;
  scope: string;
}

export function resolveOidcConfig(): OidcConfig {
  const env = import.meta.env;
  return {
    // Keycloak v17+ dropped the /auth prefix — use the modern path.
    // Override via VITE_OIDC_ISSUER for legacy keycloak servers.
    issuer: (env.VITE_OIDC_ISSUER as string | undefined) ?? '/realms/novanas',
    clientId: (env.VITE_OIDC_CLIENT_ID as string | undefined) ?? 'novanas-ui',
    redirectUri:
      (env.VITE_OIDC_REDIRECT_URI as string | undefined) ??
      `${window.location.origin}/auth/callback`,
    postLogoutRedirectUri:
      (env.VITE_OIDC_POST_LOGOUT_URI as string | undefined) ?? `${window.location.origin}/login`,
    scope: (env.VITE_OIDC_SCOPE as string | undefined) ?? 'openid profile email',
  };
}

export function createUserManager(config = resolveOidcConfig()): UserManager {
  const settings: UserManagerSettings = {
    authority: config.issuer,
    client_id: config.clientId,
    redirect_uri: config.redirectUri,
    post_logout_redirect_uri: config.postLogoutRedirectUri,
    response_type: 'code',
    scope: config.scope,
    loadUserInfo: true,
    userStore: new WebStorageStateStore({ store: window.sessionStorage }),
    stateStore: new WebStorageStateStore({ store: window.sessionStorage }),
  };
  return new UserManager(settings);
}

let singleton: UserManager | null = null;
export function getUserManager(): UserManager {
  if (!singleton) singleton = createUserManager();
  return singleton;
}
