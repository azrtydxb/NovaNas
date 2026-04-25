/**
 * Backend-For-Frontend OIDC wiring.
 *
 * The api owns the OIDC dance: server-side PKCE, code exchange, session
 * cookie. The SPA's job is to:
 *   1. Tell the api "I want to log in" → POST /api/v1/auth/login
 *   2. Send the browser to the URL the api returns (Keycloak's auth
 *      endpoint).
 *   3. Wait for Keycloak to redirect the browser back to
 *      /api/v1/auth/callback (the api handles that route, sets the
 *      session cookie, and 302's to the original URL).
 *   4. Read the resulting session via GET /api/v1/auth/me.
 *
 * No client-side PKCE → no `crypto.subtle` → no "secure context"
 * requirement → plain HTTP on a LAN IP works without certificates.
 */

import { api } from '@/lib/api';

/** Begin login. Browser leaves this page; never returns from this call. */
export async function startLogin(redirectTo: string = window.location.pathname): Promise<void> {
  const { url } = await api.post<{ url: string }>('/auth/login', { redirectTo });
  window.location.href = url;
}

/** Server-side logout: api destroys the session, returns the OIDC end-session URL. */
export async function startLogout(): Promise<void> {
  try {
    const res = await api.post<{ url?: string }>('/auth/logout', {});
    if (res?.url) {
      window.location.href = res.url;
      return;
    }
  } catch {
    /* fall through to local redirect */
  }
  window.location.href = '/login';
}
