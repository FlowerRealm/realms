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
  avg_first_token_latency: string;
  tokens_per_second: string;
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
  account: string;
  status_code: string;
  latency_ms: string;
  first_token_latency_ms: string;
  tokens_per_second: string;
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

export type AdminUsageTimeSeriesPoint = {
  bucket: string;
  requests: number;
  tokens: number;
  committed_usd: number;
  cache_ratio: number;
  avg_first_token_latency: number;
  tokens_per_second: number;
};

export type AdminUsageTimeSeriesResponse = {
  admin_time_zone: string;
  start: string;
  end: string;
  granularity: 'hour' | 'day';
  points: AdminUsageTimeSeriesPoint[];
};

export type UsageEventDetail = {
  event_id: number;
  available: boolean;
  downstream_request_body?: string;
  upstream_request_body?: string;
  upstream_response_body?: string;
  pricing_breakdown?: UsageEventPricingBreakdown;
};

export type UsageEventGroupMultiplier = {
  group_name: string;
  multiplier: string;
};

export type UsageEventPricingBreakdown = {
  cost_source: 'committed' | 'reserved' | 'none' | string;
  cost_source_usd: string;

  model_public_id?: string;
  model_found: boolean;

  input_tokens_total: number;
  input_tokens_cached: number;
  input_tokens_billable: number;
  output_tokens_total: number;
  output_tokens_cached: number;
  output_tokens_billable: number;

  input_usd_per_1m: string;
  output_usd_per_1m: string;
  cache_input_usd_per_1m: string;
  cache_output_usd_per_1m: string;

  input_cost_usd: string;
  output_cost_usd: string;
  cache_input_cost_usd: string;
  cache_output_cost_usd: string;
  base_cost_usd: string;

  user_groups: string[];
  user_group_factors: UsageEventGroupMultiplier[];
  user_multiplier: string;
  subscription_group?: string;
  effective_multiplier: string;

  final_cost_usd: string;
  diff_from_source_usd: string;
};

export async function getAdminUsagePage(params: { start?: string; end?: string; limit?: number; before_id?: number; after_id?: number }) {
  const res = await api.get<APIResponse<AdminUsagePage>>('/api/admin/usage', { params });
  return res.data;
}

export async function getAdminUsageEventDetail(eventID: number) {
  const res = await api.get<APIResponse<UsageEventDetail>>(`/api/admin/usage/events/${eventID}/detail`);
  return res.data;
}

export async function getAdminUsageTimeSeries(params?: { start?: string; end?: string; granularity?: 'hour' | 'day' }) {
  const res = await api.get<APIResponse<AdminUsageTimeSeriesResponse>>('/api/admin/usage/timeseries', { params });
  return res.data;
}
