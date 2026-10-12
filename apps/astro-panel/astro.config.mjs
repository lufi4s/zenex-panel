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
      // During development, API calls go to the running panel on port 8443.
      proxy: {
        '/api': { target: 'https://127.0.0.1:8443', changeOrigin: true, secure: false },
      },
    },
  }
});
