import { defineConfig } from 'astro/config';
import svelte from '@astrojs/svelte';
import { fileURLToPath } from 'node:url';

export default defineConfig({
  base: '/auth/',
  integrations: [
    svelte({
      compilerOptions: {
        runes: true, // Required for Svelte 5 runes (Greater Components)
      }
    })
  ],
  output: 'static',
  build: {
    inlineStylesheets: 'always',
    assets: '_assets',
  },
  vite: {
    resolve: {
      alias: {
        src: fileURLToPath(new URL('./src', import.meta.url)),
      },
    },
  },
});
