import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Dev-mode strategy:
//   - The browser hits `/api/...` on the Vite dev server.
//   - Vite proxies to nova-api on the dev box, ignoring the self-signed
//     cert. This sidesteps CORS entirely.
//   - OIDC redirects still go directly to Keycloak, so the browser will
//     prompt to trust the Keycloak self-signed cert once per session
//     (https://192.168.10.204:8443).
const NOVA_API = "https://192.168.10.204:8444";

export default defineConfig({
  plugins: [react()],
  server: {
    host: "127.0.0.1",
    port: 5173,
    proxy: {
      "/api": {
        target: NOVA_API,
        changeOrigin: true,
        secure: false,
      },
    },
  },
});
