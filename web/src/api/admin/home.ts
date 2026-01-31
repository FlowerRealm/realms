import { api } from '../client';
import type { APIResponse } from '../types';

export type AdminHomeStats = {
  users_count: number;
  channels_count: number;
  endpoints_count: number;
  requests_today: number;
  tokens_today: number;
  input_tokens_today: number;
  output_tokens_today: number;
  cost_today: string;
};

export type AdminHome = {
  admin_time_zone: string;
  stats: AdminHomeStats;
};

export async function getAdminHome() {
  const res = await api.get<APIResponse<AdminHome>>('/api/admin/home');
  return res.data;
}

