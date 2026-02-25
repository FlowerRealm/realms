import { app, BrowserWindow, Menu, Tray, clipboard, dialog, globalShortcut, nativeImage, shell } from 'electron';
import updater from 'electron-updater';
import AutoLaunch from 'auto-launch';
import { spawn } from 'node:child_process';
import fs from 'node:fs';
import net from 'node:net';
import os from 'node:os';
import path from 'node:path';

const { autoUpdater } = updater;

const BACKEND_HOST = '127.0.0.1';
const BACKEND_PORT = 8080;
const BASE_URL = `http://${BACKEND_HOST}:${BACKEND_PORT}`;
const DESKTOP_SCHEME = 'realms';

const isDev = !app.isPackaged;

// 一些 Linux/虚拟机/远程环境下 GPU 进程不可用会导致 Electron 直接崩溃（GPU process isn't usable）。
// 自用桌面壳体优先稳定性：默认禁用硬件加速。
app.disableHardwareAcceleration();
app.commandLine.appendSwitch('disable-gpu');

let mainWindow = null;
let tray = null;
let backendProc = null;
let pendingDeepLink = null;
let isQuitting = false;

let autolaunch = null;

function configPath() {
  return path.join(app.getPath('userData'), 'desktop-config.json');
}

function readConfig() {
  const defaults = {
    minimizeToTray: true,
    autoLaunch: false,
    globalShortcut: 'CommandOrControl+Shift+L',
    globalShortcutEnabled: true,
  };
  try {
    const raw = fs.readFileSync(configPath(), 'utf8');
    const parsed = JSON.parse(raw);
    return { ...defaults, ...(parsed && typeof parsed === 'object' ? parsed : {}) };
  } catch {
    return defaults;
  }
}

function writeConfig(next) {
  fs.mkdirSync(app.getPath('userData'), { recursive: true });
  fs.writeFileSync(configPath(), JSON.stringify(next, null, 2), 'utf8');
}

function backendPath() {
  const override = String(process.env.REALMS_DESKTOP_BACKEND_PATH || '').trim();
  if (override) return override;
  const binName = process.platform === 'win32' ? 'realms.exe' : 'realms';
  return path.join(process.resourcesPath, 'backend', binName);
}

function backendLogPath() {
  return path.join(app.getPath('userData'), 'backend.log');
}

function sqliteDSN() {
  const file = path.join(app.getPath('userData'), 'realms.db');
  return `${file}?_busy_timeout=30000`;
}

function ensureSingleInstance() {
  const gotLock = app.requestSingleInstanceLock();
  if (!gotLock) {
    app.quit();
    return false;
  }
  app.on('second-instance', (_event, argv) => {
    const url = argv.find((x) => typeof x === 'string' && x.startsWith(`${DESKTOP_SCHEME}://`));
    if (url) handleDeepLink(url);
    if (mainWindow) {
      if (mainWindow.isMinimized()) mainWindow.restore();
      mainWindow.show();
      mainWindow.focus();
    }
  });
  return true;
}

async function sleep(ms) {
  await new Promise((r) => setTimeout(r, ms));
}

async function waitForBackendReady({ timeoutMs }) {
  const deadline = Date.now() + timeoutMs;
  let lastErr = null;
  while (Date.now() < deadline) {
    try {
      const res = await fetch(`${BASE_URL}/healthz`, { method: 'GET' });
      if (res.ok) return;
      lastErr = new Error(`healthz not ok: ${res.status}`);
    } catch (e) {
      lastErr = e;
    }
    await sleep(250);
  }
  throw lastErr || new Error('backend not ready');
}

async function assertPortAvailableOrThrow() {
  await new Promise((resolve, reject) => {
    const s = net.createServer();
    s.once('error', reject);
    s.listen(BACKEND_PORT, BACKEND_HOST, () => {
      s.close(() => resolve());
    });
  }).catch((e) => {
    const code = e && typeof e === 'object' ? e.code : '';
    if (code === 'EADDRINUSE') {
      throw new Error(`固定端口已被占用：${BACKEND_HOST}:${BACKEND_PORT}`);
    }
    throw e;
  });
}

function spawnBackend() {
  const bin = backendPath();
  if (!fs.existsSync(bin)) {
    throw new Error(`后端二进制不存在: ${bin}`);
  }

  const env = {
    ...process.env,
    REALMS_ENV: 'desktop',
    REALMS_SELF_MODE_ENABLE: 'true',
    REALMS_ADDR: `${BACKEND_HOST}:${BACKEND_PORT}`,
    REALMS_DB_DRIVER: 'sqlite',
    REALMS_SQLITE_PATH: sqliteDSN(),
    REALMS_TICKETS_ATTACHMENTS_DIR: path.join(app.getPath('userData'), 'tickets'),
    REALMS_DISABLE_SECURE_COOKIES: 'true',
    FRONTEND_BASE_URL: '',
  };

  const logStream = fs.createWriteStream(backendLogPath(), { flags: 'a' });
  logStream.write(`\n\n[${new Date().toISOString()}] spawn backend (pid: pending)\n`);
  logStream.write(`platform=${process.platform} arch=${process.arch} node=${process.version} os=${os.release()}\n`);
  logStream.write(`REALMS_ADDR=${env.REALMS_ADDR}\n`);
  logStream.write(`REALMS_SQLITE_PATH=${env.REALMS_SQLITE_PATH}\n`);

  backendProc = spawn(bin, [], {
    env,
    stdio: ['ignore', 'pipe', 'pipe'],
    windowsHide: true,
  });
  logStream.write(`[${new Date().toISOString()}] backend pid=${backendProc.pid}\n`);

  backendProc.stdout.on('data', (buf) => logStream.write(buf));
  backendProc.stderr.on('data', (buf) => logStream.write(buf));
  backendProc.on('exit', (code, signal) => {
    logStream.write(`\n[${new Date().toISOString()}] backend exited code=${code} signal=${signal}\n`);
  });
}

async function stopBackend() {
  const p = backendProc;
  backendProc = null;
  if (!p) return;

  try {
    p.kill('SIGTERM');
  } catch {
    // ignore
  }

  const deadline = Date.now() + 2500;
  while (Date.now() < deadline) {
    if (p.exitCode !== null) return;
    await sleep(100);
  }
  try {
    p.kill('SIGKILL');
  } catch {
    // ignore
  }
}

function trayIcon() {
  const svg = `
    <svg xmlns="http://www.w3.org/2000/svg" width="64" height="64">
      <rect x="8" y="10" width="48" height="44" rx="10" fill="#0b0f14"/>
      <path d="M18 44V20h14c6 0 10 4 10 10s-4 14-10 14H18zm8-8h6c3 0 6-2 6-6s-3-4-6-4h-6v10z" fill="#ffffff"/>
    </svg>
  `.trim();
  const dataUrl = `data:image/svg+xml;base64,${Buffer.from(svg).toString('base64')}`;
  const img = nativeImage.createFromDataURL(dataUrl);
  return img.resize({ width: 16, height: 16 });
}

function setTrayMenu() {
  if (!tray) return;
  const cfg = readConfig();
  if (!autolaunch) {
    autolaunch = new AutoLaunch({
      name: 'Realms',
      path: app.getPath('exe'),
    });
  }

  const template = [
    {
      label: mainWindow && mainWindow.isVisible() ? '隐藏窗口' : '显示窗口',
      click: () => {
        if (!mainWindow) return;
        if (mainWindow.isVisible()) mainWindow.hide();
        else {
          mainWindow.show();
          mainWindow.focus();
        }
      },
    },
    {
      label: '复制 base_url（用于外部客户端）',
      click: () => clipboard.writeText(`${BASE_URL}/v1`),
    },
    {
      label: '导出客户端配置片段…',
      click: () => exportClientConfig(),
    },
    {
      label: '导出数据库备份（realms.db）…',
      click: () => exportDBBackup(),
    },
    {
      label: '导入数据库备份（会重启后端）…',
      click: () => importDBBackup(),
    },
    {
      label: '重启后端',
      click: () => restartBackend(),
    },
    { type: 'separator' },
    {
      label: '开机自启',
      type: 'checkbox',
      checked: !!cfg.autoLaunch,
      click: async (item) => {
        const next = { ...cfg, autoLaunch: !!item.checked };
        writeConfig(next);
        try {
          if (item.checked) await autolaunch.enable();
          else await autolaunch.disable();
        } catch (e) {
          dialog.showErrorBox('开机自启设置失败', String(e && e.message ? e.message : e));
        } finally {
          setTrayMenu();
        }
      },
    },
    {
      label: '关闭按钮最小化到托盘',
      type: 'checkbox',
      checked: !!cfg.minimizeToTray,
      click: (item) => {
        writeConfig({ ...cfg, minimizeToTray: !!item.checked });
        setTrayMenu();
      },
    },
    {
      label: '全局快捷键（显示/隐藏窗口）',
      type: 'checkbox',
      checked: !!cfg.globalShortcutEnabled,
      click: (item) => {
        const next = { ...cfg, globalShortcutEnabled: !!item.checked };
        writeConfig(next);
        applyGlobalShortcut(next);
        setTrayMenu();
      },
    },
    { type: 'separator' },
    {
      label: '打开数据目录',
      click: () => shell.openPath(app.getPath('userData')),
    },
    {
      label: '查看后端日志',
      click: () => shell.openPath(backendLogPath()),
    },
    { type: 'separator' },
    {
      label: '检查更新…',
      click: () => checkForUpdates(),
    },
    {
      label: '退出',
      click: async () => {
        app.quit();
      },
    },
  ];
  tray.setContextMenu(Menu.buildFromTemplate(template));
}

function applyGlobalShortcut(cfg) {
  try {
    globalShortcut.unregisterAll();
  } catch {
    // ignore
  }
  if (!cfg.globalShortcutEnabled) return;
  const acc = String(cfg.globalShortcut || '').trim();
  if (!acc) return;
  globalShortcut.register(acc, () => {
    if (!mainWindow) return;
    if (mainWindow.isVisible()) mainWindow.hide();
    else {
      mainWindow.show();
      mainWindow.focus();
    }
  });
}

async function exportClientConfig() {
  const defaultPath = path.join(app.getPath('downloads'), 'realms-client-config.txt');
  const { canceled, filePath } = await dialog.showSaveDialog({
    title: '导出客户端配置片段',
    defaultPath,
    buttonLabel: '保存',
  });
  if (canceled || !filePath) return;

  const content = [
    '# Realms Desktop（自用模式）客户端配置',
    '',
    `# base_url（固定）：${BASE_URL}/v1`,
    '# OPENAI_API_KEY：你的管理 Key（在 /login 设置；不会自动写入到此文件）',
    '',
    '## Linux/macOS（bash/zsh）',
    `export OPENAI_BASE_URL="${BASE_URL}/v1"`,
    'export OPENAI_API_KEY="<你的管理 Key>"',
    '',
    '## Windows（PowerShell）',
    `$env:OPENAI_BASE_URL = "${BASE_URL}/v1"`,
    '$env:OPENAI_API_KEY = "<你的管理 Key>"',
    '',
    '## Codex（config.toml 示例）',
    'disable_response_storage = true',
    'model_provider = "realms"',
    'model = "gpt-5.2"',
    '',
    '[model_providers.realms]',
    'name = "Realms Desktop"',
    `base_url = "${BASE_URL}/v1"`,
    'wire_api = "responses"',
    'requires_openai_auth = true',
    '',
  ].join(os.EOL);
  fs.writeFileSync(filePath, content, 'utf8');
  shell.showItemInFolder(filePath);
}

async function exportDBBackup() {
  const src = path.join(app.getPath('userData'), 'realms.db');
  if (!fs.existsSync(src)) {
    dialog.showMessageBox({ type: 'info', message: '尚未生成数据库文件（realms.db）。' });
    return;
  }
  const defaultPath = path.join(app.getPath('downloads'), 'realms.db');
  const { canceled, filePath } = await dialog.showSaveDialog({
    title: '导出数据库备份（realms.db）',
    defaultPath,
    buttonLabel: '保存',
  });
  if (canceled || !filePath) return;
  fs.copyFileSync(src, filePath);
  shell.showItemInFolder(filePath);
}

async function restartBackend(beforeStart) {
  await stopBackend();
  if (typeof beforeStart === 'function') {
    await beforeStart();
  }
  spawnBackend();
  await waitForBackendReady({ timeoutMs: 15_000 });
}

async function importDBBackup() {
  const { canceled, filePaths } = await dialog.showOpenDialog({
    title: '导入数据库备份（realms.db）',
    buttonLabel: '导入',
    properties: ['openFile'],
    filters: [{ name: 'SQLite DB', extensions: ['db'] }],
  });
  if (canceled || !filePaths || !filePaths[0]) return;

  const src = filePaths[0];
  const dst = path.join(app.getPath('userData'), 'realms.db');
  const res = await dialog.showMessageBox({
    type: 'warning',
    buttons: ['导入并重启', '取消'],
    defaultId: 0,
    cancelId: 1,
    message: '导入数据库会重启后端并覆盖当前数据。',
    detail: `源文件: ${src}\n目标文件: ${dst}`,
  });
  if (res.response !== 0) return;

  try {
    await restartBackend(async () => {
      fs.copyFileSync(src, dst);
    });
    if (mainWindow) await mainWindow.loadURL(`${BASE_URL}/login`);
  } catch (e) {
    dialog.showErrorBox('导入失败', String(e && e.message ? e.message : e));
  }
}

function normalizePathFromDeepLink(u) {
  try {
    const url = new URL(u);
    const qp = url.searchParams.get('path');
    let p = (qp && qp.trim()) || url.pathname || '/';
    if (!p.startsWith('/')) p = `/${p}`;
    return p;
  } catch {
    return '/';
  }
}

function handleDeepLink(u) {
  const p = normalizePathFromDeepLink(u);
  if (!mainWindow) {
    pendingDeepLink = p;
    return;
  }
  mainWindow.loadURL(`${BASE_URL}${p}`);
  mainWindow.show();
  mainWindow.focus();
}

async function checkForUpdates() {
  if (isDev) {
    dialog.showMessageBox({
      type: 'info',
      message: '开发模式下不检查更新。',
      detail: '打包后的应用会使用 GitHub Releases 作为更新源（需要签名/发布配置）。',
    });
    return;
  }

  try {
    autoUpdater.autoDownload = false;
    autoUpdater.removeAllListeners();

    autoUpdater.on('update-available', async (info) => {
      const res = await dialog.showMessageBox({
        type: 'info',
        buttons: ['下载更新', '稍后'],
        defaultId: 0,
        cancelId: 1,
        message: `发现新版本：${info.version}`,
        detail: '是否现在下载？',
      });
      if (res.response === 0) {
        autoUpdater.downloadUpdate();
      }
    });
    autoUpdater.on('update-not-available', () => {
      dialog.showMessageBox({ type: 'info', message: '已是最新版本。' });
    });
    autoUpdater.on('download-progress', (p) => {
      if (tray) tray.setToolTip(`Realms（下载更新中 ${Math.round(p.percent)}%）`);
    });
    autoUpdater.on('update-downloaded', async () => {
      const res = await dialog.showMessageBox({
        type: 'info',
        buttons: ['重启并安装', '稍后'],
        defaultId: 0,
        cancelId: 1,
        message: '更新已下载完成',
        detail: '是否现在重启并安装？',
      });
      if (res.response === 0) {
        autoUpdater.quitAndInstall();
      }
    });
    autoUpdater.on('error', (err) => {
      dialog.showErrorBox('检查更新失败', String(err && err.message ? err.message : err));
    });

    await autoUpdater.checkForUpdates();
  } finally {
    if (tray) tray.setToolTip('Realms');
  }
}

async function createMainWindow() {
  const cfg = readConfig();
  applyGlobalShortcut(cfg);

  mainWindow = new BrowserWindow({
    width: 1100,
    height: 760,
    show: false,
    webPreferences: {
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
    },
  });

  mainWindow.on('close', (e) => {
    const c = readConfig();
    if (c.minimizeToTray) {
      e.preventDefault();
      mainWindow.hide();
    }
  });

  const targetPath = pendingDeepLink || '/login';
  pendingDeepLink = null;
  await mainWindow.loadURL(`${BASE_URL}${targetPath}`);
  mainWindow.show();
}

function initTray() {
  tray = new Tray(trayIcon());
  tray.setToolTip('Realms');
  tray.on('double-click', () => {
    if (!mainWindow) return;
    if (mainWindow.isVisible()) mainWindow.hide();
    else {
      mainWindow.show();
      mainWindow.focus();
    }
  });
  setTrayMenu();
}

function initialDeepLinkFromArgv(argv) {
  const url = argv.find((x) => typeof x === 'string' && x.startsWith(`${DESKTOP_SCHEME}://`));
  return url || null;
}

async function bootstrap() {
  if (!ensureSingleInstance()) return;

  app.setAppUserModelId('com.flowerrealm.realms');
  if (!autolaunch) {
    autolaunch = new AutoLaunch({
      name: 'Realms',
      path: app.getPath('exe'),
    });
  }
  app.on('open-url', (event, url) => {
    event.preventDefault();
    handleDeepLink(url);
  });

  const initial = initialDeepLinkFromArgv(process.argv);
  if (initial) handleDeepLink(initial);

  try {
    app.setAsDefaultProtocolClient(DESKTOP_SCHEME);
  } catch {
    // ignore
  }

  initTray();
  try {
    const cfg = readConfig();
    if (cfg.autoLaunch) await autolaunch.enable();
    else await autolaunch.disable();
  } catch {
    // ignore
  }

  try {
    await assertPortAvailableOrThrow();
  } catch (e) {
    dialog.showErrorBox('启动失败', String(e && e.message ? e.message : e));
    app.quit();
    return;
  }

  try {
    spawnBackend();
  } catch (e) {
    dialog.showErrorBox('后端启动失败', String(e && e.message ? e.message : e));
    app.quit();
    return;
  }

  try {
    await waitForBackendReady({ timeoutMs: 15_000 });
  } catch (e) {
    const msg = [
      '后端未能在超时内就绪。',
      '',
      `固定端口：${BACKEND_HOST}:${BACKEND_PORT}`,
      '常见原因：端口被占用、或后端启动报错。',
      '',
      `你可以先查看后端日志：${backendLogPath()}`,
    ].join('\n');
    dialog.showErrorBox('启动失败', `${msg}\n\n${String(e && e.message ? e.message : e)}`);
    app.quit();
    return;
  }

  await createMainWindow();
  setTrayMenu();
}

app.on('before-quit', (event) => {
  if (isQuitting) return;
  event.preventDefault();
  isQuitting = true;
  try {
    globalShortcut.unregisterAll();
  } catch {
    // ignore
  }
  Promise.resolve()
    .then(() => stopBackend())
    .finally(() => app.exit(0));
});

app.on('window-all-closed', () => {
  // 有托盘时保持常驻（退出由托盘菜单触发）。
});

app.on('activate', () => {
  if (!mainWindow) return;
  mainWindow.show();
  mainWindow.focus();
});

app.whenReady().then(bootstrap);
