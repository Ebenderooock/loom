import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { viteStaticCopy } from "vite-plugin-static-copy";
import path from "node:path";

const proxyTargets = ["/api", "/healthz", "/readyz", "/livez", "/metrics"];

export default defineConfig({
  plugins: [
    react(),
    // Ship Monaco's prebuilt AMD assets as static files (served at /monaco/vs)
    // and load them at runtime via the @monaco-editor/react loader. This avoids
    // bundling Monaco from source, which otherwise OOMs constrained build nodes.
    viteStaticCopy({
      targets: [
        {
          src: "node_modules/monaco-editor/min/vs/**/*",
          dest: "monaco/vs",
          rename: { stripBase: 4 },
        },
      ],
    }),
  ],
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
        { target: "http://localhost:1925", changeOrigin: true },
      ]),
    ),
  },
});
