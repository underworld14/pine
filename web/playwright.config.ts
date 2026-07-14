import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './e2e',
  timeout: 30000,
  // One shared pine serve process — avoid racing mutations across workers.
  workers: 1,
  use: { baseURL: 'http://127.0.0.1:3413' },
  webServer: {
    command: 'sh ../scripts/e2e-serve.sh',
    url: 'http://127.0.0.1:3413/api/health',
    timeout: 180000,
    reuseExistingServer: !process.env.CI
  }
});
