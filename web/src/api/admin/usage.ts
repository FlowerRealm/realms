import { api } from '../client';
import type { APIResponse } from '../types';

export type AdminUsageWindow = {
  window: string;
  since: string;
  until: string;
  requests: number;
  tokens: number;
  input_tokens: number;
  output_tokens: number;
  cached_tokens: number;
  cache_ratio: string;
  rpm: string;
  tpm: string;
  committed_usd: string;
  reserved_usd: string;
  total_usd: string;
};

export type AdminUsageUser = {
  user_id: number;
  email: string;
  role: string;
  status: number;
  committed_usd: string;
  reserved_usd: string;
};

export type AdminUsageEvent = {
  id: number;
  time: string;
  user_id: number;
  user_email: string;
  endpoint: string;
  method: string;
  model: string;
  status_code: string;
  latency_ms: string;
  input_tokens: string;
  output_tokens: string;
  cached_tokens: string;
  request_bytes: string;
  response_bytes: string;
  cost_usd: string;
  state_label: string;
  state_badge_class: string;
  is_stream: boolean;
  upstream_channel_id: string;
  upstream_channel_name: string;
  request_id: string;
  error: string;
  error_class: string;
  error_message: string;
};

export type AdminUsagePage = {
  admin_time_zone: string;
  now: string;
  start: string;
  end: string;
  limit: number;
  window: AdminUsageWindow;
  top_users: AdminUsageUser[];
  events: AdminUsageEvent[];
  next_before_id?: number;
  prev_after_id?: number;
  cursor_active: boolean;
};

export type UsageEventDetail = {
  event_id: number;
  available: boolean;
  downstream_request_body?: string;
  upstream_request_body?: string;
  upstream_response_body?: string;
};

export async function getAdminUsagePage(params: { start?: string; end?: string; limit?: number; before_id?: number; after_id?: number }) {
  const res = await api.get<APIResponse<AdminUsagePage>>('/api/admin/usage', { params });
  return res.data;
}

export async function getAdminUsageEventDetail(eventID: number) {
  const res = await api.get<APIResponse<UsageEventDetail>>(`/api/admin/usage/events/${eventID}/detail`);
  return res.data;
}

