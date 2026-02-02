import { api } from './client';
import type { APIResponse } from './types';

export type UsageWindow = {
  window: string;
  since: string;
  until: string;
  requests: number;
  tokens: number;
  input_tokens: number;
  output_tokens: number;
  cached_input_tokens: number;
  cached_output_tokens: number;
  cache_ratio: number;
  used_usd: string;
  committed_usd: string;
  reserved_usd: string;
  limit_usd: string;
  remaining_usd: string;
};

export type UsageWindowsResponse = {
  now: string;
  subscription?: {
    active: boolean;
    plan_name?: string;
    start_at?: string;
    end_at?: string;
  };
  windows: UsageWindow[];
};

export type UsageEvent = {
  id: number;
  time: string;
  request_id: string;
  endpoint?: string | null;
  method?: string | null;
  token_id: number;
  upstream_endpoint_id?: number | null;
  upstream_credential_id?: number | null;
  state: string;
  model?: string | null;
  input_tokens?: number | null;
  cached_input_tokens?: number | null;
  output_tokens?: number | null;
  cached_output_tokens?: number | null;
  reserved_usd: string;
  committed_usd: string;
  reserve_expires_at: string;
  status_code: number;
  latency_ms: number;
  error_class?: string | null;
  error_message?: string | null;
  is_stream: boolean;
  request_bytes: number;
  response_bytes: number;
  created_at: string;
  updated_at: string;
};

export type UsageEventsResponse = {
  events: UsageEvent[];
  next_before_id?: number | null;
};

export async function getUsageWindows(start?: string, end?: string) {
  const res = await api.get<APIResponse<UsageWindowsResponse>>('/api/usage/windows', {
    params: {
      start: start || undefined,
      end: end || undefined,
    },
  });
  return res.data;
}

export async function getUsageEvents(limit = 100, beforeID?: number, start?: string, end?: string) {
  const res = await api.get<APIResponse<UsageEventsResponse>>('/api/usage/events', {
    params: {
      limit,
      before_id: beforeID || undefined,
      start: start || undefined,
      end: end || undefined,
    },
  });
  return res.data;
}

export type UsageEventDetail = {
  event_id: number;
  available: boolean;
  downstream_request_body?: string;
  upstream_request_body?: string;
  upstream_response_body?: string;
};

export async function getUsageEventDetail(eventID: number) {
  const res = await api.get<APIResponse<UsageEventDetail>>(`/api/usage/events/${eventID}/detail`);
  return res.data;
}
