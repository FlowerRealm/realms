import type { McpServerV2 } from '../../api/admin/mcp';

import type { McpType, PerTarget, TargetKey } from './mcpTypes';

export function typeBadge(t: string) {
  if (t === 'stdio') return 'badge bg-light text-primary border';
  if (t === 'http') return 'badge bg-light text-success border';
  if (t === 'sse') return 'badge bg-light text-info border';
  return 'badge bg-light text-secondary border';
}

export function serverType(s: McpServerV2 | undefined): McpType {
  if (!s) return 'stdio';
  return s.transport;
}

export function mainSummary(s: McpServerV2 | undefined): string {
  if (!s) return '-';
  const t = serverType(s);
  if (t === 'stdio') return s.stdio?.command || '-';
  return s.http?.url || '-';
}

export function stableStringify(v: unknown): string {
  const seen = new WeakSet<object>();
  const walk = (x: unknown): unknown => {
    if (!x || typeof x !== 'object') return x;
    if (seen.has(x as object)) return null;
    seen.add(x as object);
    if (Array.isArray(x)) return x.map(walk);
    const obj = x as Record<string, unknown>;
    const keys = Object.keys(obj).sort();
    const out: Record<string, unknown> = {};
    for (const k of keys) out[k] = walk(obj[k]);
    return out;
  };
  return JSON.stringify(walk(v));
}

export function equalSpec(a: unknown, b: unknown): boolean {
  return stableStringify(a) === stableStringify(b);
}

export function targetEnabledForServer(s: McpServerV2 | undefined, k: TargetKey): boolean {
  if (!s?.targets) return true;
  const v = s.targets[k];
  if (typeof v !== 'boolean') return true;
  return v;
}

export function withoutTargets(s: McpServerV2 | undefined): unknown {
  if (!s) return s;
  const { targets, ...rest } = s as McpServerV2 & { targets?: unknown };
  void targets;
  return rest;
}

export function equalServerCore(a: McpServerV2 | undefined, b: McpServerV2 | undefined): boolean {
  return equalSpec(withoutTargets(a), withoutTargets(b));
}

export function stableHash(v: unknown): string {
  const s = stableStringify(v);
  let h = 2166136261;
  for (let i = 0; i < s.length; i += 1) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return (h >>> 0).toString(16);
}

export function chooseActualServer(actualByTarget: PerTarget): McpServerV2 | undefined {
  for (const k of ['codex', 'claude', 'gemini'] as const) {
    const s = actualByTarget[k];
    if (s) return s;
  }
  return undefined;
}

export function chooseActualServerForTargets(actualByTarget: PerTarget, enabled: Record<TargetKey, boolean>): McpServerV2 | undefined {
  for (const k of ['codex', 'claude', 'gemini'] as const) {
    if (!enabled[k]) continue;
    const s = actualByTarget[k];
    if (s) return s;
  }
  return chooseActualServer(actualByTarget);
}
