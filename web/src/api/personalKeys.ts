import { api } from './client';
import type { APIResponse } from './types';

export type PersonalAPIKey = {
  id: number;
  name?: string | null;
  key_hint?: string | null;
  status: number;
  created_at: string;
  revoked_at?: string | null;
  last_used_at?: string | null;
};

type CreatedPersonalAPIKey = {
  key_id: number;
  key: string;
  key_hint?: string | null;
  name?: string | null;
};

export async function listPersonalAPIKeys() {
  const res = await api.get<APIResponse<PersonalAPIKey[]>>('/api/personal/keys');
  return res.data;
}

export async function createPersonalAPIKey(name?: string) {
  const res = await api.post<APIResponse<CreatedPersonalAPIKey>>('/api/personal/keys', { name: name || undefined });
  return res.data;
}

export async function revokePersonalAPIKey(keyID: number) {
  const res = await api.post<APIResponse<void>>(`/api/personal/keys/${keyID}/revoke`);
  return res.data;
}

