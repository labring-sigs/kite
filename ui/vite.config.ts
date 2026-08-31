import path from 'path'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'
import { dynamicBase } from 'vite-plugin-dynamic-base'

const devBase = process.env.KITE_BASE || ''

export default defineConfig({
  base: process.env.NODE_ENV === 'production' ? '/__dynamic_base__/' : devBase,
  plugins: [
    dynamicBase({
      publicPath: 'window.__dynamic_base__',
      transformIndexHtml: true,
    }),
    react(),
  ],
  envPrefix: ['VITE_', 'KITE_'],
  build: {
    outDir: '../static',
    emptyOutDir: true,
    rollupOptions: {
      output: {
        manualChunks: {
          // NOTE: monaco is intentionally absent. Forcing it into a manual
          // chunk also dragged Vite's dynamic-import preload helper into
          // that chunk, and the entry's static edge to the helper made the
          // browser preload all ~3.7MB of monaco at startup. Monaco is only
          // reached through dynamic imports (lib/monaco-loader), so Rollup
          // splits it into an on-demand chunk by itself.
          lodash: ['lodash'],
          recharts: ['recharts'],
        },
      },
    },
  },
  server: {
    watch: {
      ignored: ['**/.vscode/**'],
    },
    proxy: {
      [devBase + '/api/']: {
        changeOrigin: true,
        target: 'http://localhost:8080',
      },
      '^/ws/.*': {
        target: 'ws://localhost:8080',
        ws: true,
        rewriteWsOrigin: true,
      },
    },
  },
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  worker: {
    format: 'es',
  },
  define: {
    global: 'globalThis',
  },
})
