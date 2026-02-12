import { api } from '../client';
import type { APIResponse } from '../types';

export type AdminOAuthApp = {
  id: number;
  client_id: string;
  name: string;
  status: number;
  status_label: string;
  has_secret: boolean;
  redirect_uris: string[];
};

export async function listAdminOAuthApps() {
  const res = await api.get<APIResponse<AdminOAuthApp[]>>('/api/admin/oauth-apps');
  return res.data;
}

type CreateAdminOAuthAppRequest = {
  name: string;
  status?: number;
  redirect_uris: string[];
};

export async function createAdminOAuthApp(req: CreateAdminOAuthAppRequest) {
  const res = await api.post<APIResponse<{ id: number; client_id: string; client_secret: string }>>('/api/admin/oauth-apps', req);
  return res.data;
}

export async function getAdminOAuthApp(appID: number) {
  const res = await api.get<APIResponse<AdminOAuthApp>>(`/api/admin/oauth-apps/${appID}`);
  return res.data;
}

type UpdateAdminOAuthAppRequest = {
  name: string;
  status: number;
  redirect_uris: string[];
};

export async function updateAdminOAuthApp(appID: number, req: UpdateAdminOAuthAppRequest) {
  const res = await api.put<APIResponse<void>>(`/api/admin/oauth-apps/${appID}`, req);
  return res.data;
}

export async function rotateAdminOAuthAppSecret(appID: number) {
  const res = await api.post<APIResponse<{ client_secret: string }>>(`/api/admin/oauth-apps/${appID}/rotate-secret`);
  return res.data;
}
