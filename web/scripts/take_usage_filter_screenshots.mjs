import { chromium } from 'playwright-core';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const baseURL = process.env.REALMS_BASE_URL || 'http://127.0.0.1:8082';
const chromePath = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/opt/google/chrome/chrome';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(__dirname, '..', '..');
const outDir = path.join(repoRoot, '.tmp', 'pw');

async function ensureLoggedIn(page) {
  const ts = Date.now();
  const email = `root${ts}@example.com`;
  const username = `root${ts}`;
  const password = 'Passw0rd!';

  await page.goto(`${baseURL}/register`, { waitUntil: 'domcontentloaded' });
  await page.waitForSelector('input[name="email"]');
  await page.fill('input[name="email"]', email);
  await page.fill('input[name="username"]', username);
  await page.fill('input[name="password"]', password);
  await page.click('button[type="submit"]');
  await page.waitForURL(/\/dashboard(\/|$)/, { timeout: 60_000 });
}

async function openAdvAndShot(page, pathname, toggleTestId, outPath) {
  await page.goto(`${baseURL}${pathname}`, { waitUntil: 'networkidle' });
  await page.waitForTimeout(250);
  await page.click(`[data-testid="${toggleTestId}"]`);
  await page.waitForSelector('.rlm-usage-filter-dropdown', { state: 'visible' });
  await page.waitForTimeout(200);
  await page.screenshot({ path: outPath });
}

async function main() {
  const browser = await chromium.launch({
    executablePath: chromePath,
    args: ['--headless=new'],
  });
  const context = await browser.newContext({
    viewport: { width: 1400, height: 900 },
    deviceScaleFactor: 1,
  });
  const page = await context.newPage();

  await ensureLoggedIn(page);

  await openAdvAndShot(page, '/usage', 'usage-adv-toggle', path.join(outDir, 'usage-adv.png'));
  await openAdvAndShot(page, '/admin/usage', 'admin-usage-adv-toggle', path.join(outDir, 'admin-usage-adv.png'));

  await browser.close();
}

await main();

