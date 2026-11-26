import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

export default defineConfig({
  root: 'frontend',
  plugins: [svelte()],
  build: { outDir: '../frontend-dist', emptyOutDir: true },
  server: { host: '127.0.0.1' },
  preview: { host: '127.0.0.1' },
});
