import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

// Mặc định localhost:8000 cho `npm run dev` trên host. Trong Docker compose, biến
// VITE_BACKEND_URL được đặt thành http://backend:8000 (DNS nội bộ), nên giá trị
// mặc định này không ảnh hưởng.
const backendTarget = process.env.VITE_BACKEND_URL || 'http://localhost:8000'

// https://vite.dev/config/
export default defineConfig({
  plugins: [svelte()],
  base: '/',
  server: {
    port: 5173,
    host: '0.0.0.0',
    proxy: {
      '/api': {
        target: backendTarget,
        changeOrigin: true,
      },
      '/storage': {
        target: backendTarget,
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
})
