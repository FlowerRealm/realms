import type { McpServerV2 } from '../../api/admin/mcp';

import type { McpType, Row } from './mcpTypes';
import { serverType } from './mcpUtils';

export type KVRow = { k: string; v: string };

export type EditFormState = {
  id: string;
  type: McpType;

  command: string;
  args: string[];
  cwd: string;
  env: KVRow[];

  url: string;
  bearer_token_env_var: string;
  http_headers: KVRow[];

  startup_timeout_ms: string;
  tool_timeout_ms: string;
};

export function parseTimeoutFieldMs(raw: string): number {
  const v = raw.trim();
  if (!v) return 0;
  if (!/^[0-9]+$/.test(v)) return -1;
  const n = Number(v);
  if (!Number.isFinite(n) || n < 0) return -1;
  return Math.floor(n);
}

export function buildServer(type: McpType, form: EditFormState): McpServerV2 {
  const startup = parseTimeoutFieldMs(form.startup_timeout_ms);
  const tool = parseTimeoutFieldMs(form.tool_timeout_ms);
  const timeouts =
    (startup > 0 || tool > 0) && startup >= 0 && tool >= 0 ? { startup_ms: startup > 0 ? startup : undefined, tool_ms: tool > 0 ? tool : undefined } : undefined;

  if (type === 'stdio') {
    const args = form.args.map((x) => x.trim()).filter(Boolean);
    const env: Record<string, string> = {};
    for (const row of form.env) {
      const k = row.k.trim();
      const v = row.v;
      if (!k) continue;
      env[k] = v;
    }
    return {
      transport: 'stdio',
      stdio: {
        command: form.command.trim(),
        args: args.length ? args : undefined,
        cwd: form.cwd.trim() ? form.cwd.trim() : undefined,
        env: Object.keys(env).length ? env : undefined,
      },
      timeouts,
    };
  }

  const headers: Record<string, string> = {};
  for (const row of form.http_headers) {
    const k = row.k.trim();
    const v = row.v;
    if (!k) continue;
    headers[k] = v;
  }
  return {
    transport: type,
    http: {
      url: form.url.trim(),
      bearer_token_env_var: form.bearer_token_env_var.trim() ? form.bearer_token_env_var.trim() : undefined,
      headers: Object.keys(headers).length ? headers : undefined,
    },
    timeouts,
  };
}

export function initForm(row: Row | null): EditFormState {
  const s = row?.server;
  const t = serverType(s);
  const env = s?.transport === 'stdio' ? s.stdio?.env : undefined;
  const headers = s?.transport === 'http' || s?.transport === 'sse' ? s.http?.headers : undefined;
  return {
    id: row?.id || '',
    type: t,

    command: s?.stdio?.command || '',
    args: s?.stdio?.args || [],
    cwd: s?.stdio?.cwd || '',
    env: Object.entries(env || {}).map(([k, v]) => ({ k, v })),

    url: s?.http?.url || '',
    bearer_token_env_var: s?.http?.bearer_token_env_var || '',
    http_headers: Object.entries(headers || {}).map(([k, v]) => ({ k, v })),

    startup_timeout_ms: s?.timeouts?.startup_ms ? String(s.timeouts.startup_ms) : '',
    tool_timeout_ms: s?.timeouts?.tool_ms ? String(s.timeouts.tool_ms) : '',
  };
}

