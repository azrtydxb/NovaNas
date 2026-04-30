import { useEffect, useRef } from "react";
import { useAuth } from "../store/auth";

// No interactive UI — anonymous users are redirected straight to the
// (NovaNAS-themed) Keycloak login. We render a brief "Redirecting…"
// placeholder while the redirect kicks off; if it fails the user can
// click to retry.
export function Login() {
  const login = useAuth((s) => s.login);
  const fired = useRef(false);

  useEffect(() => {
    if (fired.current) return;
    fired.current = true;
    login().catch((e) => {
      console.error("OIDC redirect failed", e);
      fired.current = false;
    });
  }, [login]);

  return (
    <div className="login">
      <div className="login__wallpaper" />
      <div
        className="login__card"
        style={{ display: "grid", placeItems: "center", gap: 12 }}
      >
        <div className="login__logo">N</div>
        <div className="muted" style={{ fontSize: 11 }}>
          Redirecting to sign in…
        </div>
        <button
          className="btn"
          onClick={() => login()}
          style={{ fontSize: 11 }}
        >
          Click here if you are not redirected
        </button>
      </div>
    </div>
  );
}
