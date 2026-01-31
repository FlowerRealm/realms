import { api } from './client';
import type { APIResponse } from './types';

export type DashboardModels = Record<string, string[]>;

export type ManagedModel = {
  id: number;
  public_id: string;
  owned_by?: string | null;
  input_usd_per_1m: string;
  output_usd_per_1m: string;
  cache_input_usd_per_1m: string;
  cache_output_usd_per_1m: string;
  status: number;
  icon_url?: string | null;
};

export type UserManagedModel = {
  id: number;
  public_id: string;
  owned_by?: string | null;
  input_usd_per_1m: string;
  output_usd_per_1m: string;
  cache_input_usd_per_1m: string;
  cache_output_usd_per_1m: string;
  status: number;
  icon_url?: string | null;
};

export type PageInfo<T> = {
  page: number;
  page_size: number;
  total: number;
  items: T[];
};

export type ModelLibraryLookupResult = {
  owned_by: string;
  input_usd_per_1m: string;
  output_usd_per_1m: string;
  cache_input_usd_per_1m: string;
  cache_output_usd_per_1m: string;
  source: string;
  icon_url: string;
};

export type ImportModelPricingResult = {
  added: string[];
  updated: string[];
  unchanged: string[];
  failed: Record<string, string>;
};

export async function getDashboardModels() {
  const res = await api.get<APIResponse<DashboardModels>>('/api/models');
  return res.data;
}

export async function listUserModels() {
  const res = await api.get<APIResponse<string[]>>('/api/user/models');
  return res.data;
}

export async function listUserModelsDetail() {
  const res = await api.get<APIResponse<UserManagedModel[]>>('/api/user/models/detail');
  return res.data;
}

export async function listManagedModelsAdmin(page = 1, pageSize = 20) {
  const res = await api.get<APIResponse<PageInfo<ManagedModel>>>('/api/models/', {
    params: { p: page, page_size: pageSize },
  });
  return res.data;
}

export async function getManagedModelAdmin(id: number) {
  const res = await api.get<APIResponse<ManagedModel>>(`/api/models/${id}`);
  return res.data;
}

export async function createManagedModelAdmin(model: Omit<ManagedModel, 'id'>) {
  const res = await api.post<APIResponse<{ id: number }>>('/api/models/', model);
  return res.data;
}

export async function updateManagedModelAdmin(model: ManagedModel, statusOnly = false) {
  const res = await api.put<APIResponse<void>>('/api/models/', model, {
    params: statusOnly ? { status_only: 'true' } : undefined,
  });
  return res.data;
}

export async function deleteManagedModelAdmin(id: number) {
  const res = await api.delete<APIResponse<void>>(`/api/models/${id}`);
  return res.data;
}

export async function lookupModelFromLibraryAdmin(modelID: string) {
  const res = await api.post<APIResponse<ModelLibraryLookupResult>>('/api/models/library-lookup', { model_id: modelID });
  return res.data;
}

export async function importModelPricingAdmin(pricingJSON: string) {
  const res = await api.post<APIResponse<ImportModelPricingResult>>('/api/models/import-pricing', { pricing_json: pricingJSON });
  return res.data;
}
