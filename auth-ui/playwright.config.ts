import { defineConfig, devices } from '@playwright/test';

/**
 * Browser-level tests for the auth UI.
 *
 * These run against the real production build (auth-ui/dist) served alongside a
 * contract-shaped stub of the Lesser API, with Chrome's WebAuthn virtual
 * authenticator driving the ceremonies. Run `pnpm build` before `pnpm test`, or
 * use `pnpm test:e2e` which does both.
 */
export default defineConfig({
  testDir: './tests',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: process.env.CI ? [['list']] : [['list']],
  timeout: 30_000,
  expect: { timeout: 10_000 },
  use: {
    trace: 'retain-on-failure'
  },
  projects: [
    {
      // WebAuthn virtual authenticators are a Chrome DevTools Protocol feature,
      // so this suite is Chromium-only by construction.
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] }
    }
  ]
});
