import { defineConfig } from 'astro/config';
import svelte from '@astrojs/svelte';

export default defineConfig({
  base: '/auth/',
  integrations: [
    svelte({
      compilerOptions: {
        runes: true  // Required for Svelte 5 runes (@equaltoai/greater-components)
      }
    })
  ],
  output: 'static',
  build: {
    inlineStylesheets: 'always',
    assets: '_assets',
  },
  vite: {
    optimizeDeps: {
      exclude: ['@equaltoai/greater-components'],
    },
    ssr: {
      noExternal: ['@equaltoai/greater-components'],
    },
  },
});
