import { defineConfig } from "vite";
import solid from "vite-plugin-solid";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath, URL } from "node:url";

// The production build is written straight into the Go API's embedded folder,
// so the server still ships as a single binary.
export default defineConfig({
  plugins: [solid(), tailwindcss()],
  resolve: {
    alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) },
    // One copy of Solid for the app, Kobalte and the test library.
    dedupe: ["solid-js"],
    // Tests load the browser build of Solid, not the server build.
    conditions: ["development", "browser"],
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
    environment: "jsdom",
    include: ["src/**/*.test.{ts,tsx}"],
    // Load every dependency through Vite so Solid is only ever instantiated once.
    server: { deps: { inline: true } },
  },
});
