/// <reference types="vitest/config" />
import { defineConfig, type Plugin } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { writeFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

const outDir = fileURLToPath(new URL('../internal/webui/dist', import.meta.url));

// emptyOutDir removes the tracked .gitkeep; put it back after each build.
const keepGitkeep: Plugin = {
  name: 'netwatch-gitkeep',
  apply: 'build',
  closeBundle() {
    writeFileSync(outDir + '/.gitkeep', '');
  },
};

export default defineConfig({
  plugins: [svelte(), keepGitkeep],
  build: {
    outDir,
    emptyOutDir: true,
    modulePreload: { polyfill: false },
    assetsInlineLimit: 0,
    chunkSizeWarningLimit: 700,
  },
  server: {
    proxy: {
      '/api': { target: 'http://127.0.0.1:8501', changeOrigin: false },
      '/tiles': { target: 'http://127.0.0.1:8501' },
    },
  },
  test: {
    include: ['src/**/*.test.ts'],
    env: { TZ: 'America/New_York' },
  },
});
