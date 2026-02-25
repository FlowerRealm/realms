import { execFileSync, spawn } from 'node:child_process';
import path from 'node:path';
import fs from 'node:fs';
import { fileURLToPath } from 'node:url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const rootDir = path.resolve(__dirname, '..', '..');
const desktopDir = path.join(rootDir, 'desktop');
const backendBin = process.platform === 'win32'
  ? path.join(desktopDir, 'build', 'backend', 'realms.exe')
  : path.join(desktopDir, 'build', 'backend', 'realms');

execFileSync('node', [path.join(desktopDir, 'scripts', 'build-backend.mjs')], {
  cwd: rootDir,
  stdio: 'inherit',
});
if (!fs.existsSync(backendBin)) {
  throw new Error(`后端构建产物不存在: ${backendBin}`);
}

function shouldDisableSandboxLinux() {
  if (process.platform !== 'linux') return false;
  const sandbox = path.join(desktopDir, 'node_modules', 'electron', 'dist', 'chrome-sandbox');
  try {
    const st = fs.statSync(sandbox);
    const mode = st.mode & 0o7777;
    const ownedByRoot = st.uid === 0;
    const hasSetuid = mode === 0o4755;
    return !(ownedByRoot && hasSetuid);
  } catch {
    return false;
  }
}

const electronArgs = ['--no-install', 'electron', '.'];
if (shouldDisableSandboxLinux()) {
  electronArgs.push('--no-sandbox');
}

const child = spawn('npx', electronArgs, {
  cwd: desktopDir,
  stdio: 'inherit',
  env: {
    ...process.env,
    REALMS_DESKTOP_BACKEND_PATH: backendBin,
  },
});

child.on('exit', (code) => process.exit(code ?? 0));
