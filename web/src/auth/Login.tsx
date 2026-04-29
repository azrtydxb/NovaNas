import { useAuth } from "../store/auth";

export function Login() {
  const login = useAuth((s) => s.login);
  return (
    <div className="login">
      <div className="login__card">
        <div className="login__logo">N</div>
        <h1 className="login__title">NovaNAS</h1>
        <p className="login__sub">Sign in with your operator credentials.</p>
        <button className="btn btn--primary login__btn" onClick={() => login()}>
          Sign in
        </button>
        <div className="login__foot">
          <span className="mono">{location.host}</span> · v2.0.0
        </div>
      </div>
    </div>
  );
}
