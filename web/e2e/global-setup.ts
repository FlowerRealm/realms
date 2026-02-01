import fs from 'node:fs/promises';
import path from 'node:path';

import { chromium, type FullConfig } from '@playwright/test';

import { E2E_SEED } from './seed';

export default async function globalSetup(config: FullConfig) {
  const baseURL = (config.projects[0]?.use as { baseURL?: string }).baseURL || 'http://127.0.0.1:18181';
  const storageState = (config.projects[0]?.use as { storageState?: string }).storageState;
  if (!storageState) {
    throw new Error('playwright: missing storageState in config');
  }

  await fs.mkdir(path.dirname(storageState), { recursive: true });

  const browser = await chromium.launch();
  const page = await browser.newPage();

  await page.goto(`${baseURL}/login`, { waitUntil: 'commit' });
  await page.locator('input[name="login"]').fill(E2E_SEED.root.username);
  await page.locator('input[name="password"]').fill(E2E_SEED.root.password);
  await page.getByRole('button', { name: '立即登录' }).click();
  await page.waitForURL('**/dashboard', { timeout: 30_000 });

  await page.context().storageState({ path: storageState });
  await browser.close();
}
