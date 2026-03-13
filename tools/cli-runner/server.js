'use strict';

const http = require('node:http');
const https = require('node:https');
const { execFile, spawn } = require('node:child_process');
const crypto = require('node:crypto');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');

const PORT = parseInt(process.env.PORT || '3100', 10);
const MAX_OUTPUT = parseInt(
  process.env.REALMS_CLI_RUNNER_MAX_OUTPUT ||
    process.env.REALMS_CLI_RUNNER_MAX_OUTPUT_BYTES ||
    String(8 * 1024 * 1024),
  10,
);

const DEFAULT_PROMPT = 'Reply with exactly: OK';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function sha256Hex(s) {
  return crypto.createHash('sha256').update(String(s || '')).digest('hex');
}

function trimString(v) {
  return String(v || '').trim();
}

function resolveHomeRoot() {
  const sysTmp = path.resolve(os.tmpdir());

  const isTmpDir = (v) => {
    const p = path.resolve(v);
    if (p === sysTmp || p.startsWith(sysTmp + path.sep)) return true;
    if (p === '/tmp' || p.startsWith('/tmp' + path.sep)) return true;
    return false;
  };

  const fromEnv = trimString(process.env.REALMS_CLI_RUNNER_HOME_ROOT || process.env.CLI_RUNNER_HOME_ROOT);
  if (fromEnv && !isTmpDir(fromEnv)) {
    try {
      const v = path.resolve(fromEnv);
      fs.mkdirSync(v, { recursive: true });
      return v;
    } catch {
      // fall through
    }
  }

  const candidates = [
    path.join(os.homedir(), '.realms-cli-runner'),
    '/root/.realms-cli-runner',
    '/app/.realms-cli-runner',
    path.join(process.cwd(), '.realms-cli-runner'),
  ];
  for (const candidate of candidates) {
    const resolved = path.resolve(candidate);
    if (isTmpDir(resolved)) continue;
    try {
      fs.mkdirSync(resolved, { recursive: true });
      return resolved;
    } catch {
      // try next
    }
  }

  const fallback = path.join(os.homedir() || '/root', '.realms-cli-runner');
  try {
    fs.mkdirSync(fallback, { recursive: true });
    return fallback;
  } catch {
    return path.join(process.cwd(), '.realms-cli-runner');
  }
}

function ensureDir(p) {
  const v = path.resolve(p);
  fs.mkdirSync(v, { recursive: true });
  return v;
}

function resolveStateRoot() {
  return ensureDir(resolveHomeRoot());
}

function resolveWorkRoot() {
  const fromEnv = trimString(process.env.REALMS_CLI_RUNNER_WORK_ROOT || process.env.CLI_RUNNER_WORK_ROOT);
  if (fromEnv) {
    try {
      return ensureDir(fromEnv);
    } catch {
      // fall through
    }
  }
  return ensureDir(path.join(resolveStateRoot(), 'work'));
}

function profileDirs(cliType, baseURL, model) {
  const key = [String(cliType || ''), String(baseURL || ''), String(model || '')].join('|');
  const id = sha256Hex(key).slice(0, 24);
  const stateRoot = resolveStateRoot();
  return {
    id,
    stateRoot,
    codexHome: ensureDir(path.join(stateRoot, 'codex', id)),
    xdgCacheHome: ensureDir(path.join(stateRoot, 'xdg-cache', id)),
  };
}

function tmpWorkDir() {
  return fs.mkdtempSync(path.join(resolveWorkRoot(), 'cli-runner-'));
}

function cleanup(dir) {
  try {
    fs.rmSync(dir, { recursive: true, force: true });
  } catch {
    // best effort
  }
}

function readBody(req) {
  return new Promise((resolve, reject) => {
    const chunks = [];
    let size = 0;
    req.on('data', (chunk) => {
      size += chunk.length;
      if (size > (1 << 20)) {
        reject(new Error('body too large'));
        return;
      }
      chunks.push(chunk);
    });
    req.on('end', () => resolve(Buffer.concat(chunks).toString('utf8')));
    req.on('error', reject);
  });
}

function readBodyBuffer(req) {
  return new Promise((resolve, reject) => {
    const chunks = [];
    let size = 0;
    req.on('data', (chunk) => {
      size += chunk.length;
      if (size > (4 << 20)) {
        reject(new Error('body too large'));
        return;
      }
      chunks.push(chunk);
    });
    req.on('end', () => resolve(Buffer.concat(chunks)));
    req.on('error', reject);
  });
}

function truncate(s, max = MAX_OUTPUT) {
  if (!s) return '';
  return s.length <= max ? s : s.slice(0, max) + '…';
}

function joinOutput(stdout, stderr) {
  const out = trimString(stdout);
  const err = trimString(stderr);
  if (!out && !err) return '';
  if (!out) return err;
  if (!err) return out;
  return out + '\n' + err;
}

function sanitizePrompt(prompt) {
  return trimString(prompt) || DEFAULT_PROMPT;
}

function safeJSONParse(line) {
  const text = trimString(line).replace(/^data:\s*/, '');
  if (!text || text === '[DONE]') return null;
  try {
    return JSON.parse(text);
  } catch {
    return null;
  }
}

function getPath(obj, parts) {
  let current = obj;
  for (const part of parts) {
    if (!current || typeof current !== 'object') return undefined;
    current = current[part];
  }
  return current;
}

function normalizeTextValue(value) {
  if (typeof value === 'string') {
    const trimmed = value;
    return trimmed ? [trimmed] : [];
  }
  if (Array.isArray(value)) {
    const out = [];
    for (const item of value) out.push(...normalizeTextValue(item));
    return out;
  }
  if (!value || typeof value !== 'object') return [];

  if (typeof value.text === 'string' && value.text) return [value.text];
  if (typeof value.output_text === 'string' && value.output_text) return [value.output_text];
  if (typeof value.result === 'string' && value.result) return [value.result];
  if (typeof value.completion === 'string' && value.completion) return [value.completion];
  if (typeof value.value === 'string' && /text/i.test(String(value.type || ''))) return [value.value];
  if (typeof value.content === 'string' && (!value.role || String(value.role).toLowerCase() === 'assistant')) return [value.content];

  const out = [];
  if (Array.isArray(value.content)) out.push(...normalizeTextValue(value.content));
  if (value.delta) out.push(...normalizeTextValue(value.delta));
  if (value.message) out.push(...normalizeTextValue(value.message));
  if (value.response) out.push(...normalizeTextValue(value.response));
  if (value.content_block) out.push(...normalizeTextValue(value.content_block));
  return out;
}

function uniqStrings(values) {
  const seen = new Set();
  const out = [];
  for (const value of values) {
    const text = String(value || '');
    if (!text || seen.has(text)) continue;
    seen.add(text);
    out.push(text);
  }
  return out;
}

function extractTextFragments(event) {
  if (!event || typeof event !== 'object') return [];

  const candidates = [
    getPath(event, ['delta', 'text']),
    getPath(event, ['content_block', 'text']),
    getPath(event, ['message', 'content']),
    getPath(event, ['response', 'output']),
    getPath(event, ['response', 'content']),
    event.output_text,
    event.result,
    event.completion,
    event.text,
    event.content,
  ];

  const fragments = [];
  for (const candidate of candidates) fragments.push(...normalizeTextValue(candidate));
  return uniqStrings(fragments);
}

function findNestedModel(value) {
  if (!value || typeof value !== 'object') return '';
  const explicit = [
    getPath(value, ['response', 'model']),
    getPath(value, ['message', 'model']),
    getPath(value, ['data', 'model']),
    getPath(value, ['response', 'response', 'model']),
  ];
  for (const candidate of explicit) {
    const model = trimString(candidate);
    if (model) return model;
  }
  const type = trimString(value.type || value.event).toLowerCase();
  if ((type.includes('response') || type.includes('message')) && trimString(value.model)) {
    return trimString(value.model);
  }
  return '';
}

function extractErrorText(event) {
  if (!event || typeof event !== 'object') return '';
  if (typeof event.error === 'string') return trimString(event.error);
  if (event.error && typeof event.error === 'object') {
    if (typeof event.error.message === 'string') return trimString(event.error.message);
    if (typeof event.error.error === 'string') return trimString(event.error.error);
  }
  const type = trimString(event.type || event.event).toLowerCase();
  if (type.includes('error') && typeof event.message === 'string') return trimString(event.message);
  if (type.includes('error') && typeof event.text === 'string') return trimString(event.text);
  return '';
}

function parseStructuredEvents(kind, timedLines, stdoutText, stderrText) {
  let ttftMS = 0;
  let responseModel = '';
  let errorText = '';
  const outputParts = [];
  let parsedAnyJSON = false;

  for (const timedLine of timedLines) {
    const event = safeJSONParse(timedLine.line);
    if (!event) continue;
    parsedAnyJSON = true;

    const model = findNestedModel(event);
    if (model) responseModel = model;

    const fragments = extractTextFragments(event);
    if (fragments.length > 0) {
      if (ttftMS <= 0) ttftMS = timedLine.at_ms;
      outputParts.push(...fragments);
    }

    const maybeError = extractErrorText(event);
    if (maybeError) errorText = maybeError;
  }

  let output = uniqStrings(outputParts).join('');
  if (!output && !parsedAnyJSON) {
    output = trimString(stdoutText);
  }
  if (!output && kind === 'text') {
    output = trimString(stdoutText);
  }

  return {
    output: truncate(output),
    ttft_ms: ttftMS,
    response_model: responseModel,
    error: truncate(errorText || trimString(stderrText)),
  };
}

function createStructuredOutputParser(kind) {
  const timedLines = [];
  const collector = createTimedLineCollector((line, atMS) => {
    timedLines.push({ line, raw_at_ms: atMS });
  });
  return {
    consumeStdout(text, atMS) {
      collector.push(text, atMS);
    },
    finish({ stdout, stderr, startedAt, endedAt }) {
      const flushAt = typeof startedAt === 'number' && typeof endedAt === 'number'
        ? Math.max(0, endedAt - startedAt)
        : 0;
      collector.flush(flushAt);
      const normalizedLines = timedLines.map((item) => ({
        line: item.line,
        at_ms: typeof startedAt === 'number' ? Math.max(0, item.raw_at_ms - startedAt) : item.raw_at_ms,
      }));
      const parsed = parseStructuredEvents(kind, normalizedLines, stdout, stderr);
      return {
        output: parsed.output,
        ttft_ms: parsed.ttft_ms,
        metadata: {
          upstream_response_model: parsed.response_model,
        },
      };
    },
  };
}

function createCodexOutputParser() {
  return createStructuredOutputParser('codex');
}

function createClaudeOutputParser() {
  return createStructuredOutputParser('claude');
}

function extractResponseModelFromJSON(value) {
  if (!value || typeof value !== 'object') return '';
  const direct = [
    getPath(value, ['model']),
    getPath(value, ['response', 'model']),
    getPath(value, ['message', 'model']),
    getPath(value, ['data', 'model']),
  ];
  for (const candidate of direct) {
    const model = trimString(candidate);
    if (model) return model;
  }
  for (const nestedKey of ['response', 'message', 'data']) {
    const model = extractResponseModelFromJSON(value[nestedKey]);
    if (model) return model;
  }
  if (Array.isArray(value.output)) {
    for (const item of value.output) {
      const model = extractResponseModelFromJSON(item);
      if (model) return model;
    }
  }
  return '';
}

function extractResponseModelFromBody(bodyBuffer, contentType) {
  const bodyText = bodyBuffer.toString('utf8');
  const parsedJSON = safeJSONParse(bodyText);
  if (parsedJSON) {
    const model = extractResponseModelFromJSON(parsedJSON);
    if (model) return model;
  }

  if (String(contentType || '').includes('text/event-stream') || bodyText.includes('\ndata:')) {
    const lines = bodyText.split(/\r?\n/);
    let model = '';
    for (const line of lines) {
      if (!line.startsWith('data:')) continue;
      const event = safeJSONParse(line);
      if (!event) continue;
      const found = extractResponseModelFromJSON(event);
      if (found) model = found;
    }
    return model;
  }

  return '';
}

function extractRequestModelFromBody(bodyBuffer) {
  const payload = safeJSONParse(bodyBuffer.toString('utf8'));
  if (!payload || typeof payload !== 'object') return '';
  return trimString(payload.model);
}

function normalizeTrackedPath(kind, rawPath) {
  const pathname = new URL(rawPath || '/', 'http://127.0.0.1').pathname;
  if (kind === 'openai') {
    if (pathname.endsWith('/v1/responses') || pathname.endsWith('/responses')) return '/v1/responses';
    if (pathname.endsWith('/v1/chat/completions') || pathname.endsWith('/chat/completions')) return '/v1/chat/completions';
    return '';
  }
  if (kind === 'anthropic') {
    if (pathname.endsWith('/v1/messages') || pathname.endsWith('/messages')) return '/v1/messages';
    return '';
  }
  return '';
}

function joinTargetURL(baseURL, incomingPath) {
  const base = new URL(baseURL);
  const incoming = new URL(incomingPath || '/', 'http://127.0.0.1');
  const joined = new URL(base.toString());
  const basePath = base.pathname === '/' ? '' : base.pathname.replace(/\/+$/, '');
  const reqPath = incoming.pathname.startsWith('/') ? incoming.pathname : `/${incoming.pathname}`;
  joined.pathname = `${basePath}${reqPath}`.replace(/\/{2,}/g, '/');
  joined.search = incoming.search;
  return joined;
}

function closeServer(server) {
  return new Promise((resolve) => {
    if (!server) {
      resolve();
      return;
    }
    server.close(() => resolve());
  });
}

function listenServer(server) {
  return new Promise((resolve, reject) => {
    server.once('error', reject);
    server.listen(0, '127.0.0.1', () => {
      server.off('error', reject);
      resolve(server.address());
    });
  });
}

async function startTrackingProxy({ targetBaseURL, kind }) {
  const metadata = {
    success_path: '',
    used_fallback: false,
    forwarded_model: '',
    upstream_response_model: '',
    attempts: [],
  };

  const server = http.createServer(async (req, res) => {
    let bodyBuffer;
    try {
      bodyBuffer = await readBodyBuffer(req);
    } catch (error) {
      res.writeHead(413, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ error: error instanceof Error ? error.message : String(error) }));
      return;
    }

    const trackedPath = normalizeTrackedPath(kind, req.url);
    const requestModel = extractRequestModelFromBody(bodyBuffer);
    if (requestModel) metadata.forwarded_model = requestModel;
    if (trackedPath) metadata.attempts.push(trackedPath);

    let targetURL;
    try {
      targetURL = joinTargetURL(targetBaseURL, req.url);
    } catch (error) {
      res.writeHead(500, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ error: error instanceof Error ? error.message : String(error) }));
      return;
    }

    const headers = { ...req.headers };
    headers.host = targetURL.host;
    headers['accept-encoding'] = 'identity';
    if (!bodyBuffer.length) delete headers['content-length'];

    const transport = targetURL.protocol === 'https:' ? https : http;
    const upstreamReq = transport.request(targetURL, {
      method: req.method,
      headers,
    }, (upstreamRes) => {
      const responseChunks = [];
      res.writeHead(upstreamRes.statusCode || 502, upstreamRes.headers);
      upstreamRes.on('data', (chunk) => {
        responseChunks.push(chunk);
        res.write(chunk);
      });
      upstreamRes.on('end', () => {
        res.end();
        if ((upstreamRes.statusCode || 500) < 200 || (upstreamRes.statusCode || 500) >= 300 || !trackedPath) {
          return;
        }
        metadata.success_path = trackedPath;
        metadata.used_fallback =
          trackedPath === '/v1/chat/completions' && metadata.attempts.includes('/v1/responses');
        const responseModel = extractResponseModelFromBody(
          Buffer.concat(responseChunks),
          upstreamRes.headers['content-type'],
        );
        if (responseModel) metadata.upstream_response_model = responseModel;
      });
      upstreamRes.on('error', () => {
        res.destroy();
      });
    });

    upstreamReq.on('error', (error) => {
      if (res.headersSent) {
        res.destroy(error);
        return;
      }
      res.writeHead(502, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ error: error.message }));
    });

    if (bodyBuffer.length > 0) upstreamReq.write(bodyBuffer);
    upstreamReq.end();
  });

  const address = await listenServer(server);
  return {
    metadata,
    baseURL: `http://127.0.0.1:${address.port}`,
    async close() {
      await closeServer(server);
    },
  };
}

function createTimedLineCollector(onLine) {
  let buffered = '';
  return {
    push(text, atMS) {
      buffered += text;
      for (;;) {
        const idx = buffered.indexOf('\n');
        if (idx < 0) break;
        const line = buffered.slice(0, idx).replace(/\r$/, '');
        buffered = buffered.slice(idx + 1);
        onLine(line, atMS);
      }
    },
    flush(atMS) {
      const line = buffered.replace(/\r$/, '');
      buffered = '';
      if (line) onLine(line, atMS);
    },
  };
}

function runSpawnedCommand({
  command,
  args,
  env,
  cwd,
  timeoutMS,
  closeStdin = true,
  spawnImpl = spawn,
}) {
  return new Promise((resolve) => {
    const startedAt = Date.now();
    let settled = false;
    let timedOut = false;
    let stdout = '';
    let stderr = '';
    const timedLines = [];
    const lineCollector = createTimedLineCollector((line, atMS) => {
      timedLines.push({ line, at_ms: atMS });
    });

    const finalize = (result) => {
      if (settled) return;
      settled = true;
      resolve(result);
    };

    const child = spawnImpl(command, args, {
      env,
      cwd,
      stdio: ['pipe', 'pipe', 'pipe'],
    });

    const timer = setTimeout(() => {
      timedOut = true;
      try {
        child.kill('SIGKILL');
      } catch {
        // best effort
      }
    }, timeoutMS);

    const finish = (exitCode, signal, spawnError) => {
      clearTimeout(timer);
      lineCollector.flush(Date.now() - startedAt);
      finalize({
        ok: !timedOut && !spawnError && exitCode === 0,
        exit_code: exitCode,
        signal,
        timed_out: timedOut,
        latency_ms: Date.now() - startedAt,
        stdout,
        stderr,
        timed_lines: timedLines,
        spawn_error: spawnError || null,
      });
    };

    child.stdout.on('data', (chunk) => {
      const text = chunk.toString('utf8');
      stdout += text;
      lineCollector.push(text, Date.now() - startedAt);
    });
    child.stderr.on('data', (chunk) => {
      stderr += chunk.toString('utf8');
    });

    child.once('error', (error) => finish(null, null, error));
    child.once('close', (code, signal) => finish(code, signal, null));

    if (closeStdin && child.stdin) {
      try {
        child.stdin.end();
      } catch {
        // best effort
      }
    }
  });
}

function buildStructuredResult(execResult, parsedResult, fallbackModel, metadata, allowParserResponseModel) {
  const forwardedModel =
    trimString(metadata && metadata.forwarded_model) ||
    trimString(fallbackModel);
  const upstreamResponseModel =
    trimString(metadata && metadata.upstream_response_model) ||
    (allowParserResponseModel ? trimString(parsedResult.response_model) : '');
  const successPath = trimString(metadata && metadata.success_path);
  const usedFallback = Boolean(metadata && metadata.used_fallback);

  const errorText = execResult.ok
    ? ''
    : truncate(
      parsedResult.error ||
      joinOutput(execResult.stdout, execResult.stderr) ||
      (execResult.spawn_error && execResult.spawn_error.message) ||
      (execResult.timed_out ? `command timed out after ${execResult.latency_ms}ms` : 'command failed'),
    );

  return {
    ok: execResult.ok,
    latency_ms: execResult.latency_ms,
    ttft_ms: parsedResult.ttft_ms > 0 ? parsedResult.ttft_ms : 0,
    output: truncate(parsedResult.output),
    error: errorText,
    success_path: successPath,
    used_fallback: usedFallback,
    forwarded_model: forwardedModel,
    upstream_response_model: upstreamResponseModel,
  };
}

function finalizeRunnerResult(execResult, { requestedModel, capture }) {
  const metadata = capture || (execResult && execResult.metadata) || {};
  const forwardedModel = trimString(metadata.forwarded_model) || trimString(requestedModel);
  const upstreamResponseModel = trimString(metadata.upstream_response_model);
  const successPath = trimString(metadata.success_path);
  const usedFallback = Boolean(metadata.used_fallback);
  const ttftMS = execResult && execResult.ok
    ? (execResult.ttft_ms > 0 ? execResult.ttft_ms : execResult.latency_ms)
    : (execResult && execResult.ttft_ms > 0 ? execResult.ttft_ms : 0);
  return {
    ok: Boolean(execResult && execResult.ok),
    latency_ms: (execResult && execResult.latency_ms) || 0,
    ttft_ms: ttftMS > 0 ? ttftMS : undefined,
    output: (execResult && execResult.output) || '',
    error: (execResult && execResult.error) || '',
    success_path: successPath || undefined,
    used_fallback: usedFallback || undefined,
    forwarded_model: forwardedModel || undefined,
    upstream_response_model: upstreamResponseModel || undefined,
  };
}

function codexConfig(provider, model, baseURL) {
  const providerBlock = provider === 'custom'
    ? `\n[model_providers.custom]\nname = "Custom"\nbase_url = "${baseURL}"\nwire_api = "responses"\nrequires_openai_auth = true\nenv_key = "OPENAI_API_KEY"\n`
    : '';
  return [
    'disable_response_storage = true',
    `model_provider = "${provider}"`,
    model ? `model = "${model}"` : '',
    providerBlock,
  ].join('\n');
}

async function withOptionalProxy(baseURL, kind, fn) {
  const targetBaseURL = trimString(baseURL);
  if (!targetBaseURL) {
    return fn(null);
  }
  const proxy = await startTrackingProxy({ targetBaseURL, kind });
  try {
    return await fn(proxy);
  } finally {
    await proxy.close();
  }
}

async function runCodexAttempt({ base_url, api_key, model, prompt, timeout_seconds, _paths }, home, spawnImpl) {
  const paths = _paths || {};
  const codexDir = paths.codexHome ? path.resolve(paths.codexHome) : ensureDir(path.join(home, '.codex'));
  const cacheDir = paths.xdgCacheHome ? path.resolve(paths.xdgCacheHome) : ensureDir(path.join(home, '.cache'));
  const configDir = paths.xdgConfigHome ? path.resolve(paths.xdgConfigHome) : ensureDir(path.join(home, '.config'));
  const tmpDir = ensureDir(path.join(home, 'tmp'));

  return withOptionalProxy(base_url, 'openai', async (proxy) => {
    const configuredBaseURL = proxy ? proxy.baseURL : trimString(base_url);
    const provider = configuredBaseURL ? 'custom' : 'openai';
    fs.writeFileSync(path.join(codexDir, 'config.toml'), codexConfig(provider, model, configuredBaseURL));

    const env = {
      ...process.env,
      HOME: home,
      CODEX_HOME: codexDir,
      OPENAI_API_KEY: api_key || '',
      CODEX_API_KEY: '',
      XDG_CACHE_HOME: cacheDir,
      XDG_CONFIG_HOME: configDir,
      TMPDIR: tmpDir,
      TMP: tmpDir,
      TEMP: tmpDir,
    };

    const execResult = await runSpawnedCommand({
      command: 'codex',
      args: ['exec', '--skip-git-repo-check', '--json', sanitizePrompt(prompt)],
      env,
      cwd: home,
      timeoutMS: Math.max(1, (timeout_seconds || 30) * 1000),
      spawnImpl,
    });
    const parsedResult = parseStructuredEvents('codex', execResult.timed_lines, execResult.stdout, execResult.stderr);
    return buildStructuredResult(execResult, parsedResult, model, proxy && proxy.metadata, false);
  });
}

async function runCodex(body, home, options = {}) {
  const spawnImpl = options.spawnImpl || spawn;
  return runCodexAttempt(body, home, spawnImpl);
}

async function runCodexOAuth({ access_token, refresh_token, id_token, chatgpt_account_id, model, prompt, timeout_seconds, _paths }, home, options = {}) {
  const paths = _paths || {};
  const codexDir = paths.codexHome ? path.resolve(paths.codexHome) : ensureDir(path.join(home, '.codex'));
  const cacheDir = paths.xdgCacheHome ? path.resolve(paths.xdgCacheHome) : ensureDir(path.join(home, '.cache'));
  const configDir = paths.xdgConfigHome ? path.resolve(paths.xdgConfigHome) : ensureDir(path.join(home, '.config'));
  const tmpDir = ensureDir(path.join(home, 'tmp'));
  const spawnImpl = options.spawnImpl || spawn;

  const accessToken = trimString(access_token);
  const refreshToken = trimString(refresh_token);
  const idToken = trimString(id_token);
  const accountID = trimString(chatgpt_account_id);

  if (!accessToken || !refreshToken || !idToken || !accountID) {
    return { ok: false, latency_ms: 0, output: '', error: 'missing required oauth tokens' };
  }

  fs.writeFileSync(path.join(codexDir, 'auth.json'), JSON.stringify({
    auth_mode: 'chatgpt',
    tokens: {
      id_token: idToken,
      access_token: accessToken,
      refresh_token: refreshToken,
      account_id: accountID,
    },
    last_refresh: new Date().toISOString(),
  }, null, 2), { mode: 0o600 });

  fs.writeFileSync(path.join(codexDir, 'config.toml'), codexConfig('openai', model, ''));

  const env = {
    ...process.env,
    HOME: home,
    CODEX_HOME: codexDir,
    OPENAI_API_KEY: '',
    CODEX_API_KEY: '',
    XDG_CACHE_HOME: cacheDir,
    XDG_CONFIG_HOME: configDir,
    TMPDIR: tmpDir,
    TMP: tmpDir,
    TEMP: tmpDir,
  };

  const execResult = await runSpawnedCommand({
    command: 'codex',
    args: ['exec', '--skip-git-repo-check', '--json', sanitizePrompt(prompt)],
    env,
    cwd: home,
    timeoutMS: Math.max(1, (timeout_seconds || 30) * 1000),
    spawnImpl,
  });
  const parsedResult = parseStructuredEvents('codex', execResult.timed_lines, execResult.stdout, execResult.stderr);
  return buildStructuredResult(execResult, parsedResult, model, null, true);
}

async function runClaude({ base_url, api_key, model, prompt, timeout_seconds, _paths }, home, options = {}) {
  const paths = _paths || {};
  const cacheDir = paths.xdgCacheHome ? path.resolve(paths.xdgCacheHome) : undefined;
  const configDir = paths.xdgConfigHome ? path.resolve(paths.xdgConfigHome) : undefined;
  const spawnImpl = options.spawnImpl || spawn;

  return withOptionalProxy(base_url, 'anthropic', async (proxy) => {
    const env = {
      ...process.env,
      HOME: home,
      ANTHROPIC_API_KEY: api_key || '',
    };
    if (cacheDir) env.XDG_CACHE_HOME = cacheDir;
    if (configDir) env.XDG_CONFIG_HOME = configDir;
    if (proxy) env.ANTHROPIC_BASE_URL = proxy.baseURL;
    else if (trimString(base_url)) env.ANTHROPIC_BASE_URL = trimString(base_url);

    const args = ['-p', sanitizePrompt(prompt), '--output-format', 'stream-json'];
    if (model) args.push('--model', model);

    const execResult = await runSpawnedCommand({
      command: 'claude',
      args,
      env,
      cwd: home,
      timeoutMS: Math.max(1, (timeout_seconds || 30) * 1000),
      closeStdin: true,
      spawnImpl,
    });
    const parsedResult = parseStructuredEvents('claude', execResult.timed_lines, execResult.stdout, execResult.stderr);
    return buildStructuredResult(execResult, parsedResult, model, proxy && proxy.metadata, false);
  });
}

function runGemini({ api_key, model, prompt, timeout_seconds, _paths }, home) {
  const paths = _paths || {};
  const cacheDir = paths.xdgCacheHome ? path.resolve(paths.xdgCacheHome) : undefined;
  const configDir = paths.xdgConfigHome ? path.resolve(paths.xdgConfigHome) : undefined;
  const env = {
    ...process.env,
    HOME: home,
    GEMINI_API_KEY: api_key || '',
  };
  if (cacheDir) env.XDG_CACHE_HOME = cacheDir;
  if (configDir) env.XDG_CONFIG_HOME = configDir;

  const args = ['-p', sanitizePrompt(prompt), '-o', 'text'];
  if (model) args.push('-m', model);

  return new Promise((resolve) => {
    const startedAt = Date.now();
    const child = execFile('gemini', args, {
      env,
      cwd: home,
      timeout: Math.max(1, (timeout_seconds || 30) * 1000),
    }, (err, stdout, stderr) => {
      const latencyMS = Date.now() - startedAt;
      const combined = joinOutput(stdout, stderr);
      if (err) {
        resolve({ ok: false, latency_ms: latencyMS, output: '', error: truncate(combined || err.message) });
        return;
      }
      resolve({
        ok: true,
        latency_ms: latencyMS,
        output: truncate(combined),
        error: '',
        forwarded_model: trimString(model),
      });
    });
    try {
      child.stdin && child.stdin.end();
    } catch {
      // best effort
    }
  });
}

const runners = {
  codex: runCodex,
  codex_oauth: runCodexOAuth,
  claude: runClaude,
  gemini: runGemini,
};

// ---------------------------------------------------------------------------
// Health check
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
    state_root: resolveStateRoot(),
    work_root: resolveWorkRoot(),
    max_output_bytes: MAX_OUTPUT,
  };
}

// ---------------------------------------------------------------------------
// HTTP server
// ---------------------------------------------------------------------------

function createServer(options = {}) {
  const spawnImpl = options.spawnImpl || spawn;

  return http.createServer(async (req, res) => {
    if (req.method === 'GET' && req.url === '/healthz') {
      const payload = await healthPayload();
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify(payload));
      return;
    }

    if (req.method === 'POST' && req.url === '/v1/test') {
      let body;
      try {
        body = JSON.parse(await readBody(req));
      } catch {
        res.writeHead(400, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ ok: false, error: 'invalid JSON body' }));
        return;
      }

      const runner = runners[body.cli_type];
      if (!runner) {
        res.writeHead(400, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ ok: false, error: `unsupported cli_type: ${body.cli_type}` }));
        return;
      }

      const home = tmpWorkDir();
      const profileKey = trimString(body.profile_key) || trimString(body.base_url);
      const dirs = profileDirs(body.cli_type, profileKey, body.model);
      const configDir = ensureDir(path.join(home, 'xdg-config'));
      body._paths = {
        codexHome: dirs.codexHome,
        xdgCacheHome: dirs.xdgCacheHome,
        xdgConfigHome: configDir,
      };

      try {
        const result = await runner(body, home, { spawnImpl });
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify(result));
      } catch (error) {
        res.writeHead(500, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ ok: false, error: truncate(error instanceof Error ? error.message : String(error)) }));
      } finally {
        cleanup(home);
      }
      return;
    }

    res.writeHead(404, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ error: 'not found' }));
  });
}

if (require.main === module) {
  const server = createServer();
  server.listen(PORT, () => {
    console.log(`cli-runner listening on :${PORT}`);
  });
}

module.exports = {
  DEFAULT_PROMPT,
  MAX_OUTPUT,
  createClaudeOutputParser,
  createCodexOutputParser,
  createServer,
  createTimedLineCollector,
  extractRequestModelFromBody,
  extractResponseModelFromBody,
  finalizeRunnerResult,
  joinTargetURL,
  normalizeTrackedPath,
  parseStructuredEvents,
  profileDirs,
  readBody,
  resolveStateRoot,
  resolveWorkRoot,
  runClaude,
  runCodex,
  runCodexOAuth,
  runGemini,
  runSpawnedCommand,
  safeJSONParse,
  startTrackingProxy,
  tmpWorkDir,
  truncate,
};
