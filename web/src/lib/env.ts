export const env = {
  apiBase: import.meta.env.VITE_API_BASE as string,
  oidc: {
    authority: import.meta.env.VITE_OIDC_AUTHORITY as string,
    clientId: import.meta.env.VITE_OIDC_CLIENT_ID as string,
    redirectUri: window.location.origin + "/auth/callback",
    silentRedirectUri: window.location.origin + "/auth/silent",
    postLogoutRedirectUri: window.location.origin + "/",
  },
};
