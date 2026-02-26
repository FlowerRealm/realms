import { chromium } from 'playwright-core';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const baseURL = process.env.REALMS_BASE_URL || 'http://127.0.0.1:8080';
const key = process.env.REALMS_PERSONAL_KEY || 'demo-key';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(__dirname, '..', '..');

async function ensureLoggedIn(page) {
  await page.goto(`${baseURL}/login`, { waitUntil: 'domcontentloaded' });
  await page.waitForSelector('input[name="key"]');

  await page.fill('input[name="key"]', key);

  const confirmVisible = await page.locator('input[name="key_confirm"]').count();
  if (confirmVisible) {
    await page.fill('input[name="key_confirm"]', key);
  }

  await page.click('button[type="submit"]');
  await page.waitForURL(/\/admin(\/|$)/, { timeout: 60_000 });
}

async function screenshot(page, pathname, outPath) {
  await page.goto(`${baseURL}${pathname}`, { waitUntil: 'networkidle' });
  await page.waitForTimeout(350);
  await page.screenshot({ path: outPath });
}

async function main() {
  const browser = await chromium.launch({
    executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/opt/google/chrome/chrome',
    args: ['--headless=new'],
  });
  const context = await browser.newContext({
    viewport: { width: 2560, height: 1440 },
    deviceScaleFactor: 1,
  });
  const page = await context.newPage();

  await ensureLoggedIn(page);

  await screenshot(page, '/admin/channels', path.join(repoRoot, 'personal-2k-channels.png'));
  await screenshot(page, '/admin/usage', path.join(repoRoot, 'personal-2k-usage.png'));
  await screenshot(page, '/admin/api-keys', path.join(repoRoot, 'personal-2k-api-keys.png'));

  await browser.close();
}

await main();
