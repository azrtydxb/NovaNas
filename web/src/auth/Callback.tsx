import { useEffect } from "react";
import { useNavigate } from "react-router-dom";
import { userManager } from "./userManager";

export function Callback() {
  const navigate = useNavigate();
  useEffect(() => {
    userManager
      .signinRedirectCallback()
      .then(() => navigate("/", { replace: true }))
      .catch((err) => {
        console.error("OIDC callback failed", err);
        navigate("/", { replace: true });
      });
  }, [navigate]);
  return (
    <div style={{ padding: 40, color: "var(--fg-2)", fontSize: 12 }}>
      Completing sign-in…
    </div>
  );
}
