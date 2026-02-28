import { api } from '../client';
import type { APIResponse } from '../types';

export type SkillsTargetKey = 'codex' | 'claude' | 'gemini';

export type SkillV1 = {
  id: string;
  title: string;
  description?: string;
  prompt: string;
  install_as?: Partial<Record<SkillsTargetKey, string>>;
  per_target?: Partial<Record<SkillsTargetKey, { enabled?: boolean }>>;
};

export type SkillsStoreV1 = {
  version: number;
  skills: Record<string, SkillV1>;
};

export type SkillsTargetEnabledV1 = Partial<Record<SkillsTargetKey, boolean>>;

export type AdminSkillsTargetState = {
  enabled: boolean;
  path: string;
  exists: boolean;
};

export type AdminSkillsState = {
  store_json: string;
  skill_count: number;
  parse_error: string;
  targets?: Record<string, AdminSkillsTargetState>;
  store?: SkillsStoreV1;
  target_enabled?: SkillsTargetEnabledV1;
  target_enabled_json?: string;
  desired_hashes?: Record<string, Partial<Record<SkillsTargetKey, string>>>;
};

export async function getAdminSkills() {
  const res = await api.get<APIResponse<AdminSkillsState>>('/api/admin/skills');
  return res.data;
}

export type ScanAdminSkillsResponse = {
  targets: Record<
    string,
    {
      target?: string;
      path: string;
      exists: boolean;
      parse_error?: string;
      skill_count: number;
      skills?: Record<string, { name: string; path: string; sha256: string }>;
    }
  >;
};

export async function scanAdminSkills() {
  const res = await api.get<APIResponse<ScanAdminSkillsResponse>>('/api/admin/skills/scan');
  return res.data;
}

export type UpdateAdminSkillsRequest = {
  store: SkillsStoreV1;
  target_enabled?: Record<string, boolean>;
  apply_on_save?: boolean;
  force?: boolean;
};

export type SkillApplyResult = {
  id: string;
  target: string;
  name: string;
  path: string;
  enabled: boolean;
  changed: boolean;
  exists: boolean;
  error?: string;
};

export type SkillApplyConflict = {
  id: string;
  target: string;
  path: string;
  existing_sha256?: string;
  desired_sha256?: string;
  reason?: string;
};

export async function updateAdminSkills(req: UpdateAdminSkillsRequest) {
  const res = await api.put<
    APIResponse<{
      store_json?: string;
      skill_count?: number;
      store?: SkillsStoreV1;
      apply_results?: SkillApplyResult[];
      conflicts?: SkillApplyConflict[];
    }>
  >('/api/admin/skills', req);
  return res.data;
}

export type ApplyAdminSkillsRequest = {
  targets?: string[];
  remove_ids?: string[];
  force?: boolean;
  resolutions?: Array<{ id: string; target: string; action: 'keep' | 'overwrite' | 'rename'; name?: string }>;
};

export async function applyAdminSkills(req: ApplyAdminSkillsRequest) {
  const res = await api.post<
    APIResponse<{
      apply_results: SkillApplyResult[];
      conflicts?: SkillApplyConflict[];
      store?: SkillsStoreV1;
    }>
  >('/api/admin/skills/apply', req);
  return res.data;
}

export type ImportAdminSkillsRequest = {
  source: 'codex' | 'claude' | 'gemini';
  mode: 'merge' | 'replace';
  apply_after: boolean;
  force?: boolean;
};

export async function importAdminSkills(req: ImportAdminSkillsRequest) {
  const res = await api.post<
    APIResponse<{
      store_json?: string;
      skill_count?: number;
      store?: SkillsStoreV1;
      imported_from?: { source: string; path: string; count: number };
      apply_results?: SkillApplyResult[];
      conflicts?: SkillApplyConflict[];
    }>
  >('/api/admin/skills/import', req);
  return res.data;
}

export type DeleteAdminSkillsRequest = {
  id: string;
  targets?: string[];
  force?: boolean;
};

export async function deleteAdminSkills(req: DeleteAdminSkillsRequest) {
  const res = await api.post<
    APIResponse<{
      skill_count: number;
      apply_results: SkillApplyResult[];
      conflicts?: SkillApplyConflict[];
    }>
  >('/api/admin/skills/delete', req);
  return res.data;
}

export async function autoAdoptAdminSkills(req?: { targets?: string[] }) {
  const res = await api.post<
    APIResponse<{
      adopted_count: number;
      adopted_ids?: string[];
      conflicts?: Array<{ id: string; targets: string[]; reason: string }>;
      store?: SkillsStoreV1;
      store_json?: string;
    }>
  >('/api/admin/skills/auto_adopt', req || {});
  return res.data;
}
