// @ts-check
import { defineConfig } from 'astro/config';

import react from '@astrojs/react';
import tailwindcss from '@tailwindcss/vite';
import { fileURLToPath, URL } from 'node:url';

// https://astro.build/config
export default defineConfig({
  integrations: [react()],
  devToolbar: { enabled: false },

  vite: {
    plugins: [tailwindcss()],
    resolve: {
      alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
    },
    server: {
      // During development, API calls go to the live panel (set PANEL_API to override).
      proxy: {
        '/api': { target: process.env.PANEL_API ?? 'https://162.4.35.76:8443', changeOrigin: true, secure: false },
      },
    },
  }
});
