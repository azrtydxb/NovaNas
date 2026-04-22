import * as client from 'openid-client';
import type { Env } from '../env.js';

export interface KeycloakClient {
  config: client.Configuration;
  issuerUrl: string;
  clientId: string;
  /** Build the authorization URL and return (url, state, nonce, code_verifier). */
  buildAuthUrl(redirectUri: string): {
    url: URL;
    state: string;
    nonce: string;
    codeVerifier: string;
  };
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
  const config = await client.discovery(
    issuer,
    env.KEYCLOAK_CLIENT_ID,
    env.KEYCLOAK_CLIENT_SECRET
  );

  return {
    config,
    issuerUrl: env.KEYCLOAK_ISSUER_URL,
    clientId: env.KEYCLOAK_CLIENT_ID,
    buildAuthUrl(redirectUri: string) {
      const state = client.randomState();
      const nonce = client.randomNonce();
      const codeVerifier = client.randomPKCECodeVerifier();
      // Synchronous in openid-client v6 when using random helpers
      const codeChallenge = client.calculatePKCECodeChallenge
        ? // v6 API: returns a string synchronously via helper; fall back to base64url
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
          (client as any).calculatePKCECodeChallenge?.(codeVerifier) ?? codeVerifier
        : codeVerifier;
      const params: Record<string, string> = {
        redirect_uri: redirectUri,
        scope: 'openid profile email',
        state,
        nonce,
        code_challenge: String(codeChallenge),
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
