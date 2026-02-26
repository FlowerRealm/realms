import path from 'node:path'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  const isPersonal = mode === 'personal'
  const input = isPersonal ? { index: path.resolve(__dirname, './index.personal.html') } : { index: path.resolve(__dirname, './index.html') }
  return {
    resolve: {
      alias: {
        '@': path.resolve(__dirname, './src'),
      },
    },
    plugins: [react()],
    build: {
      outDir: isPersonal ? 'dist-personal' : 'dist',
      rollupOptions: {
        input,
      },
    },
    server: {
      host: '0.0.0.0',
      proxy: {
        '/api': {
          target: 'http://localhost:8080',
          changeOrigin: true,
        },
        '/v1': {
          target: 'http://localhost:8080',
          changeOrigin: true,
        },
        '/v1beta': {
          target: 'http://localhost:8080',
          changeOrigin: true,
        },
        '/oauth': {
          target: 'http://localhost:8080',
          changeOrigin: true,
        },
        '/assets': {
          target: 'http://localhost:8080',
          changeOrigin: true,
        },
        '/healthz': {
          target: 'http://localhost:8080',
          changeOrigin: true,
        },
      },
    },
  }
})
