import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath, URL } from "node:url";

// The production build is written straight into the Go API's embedded folder,
// so the server still ships as a single binary.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) },
  },
  build: {
    outDir: "../api/web/dist",
    emptyOutDir: true,
    sourcemap: false,
    target: "es2020",
    cssCodeSplit: false,
  },
  server: {
    port: 5173,
    // During development, API calls go to the running panel on port 8443.
    proxy: {
      "/api": { target: "https://127.0.0.1:8443", changeOrigin: true, secure: false },
    },
  },
  test: {
    environment: "node",
    include: ["src/**/*.test.{ts,tsx}"],
  },
});
