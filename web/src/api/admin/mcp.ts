import { api } from '../client';
import type { APIResponse } from '../types';

export type McpStoreV2 = {
  version: number;
  servers: Record<string, McpServerV2>;
};

export type McpServerV2 = {
  transport: 'stdio' | 'http' | 'sse';
  stdio?: {
    command: string;
    args?: string[];
    cwd?: string;
    env?: Record<string, string>;
  };
  http?: {
    url: string;
    bearer_token_env_var?: string;
    headers?: Record<string, string>;
  };
  timeouts?: {
    startup_ms?: number;
    tool_ms?: number;
  };
};

export type AdminMcpState = {
  store_json: string;
  server_count: number;
  parse_error: string;
  migrated_from_legacy?: boolean;
  targets?: Record<string, AdminMcpTargetState>;
  apply_results?: AdminMcpApplyResult[];
  store?: McpStoreV2;
};

export type AdminMcpTargetState = {
  enabled: boolean;
  path: string;
  exists: boolean;
};

export type AdminMcpApplyResult = {
  target: string;
  path: string;
  enabled: boolean;
  changed: boolean;
  exists: boolean;
  error?: string;
};

export type AdminMcpExport = {
  config_json?: string;
  config_toml?: string;
};

export async function getAdminMcp() {
  const res = await api.get<APIResponse<AdminMcpState>>('/api/admin/mcp');
  return res.data;
}

export type UpdateAdminMcpRequest = {
  store: McpStoreV2;
  target_enabled?: Record<string, boolean>;
  apply_on_save?: boolean;
  force?: boolean;
};

export async function updateAdminMcp(req: UpdateAdminMcpRequest) {
  const res = await api.put<APIResponse<AdminMcpState>>('/api/admin/mcp', req);
  return res.data;
}

export async function exportAdminMcpClaude(platform?: string) {
  const q = platform ? `?platform=${encodeURIComponent(platform)}` : '';
  const res = await api.get<APIResponse<AdminMcpExport>>(`/api/admin/mcp/export/claude${q}`);
  return res.data;
}

export async function exportAdminMcpGemini() {
  const res = await api.get<APIResponse<AdminMcpExport>>('/api/admin/mcp/export/gemini');
  return res.data;
}

export async function exportAdminMcpCodex(platform?: string) {
  const q = platform ? `?platform=${encodeURIComponent(platform)}` : '';
  const res = await api.get<APIResponse<AdminMcpExport>>(`/api/admin/mcp/export/codex${q}`);
  return res.data;
}

export type ApplyAdminMcpRequest = {
  targets?: string[];
  remove_ids?: string[];
  force?: boolean;
};

export type ApplyAdminMcpResponse = {
  apply_results: AdminMcpApplyResult[];
};

export async function applyAdminMcp(req: ApplyAdminMcpRequest) {
  const res = await api.post<APIResponse<ApplyAdminMcpResponse>>('/api/admin/mcp/apply', req);
  return res.data;
}

export type ScanAdminMcpResponse = {
  targets: Record<
    string,
    {
      target?: string;
      path: string;
      exists: boolean;
      parse_error?: string;
      server_count: number;
      servers?: Record<string, McpServerV2>;
    }
  >;
};

export async function scanAdminMcp() {
  const res = await api.get<APIResponse<ScanAdminMcpResponse>>('/api/admin/mcp/scan');
  return res.data;
}

export type ParseAdminMcpRequest = {
  source: 'codex' | 'claude' | 'gemini' | 'realms';
  content: string;
};

export type ParseAdminMcpResponse = {
  store?: McpStoreV2;
  store_json?: string;
  server_count?: number;
};

export async function parseAdminMcp(req: ParseAdminMcpRequest) {
  const res = await api.post<APIResponse<ParseAdminMcpResponse>>('/api/admin/mcp/parse', req);
  return res.data;
}

export type ImportAdminMcpRequest = {
  source: 'codex' | 'claude' | 'gemini';
  mode: 'merge' | 'replace';
  apply_after: boolean;
};

export type ImportAdminMcpResponse = {
  store?: McpStoreV2;
  imported_from?: { source: string; path: string; count: number };
  apply_results?: AdminMcpApplyResult[];
};

export async function importAdminMcp(req: ImportAdminMcpRequest) {
  const res = await api.post<APIResponse<ImportAdminMcpResponse>>('/api/admin/mcp/import', req);
  return res.data;
}

export type DeleteAdminMcpRequest = {
  id: string;
  targets?: string[];
  force?: boolean;
};

export type DeleteAdminMcpResponse = {
  server_count: number;
  apply_results: AdminMcpApplyResult[];
};

export async function deleteAdminMcp(req: DeleteAdminMcpRequest) {
  const res = await api.post<APIResponse<DeleteAdminMcpResponse>>('/api/admin/mcp/delete', req);
  return res.data;
}
