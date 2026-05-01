import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "node:path";

const proxyTargets = ["/api", "/healthz", "/readyz", "/livez", "/metrics"];

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    port: 5173,
    proxy: Object.fromEntries(
      proxyTargets.map((p) => [
        p,
        { target: "http://localhost:8989", changeOrigin: true },
      ]),
    ),
  },
});
