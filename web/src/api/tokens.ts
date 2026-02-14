import { api } from './client';
import type { APIResponse } from './types';

export type UserToken = {
  id: number;
  name?: string | null;
  token_hint?: string | null;
  status: number;
  created_at: string;
  revoked_at?: string | null;
  last_used_at?: string | null;
};

type CreatedToken = {
  token_id: number;
  token: string;
  token_hint?: string | null;
};

type RevealedToken = {
  token_id: number;
  token: string;
};

export async function listUserTokens() {
  const res = await api.get<APIResponse<UserToken[]>>('/api/token');
  return res.data;
}

export async function createUserToken(name?: string) {
  const res = await api.post<APIResponse<CreatedToken>>('/api/token', {
    name: name || undefined,
  });
  return res.data;
}

export async function rotateUserToken(tokenID: number) {
  const res = await api.post<APIResponse<CreatedToken>>(`/api/token/${tokenID}/rotate`);
  return res.data;
}

export async function revealUserToken(tokenID: number) {
  const res = await api.get<APIResponse<RevealedToken>>(`/api/token/${tokenID}/reveal`);
  return res.data;
}

export async function revokeUserToken(tokenID: number) {
  const res = await api.post<APIResponse<void>>(`/api/token/${tokenID}/revoke`);
  return res.data;
}

export async function deleteUserToken(tokenID: number) {
  const res = await api.delete<APIResponse<void>>(`/api/token/${tokenID}`);
  return res.data;
}

export type TokenChannelGroupOption = {
  name: string;
  description?: string | null;
  status: number;
  price_multiplier: string;
  user_group_priority: number;
};

export type TokenChannelGroupBinding = {
  channel_group_name: string;
  priority: number;
};

export type UserTokenChannelGroups = {
  token_id: number;
  user_group: string;
  allowed_channel_groups: TokenChannelGroupOption[];
  bindings: TokenChannelGroupBinding[];
  effective_bindings: TokenChannelGroupBinding[];
};

export async function getUserTokenChannelGroups(tokenID: number) {
  const res = await api.get<APIResponse<UserTokenChannelGroups>>(`/api/token/${tokenID}/channel-groups`);
  return res.data;
}

export async function replaceUserTokenChannelGroups(tokenID: number, channelGroups: string[]) {
  const res = await api.put<APIResponse<void>>(`/api/token/${tokenID}/channel-groups`, { channel_groups: channelGroups });
  return res.data;
}
