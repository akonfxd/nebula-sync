import { writeFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { defineConfig, type Plugin, type ProxyOptions } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

// The console is served by the go binary from an embedded filesystem, so the
// build writes straight into the package that embeds it.
const outDir = '../internal/api/ui/dist';

// Where `npm run dev` forwards /api and /health. The proxy rewrites Origin to
// match the target, because the api rejects a cross-origin write and the dev
// server would otherwise look exactly like a csrf attempt.
const target = process.env.NEBULA_API ?? 'http://127.0.0.1:8080';

const sameOrigin: ProxyOptions = {
  target,
  changeOrigin: true,
  configure: (proxy) => {
    proxy.on('proxyReq', (request) => request.setHeader('origin', target));
  },
};

/**
 * The go package embeds `dist` with `//go:embed all:dist`, which fails the
 * build when the directory is empty. A committed `.gitkeep` is what keeps a
 * checkout without a frontend build compiling, and emptyOutDir deletes it on
 * every build, so put it back.
 */
function keepDistTracked(): Plugin {
  return {
    name: 'keep-dist-tracked',
    closeBundle() {
      writeFileSync(resolve(import.meta.dirname, outDir, '.gitkeep'), '');
    },
  };
}

export default defineConfig({
  plugins: [svelte(), keepDistTracked()],
  build: {
    outDir,
    emptyOutDir: true,
    // No inline script, so the console runs under a csp without 'unsafe-inline'.
    modulePreload: { polyfill: false },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': sameOrigin,
      '/health': sameOrigin,
    },
  },
});
