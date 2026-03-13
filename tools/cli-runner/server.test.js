'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const http = require('node:http');

const {
  createClaudeOutputParser,
  createCodexOutputParser,
  finalizeRunnerResult,
  startTrackingProxy,
} = require('./server');

function listen(server) {
  return new Promise((resolve, reject) => {
    server.once('error', reject);
    server.listen(0, '127.0.0.1', () => {
      const address = server.address();
      resolve({
        server,
        url: `http://127.0.0.1:${address.port}`,
      });
    });
  });
}

async function closeServer(server) {
  if (!server) return;
  await new Promise((resolve) => server.close(resolve));
}

test('proxy captures openai responses metadata', async () => {
  const upstream = await listen(http.createServer((req, res) => {
    assert.equal(req.url, '/v1/responses');
    let body = '';
    req.on('data', (chunk) => { body += chunk.toString(); });
    req.on('end', () => {
      assert.equal(JSON.parse(body).model, 'gpt-5');
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ model: 'gpt-5-mini' }));
    });
  }));

  const proxy = await startTrackingProxy({ targetBaseURL: upstream.url, kind: 'openai' });
  try {
    const resp = await fetch(`${proxy.baseURL}/v1/responses`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ model: 'gpt-5' }),
    });
    assert.equal(resp.status, 200);
    assert.equal(proxy.metadata.forwarded_model, 'gpt-5');
    assert.equal(proxy.metadata.upstream_response_model, 'gpt-5-mini');
    assert.equal(proxy.metadata.success_path, '/v1/responses');
    assert.equal(proxy.metadata.used_fallback, false);
  } finally {
    await proxy.close();
    await closeServer(upstream.server);
  }
});

test('proxy marks fallback when chat succeeds after responses fails', async () => {
  const upstream = await listen(http.createServer((req, res) => {
    if (req.url === '/v1/responses') {
      res.writeHead(404, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ error: { message: 'unsupported' } }));
      return;
    }
    if (req.url === '/v1/chat/completions') {
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ model: 'gpt-5-chat-mini' }));
      return;
    }
    res.writeHead(404);
    res.end();
  }));

  const proxy = await startTrackingProxy({ targetBaseURL: upstream.url, kind: 'openai' });
  try {
    let resp = await fetch(`${proxy.baseURL}/v1/responses`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ model: 'gpt-5' }),
    });
    assert.equal(resp.status, 404);

    resp = await fetch(`${proxy.baseURL}/v1/chat/completions`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ model: 'gpt-5' }),
    });
    assert.equal(resp.status, 200);
    assert.equal(proxy.metadata.success_path, '/v1/chat/completions');
    assert.equal(proxy.metadata.used_fallback, true);
    assert.equal(proxy.metadata.forwarded_model, 'gpt-5');
    assert.equal(proxy.metadata.upstream_response_model, 'gpt-5-chat-mini');
  } finally {
    await proxy.close();
    await closeServer(upstream.server);
  }
});

test('proxy captures anthropic messages metadata', async () => {
  const upstream = await listen(http.createServer((req, res) => {
    assert.equal(req.url, '/v1/messages');
    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ model: 'claude-3-7-sonnet' }));
  }));

  const proxy = await startTrackingProxy({ targetBaseURL: upstream.url, kind: 'anthropic' });
  try {
    const resp = await fetch(`${proxy.baseURL}/v1/messages`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ model: 'claude-3-7-sonnet' }),
    });
    assert.equal(resp.status, 200);
    assert.equal(proxy.metadata.success_path, '/v1/messages');
    assert.equal(proxy.metadata.forwarded_model, 'claude-3-7-sonnet');
    assert.equal(proxy.metadata.upstream_response_model, 'claude-3-7-sonnet');
  } finally {
    await proxy.close();
    await closeServer(upstream.server);
  }
});

test('codex parser rebuilds output and ttft from JSONL', () => {
  const parser = createCodexOutputParser();
  parser.consumeStdout('{"type":"response.started","response":{"model":"gpt-5-mini"}}\n', 1002);
  parser.consumeStdout('{"type":"response.output_text.delta","delta":{"text":"Hel"}}\n', 1010);
  parser.consumeStdout('{"type":"response.output_text.delta","delta":{"text":"lo"}}\n', 1012);
  const result = parser.finish({ stdout: '', stderr: '', startedAt: 1000, endedAt: 1020 });
  assert.equal(result.output, 'Hello');
  assert.equal(result.ttft_ms, 10);
  assert.equal(result.metadata.upstream_response_model, 'gpt-5-mini');
});

test('claude parser rebuilds output and ttft from stream-json', () => {
  const parser = createClaudeOutputParser();
  parser.consumeStdout('{"type":"message_start","message":{"model":"claude-sonnet-4-5"}}\n', 2002);
  parser.consumeStdout('{"type":"content_block_delta","delta":{"text":"Hi"}}\n', 2008);
  parser.consumeStdout('{"type":"content_block_delta","delta":{"text":"!"}}\n', 2010);
  const result = parser.finish({ stdout: '', stderr: '', startedAt: 2000, endedAt: 2020 });
  assert.equal(result.output, 'Hi!');
  assert.equal(result.ttft_ms, 8);
  assert.equal(result.metadata.upstream_response_model, 'claude-sonnet-4-5');
});

test('finalizeRunnerResult does not invent upstream model for oauth flow', () => {
  const result = finalizeRunnerResult({
    ok: true,
    latency_ms: 80,
    ttft_ms: 0,
    output: 'OK',
    error: '',
    metadata: {},
  }, {
    requestedModel: 'gpt-5',
    capture: null,
  });
  assert.equal(result.forwarded_model, 'gpt-5');
  assert.equal(result.upstream_response_model, undefined);
  assert.equal(result.success_path, undefined);
  assert.equal(result.ttft_ms, 80);
});
