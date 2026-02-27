import { spawn } from 'node:child_process';
import http from 'node:http';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { access } from 'node:fs/promises';

import { chromium } from 'playwright-core';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const repoRoot = path.resolve(__dirname, '..', '..');
const distIndex = path.join(repoRoot, 'web', 'dist', 'index.html');

const host = '127.0.0.1';
const port = Number(process.env.REALMS_PLAYWRIGHT_PORT || '18181');
const baseURL = `http://${host}:${port}`;
const chromePath = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/opt/google/chrome/chrome';

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function httpGet(url) {
  return new Promise((resolve, reject) => {
    const req = http.get(url, (res) => {
      res.resume();
      resolve({ statusCode: res.statusCode || 0 });
    });
    req.setTimeout(1500, () => req.destroy(new Error('timeout')));
    req.on('error', reject);
  });
}

async function waitForHealthz(url, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  let lastErr = null;
  while (Date.now() < deadline) {
    try {
      const res = await httpGet(url);
      if (res.statusCode >= 200 && res.statusCode < 500) {
        return;
      }
    } catch (e) {
      lastErr = e;
    }
    await sleep(150);
  }
  throw new Error(`waitForHealthz timeout: ${url}${lastErr ? ` (${String(lastErr)})` : ''}`);
}

function runCommand(cmd, args, opts) {
  return new Promise((resolve, reject) => {
    const child = spawn(cmd, args, opts);
    const out = [];
    if (child.stdout) child.stdout.on('data', (b) => out.push(String(b)));
    if (child.stderr) child.stderr.on('data', (b) => out.push(String(b)));
    child.on('error', reject);
    child.on('exit', (code) => {
      if (code === 0) {
        resolve({ output: out.join('') });
        return;
      }
      reject(new Error(`${cmd} ${args.join(' ')} exited with code ${code}\n\n${out.join('')}`));
    });
  });
}

async function terminateProcess(child, timeoutMs) {
  if (!child || child.killed) return;

  child.kill('SIGINT');
  const exited = await Promise.race([
    new Promise((resolve) => child.once('exit', resolve)),
    sleep(timeoutMs).then(() => null),
  ]);
  if (exited === null) {
    child.kill('SIGKILL');
  }
}

async function main() {
  await access(distIndex).catch(() => {
    throw new Error(`missing frontend build: ${distIndex}. Run: (cd web && npm run build)`);
  });

  const serverBin = path.join('/tmp', `realms_playwright_${port}`);

  const serverEnv = {
    ...process.env,
    REALMS_ENV: process.env.REALMS_ENV || 'dev',
    REALMS_MODE: 'business',
    REALMS_ADDR: `${host}:${port}`,
    REALMS_DB_DRIVER: 'sqlite',
    REALMS_SQLITE_PATH: process.env.REALMS_SQLITE_PATH || `/tmp/realms_playwright_${port}.db?_busy_timeout=30000`,
    REALMS_DISABLE_SECURE_COOKIES: process.env.REALMS_DISABLE_SECURE_COOKIES || 'true',
    FRONTEND_DIST_DIR: process.env.FRONTEND_DIST_DIR || path.join(repoRoot, 'web', 'dist'),
    FRONTEND_BASE_URL: process.env.FRONTEND_BASE_URL || '',
  };

  await runCommand('go', ['build', '-o', serverBin, './cmd/realms'], {
    cwd: repoRoot,
    env: serverEnv,
    stdio: ['ignore', 'pipe', 'pipe'],
  });

  const server = spawn(serverBin, [], { cwd: repoRoot, env: serverEnv, stdio: ['ignore', 'pipe', 'pipe'] });

  const serverLogs = [];
  server.stdout.on('data', (b) => serverLogs.push(String(b)));
  server.stderr.on('data', (b) => serverLogs.push(String(b)));

  try {
    await waitForHealthz(`${baseURL}/healthz`, 15_000);

    const browser = await chromium.launch({
      executablePath: chromePath,
      headless: true,
      args: ['--no-sandbox', '--disable-dev-shm-usage'],
    });
    try {
      const page = await browser.newPage();

      const externalRequests = new Set();
      page.on('request', (req) => {
        const url = req.url();
        if (!url.startsWith('http://') && !url.startsWith('https://')) return;
        if (url.startsWith(baseURL + '/')) return;
        externalRequests.add(url);
      });

      await page.goto(`${baseURL}/login`, { waitUntil: 'domcontentloaded', timeout: 20_000 });
      await page.getByRole('heading', { name: '登录 Realms' }).waitFor({ timeout: 20_000 });

      const hasExternalStyles = (await page.locator('link[rel="stylesheet"][href^="http"]').count()) > 0;
      const hasExternalScripts = (await page.locator('script[src^="http"]').count()) > 0;

      if (hasExternalStyles || hasExternalScripts || externalRequests.size > 0) {
        const details = [
          hasExternalStyles ? 'external stylesheet tag found' : '',
          hasExternalScripts ? 'external script tag found' : '',
          externalRequests.size > 0 ? `external requests:\n${Array.from(externalRequests).join('\n')}` : '',
        ]
          .filter(Boolean)
          .join('\n\n');
        throw new Error(`CDN/offline check failed.\n\n${details}`);
      }
    } finally {
      await browser.close();
    }
  } catch (e) {
    const logs = serverLogs.join('');
    const suffix = logs.trim() ? `\n\n--- server logs ---\n${logs}` : '';
    throw new Error(`${e instanceof Error ? e.message : String(e)}${suffix}`);
  } finally {
    await terminateProcess(server, 5_000);
  }
}

main().catch((e) => {
  console.error(e instanceof Error ? e.message : String(e));
  process.exit(1);
});
