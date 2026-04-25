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
  };
}
