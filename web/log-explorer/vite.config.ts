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
      '/runs': 'http://localhost:8080',
      '/discover-tools': 'http://localhost:8080',
      '/test-connection': 'http://localhost:8080',
      '/agents': 'http://localhost:8080',
    },
  },
})
