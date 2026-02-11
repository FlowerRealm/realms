import { api } from '../client';
import type { APIResponse } from '../types';

export type AdminMainGroup = {
  name: string;
  description?: string | null;
  status: number;
  created_at: string;
  updated_at: string;
};

export type AdminMainGroupSubgroup = {
  subgroup: string;
  priority: number;
  created_at: string;
  updated_at: string;
};

export type CreateAdminMainGroupRequest = {
  name: string;
  description?: string;
  status?: number;
};

export type UpdateAdminMainGroupRequest = {
  new_name?: string;
  description?: string;
  status: number;
};

export async function listAdminMainGroups() {
  const res = await api.get<APIResponse<AdminMainGroup[]>>('/api/admin/main-groups');
  return res.data;
}

export async function createAdminMainGroup(req: CreateAdminMainGroupRequest) {
  const res = await api.post<APIResponse<void>>('/api/admin/main-groups', req);
  return res.data;
}

export async function getAdminMainGroup(name: string) {
  const res = await api.get<APIResponse<AdminMainGroup>>(`/api/admin/main-groups/${encodeURIComponent(name)}`);
  return res.data;
}

export async function updateAdminMainGroup(name: string, req: UpdateAdminMainGroupRequest) {
  const res = await api.put<APIResponse<void>>(`/api/admin/main-groups/${encodeURIComponent(name)}`, req);
  return res.data;
}

export async function deleteAdminMainGroup(name: string) {
  const res = await api.delete<APIResponse<void>>(`/api/admin/main-groups/${encodeURIComponent(name)}`);
  return res.data;
}

export async function listAdminMainGroupSubgroups(name: string) {
  const res = await api.get<APIResponse<AdminMainGroupSubgroup[]>>(`/api/admin/main-groups/${encodeURIComponent(name)}/subgroups`);
  return res.data;
}

export async function replaceAdminMainGroupSubgroups(name: string, subgroups: string[]) {
  const res = await api.put<APIResponse<void>>(`/api/admin/main-groups/${encodeURIComponent(name)}/subgroups`, { subgroups });
  return res.data;
}
