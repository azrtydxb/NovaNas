function required(name: string, value: string | undefined): string {
  if (value == null || value === "") {
    throw new Error(
      `Missing build-time env var ${name}. Set it in web/.env (or .env.production) before building.`
    );
  }
  return value;
}

export const env = {
  // VITE_API_BASE is allowed to be empty — when the SPA is served by
  // nova-api on the same origin, relative `/api/...` paths reach the
  // backend without a base prefix.
  apiBase: (import.meta.env.VITE_API_BASE as string | undefined) ?? "",
  oidc: {
    authority: required("VITE_OIDC_AUTHORITY", import.meta.env.VITE_OIDC_AUTHORITY),
    clientId: required("VITE_OIDC_CLIENT_ID", import.meta.env.VITE_OIDC_CLIENT_ID),
    redirectUri: window.location.origin + "/auth/callback",
    silentRedirectUri: window.location.origin + "/auth/silent",
    postLogoutRedirectUri: window.location.origin + "/",
  },
};
