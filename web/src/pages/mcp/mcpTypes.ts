import type { McpServerV2 } from '../../api/admin/mcp';

export type TargetKey = 'codex' | 'claude' | 'gemini';
export type McpType = 'stdio' | 'http' | 'sse';
export type ImportSource = TargetKey | 'realms';
export type ImportPick = 'keep' | 'imported';

export type Row = {
  id: string;
  server: McpServerV2;
};

export type PerTarget = Partial<Record<TargetKey, McpServerV2>>;

export type UnionRow = {
  id: string;
  desired?: McpServerV2;
  chosen?: McpServerV2;
  actualByTarget: PerTarget;
  hasActual: boolean;
  actualConflict: boolean;
  status: 'synced' | 'new' | 'missing' | 'conflict' | 'disabled';
};

export type TargetInfo = Record<TargetKey, { path: string; exists: boolean; parse_error?: string; server_count?: number }>;
export type ScannedTargets = Record<TargetKey, { servers?: Record<string, McpServerV2> }>;
