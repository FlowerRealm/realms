import type { McpServerV2 } from '../../api/admin/mcp';
import { stableHash as stableHashCore, stableStringify as stableStringifyCore } from '../../utils/stableHash';

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
  return stableStringifyCore(v);
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
  return stableHashCore(v);
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
