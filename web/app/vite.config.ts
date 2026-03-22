import { fileURLToPath, URL } from "url";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@shared": fileURLToPath(new URL("../../shared", import.meta.url)),
    },
  },
  server: {
    host: "0.0.0.0",
    port: 5173,
    fs: {
      allow: ["../.."],
    },
    proxy: {
      "/api": {
        target: "http://localhost:8080",
        changeOrigin: true,
        headers: { Origin: "http://localhost:8080" },
      },
    },
  },
});
