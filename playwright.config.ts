import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './tests/integration',
  testMatch: '*.spec.ts',
  timeout: 30000,
  retries: 1,
  use: {
    baseURL: process.env.BASE_URL || 'http://localhost:5173',
    headless: true,
  },
  projects: [
    {
      name: 'chromium',
      use: { browserName: 'chromium' },
    },
  ],
  // No webServer — we connect to the running Docker stack
});