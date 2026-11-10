import { defineConfig, devices } from '@playwright/test';

const server = (listen: string, control: string, extra = '') => ({
  command: `go run ./tools/e2eserver --listen ${listen} --control ${control} ${extra}`.trim(),
  cwd: '..',
  url: `http://${listen}/api/state`,
  reuseExistingServer: false,
  timeout: 180_000,
  stdout: 'ignore' as const,
  stderr: 'pipe' as const,
});

export default defineConfig({
  testDir: './e2e',
  workers: 1,
  fullyParallel: false,
  timeout: 60_000,
  expect: { timeout: 15_000 },
  retries: 0,
  reporter: [['list']],
  use: {
    baseURL: 'http://127.0.0.1:18501',
    trace: 'retain-on-failure',
    locale: 'ro-RO',
    timezoneId: 'America/New_York',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'], viewport: { width: 1440, height: 900 } } }],
  webServer: [server('127.0.0.1:18501', '127.0.0.1:18502'), server('127.0.0.1:18511', '127.0.0.1:18512', '--no-password --demo-flag')],
});
