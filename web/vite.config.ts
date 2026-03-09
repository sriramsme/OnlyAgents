import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    outDir: '../ui/dist',
    emptyOutDir: true,
  },
  server: {
    proxy: {
      '/v1/ws': { target: 'ws://localhost:8080', ws: true },
      '/v1': { target: 'http://localhost:8080' },
      '/auth': { target: 'http://localhost:8080' },
      '/health': { target: 'http://localhost:8080' },
    },
  },
})
