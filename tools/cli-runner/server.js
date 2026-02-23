'use strict';

const http = require('node:http');
const { execFile } = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');

const PORT = parseInt(process.env.PORT || '3100', 10);
const MAX_OUTPUT = parseInt(process.env.REALMS_CLI_RUNNER_MAX_OUTPUT || process.env.REALMS_CLI_RUNNER_MAX_OUTPUT_BYTES || String(8 * 1024 * 1024), 10);

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function resolveHomeRoot() {
  const sysTmp = path.resolve(os.tmpdir());

  const isTmpDir = (v) => {
    const p = path.resolve(v);
    if (p === sysTmp || p.startsWith(sysTmp + path.sep)) return true;
    if (p === '/tmp' || p.startsWith('/tmp' + path.sep)) return true;
    return false;
  };

  const fromEnv = (process.env.REALMS_CLI_RUNNER_HOME_ROOT || process.env.CLI_RUNNER_HOME_ROOT || '').trim();
  if (fromEnv && !isTmpDir(fromEnv)) {
    try {
      const v = path.resolve(fromEnv);
      fs.mkdirSync(v, { recursive: true });
      return v;
    } catch { /* fallthrough */ }
  }
  const candidates = [
    path.join(os.homedir(), '.realms-cli-runner'),
    '/root/.realms-cli-runner',
    '/app/.realms-cli-runner',
    path.join(process.cwd(), '.realms-cli-runner'),
  ];

  for (const c of candidates) {
    const v = path.resolve(c);
    if (isTmpDir(v)) continue;
    try {
      fs.mkdirSync(v, { recursive: true });
      return v;
    } catch { /* try next */ }
  }

  // Last resort (should not happen): fall back to a non-empty path.
  const fallback = path.join(os.homedir() || '/root', '.realms-cli-runner');
  try {
    fs.mkdirSync(fallback, { recursive: true });
    return fallback;
  } catch {
    return path.join(process.cwd(), '.realms-cli-runner');
  }
}

function tmpHome() {
  const root = resolveHomeRoot();
  fs.mkdirSync(root, { recursive: true });
  const dir = fs.mkdtempSync(path.join(root, 'cli-runner-'));
  return dir;
}

function cleanup(dir) {
  try {
    fs.rmSync(dir, { recursive: true, force: true });
  } catch { /* best effort */ }
}

function readBody(req) {
  return new Promise((resolve, reject) => {
    const chunks = [];
    let size = 0;
    req.on('data', (c) => {
      size += c.length;
      if (size > 1 << 20) { reject(new Error('body too large')); return; }
      chunks.push(c);
    });
    req.on('end', () => resolve(Buffer.concat(chunks).toString()));
    req.on('error', reject);
  });
}

function truncate(s, max) {
  if (!s) return '';
  return s.length <= max ? s : s.slice(0, max) + '…';
}

function joinOutput(stdout, stderr) {
  const out = (stdout || '').trimEnd();
  const err = (stderr || '').trimEnd();
  if (!out && !err) return '';
  if (!out) return err;
  if (!err) return out;
  return out + '\n' + err;
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

// ---------------------------------------------------------------------------
// CLI executors
// ---------------------------------------------------------------------------

function runCodex({ base_url, api_key, model, prompt, timeout_seconds }, home) {
  const codexDir = path.join(home, '.codex');
  fs.mkdirSync(codexDir, { recursive: true });
  const cacheDir = path.join(home, '.cache');
  fs.mkdirSync(cacheDir, { recursive: true });
  const tmpDir = path.join(home, 'tmp');
  fs.mkdirSync(tmpDir, { recursive: true });

  const provider = base_url ? 'custom' : 'openai';
  const providerBlock = base_url
    ? `\n[model_providers.custom]\nname = "Custom"\nbase_url = "${base_url}"\nwire_api = "responses"\nrequires_openai_auth = true\nenv_key = "OPENAI_API_KEY"\n`
    : '';

  fs.writeFileSync(path.join(codexDir, 'config.toml'), [
    'disable_response_storage = true',
    `model_provider = "${provider}"`,
    model ? `model = "${model}"` : '',
    providerBlock,
  ].join('\n'));

  const env = {
    ...process.env,
    HOME: home,
    CODEX_HOME: codexDir,
    OPENAI_API_KEY: api_key || '',
    CODEX_API_KEY: '',
    XDG_CACHE_HOME: cacheDir,
    TMPDIR: tmpDir,
    TMP: tmpDir,
    TEMP: tmpDir,
  };

  return new Promise((resolve) => {
    const startedAt = Date.now();
    const totalTimeoutMs = Math.max(1, (timeout_seconds || 30) * 1000);
    const deadlineMs = startedAt + totalTimeoutMs;

    const maxAttempts = 3;
    const retryDelayMs = 500;

    const attemptOnce = (attempt) => new Promise((r) => {
      const now = Date.now();
      const remainingMs = Math.max(1, deadlineMs - now);
      execFile('codex', ['exec', '--skip-git-repo-check', prompt || 'Reply with exactly: OK'], {
        env,
        cwd: home,
        timeout: remainingMs,
      }, (err, stdout, stderr) => {
        r({ attempt, err, stdout: stdout || '', stderr: stderr || '' });
      });
    });

    (async () => {
      const errors = [];
      for (let attempt = 1; attempt <= maxAttempts; attempt++) {
        const res = await attemptOnce(attempt);
        if (!res.err) {
          const latency_ms = Date.now() - startedAt;
          const out = joinOutput(res.stdout, res.stderr);
          resolve({ ok: true, latency_ms, output: truncate(out, MAX_OUTPUT), error: '' });
          return;
        }

        const errText = joinOutput(res.stdout, res.stderr) || (res.err && res.err.message) || '';
        errors.push(`attempt ${attempt}/${maxAttempts}\n${errText || '<no output>'}`);

        if (attempt < maxAttempts) {
          const remaining = deadlineMs - Date.now();
          if (remaining <= retryDelayMs) {
            break;
          }
          await sleep(retryDelayMs);
        }
      }

      const latency_ms = Date.now() - startedAt;
      resolve({ ok: false, latency_ms, output: '', error: truncate(errors.join('\n\n'), MAX_OUTPUT) });
    })().catch((e) => {
      const latency_ms = Date.now() - startedAt;
      resolve({ ok: false, latency_ms, output: '', error: truncate((e instanceof Error ? e.message : String(e)), MAX_OUTPUT) });
    });
  });
}

function runClaude({ base_url, api_key, model, prompt, timeout_seconds }, home) {
  const env = {
    ...process.env,
    HOME: home,
    ANTHROPIC_API_KEY: api_key || '',
  };
  if (base_url) env.ANTHROPIC_BASE_URL = base_url;

  const args = ['-p', prompt || 'Reply with exactly: OK', '--output-format', 'text'];
  if (model) args.push('--model', model);

  return new Promise((resolve) => {
    const start = Date.now();
    execFile('claude', args, {
      env,
      cwd: home,
      timeout: (timeout_seconds || 30) * 1000,
    }, (err, stdout, stderr) => {
      const latency_ms = Date.now() - start;
      const combined = joinOutput(stdout, stderr);
      if (err) {
        resolve({ ok: false, latency_ms, output: '', error: truncate(combined || err.message, MAX_OUTPUT) });
      } else {
        resolve({ ok: true, latency_ms, output: truncate(combined, MAX_OUTPUT), error: '' });
      }
    });
  });
}

function runGemini({ api_key, model, prompt, timeout_seconds }, home) {
  const env = {
    ...process.env,
    HOME: home,
    GEMINI_API_KEY: api_key || '',
  };

  const args = [prompt || 'Reply with exactly: OK'];
  if (model) args.push('-m', model);

  return new Promise((resolve) => {
    const start = Date.now();
    execFile('gemini', args, {
      env,
      cwd: home,
      timeout: (timeout_seconds || 30) * 1000,
    }, (err, stdout, stderr) => {
      const latency_ms = Date.now() - start;
      const combined = joinOutput(stdout, stderr);
      if (err) {
        resolve({ ok: false, latency_ms, output: '', error: truncate(combined || err.message, MAX_OUTPUT) });
      } else {
        resolve({ ok: true, latency_ms, output: truncate(combined, MAX_OUTPUT), error: '' });
      }
    });
  });
}

const runners = { codex: runCodex, claude: runClaude, gemini: runGemini };

// ---------------------------------------------------------------------------
// Health check: detect which CLIs are installed
// ---------------------------------------------------------------------------

function whichCLI(name) {
  return new Promise((resolve) => {
    execFile('which', [name], (err) => resolve(!err));
  });
}

async function healthPayload() {
  const [codex, claude, gemini] = await Promise.all([
    whichCLI('codex'),
    whichCLI('claude'),
    whichCLI('gemini'),
  ]);
  return {
    status: 'ok',
    cli: { codex, claude, gemini },
    cwd: process.cwd(),
    home_root: resolveHomeRoot(),
    max_output_bytes: MAX_OUTPUT,
  };
}

// ---------------------------------------------------------------------------
// HTTP server
// ---------------------------------------------------------------------------

const server = http.createServer(async (req, res) => {
  // healthz
  if (req.method === 'GET' && req.url === '/healthz') {
    const payload = await healthPayload();
    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify(payload));
    return;
  }

  // POST /v1/test
  if (req.method === 'POST' && req.url === '/v1/test') {
    let body;
    try { body = JSON.parse(await readBody(req)); } catch {
      res.writeHead(400, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ ok: false, error: 'invalid JSON body' }));
      return;
    }

    const { cli_type } = body;
    const runner = runners[cli_type];
    if (!runner) {
      res.writeHead(400, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ ok: false, error: `unsupported cli_type: ${cli_type}` }));
      return;
    }

    const home = tmpHome();
    try {
      const result = await runner(body, home);
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify(result));
    } catch (e) {
      res.writeHead(500, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ ok: false, error: truncate(e.message, MAX_OUTPUT) }));
    } finally {
      cleanup(home);
    }
    return;
  }

  res.writeHead(404, { 'Content-Type': 'application/json' });
  res.end(JSON.stringify({ error: 'not found' }));
});

server.listen(PORT, () => {
  console.log(`cli-runner listening on :${PORT}`);
});
