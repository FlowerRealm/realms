import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const rootDir = path.resolve(__dirname, '..', '..');
const desktopDir = path.join(rootDir, 'desktop');

execFileSync('node', [path.join(desktopDir, 'scripts', 'build-backend.mjs')], {
  cwd: rootDir,
  stdio: 'inherit',
});

execFileSync('npx', ['--no-install', 'electron-builder', '--publish', 'never'], {
  cwd: desktopDir,
  stdio: 'inherit',
});
