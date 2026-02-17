'use strict';

const http = require('node:http');
const { execFile } = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');

const PORT = parseInt(process.env.PORT || '3100', 10);
const MAX_OUTPUT = 1024;

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function tmpHome() {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'cli-runner-'));
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

// ---------------------------------------------------------------------------
// CLI executors
// ---------------------------------------------------------------------------

function runCodex({ base_url, api_key, model, prompt, timeout_seconds }, home) {
  const codexDir = path.join(home, '.codex');
  fs.mkdirSync(codexDir, { recursive: true });

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
    OPENAI_API_KEY: api_key || '',
    CODEX_API_KEY: '',
  };

  return new Promise((resolve) => {
    const start = Date.now();
    execFile('codex', ['exec', '--skip-git-repo-check', prompt || 'Reply with exactly: OK'], {
      env,
      cwd: home,
      timeout: (timeout_seconds || 30) * 1000,
    }, (err, stdout, stderr) => {
      const latency_ms = Date.now() - start;
      if (err) {
        resolve({ ok: false, latency_ms, output: '', error: truncate((stderr || err.message), MAX_OUTPUT) });
      } else {
        resolve({ ok: true, latency_ms, output: truncate(stdout.trim(), MAX_OUTPUT), error: '' });
      }
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
      if (err) {
        resolve({ ok: false, latency_ms, output: '', error: truncate((stderr || err.message), MAX_OUTPUT) });
      } else {
        resolve({ ok: true, latency_ms, output: truncate(stdout.trim(), MAX_OUTPUT), error: '' });
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
      if (err) {
        resolve({ ok: false, latency_ms, output: '', error: truncate((stderr || err.message), MAX_OUTPUT) });
      } else {
        resolve({ ok: true, latency_ms, output: truncate(stdout.trim(), MAX_OUTPUT), error: '' });
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
  return { status: 'ok', cli: { codex, claude, gemini } };
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
