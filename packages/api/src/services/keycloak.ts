import * as client from 'openid-client';
import type { Env } from '../env.js';

export interface KeycloakClient {
  config: client.Configuration;
  issuerUrl: string;
  clientId: string;
  /** Build the authorization URL and return (url, state, nonce, code_verifier). */
  buildAuthUrl(redirectUri: string): Promise<{
    url: URL;
    state: string;
    nonce: string;
    codeVerifier: string;
  }>;
  /** Exchange auth code for tokens. */
  exchangeCode(params: {
    currentUrl: URL;
    state: string;
    nonce: string;
    codeVerifier: string;
  }): Promise<client.TokenEndpointResponse & client.TokenEndpointResponseHelpers>;
  /** Revoke refresh token (best-effort). */
  logout(refreshToken: string): Promise<void>;
  /**
   * Direct username/password login (RFC 6749 Resource Owner Password
   * Credentials Grant). Used by the SPA's single-page login form so
   * the user never sees the Keycloak-hosted login page. Requires the
   * Keycloak client to have `directAccessGrantsEnabled=true`.
   */
  passwordLogin(
    username: string,
    password: string
  ): Promise<client.TokenEndpointResponse & client.TokenEndpointResponseHelpers>;
}

export async function createKeycloakClient(env: Env): Promise<KeycloakClient> {
  const issuer = new URL(env.KEYCLOAK_ISSUER_URL);
  // Allow http:// Keycloak endpoints for in-cluster service URLs and local dev.
  // Production deployments use the novaedge-fronted https endpoint via Ingress.
  const discoveryOptions =
    issuer.protocol === 'http:' ? { execute: [client.allowInsecureRequests] } : undefined;
  const config = await client.discovery(
    issuer,
    env.KEYCLOAK_CLIENT_ID,
    env.KEYCLOAK_CLIENT_SECRET,
    undefined,
    discoveryOptions
  );

  return {
    config,
    issuerUrl: env.KEYCLOAK_ISSUER_URL,
    clientId: env.KEYCLOAK_CLIENT_ID,
    async buildAuthUrl(redirectUri: string) {
      const state = client.randomState();
      const nonce = client.randomNonce();
      const codeVerifier = client.randomPKCECodeVerifier();
      // openid-client v6's calculatePKCECodeChallenge is async — must
      // be awaited or `code_challenge` ends up serialised as the
      // literal string "[object Promise]" and Keycloak rejects the
      // PKCE check at /token exchange time.
      const codeChallenge = await client.calculatePKCECodeChallenge(codeVerifier);
      const params: Record<string, string> = {
        redirect_uri: redirectUri,
        scope: 'openid profile email',
        state,
        nonce,
        code_challenge: codeChallenge,
        code_challenge_method: 'S256',
        response_type: 'code',
      };
      const url = client.buildAuthorizationUrl(config, params);
      return { url, state, nonce, codeVerifier };
    },
    async exchangeCode(params) {
      return client.authorizationCodeGrant(config, params.currentUrl, {
        expectedState: params.state,
        expectedNonce: params.nonce,
        pkceCodeVerifier: params.codeVerifier,
      });
    },
    async logout(refreshToken: string) {
      try {
        await client.tokenRevocation(config, refreshToken);
      } catch {
        // best-effort
      }
    },
    async passwordLogin(username: string, password: string) {
      // openid-client doesn't ship a typed wrapper for the deprecated
      // ROPC grant, so we hit /token directly. Confidential client
      // auth is the same as for any other grant: client_id +
      // client_secret in the form body.
      const tokenEndpoint = config.serverMetadata().token_endpoint;
      if (!tokenEndpoint) throw new Error('keycloak: token_endpoint missing from discovery');
      const body = new URLSearchParams({
        grant_type: 'password',
        client_id: env.KEYCLOAK_CLIENT_ID,
        client_secret: env.KEYCLOAK_CLIENT_SECRET,
        username,
        password,
        scope: 'openid profile email',
      });
      const res = await fetch(tokenEndpoint, {
        method: 'POST',
        headers: { 'content-type': 'application/x-www-form-urlencoded' },
        body: body.toString(),
      });
      if (!res.ok) {
        const detail = await res.text().catch(() => '');
        const err = new Error(`keycloak: password grant failed (${res.status}): ${detail}`);
        // Tag with status so the caller can map 401/400 → user-facing
        // "invalid credentials" without leaking server internals.
        (err as Error & { status?: number }).status = res.status;
        throw err;
      }
      const tokens = (await res.json()) as Record<string, unknown>;
      // Decorate with the same helpers openid-client puts on its
      // authorizationCodeGrant return value (.claims()).
      const decorated = tokens as client.TokenEndpointResponse &
        client.TokenEndpointResponseHelpers;
      decorated.claims = () => {
        if (typeof tokens.id_token !== 'string') return undefined;
        const parts = tokens.id_token.split('.');
        if (parts.length !== 3) return undefined;
        try {
          return JSON.parse(Buffer.from(parts[1] ?? '', 'base64url').toString('utf8'));
        } catch {
          return undefined;
        }
      };
      decorated.expiresIn = () =>
        typeof tokens.expires_in === 'number' ? tokens.expires_in : undefined;
      return decorated;
    },
  };
}
