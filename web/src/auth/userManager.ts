import { UserManager, WebStorageStateStore, type User } from "oidc-client-ts";
import { env } from "../lib/env";

export const userManager = new UserManager({
  authority: env.oidc.authority,
  client_id: env.oidc.clientId,
  redirect_uri: env.oidc.redirectUri,
  silent_redirect_uri: env.oidc.silentRedirectUri,
  post_logout_redirect_uri: env.oidc.postLogoutRedirectUri,
  response_type: "code",
  scope: "openid profile email",
  userStore: new WebStorageStateStore({ store: window.localStorage }),
  loadUserInfo: true,
  automaticSilentRenew: true,
});

export type { User };
