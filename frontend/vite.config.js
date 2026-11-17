import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const apiProxyTarget = env.API_PROXY_TARGET || 'http://localhost:18080'

  return {
    plugins: [react()],
    server: {
      proxy: { '/api/v1': { target: apiProxyTarget, changeOrigin: true } },
    },
    test: {
      environment: 'jsdom',
    },
  }
})
