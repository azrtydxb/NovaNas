import { useEffect } from "react";
import { Routes, Route } from "react-router-dom";
import { useAuth } from "./store/auth";
import { Login } from "./auth/Login";
import { Callback } from "./auth/Callback";
import { Desktop } from "./desktop/Desktop";

export function App() {
  const status = useAuth((s) => s.status);
  const init = useAuth((s) => s.init);

  useEffect(() => {
    init();
  }, [init]);

  return (
    <Routes>
      <Route path="/auth/callback" element={<Callback />} />
      <Route
        path="*"
        element={
          status === "loading" ? (
            <Loading />
          ) : status === "authenticated" ? (
            <Desktop />
          ) : (
            <Login />
          )
        }
      />
    </Routes>
  );
}

function Loading() {
  return (
    <div
      style={{
        position: "fixed",
        inset: 0,
        display: "grid",
        placeItems: "center",
        color: "var(--fg-3)",
        fontSize: 12,
      }}
    >
      Loading…
    </div>
  );
}
