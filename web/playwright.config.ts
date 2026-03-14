import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  timeout: 60_000,
  fullyParallel: false,
  use: {
    baseURL: 'http://127.0.0.1:8081',
    trace: 'on-first-retry',
  },
  webServer: {
    command: 'go run ./cmd/server -port 8081',
    cwd: '..',
    url: 'http://127.0.0.1:8081',
    reuseExistingServer: true,
    timeout: 180_000,
  },
});
