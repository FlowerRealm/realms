import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { defineConfig, devices } from '@playwright/test';

// CI/Dev 环境可能配置了全局代理；Playwright 的 webServer URL 探测若走代理会误判端口“已被占用”。
// 强制本地回环地址直连，避免 flake。
const existingNoProxy = (process.env.NO_PROXY || process.env.no_proxy || '').trim();
const localNoProxy = ['127.0.0.1', 'localhost', '::1'];
const noProxyParts = existingNoProxy
  .split(',')
  .map((s) => s.trim())
  .filter((s) => s);
for (const host of localNoProxy) {
  if (!noProxyParts.includes(host)) noProxyParts.push(host);
}
const mergedNoProxy = noProxyParts.join(',');
process.env.NO_PROXY = mergedNoProxy;
process.env.no_proxy = mergedNoProxy;

const configDir = path.dirname(fileURLToPath(import.meta.url));
const baseURL = process.env.REALMS_E2E_BASE_URL?.trim() || 'http://127.0.0.1:18181';
const u = new URL(baseURL);

const storageStatePath = path.join(configDir, 'playwright', '.auth', 'root.json');

export default defineConfig({
  testDir: path.join(configDir, 'e2e'),
  fullyParallel: false,
  workers: 1,
  retries: process.env.CI ? 1 : 0,
  timeout: 90_000,
  expect: {
    timeout: 30_000,
  },

  reporter: [
    ['list'],
    ['html', { open: 'never', outputFolder: path.join(configDir, 'playwright-report') }],
  ],

  use: {
    baseURL,
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
    storageState: storageStatePath,
    ...devices['Desktop Chrome'],
  },

  globalSetup: path.join(configDir, 'e2e', 'global-setup.ts'),

  webServer: {
    command: 'go run ./cmd/realms-e2e',
    cwd: path.resolve(configDir, '..'),
    url: `${baseURL}/healthz`,
    reuseExistingServer: false,
    timeout: 120_000,
    env: {
      ...process.env,
      REALMS_E2E_ADDR: u.host,
      REALMS_E2E_FRONTEND_DIST_DIR: path.join(configDir, 'dist'),
    },
  },
});
