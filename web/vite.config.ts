import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// vite.config.ts
const apiPort = process.env.VITE_API_PORT ?? '19965'
const apiHost = process.env.VITE_API_HOST ?? 'localhost'
const apiBase = `http://${apiHost}:${apiPort}`
const wsBase  = `ws://${apiHost}:${apiPort}`

export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    outDir: '../ui/dist',
    emptyOutDir: true,
  },
  server: {
    proxy: {
      '/v1/ws': { target: wsBase, ws: true },
      '/v1':    { target: apiBase },
      '/auth':  { target: apiBase },
      '/health':{ target: apiBase },
    },
  },
})
