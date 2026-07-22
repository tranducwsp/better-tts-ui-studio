import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

// https://vite.dev/config/
export default defineConfig({
  plugins: [svelte()],
  base: '/',
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:8000',
      '/storage': 'http://localhost:8000',
    },
  },
  build: {
    outDir: '../public',
    emptyOutDir: true,
  },
})



