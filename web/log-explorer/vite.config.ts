import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  base: '/ui/logs/',
  build: {
    outDir: 'dist',
  },
  server: {
    proxy: {
      '/api/runs': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      '/runs': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        configure: (proxy) => {
          proxy.on('proxyRes', (proxyRes) => {
            if (proxyRes.headers['content-type']?.includes('text/event-stream')) {
              proxyRes.headers['cache-control'] = 'no-cache';
              proxyRes.headers['connection'] = 'keep-alive';
            }
          });
        },
      },
      '/discover-tools': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      '/test-connection': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      '/agents': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
})
