import { useState } from "react";
import { useAuth } from "../store/auth";
import { Icon } from "../components/Icon";

export function Login() {
  const login = useAuth((s) => s.login);
  const [busy, setBusy] = useState(false);

  const onSignIn = async () => {
    setBusy(true);
    try {
      await login();
    } catch (e) {
      console.error(e);
      setBusy(false);
    }
  };

  return (
    <div className="login">
      <div className="login__wallpaper" />
      <div className="login__card">
        <div className="login__logo">N</div>
        <h1 className="login__title">NovaNAS</h1>
        <p className="login__sub">Sign in with your operator credentials</p>
        <button className="btn btn--primary login__btn" onClick={onSignIn} disabled={busy}>
          <Icon name="shield" size={12} />
          {busy ? "Redirecting…" : "Sign in with Keycloak"}
        </button>
        <div className="login__foot">
          <span className="mono">{location.host}</span>
          <span className="login__foot-dot">·</span>
          v2.0.0
        </div>
      </div>
    </div>
  );
}
