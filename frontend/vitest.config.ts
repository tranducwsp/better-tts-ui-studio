import { defineConfig } from 'vitest/config';
import { svelte } from '@sveltejs/vite-plugin-svelte';

// https://vitest.dev/config/
export default defineConfig({
  plugins: [svelte()],
  test: {
    include: ['tests/unit/**/*.test.ts'],
  },
});