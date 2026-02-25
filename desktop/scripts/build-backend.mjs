import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const rootDir = path.resolve(__dirname, '..', '..');
const desktopDir = path.join(rootDir, 'desktop');
const webDir = path.join(rootDir, 'web');
const webDistIndex = path.join(webDir, 'dist-self', 'index.html');

function run(cmd, args, opts = {}) {
  execFileSync(cmd, args, {
    cwd: rootDir,
    stdio: 'inherit',
    ...opts,
  });
}

function ensureWebDist() {
  if (fs.existsSync(webDistIndex)) return;
  run('npm', ['--prefix', 'web', 'run', 'build:self']);
  if (!fs.existsSync(webDistIndex)) {
    throw new Error('web/dist-self 构建失败：缺少 web/dist-self/index.html');
  }
}

function outPath() {
  const outDir = path.join(desktopDir, 'build', 'backend');
  fs.mkdirSync(outDir, { recursive: true });
  const bin = process.platform === 'win32' ? 'realms.exe' : 'realms';
  return path.join(outDir, bin);
}

ensureWebDist();

const out = outPath();
run('go', ['build', '-tags', 'embed_web_self', '-o', out, './cmd/realms']);
console.log(`\nbackend built: ${out}\n`);
